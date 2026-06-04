package its

import (
	"math"
	"sort"
)

// PreTrend is the parallel-trends-in-time diagnostic: over the pre-intervention
// window only, it fits the difference series D(t) = Treated(t)-Control(t) to a line
// D ~ a + b*(t-t0) and returns the slope b, its OLS standard error, and the number
// of pre months. Under parallel trends the treated and control series move together
// pre-break, so b ≈ 0; a materially non-zero pre-slope means the control is a poor
// counterfactual. Reported, never folded into the effect.
func PreTrend(pts []Point, t0, preStartT float64) (slope, se float64, n int) {
	var ts, ds []float64
	for _, p := range pts {
		if p.T >= preStartT && p.T < t0 {
			ts = append(ts, p.T-t0)
			ds = append(ds, p.Treated-p.Control)
		}
	}
	return olsLine(ts, ds)
}

// PlaceboFit refits the full segmented specification with a FAKE intervention
// instant fakeT0 placed wholly in the pre-period, using only genuinely
// pre-intervention months (T < realT0) so no real treatment leaks in. A credible
// design returns an effect indistinguishable from zero at every placebo date.
func PlaceboFit(pts []Point, realT0, fakeT0 float64, spec Spec) FitResult {
	var pre []Point
	for _, p := range pts {
		if p.T < realT0 {
			pre = append(pre, p)
		}
	}
	// Restrict the spec's pre-start so enough months sit each side of the fake date.
	s := spec
	return Fit(pre, fakeT0, s)
}

// NoAnticipation tests for a break/forestalling in the lead months just before the
// real instant: it places a placebo break `lead` months before realT0 (using only
// pre-instant data) and returns its effect and SE. An effect within ~1.96 SE of zero
// means no detectable anticipation. This is the ITS analogue of the RDD no-sorting
// (density) test.
func NoAnticipation(pts []Point, realT0 float64, lead int, spec Spec) FitResult {
	return PlaceboFit(pts, realT0, realT0-float64(lead), spec)
}

// olsLine fits y ~ a + b*x by OLS and returns slope b, its standard error, and n.
// SE is +Inf when not estimable (fewer than three points or no spread in x).
func olsLine(xs, ys []float64) (slope, se float64, n int) {
	n = len(xs)
	if n < 2 {
		return 0, math.Inf(1), n
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= float64(n)
	my /= float64(n)
	var sxx, sxy float64
	for i := range xs {
		dx := xs[i] - mx
		sxx += dx * dx
		sxy += dx * (ys[i] - my)
	}
	if sxx == 0 {
		return 0, math.Inf(1), n
	}
	slope = sxy / sxx
	if n < 3 {
		return slope, math.Inf(1), n
	}
	intercept := my - slope*mx
	var ssr float64
	for i := range xs {
		r := ys[i] - (intercept + slope*xs[i])
		ssr += r * r
	}
	sigma2 := ssr / float64(n-2)
	return slope, math.Sqrt(sigma2 / sxx), n
}

// MonthsSorted returns the points sorted by running time (helper for callers that
// build the panel from an unordered map).
func MonthsSorted(pts []Point) []Point {
	out := append([]Point(nil), pts...)
	sort.Slice(out, func(i, j int) bool { return out[i].T < out[j].T })
	return out
}
