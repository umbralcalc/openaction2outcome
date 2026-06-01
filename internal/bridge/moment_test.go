package bridge

import (
	"math"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
	"github.com/umbralcalc/stochadex/pkg/inference"
)

// analyticLinearBridge computes the EXACT posterior over τ(query) for a mechanism
// that is linear in θ with basis φ(x), Gaussian anchors, a Gaussian prior, and the
// modular (diagonal-noise) θ fit. This is the ground-truth the moment calibrator
// must reproduce to round-off in the linear-Gaussian limit. It deliberately shares
// none of the calibrator's code path — it is a closed-form reference.
func analyticLinearBridge(t *testing.T, phi func(float64) []float64, anchors []Anchor, query float64, k Kernel, m0, pvar []float64) (central, betweenVar, withinVar float64) {
	t.Helper()
	n := len(anchors)
	d := len(m0)
	xs := make([]float64, n)
	mu := make([]float64, n)
	noise := make([]float64, n)
	for i, a := range anchors {
		v, _ := a.noiseVar()
		xs[i], mu[i], noise[i] = a.X, a.mean(), v
	}
	// GP pieces.
	gp, err := newGPConditioner(xs, noise, k, defaultJitter)
	if err != nil {
		t.Fatalf("conditioner: %v", err)
	}
	withinVar = gp.condVar(query)
	w := gp.condMeanWeights(query)

	// Design matrix Φ (n×d) and posterior over θ (modular: W = diag(1/σ²)).
	Phi := make([][]float64, n)
	for i := range xs {
		Phi[i] = phi(xs[i])
	}
	// Λ = ΦᵀWΦ + diag(1/pvar); rhs = ΦᵀWμ + diag(1/pvar) m0.
	Lambda := make([][]float64, d)
	rhs := make([]float64, d)
	for a := 0; a < d; a++ {
		Lambda[a] = make([]float64, d)
		for b := 0; b < d; b++ {
			var s float64
			for i := 0; i < n; i++ {
				s += Phi[i][a] * (1.0 / noise[i]) * Phi[i][b]
			}
			Lambda[a][b] = s
		}
		Lambda[a][a] += 1.0 / pvar[a]
		var r float64
		for i := 0; i < n; i++ {
			r += Phi[i][a] * (1.0 / noise[i]) * mu[i]
		}
		rhs[a] = r + (1.0/pvar[a])*m0[a]
	}
	cov, ok := invert(Lambda)
	if !ok {
		t.Fatal("singular Lambda")
	}
	thetaHat := matVec(cov, rhs)

	// g(θ) = φ(query)ᵀθ + wᵀ(μ − Φθ) = aᵀθ + wᵀμ, a = φ(query) − Φᵀw.
	phiQ := phi(query)
	a := make([]float64, d)
	for j := 0; j < d; j++ {
		var s float64
		for i := 0; i < n; i++ {
			s += Phi[i][j] * w[i]
		}
		a[j] = phiQ[j] - s
	}
	central = dot(a, thetaHat) + dot(w, mu)
	betweenVar = quad(cov, a)
	return central, betweenVar, withinVar
}

// TestMomentExactInLinearGaussianLimit is the headline guarantee of the
// deterministic causal layer: for a linear-in-θ mechanism the unscented
// moment-propagation calibrator reproduces the analytic posterior over τ(query) to
// round-off. The QuadraticMechanism m = θ0 + θ1 x + θ2 x² is linear in θ.
func TestMomentExactInLinearGaussianLimit(t *testing.T) {
	mech := NewQuadraticMechanism()
	phi := func(x float64) []float64 { return []float64{1, x, x * x} }
	priors, _ := mech.Priors()
	m0, pvar, ok := priorMoments(priors)
	if !ok {
		t.Fatal("priorMoments failed")
	}

	for seed := int64(1); seed <= 6; seed++ {
		anchors, _, k := syntheticProblem(seed)
		query := queryFor(seed)

		post, err := CalibrateMoment(mech, anchors, query, k, false)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		wantC, wantBetween, wantWithin := analyticLinearBridge(t, phi, anchors, query, k, m0, pvar)

		if math.Abs(post.Central-wantC) > 1e-7 {
			t.Errorf("seed %d: central %.10f want %.10f", seed, post.Central, wantC)
		}
		if math.Abs(post.ThetaVar-wantBetween) > 1e-7 {
			t.Errorf("seed %d: betweenVar %.10f want %.10f", seed, post.ThetaVar, wantBetween)
		}
		if math.Abs(post.GPVar-wantWithin) > 1e-9 {
			t.Errorf("seed %d: withinVar %.10f want %.10f", seed, post.GPVar, wantWithin)
		}
		if post.Rung != rungClosedForm {
			t.Errorf("seed %d: linear mechanism should earn the closed-form rung, got %q", seed, post.Rung)
		}
	}
}

