// Package validity implements the validity battery: the manipulation,
// covariate-continuity, placebo, and robustness tests every mark ships in its
// dossier. Each returns a structured schema result embedded in the mark.
package validity

import (
	"fmt"
	"math"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/rdd"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// normalSF is the standard-normal survival function P(Z > z).
func normalSF(z float64) float64 { return 0.5 * math.Erfc(z/math.Sqrt2) }

// twoSidedP returns the two-sided normal p-value for a z statistic.
func twoSidedP(z float64) float64 { return 2 * normalSF(math.Abs(z)) }

func f64ptr(f float64) *float64 { return &f }

// DensityTest is a binned, slope-aware manipulation test (McCrary-style). It
// bins the running variable within ±h of the cutoff, fits a separate line to bin
// counts on each side, and tests whether the density is discontinuous at the
// cutoff (which sorting/manipulation would produce). Poisson bin-count variance
// is propagated into the jump's SE.
func DensityTest(running []float64, cutoff, binW, h float64) schema.TestResult {
	left := binCounts(running, cutoff-h, cutoff, binW)
	right := binCounts(running, cutoff, cutoff+h, binW)

	fL, vL, okL := poissonInterceptAtRight(left, cutoff) // density approaching cutoff from below
	fR, vR, okR := poissonInterceptAtLeft(right, cutoff) // density approaching cutoff from above
	res := schema.TestResult{Method: "binned-density-discontinuity (McCrary-style)"}
	if !okL || !okR || (vL+vR) <= 0 {
		res.Detail = "insufficient bins to estimate density either side of the cutoff"
		return res
	}
	jump := fR - fL
	se := math.Sqrt(vL + vR)
	z := jump / se
	p := twoSidedP(z)
	res.Statistic = f64ptr(jump)
	res.PValue = f64ptr(p)
	res.Passed = p > 0.05
	res.Detail = fmt.Sprintf("density at cutoff: below=%.1f above=%.1f jump=%.1f (z=%.2f); no significant discontinuity => no detected sorting", fL, fR, jump, z)
	return res
}

// CovariateContinuity tests that a pre-treatment covariate does not jump at the
// cutoff (a jump would signal that units either side differ for reasons other
// than the treatment). It reuses the RD estimator with the covariate as the
// outcome; passing means the estimated jump is within ~2 SE of zero.
func CovariateContinuity(name string, pts []rdd.Point, cutoff, h float64, treatedBelow bool) schema.NamedTestResult {
	tau, se, nTreat, nCtrl := rdd.Fit(pts, cutoff, h, treatedBelow)
	method := "covariate-continuity (local-linear jump in covariate)"
	// A degenerate fit (no usable data either side) must NOT report a free pass:
	// "no jump because no data" is inconclusive, which fails admission.
	if se <= 0 || nTreat < 2 || nCtrl < 2 {
		return schema.NamedTestResult{
			Name: name,
			TestResult: schema.TestResult{
				Method: method,
				Passed: false,
				Detail: fmt.Sprintf("inconclusive: insufficient data (n_treat=%d n_ctrl=%d se=%.4g)", nTreat, nCtrl, se),
			},
		}
	}
	z := tau / se
	p := twoSidedP(z)
	return schema.NamedTestResult{
		Name: name,
		TestResult: schema.TestResult{
			Method:    method,
			Statistic: f64ptr(tau),
			PValue:    f64ptr(p),
			Passed:    p > 0.05,
			Detail:    fmt.Sprintf("jump=%.4g se=%.4g z=%.2f (n_treat=%d n_ctrl=%d)", tau, se, z, nTreat, nCtrl),
		},
	}
}

// PlaceboCutoffs estimates the RD effect at false cutoffs placed wholly within
// one side of the real cutoff (so a non-zero estimate would indicate the method
// finds spurious jumps where none should exist). A placebo passes when its
// estimate is within 1.96 SE of zero.
func PlaceboCutoffs(pts []rdd.Point, cutoffs []float64, h float64, treatedBelow bool) []schema.PlaceboResult {
	out := make([]schema.PlaceboResult, 0, len(cutoffs))
	for _, c := range cutoffs {
		tau, se, _, _ := rdd.Fit(pts, c, h, treatedBelow)
		out = append(out, schema.PlaceboResult{
			Cutoff:   c,
			Estimate: tau,
			StdErr:   f64ptr(se),
			Passed:   math.Abs(tau) <= 1.96*se,
		})
	}
	return out
}

// DonutRobustness re-estimates the effect after excluding units within `radius`
// of the cutoff, guarding against heaping / exact-cutoff anomalies.
func DonutRobustness(pts []rdd.Point, cutoff, h float64, treatedBelow bool, radii []float64) []schema.SweepPoint {
	out := make([]schema.SweepPoint, 0, len(radii))
	for _, r := range radii {
		var kept []rdd.Point
		for _, p := range pts {
			if math.Abs(p.X-cutoff) >= r {
				kept = append(kept, p)
			}
		}
		tau, se, _, _ := rdd.Fit(kept, cutoff, h, treatedBelow)
		out = append(out, schema.SweepPoint{Param: r, Estimate: tau, StdErr: f64ptr(se)})
	}
	return out
}

// --- binned-density helpers -------------------------------------------------

type bin struct {
	mid   float64
	count float64
}

func binCounts(xs []float64, lo, hi, w float64) []bin {
	if w <= 0 || hi <= lo {
		return nil
	}
	n := int(math.Ceil((hi - lo) / w))
	bins := make([]bin, n)
	for i := range bins {
		bins[i].mid = lo + (float64(i)+0.5)*w
	}
	for _, x := range xs {
		if x < lo || x >= hi {
			continue
		}
		idx := int((x - lo) / w)
		if idx >= 0 && idx < n {
			bins[idx].count++
		}
	}
	return bins
}

// poissonInterceptAtRight fits count ~ (mid - at) by OLS over the supplied bins
// and returns the predicted count at x=at (the right edge for left-side bins),
// with variance propagated from Poisson bin counts (Var(count)=count). The
// boolean is false when there are too few bins.
func poissonInterceptAtRight(bins []bin, at float64) (val, varv float64, ok bool) {
	return poissonInterceptAt(bins, at)
}

// poissonInterceptAtLeft is the symmetric helper for right-side bins evaluated
// at the cutoff (the left edge); the OLS math is identical once recentred.
func poissonInterceptAtLeft(bins []bin, at float64) (val, varv float64, ok bool) {
	return poissonInterceptAt(bins, at)
}

// poissonInterceptAt fits an OLS line count ~ (mid-at) and evaluates it at
// (mid-at)=0, returning the intercept and its Poisson-propagated variance.
func poissonInterceptAt(bins []bin, at float64) (val, varv float64, ok bool) {
	if len(bins) < 2 {
		return 0, 0, false
	}
	b := append([]bin(nil), bins...)
	sort.Slice(b, func(i, j int) bool { return b[i].mid < b[j].mid })
	var n, sx, sxx float64
	for _, bb := range b {
		x := bb.mid - at
		n++
		sx += x
		sxx += x * x
	}
	det := n*sxx - sx*sx
	if det == 0 {
		return 0, 0, false
	}
	// beta0 = Sum(a_i * y_i) with a_i = (sxx - sx*x_i)/det; Var = Sum(a_i^2 * y_i).
	for _, bb := range b {
		x := bb.mid - at
		a := (sxx - sx*x) / det
		val += a * bb.count
		varv += a * a * bb.count // Poisson: Var(count)=count
	}
	return val, varv, true
}
