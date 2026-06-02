package rdd

import (
	"math"
	"sort"
)

// This file is the plug-in regression-KINK estimator (RKD), the slope analogue of
// the local-linear RDD in locallinear.go. Where an RDD reads a discontinuity in the
// LEVEL of E[Y|X] at a cutoff, an RKD reads a discontinuity in its SLOPE at a kink
// — the point where a deterministic policy function b(X) (a benefit/relief schedule,
// a tax rate) changes gradient. The estimand is
//
//	τ = ( dE[Y|X]/dX |_{x↓c}  −  dE[Y|X]/dX |_{x↑c} )  /  ( b'(c+) − b'(c−) )
//
// i.e. the kink in the outcome's slope normalised by the KNOWN kink in the policy
// slope. It identifies the marginal effect of the policy intensity b on the outcome
// at the kink — exactly what is needed where a tiered-relief schedule (e.g. a
// small-business-rate-relief taper) bends rather than jumps.
//
// Estimating a derivative at a boundary is more bias-prone than estimating a level,
// so the RKD convention is a local-QUADRATIC (order-2) fit per side rather than the
// local-linear fit RDD uses; the order is configurable. As in the RDD estimator the
// reported interval folds the bandwidth/specification spread into its width, not
// only the sampling SE, and a bandwidth sweep drives the specification variance.

// sideSlope is a one-sided local-polynomial fit evaluated at the boundary: the
// fitted boundary slope (first derivative) and its variance.
type sideSlope struct {
	Slope    float64 // dE[Y|X]/dX at the boundary (the order-1 coefficient)
	VarSlope float64 // variance of the boundary slope
	N        int     // points with positive kernel weight
}

// fitSideSlope runs a kernel-weighted local-polynomial least squares of Y on
// [1, d, d², …, d^order] (d = X − c) over the points on the requested side, and
// returns the boundary slope (coefficient on d) and its variance. order must be
// ≥ 1; order = 2 (local quadratic) is the RKD default.
func fitSideSlope(pts []Point, c, h float64, below bool, order int) sideSlope {
	if order < 1 {
		order = 1
	}
	p := order + 1 // number of coefficients
	// Weighted normal equations M β = v with M[i][j] = Σ w d^{i+j}, v[i] = Σ w d^i y.
	M := make([][]float64, p)
	for i := range M {
		M[i] = make([]float64, p)
	}
	v := make([]float64, p)
	var n int
	for _, pt := range pts {
		if below && pt.X > c {
			continue
		}
		if !below && pt.X < c {
			continue
		}
		d := pt.X - c
		w := triangular(d, h)
		if w <= 0 {
			continue
		}
		// powers d^0..d^{2*order}
		pow := make([]float64, 2*order+1)
		pow[0] = 1
		for k := 1; k <= 2*order; k++ {
			pow[k] = pow[k-1] * d
		}
		for i := 0; i < p; i++ {
			for j := 0; j < p; j++ {
				M[i][j] += w * pow[i+j]
			}
			v[i] += w * pow[i] * pt.Y
		}
		n++
	}
	if n < p {
		return sideSlope{N: n}
	}
	Minv, ok := invertDense(M)
	if !ok {
		return sideSlope{N: n}
	}
	beta := matVecMul(Minv, v)

	// Weighted residual variance with dof = n − p.
	var ssr float64
	for _, pt := range pts {
		if below && pt.X > c {
			continue
		}
		if !below && pt.X < c {
			continue
		}
		d := pt.X - c
		w := triangular(d, h)
		if w <= 0 {
			continue
		}
		fit := beta[0]
		dp := d
		for k := 1; k < p; k++ {
			fit += beta[k] * dp
			dp *= d
		}
		r := pt.Y - fit
		ssr += w * r * r
	}
	dof := float64(n - p)
	if dof <= 0 {
		dof = 1
	}
	sigma2 := ssr / dof
	// Cov(β) = σ² M⁻¹; the boundary slope is β[1], so Var(slope) = σ² (M⁻¹)[1][1].
	return sideSlope{Slope: beta[1], VarSlope: sigma2 * Minv[1][1], N: n}
}

