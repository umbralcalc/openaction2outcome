package validity

import (
	"testing"

	"github.com/umbralcalc/openaction2outcome/internal/bridge"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

func bridgeAnchor(id string, x, mu, sd float64) bridge.Anchor {
	return bridge.Anchor{
		MarkID: id,
		X:      x,
		Dist: schema.Distribution{
			Central:  mu,
			StdDev:   &sd,
			Interval: &schema.Interval{Level: 0.95, Lower: mu - 1.96*sd, Upper: mu + 1.96*sd},
		},
	}
}

func TestBridgeBatteryAdmitsCoherentBracketed(t *testing.T) {
	curve := func(x float64) float64 { return 0.2 + 0.5*x }
	anchors := []bridge.Anchor{
		bridgeAnchor("A", -1, curve(-1), 0.06),
		bridgeAnchor("B", -0.3, curve(-0.3), 0.06),
		bridgeAnchor("C", 0.3, curve(0.3), 0.06),
		bridgeAnchor("D", 1, curve(1), 0.06),
	}
	cfg := bridge.SMCConfig{NumParticles: 400, NumRounds: 5, Seed: 1}
	out := RunBridgeBattery(BridgeBatteryInput{
		Mechanism:  bridge.NewQuadraticMechanism(),
		Anchors:    anchors,
		Query:      0.0,
		Kernel:     bridge.SquaredExponential{SigmaF: 0.4, Lengthscale: 0.6},
		AltKernels: bridge.DefaultKernels(0.4, 0.6),
		Coherence: schema.AnchorCoherence{
			SamePopulation: true, SameRegime: true, SameOutcomeConstruct: true,
			Justification: "all anchors are the same regulator threshold on one mechanism.",
		},
		Level: 0.95,
		SMC:   cfg,
	})
	if !out.Admitted {
		t.Fatalf("coherent, bracketed bridge should be admitted; notes: %s", out.Notes)
	}
	if out.LOAOCoverage <= 0 {
		t.Fatalf("expected a non-zero LOAO coverage; got %v", out.LOAOCoverage)
	}
	if len(out.KernelSensitivity) != 2 {
		t.Fatalf("expected two kernel-sensitivity rows; got %d", len(out.KernelSensitivity))
	}
}

func TestBridgeBatteryRejectsIncoherent(t *testing.T) {
	anchors := []bridge.Anchor{
		bridgeAnchor("A", -1, 0, 0.06),
		bridgeAnchor("B", 1, 1, 0.06),
	}
	cfg := bridge.SMCConfig{NumParticles: 300, NumRounds: 4, Seed: 1}
	out := RunBridgeBattery(BridgeBatteryInput{
		Mechanism:  bridge.NewQuadraticMechanism(),
		Anchors:    anchors,
		Query:      0.0,
		Kernel:     bridge.SquaredExponential{SigmaF: 0.4, Lengthscale: 0.6},
		AltKernels: bridge.DefaultKernels(0.4, 0.6),
		Coherence:  schema.AnchorCoherence{Justification: ""}, // missing → reject
		Level:      0.95,
		SMC:        cfg,
	})
	if out.Admitted {
		t.Fatal("a bridge with no coherence justification must be rejected")
	}
}
