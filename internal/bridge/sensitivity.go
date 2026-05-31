package bridge

import "math"

// Kernel sensitivity: the covariance kernel is the load-bearing trust-decay
// assumption, so a bridge re-fits τ(query) under alternative kernels and reports
// how much the estimate and its interval move. Large movement means the estimate
// is kernel-driven rather than anchor-driven, and is flagged prominently.

// KernelSensitivityRow is τ(query) and its interval under one kernel.
type KernelSensitivityRow struct {
	Kernel  string
	Central float64
	Lower   float64
	Upper   float64
}

// KernelSensitivity is the cross-kernel comparison.
type KernelSensitivity struct {
	Level        float64
	Rows         []KernelSensitivityRow
	CentralRange float64 // max-min Central across kernels
	WidthRange   float64 // max-min interval width across kernels
	Flagged      bool    // true when movement is large relative to the baseline width
}

// flagFraction is the share of the baseline (first kernel's) interval width above
// which cross-kernel movement of the central estimate is considered large enough
// to flag the bridge as kernel-driven.
const flagFraction = 0.5

// RefitAcrossKernels recomputes τ(query) under each kernel. The first kernel is
// treated as the baseline for the flag threshold. A kernel that fails to fit is
// skipped (its row omitted); if fewer than two kernels fit, no flag is raised.
func RefitAcrossKernels(mech Mechanism, anchors []Anchor, query float64, kernels []Kernel, level float64, cfg SMCConfig) KernelSensitivity {
	ks := KernelSensitivity{Level: level}
	var baselineWidth float64
	minC, maxC := math.Inf(1), math.Inf(-1)
	minW, maxW := math.Inf(1), math.Inf(-1)
	for idx, k := range kernels {
		post, err := Calibrate(mech, anchors, query, k, cfg)
		if err != nil {
			continue
		}
		lo, hi := post.Interval(level)
		ks.Rows = append(ks.Rows, KernelSensitivityRow{Kernel: k.Name(), Central: post.Central, Lower: lo, Upper: hi})
		w := hi - lo
		if idx == 0 {
			baselineWidth = w
		}
		minC, maxC = math.Min(minC, post.Central), math.Max(maxC, post.Central)
		minW, maxW = math.Min(minW, w), math.Max(maxW, w)
	}
	if len(ks.Rows) >= 2 {
		ks.CentralRange = maxC - minC
		ks.WidthRange = maxW - minW
		if baselineWidth <= 0 {
			baselineWidth = maxW // fall back to the widest if the baseline degenerated
		}
		ks.Flagged = baselineWidth > 0 && ks.CentralRange > flagFraction*baselineWidth
	}
	return ks
}

// DefaultKernels returns the standard sensitivity pair (squared-exponential and
// Matérn-5/2) sharing the given hyperparameters. The first is the baseline.
func DefaultKernels(sigmaF, lengthscale float64) []Kernel {
	return []Kernel{
		SquaredExponential{SigmaF: sigmaF, Lengthscale: lengthscale},
		Matern52{SigmaF: sigmaF, Lengthscale: lengthscale},
	}
}
