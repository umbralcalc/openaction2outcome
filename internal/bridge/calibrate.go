// Package bridge implements Kennedy–O'Hagan Bayesian calibration specialised to a
// bridge geometry: a stochadex forward simulator m(x;θ) plus a Brownian-bridge-
// pinned Gaussian-process discrepancy δ(x), yielding a posterior over τ(query)
// that is narrow at the anchor pins, bulges between them, and is always bounded
// because the query is required to lie strictly between anchors (interpolation
// only). The simulator is never truth; the real identified anchors are the pins,
// the simulator + GP discrepancy is the span, and the honest interval is the
// posterior. This package carries the heavy stochadex dependency and stays under
// /internal; pkg/schema only describes a bridge mark, it never runs this.
package bridge

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// SMCConfig controls the SMC run over the mechanism parameters θ. It mirrors
// internal/sbi.SMCConfig so the determinism story (seed → reproducible particle
// cloud → reproducible posterior) is identical across the two estimators.
type SMCConfig struct {
	NumParticles int
	NumRounds    int
	Seed         uint64
}

// DefaultSMCConfig is a modest default suitable for a handful of anchors.
func DefaultSMCConfig() SMCConfig {
	return SMCConfig{NumParticles: 2000, NumRounds: 8, Seed: 1}
}

// Anchor is one identified mark used as a pin: its position on the policy
// variable and its posterior over τ at that point, consumed as a Gaussian
// observation (mean + variance) of the truth there.
type Anchor struct {
	MarkID string
	X      float64
	Dist   schema.Distribution
}

// mean is the anchor's posterior central estimate.
func (a Anchor) mean() float64 { return a.Dist.Central }

// noiseVar reduces the anchor's posterior to an observation variance for GP
// conditioning. A pin can never make the bridge more certain than the anchor's
// own honest interval, so this variance is the floor on the pinned uncertainty.
func (a Anchor) noiseVar() (float64, bool) {
	var lo, hi, level float64
	has := a.Dist.Interval != nil
	if has {
		lo, hi, level = a.Dist.Interval.Lower, a.Dist.Interval.Upper, a.Dist.Interval.Level
	}
	return impliedNoiseVar(a.Dist.StdDev, lo, hi, level, has, a.Dist.Samples)
}

// CheckBracketing returns nil iff query lies strictly between the anchors on the
// policy variable: at least one anchor strictly below and one strictly above.
// Enforced before any fitting — there is no extrapolation path to fall back to.
func CheckBracketing(anchors []Anchor, query float64) error {
	if len(anchors) < 2 {
		return fmt.Errorf("bridge: need >=2 anchors, got %d", len(anchors))
	}
	var below, above bool
	for _, a := range anchors {
		switch {
		case a.X < query:
			below = true
		case a.X > query:
			above = true
		}
	}
	if !below || !above {
		return fmt.Errorf("bridge: query %g not strictly bracketed by anchors (below=%v above=%v); extrapolation is out of scope", query, below, above)
	}
	return nil
}

// BridgePosterior is the calibrated posterior over τ(query): a weighted mixture
// of Gaussians over the θ-particles, with its total variance split into the GP
// discrepancy/pinning term and the simulator/θ term. It converts to the same
// schema.Distribution shape an identified mark uses.
type BridgePosterior struct {
	Query    float64
	Central  float64
	TotalSD  float64
	GPVar    float64 // within: GP conditional (discrepancy/pinning) variance
	ThetaVar float64 // between: spread of per-particle means (simulator/θ uncertainty)
	Kernel   string
	Method   string // "modular" (closed-form δ) or "joint" (sampled δ)
	dist     postShape
}

// Interval returns the central credible interval at the given level.
func (b BridgePosterior) Interval(level float64) (lo, hi float64) { return b.dist.interval(level) }

// Distribution renders the posterior as the wire-format honest interval, matching
// the identified-mark shape (Central + StdDev + Interval + Quantiles + Samples +
// an UncertaintyBudget that attributes width to the GP discrepancy vs the
// simulator/θ uncertainty). The Sampling slot carries the GP term and the
// Specification slot the θ term — the same "sampling vs identification" reading
// the RDD marks use.
func (b BridgePosterior) Distribution(level float64, quantileGrid []float64, nSamples int) schema.Distribution {
	lo, hi := b.dist.interval(level)
	sd := b.TotalSD
	qs := make([]schema.Quantile, 0, len(quantileGrid))
	for _, pv := range b.dist.quantiles(quantileGrid) {
		qs = append(qs, schema.Quantile{P: pv[0], Value: pv[1]})
	}
	gp, th := b.GPVar, b.ThetaVar
	return schema.Distribution{
		Central:           b.Central,
		StdDev:            &sd,
		Interval:          &schema.Interval{Level: level, Lower: lo, Upper: hi},
		Quantiles:         qs,
		Samples:           b.dist.samples(nSamples),
		UncertaintyBudget: &schema.UncertaintyBudget{Sampling: &gp, Specification: &th},
	}
}

