package bridge

import (
	"math"
	"testing"
)

// fourAnchors plants a known curve on four anchors (so the quadratic mechanism
// cannot exactly interpolate — the discrepancy has work to do).
func fourAnchors(curve func(float64) float64, sd float64) []Anchor {
	return []Anchor{
		anchor("A", -1, curve(-1), sd),
		anchor("B", -0.35, curve(-0.35), sd),
		anchor("C", 0.35, curve(0.35), sd),
		anchor("D", 1, curve(1), sd),
	}
}

func TestExactJointAgreesWithModular(t *testing.T) {
	// The exact (closed-form) joint and the modular cut should nearly coincide on
	// a Gaussian problem — the modular cut costs little.
	curve := func(x float64) float64 { return 0.3 + 0.5*x }
	as := fourAnchors(curve, 0.06)
	k := SquaredExponential{SigmaF: 0.4, Lengthscale: 0.6}
	cfg := SMCConfig{NumParticles: 1500, NumRounds: 8, Seed: 1}
	q := 0.6

	mod, err := Calibrate(NewQuadraticMechanism(), as, q, k, cfg)
	if err != nil {
		t.Fatal(err)
	}
	exact, err := CalibrateMarginal(NewQuadraticMechanism(), as, q, k, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(mod.Central-exact.Central) > 0.1 {
		t.Fatalf("exact joint and modular cut should nearly agree: %.4f vs %.4f", mod.Central, exact.Central)
	}
	// Both should cover the known truth at 95%.
	for _, p := range []BridgePosterior{mod, exact} {
		lo, hi := p.Interval(0.95)
		if !(lo <= curve(q) && curve(q) <= hi) {
			t.Fatalf("%s interval [%.4f,%.4f] should cover true %.4f", p.Method, lo, hi, curve(q))
		}
	}
}

func TestSampledJointIsDeterministic(t *testing.T) {
	curve := func(x float64) float64 { return 0.2 - 0.3*x }
	as := fourAnchors(curve, 0.07)
	k := SquaredExponential{SigmaF: 0.5, Lengthscale: 0.5}
	cfg := SMCConfig{NumParticles: 400, NumRounds: 5, Seed: 3}
	a, err := CalibrateJoint(NewQuadraticMechanism(), as, 0.5, k, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CalibrateJoint(NewQuadraticMechanism(), as, 0.5, k, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a.Central != b.Central || a.TotalSD != b.TotalSD {
		t.Fatalf("sampled joint must be deterministic given the seed: (%v,%v) vs (%v,%v)", a.Central, a.TotalSD, b.Central, b.TotalSD)
	}
}

func TestComparisonShowsSampledJointUnderCovers(t *testing.T) {
	if testing.Short() {
		t.Skip("comparison study is slow; skipped in -short")
	}
	cmp := RunBridgeComparison(30, 7, SMCConfig{NumParticles: 500, NumRounds: 6, Seed: 1})
	byName := map[string]MethodCoverage{}
	for _, m := range cmp.Methods {
		byName[m.Method] = m
	}
	idx95 := len(cmp.Levels) - 1 // 0.95 is last

	mod := byName["modular(cut)"]
	exact := byName["joint-exact(closed-form)"]
	sampled := byName["joint-sampled(stochadex GP)"]

	// Closed-form methods track nominal; the modular cut and the exact joint agree.
	if mod.Coverage[idx95] < 0.8 {
		t.Fatalf("modular recovery@0.95 should track nominal; got %.3f", mod.Coverage[idx95])
	}
	if exact.Coverage[idx95] < 0.8 {
		t.Fatalf("exact-joint recovery@0.95 should track nominal; got %.3f", exact.Coverage[idx95])
	}
	// The sampled joint degenerates: it under-covers badly (overconfident).
	if sampled.Coverage[idx95] > 0.6 {
		t.Fatalf("expected the sampled joint to UNDER-cover (the finding); got %.3f", sampled.Coverage[idx95])
	}
	// And its intervals are far narrower than the closed-form ones.
	if !(sampled.MeanWidth[idx95] < 0.5*exact.MeanWidth[idx95]) {
		t.Fatalf("sampled-joint intervals should be much narrower (overconfident): sampled=%.3f exact=%.3f",
			sampled.MeanWidth[idx95], exact.MeanWidth[idx95])
	}
}
