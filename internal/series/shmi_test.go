package series

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Integration test: mint the SHMI mark from the cached data. Offline — it skips
// (rather than fetching) when the input is not cached. Run `fetch` to enable it.
func TestBuildSHMIRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")
	distDir := t.TempDir()

	src, err := ingest.LoadSource(filepath.Join(rawDir, "shmi", "SOURCE.json"))
	if err != nil {
		t.Fatalf("load shmi source: %v", err)
	}
	if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
		t.Skipf("SHMI input not cached (%v); run `openaction2outcome fetch` to enable", err)
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildSHMI(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesSHMI || m.RDDType != schema.Sharp || m.Design.Direction != schema.AboveTreated {
		t.Fatalf("unexpected design: %s/%s/%s", m.Series, m.RDDType, m.Design.Direction)
	}
	if m.Effect.Interval == nil {
		t.Fatal("mark must carry an interval")
	}
	ub := m.Effect.UncertaintyBudget
	if ub == nil || ub.Specification == nil || *ub.Specification <= 0 {
		t.Fatalf("expected a non-trivial identification component, got %+v", ub)
	}
	// Pooled trust-years: a healthy number of episodes and a real near-cutoff sample.
	if m.Data.Rows < 200 {
		t.Fatalf("unexpectedly few pooled trust-years: %d", m.Data.Rows)
	}
	if len(m.Sample) == 0 {
		t.Fatal("expected a near-cutoff sample")
	}
	if _, err := os.Stat(filepath.Join(distDir, "marks", m.ID, "episodes.csv.gz")); err != nil {
		t.Fatalf("episode sidecar not staged: %v", err)
	}
}
