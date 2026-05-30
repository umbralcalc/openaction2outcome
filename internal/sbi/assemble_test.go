package sbi

import (
	"math"
	"testing"
)

// weightBySpec maps each spec to its assembled BMA weight.
func weightBySpec(r BMAResult) map[Spec]float64 {
	m := make(map[Spec]float64, len(r.Specs))
	for _, s := range r.Specs {
		m[s.Spec] = s.Weight
	}
	return m
}

// Within a single bandwidth the specs share the same data, so they are weighted
// by marginal likelihood (logZ): a spec with a higher logZ gets proportionally
// more weight (softmax of logZ).
func TestAssembleWeightsByMarginalLikelihoodWithinBandwidth(t *testing.T) {
	ests := []specEstimate{
		{spec: Spec{H: 0.5, Order: 1, Kernel: Boxcar}, mean: 1, variance: 0.01, logZ: 0},
		{spec: Spec{H: 0.5, Order: 2, Kernel: Boxcar}, mean: 2, variance: 0.01, logZ: math.Log(3)},
	}
	r := assembleBMA(ests)
	w := weightBySpec(r)
	// exp(0):exp(log 3) = 1:3 -> 0.25 / 0.75
	if math.Abs(w[ests[0].spec]-0.25) > 1e-9 || math.Abs(w[ests[1].spec]-0.75) > 1e-9 {
		t.Fatalf("within-bandwidth weights should track exp(logZ): %v", w)
	}
}

// THE core property (the fix for the original collapse): across bandwidths the
// data differs, so marginal likelihood is NOT comparable and bandwidths are
// averaged UNIFORMLY — a hugely higher logZ in one bandwidth must not dominate.
func TestAssembleUniformAcrossBandwidths(t *testing.T) {
	ests := []specEstimate{
		{spec: Spec{H: 0.5, Order: 1, Kernel: Boxcar}, mean: 1, variance: 0.01, logZ: 100}, // enormous logZ
		{spec: Spec{H: 1.0, Order: 1, Kernel: Boxcar}, mean: 5, variance: 0.01, logZ: 0},
	}
	r := assembleBMA(ests)
	w := weightBySpec(r)
	if math.Abs(w[ests[0].spec]-0.5) > 1e-9 || math.Abs(w[ests[1].spec]-0.5) > 1e-9 {
		t.Fatalf("bandwidths must be uniform-weighted regardless of logZ; got %v", w)
	}
	if math.Abs(r.Central-3.0) > 1e-9 { // 0.5*1 + 0.5*5
		t.Fatalf("central should be the uniform mean 3.0, got %v", r.Central)
	}
}

func TestAssembleHybridAndVarianceDecomposition(t *testing.T) {
	ests := []specEstimate{
		{spec: Spec{H: 0.5, Order: 1, Kernel: Boxcar}, mean: 1, variance: 0.01, logZ: 0},
		{spec: Spec{H: 0.5, Order: 2, Kernel: Boxcar}, mean: 2, variance: 0.01, logZ: math.Log(3)},
		{spec: Spec{H: 1.0, Order: 1, Kernel: Boxcar}, mean: 3, variance: 0.01, logZ: 0},
		{spec: Spec{H: 1.0, Order: 2, Kernel: Boxcar}, mean: 4, variance: 0.01, logZ: 0},
	}
	r := assembleBMA(ests)
	w := weightBySpec(r)
	// h=0.5 group (0.5 total): {0.125, 0.375}; h=1.0 group (0.5): {0.25, 0.25}
	want := map[Spec]float64{
		ests[0].spec: 0.125, ests[1].spec: 0.375, ests[2].spec: 0.25, ests[3].spec: 0.25,
	}
	var sum float64
	for s, wv := range want {
		if math.Abs(w[s]-wv) > 1e-9 {
			t.Errorf("weight for %+v: got %.4f want %.4f", s, w[s], wv)
		}
		sum += w[s]
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("weights must sum to 1, got %v", sum)
	}
	// Central = sum w*mean.
	wantCentral := 0.125*1 + 0.375*2 + 0.25*3 + 0.25*4
	if math.Abs(r.Central-wantCentral) > 1e-9 {
		t.Fatalf("central %.4f != %.4f", r.Central, wantCentral)
	}
	// within = E[var] = 0.01; total = within + between (law of total variance).
	if math.Abs(r.WithinVar-0.01) > 1e-9 {
		t.Errorf("within variance should be the weighted mean variance 0.01, got %v", r.WithinVar)
	}
	if math.Abs(math.Sqrt(r.WithinVar+r.BetweenVar)-r.TotalSD) > 1e-9 {
		t.Errorf("total sd must equal sqrt(within+between)")
	}
	if r.BetweenVar <= 0 {
		t.Errorf("specs disagree, so between-spec variance must be positive, got %v", r.BetweenVar)
	}
}

func TestAssembleEmpty(t *testing.T) {
	r := assembleBMA(nil)
	if len(r.Specs) != 0 || r.Central != 0 || r.TotalSD != 0 {
		t.Fatalf("empty assemble should be the zero result, got %+v", r)
	}
}
