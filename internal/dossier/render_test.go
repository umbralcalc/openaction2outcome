package dossier

import (
	"strings"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func f64(v float64) *float64 { return &v }

func sampleMark() schema.Mark {
	return schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            "demo-mark",
		Series:        schema.SeriesFloorStandards,
		Domain:        "Education",
		UnitType:      "school",
		RDDType:       schema.Sharp,
		Design: schema.Design{
			RunningVariable: schema.Variable{Name: "score", Description: "performance score", Units: "pts"},
			Cutoff:          -0.5,
			Direction:       schema.BelowTreated,
			Action:          "flagged for intervention",
			Alternative:     "no intervention",
			Outcome:         schema.Variable{Name: "score_later", Description: "score two years later", Units: "pts"},
			Estimand:        "sharp RD effect at the cutoff",
		},
		Context: schema.Context{Description: "schools", CovariateNames: []string{"prior_attainment"}},
		Data: schema.DataArtifact{
			URI: "https://example/episodes.csv.gz", SHA256: "deadbeef", Format: "csv.gz",
			Rows: 3098, Columns: []string{"unit_id", "running_value", "outcome"},
		},
		Effect: schema.Distribution{
			Central:  0.03,
			StdDev:   f64(0.07),
			Interval: &schema.Interval{Level: 0.95, Lower: -0.05, Upper: 0.26},
			UncertaintyBudget: &schema.UncertaintyBudget{
				Sampling: f64(0.001), Specification: f64(0.004),
			},
		},
		Dossier: schema.ValidityDossier{
			Density: schema.TestResult{Method: "binned-density (McCrary-style)", PValue: f64(0.38), Passed: true},
			CovariateContinuity: []schema.NamedTestResult{
				{Name: "prior_attainment", TestResult: schema.TestResult{Statistic: f64(0.01), PValue: f64(0.6), Passed: true}},
			},
			PlaceboCutoffs:  []schema.PlaceboResult{{Cutoff: -1.3, Estimate: 0.002, Passed: true}},
			BandwidthSweep:  []schema.SweepPoint{{Param: 0.5, Estimate: 0.03}},
			DonutRobustness: []schema.SweepPoint{{Param: 0.05, Estimate: 0.028}},
			Admitted:        true,
			Notes:           "attrition caveat applies.",
		},
		Provenance: schema.Provenance{
			Sources: []schema.Source{{
				Title: "KS4 tables", Publisher: "DfE", Licence: "OGL v3.0", SHA256: "abc",
			}},
			ContextAsOf: "2016-08-25", DecisionTimestamp: "2017-01-19", OutcomeTimestamp: "2019-01-24",
			ToolVersions: map[string]string{"go": "go1.25", "stochadex": "v0"},
		},
	}
}

func TestRenderContainsKeySections(t *testing.T) {
	out := Render(sampleMark())
	for _, want := range []string{
		"# demo-mark",
		"ADMITTED",
		"## The decision",
		"## The effect",
		"## Validity checks",
		"Density / manipulation",
		"prior_attainment", // covariate row
		"Placebo cutoffs",  // placebo section
		"## Data",
		"deadbeef", // data hash
		"## Provenance",
		"context as-of",  // point-in-time line
		"OGL v3.0",       // source licence
		"identification", // uncertainty split label
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered dossier missing %q", want)
		}
	}
}

func TestRenderDeterministic(t *testing.T) {
	m := sampleMark()
	if Render(m) != Render(m) {
		t.Fatal("Render is not deterministic")
	}
}

func TestRenderNotAdmittedAndFuzzyFirstStage(t *testing.T) {
	m := sampleMark()
	m.Dossier.Admitted = false
	m.RDDType = schema.Fuzzy
	m.Dossier.FirstStage = &schema.FirstStageResult{Jump: 0.4, Passed: true}
	out := Render(m)
	if !strings.Contains(out, "NOT ADMITTED") {
		t.Error("expected NOT ADMITTED status")
	}
	if !strings.Contains(out, "First stage") {
		t.Error("expected the fuzzy first-stage section")
	}
}
