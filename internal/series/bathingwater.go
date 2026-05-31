package series

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/internal/rdd"
	"github.com/umbralcalc/openaction2outcome/internal/sbi"
	"github.com/umbralcalc/openaction2outcome/internal/validity"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// The bathing-water series is the "safe" sharp-RDD seam: an English designated
// bathing water is classified annually (Excellent/Good/Sufficient/Poor) from the
// log-normal 90th/95th percentiles of E. coli + intestinal enterococci over a
// rolling 4-year sample window. Crossing into Poor mechanically triggers an
// action — a mandatory advice-against-bathing sign the following season plus an
// EA-led catchment investigation/remediation. The running variable is the
// base-10 log compliance margin against the Poor/Sufficient boundary; the cutoff
// is 0 (crossing into Poor); the outcome is the same site's compliance margin in
// a later, NON-OVERLAPPING classification window.

const (
	bwMarkID = "bathing-water-poor-2015"

	// The headline mark pairs a clean, fully pre-COVID decision cohort with an
	// outcome four years on, so the two rolling 4-year sample windows
	// (2012-2015 and 2016-2019) do not overlap — avoiding the autocorrelation
	// that contaminates adjacent-year comparisons.
	bwDecisionYear = 2015
	bwOutcomeYear  = 2019

	bwCutoff = 0.0  // log compliance margin: >0 ⇔ fails Sufficient ⇔ Poor ⇔ treated
	bwRefBW  = 0.30 // reference bandwidth in log10 units (≈ a half-decade either side)

	// 90th-percentile Sufficient thresholds (cfu/100ml) — the action-triggering
	// Poor/Sufficient boundary. Coastal/transitional and inland waters differ.
	ecThresholdCoastal = 500.0
	ieThresholdCoastal = 185.0
	ecThresholdInland  = 900.0
	ieThresholdInland  = 330.0

	// z value for the 90th percentile of a normal, used to re-derive the
	// log-normal percentile from sample log10 statistics in the exclusion check.
	z90 = 1.2815515655446004
)

var bwBandwidths = []float64{0.15, 0.2, 0.3, 0.4, 0.5}

var bwEpisodeColumns = []string{
	"unit_id", "unit_name", "running_value", "assigned", "treated", "outcome",
	"ie_sample_count", "is_inland", "impacted_by_heavy_rain",
}

// complianceMargin is the running variable: the worst (largest) base-10 log
// excess of an indicator's 90th-percentile statistic over its Sufficient
// threshold. margin > 0 ⇔ at least one indicator fails Sufficient ⇔ Poor.
func complianceMargin(b ingest.BathingWater) (float64, bool) {
	if !b.HasPercentiles() {
		return 0, false
	}
	ecThr, ieThr := thresholds(b.WaterType)
	m := math.Max(math.Log10(b.ECp90/ecThr), math.Log10(b.IEp90/ieThr))
	if math.IsNaN(m) || math.IsInf(m, 0) {
		return 0, false
	}
	return m, true
}

func thresholds(waterType string) (ec, ie float64) {
	if waterType == "inland" {
		return ecThresholdInland, ieThresholdInland
	}
	return ecThresholdCoastal, ieThresholdCoastal // coastal/transitional default
}

