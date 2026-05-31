package bridge

import "errors"

// defaultJitter is the relative diagonal jitter added to (K+Σ) before inversion,
// in units of the kernel prior variance σf². It guards against a near-singular
// Gram matrix when anchors are close on x or ℓ is large relative to spacing.
const defaultJitter = 1e-8

// gpConditioner precomputes the parts of GP discrepancy conditioning that do NOT
// depend on θ: the inverse Gram matrix (K + Σ + jitter)⁻¹. Given that inverse,
// the conditional mean of δ at a query is a linear functional of the residuals
// r(θ) = μ − m(·;θ), and the conditional variance is θ-independent — so the
// inverse is formed once and reused across all SMC particles.
type gpConditioner struct {
	xs     []float64 // anchor positions
	kernel Kernel
	ginv   [][]float64 // (K + Σ + jitter·σf²·I)⁻¹
}

// newGPConditioner assembles K + Σ (anchor-posterior noise as the nugget) with
// jitter and inverts it. It errors if the matrix is singular even after jitter,
// rather than returning a silently bad fit (mirrors sbi's buildSpecFit bail-out).
func newGPConditioner(xs, noiseVar []float64, k Kernel, jitter float64) (*gpConditioner, error) {
	n := len(xs)
	if n < 2 {
		return nil, errors.New("bridge: need >=2 anchors to condition the discrepancy")
	}
	g := make([][]float64, n)
	jit := jitter * k.Variance()
	for i := 0; i < n; i++ {
		g[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			g[i][j] = k.Cov(xs[i], xs[j])
		}
		g[i][i] += noiseVar[i] + jit
	}
	ginv, ok := invert(g)
	if !ok {
		return nil, errors.New("bridge: anchor Gram matrix is singular even with jitter; check anchor spacing / kernel lengthscale")
	}
	return &gpConditioner{xs: append([]float64(nil), xs...), kernel: k, ginv: ginv}, nil
}

// kstar returns the cross-covariance vector k⋆ = [k(query, xᵢ)].
func (g *gpConditioner) kstar(query float64) []float64 {
	ks := make([]float64, len(g.xs))
	for i, x := range g.xs {
		ks[i] = g.kernel.Cov(query, x)
	}
	return ks
}

// condVar is the GP conditional variance of δ at the query, Var δ(x⋆) =
// k(x⋆,x⋆) − k⋆ᵀ (K+Σ)⁻¹ k⋆. It depends only on the kernel and anchor geometry
// (NOT on θ), so it is computed once. Floored at zero against round-off.
func (g *gpConditioner) condVar(query float64) float64 {
	ks := g.kstar(query)
	v := g.kernel.Variance() - quad(g.ginv, ks)
	if v < 0 {
		v = 0
	}
	return v
}

// condMeanWeights returns w = (K+Σ)⁻¹ k⋆, so that the conditional discrepancy
// mean at the query for any residual vector r is simply dot(w, r) — letting the
// per-particle loop avoid re-touching the inverse.
func (g *gpConditioner) condMeanWeights(query float64) []float64 {
	return matVec(g.ginv, g.kstar(query))
}

// impliedNoiseVar reduces an anchor's posterior to an observation variance σ²:
// it prefers an explicit standard deviation, falls back to the interval
// half-width / z(level), then to the sample variance. Returns false if none is
// available (a pin with no width cannot anchor the GP honestly).
func impliedNoiseVar(sd *float64, lower, upper, level float64, hasInterval bool, samples []float64) (float64, bool) {
	if sd != nil && *sd > 0 {
		return (*sd) * (*sd), true
	}
	if hasInterval && upper > lower && level > 0 && level < 1 {
		z := invNormalCDF(0.5 * (1 + level))
		if z > 0 {
			s := (upper - lower) / (2 * z)
			return s * s, true
		}
	}
	if len(samples) > 1 {
		var mean float64
		for _, v := range samples {
			mean += v
		}
		mean /= float64(len(samples))
		var ss float64
		for _, v := range samples {
			d := v - mean
			ss += d * d
		}
		return ss / float64(len(samples)-1), true
	}
	return 0, false
}

// invNormalCDF inverts the standard normal CDF by bisection (deterministic).
func invNormalCDF(p float64) float64 {
	lo, hi := -12.0, 12.0
	for i := 0; i < 200; i++ {
		m := 0.5 * (lo + hi)
		if normalCDF(m) < p {
			lo = m
		} else {
			hi = m
		}
	}
	return 0.5 * (lo + hi)
}
