package sbi

import "testing"

// The headline must hold qualitatively: across synthetic problems with known
// truth, the plug-in's sampling-only intervals under-cover while SBI tracks
// nominal much more closely. Kept small/fast for CI.
func TestCalibrationGap(t *testing.T) {
	st := RunCalibrationStudy(30, 400, 0.3, 0.3, DefaultFloorSpecs(), 100,
		SMCConfig{NumParticles: 600, NumRounds: 4, Seed: 1})

	idx95 := len(st.Levels) - 1 // 0.95 is last
	if st.Levels[idx95] != 0.95 {
		t.Fatalf("expected last level 0.95, got %v", st.Levels[idx95])
	}
	plug95, sbi95 := st.PlugIn.Coverage[idx95], st.SBI.Coverage[idx95]
	if !(sbi95 > plug95) {
		t.Fatalf("expected SBI 95%% coverage (%.2f) > plug-in (%.2f)", sbi95, plug95)
	}
	if plug95 >= 0.95 {
		t.Fatalf("plug-in should under-cover the truth, got %.2f at nominal 0.95", plug95)
	}
	// SBI must be materially better calibrated and wider.
	if sbi95-plug95 < 0.03 {
		t.Fatalf("calibration gap too small: sbi=%.2f plug=%.2f", sbi95, plug95)
	}
	if st.SBI.MeanWidth[idx95] <= st.PlugIn.MeanWidth[idx95] {
		t.Fatalf("SBI interval should be wider (folds in identification): sbi=%.3f plug=%.3f",
			st.SBI.MeanWidth[idx95], st.PlugIn.MeanWidth[idx95])
	}
}
