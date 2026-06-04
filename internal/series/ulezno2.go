package series

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/its"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// The ulez-no2 series is the controlled-interrupted-time-series seam on the
// "emission-zone stringency -> roadside NO2" mechanism: each London ULEZ expansion
// is one anchor. The treated series is monthly-mean roadside NO2 inside the
// newly-covered zone; the control series is roadside NO2 that was NOT covered at
// that instant (a clean same-road-type counterfactual). The effect is the break in
// the treated-minus-control difference accumulated over the post window — the extra
// fall in the roadside increment attributable to the zone, net of the shared
// meteorology and regional trend the control absorbs.
//
// The delivered anchor is the 2019 CENTRAL expansion (8 Apr 2019), which produced a
// large, sharp, well-documented roadside-NO2 drop with a clean outer-London control
// (untreated until 2023). The 2023 London-wide expansion is a sibling event whose
// marginal effect is near-zero (the outer fleet was already ~95% compliant) and
// which does not cleanly identify under this design — it is reported, not admitted.

const (
	ulezMechanismID = "emission-zone-stringency-to-roadside-no2"

	// Running-time epoch is 2021-01 (month index 0); months before it are negative.
	// A station-month needs this many valid hours to enter (≈14 days).
	ulezCaptureMinHours = 336
	ulezMinTreated      = 3
	ulezMinControl      = 3

	// A control station is dropped from the matched set only if its pre-period NO2
	// series is ANTI-correlated with the treated aggregate (a negative pre-trend
	// correlation flags a broken/anomalous station). Per-station level correlation is
	// noisy — the robust control is the AGGREGATE of the kept outer-roadside stations,
	// whose parallel-trend credibility is established by the placebo battery, not by a
	// high per-station correlation. So this is a minimal anomaly guard, not a tight
	// match filter.
	ulezCorrThreshold = 0.0

	ulezNWLag = 4
)

// ulezEvent parametrises one emission-zone anchor: the frozen inputs, the
// intervention instant on the running-time axis, the pre/post sub-windows and the
// specification/placebo grid (all in months-since-2021-01), and the prose that
// distinguishes the zone. monthToT/tToMonth handle the negative indices of pre-2021
// events (the 2019 expansion).
type ulezEvent struct {
	markID        string
	series        schema.Series
	mechanismID   string
	instant       string // ISO date of the switch-on
	no2SourceID   string
	meteoSourceID string

	t0            int  // first full post month
	transition    int  // the switch-on month, excluded as the implementation ramp
	hasTransition bool // false when the switch-on has no partial ramp month
	prePrimary    int  // primary pre-window start
	preEnd        int  // last pre month
	postEnd       int  // last post month to include (0 = use last available; caps COVID)
	minTreated    int  // min treated stations per month (0 → default 3)
	minControl    int  // min control stations per month (0 → default 3)
	specPre       []int
	specSlope     []bool // post-slope-change terms in the grid (drop for short post windows)
	placeboTs     []int
	naLead        int

	harvestWindow  string
	action         string
	alternative    string
	estimand       string
	contextDesc    string
	treatedLabel   string
	controlLabel   string
	controlRole    string
	controlJustif  string
	regimeCaveat   string
	decisionRound  string
	contextAsOf    string
}

