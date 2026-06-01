package bridge

import "testing"

func TestBridgeRecoveryStudyTracksNominal(t *testing.T) {
	if testing.Short() {
		t.Skip("recovery study is slow; skipped in -short")
	}
	// A modest suite, fast SMC. Recovery and LOAO coverage should be in the right
	// ballpark of their nominal levels — the machinery interpolates honestly.
	study := RunBridgeRecoveryStudy(40, 20240531, SMCConfig{NumParticles: 500, NumRounds: 6, Seed: 1})

	// Find the 0.95 level index.
	idx95 := -1
	for i, L := range study.Levels {
		if L == 0.95 {
			idx95 = i
		}
	}
	if idx95 < 0 {
		t.Fatal("expected a 0.95 level")
	}
	// Recovery at 0.95 should be high (a pinned interpolant covering known truth);
	// allow a generous band given the small suite and fast SMC.
	if rec := study.Recovery.Coverage[idx95]; rec < 0.75 {
		t.Fatalf("recovery@0.95 should track nominal; got %.3f", rec)
	}
	// LOAO coverage at 0.95 should likewise be high.
	if loao := study.LOAO.Coverage[idx95]; loao < 0.6 {
		t.Fatalf("LOAO@0.95 should be reasonably high; got %.3f", loao)
	}
	// Coverage must increase monotonically-ish with nominal level.
	if study.Recovery.Coverage[0] > study.Recovery.Coverage[idx95]+1e-9 {
		t.Fatalf("coverage should not decrease with level: %v", study.Recovery.Coverage)
	}
}

func TestDeterministicLayerStudy(t *testing.T) {
	if testing.Short() {
		t.Skip("layer study runs SMC for the agreement arm; skipped in -short")
	}
	study := RunDeterministicLayerStudy(15, 7000, SMCConfig{NumParticles: 600, NumRounds: 6, Seed: 1})

	// The deterministic moment answer must match the SMC closed-form joint to within
	// Monte-Carlo error (no systematic gap), and re-mint byte-for-byte.
	if study.MaxMomentVsSMC > 0.05 {
		t.Errorf("moment vs SMC closed-form gap too large (%.4f) — should be within MC error", study.MaxMomentVsSMC)
	}
	if !study.RemintIdentical {
		t.Error("moment calibrator must re-mint byte-for-byte")
	}
	// The linear-Gaussian causal mechanism must earn the closed-form rung everywhere.
	if !study.AllClosedForm || study.GateRung != rungClosedForm {
		t.Errorf("linear-Gaussian SCM should always earn the closed-form rung; allClosedForm=%v rung=%q", study.AllClosedForm, study.GateRung)
	}
	// Recovery of the known interventional truth should track nominal (conservative ok).
	idx95 := -1
	for i, L := range study.Levels {
		if L == 0.95 {
			idx95 = i
		}
	}
	if study.Recovery.Coverage[idx95] < 0.8 {
		t.Errorf("recovery@0.95 should track nominal; got %.3f", study.Recovery.Coverage[idx95])
	}
}

func TestKernelSensitivityFlags(t *testing.T) {
	// A smooth, simulator-friendly truth → kernels should roughly agree (not flagged).
	curve := func(x float64) float64 { return 0.2 + 0.5*x }
	as := threeAnchors(curve, 0.05)
	kernels := DefaultKernels(0.4, 0.6)
	ks := RefitAcrossKernels(NewQuadraticMechanism(), as, 0.5, kernels, 0.95, testCfg)
	if len(ks.Rows) != 2 {
		t.Fatalf("expected both kernels to fit; got %d rows", len(ks.Rows))
	}
	if ks.Flagged {
		t.Fatalf("a smooth simulator-friendly truth should not be kernel-driven; central range %g", ks.CentralRange)
	}
}
