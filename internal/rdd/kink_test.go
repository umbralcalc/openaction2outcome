package rdd

import (
	"math"
	"testing"
)

// On data with a known slope kink and NO level jump, the RKD estimator must
// recover the marginal effect exactly (noise-free => zero residual variance).
//
// Construction: a policy intensity b(X) that is flat below the kink and rises with
// slope s above it, so b'(c+) − b'(c−) = s. The outcome is Y = β·b(X) (+ a smooth
// confounding trend continuous in slope at the kink). Then dE[Y]/dX kinks by β·s at
// c, and τ = (β·s)/s = β — the marginal effect of the policy intensity.
func TestFitKinkRecoversMarginalEffect(t *testing.T) {
	const c = 12.0
	const betaTrue = -0.4 // marginal effect of the policy intensity on the outcome
	const s = 1.0 / 3.0   // policy slope above the kink (slope change = s)
	b := func(x float64) float64 {
		if x <= c {
			return 0
		}
		return s * (x - c)
	}
	var pts []Point
	for x := 9.0; x <= 15.0001; x += 0.05 {
		// a baseline trend that is smooth (continuous level AND slope) through c,
		// so only the policy kink creates a slope discontinuity.
		y := 0.2 * (x - c)
		y += betaTrue * b(x)
		pts = append(pts, Point{X: x, Y: y})
	}
	tau, se, nr, nl := FitKink(pts, c, 2.5, s, 2)
	if math.Abs(tau-betaTrue) > 1e-6 {
		t.Fatalf("expected marginal effect %.6f, got %.6f", betaTrue, tau)
	}
	if se > 1e-5 {
		t.Fatalf("noise-free data should give ~0 se, got %g", se)
	}
	if nr == 0 || nl == 0 {
		t.Fatalf("expected points on both sides, got nr=%d nl=%d", nr, nl)
	}
}

// A genuine kink design has a CONTINUOUS level at the kink: LevelDiscontinuity
// should read ~0. A non-zero reading would mean a notch contaminates the kink.
func TestKinkHasNoLevelJump(t *testing.T) {
	const c = 12.0
	const s = 1.0 / 3.0
	var pts []Point
	for x := 9.0; x <= 15.0001; x += 0.05 {
		y := 0.2 * (x - c)
		if x > c {
			y += -0.4 * s * (x - c) // slope bend only, no level jump
		}
		pts = append(pts, Point{X: x, Y: y})
	}
	jump, _ := LevelDiscontinuity(pts, c, 2.5)
	if math.Abs(jump) > 1e-6 {
		t.Fatalf("a clean kink should have ~0 level jump, got %g", jump)
	}
}

// EstimateKink must fold bandwidth disagreement into the interval: with curvature
// the per-bandwidth slope estimates differ, giving a positive specification
// variance and an honest SD at least as wide as the sampling-only SD.
func TestEstimateKinkFoldsInSpecSpread(t *testing.T) {
	const c = 12.0
	const s = 1.0 / 3.0
	var pts []Point
	for x := 9.0; x <= 15.0001; x += 0.02 {
		// curvature (cubic term) makes different bandwidths disagree on the slope.
		y := 0.2*(x-c) + 0.05*math.Pow(x-c, 3)
		if x > c {
			y += -0.4 * s * (x - c)
		}
		pts = append(pts, Point{X: x, Y: y})
	}
	r := EstimateKink(pts, c, 2.0, []float64{1.5, 2.0, 2.5, 3.0}, s, 2)
	if r.SpecVar <= 0 {
		t.Fatalf("expected positive specification variance from bandwidth disagreement, got %g", r.SpecVar)
	}
	if r.TotalSD < math.Sqrt(r.SamplingVar) {
		t.Fatalf("honest SD %.4g must be >= sampling SD %.4g", r.TotalSD, math.Sqrt(r.SamplingVar))
	}
}