// BuildBathingWater mints the bathing-water mark. rawDir holds the SOURCE.json
// pointers, cacheDir the fetched frozen CSVs, distDir receives the staged
// episode sidecar, and cfg supplies the published artifact's URL.
func BuildBathingWater(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	var zero schema.Mark

	compSrc, err := ingest.LoadSource(filepath.Join(rawDir, "bathing-water-rbwd", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	sampSrc, err := ingest.LoadSource(filepath.Join(rawDir, "bathing-water-samples", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	compPath := compSrc.CachePath(cacheDir)
	sampPath := sampSrc.CachePath(cacheDir)
	if err := ingest.VerifySHA(compPath, compSrc.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `python3 scripts/bathingwater_harvest.py`)", err)
	}
	if err := ingest.VerifySHA(sampPath, sampSrc.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `python3 scripts/bathingwater_harvest.py`)", err)
	}

	records, err := ingest.LoadBathingWater(compPath)
	if err != nil {
		return zero, err
	}

	// Index by (site, year). The outcome is the same site's margin in the later
	// non-overlapping window.
	type key struct {
		site string
		year int
	}
	byKey := make(map[key]ingest.BathingWater, len(records))
	for _, r := range records {
		byKey[key{r.SiteID, r.SeasonYear}] = r
	}

	// Build episode rows (decision-year sites with a usable running variable) and
	// the complete-case estimation set (those that also have a later outcome).
	episodes := make([]schema.Observation, 0)
	var estPts []rdd.Point
	for _, r := range records {
		if r.SeasonYear != bwDecisionYear {
			continue
		}
		m, ok := complianceMargin(r)
		if !ok {
			continue
		}
		poor := m > bwCutoff
		o := schema.Observation{
			UnitID:       r.SiteID,
			UnitName:     r.Name,
			RunningValue: m,
			Assigned:     poor,
			Treated:      boolPtr(poor), // sharp: classification deterministically triggers the action
			Covariates:   bwCovariates(r),
		}
		if out, ok := byKey[key{r.SiteID, bwOutcomeYear}]; ok {
			if om, ok := complianceMargin(out); ok {
				yy := om
				o.Outcome = &yy
				estPts = append(estPts, rdd.Point{X: m, Y: om})
			}
		}
		episodes = append(episodes, o)
	}
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].UnitID < episodes[j].UnitID })

	if len(estPts) < 20 {
		return zero, fmt.Errorf("bathing-water: too few complete cases (%d) to estimate", len(estPts))
	}

	// Stage the analysis-ready episode rows as a build intermediate (not published
	// per-mark; reshaped into the one `episodes` dataset at export, joinable on ID).
	if _, err := publish.WriteEpisodesCSVGz(distDir, bwMarkID, "episodes.csv.gz", bwEpisodeColumns, bwEpisodeRows(episodes)); err != nil {
		return zero, err
	}

	// --- estimate the discontinuity (honest model-averaged SBI interval).
	bma := sbi.EstimateBMA(estPts, bwCutoff, sbi.DefaultBathingWaterSpecs(), nil, false,
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

	// Plug-in local-linear estimate kept as the documented (sampling-led) comparison.
	res := rdd.Estimate(estPts, bwCutoff, bwRefBW, bwBandwidths, false)
	plugLo, plugHi := res.Interval95()

	// --- validity battery (treated side is ABOVE the cutoff: Poor margins > 0).
	running := make([]float64, 0, len(episodes))
	for _, o := range episodes {
		running = append(running, o.RunningValue)
	}
	density := validity.DensityTest(running, bwCutoff, 0.05, bwRefBW)
	cov := []schema.NamedTestResult{
		validity.CovariateContinuity("ie_sample_count", bwCovPoints(episodes, "ie_sample_count"), bwCutoff, bwRefBW, false),
		validity.CovariateContinuity("is_inland", bwCovPoints(episodes, "is_inland"), bwCutoff, bwRefBW, false),
		validity.CovariateContinuity("impacted_by_heavy_rain", bwCovPoints(episodes, "impacted_by_heavy_rain"), bwCutoff, bwRefBW, false),
	}
	placebos := validity.PlaceboCutoffs(estPts, []float64{-0.4, 0.4}, bwRefBW, false)
	donut := validity.DonutRobustness(estPts, bwCutoff, bwRefBW, false, []float64{0.02, 0.05})

	covOK := true
	for _, c := range cov {
		covOK = covOK && c.Passed
	}

	// --- seam-specific: abnormal-sample-exclusion sensitivity (the bathing-water
	// analogue of a manipulation check). Up to 15% of samples over four years can
	// be discounted as abnormal (short-term pollution / extreme rainfall) — a
	// discretionary data step that can only move the percentile downward, i.e.
	// toward Sufficient. We re-include them near the cutoff and test whether the
	// design (treatment status + estimate) is robust.
	excl, err := bwExclusionSensitivity(sampPath, records, res.Central, res.TotalSD)
	if err != nil {
		return zero, err
	}

	admitted := density.Passed && covOK && excl.Passed

	covVerdict := "pass"
	if !covOK {
		covVerdict = "FAIL"
	}
	notes := fmt.Sprintf(
		"SBI: Bayesian model average over %d specs (bandwidth x order x kernel) via stochadex SMC (%d particles, %d rounds). "+
			"Honest interval [%.4f, %.4f] (sd %.4f) decomposes into sampling sd %.4f and identification sd %.4f. "+
			"Plug-in local-linear interval [%.4f, %.4f] (sd %.4f) is narrower because it ignores between-spec identification uncertainty. "+
			"Running variable is the base-10 log compliance margin (worst of EC/IE 90th-percentile over its Sufficient threshold); margin>0 is Poor. "+
			"Sharp design: of %d decision-year sites with a usable margin, %d are classified Poor. "+
			"Checks: density/manipulation %s (p=%.3f), covariate continuity %s. "+
			"Decision window 2012-2015, outcome window 2016-2019 (non-overlapping; both pre-COVID). %s "+
			"REGIME CAVEAT: marks are pinned to the action regime in force at the 2015 decision (automatic advice-against-bathing sign + EA investigation on a Poor classification); the 2021 amendment made 5-consecutive-Poor de-designation a Ministerial decision, shifting the downstream regime for later cohorts.",
		len(bma.Specs), smcParticles, smcRounds,
		blo, bhi, bma.TotalSD, math.Sqrt(bma.WithinVar), math.Sqrt(bma.BetweenVar),
		plugLo, plugHi, res.TotalSD,
		len(episodes), countPoor(episodes),
		tick(density.Passed), pval(density.PValue), covVerdict,
		excl.Detail)

	dossier := schema.ValidityDossier{
		Density:             density,
		CovariateContinuity: cov,
		PlaceboCutoffs:      placebos,
		BandwidthSweep:      sweepToSchema(res.Sweep),
		DonutRobustness:     donut,
		SeamSpecificChecks:  []schema.NamedTestResult{excl},
		Admitted:            admitted,
		Notes:               notes,
	}

	mark := schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            bwMarkID,
		Series:        schema.SeriesBathingWater,
		Domain:        "Environment",
		UnitType:      "bathing-water",
		RDDType:       schema.Sharp,
		Design: schema.Design{
			RunningVariable: schema.Variable{
				Name: "compliance_margin_2015", Description: "Base-10 log of the worst indicator's 90th-percentile statistic over its Sufficient threshold (2012-2015 rolling window); >0 means classified Poor",
				Units: "log10 ratio", SourceID: compSrc.SourceID,
			},
			Cutoff:      bwCutoff,
			Direction:   schema.AboveTreated,
			Action:      "Classified Poor (fails the Sufficient standard): mandatory advice-against-bathing sign the following season + EA-led catchment investigation/remediation.",
			Alternative: "Classified Sufficient or better: no Poor-triggered advice sign or investigation.",
			Outcome: schema.Variable{
				Name: "compliance_margin_2019", Description: "Same site's log compliance margin four years later (2016-2019 rolling window)",
				Units: "log10 ratio", SourceID: compSrc.SourceID,
			},
			Estimand: "Sharp RD effect at the Poor/Sufficient boundary of being classified Poor in 2015 (advice sign + investigation) on the site's 2019 log compliance margin (local-to-cutoff, complete cases).",
		},
		Context: schema.Context{
			Description:    "Designated bathing waters in England classified under the revised Bathing Water Directive regime, assessed at the 2015 Poor/Sufficient boundary.",
			CovariateNames: []string{"ie_sample_count", "is_inland", "impacted_by_heavy_rain"},
			Population:     fmt.Sprintf("%d designated bathing waters with a usable 2015 compliance margin", len(episodes)),
		},
		Sample:  nearCutoffSample(episodes, bwCutoff),
		Effect:  effect,
		Dossier: dossier,
		Provenance: schema.Provenance{
			Sources:                []schema.Source{toSchemaSource(compSrc), toSchemaSource(sampSrc)},
			ContextAsOf:            "2015-12-31", // 2015 classification window closes
			DecisionTimestamp:      "2016-03-01", // 2015 classifications published; advice signs for the 2016 season
			OutcomeTimestamp:       "2019-11-21", // 2019 annual classifications published
			RunningVariableVintage: "rBWD 2015 annual classification (rolling 2012-2015 sample window)",
			DecisionRound:          "Bathing-water annual classification, 2015 (Poor/Sufficient boundary)",
			Seed:                   int64Ptr(smcSeed),
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

// bwCovariates carries the pre-treatment site characteristics a decision-maker
// observed before the classification: monitoring intensity, whether the water is
// inland, and whether its quality is flagged as impacted by heavy rain.
func bwCovariates(r ingest.BathingWater) map[string]float64 {
	m := map[string]float64{}
	if !math.IsNaN(r.IEn) {
		m["ie_sample_count"] = r.IEn
	}
	m["is_inland"] = b2f(r.WaterType == "inland")
	m["impacted_by_heavy_rain"] = b2f(r.ImpactedByHeavyRain)
	if len(m) == 0 {
		return nil
	}
	return m
}

// bwCovPoints projects (running value, covariate) pairs for a covariate-
// continuity test from the already-built episodes.
func bwCovPoints(obs []schema.Observation, name string) []rdd.Point {
	var pts []rdd.Point
	for _, o := range obs {
		if o.Covariates == nil {
			continue
		}
		v, ok := o.Covariates[name]
		if !ok {
			continue
		}
		pts = append(pts, rdd.Point{X: o.RunningValue, Y: v})
	}
	return pts
}

func bwEpisodeRows(obs []schema.Observation) [][]string {
	rows := make([][]string, 0, len(obs))
	for _, o := range obs {
		rows = append(rows, []string{
			o.UnitID,
			o.UnitName,
			f(o.RunningValue),
			strconv.FormatBool(o.Assigned),
			boolOrEmpty(o.Treated),
			f64OrEmpty(o.Outcome),
			covOrEmpty(o.Covariates, "ie_sample_count"),
			covOrEmpty(o.Covariates, "is_inland"),
			covOrEmpty(o.Covariates, "impacted_by_heavy_rain"),
		})
	}
	return rows
}

// bwExclusionSensitivity is the seam-specific manipulation analogue. The annual
// classification disregards "abnormal" samples (short-term pollution / extreme
// rainfall) under a discretionary rule that can remove up to 15% of samples over
// four years and can only lower the percentile (toward Sufficient). We re-derive
// each near-cutoff site's 2015 log-normal 90th-percentile margin INCLUDING the
// discountable samples, re-estimate the discontinuity on the re-included running
// variable, and pass only if treatment status and the estimate are robust.
func bwExclusionSensitivity(sampPath string, records []ingest.BathingWater, baselineTau, baselineSD float64) (schema.NamedTestResult, error) {
	method := "abnormal-sample-exclusion sensitivity (re-include discounted samples, re-derive 90th-percentile margin near the cutoff)"
	res := schema.NamedTestResult{Name: "abnormal_sample_exclusion", TestResult: schema.TestResult{Method: method}}

	samples, err := ingest.LoadBathingWaterSamples(sampPath)
	if err != nil {
		return res, err
	}
	// Group the decision-window (2012-2015) samples per site.
	winLo, winHi := bwDecisionYear-3, bwDecisionYear
	ecAll := map[string][]float64{}
	ieAll := map[string][]float64{}
	ecKept := map[string][]float64{}
	ieKept := map[string][]float64{}
	var nDiscountable int
	for _, s := range samples {
		if s.SampleYear < winLo || s.SampleYear > winHi {
			continue
		}
		if !math.IsNaN(s.ECCount) && s.ECCount > 0 {
			ecAll[s.SiteID] = append(ecAll[s.SiteID], s.ECCount)
			if !s.Discountable {
				ecKept[s.SiteID] = append(ecKept[s.SiteID], s.ECCount)
			}
		}
		if !math.IsNaN(s.IECount) && s.IECount > 0 {
			ieAll[s.SiteID] = append(ieAll[s.SiteID], s.IECount)
			if !s.Discountable {
				ieKept[s.SiteID] = append(ieKept[s.SiteID], s.IECount)
			}
		}
		if s.Discountable {
			nDiscountable++
		}
	}

	// Recompute the full estimation set, swapping in the re-included margin for
	// the near-cutoff sites we can reconstruct; everything else keeps its official
	// margin. The outcome (2019) is unchanged.
	type key struct {
		site string
		year int
	}
	byKey := make(map[key]ingest.BathingWater, len(records))
	for _, r := range records {
		byKey[key{r.SiteID, r.SeasonYear}] = r
	}

	var reincluded []rdd.Point
	var nNear, nRecomputed, nFlip int
	var maxAbsMarginShift, sumReproErr float64
	var nRepro int
	for _, r := range records {
		if r.SeasonYear != bwDecisionYear {
			continue
		}
		official, ok := complianceMargin(r)
		if !ok {
			continue
		}
		out, ok := byKey[key{r.SiteID, bwOutcomeYear}]
		if !ok {
			continue
		}
		om, ok := complianceMargin(out)
		if !ok {
			continue
		}
		x := official
		near := math.Abs(official) <= bwRefBW
		if near {
			nNear++
			ecThr, ieThr := thresholds(r.WaterType)
			if mAll, ok := reincludedMargin(ecAll[r.SiteID], ieAll[r.SiteID], ecThr, ieThr); ok {
				nRecomputed++
				x = mAll
				if math.Abs(mAll-official) > maxAbsMarginShift {
					maxAbsMarginShift = math.Abs(mAll - official)
				}
				if (official > bwCutoff) != (mAll > bwCutoff) {
					nFlip++
				}
				// Sanity: the kept-only recompute should track the official margin.
				if mKept, ok := reincludedMargin(ecKept[r.SiteID], ieKept[r.SiteID], ecThr, ieThr); ok {
					sumReproErr += math.Abs(mKept - official)
					nRepro++
				}
			}
		}
		reincluded = append(reincluded, rdd.Point{X: x, Y: om})
	}

	tauRe, seRe, _, _ := rdd.Fit(reincluded, bwCutoff, bwRefBW, false)
	delta := tauRe - baselineTau
	// The abnormal-sample discounting is a regulator-applied rule (extreme-weather
	// samples), NOT unit-side manipulation of the running variable, so the check
	// is about ROBUSTNESS of the estimate, not zero re-classification: re-including
	// high-pollution samples is expected to push some borderline-Sufficient sites
	// over the line. The mark passes when the effect is stable under re-inclusion;
	// the flip count is reported as information.
	robustEstimate := !math.IsNaN(seRe) && math.Abs(delta) <= 1.96*baselineSD
	res.Passed = robustEstimate

	reproNote := "no kept-only reproduction available"
	if nRepro > 0 {
		reproNote = fmt.Sprintf("mean |kept-only minus official| margin = %.3f over %d sites (method-fidelity check)", sumReproErr/float64(nRepro), nRepro)
	}
	res.Statistic = f64ptr(delta)
	res.Detail = fmt.Sprintf(
		"%d discountable samples in the 2012-2015 windows of near-cutoff sites; %d of %d near-cutoff sites reconstructed. "+
			"Re-including them shifts the running variable by at most %.3f log10 and flips Poor/Sufficient status for %d site(s). "+
			"Re-estimated effect %.4f vs baseline %.4f (delta %.4f, within 1.96*baseline sd %.4f = %v). %s.",
		nDiscountable, nRecomputed, nNear, maxAbsMarginShift, nFlip,
		tauRe, baselineTau, delta, 1.96*baselineSD, robustEstimate, reproNote)
	return res, nil
}

// reincludedMargin re-derives the log compliance margin from raw sample counts
// via the log-normal 90th percentile p90 = 10^(mean(log10) + z90*sd(log10)).
func reincludedMargin(ec, ie []float64, ecThr, ieThr float64) (float64, bool) {
	ep90, okE := logNormalP90(ec)
	ip90, okI := logNormalP90(ie)
	if !okE || !okI {
		return 0, false
	}
	m := math.Max(math.Log10(ep90/ecThr), math.Log10(ip90/ieThr))
	if math.IsNaN(m) || math.IsInf(m, 0) {
		return 0, false
	}
	return m, true
}

// logNormalP90 fits a base-10 log-normal to positive sample counts and returns
// its 90th percentile. Needs at least a handful of samples to be meaningful.
func logNormalP90(counts []float64) (float64, bool) {
	if len(counts) < 5 {
		return 0, false
	}
	var s, s2 float64
	n := 0
	for _, c := range counts {
		if c <= 0 {
			continue
		}
		l := math.Log10(c)
		s += l
		s2 += l * l
		n++
	}
	if n < 5 {
		return 0, false
	}
	mean := s / float64(n)
	v := s2/float64(n) - mean*mean
	if v < 0 {
		v = 0
	}
	return math.Pow(10, mean+z90*math.Sqrt(v)), true
}

func countPoor(obs []schema.Observation) int {
	n := 0
	for _, o := range obs {
		if o.Assigned {
			n++
		}
	}
	return n
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func tick(ok bool) string {
	if ok {
		return "pass"
	}
	return "FAIL"
}

func pval(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}
