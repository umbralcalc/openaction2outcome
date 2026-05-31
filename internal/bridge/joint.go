package bridge

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file is the JOINT calibrator: full-Bayesian SBI over (θ, δ) where the
// discrepancy δ is *sampled* through stochadex rather than analytically
// conditioned. It is the alternative to the modular Calibrate, built to measure
// what the modular factorisation actually costs.
//
// How δ is sampled in one SMC pass. The discrepancy at the anchors AND the query
// is a correlated Gaussian with prior covariance K_full (the kernel over the
// n anchors plus the query point). We whiten it: δ = L z with L = chol(K_full)
// and z ~ iid N(0,1). So the latent parameters are φ = (θ, z) with independent
// priors stochadex can sample directly, and the likelihood is
//
//	Σᵢ log N(μᵢ | m(xᵢ; θ) + (L z)_i , σᵢ²)
//
// over the anchor rows only — the query row of L z carries no observation, so its
// posterior is driven by the prior plus its correlation with the anchors, which
// is exactly GP conditioning expressed as sampling. Each posterior particle then
// yields one draw τ(query) = m(query; θ) + (L z)_query, and the posterior over
// τ(query) is the weighted empirical distribution of those draws — no Gaussian
// assumption, so a skewed/non-Gaussian joint posterior is represented faithfully.
//
// Unlike the modular calibrator, the θ fit here sees the full GP coupling (the
// discrepancy prior is in the model), so this is the no-"cut" version: the
// difference between the two on the same problem is the price of modularisation.
//
// It is deterministic given the SMC seed (same engine as Calibrate), so a joint
// bridge mark is still re-mintable byte-for-byte.

// CalibrateJoint is the joint analogue of Calibrate with the identical signature.
func CalibrateJoint(mech Mechanism, anchors []Anchor, query float64, k Kernel, cfg SMCConfig) (BridgePosterior, error) {
	var zero BridgePosterior
	if err := CheckBracketing(anchors, query); err != nil {
		return zero, err
	}

	n := len(anchors)
	xs := make([]float64, n)
	mu := make([]float64, n)
	noise := make([]float64, n)
	for i, a := range anchors {
		v, ok := a.noiseVar()
		if !ok {
			return zero, fmt.Errorf("bridge: anchor %q has no usable posterior width", a.MarkID)
		}
		xs[i], mu[i], noise[i] = a.X, a.mean(), v
	}

	// K_full over [anchors..., query], whitened by Cholesky. Jitter guards PD-ness.
	pts := append(append([]float64(nil), xs...), query)
	m1 := len(pts)
	kf := make([][]float64, m1)
	jit := defaultJitter * k.Variance()
	for i := 0; i < m1; i++ {
		kf[i] = make([]float64, m1)
		for j := 0; j < m1; j++ {
			kf[i][j] = k.Cov(pts[i], pts[j])
		}
		kf[i][i] += jit
	}
	L, ok := cholesky(kf)
	if !ok {
		return zero, fmt.Errorf("bridge: GP prior covariance not positive-definite (check kernel/anchor spacing)")
	}

	cloud, err := fitJointSMC(mech, xs, mu, noise, L, cfg)
	if err != nil {
		return zero, err
	}

	d := mech.ParamDim()
	taus := make([]float64, len(cloud.params))
	mPart := make([]float64, len(cloud.params)) // simulator component m(query;θ), for the variance split
	for p, phi := range cloud.params {
		theta := phi[:d]
		z := phi[d : d+m1]
		deltaQuery := dotRow(L, m1-1, z) // (L z) at the query row
		mq := mech.Predict(query, theta)
		mPart[p] = mq
		taus[p] = mq + deltaQuery
	}

	emp := newEmpirical(taus, cloud.weights)
	central := emp.central()
	// Total variance from the weighted particles.
	total := weightedVar(taus, cloud.weights, central)
	// Approximate the simulator/θ vs discrepancy split. The joint posterior does
	// not decompose orthogonally (θ and δ are correlated a posteriori), so we
	// report the simulator-driven spread Var_p[m(query;θ)] as ThetaVar and assign
	// the remainder to the GP/discrepancy term, clamped at zero.
	thetaVar := weightedVar(mPart, cloud.weights, weightedMean(mPart, cloud.weights))
	gpVar := total - thetaVar
	if gpVar < 0 {
		gpVar = 0
	}

	return BridgePosterior{
		Query:    query,
		Central:  central,
		TotalSD:  math.Sqrt(total),
		GPVar:    gpVar,
		ThetaVar: thetaVar,
		Kernel:   k.Name(),
		Method:   "joint",
		dist:     emp,
	}, nil
}

