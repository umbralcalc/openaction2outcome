// Package score is the public, dependency-light evaluator. It compares a model's
// predicted effect distribution against a reference mark on two independent
// scores:
//
//	DecisionScore    — does the model get the direction of the effect right, and
//	                   what does a wrong call cost?
//	CalibrationScore — does the model's stated uncertainty match the truth
//	                   (interval overlap, a CRPS-style distribution distance, a
//	                   calibration curve, and a confidently-wrong flag)?
//
// It imports only the standard library and pkg/schema, so a consumer scoring a
// model pulls a tiny dependency tree; the estimation machinery stays in /internal.
package score

import (
	"math"
	"sort"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// cdfEval returns P(X <= x) for a schema.Distribution, using the richest
// representation available: empirical CDF from samples, else a piecewise-linear
// CDF through the supplied quantiles, else a Gaussian implied by the central
// estimate and interval. The boolean reports whether any representation was
// usable.
func cdfEval(d schema.Distribution, x float64) (float64, bool) {
	if len(d.Samples) > 0 {
		n := 0
		for _, s := range d.Samples {
			if s <= x {
				n++
			}
		}
		return float64(n) / float64(len(d.Samples)), true
	}
	if len(d.Quantiles) > 0 {
		qs := d.Quantiles // assumed sorted ascending by P (validated upstream)
		if x <= qs[0].Value {
			return qs[0].P, true
		}
		if x >= qs[len(qs)-1].Value {
			return qs[len(qs)-1].P, true
		}
		for i := 1; i < len(qs); i++ {
			if x <= qs[i].Value {
				lo, hi := qs[i-1], qs[i]
				if hi.Value == lo.Value {
					return hi.P, true
				}
				frac := (x - lo.Value) / (hi.Value - lo.Value)
				return lo.P + frac*(hi.P-lo.P), true
			}
		}
	}
	if sd, ok := impliedStdDev(d); ok {
		return gaussianCDF(x, d.Central, sd), true
	}
	return 0, false
}

// impliedStdDev recovers a standard deviation from a distribution: the explicit
// StdDev if present, otherwise inferred from a symmetric interval treated as
// Gaussian (half-width / z(level)).
func impliedStdDev(d schema.Distribution) (float64, bool) {
	if d.StdDev != nil && *d.StdDev > 0 {
		return *d.StdDev, true
	}
	if d.Interval != nil && d.Interval.Upper > d.Interval.Lower {
		half := (d.Interval.Upper - d.Interval.Lower) / 2
		z := normalQuantile(0.5 + d.Interval.Level/2)
		if z > 0 {
			return half / z, true
		}
	}
	return 0, false
}

// support returns a representative sorted set of x-values spanning a
// distribution, used to build an integration grid.
func support(d schema.Distribution) []float64 {
	switch {
	case len(d.Samples) > 0:
		s := append([]float64(nil), d.Samples...)
		sort.Float64s(s)
		return s
	case len(d.Quantiles) > 0:
		v := make([]float64, len(d.Quantiles))
		for i, q := range d.Quantiles {
			v[i] = q.Value
		}
		sort.Float64s(v)
		return v
	default:
		if sd, ok := impliedStdDev(d); ok {
			return []float64{d.Central - 4*sd, d.Central, d.Central + 4*sd}
		}
		return []float64{d.Central}
	}
}

// gaussianCDF is the standard normal CDF scaled to (mean, sd).
func gaussianCDF(x, mean, sd float64) float64 {
	return 0.5 * (1 + math.Erf((x-mean)/(sd*math.Sqrt2)))
}

// normalQuantile is the inverse standard-normal CDF (probit), via the
// Acklam/Beasley-Springer rational approximation. Accurate to ~1e-9, which is
// far more than enough for interval<->sd conversions here.
func normalQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	// Coefficients for the rational approximation.
	a := []float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02, 1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := []float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02, 6.680131188771972e+01, -1.328068155288572e+01}
	c := []float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00, -2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := []float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00, 3.754408661907416e+00}
	const plow = 0.02425
	const phigh = 1 - plow
	switch {
	case p < plow:
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p <= phigh:
		q := p - 0.5
		r := q * q
		return (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q /
			(((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	default:
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}
}
