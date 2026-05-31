package bridge

import "math"

// Small dense linear algebra for GP conditioning on the anchors. The Gram matrix
// is n x n where n = number of anchors (2–5 in practice), so a plain Gauss-Jordan
// inverse is more than adequate. These helpers are copied from internal/sbi to
// keep the two packages decoupled (the same judgement sbi made keeping itself
// self-contained); they are tiny and dependency-free.

// invert returns the inverse of a square matrix a (n x n) and whether it
// succeeded (false if singular). a is not modified.
func invert(a [][]float64) ([][]float64, bool) {
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		piv := col
		best := abs(m[col][col])
		for r := col + 1; r < n; r++ {
			if v := abs(m[r][col]); v > best {
				best, piv = v, r
			}
		}
		if best < 1e-14 {
			return nil, false
		}
		m[col], m[piv] = m[piv], m[col]
		d := m[col][col]
		for j := 0; j < 2*n; j++ {
			m[col][j] /= d
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := m[r][col]
			if f == 0 {
				continue
			}
			for j := 0; j < 2*n; j++ {
				m[r][j] -= f * m[col][j]
			}
		}
	}
	inv := make([][]float64, n)
	for i := range inv {
		inv[i] = make([]float64, n)
		copy(inv[i], m[i][n:])
	}
	return inv, true
}

func matVec(a [][]float64, x []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		var s float64
		for j := range x {
			s += a[i][j] * x[j]
		}
		out[i] = s
	}
	return out
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// quad returns xᵀ A x.
func quad(a [][]float64, x []float64) float64 {
	return dot(x, matVec(a, x))
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// cholesky returns the lower-triangular L with L Lᵀ = a for a symmetric
// positive-definite a, and whether it succeeded (false if not PD). Used by the
// joint calibrator to whiten the GP prior: δ = L z with z ~ iid N(0,1) has
// covariance L Lᵀ = a, so the correlated discrepancy prior becomes independent
// standard-normal latents that stochadex SMC can sample directly.
func cholesky(a [][]float64) ([][]float64, bool) {
	n := len(a)
	l := make([][]float64, n)
	for i := range l {
		l[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			s := a[i][j]
			for k := 0; k < j; k++ {
				s -= l[i][k] * l[j][k]
			}
			if i == j {
				if s <= 0 {
					return nil, false
				}
				l[i][j] = math.Sqrt(s)
			} else {
				l[i][j] = s / l[j][j]
			}
		}
	}
	return l, true
}