// TestMomentDeterministic confirms the moment calibrator carries no RNG: identical
// inputs give byte-identical posteriors (re-mintability).
func TestMomentDeterministic(t *testing.T) {
	mech := NewQuadraticMechanism()
	anchors, _, k := syntheticProblem(3)
	query := queryFor(3)
	a, err := CalibrateMoment(mech, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CalibrateMoment(mech, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Central != b.Central || a.GPVar != b.GPVar || a.ThetaVar != b.ThetaVar {
		t.Errorf("moment calibration not deterministic: %+v vs %+v", a, b)
	}
}

// TestGatePassesLinearMechanism checks the tractability gate certifies the
// linear-Gaussian quadratic mechanism (zero curvature gap) and recommends the
// closed-form rung.
func TestGatePassesLinearMechanism(t *testing.T) {
	mech := NewQuadraticMechanism()
	anchors, _, k := syntheticProblem(2)
	query := queryFor(2)
	v, err := TractabilityGate(mech, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Pass {
		t.Fatalf("linear mechanism failed the gate: %+v", v)
	}
	if !v.Linear || v.Rung != rungClosedForm {
		t.Errorf("expected linear/closed-form, got linear=%v rung=%q", v.Linear, v.Rung)
	}
	if v.NonlinearityGap > 1e-6 {
		t.Errorf("linear mechanism should have ~0 nonlinearity gap, got %g", v.NonlinearityGap)
	}
}

// expMechanism is a deliberately nonlinear test mechanism, m(x;θ) = θ0 + exp(θ1·x),
// used only to confirm the tractability gate detects and rejects genuine curvature.
type expMechanism struct{}

func (expMechanism) Predict(x float64, theta []float64) float64 {
	return theta[0] + math.Exp(theta[1]*x)
}
func (expMechanism) ParamDim() int { return 2 }
func (expMechanism) Priors() ([]inference.Prior, []string) {
	return []inference.Prior{
		&inference.TruncatedNormalPrior{Mu: 0, Sigma: 2, Lo: -10, Hi: 10},
		&inference.TruncatedNormalPrior{Mu: 1, Sigma: 1, Lo: -5, Hi: 5},
	}, []string{"b0", "rate"}
}
func (expMechanism) ID() string      { return "exp-nonlinear-test" }
func (expMechanism) Version() string { return "test-1" }

// TestGateRejectsNonlinearMechanism confirms the gate detects genuine curvature: an
// exponential mechanism with a wide θ posterior (anchors declared with broad noise)
// produces an unscented-vs-linearised variance gap beyond tolerance, so the gate
// fails and routes to the deferred sampling path instead of minting a deterministic
// interval. CalibrateDeterministic must return a nil posterior in that case.
func TestGateRejectsNonlinearMechanism(t *testing.T) {
	mech := expMechanism{}
	rate := 1.4
	xs := []float64{-1.0, -0.3, 0.4, 1.0}
	sd := 5.0 // broad anchor uncertainty → wide θ posterior → real curvature
	anchors := make([]Anchor, len(xs))
	for i, x := range xs {
		mu := math.Exp(rate * x)
		anchors[i] = Anchor{MarkID: anchorID(i), X: x, Dist: schema.Distribution{
			Central: mu, StdDev: f64(sd),
			Interval: &schema.Interval{Level: 0.95, Lower: mu - 1.96*sd, Upper: mu + 1.96*sd}}}
	}
	k := SquaredExponential{SigmaF: 0.5, Lengthscale: 0.5}
	query := 0.1

	v, err := TractabilityGate(mech, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if v.Linear {
		t.Error("exponential mechanism wrongly detected as linear")
	}
	if v.Pass {
		t.Fatalf("gate should reject the curved mechanism, but passed: gap=%.3f skew=%.3f", v.NonlinearityGap, v.Skew)
	}
	if v.Rung != rungSampled {
		t.Errorf("failed gate should route to sampled rung, got %q", v.Rung)
	}

	res, err := CalibrateDeterministic(mech, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Posterior != nil {
		t.Error("CalibrateDeterministic must return a nil posterior when the gate fails")
	}
}
