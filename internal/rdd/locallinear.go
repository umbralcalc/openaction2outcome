// Package rdd is the plug-in regression-discontinuity estimator: a triangular-
// kernel local-linear fit on each side of the cutoff. The reported interval
// folds bandwidth/specification spread into its width, not only sampling SE.
// It serves as the simple comparison baseline against the model-averaged
// estimator in package sbi.
package rdd

import (
	"math"
	"sort"
)

// Point is one unit: running variable X and outcome Y.
type Point struct {
	X, Y float64
}

// SideFit is a one-sided weighted local-linear fit evaluated at the cutoff.
type SideFit struct {
	Intercept float64 // predicted outcome at the cutoff
	Slope     float64
	VarInt    float64 // variance of the intercept
	N         int     // points with positive kernel weight
}

// triangular returns the triangular kernel weight for a point at distance d
// (in running-variable units) given bandwidth h.
func triangular(d, h float64) float64 {
	w := 1 - math.Abs(d)/h
	if w < 0 {
		return 0
	}
	return w
}

// fitSide runs a kernel-weighted least-squares line on the points whose running
// value is on the requested side of the cutoff, and evaluates it at the cutoff.
// treatedBelow selects which side is "treated" only for documentation; this
// routine just fits whichever subset is passed via `below`.
func fitSide(pts []Point, cutoff, h float64, below bool) SideFit {
	var sw, swx, swxx, swy, swxy float64
	var n int
	for _, p := range pts {
		if below && p.X > cutoff {
			continue
		}
		if !below && p.X < cutoff {
			continue
		}
		// Points exactly at the cutoff are assigned to neither strict side here;
		// they contribute to whichever side the caller designates via `below`
		// when X==cutoff. To keep the boundary unambiguous we include X==cutoff
		// on the treated (below==true) side only when below, else on the control.
		d := p.X - cutoff
		w := triangular(d, h)
		if w <= 0 {
			continue
		}
		x := d
		sw += w
		swx += w * x
		swxx += w * x * x
		swy += w * p.Y
		swxy += w * x * p.Y
		n++
	}
	det := sw*swxx - swx*swx
	if n < 2 || det == 0 {
		return SideFit{N: n}
	}
	beta0 := (swxx*swy - swx*swxy) / det
	beta1 := (sw*swxy - swx*swy) / det

	// Residual variance (weighted), then Var(beta0) = sigma^2 * Swxx/det.
	var ssr float64
	for _, p := range pts {
		if below && p.X > cutoff {
			continue
		}
		if !below && p.X < cutoff {
			continue
		}
		d := p.X - cutoff
		w := triangular(d, h)
		if w <= 0 {
			continue
		}
		r := p.Y - (beta0 + beta1*d)
		ssr += w * r * r
	}
	dof := float64(n - 2)
	if dof <= 0 {
		dof = 1
	}
	sigma2 := ssr / dof
	return SideFit{Intercept: beta0, Slope: beta1, VarInt: sigma2 * swxx / det, N: n}
}

// Fit estimates the RD effect tau = E[Y|treated side at c] - E[Y|control side at
// c] at a single bandwidth. For a floor-style design treatedBelow=true (units
// below the cutoff receive the action).
func Fit(pts []Point, cutoff, h float64, treatedBelow bool) (tau, se float64, nTreat, nCtrl int) {
	treat := fitSide(pts, cutoff, h, treatedBelow)
	ctrl := fitSide(pts, cutoff, h, !treatedBelow)
	tau = treat.Intercept - ctrl.Intercept
	se = math.Sqrt(treat.VarInt + ctrl.VarInt)
	return tau, se, treat.N, ctrl.N
}

// SweepPoint records the estimate at one bandwidth.
type SweepPoint struct {
	Bandwidth float64
	Tau       float64
	SE        float64
	NTreat    int
	NCtrl     int
}

// Result is the honest summary of a discontinuity estimate.
type Result struct {
	Central       float64      // tau at the reference bandwidth
	SamplingVar   float64      // sampling variance at the reference bandwidth
	SpecVar       float64      // variance of tau across the bandwidth sweep (identification spread)
	TotalSD       float64      // sqrt(SamplingVar + SpecVar)
	RefBandwidth  float64      //
	Sweep         []SweepPoint //
	NTreat, NCtrl int          // sample sizes at the reference bandwidth
}

// Interval95 returns the central 95% interval implied by TotalSD (Gaussian).
func (r Result) Interval95() (lo, hi float64) {
	const z = 1.959963984540054
	return r.Central - z*r.TotalSD, r.Central + z*r.TotalSD
}

// Estimate runs a bandwidth sweep and folds the sweep spread into the interval.
// The reported Central is the estimate at refBW; the interval width combines the
// sampling variance at refBW with the *specification* variance (spread of tau
// across bandwidths) — so a method that ignored bandwidth uncertainty (plug-in)
// would report a narrower, less honest interval. bandwidths must be non-empty
// and include refBW.
func Estimate(pts []Point, cutoff, refBW float64, bandwidths []float64, treatedBelow bool) Result {
	bw := append([]float64(nil), bandwidths...)
	sort.Float64s(bw)
	res := Result{RefBandwidth: refBW}
	taus := make([]float64, 0, len(bw))
	for _, h := range bw {
		tau, se, nt, nc := Fit(pts, cutoff, h, treatedBelow)
		res.Sweep = append(res.Sweep, SweepPoint{Bandwidth: h, Tau: tau, SE: se, NTreat: nt, NCtrl: nc})
		taus = append(taus, tau)
		if h == refBW {
			res.Central = tau
			res.SamplingVar = se * se
			res.NTreat, res.NCtrl = nt, nc
		}
	}
	res.SpecVar = variance(taus)
	res.TotalSD = math.Sqrt(res.SamplingVar + res.SpecVar)
	return res
}

func variance(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var mean float64
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	var s float64
	for _, x := range xs {
		d := x - mean
		s += d * d
	}
	return s / float64(len(xs)-1)
}
