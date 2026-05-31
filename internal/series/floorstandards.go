// Package series orchestrates per-series mark minting: it wires the cached frozen
// inputs (ingest) through the RDD estimator (rdd) and the validity battery
// (validity) into a finished schema.Mark, and stages the analysis-ready episode
// table for object-storage publishing (publish). This is the path for
// the education floor-standards series — a sharp RDD on the 2016 Progress 8 floor
// of -0.5, scored against each school's Progress 8 two years later.
package series

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/internal/rdd"
	"github.com/umbralcalc/openaction2outcome/internal/sbi"
	"github.com/umbralcalc/openaction2outcome/internal/validity"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

const (
	markID      = "floor-standards-p8-2016"
	floorCutoff = -0.5
	floorRefBW  = 0.5

	// sampleHalfWidth bounds the inline near-cutoff Sample carried in the mark.
	sampleHalfWidth = 0.05
	sampleMaxRows   = 40

	// SMC settings for the SBI posterior (recorded in provenance for re-mints).
	smcParticles = 4000
	smcRounds    = 8
	smcSeed      = 1

	postSamples = 200 // deterministic posterior samples shipped for CRPS scoring
)

var floorBandwidths = []float64{0.3, 0.4, 0.5, 0.6, 0.7, 0.8}

// posteriorQuantileGrid is the set of probabilities at which the mark ships
// posterior quantiles (for finer calibration / CRPS than the headline interval).
var posteriorQuantileGrid = []float64{0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 0.9, 0.95, 0.975}

var episodeColumns = []string{
	"unit_id", "unit_name", "running_value", "assigned", "treated", "outcome",
	"ks2_prior_attainment", "pct_disadvantaged_fsm", "ks4_cohort_size",
}