// Calibrate is the main entry point: the MODULAR ("cut") calibrator. θ is fit to
// the anchors with a diagonal likelihood that ignores the discrepancy's prior
// covariance, then the GP discrepancy is conditioned on the residuals in closed
// form and τ(query) is marginalised over the θ-particle cloud. Cutting the
// discrepancy out of the θ fit is the standard modularisation that stops a
// flexible simulator from over-explaining the anchors. This is the shipped
// default — exact, deterministic, and robust.
func Calibrate(mech Mechanism, anchors []Anchor, query float64, k Kernel, cfg SMCConfig) (BridgePosterior, error) {
	return calibrateClosedForm(mech, anchors, query, k, cfg, false)
}

// CalibrateMarginal is the EXACT joint posterior, computed in closed form. Because
// the discrepancy δ is a GP and the anchors are Gaussian observations, δ can be
// marginalised analytically: the anchors given θ are μ ~ N(m(·;θ), K+Σ), so θ is
// fit with that GP *marginal likelihood* (no cut — the discrepancy's covariance is
// in the θ fit), and δ(query) is then conditioned in closed form exactly as in the
// modular case. This is the no-approximation joint over (θ, δ); the difference
// from Calibrate is purely the modular cut, and the difference from the sampled
// CalibrateJoint is purely Monte-Carlo / degeneracy.
func CalibrateMarginal(mech Mechanism, anchors []Anchor, query float64, k Kernel, cfg SMCConfig) (BridgePosterior, error) {
	return calibrateClosedForm(mech, anchors, query, k, cfg, true)
}

// calibrateClosedForm is shared by the modular and exact-joint calibrators. Only
// the θ likelihood differs (diagonal vs GP marginal); the δ conditioning and the
// θ-particle marginalisation are identical.
func calibrateClosedForm(mech Mechanism, anchors []Anchor, query float64, k Kernel, cfg SMCConfig, useMarginal bool) (BridgePosterior, error) {
	var zero BridgePosterior
	if err := CheckBracketing(anchors, query); err != nil {
		return zero, err
	}

	xs := make([]float64, len(anchors))
	mu := make([]float64, len(anchors))
	noise := make([]float64, len(anchors))
	for i, a := range anchors {
		v, ok := a.noiseVar()
		if !ok {
			return zero, fmt.Errorf("bridge: anchor %q has no usable posterior width (need std_dev, interval, or samples)", a.MarkID)
		}
		xs[i], mu[i], noise[i] = a.X, a.mean(), v
	}

	gp, err := newGPConditioner(xs, noise, k, defaultJitter)
	if err != nil {
		return zero, err
	}

	// The no-cut variant fits θ with the GP marginal likelihood, whose residual
	// precision is (K+Σ)⁻¹ — exactly the inverse the conditioner already holds.
	var ginv [][]float64
	method := "modular"
	if useMarginal {
		ginv = gp.ginv
		method = "joint-exact"
	}

	cloud, err := fitMechanismSMC(mech, xs, mu, noise, ginv, cfg)
	if err != nil {
		return zero, err
	}

	// θ-independent pieces of the GP conditioning, computed once.
	condVar := gp.condVar(query)
	w := gp.condMeanWeights(query) // (K+Σ)⁻¹ k⋆

	means := make([]float64, len(cloud.params))
	vars := make([]float64, len(cloud.params))
	for p, theta := range cloud.params {
		// residual r(θ) = μ − m(·;θ) at the anchors
		resid := make([]float64, len(xs))
		for i := range xs {
			resid[i] = mu[i] - mech.Predict(xs[i], theta)
		}
		deltaMean := dot(w, resid) // k⋆ᵀ(K+Σ)⁻¹ r
		means[p] = mech.Predict(query, theta) + deltaMean
		vars[p] = condVar
	}
	mix := gaussMixture{means: means, vars: vars, weights: cloud.weights}

	return BridgePosterior{
		Query:    query,
		Central:  mix.central(),
		TotalSD:  math.Sqrt(mix.withinVar() + mix.betweenVar()),
		GPVar:    mix.withinVar(),
		ThetaVar: mix.betweenVar(),
		Kernel:   k.Name(),
		Method:   method,
		dist:     mix,
	}, nil
}

