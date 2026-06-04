package its

import "math"

// dot is the inner product of two equal-length vectors.
func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// quadForm returns c' M c for a symmetric k×k matrix M (row-major) and vector c.
func quadForm(c []float64, M [][]float64) float64 {
	var s float64
	for i := range c {
		for j := range c {
			s += c[i] * M[i][j] * c[j]
		}
	}
	return s
}

// ols solves the normal equations beta = (X'X)^{-1} X'y by Gauss-Jordan inversion
// of X'X. It returns beta, the (X'X)^{-1} matrix (needed for the HAC sandwich), and
// ok=false when X'X is singular. Dimensions are small (<=8 regressors).
func ols(X [][]float64, y []float64) (beta []float64, xtxInv [][]float64, ok bool) {
	n := len(X)
	if n == 0 {
		return nil, nil, false
	}
	k := len(X[0])
	xtx := make([][]float64, k)
	xty := make([]float64, k)
	for i := 0; i < k; i++ {
		xtx[i] = make([]float64, k)
	}
	for r := 0; r < n; r++ {
		xr := X[r]
		yr := y[r]
		for i := 0; i < k; i++ {
			xty[i] += xr[i] * yr
			for j := 0; j < k; j++ {
				xtx[i][j] += xr[i] * xr[j]
			}
		}
	}
	inv, ok := invert(xtx)
	if !ok {
		return nil, nil, false
	}
	beta = make([]float64, k)
	for i := 0; i < k; i++ {
		var s float64
		for j := 0; j < k; j++ {
			s += inv[i][j] * xty[j]
		}
		beta[i] = s
	}
	return beta, inv, true
}

// neweyWest returns the Bartlett-kernel HAC covariance of beta:
//
//	V = (X'X)^{-1} S (X'X)^{-1},  S = S0 + Σ_{l=1}^{L} w_l (Σ_t e_t e_{t-l}(x_t x_{t-l}' + x_{t-l} x_t')),
//
// with w_l = 1 - l/(L+1). resid are the OLS residuals aligned to X (time-ordered);
// xtxInv is (X'X)^{-1} from the OLS fit.
func neweyWest(X [][]float64, resid []float64, xtxInv [][]float64, L int) [][]float64 {
	n := len(X)
	k := len(X[0])
	S := zeros(k)
	// lag 0
	for t := 0; t < n; t++ {
		e2 := resid[t] * resid[t]
		xt := X[t]
		for i := 0; i < k; i++ {
			for j := 0; j < k; j++ {
				S[i][j] += e2 * xt[i] * xt[j]
			}
		}
	}
	// lags 1..L (Bartlett weighted, symmetrised)
	for l := 1; l <= L && l < n; l++ {
		w := 1 - float64(l)/float64(L+1)
		for t := l; t < n; t++ {
			ee := resid[t] * resid[t-l]
			xt, xtl := X[t], X[t-l]
			for i := 0; i < k; i++ {
				for j := 0; j < k; j++ {
					S[i][j] += w * ee * (xt[i]*xtl[j] + xtl[i]*xt[j])
				}
			}
		}
	}
	// V = xtxInv S xtxInv
	return matMul(matMul(xtxInv, S), xtxInv)
}

func zeros(k int) [][]float64 {
	m := make([][]float64, k)
	for i := range m {
		m[i] = make([]float64, k)
	}
	return m
}

func matMul(A, B [][]float64) [][]float64 {
	n := len(A)
	k := len(B)
	m := len(B[0])
	C := make([][]float64, n)
	for i := 0; i < n; i++ {
		C[i] = make([]float64, m)
		for j := 0; j < m; j++ {
			var s float64
			for t := 0; t < k; t++ {
				s += A[i][t] * B[t][j]
			}
			C[i][j] = s
		}
	}
	return C
}

// invert inverts a square matrix by Gauss-Jordan elimination with partial pivoting.
// Returns ok=false if the matrix is singular (no usable pivot).
func invert(a [][]float64) ([][]float64, bool) {
	n := len(a)
	// Augment [a | I].
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		// pivot
		piv := col
		best := math.Abs(m[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(m[r][col]); v > best {
				best = v
				piv = r
			}
		}
		if best < 1e-12 {
			return nil, false
		}
		m[col], m[piv] = m[piv], m[col]
		// normalise pivot row
		pv := m[col][col]
		for j := 0; j < 2*n; j++ {
			m[col][j] /= pv
		}
		// eliminate other rows
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
	for i := 0; i < n; i++ {
		inv[i] = make([]float64, n)
		copy(inv[i], m[i][n:])
	}
	return inv, true
}

// lag1corr is the lag-1 autocorrelation of a series (0 when undefined).
func lag1corr(e []float64) float64 {
	n := len(e)
	if n < 3 {
		return 0
	}
	var mean float64
	for _, v := range e {
		mean += v
	}
	mean /= float64(n)
	var num, den float64
	for i := 0; i < n; i++ {
		d := e[i] - mean
		den += d * d
		if i > 0 {
			num += d * (e[i-1] - mean)
		}
	}
	if den == 0 {
		return 0
	}
	return num / den
}