// BuildFloorStandards mints the floor-standards mark. rawDir holds the
// SOURCE.json pointers, cacheDir the fetched bytes, distDir receives the staged
// episode sidecar, and cfg supplies the published artifact's URL.
func BuildFloorStandards(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	var zero schema.Mark

	src16, err := ingest.LoadSource(filepath.Join(rawDir, "ks4-2015-2016", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	src18, err := ingest.LoadSource(filepath.Join(rawDir, "ks4-2017-2018", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	path16 := src16.CachePath(cacheDir)
	path18 := src18.CachePath(cacheDir)
	// Integrity: fail loudly if a cached input is missing or has drifted.
	if err := ingest.VerifySHA(path16, src16.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `openaction2outcome fetch`)", err)
	}
	if err := ingest.VerifySHA(path18, src18.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `openaction2outcome fetch`)", err)
	}

	schools16, err := ingest.LoadKS4(path16)
	if err != nil {
		return zero, err
	}
	schools18, err := ingest.LoadKS4(path18)
	if err != nil {
		return zero, err
	}
	outcome := make(map[string]float64, len(schools18))
	for _, s := range schools18 {
		outcome[s.URN] = s.P8 // numeric P8 only (loader filtered)
	}

	// Build episode rows (all units) and the complete-case estimation set.
	episodes := make([]schema.Observation, 0, len(schools16))
	var estPts []rdd.Point
	var ciExcluded int
	for _, s := range schools16 {
		below := s.P8 < floorCutoff
		belowFloor := below && (math.IsNaN(s.P8CIUpp) || s.P8CIUpp < 0)
		if below && !belowFloor {
			ciExcluded++
		}
		o := schema.Observation{
			UnitID:       s.URN,
			UnitName:     s.Name,
			RunningValue: s.P8,
			Assigned:     below,
			Treated:      boolPtr(belowFloor),
			Covariates:   covariates(s),
		}
		if y, ok := outcome[s.URN]; ok {
			yy := y
			o.Outcome = &yy
			estPts = append(estPts, rdd.Point{X: s.P8, Y: y})
		}
		episodes = append(episodes, o)
	}
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].UnitID < episodes[j].UnitID })

	if len(estPts) < 20 {
		return zero, fmt.Errorf("floor-standards: too few complete cases (%d) to estimate", len(estPts))
	}

	// Stage the analysis-ready episode rows as a build intermediate. They are not
	// published per-mark; the export step reshapes every mark's rows into the one
	// `episodes` dataset, joinable on the mark ID.
	if _, err := publish.WriteEpisodesCSVGz(distDir, markID, "episodes.csv.gz", episodeColumns, episodeRows(episodes)); err != nil {
		return zero, err
	}

	// --- estimate the discontinuity.
	// The honest interval is a Bayesian model-averaged SBI posterior over tau (stochadex
	// SMC across a bandwidth x order x kernel grid). Its width splits exactly into
	// within-spec sampling variance and between-spec identification variance.
	bma := sbi.EstimateBMA(estPts, floorCutoff, sbi.DefaultFloorSpecs(), nil, true,
		sbi.SMCConfig{NumParticles: smcParticles, NumRounds: smcRounds, Seed: smcSeed})
	blo, bhi := bma.Interval(0.95)
	effect := schema.Distribution{
		Central:           bma.Central,
		StdDev:            f64ptr(bma.TotalSD),
		Interval:          &schema.Interval{Level: 0.95, Lower: blo, Upper: bhi},
		Quantiles:         toQuantiles(bma, posteriorQuantileGrid),
		Samples:           bma.Samples(postSamples),
		UncertaintyBudget: &schema.UncertaintyBudget{Sampling: f64ptr(bma.WithinVar), Specification: f64ptr(bma.BetweenVar)},
	}

	// Plug-in local-linear estimate kept as the documented comparison: it reports
	// a narrower, sampling-led interval and is what under-covers, failing calibration.
	res := rdd.Estimate(estPts, floorCutoff, floorRefBW, floorBandwidths, true)
	plugLo, plugHi := res.Interval95()

	// --- validity battery.
	running := make([]float64, 0, len(schools16))
	for _, s := range schools16 {
		running = append(running, s.P8)
	}
	density := validity.DensityTest(running, floorCutoff, 0.05, floorRefBW)
	cov := []schema.NamedTestResult{
		validity.CovariateContinuity("ks2_prior_attainment", covPoints(schools16, func(s ingest.SchoolKS4) float64 { return s.KS2APS }), floorCutoff, floorRefBW, true),
		validity.CovariateContinuity("pct_disadvantaged_fsm", covPoints(schools16, func(s ingest.SchoolKS4) float64 { return s.PctFSM }), floorCutoff, floorRefBW, true),
		validity.CovariateContinuity("ks4_cohort_size", covPoints(schools16, func(s ingest.SchoolKS4) float64 { return s.Cohort }), floorCutoff, floorRefBW, true),
	}
	placebos := validity.PlaceboCutoffs(estPts, []float64{-1.3, 0.3}, floorRefBW, true)
	donut := validity.DonutRobustness(estPts, floorCutoff, floorRefBW, true, []float64{0.05, 0.1})

	covOK := true
	for _, c := range cov {
		covOK = covOK && c.Passed
	}
	admitted := density.Passed && covOK

	attBelow, attAbove := attritionRates(schools16, outcome, floorCutoff, floorRefBW)
	plugSD := res.TotalSD
	notes := fmt.Sprintf(
		"SBI: Bayesian model average over %d specs (bandwidth x order x kernel) via stochadex SMC (%d particles, %d rounds). "+
			"Honest interval [%.4f, %.4f] (sd %.4f) decomposes into sampling sd %.4f and identification sd %.4f. "+
			"For comparison, the plug-in local-linear interval is [%.4f, %.4f] (sd %.4f) — narrower because it ignores between-spec identification uncertainty; this is the gap a model that reports only sampling SE should fail the calibration score. "+
			"Design is effectively sharp: of %d schools below -0.5, only %d are excluded by the floor's CI condition (P8CIUPP>=0). "+
			"DIFFERENTIAL ATTRITION CAVEAT: within +/-%.1f of the cutoff, %.1f%% of below-floor schools vs %.1f%% of above-floor schools lack a linked 2017/18 P8 (sponsored academies are re-issued a new URN). "+
			"This attrition is correlated with treatment and biases the complete-case estimate; it is the dominant threat to this mark and motivates a future attrition-aware treatment.",
		len(bma.Specs), smcParticles, smcRounds,
		blo, bhi, bma.TotalSD, math.Sqrt(bma.WithinVar), math.Sqrt(bma.BetweenVar),
		plugLo, plugHi, plugSD,
		countBelow(schools16, floorCutoff), ciExcluded, floorRefBW, 100*attBelow, 100*attAbove)

	dossier := schema.ValidityDossier{
		Density:             density,
		CovariateContinuity: cov,
		PlaceboCutoffs:      placebos,
		BandwidthSweep:      sweepToSchema(res.Sweep),
		DonutRobustness:     donut,
		Admitted:            admitted,
		Notes:               notes,
	}

	mark := schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            markID,
		Series:        schema.SeriesFloorStandards,
		Domain:        "Education",
		UnitType:      "school",
		RDDType:       schema.Sharp,
		Design: schema.Design{
			RunningVariable: schema.Variable{
				Name: "progress_8_2016", Description: "Progress 8 score, 2015/16 revised performance tables",
				Units: "P8 score", SourceID: src16.SourceID,
			},
			Cutoff:      floorCutoff,
			Direction:   schema.BelowTreated,
			Action:      "Below the Progress 8 floor standard (-0.5): flagged for intervention/scrutiny (support, possible academy order).",
			Alternative: "At or above the floor: no floor-triggered intervention.",
			Outcome: schema.Variable{
				Name: "progress_8_2018", Description: "Progress 8 score two years later, 2017/18 revised performance tables",
				Units: "P8 score", SourceID: src18.SourceID,
			},
			Estimand: "Sharp RD effect at -0.5 of being flagged below the 2016 Progress 8 floor on the school's 2017/18 Progress 8 (local-to-cutoff, complete cases).",
		},
		Context: schema.Context{
			Description:    "State-funded mainstream secondary schools in England assessed against the 2016 Progress 8 floor standard.",
			CovariateNames: []string{"ks2_prior_attainment", "pct_disadvantaged_fsm", "ks4_cohort_size"},
			Population:     fmt.Sprintf("%d mainstream secondary schools with a numeric 2015/16 Progress 8 score", len(schools16)),
		},
		Sample:  nearCutoffSample(episodes, floorCutoff),
		Effect:  effect,
		Dossier: dossier,
		Provenance: schema.Provenance{
			Sources:                []schema.Source{toSchemaSource(src16), toSchemaSource(src18)},
			ContextAsOf:            "2016-08-25", // 2015/16 KS4 results realized
			DecisionTimestamp:      "2017-01-19", // revised 2015/16 tables + floor designation
			OutcomeTimestamp:       "2019-01-24", // revised 2017/18 tables published
			RunningVariableVintage: "KS4 2015/16 revised performance tables (Progress 8)",
			DecisionRound:          "Progress 8 floor standard, 2016 (assessed on 2015/16 results)",
			Seed:                   int64Ptr(smcSeed), // SMC seed; minting is deterministic given it
			ToolVersions: map[string]string{
				"go":                 runtime.Version(),
				"openaction2outcome": schema.SchemaVersion,
				"stochadex":          stochadexVersion(),
				"smc":                fmt.Sprintf("particles=%d,rounds=%d", smcParticles, smcRounds),
			},
			OutcomeRealized: true,
		},
	}
	if err := mark.Validate(); err != nil {
		return zero, fmt.Errorf("minted mark failed validation: %w", err)
	}
	return mark, nil
}

