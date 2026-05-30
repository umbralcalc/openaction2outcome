package hfexport

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func f64(v float64) *float64 { return &v }

func demoMark(id string, series schema.Series) schema.Mark {
	return schema.Mark{
		SchemaVersion: schema.SchemaVersion, ID: id, Series: series, Domain: "Education",
		UnitType: "school", RDDType: schema.Sharp,
		Design: schema.Design{
			RunningVariable: schema.Variable{Name: "score", Description: "a score"},
			Cutoff:          -0.5, Direction: schema.BelowTreated, Action: "intervene", Alternative: "nothing",
			Outcome: schema.Variable{Name: "later", Description: "later score"}, Estimand: "RD effect",
		},
		Context: schema.Context{Description: "schools", CovariateNames: []string{"prior"}},
		Data:    schema.DataArtifact{URI: "https://x/e.csv.gz", SHA256: "abc", Rows: 100, Columns: []string{"unit_id"}},
		Effect: schema.Distribution{
			Central: 0.03, StdDev: f64(0.07),
			Interval:          &schema.Interval{Level: 0.95, Lower: -0.05, Upper: 0.26},
			Quantiles:         []schema.Quantile{{P: 0.5, Value: 0.03}},
			Samples:           []float64{0.0, 0.05},
			UncertaintyBudget: &schema.UncertaintyBudget{Sampling: f64(0.001), Specification: f64(0.004)},
		},
		Dossier:    schema.ValidityDossier{Admitted: true},
		Provenance: schema.Provenance{Sources: []schema.Source{{Title: "T", Publisher: "P", Licence: "OGL"}}, ContextAsOf: "2016-01-01", DecisionTimestamp: "2017-01-01", OutcomeTimestamp: "2019-01-01", OutcomeRealized: true},
	}
}

func TestToRecord(t *testing.T) {
	r := ToRecord(demoMark("m1", schema.SeriesFloorStandards))
	if r.ID != "m1" || r.Series != "floor-standards" || r.Cutoff != -0.5 {
		t.Fatalf("bad core fields: %+v", r)
	}
	if r.EffectLower != -0.05 || r.EffectUpper != 0.26 || r.EffectCentral != 0.03 {
		t.Errorf("effect interval not flattened: %+v", r)
	}
	// variance -> sd
	if r.SamplingSD < 0.03 || r.SamplingSD > 0.04 || r.IdentSD < 0.06 || r.IdentSD > 0.07 {
		t.Errorf("uncertainty split sds wrong: sampling=%v ident=%v", r.SamplingSD, r.IdentSD)
	}
	if r.MarkJSONPath != "marks/m1.json" || r.EpisodeURL != "https://x/e.csv.gz" {
		t.Errorf("links wrong: %+v", r)
	}
}

func TestExportWritesLoadableSplits(t *testing.T) {
	dir := t.TempDir()
	card := filepath.Join(dir, "card.md")
	os.WriteFile(card, []byte("# card\n"), 0o644)
	out := filepath.Join(dir, "hf")

	marks := []schema.Mark{demoMark("a", schema.SeriesFloorStandards), demoMark("b", schema.SeriesSHMI)}
	if err := Export(marks, out, card); err != nil {
		t.Fatal(err)
	}
	// README copied, split files named with underscores.
	for _, name := range []string{"README.md", "floor_standards.jsonl", "shmi.jsonl"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	// Each JSONL line is valid JSON with the expected id.
	f, _ := os.Open(filepath.Join(out, "floor_standards.jsonl"))
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("invalid jsonl line: %v", err)
		}
		if rec["id"] != "a" {
			t.Errorf("unexpected id %v", rec["id"])
		}
		n++
	}
	if n != 1 {
		t.Fatalf("expected 1 floor-standards record, got %d", n)
	}
}
