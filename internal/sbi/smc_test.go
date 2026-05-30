package sbi

import (
	"math"
	"math/rand"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/rdd"
)

// syntheticJump builds points with a known discontinuity tau at cutoff 0, with
// the treated side being x < 0 (a floor-style design).
func syntheticJump(n int, alpha, tau, slope, sigma float64, seed int64) []rdd.Point {
	rng := rand.New(rand.NewSource(seed))
	pts := make([]rdd.Point, n)
	for i := range pts {
		x := -1 + 2*rng.Float64() // uniform in [-1, 1]
		treated := x < 0
		mean := alpha + slope*x
		if treated {
			mean += tau
		}
		pts[i] = rdd.Point{X: x, Y: mean + rng.NormFloat64()*sigma}
	}
	return pts
}

// The SMC posterior for tau must agree with the closed-form WLS posterior on a
// linear-Gaussian model (where SMC is approximating an exact Gaussian target),
// validating the stochadex wiring end to end.
func TestSMCMatchesClosedForm(t *testing.T) {
	pts := syntheticJump(500, 0.2, 0.3, 0.15, 0.2, 7)
	sf := buildSpecFit(pts, 0, Spec{H: 1.0, Order: 1, Kernel: Triangular}, true)
	if !sf.ok {
		t.Fatal("spec fit failed")
	}
	exMean, exVar := sf.exactTau()
	if math.Abs(exMean-0.3) > 0.06 {
		t.Fatalf("closed-form tau mean %.4f far from true 0.3", exMean)
	}

	post := fitSpecSMC(sf, SMCConfig{NumParticles: 4000, NumRounds: 8, Seed: 1})
	if !post.ok {
		t.Fatal("SMC fit failed")
	}
	if math.Abs(post.tauMean-exMean) > 0.05 {
		t.Fatalf("SMC tau mean %.4f disagrees with closed form %.4f", post.tauMean, exMean)
	}
	// Posterior SD should be the right order of magnitude (within ~2x).
	exSD, smcSD := math.Sqrt(exVar), math.Sqrt(post.tauVar)
	if ratio := smcSD / exSD; ratio < 0.4 || ratio > 2.5 {
		t.Fatalf("SMC tau sd %.4f vs closed-form %.4f (ratio %.2f out of range)", smcSD, exSD, ratio)
	}
	if math.IsNaN(post.logZ) || math.IsInf(post.logZ, 0) {
		t.Fatalf("log marginal likelihood not finite: %v", post.logZ)
	}
}

// Same seed must reproduce the posterior exactly (re-mintable marks).
func TestSMCDeterministic(t *testing.T) {
	pts := syntheticJump(300, 0.1, 0.25, 0.1, 0.2, 11)
	sf := buildSpecFit(pts, 0, Spec{H: 1.0, Order: 1, Kernel: Triangular}, true)
	cfg := SMCConfig{NumParticles: 1500, NumRounds: 5, Seed: 3}
	a := fitSpecSMC(sf, cfg)
	b := fitSpecSMC(sf, cfg)
	if a.tauMean != b.tauMean || a.tauVar != b.tauVar || a.logZ != b.logZ {
		t.Fatalf("non-deterministic SMC: %+v vs %+v", a, b)
	}
}
