package dossier

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// renderBridge renders the dossier for a bridge mark. It leads with the pin/span
// discipline: this is a simulator-bridged estimate between identified anchors,
// never identified truth. The pin/span picture, the load-bearing kernel, the
// anchor-coherence justification, and the headline LOAO coverage are all surfaced
// so the boundary is unmissable.
func renderBridge(m schema.Mark) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	bs := m.Bridge
	bc := m.Dossier.Bridge

	w("# %s\n\n", m.ID)
	verdict := "ADMITTED"
	if bc == nil || !bc.Admitted {
		verdict = "NOT ADMITTED"
	}
	w("**Category:** bridge (simulator-bridged estimate — a span, NOT identified truth)  ·  "+
		"**Truth source:** %s  ·  **Domain:** %s  ·  **Status:** %s\n\n", m.TruthSource, m.Domain, verdict)
	w("> This mark is **not** a clean natural experiment. It is a calibrated estimate of the effect at a " +
		"point where no cutoff exists, pinned to real identified anchors on both sides and spanned by a " +
		"simulator plus a Gaussian-process discrepancy. The honest interval below is the posterior: narrow " +
		"at the pins, wider between them, always bounded because the query is bracketed. Filter the " +
		"collection to `category == identified` to exclude every simulated quantity.\n\n")

	if bs == nil {
		w("_Bridge specification missing._\n")
		return b.String()
	}

	// The pin/span picture.
	w("## The bridge\n\n")
	w("- **Mechanism:** %s\n", bs.Mechanism)
	w("- **Policy variable (x):** %s\n", bs.PolicyVariable)
	w("- **Query point:** x = %g (the point this mark estimates τ at)\n\n", bs.QueryPoint)
	w("**Pin/span picture** (anchors are real identified marks; the query is interpolated strictly between them):\n\n")
	w("```\n%s\n```\n\n", pinSpanPicture(bs.Anchors, bs.QueryPoint))
	w("| anchor mark | policy point x | side of query |\n|---|---|---|\n")
	sorted := append([]schema.AnchorRef(nil), bs.Anchors...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].PolicyPoint < sorted[j].PolicyPoint })
	for _, a := range sorted {
		w("| `%s` | %g | %s |\n", a.MarkID, a.PolicyPoint, sideOf(a.PolicyPoint, bs.QueryPoint))
	}
	w("\n")

	// The effect.
	w("## The effect (honest interval = the calibrated posterior)\n\n")
	e := m.Effect
	if e.Interval != nil {
		w("**%g** with a %.0f%% interval of **[%g, %g]**.\n\n",
			round4(e.Central), 100*e.Interval.Level, round4(e.Interval.Lower), round4(e.Interval.Upper))
	} else {
		w("**%g**.\n\n", round4(e.Central))
	}
	if ub := e.UncertaintyBudget; ub != nil {
		w("The interval width separates into:\n\n")
		w("| source | standard deviation |\n|---|---|\n")
		if ub.Sampling != nil {
			w("| GP discrepancy / pinning | %.4f |\n", math.Sqrt(*ub.Sampling))
		}
		if ub.Specification != nil {
			w("| simulator / parameter (θ) uncertainty | %.4f |\n", math.Sqrt(*ub.Specification))
		}
		if e.StdDev != nil {
			w("| **total** | **%.4f** |\n", *e.StdDev)
		}
		w("\n")
	}

	// The load-bearing kernel.
	w("## The trust-decay assumption (the load-bearing kernel)\n\n")
	w("The shape of the interval between the pins is governed by the discrepancy GP's covariance kernel. " +
		"It is a *choice*, shipped openly so you see exactly what trust-decay assumption the estimate rests on.\n\n")
	w("- **Kernel family:** `%s`\n", bs.Kernel.Family)
	if len(bs.Kernel.Params) > 0 {
		keys := make([]string, 0, len(bs.Kernel.Params))
		for k := range bs.Kernel.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s = %g", k, bs.Kernel.Params[k]))
		}
		w("- **Hyperparameters:** %s\n", strings.Join(parts, ", "))
	}
	if bs.Kernel.Jitter > 0 {
		w("- **Numerical jitter:** %g\n", bs.Kernel.Jitter)
	}
	w("\n")

	// Validity battery.
	w("## Bridge validity checks\n\n")
	if bc == nil {
		w("_No bridge validity battery recorded._\n\n")
	} else {
		// Anchor coherence (the load-bearing check).
		w("**Anchor coherence** (the load-bearing check — anchors must lie on one mechanism): %s\n\n",
			tick(bc.Coherence.SamePopulation && bc.Coherence.SameRegime && bc.Coherence.SameOutcomeConstruct && bc.Coherence.Justification != ""))
		w("| same population | same regime | same outcome construct |\n|---|---|---|\n")
		w("| %s | %s | %s |\n\n", tick(bc.Coherence.SamePopulation), tick(bc.Coherence.SameRegime), tick(bc.Coherence.SameOutcomeConstruct))
		if bc.Coherence.Justification != "" {
			w("> %s\n\n", bc.Coherence.Justification)
		}

		w("**Bracketing** (interpolation only — query strictly between anchors): %s\n\n", tick(bc.BracketingOK))

		// LOAO — the headline credibility number.
		w("**Leave-one-anchor-out (LOAO) coverage** — the headline credibility number: **%.0f%%** of held-out "+
			"anchors fell within the bridge's predicted %.0f%% interval.\n\n", 100*bc.LOAOCoverage, 100*bc.LOAOLevel)
		if len(bc.LOAO) > 0 {
			w("| held-out anchor | x | anchor central | predicted interval | covered |\n|---|---|---|---|---|\n")
			for _, r := range bc.LOAO {
				if r.Skipped {
					w("| `%s` | %g | %.4f | — | skipped (endpoint) |\n", r.HeldMarkID, r.PolicyPoint, r.AnchorCentral)
					continue
				}
				w("| `%s` | %g | %.4f | [%.4f, %.4f] | %s |\n",
					r.HeldMarkID, r.PolicyPoint, r.AnchorCentral, r.PredLower, r.PredUpper, tick(r.Covered))
			}
			w("\n")
		}

		// Kernel sensitivity.
		if len(bc.KernelSensitivity) > 0 {
			flag := "stable across kernels"
			if bc.KernelFlagged {
				flag = "**KERNEL-SENSITIVE — the estimate is partly kernel-driven**"
			}
			w("**Kernel sensitivity** (τ(query) under alternative covariances): %s\n\n", flag)
			w("| kernel | central | interval |\n|---|---|---|\n")
			for _, r := range bc.KernelSensitivity {
				w("| `%s` | %.4f | [%.4f, %.4f] |\n", r.Kernel, r.Central, r.Lower, r.Upper)
			}
			w("\n")
		}

		if bc.Notes != "" {
			w("**Notes.** %s\n\n", bc.Notes)
		}
	}

	// Provenance (simulator determinism + anchor sources).
	w("## Provenance\n\n")
	w("- **Simulator:** `%s` version `%s`", bs.Simulator.ModelID, bs.Simulator.Version)
	if bs.Simulator.Seed != nil {
		w(" (seed %d)", *bs.Simulator.Seed)
	}
	w("\n")
	pr := m.Provenance
	if len(pr.Sources) > 0 {
		w("- **Anchor sources:**\n")
		for _, s := range pr.Sources {
			w("  - %s — %s. Licence: %s. SHA-256 `%s`.\n", s.Title, s.Publisher, s.Licence, s.SHA256)
		}
	}
	if len(pr.ToolVersions) > 0 {
		keys := make([]string, 0, len(pr.ToolVersions))
		for k := range pr.ToolVersions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s %s", k, pr.ToolVersions[k]))
		}
		w("- **Reproducibility:** %s. The bridge re-mints byte-for-byte from the anchors + simulator seed + kernel.\n", strings.Join(parts, ", "))
	}
	return b.String()
}

