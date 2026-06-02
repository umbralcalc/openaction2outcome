package did

import (
	"fmt"
	"math"
	"testing"
)

// makePanel builds a synthetic panel: nT treated + nC control units, observed at
// integer times in [t0, t1]. Each unit i has its own fixed level (offset), a common
// time trend g(t) shared by both groups (parallel trends), plus, for treated units,
// an effect added at/after treatTime. preTrendTreated adds a treated-only linear
// pre-trend (to violate parallel trends when non-zero). effect(t) gives the dynamic
// treatment effect at post time t.
func makePanel(nT, nC int, t0, t1, treatTime int, g func(float64) float64, preTrendTreated float64, effect func(float64) float64) []Unit {
	var units []Unit
	mk := func(id string, treated bool, level float64) Unit {
		u := Unit{ID: id, Treated: treated}
		for t := t0; t <= t1; t++ {
			ft := float64(t)
			y := level + g(ft)
			if treated {
				y += preTrendTreated * ft
				if t >= treatTime {
					y += effect(ft)
				}
			}
			u.Times = append(u.Times, ft)
			u.Y = append(u.Y, y)
		}
		return u
	}
	for i := 0; i < nT; i++ {
		units = append(units, mk(fmt.Sprintf("T%d", i), true, 0.1*float64(i)))
	}
	for i := 0; i < nC; i++ {
		units = append(units, mk(fmt.Sprintf("C%d", i), false, 0.3+0.1*float64(i)))
	}
	return units
}

// With parallel trends and a constant treatment effect, the DiD must recover the
// effect exactly (noise-free) and the pre-trend slope must be ~0.
func TestDiDRecoversKnownEffect(t *testing.T) {
	const tauTrue = 1.7
	g := func(x float64) float64 { return 0.4 * x } // common trend, both groups
	units := makePanel(8, 9, 0, 10, 6, g, 0, func(float64) float64 { return tauTrue })

	tau, se, nT, nC := FitDiD(units, 6, 4)
	if math.Abs(tau-tauTrue) > 1e-9 {
		t.Fatalf("expected DiD effect %.6f, got %.6f", tauTrue, tau)
	}
	if se > 1e-9 {
		t.Fatalf("noise-free, homogeneous groups should give ~0 SE, got %g", se)
	}
	if nT == 0 || nC == 0 {
		t.Fatalf("expected both groups to contribute, got nT=%d nC=%d", nT, nC)
	}

	slope, _, npts := PreTrend(units, 6, 4)
	if math.Abs(slope) > 1e-9 {
		t.Fatalf("parallel pre-trends should give ~0 pre-trend slope, got %g", slope)
	}
	if npts < 2 {
		t.Fatalf("expected pre points for the trend check, got %d", npts)
	}
}

// A treated-only differential pre-trend must be caught by the pre-trend diagnostic
// (slope ≈ the planted differential), so a bad control cannot pass silently.
func TestDiDDetectsPreTrendViolation(t *testing.T) {
	const diffTrend = 0.25
	g := func(x float64) float64 { return 0.4 * x }
	units := makePanel(8, 9, 0, 10, 6, g, diffTrend, func(float64) float64 { return 1.0 })

	slope, se, _ := PreTrend(units, 6, 5)
	if math.Abs(slope-diffTrend) > 1e-9 {
		t.Fatalf("pre-trend slope should detect the planted differential %.3f, got %.3f", diffTrend, slope)
	}
	// The violation should be statistically distinguishable from zero.
	if !(se < math.Abs(slope)/2) {
		t.Fatalf("planted pre-trend should be clearly non-zero: slope=%.3f se=%.3f", slope, se)
	}
}

// A dynamic (growing) treatment effect makes different post-window widths disagree,
// so Estimate must report a positive specification variance and an honest SD at
// least as wide as the sampling-only SD.
func TestEstimateFoldsSpecSpread(t *testing.T) {
	g := func(x float64) float64 { return 0.2 * x }
	// effect grows with time since treatment → window-dependent average effect.
	eff := func(x float64) float64 { return 0.5 * (x - 6) }
	units := makePanel(10, 10, 0, 12, 6, g, 0, eff)

	r := Estimate(units, 6, 3, []float64{2, 3, 4, 5})
	if r.SpecVar <= 0 {
		t.Fatalf("a dynamic effect should produce positive specification variance, got %g", r.SpecVar)
	}
	if r.TotalSD < math.Sqrt(r.SamplingVar) {
		t.Fatalf("honest SD %.4g must be >= sampling SD %.4g", r.TotalSD, math.Sqrt(r.SamplingVar))
	}
	if math.Abs(r.PreTrendSlope) > 1e-9 {
		t.Fatalf("parallel pre-trends expected, got slope %g", r.PreTrendSlope)
	}
}
