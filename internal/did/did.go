// Package did is the plug-in difference-in-differences estimator: a clean 2×2
// (unit-first-difference) DiD comparing the pre→post change in a treated group to a
// control group, with unit-clustered inference and an honest interval that folds the
// estimation-window / specification spread into its width — mirroring the
// regression-discontinuity estimators in internal/rdd.
//
// Why this design, and why now. Each anchor on a dose/staggered-rollout mechanism
// (a policy intensity ratcheted up in steps, or a scheme rolled out across areas at
// different times) is ONE difference-in-differences event — one treated group vs a
// clean control, pre vs post. A FAMILY of such anchors at different doses is exactly
// what a bridge mark interpolates across. So a DiD estimator is the gating
// prerequisite for the dose/rollout bridge seams (alcohol minimum-unit pricing,
// emission zones, minimum wage) the way the regression-kink estimator was for
// tiered-relief seams.
//
// Deliberate scope: a clean two-group 2×2 (or block) DiD around a SINGLE event,
// estimated by differencing each unit's post-mean minus pre-mean and comparing group
// means. This is the unit of a bridge anchor and it sidesteps the well-known
// staggered-adoption / two-way-fixed-effects bias that afflicts a single pooled
// regression over many heterogeneous events. Inference clusters at the unit level
// (the variance of the unit first-differences within each group), which is the
// standard robust DiD standard error.
//
// The identifying assumption is PARALLEL TRENDS, so the estimator ships a pre-trend
// diagnostic (the cross-group trend in the pre-period, which should be ≈0) — the DiD
// analogue of the RDD density/continuity manipulation check. It is reported, not
// folded into the effect.
package did

import (
	"math"
	"sort"
)

// Unit is one panel unit's time series and its treatment-group membership.
type Unit struct {
	ID      string
	Treated bool      // ever-treated (treatment group) vs control (never-treated)
	Times   []float64 // observation times (any order)
	Y       []float64 // outcome, aligned to Times
}

// unitDelta returns a unit's post-mean minus pre-mean within a window of half-width
// h around treatTime: pre = times in [treatTime−h, treatTime), post = times in
// [treatTime, treatTime+h]. ok is false if the unit lacks an observation on either
// side (it then contributes to neither group).
func unitDelta(u Unit, treatTime, h float64) (float64, bool) {
	var preSum, postSum float64
	var preN, postN int
	for i, t := range u.Times {
		switch {
		case t >= treatTime-h && t < treatTime:
			preSum += u.Y[i]
			preN++
		case t >= treatTime && t <= treatTime+h:
			postSum += u.Y[i]
			postN++
		}
	}
	if preN == 0 || postN == 0 {
		return 0, false
	}
	return postSum/float64(postN) - preSum/float64(preN), true
}

// FitDiD estimates the DiD effect at one window half-width h: the treated group's
// mean pre→post change minus the control group's, with a unit-clustered standard
// error (the two-sample SE of the unit first-differences). Returns the effect, its
// SE, and the treated/control unit counts that contributed.
func FitDiD(units []Unit, treatTime, h float64) (tau, se float64, nTreat, nControl int) {
	var dT, dC []float64
	for _, u := range units {
		d, ok := unitDelta(u, treatTime, h)
		if !ok {
			continue
		}
		if u.Treated {
			dT = append(dT, d)
		} else {
			dC = append(dC, d)
		}
	}
	mT, vT := meanVar(dT)
	mC, vC := meanVar(dC)
	tau = mT - mC
	var seVar float64
	if len(dT) > 0 {
		seVar += vT / float64(len(dT))
	}
	if len(dC) > 0 {
		seVar += vC / float64(len(dC))
	}
	return tau, math.Sqrt(seVar), len(dT), len(dC)
}

// PreTrend is the parallel-trends diagnostic: using ONLY pre-period data in
// [treatTime−h, treatTime), it forms the cross-group mean difference
// D(t) = meanY_treated(t) − meanY_control(t) at each pre time, and fits a line
// D ~ a + b·t. The slope b is ≈0 when trends are parallel; a materially non-zero
// slope means the control is a poor counterfactual and the DiD is not credible.
// Returns the slope, its standard error, and the number of pre points used.
func PreTrend(units []Unit, treatTime, h float64) (slope, se float64, nPoints int) {
	// Collect per-time group sums in the pre window.
	type acc struct {
		sumT, sumC float64
		nT, nC     int
	}
	byTime := map[float64]*acc{}
	for _, u := range units {
		for i, t := range u.Times {
			if t >= treatTime-h && t < treatTime {
				a := byTime[t]
				if a == nil {
					a = &acc{}
					byTime[t] = a
				}
				if u.Treated {
					a.sumT += u.Y[i]
					a.nT++
				} else {
					a.sumC += u.Y[i]
					a.nC++
				}
			}
		}
	}
	// Gather the pre-period times in sorted order so the OLS sums below run in a
	// deterministic sequence — float addition is not associative, so iterating the
	// byTime map in Go's randomised order would make the slope wobble between rebuilds.
	times := make([]float64, 0, len(byTime))
	for t := range byTime {
		times = append(times, t)
	}
	sort.Float64s(times)
	var ts, ds []float64
	for _, t := range times {
		a := byTime[t]
		if a.nT == 0 || a.nC == 0 {
			continue
		}
		ts = append(ts, t)
		ds = append(ds, a.sumT/float64(a.nT)-a.sumC/float64(a.nC))
	}
	return olsSlope(ts, ds)
}

