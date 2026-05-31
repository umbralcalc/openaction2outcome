package bridge

import (
	"math"
	"sort"
)

// postShape is the minimal interface a BridgePosterior needs from its underlying
// representation: a central estimate, a credible interval, a quantile grid, and
// deterministic samples. The modular calibrator backs it with a Gaussian mixture
// (gaussMixture); the joint calibrator backs it with a weighted particle set
// (empirical). Both are deterministic given the SMC seed.
type postShape interface {
	central() float64
	interval(level float64) (lo, hi float64)
	quantiles(ps []float64) [][2]float64
	samples(k int) []float64
}

// gaussMixture is a weighted mixture of Gaussians — here one component per SMC
// θ-particle, each N(meanₚ, varₚ) with normalised weight wₚ. It is the bridge
// analogue of internal/sbi's BMAResult and uses the same law-of-total-variance
// split (within = E_p[varₚ], between = Var_p[meanₚ]) and the same deterministic
// quantile/interval/sample machinery, so a bridge posterior is shaped exactly
// like an identified mark's and is re-mintable byte-for-byte (no RNG).
type gaussMixture struct {
	means   []float64
	vars    []float64
	weights []float64 // normalised
}

// central is the mixture mean Σ wₚ meanₚ.
func (g gaussMixture) central() float64 {
	var c float64
	for i := range g.means {
		c += g.weights[i] * g.means[i]
	}
	return c
}

// withinVar = E_p[varₚ]: the average GP conditional (pinning/discrepancy) variance.
func (g gaussMixture) withinVar() float64 {
	var v float64
	for i := range g.vars {
		v += g.weights[i] * g.vars[i]
	}
	return v
}

// betweenVar = Var_p[meanₚ]: the spread of per-particle means — the simulator/θ
// (identification) uncertainty.
func (g gaussMixture) betweenVar() float64 {
	c := g.central()
	var v float64
	for i := range g.means {
		d := g.means[i] - c
		v += g.weights[i] * d * d
	}
	return v
}

// cdf evaluates the mixture CDF at x.
func (g gaussMixture) cdf(x float64) float64 {
	var c float64
	for i := range g.means {
		sd := math.Sqrt(g.vars[i])
		if sd <= 0 {
			if x >= g.means[i] {
				c += g.weights[i]
			}
			continue
		}
		c += g.weights[i] * normalCDF((x-g.means[i])/sd)
	}
	return c
}

// quantile inverts the mixture CDF by bisection (deterministic).
func (g gaussMixture) quantile(p float64) float64 {
	if len(g.means) == 0 {
		return 0
	}
	if p <= 0 {
		p = 1e-9
	}
	if p >= 1 {
		p = 1 - 1e-9
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range g.means {
		sd := math.Sqrt(g.vars[i])
		lo = math.Min(lo, g.means[i]-10*sd)
		hi = math.Max(hi, g.means[i]+10*sd)
	}
	if math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		return g.central()
	}
	for i := 0; i < 200; i++ {
		mid := 0.5 * (lo + hi)
		if g.cdf(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return 0.5 * (lo + hi)
}

func (g gaussMixture) interval(level float64) (lo, hi float64) {
	a := 0.5 * (1 - level)
	return g.quantile(a), g.quantile(1 - a)
}

func (g gaussMixture) quantiles(ps []float64) [][2]float64 {
	out := make([][2]float64, len(ps))
	for i, p := range ps {
		out[i] = [2]float64{p, g.quantile(p)}
	}
	return out
}

// samples returns k deterministic stratified inverse-CDF draws (quantiles at
// (i+0.5)/k) — re-mintable and a faithful empirical CRPS representation.
func (g gaussMixture) samples(k int) []float64 {
	out := make([]float64, k)
	for i := 0; i < k; i++ {
		out[i] = g.quantile((float64(i) + 0.5) / float64(k))
	}
	return out
}

func normalCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// empirical is a weighted set of scalar particles — the joint calibrator's
// posterior over τ(query), where each SMC particle yields one (θ, δ) draw and
// hence one τ value. Unlike gaussMixture it makes no Gaussian assumption: the
// interval and quantiles are read straight off the weighted empirical CDF, so a
// skewed or multi-modal joint posterior is represented faithfully.
type empirical struct {
	xs []float64 // sorted ascending
	cw []float64 // cumulative normalised weights aligned to xs (cw[i] = Σ_{j<=i} w_j)
}

// newEmpirical builds the sorted, cumulative-weight representation from raw
// weighted particles. Weights need not be normalised.
func newEmpirical(values, weights []float64) empirical {
	type pair struct{ x, w float64 }
	ps := make([]pair, len(values))
	var total float64
	for i := range values {
		w := weights[i]
		if w < 0 {
			w = 0
		}
		ps[i] = pair{values[i], w}
		total += w
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].x < ps[j].x })
	xs := make([]float64, len(ps))
	cw := make([]float64, len(ps))
	var acc float64
	for i, p := range ps {
		xs[i] = p.x
		if total > 0 {
			acc += p.w / total
		}
		cw[i] = acc
	}
	return empirical{xs: xs, cw: cw}
}

func (e empirical) central() float64 {
	// weighted mean recovered from the cumulative weights
	if len(e.xs) == 0 {
		return 0
	}
	var mean, prev float64
	for i := range e.xs {
		w := e.cw[i] - prev
		mean += w * e.xs[i]
		prev = e.cw[i]
	}
	return mean
}

// quantile reads the weighted empirical CDF (the smallest x whose cumulative
// weight reaches p).
func (e empirical) quantile(p float64) float64 {
	if len(e.xs) == 0 {
		return 0
	}
	if p <= 0 {
		return e.xs[0]
	}
	if p >= 1 {
		return e.xs[len(e.xs)-1]
	}
	idx := sort.Search(len(e.cw), func(i int) bool { return e.cw[i] >= p })
	if idx >= len(e.xs) {
		idx = len(e.xs) - 1
	}
	return e.xs[idx]
}

func (e empirical) interval(level float64) (lo, hi float64) {
	a := 0.5 * (1 - level)
	return e.quantile(a), e.quantile(1 - a)
}

func (e empirical) quantiles(ps []float64) [][2]float64 {
	out := make([][2]float64, len(ps))
	for i, p := range ps {
		out[i] = [2]float64{p, e.quantile(p)}
	}
	return out
}

func (e empirical) samples(k int) []float64 {
	out := make([]float64, k)
	for i := 0; i < k; i++ {
		out[i] = e.quantile((float64(i) + 0.5) / float64(k))
	}
	return out
}
