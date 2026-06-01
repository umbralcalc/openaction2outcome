package series

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/internal/rdd"
	"github.com/umbralcalc/openaction2outcome/internal/sbi"
	"github.com/umbralcalc/openaction2outcome/internal/validity"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

const (
	shmiMarkID = "shmi-higher-than-expected-banding"
	shmiCutoff = 0.0
	shmiRefBW  = 0.15
)

// shmiWindowPairs are non-overlapping (decision, outcome) reporting windows. The
// running variable is read from the decision window; the outcome from the next
// window 12 months later. The three heavily COVID-distorted decision windows
// (Apr 2019 - Mar 2022) are deliberately excluded.
var shmiWindowPairs = [][2]string{
	{"APR18_MAR19", "APR19_MAR20"},
	{"APR22_MAR23", "APR23_MAR24"},
	{"APR23_MAR24", "APR24_MAR25"},
}

var shmiBandwidths = []float64{0.08, 0.12, 0.15, 0.2, 0.25}

var shmiEpisodeColumns = []string{
	"unit_id", "unit_name", "running_value", "assigned", "treated", "outcome",
	"expected_deaths", "decision_shmi",
}

// BuildSHMI mints the SHMI series mark: a sharp intention-to-treat RDD on the
// "higher than expected" mortality banding. A trust is flagged when its SHMI
// exceeds the overdispersed upper control limit (OD_UL), so the running variable
// is SHMI - OD_UL with a cutoff of 0 and the flagged side above. The outcome is
// the trust's SHMI in the following 12-month window. Trust-years are pooled
// across non-overlapping windows.
func BuildSHMI(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	var zero schema.Mark

	src, err := ingest.LoadSource(filepath.Join(rawDir, "shmi", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	path := src.CachePath(cacheDir)
	if err := ingest.VerifySHA(path, src.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `openaction2outcome fetch`)", err)
	}
	rows, err := ingest.LoadSHMI(path)
	if err != nil {
		return zero, err
	}

	// Index: reporting window -> provider code -> record.
	byPeriod := make(map[string]map[string]ingest.TrustSHMI)
	for _, r := range rows {
		if math.IsNaN(r.SHMI) || math.IsNaN(r.ODUpper) {
			continue
		}
		m := byPeriod[r.TimePeriod]
		if m == nil {
			m = make(map[string]ingest.TrustSHMI)
			byPeriod[r.TimePeriod] = m
		}
		byPeriod[r.TimePeriod][r.ProviderCode] = r
	}

	episodes := make([]schema.Observation, 0)
	pairsUsed := 0
	for _, wp := range shmiWindowPairs {
		dec, out := byPeriod[wp[0]], byPeriod[wp[1]]
		if dec == nil || out == nil {
			continue
		}
		pairsUsed++
		for code, d := range dec {
			rv := d.SHMI - d.ODUpper
			treated := rv > shmiCutoff
			o := schema.Observation{
				UnitID:       code + "@" + wp[0],
				UnitName:     d.ProviderName,
				RunningValue: rv,
				Assigned:     treated,
				Treated:      boolPtr(treated),
				Covariates:   shmiCovariates(d),
			}
			if oc, ok := out[code]; ok && !math.IsNaN(oc.SHMI) {
				y := oc.SHMI
				o.Outcome = &y
			}
			episodes = append(episodes, o)
		}
	}
	if pairsUsed == 0 {
		return zero, fmt.Errorf("shmi: none of the configured window pairs were found in the data")
	}
	// Sort first, then derive the running values and estimation points in a
	// deterministic order (map iteration above is unordered, and floating-point
	// summation is not associative, so a fixed order keeps re-mints identical).
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].UnitID < episodes[j].UnitID })
	var estPts []rdd.Point
	running := make([]float64, 0, len(episodes))
	for _, o := range episodes {
		running = append(running, o.RunningValue)
		if o.Outcome != nil {
			estPts = append(estPts, rdd.Point{X: o.RunningValue, Y: *o.Outcome})
		}
	}
	if len(estPts) < 20 {
		return zero, fmt.Errorf("shmi: too few linked trust-years (%d) to estimate", len(estPts))
	}

	// Stage the analysis-ready episode rows as a build intermediate (not published
	// per-mark; reshaped into the one `episodes` dataset at export, joinable on ID).
	if _, err := publish.WriteEpisodesCSVGz(distDir, shmiMarkID, "episodes.csv.gz", shmiEpisodeColumns, shmiEpisodeRows(episodes)); err != nil {
		return zero, err
	}

	// Model-averaged SBI estimate (the flagged side is ABOVE the cutoff).
	bma := sbi.EstimateBMA(estPts, shmiCutoff, sbi.DefaultSHMISpecs(), nil, false,
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
	plug := rdd.Estimate(estPts, shmiCutoff, shmiRefBW, shmiBandwidths, false)
	plugLo, plugHi := plug.Interval95()

	// Validity battery.
	density := validity.DensityTest(running, shmiCutoff, 0.02, shmiRefBW)
	cov := []schema.NamedTestResult{
		validity.CovariateContinuity("expected_deaths", covPointsFromObs(episodes, "expected_deaths"), shmiCutoff, shmiRefBW, false),
	}
	placebos := validity.PlaceboCutoffs(estPts, []float64{-0.2, -0.1}, shmiRefBW, false)
	donut := validity.DonutRobustness(estPts, shmiCutoff, shmiRefBW, false, []float64{0.02, 0.04})

	covOK := true
	for _, c := range cov {
		covOK = covOK && c.Passed
	}
	admitted := density.Passed && covOK

	notes := fmt.Sprintf(
		"Sharp intention-to-treat RDD on the SHMI 'higher than expected' banding, pooled over %d non-overlapping reporting-window pairs (trust-years). "+
			"Effect of being publicly flagged (SHMI above the overdispersed upper control limit) on the trust's SHMI in the following 12-month window. "+
			"SBI honest interval [%.4f, %.4f] (sd %.4f) splits into sampling sd %.4f and identification sd %.4f; the plug-in interval is [%.4f, %.4f] (sd %.4f). "+
			"SMALL-N CAVEAT: only ~120 acute trusts per window, so the interval is wide by design. "+
			"The RDD nets out SHMI's mean reversion (smooth through the cutoff); the flagging effect is the discontinuity. "+
			"Pooling assumes a stable effect across windows and treats trust-years as units (within-trust serial correlation is not modelled, which understates the sampling component). "+
			"COVID windows (Apr 2019 - Mar 2022 decisions) are excluded. This is intention-to-treat on the banding label, not the effect of any specific downstream intervention.",
		pairsUsed, blo, bhi, bma.TotalSD, math.Sqrt(bma.WithinVar), math.Sqrt(bma.BetweenVar), plugLo, plugHi, plug.TotalSD)

	dossier := schema.ValidityDossier{
		Density:             density,
		CovariateContinuity: cov,
		PlaceboCutoffs:      placebos,
		BandwidthSweep:      sweepToSchema(plug.Sweep),
		DonutRobustness:     donut,
		Admitted:            admitted,
		Notes:               notes,
	}

	mark := schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		MechanismID:   "shmi-mortality-banding",
		Category:      schema.CategoryIdentified,
		TruthSource:   schema.TruthIdentified,
		ID:            shmiMarkID,
		Series:        schema.SeriesSHMI,
		Domain:        "Health",
		UnitType:      "nhs-trust",
		RDDType:       schema.Sharp,
		Design: schema.Design{
			RunningVariable: schema.Variable{
				Name: "shmi_minus_od_upper", Description: "SHMI minus the overdispersed upper control limit (positive => banded 'higher than expected')",
				Units: "SHMI ratio", SourceID: src.SourceID,
			},
			Cutoff:      shmiCutoff,
			Direction:   schema.AboveTreated,
			Action:      "Publicly banded 'higher than expected' mortality (SHMI above the overdispersed upper control limit) — a 'smoke alarm' that raises scrutiny.",
			Alternative: "Banded 'as expected' (SHMI within the control limits): no higher-than-expected flag.",
			Outcome: schema.Variable{
				Name: "shmi_next_window", Description: "The trust's SHMI in the following non-overlapping 12-month window",
				Units: "SHMI ratio", SourceID: src.SourceID,
			},
			Estimand: "Sharp RD (intention-to-treat) effect at the upper control limit of being banded 'higher than expected' on the trust's SHMI in the following 12-month window (pooled trust-years, local-to-cutoff).",
		},
		Context: schema.Context{
			Description:    "English non-specialist acute NHS trusts assessed against the SHMI 'higher than expected' mortality banding.",
			CovariateNames: []string{"expected_deaths", "decision_shmi"},
			Population:     fmt.Sprintf("%d pooled trust-years across non-overlapping reporting windows", len(episodes)),
		},
		Sample:  nearCutoffSample(episodes, shmiCutoff),
		Effect:  effect,
		Dossier: dossier,
		Provenance: schema.Provenance{
			Sources:                []schema.Source{toSchemaSource(src)},
			ContextAsOf:            "2018-04-01", // earliest decision window start
			DecisionTimestamp:      "2024-03-31", // latest decision window end
			OutcomeTimestamp:       "2025-03-31", // latest outcome window end
			RunningVariableVintage: "SHMI historical trust-level series, as published (release Oct 2024 - Sep 2025)",
			DecisionRound:          "SHMI 'higher than expected' banding, pooled non-overlapping windows",
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

func shmiCovariates(d ingest.TrustSHMI) map[string]float64 {
	m := map[string]float64{"decision_shmi": d.SHMI}
	if !math.IsNaN(d.Expected) {
		m["expected_deaths"] = d.Expected
	}
	return m
}

func covPointsFromObs(obs []schema.Observation, key string) []rdd.Point {
	var pts []rdd.Point
	for _, o := range obs {
		if o.Covariates == nil {
			continue
		}
		v, ok := o.Covariates[key]
		if !ok {
			continue
		}
		pts = append(pts, rdd.Point{X: o.RunningValue, Y: v})
	}
	return pts
}

func shmiEpisodeRows(obs []schema.Observation) [][]string {
	rows := make([][]string, 0, len(obs))
	for _, o := range obs {
		rows = append(rows, []string{
			o.UnitID,
			o.UnitName,
			f(o.RunningValue),
			boolOrEmpty(&o.Assigned),
			boolOrEmpty(o.Treated),
			f64OrEmpty(o.Outcome),
			covOrEmpty(o.Covariates, "expected_deaths"),
			covOrEmpty(o.Covariates, "decision_shmi"),
		})
	}
	return rows
}