// ulez2019Event is the delivered anchor: the 8 Apr 2019 central-London ULEZ. April
// 2019 is the ramp month; May 2019 is the first full post month; the post window
// stops at Feb 2020 to keep the COVID traffic collapse (from late Mar 2020) out.
func ulez2019Event() ulezEvent {
	return ulezEvent{
		markID:        "ulez-no2-2019",
		series:        schema.SeriesULEZNO2,
		mechanismID:   ulezMechanismID,
		instant:       "2019-04-08",
		no2SourceID:   "ulez-no2-laqn-2019",
		meteoSourceID: "ulez-no2-meteo-2019",
		t0:            -20, // 2019-05
		transition:    -21, // 2019-04 (switch-on month)
		hasTransition: true,
		prePrimary:    -45, // 2017-04
		preEnd:        -22, // 2019-03
		postEnd:       -11, // 2020-02 (pre-COVID cap)
		// Only 10 post months: a post-slope change is not reliably identified, and a
		// 12-month pre-window can't support trend + 2 harmonics — so the grid uses
		// level-break specs over the two adequate pre-windows (24 and 18 months).
		specPre:       []int{-45, -39}, // 2017-04, 2017-10
		specSlope:     []bool{false},
		// Placebos span the pre-period at points leaving >=8 months either side
		// (degenerate edge dates are skipped by the adequacy guard).
		placeboTs:     []int{-37, -33, -29}, // 2017-12, 2018-04, 2018-08
		naLead:        4,
		harvestWindow: "2017-01..2020-06",
		action:        "Central-London ULEZ switch-on (8 Apr 2019): the Ultra Low Emission Zone replaced the T-Charge across the Congestion Charge Zone, charging non-compliant (pre-Euro-4 petrol / pre-Euro-6 diesel) vehicles 24/7 to enter central London.",
		alternative:   "Roads outside the central zone: no ULEZ charge, no emission-standard requirement (outer-London roads, not covered until the 2023 expansion).",
		estimand:      "Population effect over the post-intervention window (2019-05 to 2020-02, pre-COVID): the change in central-London roadside NO2, net of an outer-London roadside control series sharing the pre-intervention trend.",
		contextDesc:   "London Air Quality Network roadside/kerbside stations, split into central congestion-charge-zone stations (treated by the 2019 ULEZ) and outer-London stations (control, not covered until 2023), with a central-London meteorology join.",
		treatedLabel:  "central congestion-charge-zone roadside/kerbside stations covered by the 2019 ULEZ",
		controlLabel:  "outer-London roadside/kerbside stations not covered until the 2023 expansion",
		controlRole:   "parallel-trend",
		controlJustif: "Outer-London roadside stations are the same road-type as the treated central stations and share their pre-intervention trend (selected by pre-period correlation), but were NOT in any ULEZ until the 2023 expansion — a clean never-treated-in-period control for the central 2019 switch-on.",
		regimeCaveat:  "Pinned to the 2019 central-London ULEZ (Euro-4 petrol / Euro-6 diesel standard, 24/7 daily charge across the Congestion Charge Zone). The 2021 inner and 2023 London-wide expansions are sibling anchors on the same mechanism at wider coverage.",
		decisionRound: "Central-London ULEZ switch-on, 8 April 2019",
		contextAsOf:   "2019-04-07",
	}
}

// ulez2023Event is the 29 Aug 2023 London-wide expansion. Sept 2023 is the first
// full post month; Aug 2023 is the ramp. Reported but typically not admitted.
func ulez2023Event() ulezEvent {
	return ulezEvent{
		markID:        "ulez-no2-2023",
		series:        schema.SeriesULEZNO2,
		mechanismID:   ulezMechanismID,
		instant:       "2023-08-29",
		no2SourceID:   "ulez-no2-laqn",
		meteoSourceID: "ulez-no2-meteo",
		t0:            32, // 2023-09
		transition:    31, // 2023-08
		hasTransition: true,
		prePrimary:    12, // 2022-01
		preEnd:        30, // 2023-07
		postEnd:       0,  // use last available
		specPre:       []int{8, 12, 15}, // 2021-09, 2022-01, 2022-04
		specSlope:     []bool{false, true},
		placeboTs:     []int{16, 20, 24}, // 2022-05, 2022-09, 2023-01
		naLead:        6,
		harvestWindow: "2021-09..2026-04",
		action:        "London-wide ULEZ expansion (29 Aug 2023): the Ultra Low Emission Zone extended to the GLA boundary, bringing outer-London roads into the daily-charge zone for non-compliant (pre-Euro-4 petrol / pre-Euro-6 diesel) vehicles.",
		alternative:   "Roads remain outside the ULEZ: no daily charge, no emission-standard requirement.",
		estimand:      "Population effect over the post-intervention window (2023-09 onward): the change in roadside NO2 in the newly-covered outer-London zone, net of an urban-background control series sharing the pre-intervention trend.",
		contextDesc:   "London Air Quality Network monitoring stations, split into outer-London roadside/kerbside (treated by the 2023 London-wide expansion) and urban-background/suburban (control), with a central-London meteorology join.",
		treatedLabel:  "outer-London roadside/kerbside stations newly covered by the 29 Aug 2023 London-wide ULEZ expansion",
		controlLabel:  "urban-background/suburban stations",
		controlRole:   "parallel-trend",
		controlJustif: "Urban-background/suburban stations share the treated stations' airshed and pre-intervention trend (selected by pre-period correlation) but receive far weaker DIRECT kerbside-traffic treatment, so their series is the counterfactual for the regional/meteorological component of roadside NO2.",
		regimeCaveat:  "Pinned to the 2023 London-wide expansion (Euro-4 petrol / Euro-6 diesel standard, daily charge for non-compliant vehicles, with the 2023 scrappage scheme co-running); the 2019/2021 expansions are sibling anchors on the same mechanism.",
		decisionRound: "London-wide ULEZ expansion, 29 August 2023",
		contextAsOf:   "2023-08-28",
	}
}

