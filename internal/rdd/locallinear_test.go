package rdd

import (
	"math"
	"testing"
)

// On exactly-linear data with a known jump at the cutoff, the local-linear fit
// must recover the jump exactly (no noise => zero residual variance).
func TestFitRecoversKnownJump(t *testing.T) {
	const cutoff = 0.0
	const tauTrue = 1.0
	var pts []Point
	for x := -1.0; x <= 1.0001; x += 0.05 {
		// treated side is below the cutoff; give it a +tauTrue level shift.
		y := 0.3 * x
		if x < cutoff {
			y += tauTrue
		}
		pts = append(pts, Point{X: x, Y: y})
	}
	tau, se, nt, nc := Fit(pts, cutoff, 0.6, true)
	if math.Abs(tau-tauTrue) > 1e-9 {
		t.Fatalf("expected tau=%.6f, got %.6f", tauTrue, tau)
	}
	if se > 1e-6 {
		t.Fatalf("noise-free data should give ~0 se, got %g", se)
	}
	if nt == 0 || nc == 0 {
		t.Fatalf("expected points on both sides, got nt=%d nc=%d", nt, nc)
	}
}

// The honest interval must be at least as wide as the sampling-only interval,
// because it folds in the bandwidth/specification spread.
func TestEstimateFoldsInSpecSpread(t *testing.T) {
	var pts []Point
	// A slightly non-linear relationship so different bandwidths disagree => a
	// non-zero specification variance.
	for x := -1.0; x <= 1.0001; x += 0.02 {
		y := 0.3*x + 0.4*x*x
		if x < 0 {
			y += 0.5
		}
		pts = append(pts, Point{X: x, Y: y})
	}
	r := Estimate(pts, 0, 0.5, []float64{0.3, 0.4, 0.5, 0.6, 0.7}, true)
	if r.SpecVar <= 0 {
		t.Fatalf("expected positive specification variance from bandwidth disagreement, got %g", r.SpecVar)
	}
	if r.TotalSD < math.Sqrt(r.SamplingVar) {
		t.Fatalf("honest SD %.4g must be >= sampling SD %.4g", r.TotalSD, math.Sqrt(r.SamplingVar))
	}
}