// WindowPoint records the DiD estimate at one window half-width.
type WindowPoint struct {
	Window   float64
	Tau      float64
	SE       float64
	NTreat   int
	NControl int
}

// Result is the honest summary of a DiD estimate, mirroring rdd.Result.
type Result struct {
	Central       float64
	SamplingVar   float64 // unit-clustered sampling variance at the reference window
	SpecVar       float64 // variance of the effect across the window sweep
	TotalSD       float64 // sqrt(SamplingVar + SpecVar)
	RefWindow     float64
	NTreat        int
	NControl      int
	PreTrendSlope float64 // parallel-trends diagnostic (cross-group pre-trend slope)
	PreTrendSE    float64
	Windows       []WindowPoint
}

// Interval95 returns the central 95% interval implied by TotalSD (Gaussian).
func (r Result) Interval95() (lo, hi float64) {
	const z = 1.959963984540054
	return r.Central - z*r.TotalSD, r.Central + z*r.TotalSD
}

// Estimate runs a window sweep and folds the spread into the interval, exactly as
// rdd.Estimate does for the discontinuity effect: the reported Central is the effect
// at refWindow, and the interval width combines the unit-clustered sampling variance
// at refWindow with the specification variance (spread of the effect across windows),
// so a method that ignored window choice would report a narrower, less honest
// interval. windows must be non-empty and include refWindow. The parallel-trends
// diagnostic is reported at refWindow.
func Estimate(units []Unit, treatTime, refWindow float64, windows []float64) Result {
	res := Result{RefWindow: refWindow}
	taus := make([]float64, 0, len(windows))
	for _, h := range windows {
		tau, se, nt, nc := FitDiD(units, treatTime, h)
		res.Windows = append(res.Windows, WindowPoint{Window: h, Tau: tau, SE: se, NTreat: nt, NControl: nc})
		taus = append(taus, tau)
		if h == refWindow {
			res.Central = tau
			res.SamplingVar = se * se
			res.NTreat, res.NControl = nt, nc
		}
	}
	res.SpecVar = variance(taus)
	res.TotalSD = math.Sqrt(res.SamplingVar + res.SpecVar)
	res.PreTrendSlope, res.PreTrendSE, _ = PreTrend(units, treatTime, refWindow)
	return res
}

// meanVar returns the sample mean and the (n−1) sample variance of xs; variance is
// 0 when fewer than two observations (it cannot be estimated).
func meanVar(xs []float64) (mean, varc float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	for _, x := range xs {
		d := x - mean
		varc += d * d
	}
	varc /= float64(len(xs) - 1)
	return mean, varc
}

// variance returns the (n−1) sample variance of xs (0 for fewer than two values).
func variance(xs []float64) float64 {
	_, v := meanVar(xs)
	return v
}

// olsSlope fits D ~ a + b·t by ordinary least squares and returns the slope b, its
// standard error, and the number of points. Slope and SE are 0 / +Inf-safe for
// degenerate inputs (fewer than three points, or no spread in t).
func olsSlope(ts, ds []float64) (slope, se float64, n int) {
	n = len(ts)
	if n < 2 {
		return 0, math.Inf(1), n
	}
	var mt, md float64
	for i := range ts {
		mt += ts[i]
		md += ds[i]
	}
	mt /= float64(n)
	md /= float64(n)
	var sxx, sxy float64
	for i := range ts {
		dx := ts[i] - mt
		sxx += dx * dx
		sxy += dx * (ds[i] - md)
	}
	if sxx == 0 {
		return 0, math.Inf(1), n
	}
	slope = sxy / sxx
	intercept := md - slope*mt
	if n < 3 {
		return slope, math.Inf(1), n // slope identified but SE not estimable
	}
	var ssr float64
	for i := range ts {
		r := ds[i] - (intercept + slope*ts[i])
		ssr += r * r
	}
	sigma2 := ssr / float64(n-2)
	return slope, math.Sqrt(sigma2 / sxx), n
}
