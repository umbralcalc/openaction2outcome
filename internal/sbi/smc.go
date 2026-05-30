// Package sbi is the simulation-based-inference estimator for a mark's
// honest interval. It replaces the plug-in fit with a Bayesian
// model-averaging posterior over the discontinuity effect tau:
//
//   - the local-polynomial RDD model is fit on each specification in a grid
//     (bandwidth x polynomial order x kernel);
//   - within each spec, stochadex SMC returns the posterior over the model
//     parameters (hence tau) AND the log marginal likelihood;
//   - specs are combined by Bayesian model averaging (weight proportional to
//     marginal likelihood x spec prior), giving a mixture-of-Gaussians posterior
//     over tau whose width decomposes exactly into within-spec (sampling) and
//     between-spec (identification) variance.
//
// That between-spec term is the identification uncertainty a plug-in method
// (fixed spec, sampling SE only) omits — the reason it under-covers, failing calibration.
//
// For speed each spec's data is compressed to the sufficient statistics of the
// kernel-weighted normal regression (A = Phi' W Phi, b = Phi' W y, c0 = y' W y),
// and a custom stochadex Iteration evaluates the exact Gaussian regression
// log-likelihood from them — so SMC cost scales with particles x rounds, not
// with the number of units.
package sbi

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"

	"github.com/umbralcalc/openaction2outcome/internal/rdd"
)

// Kernel selects the weighting of units by distance to the cutoff.
type Kernel int

const (
	Triangular Kernel = iota
	Boxcar
)

func (k Kernel) String() string {
	if k == Boxcar {
		return "boxcar"
	}
	return "triangular"
}

func (k Kernel) weight(x, h float64) float64 {
	ax := math.Abs(x)
	if ax >= h {
		return 0
	}
	if k == Boxcar {
		return 1
	}
	return 1 - ax/h // triangular
}

// Spec is one identification specification.
type Spec struct {
	H      float64
	Order  int
	Kernel Kernel
}

func (s Spec) Label() string {
	return fmt.Sprintf("h=%.2g,order=%d,kernel=%s", s.H, s.Order, s.Kernel)
}

// tauIndex is the position of tau in the parameter vector theta = [alpha, tau,
// control slopes..., treated slopes...].
const tauIndex = 1

// specFit holds the sufficient statistics and closed-form summaries for one spec.
type specFit struct {
	spec     Spec
	d        int         // parameter dimension = 2 + 2*order
	A        [][]float64 // Phi' W Phi
	Ainv     [][]float64 // (Phi' W Phi)^-1
	b        []float64   // Phi' W y
	c0       float64     // y' W y
	sigma2   float64     // plug-in residual variance (profiled per spec)
	m        int         // units in window with positive weight
	sumLogW  float64     // sum of log weights (enters the Gaussian normaliser)
	thetaHat []float64   // WLS point estimate (closed form)
	ok       bool
}

// designRow builds the local-polynomial design row for a unit at signed
// distance x = X - cutoff. theta = [alpha, tau, ctrl poly(order), treat poly(order)].
// treated == true means the unit is on the action side of the cutoff.
func designRow(x float64, treated bool, order int) []float64 {
	d := 2 + 2*order
	row := make([]float64, d)
	row[0] = 1 // alpha: control intercept at the cutoff
	if treated {
		row[1] = 1 // tau: jump to the treated intercept
	}
	xp := 1.0
	for k := 1; k <= order; k++ {
		xp *= x
		if treated {
			row[2+order+(k-1)] = xp // treated-side slope columns
		} else {
			row[2+(k-1)] = xp // control-side slope columns
		}
	}
	return row
}

// buildSpecFit assembles the kernel-weighted normal-equation sufficient
// statistics and the plug-in posterior summaries for one spec. treatedBelow
// selects which side of the cutoff receives the action (true for a floor design).
func buildSpecFit(pts []rdd.Point, cutoff float64, s Spec, treatedBelow bool) specFit {
	d := 2 + 2*s.Order
	A := make([][]float64, d)
	for i := range A {
		A[i] = make([]float64, d)
	}
	b := make([]float64, d)
	var c0, sumLogW float64
	var m int
	for _, p := range pts {
		x := p.X - cutoff
		w := s.Kernel.weight(x, s.H)
		if w <= 0 {
			continue
		}
		treated := (x < 0) == treatedBelow
		row := designRow(x, treated, s.Order)
		for i := 0; i < d; i++ {
			b[i] += w * row[i] * p.Y
			for j := 0; j < d; j++ {
				A[i][j] += w * row[i] * row[j]
			}
		}
		c0 += w * p.Y * p.Y
		sumLogW += math.Log(w)
		m++
	}
	if m <= d+1 {
		return specFit{spec: s, d: d, ok: false}
	}
	Ainv, ok := invert(A)
	if !ok {
		return specFit{spec: s, d: d, ok: false}
	}
	thetaHat := matVec(Ainv, b)
	// Weighted residual sum of squares = c0 - 2 theta'b + theta'A theta, at thetaHat
	// this reduces to c0 - theta'b.
	rss := c0 - dot(thetaHat, b)
	sigma2 := rss / float64(m-d)
	if sigma2 <= 0 || math.IsNaN(sigma2) {
		return specFit{spec: s, d: d, ok: false}
	}
	return specFit{
		spec: s, d: d, A: A, Ainv: Ainv, b: b, c0: c0,
		sigma2: sigma2, m: m, sumLogW: sumLogW, thetaHat: thetaHat, ok: true,
	}
}

