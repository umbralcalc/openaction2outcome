package publish

import (
	"compress/gzip"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "publish.json")
	os.WriteFile(path, []byte(`{"base_url":"https://base.r2.dev","bucket":"b"}`), 0o644)
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.MarksPrefix != "marks" || c.RawPrefix != "raw" || c.DatasetsPrefix != "datasets" {
		t.Errorf("missing prefixes should default: %+v", c)
	}
}

func TestDatasetURLs(t *testing.T) {
	c := Config{BaseURL: "https://base.r2.dev/", Bucket: "b", DatasetsPrefix: "datasets"}
	if got := c.DatasetObjectKey("episodes.parquet"); got != "datasets/episodes.parquet" {
		t.Errorf("object key: %q", got)
	}
	if got := c.DatasetArtifactURL("episodes.parquet"); got != "https://base.r2.dev/datasets/episodes.parquet" {
		t.Errorf("artifact URL (trailing slash should be trimmed): %q", got)
	}
}

func TestWriteEpisodesDeterministicAndReadable(t *testing.T) {
	dir := t.TempDir()
	header := []string{"unit_id", "running_value", "outcome"}
	rows := [][]string{
		{"100", "0.3", "0.4"},
		{"103", "-0.6", ""},
	}

	a, err := WriteEpisodesCSVGz(dir, "m1", "episodes.csv.gz", header, rows)
	if err != nil {
		t.Fatal(err)
	}
	if a.Rows != 2 || a.Bytes <= 0 || a.SHA256 == "" {
		t.Fatalf("bad artifact metadata: %+v", a)
	}

	// Re-writing identical data must produce an identical hash (re-mintable).
	b, err := WriteEpisodesCSVGz(t.TempDir(), "m1", "episodes.csv.gz", header, rows)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != b.SHA256 {
		t.Fatalf("non-deterministic gzip: %s vs %s", a.SHA256, b.SHA256)
	}

	// Content round-trips.
	f, err := os.Open(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(gz).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0][0] != "unit_id" || recs[2][1] != "-0.6" {
		t.Fatalf("unexpected csv content: %v", recs)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing config")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o644)
	if _, err := LoadConfig(bad); err == nil {
		t.Fatal("expected decode error")
	}
}
