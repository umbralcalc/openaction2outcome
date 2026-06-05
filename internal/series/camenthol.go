package series

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/umbralcalc/openaction2outcome/internal/did"
	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// The ca-menthol-smoking series is the corpus's first difference-in-differences
// mark: Canada's staggered provincial menthol-cigarette bans (2015-2017) vs the
// provinces covered only by the federal ban (2 Oct 2017), on adult current-smoking
// prevalence. Treated = the seven provinces that banned menthol provincially
// (NS/PE/AB 2015, NB/QC 2016, ON/NL 2017); control = BC, MB, SK (federal-only).
// The pre-period (2007-2014) spans the 2015 CCHS redesign, which is a level shock
// common to all provinces and so is differenced out; the post window is 2016-2017
// (before the federal ban universalises treatment); 2015 is dropped as the rollout
// ramp. It is the first anchor on the "menthol/flavour restriction -> smoking"
// mechanism — the cross-country bridge partner for US comprehensive flavour bans.

const (
	caMentholMarkID      = "ca-menthol-smoking-2016"
	caMentholMechanismID = "menthol-restriction-to-smoking"

	// Split point for the DiD: years strictly before are pre, on/after are post. The
	// panel excludes 2015 (rollout ramp: NS/PE/AB banned mid-2015) and 2018-2019
	// (after the federal Oct-2017 ban treats the control provinces too), so pre =
	// 2007-2014 and post = 2016-2017.
	caTreatSplit = 2015.0
	caRefWindow  = 3.0
)

var (
	caWindows = []float64{2, 3, 4, 5}

	// Provincial menthol-ban effective YEAR (Canada Gazette / provincial legislation);
	// provinces absent here (BC, MB, SK) were covered only by the federal 2 Oct 2017
	// ban and form the control group.
	caBanYear = map[string]int{
		"NS": 2015, "PE": 2015, "AB": 2015, "NB": 2016, "QC": 2016, "ON": 2017, "NL": 2017,
	}
	caProvinceName = map[string]string{
		"NL": "Newfoundland and Labrador", "PE": "Prince Edward Island", "NS": "Nova Scotia",
		"NB": "New Brunswick", "QC": "Quebec", "ON": "Ontario", "MB": "Manitoba",
		"SK": "Saskatchewan", "AB": "Alberta", "BC": "British Columbia",
	}
)

var caMentholEpisodeColumns = []string{
	"province", "province_name", "year", "treated", "ban_year", "is_post",
	"smoking_pct", "smoking_lo", "smoking_hi",
}

