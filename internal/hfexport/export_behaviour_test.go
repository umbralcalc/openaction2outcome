package hfexport

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// A record must carry everything needed to score a model against it: a central
// estimate, an interval, and the distribution (quantiles + samples).
func TestRecordHasEverythingToScore(t *testing.T) {
	r := ToRecord(demoMark("m", schema.SeriesFloorStandards))
	if !(r.EffectLower < r.EffectCentral && r.EffectCentral < r.EffectUpper) {
		t.Fatalf("central must sit inside the interval: %v in [%v, %v]", r.EffectCentral, r.EffectLower, r.EffectUpper)
	}
	if r.EffectLevel <= 0 || r.EffectLevel >= 1 {
		t.Errorf("interval level should be a coverage in (0,1): %v", r.EffectLevel)
	}
	if len(r.Quantiles) == 0 || len(r.Samples) == 0 {
		t.Errorf("the distribution (quantiles + samples) must be carried for scoring")
	}
}

// The uncertainty split is reported as standard deviations (sqrt of the variance
// components stored on the mark).
func TestRecordUncertaintySplitIsSqrtOfVariance(t *testing.T) {
	// demoMark stores sampling variance 0.001 and specification variance 0.004.
	r := ToRecord(demoMark("m", schema.SeriesFloorStandards))
	if math.Abs(r.SamplingSD-math.Sqrt(0.001)) > 1e-9 {
		t.Errorf("sampling sd %.6f != sqrt(0.001)", r.SamplingSD)
	}
	if math.Abs(r.IdentSD-math.Sqrt(0.004)) > 1e-9 {
		t.Errorf("identification sd %.6f != sqrt(0.004)", r.IdentSD)
	}
}

// Export routes each mark to its series file (config), not a shared split.
func TestExportRoutesBySeries(t *testing.T) {
	dir := t.TempDir()
	card := filepath.Join(dir, "c.md")
	os.WriteFile(card, []byte("# c\n"), 0o644)
	out := filepath.Join(dir, "hf")
	marks := []schema.Mark{
		demoMark("a", schema.SeriesFloorStandards),
		demoMark("b", schema.SeriesFloorStandards),
		demoMark("c", schema.SeriesSHMI),
	}
	if err := Export(marks, out, card); err != nil {
		t.Fatal(err)
	}
	if n := countLines(t, filepath.Join(out, "floor_standards.jsonl")); n != 2 {
		t.Errorf("floor-standards config should have 2 records, got %d", n)
	}
	if n := countLines(t, filepath.Join(out, "shmi.jsonl")); n != 1 {
		t.Errorf("shmi config should have 1 record, got %d", n)
	}
}

// A mark missing optional fields (no quantiles/samples/uncertainty budget/sources)
// must still flatten and export to valid JSON, not panic.
func TestRecordRobustToMissingOptionalFields(t *testing.T) {
	bare := schema.Mark{
		SchemaVersion: schema.SchemaVersion, ID: "bare", Series: schema.SeriesSHMI,
		Domain: "Health", UnitType: "nhs-trust", RDDType: schema.Sharp,
		Design: schema.Design{Direction: schema.AboveTreated, Cutoff: 0},
		Effect: schema.Distribution{Central: 0.1, Interval: &schema.Interval{Level: 0.95, Lower: -0.1, Upper: 0.3}},
	}
	r := ToRecord(bare)
	if r.SamplingSD != 0 || r.IdentSD != 0 || len(r.Quantiles) != 0 || len(r.Samples) != 0 {
		t.Errorf("absent optional fields should be zero/empty, got %+v", r)
	}
	dir := t.TempDir()
	card := filepath.Join(dir, "c.md")
	os.WriteFile(card, []byte("#\n"), 0o644)
	out := filepath.Join(dir, "hf")
	if err := Export([]schema.Mark{bare}, out, card); err != nil {
		t.Fatalf("export of a minimal mark failed: %v", err)
	}
	if countLines(t, filepath.Join(out, "shmi.jsonl")) != 1 {
		t.Error("minimal mark should still produce one record")
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("%s: invalid JSON line: %v", path, err)
		}
		n++
	}
	return n
}
