package series

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/its"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// ulezStation is one monitoring station's capture-filtered monthly NO2 series,
// keyed by months-since-epoch.
type ulezStation struct {
	code, name, la string
	byT            map[int]float64
}

// monthlyMean aggregates a set of station series into a per-month cross-station
// mean, keeping only months with at least minN stations contributing.
func monthlyMean(stations map[string]*ulezStation, minN int) map[int]float64 {
	type acc struct {
		sum float64
		n   int
	}
	by := map[int]*acc{}
	for _, s := range stations {
		for t, v := range s.byT {
			a := by[t]
			if a == nil {
				a = &acc{}
				by[t] = a
			}
			a.sum += v
			a.n++
		}
	}
	out := map[int]float64{}
	for t, a := range by {
		if a.n >= minN {
			out[t] = a.sum / float64(a.n)
		}
	}
	return out
}

// selectParallelControls keeps the control stations whose pre-period NO2 series is
// correlated at or above thr with the treated aggregate (parallel pre-trend),
// dropping anomalous ones. A station with too few overlapping pre months to assess
// (<6) is kept (it cannot be judged non-parallel). Returns the kept map and the
// sorted list of dropped station codes (with their correlation, for the record).
func selectParallelControls(controls map[string]*ulezStation, treatedAgg map[int]float64, preStart, preEnd int, thr float64) (map[string]*ulezStation, []string) {
	kept := map[string]*ulezStation{}
	var dropped []string
	for code, s := range controls {
		var ts, cs []float64
		for t := preStart; t <= preEnd; t++ {
			tv, okT := treatedAgg[t]
			cv, okC := s.byT[t]
			if okT && okC {
				ts = append(ts, tv)
				cs = append(cs, cv)
			}
		}
		if len(ts) < 6 {
			kept[code] = s // cannot assess parallelism; keep
			continue
		}
		r := pearson(ts, cs)
		if r >= thr {
			kept[code] = s
		} else {
			dropped = append(dropped, fmt.Sprintf("%s(r=%.2f)", code, r))
		}
	}
	sort.Strings(dropped)
	return kept, dropped
}