// jointLogLike is the stochadex Iteration for the joint model: state[0] is the
// Gaussian observation log-likelihood of the anchors under m(·;θ) + L z.
type jointLogLike struct {
	mech  Mechanism
	xs    []float64
	mu    []float64
	noise []float64
	l     [][]float64 // Cholesky factor over [anchors..., query]
	d     int         // θ dimension
}

func (r *jointLogLike) Configure(partitionIndex int, settings *simulator.Settings) {}

func (r *jointLogLike) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	phi := params.Get("phi")
	theta := phi[:r.d]
	z := phi[r.d:]
	var ll float64
	for i := range r.xs {
		delta := dotRow(r.l, i, z) // (L z)_i — discrepancy at anchor i
		m := r.mech.Predict(r.xs[i], theta)
		resid := r.mu[i] - m - delta
		ll += -0.5 * (math.Log(2*math.Pi*r.noise[i]) + resid*resid/r.noise[i])
	}
	return []float64{ll}
}

// fitJointSMC runs stochadex SMC over φ = (θ, z) and returns the weighted cloud.
// The plumbing mirrors fitMechanismSMC; only the parameter vector (θ plus the
// whitened GP latents) and the likelihood differ.
func fitJointSMC(mech Mechanism, xs, mu, noise []float64, L [][]float64, cfg SMCConfig) (thetaCloud, error) {
	thetaPriors, thetaNames := mech.Priors()
	d := mech.ParamDim()
	m1 := len(L) // n anchors + 1 query

	priors := make([]inference.Prior, 0, d+m1)
	names := make([]string, 0, d+m1)
	priors = append(priors, thetaPriors...)
	names = append(names, thetaNames...)
	for i := 0; i < m1; i++ {
		// Standard-normal whitened latent, truncated at ±8σ (negligible mass) so
		// it is a proper, bounded prior the SMC proposal handles cleanly.
		priors = append(priors, &inference.TruncatedNormalPrior{Mu: 0, Sigma: 1, Lo: -8, Hi: 8})
		names = append(names, fmt.Sprintf("z_%d", i))
	}
	dim := d + m1

	model := analysis.SMCParticleModel{
		Build: func(N, _ int) *analysis.SMCInnerSimConfig {
			parts := make([]*simulator.PartitionConfig, 0, N)
			loglikeParts := make([]string, N)
			fwd := make(map[string][]int, N)
			for p := 0; p < N; p++ {
				name := fmt.Sprintf("ll_%d", p)
				parts = append(parts, &simulator.PartitionConfig{
					Name:              name,
					Iteration:         &jointLogLike{mech: mech, xs: xs, mu: mu, noise: noise, l: L, d: d},
					Params:            simulator.NewParams(map[string][]float64{"phi": make([]float64, dim)}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 2,
					Seed:              0,
				})
				loglikeParts[p] = name
				idx := make([]int, dim)
				for j := 0; j < dim; j++ {
					idx[j] = p*dim + j
				}
				fwd[name+"/phi"] = idx
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
		return thetaCloud{}, fmt.Errorf("bridge: joint SMC returned no usable particle cloud")
	}
	for _, pp := range res.ParticleParams {
		if len(pp) < dim {
			return thetaCloud{}, fmt.Errorf("bridge: joint SMC particle has %d params, want %d", len(pp), dim)
		}
	}
	return thetaCloud{params: res.ParticleParams, weights: res.Weights}, nil
}

// dotRow returns (M x)_row for a lower-triangular M: Σ_j M[row][j] x[j].
func dotRow(m [][]float64, row int, x []float64) float64 {
	var s float64
	for j := range x {
		s += m[row][j] * x[j]
	}
	return s
}

func weightedMean(v, w []float64) float64 {
	var sw, s float64
	for i := range v {
		s += w[i] * v[i]
		sw += w[i]
	}
	if sw == 0 {
		return 0
	}
	return s / sw
}

func weightedVar(v, w []float64, mean float64) float64 {
	var sw, s float64
	for i := range v {
		d := v[i] - mean
		s += w[i] * d * d
		sw += w[i]
	}
	if sw == 0 {
		return 0
	}
	return s / sw
}
