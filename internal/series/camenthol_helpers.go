package series

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/umbralcalc/openaction2outcome/internal/did"
	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// caMentholChecks runs the DiD validity battery and returns the seam-specific
// checks (parallel trends [0], placebo pre-period ban [1], leave-one-province-out
// [2]) plus the admission verdict. Admission gates on parallel pre-trends holding
// and the placebo being indistinguishable from zero — a wide interval is never a
// failure.
func caMentholChecks(units []did.Unit, res did.Result) ([]schema.NamedTestResult, bool) {
	// Parallel pre-trends: the cross-group pre-period slope should be ≈0.
	slope, se := res.PreTrendSlope, res.PreTrendSE
	parallelPass := !math.IsInf(se, 0) && !math.IsNaN(se) && math.Abs(slope) <= 1.96*se
	parallel := schema.NamedTestResult{
		Name: "parallel_trends",
		TestResult: schema.TestResult{
			Method:    "cross-group pre-period trend slope of (treated − control) smoking prevalence (should be ≈0)",
			Statistic: f64ptr(slope),
			Passed:    parallelPass,
			Detail:    fmt.Sprintf("pre-trend slope %.3f pp/yr (se %.3f): trends are %s parallel", slope, se, map[bool]string{true: "statistically", false: "NOT"}[parallelPass]),
		},
	}

	// Placebo: a fake ban at 2012 using only pre-2015 data should show ~0 effect.
	ptau, psd := caPlaceboEstimate(units)
	placeboPass := !math.IsNaN(psd) && psd > 0 && math.Abs(ptau) <= 1.96*psd
	placebo := schema.NamedTestResult{
		Name: "placebo_pre_period_ban",
		TestResult: schema.TestResult{
			Method:    "fake menthol ban at 2012 using pre-period data only (should be ≈0)",
			Statistic: f64ptr(ptau),
			Passed:    placeboPass,
			Detail:    fmt.Sprintf("placebo ATT %.3f pp (sd %.3f): %s", ptau, psd, passFail(placeboPass)),
		},
	}

	// Leave-one-province-out: re-estimate dropping each treated province in turn; the
	// effect should not hinge on any single province. Reported as robustness.
	loo := caLeaveOneOut(units, res.Central)

	return []schema.NamedTestResult{parallel, placebo, loo}, parallelPass && placeboPass
}

// caPlaceboEstimate refits the DiD with a fake 2012 ban on pre-2015 data only.
func caPlaceboEstimate(units []did.Unit) (tau, sd float64) {
	var pre []did.Unit
	for _, u := range units {
		var t, y []float64
		for i, tt := range u.Times {
			if tt < 2015 {
				t = append(t, tt)
				y = append(y, u.Y[i])
			}
		}
		pre = append(pre, did.Unit{ID: u.ID, Treated: u.Treated, Times: t, Y: y})
	}
	r := did.Estimate(pre, 2012, 1, []float64{1, 2})
	return r.Central, r.TotalSD
}

// caLeaveOneOut drops each treated province in turn and reports the range of the
// re-estimated ATT; it passes when every leave-one-out estimate keeps the sign of
// the full estimate (no single province drives the result).
func caLeaveOneOut(units []did.Unit, full float64) schema.NamedTestResult {
	var minTau, maxTau float64
	first := true
	sameSign := true
	for _, drop := range units {
		if !drop.Treated {
			continue
		}
		var kept []did.Unit
		for _, u := range units {
			if u.ID != drop.ID {
				kept = append(kept, u)
			}
		}
		r := did.Estimate(kept, caTreatSplit, caRefWindow, caWindows)
		if first || r.Central < minTau {
			minTau = r.Central
		}
		if first || r.Central > maxTau {
			maxTau = r.Central
		}
		first = false
		if (r.Central < 0) != (full < 0) {
			sameSign = false
		}
	}
	return schema.NamedTestResult{
		Name: "leave_one_province_out",
		TestResult: schema.TestResult{
			Method:    "drop each treated province in turn; the ATT sign should be stable",
			Statistic: f64ptr(maxTau - minTau),
			Passed:    sameSign,
			Detail:    fmt.Sprintf("leave-one-out ATT ranges [%.3f, %.3f] vs full %.3f; sign stable=%v", minTau, maxTau, full, sameSign),
		},
	}
}

