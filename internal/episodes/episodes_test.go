package episodes

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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

func TestCopyToHF(t *testing.T) {
	dist := t.TempDir()
	m := stageMark(t, dist, "m1", schema.SeriesFloorStandards, testHeader, [][]string{
		{"u1", "U1", "-0.8", "true", "true", "0.2", "1.5", "10"},
	})

	hf := t.TempDir()
	written, err := CopyToHF([]schema.Mark{m}, dist, hf)
	if err != nil {
		t.Fatalf("CopyToHF: %v", err)
	}
	if len(written) != 1 || written[0] != filepath.Join("episodes", "m1.csv.gz") {
		t.Fatalf("written = %v, want [episodes/m1.csv.gz]", written)
	}
	// The HF copy must be byte-identical to the staged object-storage file —
	// same schema, same bytes, no unioned re-encoding.
	src, err := os.ReadFile(filepath.Join(dist, "marks", "m1", stagedTableName))
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.ReadFile(filepath.Join(hf, "episodes", "m1.csv.gz"))
	if err != nil {
		t.Fatalf("read HF copy: %v", err)
	}
	if !bytes.Equal(src, dst) {
		t.Error("HF episodes copy differs from the staged object-storage file")
	}
}

func TestLoadTableMissing(t *testing.T) {
	m := schema.Mark{ID: "nope"}
	if _, _, err := LoadTable(m, t.TempDir()); err == nil {
		t.Fatal("expected not-staged error, got nil")
	}
}

func TestNewManifestPerMark(t *testing.T) {
	dist := t.TempDir()
	m1 := stageMark(t, dist, "m1", schema.SeriesFloorStandards, testHeader, [][]string{
		{"u1", "Unit One", "-0.8", "true", "true", "0.2", "1.5", "10"},
		{"u2", "Unit Two", "-0.2", "false", "false", "", "2.5", "20"},
	})
	m2 := stageMark(t, dist, "m2", schema.SeriesSHMI, testHeader, [][]string{
		{"u3", "Unit Three", "-0.4", "false", "false", "0.5", "3.5", "30"},
	})

	cfg := publish.Config{BaseURL: "https://ex.test/", MarksPrefix: "marks"}
	mf, err := NewManifest([]schema.Mark{m1, m2}, dist, cfg)
	if err != nil {
		t.Fatalf("NewManifest: %v", err)
	}
	if mf.Format != "csv.gz" {
		t.Errorf("format = %q, want csv.gz", mf.Format)
	}
	if mf.TotalRows != 3 {
		t.Errorf("total rows = %d, want 3", mf.TotalRows)
	}
	if len(mf.Marks) != 2 {
		t.Fatalf("got %d mark artifacts, want 2", len(mf.Marks))
	}
	a := mf.Marks[0] // sorted by mark_id
	if a.MarkID != "m1" || a.Rows != 2 {
		t.Errorf("artifact[0] = %+v, want m1 with 2 rows", a)
	}
	if a.URI != "https://ex.test/marks/m1/episodes.csv.gz" {
		t.Errorf("artifact[0] uri = %q", a.URI)
	}
	if len(a.SHA256) != 64 || a.Bytes == 0 {
		t.Errorf("artifact[0] must carry a sha256 + size, got sha=%q bytes=%d", a.SHA256, a.Bytes)
	}
	if got := a.Covariates; len(got) != 2 || got[0] != "cov_a" || got[1] != "cov_b" {
		t.Errorf("artifact[0] covariates = %v, want [cov_a cov_b]", got)
	}
}
