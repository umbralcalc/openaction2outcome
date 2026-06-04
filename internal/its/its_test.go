package its

import (
	"math"
	"testing"
)

// synthPanel builds a controlled-ITS panel with a KNOWN level break in the
// treated-minus-control difference at t0. Treated and control share a common trend
// and seasonality (which the difference nets out); the treated series additionally
// drops by `effect` from t0 onward. Deterministic pseudo-noise keeps the test
// reproducible without importing math/rand.
func synthPanel(n int, t0 float64, effect float64) []Point {
	pts := make([]Point, n)
	for i := 0; i < n; i++ {
		t := float64(i)
		common := 40 + 0.1*t + 5*math.Sin(2*math.Pi*t/12) // shared trend + season
		// deterministic small wobble, distinct per series
		nT := 0.5 * math.Sin(t*1.3)
		nC := 0.5 * math.Cos(t*0.7)
		treated := common + 12 + nT // roadside increment of ~12 over background
		control := common + nC
		if t >= t0 {
			treated += effect // the policy break (effect<0 = a reduction)
		}
		pts[i] = Point{T: t, Treated: treated, Control: control}
	}
	return pts
}

func TestFitRecoversKnownBreak(t *testing.T) {
	t0 := 30.0
	const truth = -4.0
	pts := synthPanel(60, t0, truth)
	spec := Spec{Name: "level+season", PreStartT: 0, Harmonics: 1, Slope: false, NWLag: 4}
	f := Fit(pts, t0, spec)
	if !f.OK {
		t.Fatalf("fit not OK: %+v", f)
	}
	if math.Abs(f.Effect-truth) > 0.5 {
		t.Errorf("effect %.3f not within 0.5 of truth %.1f", f.Effect, truth)
	}
	if f.SE() <= 0 || math.IsNaN(f.SE()) {
		t.Errorf("bad SE %.4g", f.SE())
	}
	// 95% interval should cover the truth.
	lo, hi := f.Effect-1.96*f.SE(), f.Effect+1.96*f.SE()
	if truth < lo || truth > hi {
		t.Errorf("95%% interval [%.3f,%.3f] excludes truth %.1f", lo, hi, truth)
	}
}

func TestBMAHonestIntervalCoversAndDecomposes(t *testing.T) {
	t0 := 30.0
	const truth = -5.0
	pts := synthPanel(66, t0, truth)
	specs := []Spec{
		{Name: "h1-level", PreStartT: 0, Harmonics: 1, Slope: false, NWLag: 4},
		{Name: "h2-level", PreStartT: 0, Harmonics: 2, Slope: false, NWLag: 4},
		{Name: "h1-slope", PreStartT: 0, Harmonics: 1, Slope: true, NWLag: 4},
		{Name: "h1-level-late", PreStartT: 6, Harmonics: 1, Slope: false, NWLag: 4},
	}
	r := EstimateBMA(pts, t0, specs)
	if len(r.Specs) != len(specs) {
		t.Fatalf("expected %d surviving specs, got %d", len(specs), len(r.Specs))
	}
	if math.Abs(r.Central-truth) > 0.7 {
		t.Errorf("BMA central %.3f not within 0.7 of truth %.1f", r.Central, truth)
	}
	lo, hi := r.Interval(0.95)
	if truth < lo || truth > hi {
		t.Errorf("honest 95%% interval [%.3f,%.3f] excludes truth %.1f", lo, hi, truth)
	}
	if r.WithinVar <= 0 || r.BetweenVar < 0 {
		t.Errorf("variance decomposition off: within=%.4g between=%.4g", r.WithinVar, r.BetweenVar)
	}
	// The total interval must be at least as wide as the widest single-spec interval
	// would be from sampling alone (identification variance only adds width).
	if r.TotalSD*r.TotalSD < r.WithinVar-1e-9 {
		t.Errorf("total var %.4g < within var %.4g", r.TotalSD*r.TotalSD, r.WithinVar)
	}
}

func TestPlaceboAndPreTrendAreClean(t *testing.T) {
	t0 := 30.0
	pts := synthPanel(60, t0, -4.0)
	// Pre-trend in the difference should be ~flat (parallel trends hold by construction).
	slope, se, n := PreTrend(pts, t0, 0)
	if n < 10 {
		t.Fatalf("too few pre months: %d", n)
	}
	if math.Abs(slope) > 1.96*se {
		t.Errorf("pre-trend slope %.4f (se %.4f) is significantly non-zero", slope, se)
	}
	// A placebo intervention at t=15 (well inside the pre-period) should show ~0 effect.
	spec := Spec{Name: "placebo", PreStartT: 0, Harmonics: 1, Slope: false, NWLag: 4}
	pf := PlaceboFit(pts, t0, 15, spec)
	if !pf.OK {
		t.Fatalf("placebo fit not OK")
	}
	if math.Abs(pf.Effect) > 1.96*pf.SE()+0.5 {
		t.Errorf("placebo effect %.3f (se %.3f) not indistinguishable from zero", pf.Effect, pf.SE())
	}
}

func TestSingularDesignReturnsNotOK(t *testing.T) {
	// All-post (no pre months) must not produce a fit.
	pts := synthPanel(10, -1, -3)
	f := Fit(pts, 0, Spec{Name: "x", PreStartT: 0, Harmonics: 1})
	if f.OK {
		t.Errorf("expected not-OK fit with no pre period, got %+v", f)
	}
}
