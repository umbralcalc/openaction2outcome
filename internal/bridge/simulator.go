package bridge

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/inference"
)

// Mechanism is a forward model m(x; θ) of a policy mechanism: it maps a point x
// on the policy variable and a parameter vector θ to the predicted effect at x.
// It carries the mechanistic content — interactions, feedback, dynamics — that a
// single identified mark cannot. The simulator is NEVER the source of truth; the
// real identified anchors are the pins and the simulator + GP discrepancy is the
// span between them.
//
// Real mechanisms wrap a stochadex simulator (its forward run, parameterised by θ
// via the same ParamForwarding idiom internal/sbi uses). The synthetic mechanism
// below is an analytic implementation used to validate the calibration machinery
// against a known τ(x) before any real mechanism is wired in.
type Mechanism interface {
	// Predict returns m(x; θ). θ has length ParamDim().
	Predict(x float64, theta []float64) float64
	// ParamDim is the dimension of θ.
	ParamDim() int
	// Priors returns proper, wide priors over θ and their names, in the same
	// idiom as sbi.priorsFor — required for SMC and for comparable marginal
	// likelihoods.
	Priors() ([]inference.Prior, []string)
	// ID and Version identify the mechanism for provenance / re-mintability.
	ID() string
	Version() string
}

// QuadraticMechanism is the synthetic forward model used for machinery
// validation: m(x; θ) = θ0 + θ1·x + θ2·x². It is deliberately simple so the GP
// discrepancy has real work to do — the synthetic study plants a true curve
// τ*(x) = m(x; θ*) + δ*(x) with a non-zero δ* (e.g. a smooth bump) the quadratic
// cannot represent, so a passing bridge proves the discrepancy term earns its
// keep rather than the simulator doing all the lifting.
type QuadraticMechanism struct {
	version string
}

// NewQuadraticMechanism returns the synthetic quadratic mechanism.
func NewQuadraticMechanism() QuadraticMechanism { return QuadraticMechanism{version: "synthetic-1"} }

func (m QuadraticMechanism) Predict(x float64, theta []float64) float64 {
	return theta[0] + theta[1]*x + theta[2]*x*x
}

func (m QuadraticMechanism) ParamDim() int { return 3 }

func (m QuadraticMechanism) Priors() ([]inference.Prior, []string) {
	return []inference.Prior{
		&inference.UniformPrior{Lo: -10, Hi: 10}, // intercept
		&inference.UniformPrior{Lo: -10, Hi: 10}, // linear
		&inference.UniformPrior{Lo: -10, Hi: 10}, // quadratic
	}, []string{"b0", "b1", "b2"}
}

func (m QuadraticMechanism) ID() string      { return "quadratic-synthetic" }
func (m QuadraticMechanism) Version() string { return m.version }

// TrueCurve evaluates the synthetic mechanism's planted ground truth τ*(x) =
// quadratic(θ*) + a smooth Gaussian bump the quadratic cannot capture. It is used
// ONLY by the synthetic study to score recovery; it is never part of a real fit.
func TrueCurve(x float64, thetaStar []float64, bumpAmp, bumpCentre, bumpWidth float64) float64 {
	base := thetaStar[0] + thetaStar[1]*x + thetaStar[2]*x*x
	d := x - bumpCentre
	return base + bumpAmp*math.Exp(-(d*d)/(2*bumpWidth*bumpWidth))
}
