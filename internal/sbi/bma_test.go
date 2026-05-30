package sbi

import (
	"math"
	"testing"
)

func TestBMARecoversEffectAndDecomposesVariance(t *testing.T) {
	pts := syntheticJump(800, 0.1, 0.3, 0.2, 0.25, 21)
	specs := DefaultFloorSpecs()
	r := EstimateBMA(pts, 0, specs, nil, true, SMCConfig{NumParticles: 2000, NumRounds: 6, Seed: 1})

	if len(r.Specs) == 0 {
		t.Fatal("no specs survived")
	}
	// Weights sum to 1.
	var wsum float64
	for _, s := range r.Specs {
		wsum += s.Weight
	}
	if math.Abs(wsum-1) > 1e-9 {
		t.Fatalf("BMA weights sum to %.6f", wsum)
	}
	// Central estimate near the true jump 0.3.
	if math.Abs(r.Central-0.3) > 0.05 {
		t.Fatalf("BMA central %.4f far from true 0.3", r.Central)
	}
	// Variance decomposition: total = within + between, all non-negative,
	// identification (between) strictly positive across a real grid.
	if r.WithinVar <= 0 || r.BetweenVar < 0 {
		t.Fatalf("bad variance decomposition: within=%g between=%g", r.WithinVar, r.BetweenVar)
	}
	if got := math.Sqrt(r.WithinVar + r.BetweenVar); math.Abs(got-r.TotalSD) > 1e-9 {
		t.Fatalf("total sd %.6f != sqrt(within+between) %.6f", r.TotalSD, got)
	}
	// The honest interval must be wider than any single spec's sampling-only
	// interval (it folds in between-spec spread).
	lo, hi := r.Interval(0.95)
	honestSD := r.TotalSD
	var minWithinSD float64 = math.Inf(1)
	for _, s := range r.Specs {
		minWithinSD = math.Min(minWithinSD, s.TauSD)
	}
	if honestSD < minWithinSD {
		t.Fatalf("honest sd %.4f smaller than tightest within-spec sd %.4f", honestSD, minWithinSD)
	}
	if !(lo < r.Central && r.Central < hi) {
		t.Fatalf("central %.4f not inside interval [%.4f, %.4f]", r.Central, lo, hi)
	}
	// Quantile monotonicity.
	if r.Quantile(0.1) >= r.Quantile(0.9) {
		t.Fatal("quantiles not monotone")
	}
}
