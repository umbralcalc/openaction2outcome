package sbi

import (
	"math"
	"math/rand"
	"testing"
)

// syntheticFuzzy plants a known LATE (tau) and a first-stage jump (jump) in
// treatment probability at cutoff 0, with the encouraged side above. The outcome
// jumps only through D, so the reduced-form jump is tau*jump and LATE = tau.
func syntheticFuzzy(n int, tau, jump, slope, sigma float64, seed int64) []FuzzyPoint {
	rng := rand.New(rand.NewSource(seed))
	pts := make([]FuzzyPoint, n)
	for i := range pts {
		x := -1 + 2*rng.Float64()
		p := 0.2 + 0.1*x
		if x > 0 {
			p += jump
		}
		p = math.Max(0, math.Min(1, p))
		d := 0.0
		if rng.Float64() < p {
			d = 1
		}
		y := tau*d + slope*x + rng.NormFloat64()*sigma
		pts[i] = FuzzyPoint{X: x, D: d, Y: y}
	}
	return pts
}

func TestFuzzyRecoversLATEWithStrongFirstStage(t *testing.T) {
	pts := syntheticFuzzy(1500, 0.3, 0.4, 0.5, 0.3, 9)
	r := EstimateFuzzyBMA(pts, 0, DefaultFloorSpecs(), false, SMCConfig{NumParticles: 1500, NumRounds: 5, Seed: 1})

	if !r.FirstStage.Passed {
		t.Fatalf("a 0.4 first-stage jump should pass the strength gate (jump=%.3f sd=%.3f F=%.1f)",
			r.FirstStage.Jump, r.FirstStage.SD, r.FirstStage.FStat)
	}
	if math.Abs(r.FirstStage.Jump-0.4) > 0.15 {
		t.Errorf("first-stage jump %.3f far from planted 0.4", r.FirstStage.Jump)
	}
	if math.Abs(r.LATE.Central-0.3) > 0.12 {
		t.Errorf("LATE central %.3f far from planted 0.3", r.LATE.Central)
	}
	lo, hi := r.LATE.Interval(0.95)
	if !(lo < 0.3 && 0.3 < hi) {
		t.Errorf("95%% interval [%.3f, %.3f] should contain the true LATE 0.3", lo, hi)
	}
	// The honest interval folds in identification spread across specs.
	if r.LATE.BetweenVar < 0 || r.LATE.WithinVar <= 0 {
		t.Errorf("bad variance decomposition: within=%g between=%g", r.LATE.WithinVar, r.LATE.BetweenVar)
	}
}

func TestFuzzyWeakFirstStageFailsGate(t *testing.T) {
	pts := syntheticFuzzy(1500, 0.3, 0.04, 0.5, 0.3, 10)
	r := EstimateFuzzyBMA(pts, 0, DefaultFloorSpecs(), false, SMCConfig{NumParticles: 1500, NumRounds: 5, Seed: 1})
	if r.FirstStage.Passed {
		t.Fatalf("a 0.04 first-stage jump should NOT pass the strength gate (F=%.2f)", r.FirstStage.FStat)
	}
}

func TestFuzzyDeterministic(t *testing.T) {
	pts := syntheticFuzzy(800, 0.3, 0.4, 0.5, 0.3, 11)
	cfg := SMCConfig{NumParticles: 1000, NumRounds: 4, Seed: 2}
	a := EstimateFuzzyBMA(pts, 0, DefaultFloorSpecs(), false, cfg)
	b := EstimateFuzzyBMA(pts, 0, DefaultFloorSpecs(), false, cfg)
	if a.LATE.Central != b.LATE.Central || a.FirstStage.Jump != b.FirstStage.Jump {
		t.Fatalf("non-deterministic fuzzy estimate: %+v vs %+v", a.FirstStage, b.FirstStage)
	}
}
