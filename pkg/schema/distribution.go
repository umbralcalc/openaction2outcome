package schema

import (
	"errors"
	"sort"
)

// Distribution is the common representation of an uncertain scalar effect used
// on both sides of the yardstick: a Mark's honest interval over the true effect,
// and a model's predicted effect with its own uncertainty.
//
// The width of a Mark's Distribution is meant to absorb *identification*
// uncertainty (bandwidth, polynomial order, kernel) and not only sampling
// error — that is the whole point of the instrument (BRIEF §3, §4). The
// representation is deliberately redundant so that both cheap (interval-only)
// and rich (sample-based, for CRPS) consumers are served:
//
//   - Central is always present (posterior median / point estimate).
//   - Interval is the headline honest interval (e.g. 95%).
//   - Quantiles and/or Samples may be supplied for distribution-vs-distribution
//     scoring (CRPS); evaluators degrade gracefully when they are absent.
type Distribution struct {
	// Central is the central estimate of the effect (posterior median for SBI
	// marks; the point estimate for a plug-in fit). Always required.
	Central float64 `json:"central"`

	// Mean and StdDev are optional moment summaries.
	Mean   *float64 `json:"mean,omitempty"`
	StdDev *float64 `json:"std_dev,omitempty"`

	// Interval is the headline honest interval. Required for a Mark.
	Interval *Interval `json:"interval,omitempty"`

	// Quantiles is an optional richer description of the distribution, sorted
	// ascending by P. Used for finer calibration / CRPS approximation.
	Quantiles []Quantile `json:"quantiles,omitempty"`

	// Samples is an optional sample-based representation (e.g. SBI posterior
	// draws), enabling a direct empirical CRPS. Order is not significant.
	Samples []float64 `json:"samples,omitempty"`

	// UncertaintyBudget optionally attributes interval width to its sources
	// (sampling vs bandwidth vs specification). Informational; it documents the
	// claim that identification uncertainty — not just sampling SE — is included.
	UncertaintyBudget *UncertaintyBudget `json:"uncertainty_budget,omitempty"`
}

// Interval is a central credible/confidence interval at a stated coverage Level
// (e.g. 0.95 for a 95% interval).
type Interval struct {
	Level float64 `json:"level"`
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// Quantile is a single (probability, value) point of a distribution's CDF.
type Quantile struct {
	P     float64 `json:"p"`
	Value float64 `json:"value"`
}

// UncertaintyBudget attributes the variance (or interval half-width) of a mark's
// effect distribution to its identification sources. Fields are in the same
// units as the variance contribution; nil means "not separately quantified".
// The headline claim of the yardstick is that Specification + Bandwidth are
// non-trivial — that is why plug-in (Sampling-only) methods fail Track B.
type UncertaintyBudget struct {
	Sampling      *float64 `json:"sampling,omitempty"`
	Bandwidth     *float64 `json:"bandwidth,omitempty"`
	Specification *float64 `json:"specification,omitempty"`
}

// Validate checks a Distribution for internal consistency.
func (d Distribution) Validate() error {
	if d.Interval != nil {
		iv := d.Interval
		if !(iv.Level > 0 && iv.Level < 1) {
			return errors.New("distribution: interval level must be in (0,1)")
		}
		if iv.Lower > iv.Upper {
			return errors.New("distribution: interval lower exceeds upper")
		}
		if d.Central < iv.Lower || d.Central > iv.Upper {
			return errors.New("distribution: central estimate lies outside its interval")
		}
	}
	if d.StdDev != nil && *d.StdDev < 0 {
		return errors.New("distribution: std_dev is negative")
	}
	prev := -1.0
	for _, q := range d.Quantiles {
		if !(q.P >= 0 && q.P <= 1) {
			return errors.New("distribution: quantile p must be in [0,1]")
		}
		if q.P < prev {
			return errors.New("distribution: quantiles must be sorted ascending by p")
		}
		prev = q.P
	}
	return nil
}

// QuantileAt returns the value at probability p, linearly interpolating between
// supplied Quantiles, or — if no Quantiles are present but Samples are — the
// empirical sample quantile. The second return is false when neither source can
// answer (only Central/Interval present).
func (d Distribution) QuantileAt(p float64) (float64, bool) {
	if len(d.Quantiles) > 0 {
		qs := d.Quantiles
		if p <= qs[0].P {
			return qs[0].Value, true
		}
		if p >= qs[len(qs)-1].P {
			return qs[len(qs)-1].Value, true
		}
		for i := 1; i < len(qs); i++ {
			if p <= qs[i].P {
				lo, hi := qs[i-1], qs[i]
				if hi.P == lo.P {
					return lo.Value, true
				}
				frac := (p - lo.P) / (hi.P - lo.P)
				return lo.Value + frac*(hi.Value-lo.Value), true
			}
		}
	}
	if len(d.Samples) > 0 {
		s := append([]float64(nil), d.Samples...)
		sort.Float64s(s)
		if p <= 0 {
			return s[0], true
		}
		if p >= 1 {
			return s[len(s)-1], true
		}
		idx := p * float64(len(s)-1)
		lo := int(idx)
		frac := idx - float64(lo)
		if lo+1 >= len(s) {
			return s[lo], true
		}
		return s[lo] + frac*(s[lo+1]-s[lo]), true
	}
	return 0, false
}