// berlinLEZEvent is the Berlin Umweltzone stage-2 anchor (1 Jan 2010, the green-
// sticker / Euro-4-diesel standard-BAN tightening). Pre 2008-2009 starts AFTER
// stage 1 (Jan 2008) so that prior step is excluded; post 2010-2011 is pre-COVID.
// The treated network is thin (only two in-zone traffic NO2 stations survive in the
// EEA historical archive) — recorded as a limitation; it widens the honest interval.
func berlinLEZEvent() ulezEvent {
	return ulezEvent{
		markID:        "berlin-lez-no2-2010",
		series:        schema.SeriesBerlinLEZ,
		mechanismID:   "lez-ban-stringency-to-roadside-no2",
		instant:       "2010-01-01",
		no2SourceID:   "berlin-lez-no2",
		meteoSourceID: "berlin-lez-meteo",
		t0:            -132, // 2010-01 (first full post month; the ban took effect on the 1st)
		hasTransition: false,
		minTreated:    2, // only two in-zone traffic NO2 stations exist historically
		minControl:    3,
		prePrimary:    -156, // 2008-01
		preEnd:        -133, // 2009-12
		postEnd:       -108, // 2011-12
		specPre:       []int{-156, -150}, // 2008-01, 2008-07
		specSlope:     []bool{false, true},
		placeboTs:     []int{-148, -144, -140}, // 2008-09, 2009-01, 2009-05
		naLead:        6,
		harvestWindow: "2008-01..2011-12",
		action:        "Berlin Umweltzone stage 2 (1 Jan 2010): a green emissions sticker (Euro-4 diesel / Euro-1 petrol) became mandatory to drive inside the S-Bahn Ring — older non-compliant vehicles banned (not charged).",
		alternative:   "Roads outside the Umweltzone (outside the S-Bahn Ring): no emissions-sticker requirement.",
		estimand:      "Population effect over the post-intervention window (2010-01 to 2011-12, pre-COVID): the change in in-zone roadside NO2, net of an in-zone urban-background control series sharing the pre-intervention trend.",
		contextDesc:   "Berlin BLUME air-quality stations inside the Umweltzone (S-Bahn Ring), split into traffic (treated by the 2010 stage-2 ban) and urban-background (control, far weaker direct kerbside treatment), with a Berlin meteorology join. Data from the EEA historical (AirBase) archive.",
		treatedLabel:  "in-zone (S-Bahn Ring) traffic stations subject to the 2010 Umweltzone stage-2 ban",
		controlLabel:  "in-zone urban-background stations",
		controlRole:   "parallel-trend",
		controlJustif: "In-zone urban-background stations share the treated traffic stations' airshed and pre-intervention trend but receive far weaker DIRECT kerbside-traffic treatment, so their series is the counterfactual for the regional/meteorological component of in-zone NO2.",
		regimeCaveat:  "Pinned to the Berlin Umweltzone stage 2 (green-sticker Euro-4-diesel BAN inside the S-Bahn Ring). This is a standard-BAN low-emission zone — kept SEPARATE from charge-type zones (London ULEZ) for any future bridge. Germany's 2009 Umweltprämie scrappage scheme falls in the pre-window but is national, so the controlled difference nets it out.",
		decisionRound: "Berlin Umweltzone stage 2, 1 January 2010",
		contextAsOf:   "2009-12-31",
	}
}