func covariates(s ingest.SchoolKS4) map[string]float64 {
	m := map[string]float64{}
	if !math.IsNaN(s.KS2APS) {
		m["ks2_prior_attainment"] = s.KS2APS
	}
	if !math.IsNaN(s.PctFSM) {
		m["pct_disadvantaged_fsm"] = s.PctFSM
	}
	if !math.IsNaN(s.Cohort) {
		m["ks4_cohort_size"] = s.Cohort
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func covPoints(schools []ingest.SchoolKS4, pick func(ingest.SchoolKS4) float64) []rdd.Point {
	var pts []rdd.Point
	for _, s := range schools {
		v := pick(s)
		if math.IsNaN(v) {
			continue
		}
		pts = append(pts, rdd.Point{X: s.P8, Y: v})
	}
	return pts
}

// episodeRows serialises the full episode table in the fixed column order.
func episodeRows(obs []schema.Observation) [][]string {
	rows := make([][]string, 0, len(obs))
	for _, o := range obs {
		rows = append(rows, []string{
			o.UnitID,
			o.UnitName,
			f(o.RunningValue),
			strconv.FormatBool(o.Assigned),
			boolOrEmpty(o.Treated),
			f64OrEmpty(o.Outcome),
			covOrEmpty(o.Covariates, "ks2_prior_attainment"),
			covOrEmpty(o.Covariates, "pct_disadvantaged_fsm"),
			covOrEmpty(o.Covariates, "ks4_cohort_size"),
		})
	}
	return rows
}

// nearCutoffSample returns the up-to-sampleMaxRows episode rows nearest the
// cutoff, re-sorted by unit_id for deterministic output.
func nearCutoffSample(obs []schema.Observation, cutoff float64) []schema.Observation {
	var near []schema.Observation
	for _, o := range obs {
		if math.Abs(o.RunningValue-cutoff) <= sampleHalfWidth {
			near = append(near, o)
		}
	}
	sort.Slice(near, func(i, j int) bool {
		return math.Abs(near[i].RunningValue-cutoff) < math.Abs(near[j].RunningValue-cutoff)
	})
	if len(near) > sampleMaxRows {
		near = near[:sampleMaxRows]
	}
	sort.Slice(near, func(i, j int) bool { return near[i].UnitID < near[j].UnitID })
	return near
}

// toQuantiles evaluates the BMA posterior at the given probabilities.
func toQuantiles(bma sbi.BMAResult, ps []float64) []schema.Quantile {
	out := make([]schema.Quantile, 0, len(ps))
	for _, pv := range bma.Quantiles(ps) {
		out = append(out, schema.Quantile{P: pv[0], Value: pv[1]})
	}
	return out
}

// stochadexVersion reports the resolved stochadex module version for provenance.
func stochadexVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/umbralcalc/stochadex" {
				return d.Version
			}
		}
	}
	return "unknown"
}

