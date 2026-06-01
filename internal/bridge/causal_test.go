package bridge

import (
	"math"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// TestSCMGraphMatchesAnalyticIntervention checks the stochadex graph computes the
// interventional effect τ(x) = αY + gCY·c0 + β·x exactly — i.e. the do(T=x) graph
// (C→T severed, T pinned to x) propagates to the analytic closed form.
func TestSCMGraphMatchesAnalyticIntervention(t *testing.T) {
	m := NewLinearSCMMechanism()
	beta, alphaY := 1.3, -0.4
	for _, x := range []float64{-2, -0.5, 0, 0.7, 2.5} {
		got := m.Predict(x, []float64{beta, alphaY})
		want := alphaY + m.GammaCY*m.C0 + beta*x
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("x=%g: graph τ=%.10f want %.10f", x, got, want)
		}
	}
}

// TestSCMDoDiffersFromObserve confirms the directed graph carries genuine
// interventional content: the do-slope (the causal β) differs from the confounded
// observational association slope. A covariance/kernel could only recover the
// latter — this is why the mechanism is a directed structural model.
func TestSCMDoDiffersFromObserve(t *testing.T) {
	m := NewLinearSCMMechanism()
	theta := []float64{1.0, 0.0}
	doSlope := m.InterventionalSlope(theta)
	obsSlope := m.ObservationalSlope(theta)
	if math.Abs(doSlope-obsSlope) < 1e-6 {
		t.Fatalf("do and observe slopes should differ under confounding, got do=%g obs=%g", doSlope, obsSlope)
	}
	// The back-door bias is positive here (both confounding coefficients positive).
	if obsSlope <= doSlope {
		t.Errorf("expected positive back-door bias, do=%g obs=%g", doSlope, obsSlope)
	}
}

// TestSCMDeterministicRemint confirms the graph run is byte-for-byte reproducible.
func TestSCMDeterministicRemint(t *testing.T) {
	m := NewLinearSCMMechanism()
	a := m.Predict(0.7, []float64{1.3, -0.4})
	b := m.Predict(0.7, []float64{1.3, -0.4})
	if a != b {
		t.Errorf("SCM graph not deterministic: %v vs %v", a, b)
	}
}

// TestSCMMomentCalibrationExact ties the causal layer together: with the SCM as the
// mechanism, the deterministic moment calibrator reproduces the analytic posterior
// over τ(query) to round-off, and the gate certifies the closed-form rung. This is
// the spec's "agreement with the analytic answer in the linear-Gaussian limit" for
// a genuinely causal mechanism.
func TestSCMMomentCalibrationExact(t *testing.T) {
	m := NewLinearSCMMechanism()
	betaStar, alphaStar := 1.1, -0.3
	tau := func(x float64) float64 { return alphaStar + m.GammaCY*m.C0 + betaStar*x }

	xs := []float64{-1.5, -0.4, 0.5, 1.6}
	sd := 0.1
	anchors := make([]Anchor, len(xs))
	for i, x := range xs {
		mu := tau(x)
		anchors[i] = Anchor{MarkID: anchorID(i), X: x, Dist: schema.Distribution{
			Central: mu, StdDev: f64(sd),
			Interval: &schema.Interval{Level: 0.95, Lower: mu - 1.96*sd, Upper: mu + 1.96*sd}}}
	}
	k := SquaredExponential{SigmaF: 0.3, Lengthscale: 0.6}
	query := 0.1

	// Analytic reference with basis φ(x) = [x, 1] (β multiplies x, αY is the
	// intercept; the fixed confounding offset gCY·c0 is folded in below).
	priors, _ := m.Priors()
	m0, pvar, _ := priorMoments(priors)
	// Shift the anchor means by the known confounding offset so the linear basis is
	// exactly m(x;θ) = β·x + αY (Predict adds gCY·c0; the reference must match it).
	offset := m.GammaCY * m.C0
	shifted := make([]Anchor, len(anchors))
	for i, a := range anchors {
		d := a.Dist
		d.Central = a.Dist.Central - offset
		shifted[i] = Anchor{MarkID: a.MarkID, X: a.X, Dist: d}
	}
	phi := func(x float64) []float64 { return []float64{x, 1} }
	wantC, wantBetween, wantWithin := analyticLinearBridge(t, phi, shifted, query, k, m0, pvar)
	wantC += offset // undo the shift on the central estimate

	post, err := CalibrateMoment(m, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(post.Central-wantC) > 1e-6 {
		t.Errorf("central %.10f want %.10f", post.Central, wantC)
	}
	if math.Abs(post.ThetaVar-wantBetween) > 1e-6 {
		t.Errorf("betweenVar %.10f want %.10f", post.ThetaVar, wantBetween)
	}
	if math.Abs(post.GPVar-wantWithin) > 1e-9 {
		t.Errorf("withinVar %.10f want %.10f", post.GPVar, wantWithin)
	}

	res, err := CalibrateDeterministic(m, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Posterior == nil || res.Verdict.Rung != rungClosedForm {
		t.Errorf("expected closed-form gated posterior, got verdict=%+v", res.Verdict)
	}
}