// FitKink estimates the RKD effect at a single bandwidth: the kink in the outcome
// slope (right minus left) divided by policySlopeChange = b'(c+) − b'(c−), the known
// change in the policy function's slope at the kink. se is the delta-method standard
// error. order is the local-polynomial order per side (2 = the RKD default).
func FitKink(pts []Point, kink, h, policySlopeChange float64, order int) (tau, se float64, nRight, nLeft int) {
	right := fitSideSlope(pts, kink, h, false, order) // x ≥ c
	left := fitSideSlope(pts, kink, h, true, order)   // x ≤ c
	if policySlopeChange == 0 {
		return math.NaN(), math.NaN(), right.N, left.N
	}
	tau = (right.Slope - left.Slope) / policySlopeChange
	se = math.Sqrt(right.VarSlope+left.VarSlope) / math.Abs(policySlopeChange)
	return tau, se, right.N, left.N
}

// LevelDiscontinuity returns the LEVEL jump in E[Y|X] at the kink (treated-minus-
// control intercept), reusing the RDD local-linear side fit. An RKD assumes the
// conditional mean is CONTINUOUS at the kink (only its slope bends); a non-zero
// level jump here means the design is contaminated by a notch and the kink estimate
// is not clean. This is the RKD analogue of an RDD placebo — surfaced as a validity
// number, not folded into τ.
func LevelDiscontinuity(pts []Point, kink, h float64) (jump, se float64) {
	right := fitSide(pts, kink, h, false)
	left := fitSide(pts, kink, h, true)
	return right.Intercept - left.Intercept, math.Sqrt(right.VarInt + left.VarInt)
}

// KinkResult is the honest summary of a regression-kink estimate, mirroring Result.
type KinkResult struct {
	Central           float64      // τ at the reference bandwidth
	SamplingVar       float64      // sampling variance at the reference bandwidth
	SpecVar           float64      // variance of τ across the bandwidth sweep
	TotalSD           float64      // sqrt(SamplingVar + SpecVar)
	RefBandwidth      float64      //
	PolicySlopeChange float64      // b'(c+) − b'(c−), the known policy-slope kink
	Order             int          // local-polynomial order per side
	Sweep             []SweepPoint //
	NRight, NLeft     int          // sample sizes at the reference bandwidth
}

// Interval95 returns the central 95% interval implied by TotalSD (Gaussian).
func (r KinkResult) Interval95() (lo, hi float64) {
	const z = 1.959963984540054
	return r.Central - z*r.TotalSD, r.Central + z*r.TotalSD
}

// EstimateKink runs a bandwidth sweep for the RKD effect and folds the sweep spread
// into the interval, exactly as Estimate does for the RDD level effect. bandwidths
// must be non-empty and include refBW. policySlopeChange is the known kink in the
// policy function's slope; order is the local-polynomial order (2 = RKD default).
func EstimateKink(pts []Point, kink, refBW float64, bandwidths []float64, policySlopeChange float64, order int) KinkResult {
	bw := append([]float64(nil), bandwidths...)
	sort.Float64s(bw)
	res := KinkResult{RefBandwidth: refBW, PolicySlopeChange: policySlopeChange, Order: order}
	taus := make([]float64, 0, len(bw))
	for _, h := range bw {
		tau, se, nr, nl := FitKink(pts, kink, h, policySlopeChange, order)
		res.Sweep = append(res.Sweep, SweepPoint{Bandwidth: h, Tau: tau, SE: se, NTreat: nr, NCtrl: nl})
		taus = append(taus, tau)
		if h == refBW {
			res.Central = tau
			res.SamplingVar = se * se
			res.NRight, res.NLeft = nr, nl
		}
	}
	res.SpecVar = variance(taus)
	res.TotalSD = math.Sqrt(res.SamplingVar + res.SpecVar)
	return res
}

// invertDense returns the inverse of a small square matrix via Gauss-Jordan with
// partial pivoting, and whether it succeeded. Sized for the (order+1) RKD normal
// equations (2×2 or 3×3), so a plain dense inverse is ample.
func invertDense(a [][]float64) ([][]float64, bool) {
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, 2*n)
		copy(m[i], a[i])
		m[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		piv := col
		best := math.Abs(m[col][col])
		for r := col + 1; r < n; r++ {
			if vv := math.Abs(m[r][col]); vv > best {
				best, piv = vv, r
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

func matVecMul(a [][]float64, x []float64) []float64 {
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
