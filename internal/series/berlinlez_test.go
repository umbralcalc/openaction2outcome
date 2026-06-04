package series

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Offline integration test: mint the real Berlin Umweltzone stage-2 mark from the
// cached frozen EEA CSVs. Skips cleanly when the harvest output is not cached.
func TestBuildBerlinLEZRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")
	distDir := t.TempDir()

	for _, id := range []string{"berlin-lez-no2", "berlin-lez-meteo"} {
		src, err := ingest.LoadSource(filepath.Join(rawDir, id, "SOURCE.json"))
		if err != nil {
			t.Skipf("source %s not present yet (%v)", id, err)
		}
		if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
			t.Skipf("input %s not cached (%v); run scripts/berlin_lez_harvest.py to enable", id, err)
		}
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildBerlinLEZ(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesBerlinLEZ || m.EffectiveIdentification() != schema.IDITSControlled {
		t.Fatalf("unexpected series/identification: %s/%s", m.Series, m.EffectiveIdentification())
	}
	if m.EffectiveRowShape() != schema.RowPanel {
		t.Fatalf("ITS mark should have panel rows, got %s", m.EffectiveRowShape())
	}
	// A ban-type LEZ on its own mechanism (kept separate from the charge-type ULEZ).
	if m.MechanismID != "lez-ban-stringency-to-roadside-no2" {
		t.Fatalf("unexpected mechanism: %s", m.MechanismID)
	}
	if _, ok := schema.MechanismByID(m.MechanismID); !ok {
		t.Fatalf("mechanism %s not in registry", m.MechanismID)
	}
	// This event has no implementation ramp (the ban took effect on 1 Jan), so the
	// design must NOT carry a transition window.
	if m.Design.ITS == nil || m.Design.ITS.Transition != nil {
		t.Fatalf("ban with no ramp must have a nil transition window")
	}
	if m.Design.ITS.Control == nil {
		t.Fatal("ITS design requires a control series")
	}
	// The honest interval must fold in specification uncertainty.
	if m.Effect.UncertaintyBudget == nil || m.Effect.UncertaintyBudget.Specification == nil {
		t.Fatal("effect must decompose uncertainty into sampling + specification")
	}
	if m.Dossier.ITS == nil || len(m.Dossier.ITS.PlaceboDates) == 0 {
		t.Fatal("ITS dossier must carry the validity battery with placebo dates")
	}
	// Effect sign/size is information, not an admission criterion (LEZ→NO2 is known
	// to be a small/near-null effect); only logged.
	t.Logf("Berlin LEZ→NO2 effect = %.3f µg/m³ [%.3f, %.3f], admitted=%v",
		m.Effect.Central, m.Effect.Interval.Lower, m.Effect.Interval.Upper, m.Dossier.Admitted)
}