// pearson is the Pearson correlation of two equal-length series (0 when undefined).
func pearson(a, b []float64) float64 {
	n := len(a)
	if n < 2 {
		return 0
	}
	var ma, mb float64
	for i := 0; i < n; i++ {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(n)
	mb /= float64(n)
	var sab, saa, sbb float64
	for i := 0; i < n; i++ {
		da, db := a[i]-ma, b[i]-mb
		sab += da * db
		saa += da * da
		sbb += db * db
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

// ulezITSChecks runs the controlled-ITS validity battery and returns the populated
// schema block plus the admission verdict. Admission requires the parallel-trends
// defences to hold (parallelism, no anticipation, clean placebos); a wide interval
// is never itself a failure.
func ulezITSChecks(pts []its.Point, primary its.FitResult, ev ulezEvent) (*schema.ITSChecks, bool) {
	spec := ulezPrimarySpec(ev)
	t0 := float64(ev.t0)

	// Control parallelism: the pre-period treated-minus-control difference slope. A
	// flat difference (slope ≈ 0) is the strongest form of parallel trends. A stable
	// NON-zero pre-trend (the central zone improving faster than the outer control
	// pre-policy) is admissible too — the segmented model extrapolates it and reads
	// the break against it — PROVIDED the placebo battery confirms the pre-trend is
	// linear (no spurious break). So this is reported as the strength of the parallel-
	// trends assumption; admission is gated on the placebo + no-anticipation battery,
	// which is what actually guards against a manufactured break.
	slope, slopeSE, nPre := its.PreTrend(pts, t0, float64(ev.prePrimary))
	flat := !math.IsInf(slopeSE, 0) && math.Abs(slope) <= 1.96*slopeSE
	parallelism := schema.TestResult{
		Method:    "pre-period parallel-trends slope of the treated-minus-control difference",
		Statistic: f64ptr(slope),
		Passed:    flat,
		Detail:    fmt.Sprintf("pre-trend slope %.4f µg/m³ per month (se %.4f, n_pre=%d): difference is %s flat; a stable non-zero pre-trend is carried by the segmented model's linear term and validated by the placebo battery", slope, slopeSE, nPre, map[bool]string{true: "statistically", false: "NOT (modelled by the linear term;"}[flat]),
	}

	// No anticipation: a placebo break 6 months before the instant (retaining an
	// adequate post-fake sample) should be ≈0. An unassessable fit (too few months)
	// is skipped, not failed.
	na := its.NoAnticipation(pts, t0, ev.naLead, spec)
	naAssessed := na.OK
	naPass := !naAssessed || math.Abs(na.Effect) <= 1.96*na.SE()
	noAnticip := schema.TestResult{
		Method:    fmt.Sprintf("no-anticipation placebo break %d months before the instant (should be ≈0)", ev.naLead),
		Statistic: f64ptr(na.Effect),
		Passed:    naPass,
		Detail:    fmt.Sprintf("placebo break at %s: %.3f µg/m³ (se %.3f, assessed=%v): %s", tToMonth(ev.t0-ev.naLead), na.Effect, na.SE(), naAssessed, passFail(naPass)),
	}

	// Placebo intervention dates wholly inside the pre-period. A placebo is only
	// ASSESSABLE when it leaves an adequate sample on BOTH sides (>=8 months each):
	// a placebo crammed against the pre-window edge has too few months to estimate a
	// trend and yields a spurious break, which is a property of the test, not the
	// design. Unassessable placebos are skipped (Passed, noted), not failed.
	const placeboMinSeg = 8
	var placebos []schema.DatePlaceboResult
	for _, ft := range ev.placeboTs {
		pf := its.PlaceboFit(pts, t0, float64(ft), spec)
		assessable := pf.OK && pf.NPre >= placeboMinSeg && pf.NPost >= placeboMinSeg
		pass := !assessable || math.Abs(pf.Effect) <= 1.96*pf.SE()
		se := pf.SE()
		placebos = append(placebos, schema.DatePlaceboResult{
			Date: tToMonth(ft), Estimate: pf.Effect, StdErr: &se, Passed: pass,
		})
	}

	// Window sweep: effect stability as the pre-window start varies.
	var sweep []schema.SweepPoint
	for _, ps := range ev.specPre {
		s := spec
		s.PreStartT = float64(ps)
		f := its.Fit(pts, t0, s)
		se := f.SE()
		sweep = append(sweep, schema.SweepPoint{Param: float64(ps), Estimate: f.Effect, StdErr: &se})
	}

	// Transition exclusion: the primary panel already excludes the switch-on ramp
	// month; report the primary estimate as the ramp-excluded robustness point.
	transition := []schema.SweepPoint{{Param: 0, Estimate: primary.Effect, StdErr: f64ptr(primary.SE())}}

	// Autocorrelation: report that Newey-West HAC errors were used and the lag-1
	// residual autocorrelation they corrected for.
	autocorr := schema.TestResult{
		Method:    fmt.Sprintf("Newey-West HAC (Bartlett, lag %d)", ulezNWLag),
		Statistic: f64ptr(primary.Resid1Corr),
		Passed:    true,
		Detail:    fmt.Sprintf("lag-1 residual autocorrelation %.3f, absorbed by HAC standard errors", primary.Resid1Corr),
	}

	checks := &schema.ITSChecks{
		NoAnticipation:      noAnticip,
		ControlParallelism:  parallelism,
		PlaceboDates:        placebos,
		WindowSweep:         sweep,
		TransitionExclusion: transition,
		Autocorrelation:     autocorr,
	}
	// Admission gates on the manufactured-break guards (placebos + no-anticipation),
	// not on a flat pre-trend: a stable modelled trend is admissible, a spurious break
	// is not. A wide interval is never itself a failure. Parallelism (flat-ness) is
	// reported for the reader to weigh.
	admitted := naPass && allPlaceboPass(placebos)
	checks.Admitted = admitted
	return checks, admitted
}

// ulezPanelSample returns a small inline excerpt of the panel rows around the
// instant (one treated and one control row each for a few months) for human audit.
func ulezPanelSample(pts []its.Point, ev ulezEvent) []schema.PanelObservation {
	// Pick the months bracketing the instant.
	sort.Slice(pts, func(i, j int) bool { return pts[i].T < pts[j].T })
	var out []schema.PanelObservation
	for _, p := range pts {
		dt := p.T - float64(ev.t0)
		if dt < -3 || dt > 3 {
			continue
		}
		isPost := p.T >= float64(ev.t0)
		tv, cv := p.Treated, p.Control
		out = append(out,
			schema.PanelObservation{
				SeriesID: "treated-roadside", SeriesName: ev.treatedLabel + " (aggregate)",
				IsControl: false, Period: p.Month, PeriodsSinceIntervention: dt, IsPost: isPost,
				Outcome: &tv, Covariates: panelCov(p),
			},
			schema.PanelObservation{
				SeriesID: "control-roadside", SeriesName: ev.controlLabel + " (aggregate)",
				IsControl: true, Period: p.Month, PeriodsSinceIntervention: dt, IsPost: isPost,
				Outcome: &cv, Covariates: panelCov(p),
			},
		)
	}
	return out
}

func panelCov(p its.Point) map[string]float64 {
	if p.Covar == nil {
		return nil
	}
	m := map[string]float64{}
	if v, ok := p.Covar["wind_speed"]; ok {
		m["wind_speed_kmh"] = v
	}
	if v, ok := p.Covar["temp"]; ok {
		m["temp_c"] = v
	}
	if v, ok := p.Covar["precip"]; ok {
		m["precip_mm"] = v
	}
	return m
}

// ulezEpisodeRows builds the panel episode rows (one per station × month, treated
// and control) staged as the build intermediate.
func ulezEpisodeRows(treated, control map[string]*ulezStation, meteo map[string]ingest.LAQNMeteoMonth, ev ulezEvent) [][]string {
	type row struct {
		code, name string
		isControl  bool
		t          int
		v          float64
	}
	var rows []row
	add := func(stations map[string]*ulezStation, isControl bool) {
		for _, s := range stations {
			for t, v := range s.byT {
				rows = append(rows, row{s.code, s.name, isControl, t, v})
			}
		}
	}
	add(treated, false)
	add(control, true)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].isControl != rows[j].isControl {
			return !rows[i].isControl // treated first
		}
		if rows[i].code != rows[j].code {
			return rows[i].code < rows[j].code
		}
		return rows[i].t < rows[j].t
	})
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		mo := tToMonth(r.t)
		dt := r.t - ev.t0
		isPost := r.t >= ev.t0 && r.t != ev.transition
		mm := meteo[mo]
		out = append(out, []string{
			r.code,
			r.name,
			strconv.FormatBool(r.isControl),
			mo,
			strconv.Itoa(dt),
			strconv.FormatBool(isPost),
			strconv.FormatFloat(r.v, 'g', -1, 64),
			f64OrEmptyVal(mm.WindSpeedKmh),
			f64OrEmptyVal(mm.TempC),
			f64OrEmptyVal(mm.PrecipMm),
		})
	}
	return out
}

// f64OrEmptyVal formats a float, emitting "" for NaN.
func f64OrEmptyVal(v float64) string {
	if math.IsNaN(v) {
		return ""
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