// caPlacebo renders the placebo as a PlaceboResult (keyed by the fake ban year) for
// the dossier's placebo section.
func caPlacebo(units []did.Unit) []schema.PlaceboResult {
	tau, sd := caPlaceboEstimate(units)
	return []schema.PlaceboResult{{
		Cutoff: 2012, Estimate: tau, StdErr: &sd,
		Passed: !math.IsNaN(sd) && sd > 0 && math.Abs(tau) <= 1.96*sd,
	}}
}

// didWindowSweep maps the DiD window sweep to schema SweepPoints.
func didWindowSweep(res did.Result) []schema.SweepPoint {
	out := make([]schema.SweepPoint, 0, len(res.Windows))
	for _, w := range res.Windows {
		se := w.SE
		out = append(out, schema.SweepPoint{Param: w.Window, Estimate: w.Tau, StdErr: &se})
	}
	return out
}

// caMentholEpisodeRows builds one episode row per (province × year).
func caMentholEpisodeRows(obs []ingest.SmokingObs) [][]string {
	rows := append([]ingest.SmokingObs(nil), obs...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Province != rows[j].Province {
			return rows[i].Province < rows[j].Province
		}
		return rows[i].Year < rows[j].Year
	})
	out := make([][]string, 0, len(rows))
	for _, o := range rows {
		banYr, treated := caBanYear[o.Province]
		isPost := treated && o.Year >= banYr && o.Year != 2015 && o.Year < 2018
		out = append(out, []string{
			o.Province,
			caProvinceName[o.Province],
			strconv.Itoa(o.Year),
			strconv.FormatBool(treated),
			emptyIfZero(banYr),
			strconv.FormatBool(isPost),
			f(o.Pct),
			f64OrNaNEmpty(o.Lo),
			f64OrNaNEmpty(o.Hi),
		})
	}
	return out
}

// caMentholPanelSample returns a small excerpt: each province's 2014 (last pre) and
// 2016 (first post) smoking values, for human audit.
func caMentholPanelSample(obs []ingest.SmokingObs) []schema.PanelObservation {
	var out []schema.PanelObservation
	for _, o := range obs {
		if o.Year != 2014 && o.Year != 2016 {
			continue
		}
		_, treated := caBanYear[o.Province]
		pct := o.Pct
		dt := float64(o.Year - 2015)
		out = append(out, schema.PanelObservation{
			SeriesID:                 o.Province,
			SeriesName:               caProvinceName[o.Province],
			IsControl:                !treated,
			Period:                   strconv.Itoa(o.Year),
			PeriodsSinceIntervention: dt,
			IsPost:                   o.Year >= 2016,
			Outcome:                  &pct,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SeriesID != out[j].SeriesID {
			return out[i].SeriesID < out[j].SeriesID
		}
		return out[i].Period < out[j].Period
	})
	return out
}

// gaussianQuantiles returns the schema quantiles of N(mean, sd^2) at the grid ps.
func gaussianQuantiles(mean, sd float64, ps []float64) []schema.Quantile {
	out := make([]schema.Quantile, 0, len(ps))
	for _, p := range ps {
		out = append(out, schema.Quantile{P: p, Value: normalQuantile(p, mean, sd)})
	}
	return out
}

// gaussianSamples returns k deterministic stratified inverse-CDF samples of N(mean, sd^2).
func gaussianSamples(mean, sd float64, k int) []float64 {
	out := make([]float64, k)
	for i := 0; i < k; i++ {
		out[i] = normalQuantile((float64(i)+0.5)/float64(k), mean, sd)
	}
	return out
}

func normalQuantile(p, mean, sd float64) float64 {
	if p <= 0 {
		p = 1e-9
	}
	if p >= 1 {
		p = 1 - 1e-9
	}
	return mean + sd*math.Sqrt2*math.Erfinv(2*p-1)
}

func emptyIfZero(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func f64OrNaNEmpty(v float64) string {
	if math.IsNaN(v) {
		return ""
	}
	return f(v)
}
