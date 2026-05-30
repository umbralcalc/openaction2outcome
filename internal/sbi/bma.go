package sbi

import (
	"math"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/rdd"
)

// SpecWeight is one spec's contribution to the Bayesian model average.
type SpecWeight struct {
	Spec    Spec
	TauMean float64
	TauSD   float64
	LogZ    float64
	Weight  float64 // normalised BMA weight
}

// BMAResult is the model-averaged posterior over the discontinuity effect tau:
// a marginal-likelihood-weighted mixture of the per-spec Gaussian posteriors.
// Its width splits exactly (law of total variance) into within-spec sampling
// variance and between-spec identification variance.
type BMAResult struct {
	Central    float64 // mixture mean
	Median     float64
	TotalSD    float64
	WithinVar  float64 // E_k[Var(tau|spec)] — sampling
	BetweenVar float64 // Var_k[E(tau|spec)] — identification
	Specs      []SpecWeight
}

// EstimateBMA fits every spec with stochadex SMC and averages them with the
// hybrid scheme: marginal likelihood is only comparable conditional on the same
// data, so specs are weighted by SMC marginal likelihood WITHIN each bandwidth
// (where the order x kernel models share the same units), and bandwidths — which
// change the data subset, making their marginal likelihoods incomparable — are
// averaged uniformly. specPrior gives an optional per-spec prior applied within
// bandwidth groups (nil = uniform). The resulting between-spec variance captures
// identification uncertainty across bandwidth, order, and kernel; the within-spec
// variance is sampling uncertainty.
func EstimateBMA(
	pts []rdd.Point,
	cutoff float64,
	specs []Spec,
	specPrior []float64,
	treatedBelow bool,
	cfg SMCConfig,
) BMAResult {
	type fitted struct {
		post     specPosterior
		logPrior float64
	}
	var fits []fitted
	for i, s := range specs {
		sf := buildSpecFit(pts, cutoff, s, treatedBelow)
		post := fitSpecSMC(sf, cfg)
		if !post.ok || post.tauVar <= 0 || math.IsNaN(post.logZ) || math.IsInf(post.logZ, 0) {
			continue
		}
		pw := 1.0
		if specPrior != nil {
			pw = specPrior[i]
		}
		fits = append(fits, fitted{post: post, logPrior: math.Log(pw)})
	}
	if len(fits) == 0 {
		return BMAResult{}
	}

	// Group surviving specs by bandwidth (same data within a group).
	groups := make(map[float64][]int)
	var bwOrder []float64
	for i, f := range fits {
		h := f.post.spec.H
		if _, seen := groups[h]; !seen {
			bwOrder = append(bwOrder, h)
		}
		groups[h] = append(groups[h], i)
	}
	groupWeight := 1.0 / float64(len(bwOrder)) // uniform across bandwidths

	weights := make([]float64, len(fits))
	for _, h := range bwOrder {
		idxs := groups[h]
		logs := make([]float64, len(idxs))
		for j, ix := range idxs {
			logs[j] = fits[ix].post.logZ + fits[ix].logPrior // valid: shared data
		}
		lse := logSumExp(logs)
		for j, ix := range idxs {
			weights[ix] = groupWeight * math.Exp(logs[j]-lse)
		}
	}

	res := BMAResult{Specs: make([]SpecWeight, len(fits))}
	for i, f := range fits {
		res.Specs[i] = SpecWeight{
			Spec: f.post.spec, TauMean: f.post.tauMean, TauSD: math.Sqrt(f.post.tauVar),
			LogZ: f.post.logZ, Weight: weights[i],
		}
		res.Central += weights[i] * f.post.tauMean
		res.WithinVar += weights[i] * f.post.tauVar
	}
	for _, sw := range res.Specs {
		d := sw.TauMean - res.Central
		res.BetweenVar += sw.Weight * d * d
	}
	res.TotalSD = math.Sqrt(res.WithinVar + res.BetweenVar)
	res.Median = res.Quantile(0.5)
	// Keep specs in a stable, human-friendly order (heaviest weight first).
	sort.SliceStable(res.Specs, func(i, j int) bool { return res.Specs[i].Weight > res.Specs[j].Weight })
	return res
}