var ulezEpisodeColumns = []string{
	"series_id", "series_name", "is_control", "period", "periods_since_intervention",
	"is_post", "outcome", "wind_speed_kmh", "temp_c", "precip_mm",
}

// monthToT maps "YYYY-MM" to months since the 2021-01 epoch (Jan 2021 = 0); pre-2021
// months are negative.
func monthToT(m string) (int, bool) {
	if len(m) != 7 || m[4] != '-' {
		return 0, false
	}
	y, err1 := strconv.Atoi(m[:4])
	mo, err2 := strconv.Atoi(m[5:7])
	if err1 != nil || err2 != nil || mo < 1 || mo > 12 {
		return 0, false
	}
	return (y-2021)*12 + (mo - 1), true
}

// tToMonth is the inverse of monthToT, correct for negative indices.
func tToMonth(t int) string {
	y := 2021 + t/12
	mo := t%12 + 1
	if t < 0 && t%12 != 0 {
		y = 2021 + (t-11)/12
		mo = ((t%12)+12)%12 + 1
	}
	return fmt.Sprintf("%04d-%02d", y, mo)
}

// ulezSpecs is the controlled-ITS specification grid the honest interval averages
// over: pre-window start × seasonal harmonics × level-only-vs-level+slope ×
// meteorology-adjusted-vs-unadjusted. The between-spec spread is the identification
// uncertainty folded into the mark's width (the ITS analogue of bandwidth×order×
// kernel for the RDD marks).
func ulezSpecs(ev ulezEvent) []its.Spec {
	var specs []its.Spec
	for _, ps := range ev.specPre {
		for _, h := range []int{1, 2} {
			for _, slope := range ev.specSlope {
				for _, meteo := range [][]string{nil, {"wind_speed", "temp"}} {
					name := fmt.Sprintf("pre%s/h%d/%s/%s", tToMonth(ps), h,
						map[bool]string{true: "level+slope", false: "level"}[slope],
						map[bool]string{true: "meteo-adj", false: "unadj"}[len(meteo) > 0])
					specs = append(specs, its.Spec{
						Name: name, PreStartT: float64(ps), Harmonics: h, Slope: slope,
						Meteo: meteo, NWLag: ulezNWLag,
					})
				}
			}
		}
	}
	return specs
}

// ulezPrimarySpec is the single specification used for the diagnostics and the
// plug-in comparison — a parsimonious level-break model with one seasonal harmonic
// on the difference series, no meteorology adjustment (the control already absorbs
// the shared meteorology).
func ulezPrimarySpec(ev ulezEvent) its.Spec {
	return its.Spec{Name: "primary", PreStartT: float64(ev.prePrimary), Harmonics: 1, Slope: false, NWLag: ulezNWLag}
}

// BuildULEZNO2 mints the delivered ULEZ→roadside-NO2 anchor (the 2019 central
// expansion). The 2023 London-wide expansion is available via BuildULEZNO22023.
func BuildULEZNO2(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	return buildULEZ(rawDir, cacheDir, distDir, cfg, ulez2019Event())
}

// BuildULEZNO22023 mints the 2023 London-wide expansion sibling (reported; usually
// not admitted because the marginal effect is near-zero and the design does not
// cleanly identify it).
func BuildULEZNO22023(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	return buildULEZ(rawDir, cacheDir, distDir, cfg, ulez2023Event())
}

// BuildBerlinLEZ mints the Berlin Umweltzone stage-2 (2010) controlled-ITS mark — a
// standard-BAN low-emission zone, the same ITS machinery as the ULEZ events.
func BuildBerlinLEZ(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	return buildULEZ(rawDir, cacheDir, distDir, cfg, berlinLEZEvent())
}

