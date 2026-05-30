package sbi

import (
	"math"
	"testing"
)

var fuzzyTestCfg = SMCConfig{NumParticles: 1500, NumRounds: 5, Seed: 1}

// The Wald estimator must respect sign: a negative true LATE comes back negative.
func TestFuzzyRecoversNegativeLATE(t *testing.T) {
	pts := syntheticFuzzy(1500, -0.3, 0.4, 0.5, 0.3, 17)
	r := EstimateFuzzyBMA(pts, 0, DefaultFloorSpecs(), false, fuzzyTestCfg)
	if !r.FirstStage.Passed {
		t.Fatalf("a 0.4 first stage should pass (F=%.1f)", r.FirstStage.FStat)
	}
	if math.Abs(r.LATE.Central-(-0.3)) > 0.12 {
		t.Errorf("LATE central %.3f far from the true -0.3", r.LATE.Central)
	}
	lo, hi := r.LATE.Interval(0.95)
	if !(lo < -0.3 && -0.3 < hi) {
		t.Errorf("interval [%.3f, %.3f] should contain -0.3", lo, hi)
	}
}

// The reported first-stage jump and reduced form should recover what was planted:
// first stage = jump, reduced form = tau*jump, and LATE = their ratio = tau.
func TestFuzzyFirstStageAndReducedFormRecovered(t *testing.T) {
	tau, jump := 0.3, 0.4
	r := EstimateFuzzyBMA(syntheticFuzzy(1500, tau, jump, 0.5, 0.3, 9), 0, DefaultFloorSpecs(), false, fuzzyTestCfg)
	if math.Abs(r.FirstStage.Jump-jump) > 0.12 {
		t.Errorf("first-stage jump %.3f far from planted %.2f", r.FirstStage.Jump, jump)
	}
	if math.Abs(r.ReducedFormCentral-tau*jump) > 0.06 {
		t.Errorf("reduced form %.3f far from tau*jump=%.3f", r.ReducedFormCentral, tau*jump)
	}
	// Wald identity holds approximately: LATE ~ reduced form / first stage.
	if math.Abs(r.LATE.Central-r.ReducedFormCentral/r.FirstStage.Jump) > 0.08 {
		t.Errorf("LATE %.3f should be ~ reducedForm/firstStage %.3f", r.LATE.Central, r.ReducedFormCentral/r.FirstStage.Jump)
	}
}

// The instrument's informativeness is governed by the ESTIMATED first stage, not
// the planted jump (a strong planted jump can still be estimated near zero at the
// cutoff in a small binary sample — which is exactly why the gate exists). The
// invariant that matters: whenever a sample PASSES the gate, its Wald LATE is
// informative — a bounded interval centred near the truth, not a blown-up ratio.
func TestFuzzyPassingGateImpliesInformativeLATE(t *testing.T) {
	cfg := SMCConfig{NumParticles: 800, NumRounds: 4, Seed: 1}
	passed := 0
	for _, seed := range []int64{1, 2, 3, 4, 5, 6} {
		r := EstimateFuzzyBMA(syntheticFuzzy(1500, 0.3, 0.6, 0.5, 0.3, seed), 0, DefaultFloorSpecs(), false, cfg)
		if !r.FirstStage.Passed {
			continue
		}
		passed++
		lo, hi := r.LATE.Interval(0.95)
		if hi-lo > 1.5 {
			t.Errorf("seed %d passed the gate but the LATE interval width %.2f is uninformative", seed, hi-lo)
		}
		if math.Abs(r.LATE.Central-0.3) > 0.2 {
			t.Errorf("seed %d passed the gate but LATE central %.2f is far from 0.3", seed, r.LATE.Central)
		}
	}
	if passed == 0 {
		t.Fatal("with a strong (0.6) planted jump, at least one sample should pass the gate")
	}
}
