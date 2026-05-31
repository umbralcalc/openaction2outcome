package validity

import (
	"fmt"

	"github.com/umbralcalc/openaction2outcome/internal/bridge"
	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// This file is the bridge-mark validity battery — the analogue of the
// manipulation/continuity/placebo battery for identified marks. A bridge is only
// admissible when:
//
//  1. Anchor coherence holds: the anchors share one mechanism (same population,
//     regime, and outcome construct) with a written justification. This is the
//     load-bearing, mandatory check — a bridge across anchors from different
//     causal regimes is a category error.
//  2. The query point is strictly bracketed by anchors (interpolation only).
//
// On top of those gates it reports two credibility numbers that ship in the
// dossier but do not by themselves reject a bridge: leave-one-anchor-out (LOAO)
// coverage — the empirical "does the bridge interpolate real truth" test — and
// kernel sensitivity — how much the estimate moves under an alternative
// covariance, flagged when the estimate is kernel-driven.

// BridgeBatteryInput bundles everything the battery needs to validate one bridge.
type BridgeBatteryInput struct {
	Mechanism  bridge.Mechanism
	Anchors    []bridge.Anchor
	Query      float64
	Kernel     bridge.Kernel   // the bridge's chosen kernel
	AltKernels []bridge.Kernel // alternatives for sensitivity (include Kernel first)
	Coherence  schema.AnchorCoherence
	Level      float64 // credible level for LOAO / sensitivity intervals (e.g. 0.95)
	SMC        bridge.SMCConfig
}

// RunBridgeBattery executes the bridge validity battery and returns the dossier
// section with the admission verdict. It never mutates its inputs.
func RunBridgeBattery(in BridgeBatteryInput) schema.BridgeChecks {
	checks := schema.BridgeChecks{
		Coherence: in.Coherence,
		LOAOLevel: in.Level,
	}

	coherenceOK, coherenceMsg := coherenceVerdict(in.Coherence)
	bracketingOK := bridge.CheckBracketing(in.Anchors, in.Query) == nil
	checks.BracketingOK = bracketingOK

	// LOAO coverage (headline credibility number).
	loao := bridge.LeaveOneAnchorOut(in.Mechanism, in.Anchors, in.Kernel, in.Level, in.SMC)
	checks.LOAOCoverage = loao.Coverage
	checks.LOAO = toSchemaLOAO(loao.Rows)

	// Kernel sensitivity.
	if len(in.AltKernels) >= 2 {
		ks := bridge.RefitAcrossKernels(in.Mechanism, in.Anchors, in.Query, in.AltKernels, in.Level, in.SMC)
		checks.KernelSensitivity = toSchemaKernelSensitivity(ks.Rows)
		checks.KernelFlagged = ks.Flagged
	}

	// Admission: coherence and bracketing are the hard gates.
	checks.Admitted = coherenceOK && bracketingOK
	checks.Notes = bridgeNotes(coherenceOK, coherenceMsg, bracketingOK, checks.LOAOCoverage, in.Level, checks.KernelFlagged)
	return checks
}

// coherenceVerdict requires the structured claim (same population, regime, and
// outcome construct) and a non-empty written justification. Without it the
// bridge is rejected — it is the bridge-specific load-bearing check.
func coherenceVerdict(c schema.AnchorCoherence) (bool, string) {
	if c.Justification == "" {
		return false, "missing anchor-coherence justification (mandatory)"
	}
	if !c.SamePopulation || !c.SameRegime || !c.SameOutcomeConstruct {
		return false, fmt.Sprintf("anchors not asserted coherent (same_population=%v same_regime=%v same_outcome_construct=%v)",
			c.SamePopulation, c.SameRegime, c.SameOutcomeConstruct)
	}
	return true, "anchors asserted coherent on one mechanism"
}

func bridgeNotes(coherenceOK bool, coherenceMsg string, bracketingOK bool, loaoCov, level float64, kernelFlagged bool) string {
	verdict := "ADMITTED"
	if !coherenceOK || !bracketingOK {
		verdict = "REJECTED"
	}
	s := fmt.Sprintf("%s. Coherence: %s. Bracketing: %s (interpolation only). "+
		"LOAO coverage at %.0f%%: %.0f%% of held-out anchors fell within the bridge's predicted interval.",
		verdict, coherenceMsg, bracketedWord(bracketingOK), 100*level, 100*loaoCov)
	if kernelFlagged {
		s += " KERNEL-SENSITIVE: τ(query) moves materially under an alternative covariance kernel — the estimate is partly kernel-driven; read the kernel-sensitivity table."
	}
	return s
}

func bracketedWord(ok bool) string {
	if ok {
		return "query strictly bracketed by anchors"
	}
	return "query NOT bracketed (extrapolation — out of scope)"
}

func toSchemaLOAO(rows []bridge.LOAORow) []schema.LOAORow {
	out := make([]schema.LOAORow, len(rows))
	for i, r := range rows {
		out[i] = schema.LOAORow{
			HeldMarkID:    r.HeldMarkID,
			PolicyPoint:   r.PolicyPoint,
			AnchorCentral: r.AnchorCentral,
			PredLower:     r.PredLower,
			PredUpper:     r.PredUpper,
			Covered:       r.Covered,
			Skipped:       r.Skipped,
			SkipReason:    r.SkipReason,
		}
	}
	return out
}

func toSchemaKernelSensitivity(rows []bridge.KernelSensitivityRow) []schema.KernelSensitivityRow {
	out := make([]schema.KernelSensitivityRow, len(rows))
	for i, r := range rows {
		out[i] = schema.KernelSensitivityRow{
			Kernel:  r.Kernel,
			Central: r.Central,
			Lower:   r.Lower,
			Upper:   r.Upper,
		}
	}
	return out
}