// buildULEZ is the event-parametrised controlled-ITS builder.
func buildULEZ(rawDir, cacheDir, distDir string, cfg publish.Config, ev ulezEvent) (schema.Mark, error) {
	var zero schema.Mark

	no2Src, err := ingest.LoadSource(filepath.Join(rawDir, ev.no2SourceID, "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	meteoSrc, err := ingest.LoadSource(filepath.Join(rawDir, ev.meteoSourceID, "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	no2Path := no2Src.CachePath(cacheDir)
	meteoPath := meteoSrc.CachePath(cacheDir)
	if err := ingest.VerifySHA(no2Path, no2Src.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `python3 scripts/ulez_harvest.py --event %s`)", err, eventTag(ev))
	}
	if err := ingest.VerifySHA(meteoPath, meteoSrc.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `python3 scripts/ulez_harvest.py --event %s`)", err, eventTag(ev))
	}

	panel, err := ingest.LoadLAQNNO2(no2Path)
	if err != nil {
		return zero, err
	}
	meteo, err := ingest.LoadLAQNMeteo(meteoPath)
	if err != nil {
		return zero, err
	}

	// Index station-months by (group, station, T), capture-filtered.
	treatedStations := map[string]*ulezStation{}
	controlStations := map[string]*ulezStation{}
	for _, r := range panel {
		if r.NHours < ulezCaptureMinHours {
			continue
		}
		t, ok := monthToT(r.Month)
		if !ok {
			continue
		}
		dst := controlStations
		if r.Group == "treated" {
			dst = treatedStations
		} else if r.Group != "control" {
			continue
		}
		ss := dst[r.SiteCode]
		if ss == nil {
			ss = &ulezStation{code: r.SiteCode, name: r.SiteName, la: r.LAName, byT: map[int]float64{}}
			dst[r.SiteCode] = ss
		}
		ss.byT[t] = r.NO2Mean
	}

	minTreated := pickInt(ev.minTreated, ulezMinTreated)
	minControl := pickInt(ev.minControl, ulezMinControl)
	treatedAgg := monthlyMean(treatedStations, minTreated)
	keptControls, droppedControls := selectParallelControls(controlStations, treatedAgg, ev.prePrimary, ev.preEnd, ulezCorrThreshold)
	controlAgg := monthlyMean(keptControls, minControl)

	// Assemble the analysis panel: months with both aggregates present, excluding the
	// transition month and anything past the post-window cap (keeps COVID out).
	var pts []its.Point
	var months []int
	for t := range treatedAgg {
		if _, ok := controlAgg[t]; ok {
			months = append(months, t)
		}
	}
	sort.Ints(months)
	for _, t := range months {
		if ev.hasTransition && t == ev.transition {
			continue
		}
		if ev.postEnd != 0 && t > ev.postEnd {
			continue
		}
		mo := tToMonth(t)
		cov := map[string]float64{}
		if mm, ok := meteo[mo]; ok {
			cov["wind_speed"] = mm.WindSpeedKmh
			cov["temp"] = mm.TempC
			cov["precip"] = mm.PrecipMm
		}
		pts = append(pts, its.Point{T: float64(t), Month: mo, Treated: treatedAgg[t], Control: controlAgg[t], Covar: cov})
	}
	if len(pts) < 20 {
		return zero, fmt.Errorf("%s: too few usable months (%d) to estimate", ev.markID, len(pts))
	}
	lastT := pts[len(pts)-1].T
	lastMonth := tToMonth(int(lastT))

	// Stage the panel episode rows (build intermediate; reshaped at export).
	if _, err := publish.WriteEpisodesCSVGz(distDir, ev.markID, "episodes.csv.gz", ulezEpisodeColumns,
		ulezEpisodeRows(treatedStations, keptControls, meteo, ev)); err != nil {
		return zero, err
	}

	// --- honest model-averaged effect over the specification grid.
	bma := its.EstimateBMA(pts, float64(ev.t0), ulezSpecs(ev))
	if len(bma.Specs) == 0 {
		return zero, fmt.Errorf("%s: no specification produced a usable fit", ev.markID)
	}
	lo, hi := bma.Interval(0.95)
	effect := schema.Distribution{
		Central:           bma.Central,
		StdDev:            f64ptr(bma.TotalSD),
		Interval:          &schema.Interval{Level: 0.95, Lower: lo, Upper: hi},
		Quantiles:         itsQuantiles(bma, posteriorQuantileGrid),
		Samples:           bma.Samples(postSamples),
		UncertaintyBudget: &schema.UncertaintyBudget{Sampling: f64ptr(bma.WithinVar), Specification: f64ptr(bma.BetweenVar)},
	}

	// Plug-in (single-spec) estimate, kept as the documented sampling-led comparison.
	primary := its.Fit(pts, float64(ev.t0), ulezPrimarySpec(ev))
	plugLo, plugHi := primary.Effect-1.96*primary.SE(), primary.Effect+1.96*primary.SE()

	// --- ITS validity battery.
	checks, admitted := ulezITSChecks(pts, primary, ev)

	// Meteorology-adjusted vs unadjusted (the load-bearing-join evidence).
	adjSpec := ulezPrimarySpec(ev)
	adjSpec.Meteo = []string{"wind_speed", "temp"}
	adj := its.Fit(pts, float64(ev.t0), adjSpec)

	notes := fmt.Sprintf(
		"Controlled ITS: model average over %d specs (pre-window × harmonics × level/slope × meteorology) on the treated-minus-control monthly NO2 difference, Newey-West HAC (Bartlett lag %d) within each. "+
			"Honest 95%% interval [%.3f, %.3f] (sd %.3f) decomposes into sampling sd %.3f and identification sd %.3f. "+
			"Plug-in single-spec interval [%.3f, %.3f] (sd %.3f) is narrower because it ignores between-spec identification uncertainty. "+
			"Treated = %d %s; control = %d %s kept after pre-trend matching (%d dropped: %v). "+
			"Effect is the extra change in treated roadside NO2 net of the control series over %d post months (%s..%s). "+
			"Meteorology-adjusted refit shifts the effect to %.3f (Δ%.3f), confirming the control series already absorbs most short-window dispersion. "+
			"Checks: control parallelism %s, no-anticipation %s, placebo dates %s. REGIME CAVEAT: %s",
		len(bma.Specs), ulezNWLag,
		lo, hi, bma.TotalSD, math.Sqrt(bma.WithinVar), math.Sqrt(bma.BetweenVar),
		plugLo, plugHi, primary.SE(),
		len(treatedStations), ev.treatedLabel, len(keptControls), ev.controlLabel, len(droppedControls), droppedControls,
		primary.NPost, tToMonth(ev.t0), lastMonth,
		adj.Effect, adj.Effect-primary.Effect,
		passFail(checks.ControlParallelism.Passed), passFail(checks.NoAnticipation.Passed), passFail(allPlaceboPass(checks.PlaceboDates)),
		ev.regimeCaveat)

	checks.Notes = notes
	dossier := schema.ValidityDossier{ITS: checks, Admitted: admitted, Notes: notes}

	mark := schema.Mark{
		SchemaVersion:  schema.SchemaVersion,
		MechanismID:    ev.mechanismID,
		Category:       schema.CategoryIdentified,
		TruthSource:    schema.TruthIdentified,
		ID:             ev.markID,
		Series:         ev.series,
		Domain:         "Environment",
		UnitType:       "air-quality-monitoring-station",
		Identification: schema.IDITSControlled,
		RowShape:       schema.RowPanel,
		Design: schema.Design{
			RunningVariable: schema.Variable{
				Name: "month", Description: "Calendar month on the running-time axis (epoch 2021-01)",
				Units: "month", SourceID: no2Src.SourceID,
			},
			Action:      ev.action,
			Alternative: ev.alternative,
			Outcome: schema.Variable{
				Name: "no2_concentration", Description: "Monthly-mean NO2 concentration at the monitoring station",
				Units: "µg/m³", SourceID: no2Src.SourceID,
			},
			Estimand: ev.estimand,
			ITS: &schema.ITSDesign{
				InterventionInstant: ev.instant,
				RunningTime:         schema.Variable{Name: "month", Description: "Months since 2021-01", Units: "month", SourceID: no2Src.SourceID},
				PreWindow:           schema.Window{Start: tToMonth(ev.prePrimary), End: tToMonth(ev.preEnd)},
				PostWindow:          schema.Window{Start: tToMonth(ev.t0), End: lastMonth},
				Transition:          transitionWindow(ev),
				Counterfactual: schema.Counterfactual{
					Family:        "segmented-regression on the treated-minus-control difference",
					Terms:         []string{"intercept", "linear-time", "post-level", "post-slope", "seasonal-harmonics", "meteorology(optional)"},
					Seasonality:   "harmonic terms at the 12-month period (1–2 pairs across specs)",
					Justification: "The difference of treated and control roadside NO2 nets out the shared regional/meteorological trend and most seasonality; a segmented regression on that difference reads the policy break (level + optional slope) against the extrapolated pre-trend, with residual seasonality carried by harmonic terms and serial correlation handled by Newey-West HAC errors.",
				},
				Control: &schema.ControlSeries{
					SeriesID:      no2Src.SourceID,
					Role:          ev.controlRole,
					Justification: ev.controlJustif,
				},
			},
		},
		Context: schema.Context{
			Description:    ev.contextDesc,
			CovariateNames: []string{"wind_speed_kmh", "temp_c", "precip_mm"},
			Population:     fmt.Sprintf("%d treated + %d control monitoring stations over %d analysis months", len(treatedStations), len(keptControls), len(pts)),
		},
		PanelSample: ulezPanelSample(pts, ev),
		Effect:      effect,
		Dossier:     dossier,
		Provenance: schema.Provenance{
			Sources:                []schema.Source{toSchemaSource(no2Src), toSchemaSource(meteoSrc)},
			ContextAsOf:            ev.contextAsOf,
			DecisionTimestamp:      ev.instant,
			OutcomeTimestamp:       lastMonth + "-28",
			RunningVariableVintage: "LAQN ratified hourly NO2 aggregated to monthly means, harvest window " + ev.harvestWindow,
			DecisionRound:          ev.decisionRound,
			ToolVersions: map[string]string{
				"go":                 runtime.Version(),
				"openaction2outcome": schema.SchemaVersion,
				"its":                fmt.Sprintf("controlled-segmented-regression,newey-west-lag=%d,specs=%d", ulezNWLag, len(bma.Specs)),
			},
			OutcomeRealized: true,
		},
	}
	if err := mark.Validate(); err != nil {
		return zero, fmt.Errorf("minted mark failed validation: %w", err)
	}
	return mark, nil
}

