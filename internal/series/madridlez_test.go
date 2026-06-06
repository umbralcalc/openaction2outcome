package series

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Offline integration test: mint the real Madrid Central LEZ mark from the cached
// frozen EEA CSVs. Skips cleanly when the harvest output is not cached.
func TestBuildMadridLEZRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")
	distDir := t.TempDir()

	for _, id := range []string{"madrid-lez-no2", "madrid-lez-meteo"} {
		src, err := ingest.LoadSource(filepath.Join(rawDir, id, "SOURCE.json"))
		if err != nil {
			t.Skipf("source %s not present yet (%v)", id, err)
		}
		if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
			t.Skipf("input %s not cached (%v); run scripts/madrid_lez_harvest.py to enable", id, err)
		}
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildMadridLEZ(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesMadridLEZ || m.EffectiveIdentification() != schema.IDITSControlled {
		t.Fatalf("unexpected series/identification: %s/%s", m.Series, m.EffectiveIdentification())
	}
	if m.EffectiveRowShape() != schema.RowPanel {
		t.Fatalf("ITS mark should have panel rows, got %s", m.EffectiveRowShape())
	}
	// Madrid Central is an access-BAN LEZ on the SAME mechanism as the Berlin
	// Umweltzone — the second anchor that the LEZ→NO2 bridge needs.
	if m.MechanismID != "lez-ban-stringency-to-roadside-no2" {
		t.Fatalf("unexpected mechanism: %s", m.MechanismID)
	}
	if _, ok := schema.MechanismByID(m.MechanismID); !ok {
		t.Fatalf("mechanism %s not in registry", m.MechanismID)
	}
	// The LEZ took effect on 30 Nov 2018 with December the first full post month, so
	// the design carries no implementation-ramp transition window.
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
	// A single in-zone monitor (Plaza del Carmen) drives the treated aggregate.
	if !strings.HasPrefix(m.Context.Population, "1 treated") {
		t.Fatalf("expected a single treated station, got population %q", m.Context.Population)
	}
	// Effect sign/size is information, not an admission criterion; only logged.
	t.Logf("Madrid Central LEZ→NO2 effect = %.3f µg/m³ [%.3f, %.3f], admitted=%v",
		m.Effect.Central, m.Effect.Interval.Lower, m.Effect.Interval.Upper, m.Dossier.Admitted)
}
