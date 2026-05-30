package sbi

// Small dense linear algebra for the local-polynomial normal equations. The
// design dimension d = 2 + 2*order is tiny (<= 6 here), so a plain Gauss-Jordan
// inverse is more than adequate and keeps this package dependency-free.

// invert returns the inverse of a square matrix a (n x n) and whether it
// succeeded (false if singular). a is not modified.
func invert(a [][]float64) ([][]float64, bool) {
	n := len(a)
	// Augmented [a | I].
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		// Partial pivot.
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
		// Normalise pivot row.
		d := m[col][col]
		for j := 0; j < 2*n; j++ {
			m[col][j] /= d
		}
		// Eliminate other rows.
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