// eventTag maps an event to its harvest --event flag (for the re-harvest hint).
func eventTag(ev ulezEvent) string {
	if ev.markID == "ulez-no2-2019" {
		return "2019"
	}
	return "2023"
}

// passFail renders a boolean check verdict as "pass"/"FAIL".
func passFail(ok bool) string {
	if ok {
		return "pass"
	}
	return "FAIL"
}

// pickInt returns v if non-zero, else def.
func pickInt(v, def int) int {
	if v != 0 {
		return v
	}
	return def
}

// transitionWindow returns the implementation-ramp window to record, or nil when the
// switch-on had no partial month (a ban that took effect on the 1st of a month).
func transitionWindow(ev ulezEvent) *schema.Window {
	if !ev.hasTransition {
		return nil
	}
	return &schema.Window{Start: tToMonth(ev.transition), End: tToMonth(ev.transition)}
}

// itsQuantiles maps an ITS BMA result's quantiles onto the schema quantile grid.
func itsQuantiles(bma its.BMAResult, ps []float64) []schema.Quantile {
	out := make([]schema.Quantile, 0, len(ps))
	for _, p := range ps {
		out = append(out, schema.Quantile{P: p, Value: bma.Quantile(p)})
	}
	return out
}

func allPlaceboPass(ps []schema.DatePlaceboResult) bool {
	for _, p := range ps {
		if !p.Passed {
			return false
		}
	}
	return true
}