// logLike evaluates the exact kernel-weighted Gaussian regression log-likelihood
// at theta, using the precomputed sufficient statistics. Spec-dependent constants
// (m, sigma2, sum log w) are included so log marginal likelihoods are comparable
// across specs for Bayesian model averaging.
func (sf specFit) logLike(theta []float64) float64 {
	quadForm := sf.c0 - 2*dot(theta, sf.b) + quad(sf.A, theta)
	return -0.5 * (float64(sf.m)*math.Log(2*math.Pi*sf.sigma2) - sf.sumLogW + quadForm/sf.sigma2)
}

// exactTau returns the closed-form (flat-prior, plug-in sigma2) posterior for tau:
// mean = WLS estimate, variance = sigma2 * (A^-1)_{tau,tau}. Used to validate SMC.
func (sf specFit) exactTau() (mean, variance float64) {
	return sf.thetaHat[tauIndex], sf.sigma2 * sf.Ainv[tauIndex][tauIndex]
}

// regLogLike is a stochadex Iteration whose state[0] is the full-data regression
// log-likelihood at the particle's forwarded theta. One inner step suffices.
type regLogLike struct {
	sf specFit
}

func (r *regLogLike) Configure(partitionIndex int, settings *simulator.Settings) {}

func (r *regLogLike) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	theta := params.Get("theta")
	return []float64{r.sf.logLike(theta)}
}

// SMCConfig controls the per-spec SMC run.
type SMCConfig struct {
	NumParticles int
	NumRounds    int
	Seed         uint64
}

// DefaultSMCConfig is a reasonable default for the floor-standards model.
func DefaultSMCConfig() SMCConfig {
	return SMCConfig{NumParticles: 4000, NumRounds: 8, Seed: 1}
}

// priorsFor builds wide, proper, spec-independent priors over theta so that the
// SMC posterior tracks the likelihood and marginal likelihoods are comparable
// across specs. Bounds are generous for Progress 8 (scores broadly within +-4).
func priorsFor(order int) ([]inference.Prior, []string) {
	d := 2 + 2*order
	priors := make([]inference.Prior, d)
	names := make([]string, d)
	priors[0] = &inference.UniformPrior{Lo: -4, Hi: 4} // alpha
	names[0] = "alpha"
	priors[1] = &inference.UniformPrior{Lo: -4, Hi: 4} // tau
	names[1] = "tau"
	for k := 1; k <= order; k++ {
		priors[1+k] = &inference.UniformPrior{Lo: -10, Hi: 10}
		names[1+k] = fmt.Sprintf("ctrl_b%d", k)
		priors[1+order+k] = &inference.UniformPrior{Lo: -10, Hi: 10}
		names[1+order+k] = fmt.Sprintf("treat_b%d", k)
	}
	return priors, names
}

// specPosterior is the within-spec result fed to the BMA layer.
type specPosterior struct {
	spec    Spec
	tauMean float64
	tauVar  float64
	logZ    float64
	ok      bool
}

// fitSpecSMC runs stochadex SMC for one spec and extracts the tau posterior and
// log marginal likelihood.
func fitSpecSMC(sf specFit, cfg SMCConfig) specPosterior {
	if !sf.ok {
		return specPosterior{spec: sf.spec, ok: false}
	}
	priors, names := priorsFor(sf.spec.Order)

	model := analysis.SMCParticleModel{
		Build: func(N, d int) *analysis.SMCInnerSimConfig {
			parts := make([]*simulator.PartitionConfig, 0, N)
			loglikeParts := make([]string, N)
			fwd := make(map[string][]int, N)
			for p := 0; p < N; p++ {
				name := fmt.Sprintf("ll_%d", p)
				parts = append(parts, &simulator.PartitionConfig{
					Name:              name,
					Iteration:         &regLogLike{sf: sf},
					Params:            simulator.NewParams(map[string][]float64{"theta": make([]float64, d)}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 2,
					Seed:              0,
				})
				loglikeParts[p] = name
				idx := make([]int, d)
				for j := 0; j < d; j++ {
					idx[j] = p*d + j
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
	if res == nil || len(res.PosteriorMean) <= tauIndex {
		return specPosterior{spec: sf.spec, ok: false}
	}
	d := sf.d
	return specPosterior{
		spec:    sf.spec,
		tauMean: res.PosteriorMean[tauIndex],
		tauVar:  res.PosteriorCov[tauIndex*d+tauIndex],
		logZ:    res.LogMarginalLik,
		ok:      true,
	}
}
