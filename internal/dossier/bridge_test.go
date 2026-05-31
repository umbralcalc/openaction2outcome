package dossier

import (
	"strings"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func sampleBridgeMark() schema.Mark {
	sd := 0.3
	seed := int64(1)
	return schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            "synthetic-bridge-demo",
		Series:        schema.SeriesBathingWater,
		Domain:        "Synthetic",
		Category:      schema.CategoryBridge,
		TruthSource:   schema.TruthSimulatorBridged,
		Effect: schema.Distribution{
			Central:           0.4,
			StdDev:            &sd,
			Interval:          &schema.Interval{Level: 0.95, Lower: -0.2, Upper: 1.0},
			UncertaintyBudget: &schema.UncertaintyBudget{Sampling: f(0.04), Specification: f(0.05)},
		},
		Bridge: &schema.BridgeSpec{
			Mechanism:      "synthetic-mechanism",
			PolicyVariable: "intensity",
			QueryPoint:     0.0,
			Anchors: []schema.AnchorRef{
				{MarkID: "anchor-lo", PolicyPoint: -1},
				{MarkID: "anchor-hi", PolicyPoint: 1},
			},
			Simulator: schema.SimulatorRef{ModelID: "quadratic-synthetic", Version: "synthetic-1", Seed: &seed},
			Kernel:    schema.KernelSpec{Family: "squared-exponential", Params: map[string]float64{"sigma_f": 0.5, "lengthscale": 0.6}, Jitter: 1e-8},
			AnchorCoherence: schema.AnchorCoherence{
				SamePopulation: true, SameRegime: true, SameOutcomeConstruct: true,
				Justification: "anchors share the mechanism.",
			},
		},
		Dossier: schema.ValidityDossier{
			Bridge: &schema.BridgeChecks{
				Coherence:    schema.AnchorCoherence{SamePopulation: true, SameRegime: true, SameOutcomeConstruct: true, Justification: "anchors share the mechanism."},
				BracketingOK: true,
				LOAOCoverage: 1.0,
				LOAOLevel:    0.95,
				Admitted:     true,
			},
		},
	}
}

func f(v float64) *float64 { return &v }

func TestRenderBridgeDossier(t *testing.T) {
	m := sampleBridgeMark()
	out := Render(m)
	for _, want := range []string{
		"simulator-bridged",
		"NOT identified truth",
		"Pin/span picture",
		"trust-decay assumption",
		"squared-exponential",
		"Leave-one-anchor-out",
		"category == identified", // tells consumers how to filter bridges out
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("bridge dossier missing %q:\n%s", want, out)
		}
	}
	// The ASCII pin/span picture must mark the interpolated query.
	if !strings.Contains(out, "?") {
		t.Fatalf("pin/span picture should mark the query with '?':\n%s", out)
	}
}

func TestRenderIdentifiedStillWorks(t *testing.T) {
	m := schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            "id-demo",
		Series:        schema.SeriesFloorStandards,
		Domain:        "Education",
		UnitType:      "school",
		RDDType:       schema.Sharp,
		Effect:        schema.Distribution{Central: 0.1, Interval: &schema.Interval{Level: 0.95, Lower: 0, Upper: 0.2}},
		Dossier:       schema.ValidityDossier{Admitted: true},
	}
	out := Render(m)
	if !strings.Contains(out, "identified (design-based truth") {
		t.Fatalf("identified dossier should label its category:\n%s", out)
	}
}
