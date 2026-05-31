package series

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// repoDir resolves the repo root relative to this test file, so the integration
// test runs from any working directory.
func repoDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// Integration test: mint the real floor-standards mark from the cached KS4 data
// and assert the structural and validity invariants hold. The test is offline —
// it skips (rather than fetching) when the cache is not populated, so `go test`
// never reaches the network. Run `openaction2outcome fetch` first to enable it.
func TestBuildFloorStandardsRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")
	distDir := t.TempDir()

	// Skip cleanly if inputs are not cached (offline / fresh checkout).
	for _, id := range []string{"ks4-2015-2016", "ks4-2017-2018"} {
		src, err := ingest.LoadSource(filepath.Join(rawDir, id, "SOURCE.json"))
		if err != nil {
			t.Fatalf("load source %s: %v", id, err)
		}
		if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
			t.Skipf("input %s not cached (%v); run `openaction2outcome fetch` to enable this test", id, err)
		}
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildFloorStandards(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesFloorStandards || m.RDDType != schema.Sharp {
		t.Fatalf("unexpected series/type: %s/%s", m.Series, m.RDDType)
	}
	if m.Effect.Interval == nil {
		t.Fatal("mark must carry an honest interval")
	}
	// The honest interval must fold in specification (bandwidth) uncertainty.
	ub := m.Effect.UncertaintyBudget
	if ub == nil || ub.Sampling == nil || ub.Specification == nil || *ub.Specification <= 0 {
		t.Fatalf("expected a non-trivial specification uncertainty contribution, got %+v", ub)
	}
	// A real, non-degenerate covariate-continuity battery (no free passes).
	if len(m.Dossier.CovariateContinuity) == 0 {
		t.Fatal("expected covariate-continuity tests in the dossier")
	}
	for _, c := range m.Dossier.CovariateContinuity {
		if c.Statistic == nil {
			t.Fatalf("covariate %q has no statistic (degenerate/free-pass test)", c.Name)
		}
	}
	// The episode rows must be staged as a build intermediate (reshaped into the
	// episodes dataset at export; not published per-mark).
	fi, err := os.Stat(filepath.Join(distDir, "marks", m.ID, "episodes.csv.gz"))
	if err != nil {
		t.Fatalf("episode rows not staged: %v", err)
	}
	if fi.Size() < 1024 {
		t.Fatalf("staged episode rows unexpectedly small: %d bytes", fi.Size())
	}
	// The inline near-cutoff sample must be small and consistent.
	if len(m.Sample) == 0 || len(m.Sample) > sampleMaxRows {
		t.Fatalf("unexpected sample size: %d", len(m.Sample))
	}
}
