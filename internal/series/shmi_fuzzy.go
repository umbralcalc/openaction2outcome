package series

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/internal/sbi"
	"github.com/umbralcalc/openaction2outcome/internal/validity"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

const (
	shmiFuzzyMarkID         = "shmi-cqc-inspection-fuzzy"
	shmiFuzzyDecisionWindow = "APR18_MAR19"
	shmiFuzzyOutcomeWindow  = "APR19_MAR20"
	// The decision window was published Aug 2019; treatment = a CQC inspection in
	// roughly the 18 months after the flag.
	shmiFuzzyTreatStart = "2019-08-01"
	shmiFuzzyTreatEnd   = "2021-02-01"
)

var shmiFuzzyEpisodeColumns = []string{
	"unit_id", "unit_name", "running_value", "assigned", "treated", "outcome",
	"expected_deaths", "decision_shmi",
}

// BuildSHMIFuzzy mints the fuzzy SHMI mark: being banded "higher than expected"
// (running variable SHMI - OD_UL above 0) is an instrument for receiving a CQC
// inspection; the effect of that inspection on the trust's later SHMI is the Wald
// ratio at the cutoff. The mark is admitted only if the first stage is strong.
func BuildSHMIFuzzy(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	var zero schema.Mark

	shmiSrc, err := ingest.LoadSource(filepath.Join(rawDir, "shmi", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	cqcSrc, err := ingest.LoadSource(filepath.Join(rawDir, "cqc", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	shmiPath, cqcPath := shmiSrc.CachePath(cacheDir), cqcSrc.CachePath(cacheDir)
	if err := ingest.VerifySHA(shmiPath, shmiSrc.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `openaction2outcome fetch`)", err)
	}
	if err := ingest.VerifySHA(cqcPath, cqcSrc.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `openaction2outcome fetch`)", err)
	}
	shmiRows, err := ingest.LoadSHMI(shmiPath)
	if err != nil {
		return zero, err
	}
	cqc, err := ingest.LoadCQCInspections(cqcPath)
	if err != nil {
		return zero, err
	}

	byPeriod := make(map[string]map[string]ingest.TrustSHMI)
	for _, r := range shmiRows {
		if math.IsNaN(r.SHMI) || math.IsNaN(r.ODUpper) {
			continue
		}
		if byPeriod[r.TimePeriod] == nil {
			byPeriod[r.TimePeriod] = make(map[string]ingest.TrustSHMI)
		}
		byPeriod[r.TimePeriod][r.ProviderCode] = r
	}
	dec, out := byPeriod[shmiFuzzyDecisionWindow], byPeriod[shmiFuzzyOutcomeWindow]
	if dec == nil || out == nil {
		return zero, fmt.Errorf("shmi-fuzzy: decision/outcome window not found")
	}

	episodes := make([]schema.Observation, 0, len(dec))
	for code, d := range dec {
		rv := d.SHMI - d.ODUpper
		banded := rv > 0
		inspected := cqc.InspectedBetween(code, shmiFuzzyTreatStart, shmiFuzzyTreatEnd)
		o := schema.Observation{
			UnitID:       code + "@" + shmiFuzzyDecisionWindow,
			UnitName:     d.ProviderName,
			RunningValue: rv,
			Assigned:     banded,             // instrument
			Treated:      boolPtr(inspected), // realized treatment
			Covariates:   shmiCovariates(d),
		}
		if oc, ok := out[code]; ok && !math.IsNaN(oc.SHMI) {
			y := oc.SHMI
			o.Outcome = &y
		}
		episodes = append(episodes, o)
	}
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].UnitID < episodes[j].UnitID })

	var pts []sbi.FuzzyPoint
	var running []float64
	for _, o := range episodes {
		running = append(running, o.RunningValue)
		if o.Outcome != nil {
			d := 0.0
			if o.Treated != nil && *o.Treated {
				d = 1
			}
			pts = append(pts, sbi.FuzzyPoint{X: o.RunningValue, D: d, Y: *o.Outcome})
		}
	}
	if len(pts) < 20 {
		return zero, fmt.Errorf("shmi-fuzzy: too few complete cases (%d)", len(pts))
	}

	// Stage the analysis-ready episode rows as a build intermediate (not published
	// per-mark; reshaped into the one `episodes` dataset at export, joinable on ID).
	if _, err := publish.WriteEpisodesCSVGz(distDir, shmiFuzzyMarkID, "episodes.csv.gz", shmiFuzzyEpisodeColumns, shmiFuzzyEpisodeRows(episodes)); err != nil {
		return zero, err
	}

	fuzzy := sbi.EstimateFuzzyBMA(pts, 0, sbi.DefaultSHMISpecs(), false,
		sbi.SMCConfig{NumParticles: smcParticles, NumRounds: smcRounds, Seed: smcSeed})
	late := fuzzy.LATE
	blo, bhi := late.Interval(0.95)
	effect := schema.Distribution{
		Central:           late.Central,
		StdDev:            f64ptr(late.TotalSD),
		Interval:          &schema.Interval{Level: 0.95, Lower: blo, Upper: bhi},
		Quantiles:         toQuantiles(late, posteriorQuantileGrid),
		Samples:           late.Samples(postSamples),
		UncertaintyBudget: &schema.UncertaintyBudget{Sampling: f64ptr(late.WithinVar), Specification: f64ptr(late.BetweenVar)},
	}

	// Validity battery + the first-stage gate (a fuzzy mark needs a strong first stage).
	density := validity.DensityTest(running, 0, 0.02, shmiRefBW)
	cov := []schema.NamedTestResult{
		validity.CovariateContinuity("expected_deaths", covPointsFromObs(episodes, "expected_deaths"), 0, shmiRefBW, false),
	}
	covOK := true
	for _, c := range cov {
		covOK = covOK && c.Passed
	}
	fs := fuzzy.FirstStage
	firstStage := &schema.FirstStageResult{Jump: fs.Jump, StdErr: f64ptr(fs.SD), FStat: f64ptr(fs.FStat), Passed: fs.Passed}
	admitted := fs.Passed && density.Passed && covOK

	nInspected := 0
	for _, o := range episodes {
		if o.Treated != nil && *o.Treated {
			nInspected++
		}
	}
	notes := fmt.Sprintf(
		"Fuzzy RDD: being banded 'higher than expected' (SHMI above OD_UL) instruments receipt of a CQC inspection in the ~18 months after the flag; the effect of inspection on the trust's later SHMI is the Wald ratio at the cutoff. "+
			"First stage: jump in inspection probability %.3f (sd %.3f, F=%.1f) — %s the weak-instrument threshold (F>=10). "+
			"Of %d trusts, %d were inspected in the treatment window. "+
			"SINGLE-WINDOW CAVEAT: CQC largely stopped routine acute-trust inspections after 2019, so only the pre-pandemic window (APR18_MAR19 -> APR19_MAR20) has treatment variation; only ~13 trusts are flagged, so the first stage is imprecise. "+
			"TIMING CAVEAT: the outcome window overlaps the treatment (inspection) period in calendar time, and a strictly-post-treatment outcome would fall in the COVID-distorted period. "+
			"Admission requires a strong first stage; where it is not met the fuzzy mark is not admitted and the sharp intention-to-treat SHMI mark stands.",
		fs.Jump, fs.SD, fs.FStat, passFail(fs.Passed), len(episodes), nInspected)

	dossier := schema.ValidityDossier{
		Density:             density,
		CovariateContinuity: cov,
		FirstStage:          firstStage,
		Admitted:            admitted,
		Notes:               notes,
	}

	mark := schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            shmiFuzzyMarkID,
		Series:        schema.SeriesSHMI,
		Domain:        "Health",
		UnitType:      "nhs-trust",
		RDDType:       schema.Fuzzy,
		Design: schema.Design{
			RunningVariable: schema.Variable{
				Name: "shmi_minus_od_upper", Description: "SHMI minus the overdispersed upper control limit (positive => banded 'higher than expected')",
				Units: "SHMI ratio", SourceID: shmiSrc.SourceID,
			},
			Cutoff:      0,
			Direction:   schema.AboveTreated,
			Action:      "Banded 'higher than expected' mortality, which raises the probability of (but does not guarantee) a CQC inspection.",
			Alternative: "Banded 'as expected': lower probability of a CQC inspection.",
			Outcome: schema.Variable{
				Name: "shmi_next_window", Description: "The trust's SHMI in the following 12-month window",
				Units: "SHMI ratio", SourceID: shmiSrc.SourceID,
			},
			Estimand: "Fuzzy RD local average treatment effect at the upper control limit: the effect of receiving a CQC inspection (instrumented by the 'higher than expected' banding) on the trust's later SHMI.",
		},
		Context: schema.Context{
			Description:    "English non-specialist acute NHS trusts; the banding instruments CQC inspection.",
			CovariateNames: []string{"expected_deaths", "decision_shmi"},
			Population:     fmt.Sprintf("%d acute trusts in the %s window", len(episodes), shmiFuzzyDecisionWindow),
		},
		Sample:  nearCutoffSample(episodes, 0),
		Effect:  effect,
		Dossier: dossier,
		Provenance: schema.Provenance{
			Sources:                []schema.Source{toSchemaSource(shmiSrc), toSchemaSource(cqcSrc)},
			ContextAsOf:            "2019-03-31",
			DecisionTimestamp:      "2019-08-01", // banding published
			OutcomeTimestamp:       "2020-08-01", // outcome window published
			RunningVariableVintage: "SHMI historical trust-level series, as published (release Oct 2024 - Sep 2025)",
			DecisionRound:          "SHMI 'higher than expected' banding, APR18_MAR19; CQC inspection treatment",
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

func passFail(ok bool) string {
	if ok {
		return "meets"
	}
	return "does NOT meet"
}

func shmiFuzzyEpisodeRows(obs []schema.Observation) [][]string {
	rows := make([][]string, 0, len(obs))
	for _, o := range obs {
		rows = append(rows, []string{
			o.UnitID, o.UnitName, f(o.RunningValue),
			boolOrEmpty(&o.Assigned), boolOrEmpty(o.Treated), f64OrEmpty(o.Outcome),
			covOrEmpty(o.Covariates, "expected_deaths"), covOrEmpty(o.Covariates, "decision_shmi"),
		})
	}
	return rows
}
