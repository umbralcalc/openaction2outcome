package bridge

import (
	"math"
	"testing"
)

// gramPD checks the Gram matrix of a kernel over a set of points is symmetric
// positive-definite (a necessary property of a valid covariance).
func gramPD(t *testing.T, cov func(i, j int) float64, n int) {
	t.Helper()
	g := make([][]float64, n)
	for i := 0; i < n; i++ {
		g[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			g[i][j] = cov(i, j)
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if math.Abs(g[i][j]-g[j][i]) > 1e-12 {
				t.Fatalf("Gram not symmetric at (%d,%d): %g vs %g", i, j, g[i][j], g[j][i])
			}
		}
	}
	if _, ok := cholesky(g); !ok {
		t.Fatal("Gram matrix is not positive-definite")
	}
}

// TestOUKernelValid confirms the structured OU/latent-force kernel is a valid
// covariance (PD Gram, peak at zero lag, monotone decay).
func TestOUKernelValid(t *testing.T) {
	k := OrnsteinUhlenbeck{SigmaF: 1.2, Lengthscale: 0.7}
	xs := []float64{-1, -0.3, 0.2, 0.9, 1.5}
	gramPD(t, func(i, j int) float64 { return k.Cov(xs[i], xs[j]) }, len(xs))
	if math.Abs(k.Cov(0.5, 0.5)-k.Variance()) > 1e-12 {
		t.Error("Cov at zero lag must equal Variance")
	}
	if k.Cov(0, 0.5) <= k.Cov(0, 1.5) {
		t.Error("OU covariance must decay with distance")
	}
}

// TestOUKernelCalibrates confirms the OU kernel slots into the deterministic
// calibrator and yields a bounded, pinned interval like any other kernel.
func TestOUKernelCalibrates(t *testing.T) {
	mech := NewQuadraticMechanism()
	anchors, _, _ := syntheticProblem(1)
	query := queryFor(1)
	k := OrnsteinUhlenbeck{SigmaF: 0.5, Lengthscale: 0.5}
	post, err := CalibrateMoment(mech, anchors, query, k, false)
	if err != nil {
		t.Fatal(err)
	}
	if post.TotalSD <= 0 || math.IsNaN(post.Central) {
		t.Errorf("degenerate posterior: %+v", post)
	}
}

// TestCoregionalizationPSD confirms the ICM coregionalisation matrix is PSD and the
// joint multi-output Gram over (x, output) pairs is positive-definite.
func TestCoregionalizationPSD(t *testing.T) {
	base := SquaredExponential{SigmaF: 1, Lengthscale: 0.6}
	// Two outputs, rank-1 coupling plus a per-output floor.
	W := [][]float64{{0.8}, {0.5}}
	kappa := []float64{0.2, 0.3}
	ck, err := NewCoregionalization(base, W, kappa, 0)
	if err != nil {
		t.Fatal(err)
	}
	// B must be symmetric PSD.
	gramPD(t, func(i, j int) float64 { return ck.B[i][j] }, ck.NumOutputs())

	// Joint Gram over interleaved outputs at shared x's must be PD.
	xs := []float64{-1, -1, 0.4, 0.4, 1.2, 1.2}
	outs := []int{0, 1, 0, 1, 0, 1}
	g, err := ck.JointGram(xs, outs)
	if err != nil {
		t.Fatal(err)
	}
	// add tiny jitter as conditioning would, then require PD
	for i := range g {
		g[i][i] += 1e-9
	}
	if _, ok := cholesky(g); !ok {
		t.Fatal("joint multi-output Gram not positive-definite")
	}

	// The scalar Kernel face equals the primary output's within-covariance.
	if math.Abs(ck.Cov(0.2, 0.5)-ck.B[0][0]*base.Cov(0.2, 0.5)) > 1e-12 {
		t.Error("scalar Cov must equal B[primary,primary]·base")
	}
	if math.Abs(ck.CovMulti(0.2, 0, 0.2, 1)-ck.B[0][1]*base.Variance()) > 1e-12 {
		t.Error("cross-output covariance must be B[0][1]·base.Variance at zero lag")
	}
}
