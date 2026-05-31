package series

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func TestComplianceMargin(t *testing.T) {
	// Coastal water exactly on the IE Sufficient threshold (185) and below on EC:
	// margin should be 0 (on the cutoff, i.e. just meets Sufficient).
	onEdge := ingest.BathingWater{WaterType: "coastal", ECp90: 250, ECp95: 400, IEp90: 185, IEp95: 200}
	if mv, ok := complianceMargin(onEdge); !ok || math.Abs(mv) > 1e-9 {
		t.Fatalf("on-threshold margin should be 0, got %v ok=%v", mv, ok)
	}

	// A clearly Poor coastal water (IE 90th pct well over 185): margin > 0.
	poor := ingest.BathingWater{WaterType: "coastal", ECp90: 600, ECp95: 900, IEp90: 370, IEp95: 600}
	if mv, ok := complianceMargin(poor); !ok || mv <= 0 {
		t.Fatalf("Poor water should have positive margin, got %v", mv)
	}
	// The worst indicator binds: IE 370/185 = 2x => log10(2) ~ 0.301, EC 600/500 ~ 0.079.
	mv, _ := complianceMargin(poor)
	if math.Abs(mv-math.Log10(2)) > 1e-6 {
		t.Fatalf("margin should be the worst (IE) indicator, got %v want %v", mv, math.Log10(2))
	}

	// Inland thresholds are looser, so the same counts give a smaller margin.
	inland := ingest.BathingWater{WaterType: "inland", ECp90: 600, ECp95: 900, IEp90: 370, IEp95: 600}
	mi, _ := complianceMargin(inland)
	if mi >= mv {
		t.Fatalf("inland margin (%v) should be below coastal margin (%v) for identical counts", mi, mv)
	}

	// Missing percentiles => not usable.
	if _, ok := complianceMargin(ingest.BathingWater{WaterType: "coastal", ECp90: math.NaN(), IEp90: math.NaN()}); ok {
		t.Fatal("missing percentiles should report not-usable")
	}
}

func TestLogNormalP90AndReinclusion(t *testing.T) {
	// Too few samples => not usable.
	if _, ok := logNormalP90([]float64{10, 20}); ok {
		t.Fatal("fewer than 5 samples should be rejected")
	}
	// A spread of counts gives a 90th percentile above the geometric mean.
	counts := []float64{10, 20, 30, 40, 100, 200}
	p90, ok := logNormalP90(counts)
	if !ok {
		t.Fatal("expected a usable p90")
	}
	gm := 1.0
	// geometric mean for reference
	sum := 0.0
	for _, c := range counts {
		sum += math.Log10(c)
	}
	gm = math.Pow(10, sum/float64(len(counts)))
	if p90 <= gm {
		t.Fatalf("p90 (%v) should exceed the geometric mean (%v)", p90, gm)
	}

	// Re-including a high (discountable) spike raises the margin.
	clean := []float64{10, 20, 30, 40, 50, 60}
	withSpike := append(append([]float64(nil), clean...), 5000, 8000)
	mClean, ok1 := reincludedMargin(clean, clean, ecThresholdCoastal, ieThresholdCoastal)
	mSpike, ok2 := reincludedMargin(withSpike, withSpike, ecThresholdCoastal, ieThresholdCoastal)
	if !ok1 || !ok2 {
		t.Fatal("expected usable margins")
	}
	if mSpike <= mClean {
		t.Fatalf("re-including high samples should raise the margin: clean=%v spike=%v", mClean, mSpike)
	}
}

// Offline integration test: mint the real bathing-water mark from the cached
// frozen CSVs. Skips cleanly when the harvest output is not cached.
func TestBuildBathingWaterRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")
	distDir := t.TempDir()

	for _, id := range []string{"bathing-water-rbwd", "bathing-water-samples"} {
		src, err := ingest.LoadSource(filepath.Join(rawDir, id, "SOURCE.json"))
		if err != nil {
			t.Skipf("source %s not present yet (%v)", id, err)
		}
		if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
			t.Skipf("input %s not cached (%v); run scripts/bathingwater_harvest.py to enable", id, err)
		}
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildBathingWater(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesBathingWater || m.RDDType != schema.Sharp {
		t.Fatalf("unexpected series/type: %s/%s", m.Series, m.RDDType)
	}
	if m.Design.Direction != schema.AboveTreated || m.Design.Cutoff != 0 {
		t.Fatalf("unexpected design: dir=%s cutoff=%v", m.Design.Direction, m.Design.Cutoff)
	}
	if m.Effect.Interval == nil {
		t.Fatal("mark must carry an honest interval")
	}
	// The honest interval must fold in specification uncertainty.
	if m.Effect.UncertaintyBudget == nil || m.Effect.UncertaintyBudget.Specification == nil {
		t.Fatal("effect must decompose uncertainty into sampling + specification")
	}
	// The seam-specific exclusion-sensitivity check must be present.
	if len(m.Dossier.SeamSpecificChecks) == 0 {
		t.Fatal("dossier must carry the abnormal-sample-exclusion sensitivity check")
	}
}
