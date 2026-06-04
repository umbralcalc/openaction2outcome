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

func TestMonthToTRoundTrip(t *testing.T) {
	cases := map[string]int{"2021-01": 0, "2021-12": 11, "2022-01": 12, "2023-09": 32, "2023-08": 31}
	for m, want := range cases {
		got, ok := monthToT(m)
		if !ok || got != want {
			t.Errorf("monthToT(%q)=%d ok=%v, want %d", m, got, ok, want)
		}
		if back := tToMonth(want); back != m {
			t.Errorf("tToMonth(%d)=%q, want %q", want, back, m)
		}
	}
	if _, ok := monthToT("2021-13"); ok {
		t.Error("invalid month should fail")
	}
}

func TestPearsonAndMonthlyMean(t *testing.T) {
	if r := pearson([]float64{1, 2, 3}, []float64{2, 4, 6}); math.Abs(r-1) > 1e-9 {
		t.Errorf("perfect positive correlation should be 1, got %v", r)
	}
	if r := pearson([]float64{1, 2, 3}, []float64{6, 4, 2}); math.Abs(r+1) > 1e-9 {
		t.Errorf("perfect negative correlation should be -1, got %v", r)
	}
	stations := map[string]*ulezStation{
		"A": {code: "A", byT: map[int]float64{0: 10, 1: 20}},
		"B": {code: "B", byT: map[int]float64{0: 20, 1: 40}},
		"C": {code: "C", byT: map[int]float64{0: 30}}, // only month 0
	}
	mm := monthlyMean(stations, 2)
	if math.Abs(mm[0]-20) > 1e-9 { // (10+20+30)/3
		t.Errorf("month 0 mean = %v, want 20", mm[0])
	}
	if _, ok := mm[1]; !ok || math.Abs(mm[1]-30) > 1e-9 { // (20+40)/2
		t.Errorf("month 1 mean = %v, want 30", mm[1])
	}
	// With minN=3, month 1 (only 2 stations) drops out.
	mm3 := monthlyMean(stations, 3)
	if _, ok := mm3[1]; ok {
		t.Error("month 1 should drop with minN=3")
	}
}

func TestSelectParallelControlsDropsAnomalous(t *testing.T) {
	// Treated aggregate trending down over 8 pre months.
	treated := map[int]float64{}
	for tt := 12; tt <= 19; tt++ {
		treated[tt] = 50 - float64(tt-12)
	}
	parallel := &ulezStation{code: "P", byT: map[int]float64{}}
	anti := &ulezStation{code: "X", byT: map[int]float64{}}
	for tt := 12; tt <= 19; tt++ {
		parallel.byT[tt] = 30 - float64(tt-12) // same downward trend
		anti.byT[tt] = 30 + float64(tt-12)     // opposite trend
	}
	kept, dropped := selectParallelControls(
		map[string]*ulezStation{"P": parallel, "X": anti}, treated, 12, 19, 0.5)
	if _, ok := kept["P"]; !ok {
		t.Error("parallel control should be kept")
	}
	if _, ok := kept["X"]; ok {
		t.Error("anti-correlated control should be dropped")
	}
	if len(dropped) != 1 {
		t.Errorf("expected 1 dropped control, got %v", dropped)
	}
}

// Offline integration test: mint the real ULEZ→NO2 mark from the cached frozen
// CSVs. Skips cleanly when the harvest output is not cached.
func TestBuildULEZNO2RealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")
	distDir := t.TempDir()

	for _, id := range []string{"ulez-no2-laqn-2019", "ulez-no2-meteo-2019"} {
		src, err := ingest.LoadSource(filepath.Join(rawDir, id, "SOURCE.json"))
		if err != nil {
			t.Skipf("source %s not present yet (%v)", id, err)
		}
		if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
			t.Skipf("input %s not cached (%v); run scripts/ulez_harvest.py --event 2019 to enable", id, err)
		}
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildULEZNO2(rawDir, cacheDir, distDir, cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesULEZNO2 || m.EffectiveIdentification() != schema.IDITSControlled {
		t.Fatalf("unexpected series/identification: %s/%s", m.Series, m.EffectiveIdentification())
	}
	if m.EffectiveRowShape() != schema.RowPanel {
		t.Fatalf("ITS mark should have panel rows, got %s", m.EffectiveRowShape())
	}
	if m.Design.ITS == nil || m.Design.ITS.Control == nil {
		t.Fatal("ITS design block with a control series is required")
	}
	if m.Effect.Interval == nil {
		t.Fatal("mark must carry an honest interval")
	}
	// The honest interval must fold in specification uncertainty.
	if m.Effect.UncertaintyBudget == nil || m.Effect.UncertaintyBudget.Specification == nil {
		t.Fatal("effect must decompose uncertainty into sampling + specification")
	}
	// The ITS validity battery must be present.
	if m.Dossier.ITS == nil {
		t.Fatal("dossier must carry the ITS validity battery")
	}
	if len(m.Dossier.ITS.PlaceboDates) == 0 || len(m.Dossier.ITS.WindowSweep) == 0 {
		t.Fatal("ITS dossier must carry placebo dates and a window sweep")
	}
	// The expected sign: roadside NO2 falls relative to background, so the effect
	// is negative (a reduction). It is information, not an admission criterion, so
	// only logged.
	t.Logf("ULEZ→NO2 effect = %.3f µg/m³ [%.3f, %.3f], admitted=%v",
		m.Effect.Central, m.Effect.Interval.Lower, m.Effect.Interval.Upper, m.Dossier.Admitted)
}
