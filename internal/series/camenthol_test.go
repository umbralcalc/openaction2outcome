package series

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/did"
	"github.com/umbralcalc/openaction2outcome/internal/ingest"
	"github.com/umbralcalc/openaction2outcome/internal/publish"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func TestGaussianQuantilesAndSamples(t *testing.T) {
	// Median of N(mean, sd) is the mean; symmetric quantiles bracket it.
	if v := normalQuantile(0.5, -1.5, 0.8); math.Abs(v-(-1.5)) > 1e-9 {
		t.Errorf("median quantile = %v, want -1.5", v)
	}
	q := gaussianQuantiles(-1.5, 0.8, []float64{0.025, 0.5, 0.975})
	if !(q[0].Value < q[1].Value && q[1].Value < q[2].Value) {
		t.Errorf("quantiles not increasing: %v", q)
	}
	// 95% half-width ≈ 1.96*sd.
	if hw := q[2].Value - q[1].Value; math.Abs(hw-1.96*0.8) > 0.02 {
		t.Errorf("upper 95%% half-width = %v, want ~%v", hw, 1.96*0.8)
	}
	s := gaussianSamples(0, 1, 200)
	var mean float64
	for _, x := range s {
		mean += x
	}
	if math.Abs(mean/200) > 0.05 {
		t.Errorf("stratified samples mean = %v, want ~0", mean/200)
	}
}

func TestLeaveOneOutSignStable(t *testing.T) {
	// Two treated units that both fall, two control units flat → dropping either
	// treated keeps a negative ATT.
	units := []did.Unit{
		{ID: "T1", Treated: true, Times: []float64{2012, 2013, 2016, 2017}, Y: []float64{20, 20, 16, 16}},
		{ID: "T2", Treated: true, Times: []float64{2012, 2013, 2016, 2017}, Y: []float64{22, 22, 18, 18}},
		{ID: "C1", Treated: false, Times: []float64{2012, 2013, 2016, 2017}, Y: []float64{18, 18, 18, 18}},
		{ID: "C2", Treated: false, Times: []float64{2012, 2013, 2016, 2017}, Y: []float64{19, 19, 19, 19}},
	}
	loo := caLeaveOneOut(units, -4)
	if !loo.Passed {
		t.Errorf("expected sign-stable leave-one-out, got %s", loo.Detail)
	}
}

// Offline integration test: mint the real menthol DiD mark from the cached frozen
// panel. Skips cleanly when the harvest output is not cached.
func TestBuildCAMentholRealData(t *testing.T) {
	root := repoDir(t)
	rawDir := filepath.Join(root, "data", "raw")
	cacheDir := filepath.Join(root, "data", "cache")

	src, err := ingest.LoadSource(filepath.Join(rawDir, "ca-menthol-smoking", "SOURCE.json"))
	if err != nil {
		t.Skipf("source not present yet (%v)", err)
	}
	if _, err := os.Stat(src.CachePath(cacheDir)); err != nil {
		t.Skipf("input not cached (%v); run scripts/menthol_harvest.py to enable", err)
	}

	cfg := publish.Config{BaseURL: "https://example.test", MarksPrefix: "marks", RawPrefix: "raw"}
	m, err := BuildCAMenthol(rawDir, cacheDir, t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("minted mark invalid: %v", err)
	}
	if m.Series != schema.SeriesCAMenthol || m.EffectiveIdentification() != schema.IDDiD {
		t.Fatalf("unexpected series/identification: %s/%s", m.Series, m.EffectiveIdentification())
	}
	if m.Effect.Interval == nil || m.Effect.UncertaintyBudget == nil || m.Effect.UncertaintyBudget.Specification == nil {
		t.Fatal("DiD effect must carry an honest interval with a sampling+specification budget")
	}
	// The validity battery must carry the DiD checks.
	names := map[string]bool{}
	for _, c := range m.Dossier.SeamSpecificChecks {
		names[c.Name] = true
	}
	for _, want := range []string{"parallel_trends", "placebo_pre_period_ban", "leave_one_province_out"} {
		if !names[want] {
			t.Errorf("dossier missing DiD check %q", want)
		}
	}
	t.Logf("CA menthol DiD ATT = %.3f pp [%.3f, %.3f], admitted=%v",
		m.Effect.Central, m.Effect.Interval.Lower, m.Effect.Interval.Upper, m.Dossier.Admitted)
}
