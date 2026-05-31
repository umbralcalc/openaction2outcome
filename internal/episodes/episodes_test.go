package episodes

import (
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// stageMark writes a synthetic episode table under distDir and returns a mark
// whose rows can be recovered by LoadTable (keyed only on the mark id).
func stageMark(t *testing.T, distDir, id string, series schema.Series, header []string, rows [][]string) schema.Mark {
	t.Helper()
	if _, err := publish.WriteEpisodesCSVGz(distDir, id, stagedTableName, header, rows); err != nil {
		t.Fatalf("stage episode table: %v", err)
	}
	sd := 0.12
	return schema.Mark{
		SchemaVersion: schema.SchemaVersion,
		ID:            id,
		Series:        series,
		Design: schema.Design{
			Cutoff:      -0.5,
			Direction:   schema.BelowTreated,
			Action:      "flagged",
			Alternative: "not flagged",
		},
		Context: schema.Context{CovariateNames: []string{"cov_a", "cov_b"}},
		Effect: schema.Distribution{
			Central:  0.08,
			StdDev:   &sd,
			Interval: &schema.Interval{Level: 0.95, Lower: -0.1, Upper: 0.3},
		},
	}
}

var testHeader = []string{"unit_id", "unit_name", "running_value", "assigned", "treated", "outcome", "cov_a", "cov_b"}

func TestBuildMapping(t *testing.T) {
	dist := t.TempDir()
	rows := [][]string{
		{"u1", "Unit One", "-0.8", "true", "true", "0.2", "1.5", "10"},
		{"u2", "Unit Two", "-0.2", "false", "false", "", "2.5", ""},
	}
	m := stageMark(t, dist, "m1", schema.SeriesFloorStandards, testHeader, rows)

	rs, err := Build([]schema.Mark{m}, dist)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(rs) != 2 {
		t.Fatalf("got %d rows, want 2", len(rs))
	}
	r1, r2 := rs[0], rs[1] // sorted by (series, mark_id, unit_id)

	if r1.UnitID != "u1" || r1.MarkID != "m1" || r1.Series != "floor-standards" {
		t.Errorf("r1 identity wrong: %+v", r1)
	}
	if got := r1.DistanceToCutoff; got < -0.3001 || got > -0.2999 { // -0.8 - (-0.5)
		t.Errorf("r1 distance_to_cutoff = %v, want -0.3", got)
	}
	if !r1.Assigned || r1.Treated == nil || !*r1.Treated {
		t.Errorf("r1 action wrong: assigned=%v treated=%v", r1.Assigned, r1.Treated)
	}
	if r1.Action != "flagged" || r1.Alternative != "not flagged" {
		t.Errorf("r1 action labels wrong: %q / %q", r1.Action, r1.Alternative)
	}
	if !r1.OutcomeObserved || r1.Outcome == nil || *r1.Outcome != 0.2 {
		t.Errorf("r1 outcome wrong: observed=%v outcome=%v", r1.OutcomeObserved, r1.Outcome)
	}
	if len(r1.Covariates) != 2 || r1.Covariates[0].Name != "cov_a" || r1.Covariates[0].Value != 1.5 || r1.Covariates[1].Name != "cov_b" {
		t.Errorf("r1 covariates wrong (want key-sorted cov_a,cov_b): %+v", r1.Covariates)
	}
	if r1.EffectCentral != 0.08 || r1.EffectLower != -0.1 || r1.EffectUpper != 0.3 || r1.EffectIntervalLevel != 0.95 || r1.EffectStdDev != 0.12 {
		t.Errorf("r1 effect summary wrong: %+v", r1)
	}

	// u2: no outcome -> unobserved; cov_b absent -> only cov_a.
	if r2.OutcomeObserved || r2.Outcome != nil {
		t.Errorf("r2 should have no outcome: observed=%v outcome=%v", r2.OutcomeObserved, r2.Outcome)
	}
	if len(r2.Covariates) != 1 || r2.Covariates[0].Name != "cov_a" {
		t.Errorf("r2 covariates should be only cov_a: %+v", r2.Covariates)
	}
}

func TestCovariateKeysSubset(t *testing.T) {
	dist := t.TempDir()
	rows := [][]string{{"u1", "U", "-0.8", "true", "true", "0.2", "1.5", "10"}}
	m := stageMark(t, dist, "m1", schema.SeriesFloorStandards, testHeader, rows)
	rs, err := Build([]schema.Mark{m}, dist)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	allowed := map[string]bool{}
	for _, c := range m.Context.CovariateNames {
		allowed[c] = true
	}
	for _, row := range rs {
		if row.MarkID != "m1" {
			t.Errorf("unexpected mark_id %q", row.MarkID)
		}
		for _, e := range row.Covariates {
			if !allowed[e.Name] {
				t.Errorf("covariate %q not in mark covariate_names", e.Name)
			}
		}
	}
}

func TestWriteParquetDeterministic(t *testing.T) {
	dist := t.TempDir()
	rows := [][]string{
		{"u1", "U1", "-0.8", "true", "true", "0.2", "1.5", "10"},
		{"u2", "U2", "-0.2", "false", "false", "", "2.5", ""},
	}
	m := stageMark(t, dist, "m1", schema.SeriesFloorStandards, testHeader, rows)
	rs, err := Build([]schema.Mark{m}, dist)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wa1, err := WriteParquet(filepath.Join(t.TempDir(), "a.parquet"), rs)
	if err != nil {
		t.Fatalf("WriteParquet 1: %v", err)
	}
	wa2, err := WriteParquet(filepath.Join(t.TempDir(), "b.parquet"), rs)
	if err != nil {
		t.Fatalf("WriteParquet 2: %v", err)
	}
	if wa1.SHA256 != wa2.SHA256 {
		t.Errorf("parquet not byte-deterministic: %s != %s", wa1.SHA256, wa2.SHA256)
	}
}

func TestParquetRoundTrip(t *testing.T) {
	dist := t.TempDir()
	rows := [][]string{
		{"u1", "U1", "-0.8", "true", "true", "0.2", "1.5", "10"},
		{"u2", "U2", "-0.2", "false", "false", "", "2.5", ""},
	}
	m := stageMark(t, dist, "m1", schema.SeriesFloorStandards, testHeader, rows)
	rs, err := Build([]schema.Mark{m}, dist)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	p := filepath.Join(t.TempDir(), "e.parquet")
	if _, err := WriteParquet(p, rs); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}
	got, err := parquet.ReadFile[Row](p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != len(rs) {
		t.Fatalf("round-trip row count %d != %d", len(got), len(rs))
	}
	if got[0].UnitID != "u1" || got[0].Outcome == nil || *got[0].Outcome != 0.2 {
		t.Errorf("round-trip r0 wrong: %+v", got[0])
	}
	if len(got[0].Covariates) != 2 || got[0].Covariates[0].Name != "cov_a" {
		t.Errorf("round-trip covariates wrong: %+v", got[0].Covariates)
	}
	if got[1].Outcome != nil {
		t.Errorf("round-trip r1 outcome should be nil: %v", got[1].Outcome)
	}
}

func TestLoadTableMissing(t *testing.T) {
	m := schema.Mark{ID: "nope"}
	if _, _, err := LoadTable(m, t.TempDir()); err == nil {
		t.Fatal("expected not-staged error, got nil")
	}
}