// thetaCloud is the weighted particle representation of the θ posterior.
type thetaCloud struct {
	params  [][]float64
	weights []float64
}

// mechLogLike is a stochadex Iteration whose state[0] is the Gaussian
// log-likelihood of the anchors under the mechanism at the particle's forwarded
// θ. With ginv nil it is the modular diagonal likelihood Σᵢ logN(μᵢ | m(xᵢ;θ),
// σᵢ²); with ginv = (K+Σ)⁻¹ it is the GP marginal likelihood −½ rᵀ(K+Σ)⁻¹r
// (the discrepancy covariance folded into the θ fit — the no-cut / exact joint).
// The θ-independent log-determinant term is dropped: it is constant across
// particles and cancels in the SMC weights.
type mechLogLike struct {
	mech  Mechanism
	xs    []float64
	mu    []float64
	noise []float64
	ginv  [][]float64 // nil → diagonal likelihood; non-nil → GP marginal likelihood
}

func (r *mechLogLike) Configure(partitionIndex int, settings *simulator.Settings) {}

func (r *mechLogLike) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	theta := params.Get("theta")
	resid := make([]float64, len(r.xs))
	for i := range r.xs {
		resid[i] = r.mu[i] - r.mech.Predict(r.xs[i], theta)
	}
	if r.ginv != nil {
		return []float64{-0.5 * quad(r.ginv, resid)}
	}
	var ll float64
	for i := range r.xs {
		ll += -0.5 * (math.Log(2*math.Pi*r.noise[i]) + resid[i]*resid[i]/r.noise[i])
	}
	return []float64{ll}
}

// fitMechanismSMC runs stochadex SMC over θ given the anchors and returns the
// full weighted particle cloud (unlike sbi.fitSpecSMC, which keeps only the
// mean/cov). ginv selects the likelihood (nil = diagonal/cut, non-nil = GP
// marginal). The SMC plumbing mirrors fitSpecSMC so determinism and behaviour match.
func fitMechanismSMC(mech Mechanism, xs, mu, noise []float64, ginv [][]float64, cfg SMCConfig) (thetaCloud, error) {
	priors, names := mech.Priors()
	d := mech.ParamDim()

	model := analysis.SMCParticleModel{
		Build: func(N, dim int) *analysis.SMCInnerSimConfig {
			parts := make([]*simulator.PartitionConfig, 0, N)
			loglikeParts := make([]string, N)
			fwd := make(map[string][]int, N)
			for p := 0; p < N; p++ {
				name := fmt.Sprintf("ll_%d", p)
				parts = append(parts, &simulator.PartitionConfig{
					Name:              name,
					Iteration:         &mechLogLike{mech: mech, xs: xs, mu: mu, noise: noise, ginv: ginv},
					Params:            simulator.NewParams(map[string][]float64{"theta": make([]float64, dim)}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 2,
					Seed:              0,
				})
				loglikeParts[p] = name
				idx := make([]int, dim)
				for j := 0; j < dim; j++ {
					idx[j] = p*dim + j
				}
				fwd[name+"/theta"] = idx
			}
			return &analysis.SMCInnerSimConfig{
				Partitions: parts,
				Simulation: &simulator.SimulationConfig{
					OutputCondition:      &simulator.NilOutputCondition{},
					OutputFunction:       &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
					TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:        0.0,
				},
				LoglikePartitions: loglikeParts,
				ParamForwarding:   fwd,
			}
		},
	}

	res := analysis.RunSMCInference(analysis.AppliedSMCInference{
		ProposalName:  "proposal",
		SimName:       "sim",
		PosteriorName: "posterior",
		NumParticles:  cfg.NumParticles,
		NumRounds:     cfg.NumRounds,
		Priors:        priors,
		ParamNames:    names,
		Model:         model,
		Seed:          cfg.Seed,
	})
	if res == nil || len(res.ParticleParams) == 0 || len(res.Weights) != len(res.ParticleParams) {
		return thetaCloud{}, fmt.Errorf("bridge: SMC returned no usable particle cloud")
	}
	// Defensive: each particle must carry d params.
	for _, pp := range res.ParticleParams {
		if len(pp) < d {
			return thetaCloud{}, fmt.Errorf("bridge: SMC particle has %d params, want %d", len(pp), d)
		}
	}
	return thetaCloud{params: res.ParticleParams, weights: res.Weights}, nil
}