// pinSpanPicture draws a tiny ASCII number line with the anchors as pins (|) and
// the query as the interpolated point (?), so the bracketing is visible at a glance.
func pinSpanPicture(anchors []schema.AnchorRef, query float64) string {
	pts := append([]schema.AnchorRef(nil), anchors...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].PolicyPoint < pts[j].PolicyPoint })
	if len(pts) == 0 {
		return ""
	}
	lo := pts[0].PolicyPoint
	hi := pts[len(pts)-1].PolicyPoint
	if hi <= lo {
		hi = lo + 1
	}
	const width = 50
	pos := func(x float64) int {
		p := int(math.Round((x - lo) / (hi - lo) * float64(width)))
		if p < 0 {
			p = 0
		}
		if p > width {
			p = width
		}
		return p
	}
	line := make([]rune, width+1)
	for i := range line {
		line[i] = '-'
	}
	for _, a := range pts {
		line[pos(a.PolicyPoint)] = '|'
	}
	q := []rune(string(line))
	q[pos(query)] = '?'
	return fmt.Sprintf("%s\n%s\n(| = real identified anchor (pin)   ? = interpolated query (span)   x: %g … %g)",
		string(line), string(q), lo, hi)
}

// sideOf reports whether an anchor sits below or above the query point.
func sideOf(x, query float64) string {
	if x < query {
		return "below"
	}
	if x > query {
		return "above"
	}
	return "at query"
}
