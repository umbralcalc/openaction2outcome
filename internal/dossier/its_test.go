package dossier

import (
	"strings"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func sampleITSMark() schema.Mark {
	m := sampleMark()
	m.ID = "demo-its"
	m.RDDType = ""
	m.Identification = schema.IDITSControlled
	m.Design = schema.Design{
		Action:      "minimum unit pricing switched on",
		Alternative: "no minimum unit price",
		Outcome:     schema.Variable{Name: "alcohol_deaths", Description: "alcohol-specific deaths", Units: "per 100k"},
		Estimand:    "population effect over the post window",
		ITS: &schema.ITSDesign{
			InterventionInstant: "2018-05-01",
			RunningTime:         schema.Variable{Name: "month", Units: "month"},
			PreWindow:           schema.Window{Start: "2014-01", End: "2018-04"},
			PostWindow:          schema.Window{Start: "2018-05", End: "2020-12"},
			Counterfactual:      schema.Counterfactual{Family: "segmented-regression", Seasonality: "monthly dummies", Justification: "linear pre-trend"},
			Control:             &schema.ControlSeries{SeriesID: "england", Role: "parallel-trend", Justification: "no MUP in England"},
		},
	}
	m.Dossier = schema.ValidityDossier{
		Admitted: true,
		ITS: &schema.ITSChecks{
			NoAnticipation:     schema.TestResult{Method: "pre-trend break test", PValue: f64(0.6), Passed: true},
			ControlParallelism: schema.TestResult{Method: "pre-trend interaction", PValue: f64(0.4), Passed: true},
			PlaceboDates:       []schema.DatePlaceboResult{{Date: "2016-05", Estimate: 0.01, Passed: true}},
			WindowSweep:        []schema.SweepPoint{{Param: 36, Estimate: -2.1}},
			DoseCheck:          &schema.FirstStageResult{Jump: 0.12, Passed: true},
			Autocorrelation:    schema.TestResult{Method: "Newey-West", Passed: true},
			Admitted:           true,
		},
	}
	return m
}

func TestRenderITSSections(t *testing.T) {
	out := Render(sampleITSMark())
	for _, want := range []string{
		"controlled interrupted time series",
		"population",
		"Intervention instant:",
		"Counterfactual:",
		"Control series:",
		"No anticipation",
		"Control parallelism",
		"Placebo dates",
		"Dose check",
		"panel",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ITS dossier missing %q\n---\n%s", want, out)
		}
	}
	// It must not render the RDD-only cutoff line.
	if strings.Contains(out, "Cutoff:") {
		t.Error("ITS dossier should not render an RDD cutoff line")
	}
}
