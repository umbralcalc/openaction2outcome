package bridge

import (
	"math"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// testCfg is a small, fast SMC config for CI.
var testCfg = SMCConfig{NumParticles: 600, NumRounds: 6, Seed: 1}

func anchor(id string, x, mu, sd float64) Anchor {
	return Anchor{
		MarkID: id,
		X:      x,
		Dist: schema.Distribution{
			Central:  mu,
			StdDev:   f64(sd),
			Interval: &schema.Interval{Level: 0.95, Lower: mu - 1.96*sd, Upper: mu + 1.96*sd},
		},
	}
}

// threeAnchors plants a known smooth curve and pins anchors on it.
func threeAnchors(curve func(float64) float64, sd float64) []Anchor {
	return []Anchor{
		anchor("A", -1, curve(-1), sd),
		anchor("B", 0, curve(0), sd),
		anchor("C", 1, curve(1), sd),
	}
}

func TestBracketingRejectsOutOfHull(t *testing.T) {
	as := threeAnchors(func(x float64) float64 { return x }, 0.1)
	if err := CheckBracketing(as, 1.5); err == nil {
		t.Fatal("query beyond the anchor hull must be rejected (no extrapolation)")
	}
	if err := CheckBracketing(as, 0.5); err != nil {
		t.Fatalf("bracketed query must be accepted; got %v", err)
	}
}

func TestVarianceCollapsesAtPinAndBulgesBetween(t *testing.T) {
	curve := func(x float64) float64 { return 0.5 + 0.3*x }
	as := threeAnchors(curve, 0.05)
	k := SquaredExponential{SigmaF: 0.5, Lengthscale: 0.5}

	// Near a pin (B at x=0) the SD should be small; midway between B and C it bulges.
	nearPin, err := Calibrate(NewQuadraticMechanism(), as, 0.001, k, testCfg)
	if err != nil {
		t.Fatal(err)
	}
	midBridge, err := Calibrate(NewQuadraticMechanism(), as, 0.5, k, testCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !(nearPin.GPVar < midBridge.GPVar) {
		t.Fatalf("GP variance must collapse at a pin and bulge between: near-pin=%g mid=%g", nearPin.GPVar, midBridge.GPVar)
	}
	// Bounded: mid-bridge GP variance never exceeds the kernel prior variance.
	if midBridge.GPVar > k.Variance()+1e-9 {
		t.Fatalf("GP variance must stay bounded by σf²=%g; got %g", k.Variance(), midBridge.GPVar)
	}
}

func TestRecoversLinearTruthBetweenAnchors(t *testing.T) {
	// A clean linear truth the quadratic simulator can fit: the bridge central
	// estimate at a query should land near the true value.
	curve := func(x float64) float64 { return 0.2 + 0.6*x }
	as := threeAnchors(curve, 0.05)
	k := SquaredExponential{SigmaF: 0.4, Lengthscale: 0.6}
	post, err := Calibrate(NewQuadraticMechanism(), as, 0.5, k, testCfg)
	if err != nil {
		t.Fatal(err)
	}
	want := curve(0.5)
	if math.Abs(post.Central-want) > 0.15 {
		t.Fatalf("bridge central %g should be near true %g", post.Central, want)
	}
	lo, hi := post.Interval(0.95)
	if !(lo <= want && want <= hi) {
		t.Fatalf("95%% interval [%g,%g] should contain true %g", lo, hi, want)
	}
}

func TestCalibrateIsDeterministic(t *testing.T) {
	curve := func(x float64) float64 { return 0.1 - 0.4*x + 0.2*x*x }
	as := threeAnchors(curve, 0.07)
	k := SquaredExponential{SigmaF: 0.5, Lengthscale: 0.5}
	a, err := Calibrate(NewQuadraticMechanism(), as, 0.3, k, testCfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Calibrate(NewQuadraticMechanism(), as, 0.3, k, testCfg)
	if err != nil {
		t.Fatal(err)
	}
	if a.Central != b.Central || a.TotalSD != b.TotalSD {
		t.Fatalf("two identical fits must be bit-identical: (%v,%v) vs (%v,%v)", a.Central, a.TotalSD, b.Central, b.TotalSD)
	}
}

func TestDistributionShapeIsValid(t *testing.T) {
	curve := func(x float64) float64 { return 0.3 + 0.2*x }
	as := threeAnchors(curve, 0.05)
	k := SquaredExponential{SigmaF: 0.4, Lengthscale: 0.6}
	post, err := Calibrate(NewQuadraticMechanism(), as, 0.5, k, testCfg)
	if err != nil {
		t.Fatal(err)
	}
	d := post.Distribution(0.95, []float64{0.025, 0.25, 0.5, 0.75, 0.975}, 50)
	if err := d.Validate(); err != nil {
		t.Fatalf("bridge posterior must produce a valid Distribution: %v", err)
	}
	if d.UncertaintyBudget == nil || d.UncertaintyBudget.Sampling == nil || d.UncertaintyBudget.Specification == nil {
		t.Fatal("bridge distribution must attribute width to GP (sampling) and θ (specification)")
	}
}

func TestLOAOSkipsEndpoints(t *testing.T) {
	curve := func(x float64) float64 { return 0.2 + 0.5*x }
	as := []Anchor{
		anchor("A", -1, curve(-1), 0.06),
		anchor("B", -0.3, curve(-0.3), 0.06),
		anchor("C", 0.3, curve(0.3), 0.06),
		anchor("D", 1, curve(1), 0.06),
	}
	k := SquaredExponential{SigmaF: 0.4, Lengthscale: 0.6}
	rep := LeaveOneAnchorOut(NewQuadraticMechanism(), as, k, 0.95, testCfg)
	var skipped, scored int
	for _, row := range rep.Rows {
		if row.Skipped {
			skipped++
		} else {
			scored++
		}
	}
	if skipped != 2 {
		t.Fatalf("the two endpoint anchors must be skipped; got %d skipped", skipped)
	}
	if scored != 2 {
		t.Fatalf("the two interior anchors must be scored; got %d scored", scored)
	}
}