func sweepToSchema(sw []rdd.SweepPoint) []schema.SweepPoint {
	out := make([]schema.SweepPoint, len(sw))
	for i, s := range sw {
		se := s.SE
		out[i] = schema.SweepPoint{Param: s.Bandwidth, Estimate: s.Tau, StdErr: &se}
	}
	return out
}

func toSchemaSource(s ingest.Source) schema.Source {
	return schema.Source{
		SourceID:    s.SourceID,
		Title:       s.Title,
		Publisher:   s.Publisher,
		DownloadURI: s.DownloadURI,
		LandingPage: s.LandingPage,
		RetrievedAt: "", // retrieval date is recorded per-fetch; canonical bytes are hash-pinned
		Licence:     s.Licence,
		SHA256:      s.SHA256,
		Vintage:     s.Vintage,
	}
}

func attritionRates(schools []ingest.SchoolKS4, outcome map[string]float64, cutoff, h float64) (below, above float64) {
	var nB, mB, nA, mA int
	for _, s := range schools {
		if math.Abs(s.P8-cutoff) > h {
			continue
		}
		_, linked := outcome[s.URN]
		if s.P8 < cutoff {
			nB++
			if !linked {
				mB++
			}
		} else {
			nA++
			if !linked {
				mA++
			}
		}
	}
	if nB > 0 {
		below = float64(mB) / float64(nB)
	}
	if nA > 0 {
		above = float64(mA) / float64(nA)
	}
	return below, above
}

func countBelow(schools []ingest.SchoolKS4, cutoff float64) int {
	n := 0
	for _, s := range schools {
		if s.P8 < cutoff {
			n++
		}
	}
	return n
}

func f(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func f64OrEmpty(p *float64) string {
	if p == nil {
		return ""
	}
	return f(*p)
}

func boolOrEmpty(p *bool) string {
	if p == nil {
		return ""
	}
	return strconv.FormatBool(*p)
}

func covOrEmpty(m map[string]float64, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	return f(v)
}

func boolPtr(b bool) *bool      { return &b }
func f64ptr(f float64) *float64 { return &f }
func int64Ptr(i int64) *int64   { return &i }
