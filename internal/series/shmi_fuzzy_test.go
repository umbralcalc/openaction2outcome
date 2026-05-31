package series

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Integration test of the real fuzzy SHMI pipeline. Offline — skips unless both
// the SHMI and CQC inputs are cached. It asserts the genuine behaviour: a fuzzy
// design that carries a first-stage result and is NOT admitted, because the
// CQC-instrumented first stage at the cutoff is too weak (the documented result).
func TestBuildSHMIFuzzyRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")

	for _, id := range []string{"shmi", "cqc"} {
		src, err := ingest.LoadSource(filepath.Join(rawDir, id, "SOURCE.json"))
		if err != nil {
			t.Fatalf("load source %s: %v", id, err)
		}
		if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
			t.Skipf("%s input not cached (%v); run `openaction2outcome fetch`", id, err)
		}
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	distDir := t.TempDir()
	m, err := BuildSHMIFuzzy(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.RDDType != schema.Fuzzy || m.Design.Direction != schema.AboveTreated {
		t.Fatalf("expected a fuzzy above-treated design, got %s/%s", m.RDDType, m.Design.Direction)
	}
	if m.Dossier.FirstStage == nil {
		t.Fatal("a fuzzy mark must carry a first-stage result")
	}
	// The documented finding: the first stage is too weak to admit.
	if m.Dossier.FirstStage.Passed {
		t.Errorf("first stage F=%.2f unexpectedly passed the gate", deref(m.Dossier.FirstStage.FStat))
	}
	if m.Dossier.Admitted {
		t.Error("the fuzzy SHMI mark is not expected to be admitted")
	}
	if m.Effect.Interval == nil {
		t.Error("the LATE must still carry an interval")
	}
	// Rows are staged as a build intermediate even for an unadmitted mark.
	fi, err := os.Stat(filepath.Join(distDir, "marks", m.ID, "episodes.csv.gz"))
	if err != nil {
		t.Fatalf("episode rows not staged: %v", err)
	}
	if fi.Size() < 1024 {
		t.Errorf("staged episode rows unexpectedly small: %d bytes", fi.Size())
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
