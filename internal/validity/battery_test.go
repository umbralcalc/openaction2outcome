package validity

import (
	"math"
	"math/rand"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/rdd"
)

// covariatePoints builds (running, covariate) points; jump adds a discontinuity
// of size `jump` on the treated side (x < 0).
func covariatePoints(n int, slope, jump, sigma float64, seed int64) []rdd.Point {
	rng := rand.New(rand.NewSource(seed))
	pts := make([]rdd.Point, n)
	for i := range pts {
		x := -1 + 2*rng.Float64()
		y := slope*x + rng.NormFloat64()*sigma
		if x < 0 {
			y += jump
		}
		pts[i] = rdd.Point{X: x, Y: y}
	}
	return pts
}

func TestDensityTestPassesWhenFlat(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	running := make([]float64, 4000)
	for i := range running {
		running[i] = -1 + 2*rng.Float64() // uniform, no bunching
	}
	res := DensityTest(running, 0, 0.05, 0.5)
	if !res.Passed {
		t.Fatalf("flat density should pass (p=%v)", res.PValue)
	}
}

func TestDensityTestFailsWithBunching(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var running []float64
	for i := 0; i < 4000; i++ {
		running = append(running, -1+2*rng.Float64())
	}
	// Pile units just above the cutoff (sorting into the just-treated side).
	for i := 0; i < 1500; i++ {
		running = append(running, rng.Float64()*0.1)
	}
	res := DensityTest(running, 0, 0.05, 0.5)
	if res.Passed {
		t.Fatalf("bunching at the cutoff should fail the density test (p=%v)", res.PValue)
	}
}

func TestCovariateContinuity(t *testing.T) {
	// No jump -> continuity holds -> pass.
	cont := CovariateContinuity("x", covariatePoints(800, 0.5, 0.0, 0.2, 3), 0, 0.8, true)
	if !cont.Passed {
		t.Errorf("continuous covariate should pass (p=%v)", cont.PValue)
	}
	if cont.Statistic == nil {
		t.Error("a real test must report a statistic (no free pass)")
	}
	// Large jump -> fail.
	jump := CovariateContinuity("x", covariatePoints(800, 0.5, 1.0, 0.2, 4), 0, 0.8, true)
	if jump.Passed {
		t.Errorf("a covariate jump should fail continuity (jump stat=%v)", jump.Statistic)
	}
}

func TestCovariateContinuityInconclusiveFailsClosed(t *testing.T) {
	// Only one side has data -> inconclusive must NOT be a free pass.
	pts := []rdd.Point{{X: 0.2, Y: 1}, {X: 0.3, Y: 1}, {X: 0.4, Y: 1}}
	r := CovariateContinuity("x", pts, 0, 0.8, true)
	if r.Passed {
		t.Fatal("inconclusive continuity (no control-side data) should fail closed")
	}
}

func TestPlaceboCutoffsPassAwayFromRealJump(t *testing.T) {
	// Real jump only at 0; placebos at -0.4 and 0.4 with a narrow window see none.
	pts := covariatePoints(1200, 0.3, 0.8, 0.2, 5)
	res := PlaceboCutoffs(pts, []float64{-0.4, 0.4}, 0.3, true)
	if len(res) != 2 {
		t.Fatalf("expected 2 placebo results, got %d", len(res))
	}
	for _, p := range res {
		if !p.Passed {
			t.Errorf("placebo at %g should be indistinguishable from zero (est=%.4f)", p.Cutoff, p.Estimate)
		}
	}
}

func TestDonutRobustness(t *testing.T) {
	pts := covariatePoints(1000, 0.3, 0.5, 0.2, 6)
	res := DonutRobustness(pts, 0, 0.6, true, []float64{0.05, 0.1})
	if len(res) != 2 {
		t.Fatalf("expected 2 donut points, got %d", len(res))
	}
	for _, s := range res {
		if math.IsNaN(s.Estimate) {
			t.Errorf("donut radius %g produced NaN estimate", s.Param)
		}
	}
}