// components returns the (mean, sd, weight) mixture components.
func (r BMAResult) components() (means, sds, weights []float64) {
	means = make([]float64, len(r.Specs))
	sds = make([]float64, len(r.Specs))
	weights = make([]float64, len(r.Specs))
	for i, s := range r.Specs {
		means[i], sds[i], weights[i] = s.TauMean, s.TauSD, s.Weight
	}
	return
}

// cdf evaluates the mixture CDF at x.
func (r BMAResult) cdf(x float64) float64 {
	means, sds, weights := r.components()
	var c float64
	for i := range means {
		c += weights[i] * normalCDF((x-means[i])/sds[i])
	}
	return c
}

// Quantile inverts the mixture CDF by bisection.
func (r BMAResult) Quantile(p float64) float64 {
	if len(r.Specs) == 0 {
		return 0
	}
	if p <= 0 {
		p = 1e-9
	}
	if p >= 1 {
		p = 1 - 1e-9
	}
	// Bracket using the widest component.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, s := range r.Specs {
		lo = math.Min(lo, s.TauMean-10*s.TauSD)
		hi = math.Max(hi, s.TauMean+10*s.TauSD)
	}
	for i := 0; i < 200; i++ {
		mid := 0.5 * (lo + hi)
		if r.cdf(mid) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return 0.5 * (lo + hi)
}

// Interval returns the central credible interval at the given coverage level.
func (r BMAResult) Interval(level float64) (lo, hi float64) {
	a := 0.5 * (1 - level)
	return r.Quantile(a), r.Quantile(1 - a)
}

// Quantiles returns (p, value) pairs at the supplied probabilities.
func (r BMAResult) Quantiles(ps []float64) [][2]float64 {
	out := make([][2]float64, len(ps))
	for i, p := range ps {
		out[i] = [2]float64{p, r.Quantile(p)}
	}
	return out
}

// Samples returns k deterministic samples via stratified inverse-CDF (the
// quantiles at (i+0.5)/k). Deterministic so marks are re-mintable, and a faithful
// empirical representation of the mixture for CRPS-style scoring.
func (r BMAResult) Samples(k int) []float64 {
	out := make([]float64, k)
	for i := 0; i < k; i++ {
		out[i] = r.Quantile((float64(i) + 0.5) / float64(k))
	}
	return out
}

func logSumExp(xs []float64) float64 {
	if len(xs) == 0 {
		return math.Inf(-1)
	}
	mx := xs[0]
	for _, x := range xs[1:] {
		if x > mx {
			mx = x
		}
	}
	if math.IsInf(mx, -1) {
		return mx
	}
	var s float64
	for _, x := range xs {
		s += math.Exp(x - mx)
	}
	return mx + math.Log(s)
}

// normalCDF is the standard normal CDF.
func normalCDF(z float64) float64 {
	return 0.5 * math.Erfc(-z/math.Sqrt2)
}

// gridSpecs builds a bandwidth x order x kernel specification grid.
func gridSpecs(bandwidths []float64, orders []int, kernels []Kernel) []Spec {
	specs := make([]Spec, 0, len(bandwidths)*len(orders)*len(kernels))
	for _, k := range kernels {
		for _, o := range orders {
			for _, h := range bandwidths {
				specs = append(specs, Spec{H: h, Order: o, Kernel: k})
			}
		}
	}
	return specs
}

// DefaultFloorSpecs is the specification grid for the floor-standards series,
// whose running variable (Progress 8) spans a few points.
func DefaultFloorSpecs() []Spec {
	return gridSpecs([]float64{0.4, 0.5, 0.6, 0.7, 0.8}, []int{1, 2}, []Kernel{Triangular, Boxcar})
}

// DefaultSHMISpecs is the grid for the SHMI series, whose running variable
// (SHMI minus its control limit) spans a much smaller range, so the bandwidths
// are correspondingly tighter.
func DefaultSHMISpecs() []Spec {
	return gridSpecs([]float64{0.08, 0.12, 0.16, 0.20, 0.25}, []int{1, 2}, []Kernel{Triangular, Boxcar})
}