// BuildCAMenthol mints the Canadian menthol-ban DiD mark.
func BuildCAMenthol(rawDir, cacheDir, distDir string, cfg publish.Config) (schema.Mark, error) {
	var zero schema.Mark

	src, err := ingest.LoadSource(filepath.Join(rawDir, "ca-menthol-smoking", "SOURCE.json"))
	if err != nil {
		return zero, err
	}
	path := src.CachePath(cacheDir)
	if err := ingest.VerifySHA(path, src.SHA256); err != nil {
		return zero, fmt.Errorf("%w (run `python3 scripts/menthol_harvest.py`)", err)
	}
	panel, err := ingest.LoadSmokingPanel(path)
	if err != nil {
		return zero, err
	}

	// Build one DiD unit per province; exclude 2015 (ramp) and 2018+ (post-federal).
	byProv := map[string]*did.Unit{}
	var allObs []ingest.SmokingObs
	for _, o := range panel {
		allObs = append(allObs, o)
		if o.Year == 2015 || o.Year >= 2018 {
			continue
		}
		u := byProv[o.Province]
		if u == nil {
			_, treated := caBanYear[o.Province]
			u = &did.Unit{ID: o.Province, Treated: treated}
			byProv[o.Province] = u
		}
		u.Times = append(u.Times, float64(o.Year))
		u.Y = append(u.Y, o.Pct)
	}
	codes := sortedKeys(byProv)
	units := make([]did.Unit, 0, len(codes))
	for _, c := range codes {
		units = append(units, *byProv[c])
	}
	if len(units) < 6 {
		return zero, fmt.Errorf("ca-menthol: too few provinces (%d) to estimate", len(units))
	}

	// --- difference-in-differences with the window sweep folded into the interval.
	res := did.Estimate(units, caTreatSplit, caRefWindow, caWindows)
	lo, hi := res.Interval95()
	effect := schema.Distribution{
		Central:           res.Central,
		StdDev:            f64ptr(res.TotalSD),
		Interval:          &schema.Interval{Level: 0.95, Lower: lo, Upper: hi},
		Quantiles:         gaussianQuantiles(res.Central, res.TotalSD, posteriorQuantileGrid),
		Samples:           gaussianSamples(res.Central, res.TotalSD, postSamples),
		UncertaintyBudget: &schema.UncertaintyBudget{Sampling: f64ptr(res.SamplingVar), Specification: f64ptr(res.SpecVar)},
	}

	// --- validity battery.
	checks, admitted := caMentholChecks(units, res)

	// Stage panel episode rows (province × year).
	if _, err := publish.WriteEpisodesCSVGz(distDir, caMentholMarkID, "episodes.csv.gz",
		caMentholEpisodeColumns, caMentholEpisodeRows(allObs)); err != nil {
		return zero, err
	}

	parallelPass := checks[0].Passed
	placeboPass := true
	for _, p := range checks {
		if p.Name == "placebo_pre_period_ban" {
			placeboPass = p.Passed
		}
	}
	notes := fmt.Sprintf(
		"Difference-in-differences (unit-clustered, window sweep folded into the interval): %d treated provinces with a provincial menthol ban (NS/PE/AB 2015, NB/QC 2016, ON/NL 2017) vs %d control provinces covered only by the federal 2 Oct 2017 ban (BC, MB, SK). "+
			"ATT = %.3f pp on adult current-smoking prevalence, honest 95%% interval [%.3f, %.3f] (sd %.3f = sampling %.3f + specification %.3f). "+
			"Pre-period 2007-2014 (spans and differences out the 2015 CCHS redesign), post window 2016-2017 (before the federal ban universalises treatment); 2015 dropped as the rollout ramp. "+
			"Checks: parallel pre-trends %s (cross-group pre-slope %.3f), placebo pre-period ban %s. "+
			"CAVEATS: the federal Oct-2017 ban caps the clean post window at 2017; the effect is on TOTAL smoking, diluted by substitution to non-menthol products, so it is smaller than the menthol-specific reductions in the literature; with only 3 control provinces the unit-clustered interval is wide.",
		res.NTreat, res.NControl,
		res.Central, lo, hi, res.TotalSD, math.Sqrt(res.SamplingVar), math.Sqrt(res.SpecVar),
		passFail(parallelPass), res.PreTrendSlope, passFail(placeboPass))

	dossier := schema.ValidityDossier{
		PlaceboCutoffs:     caPlacebo(units),
		BandwidthSweep:     didWindowSweep(res),
		SeamSpecificChecks: checks,
		Admitted:           admitted,
		Notes:              notes,
	}

	mark := schema.Mark{
		SchemaVersion:  schema.SchemaVersion,
		MechanismID:    caMentholMechanismID,
		Category:       schema.CategoryIdentified,
		TruthSource:    schema.TruthIdentified,
		ID:             caMentholMarkID,
		Series:         schema.SeriesCAMenthol,
		Domain:         "Health",
		UnitType:       "province",
		Identification: schema.IDDiD,
		RDDType:        schema.DiD,
		RowShape:       schema.RowPanel,
		Design: schema.Design{
			RunningVariable: schema.Variable{
				Name: "year", Description: "Calendar year (the DiD time axis)", Units: "year", SourceID: src.SourceID,
			},
			Action:      "Provincial menthol-cigarette ban (2015-2017): the province prohibited the sale of menthol-flavoured cigarettes ahead of the federal ban.",
			Alternative: "No provincial menthol ban — menthol cigarettes remained legal until the federal ban took effect on 2 Oct 2017.",
			Outcome: schema.Variable{
				Name: "current_smoking_prevalence", Description: "Adult (12+) current-smoker (daily or occasional) prevalence, CCHS",
				Units: "percent", SourceID: src.SourceID,
			},
			Estimand: "ATT of a provincial menthol-cigarette ban on adult current-smoking prevalence (treated provinces vs federal-only control provinces, 2007-2014 pre vs 2016-2017 post), under parallel trends.",
		},
		Context: schema.Context{
			Description:    "The ten Canadian provinces, with adult current-smoking prevalence from the Canadian Community Health Survey, split into provinces that banned menthol provincially (treated) and those covered only by the federal 2 Oct 2017 ban (control).",
			CovariateNames: []string{"smoking_lo", "smoking_hi"},
			Population:     fmt.Sprintf("%d Canadian provinces (%d treated, %d control) over %d analysis years", len(units), res.NTreat, res.NControl, countYears(allObs)),
		},
		PanelSample: caMentholPanelSample(allObs),
		Effect:      effect,
		Dossier:     dossier,
		Provenance: schema.Provenance{
			Sources:                []schema.Source{toSchemaSource(src)},
			ContextAsOf:            "2014-12-31", // pre-period closes before the first 2015 ban
			DecisionTimestamp:      "2015-05-31", // first provincial ban (Nova Scotia)
			OutcomeTimestamp:       "2017-12-31", // post window closes
			RunningVariableVintage: "CCHS current-smoking prevalence by province-year (StatCan 13-10-0451 + 13-10-0096), stitched across the 2015 redesign",
			DecisionRound:          "Canadian provincial menthol bans, 2015-2017 (staggered)",
			ToolVersions: map[string]string{
				"go":                 runtime.Version(),
				"openaction2outcome": schema.SchemaVersion,
				"did":                fmt.Sprintf("unit-clustered-2x2,split=%g,windows=%v", caTreatSplit, caWindows),
			},
			OutcomeRealized: true,
		},
	}
	if err := mark.Validate(); err != nil {
		return zero, fmt.Errorf("minted mark failed validation: %w", err)
	}
	return mark, nil
}

func sortedKeys(m map[string]*did.Unit) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func countYears(obs []ingest.SmokingObs) int {
	ys := map[int]bool{}
	for _, o := range obs {
		if o.Year != 2015 && o.Year < 2018 {
			ys[o.Year] = true
		}
	}
	return len(ys)
}
