// Package dossier renders a mark into a human-readable validity dossier
// (Markdown) at mint time. The dossier is the static, reviewable evidence that a
// mark earned its place: the design, the effect with its uncertainty split, the
// full validity battery, and the provenance. Rendering is deterministic.
package dossier

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/umbralcalc/openaction2outcome/pkg/schema"
)

// Render returns the Markdown dossier for a mark.
func Render(m schema.Mark) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# %s\n\n", m.ID)
	verdict := "ADMITTED"
	if !m.Dossier.Admitted {
		verdict = "NOT ADMITTED"
	}
	w("**Series:** %s  ·  **Domain:** %s  ·  **Unit:** %s  ·  **Design:** %s RDD  ·  **Status:** %s\n\n",
		m.Series, m.Domain, m.UnitType, m.RDDType, verdict)

	// Design.
	w("## The decision\n\n")
	w("- **Running variable:** %s — %s (%s)\n", m.Design.RunningVariable.Name, m.Design.RunningVariable.Description, m.Design.RunningVariable.Units)
	w("- **Cutoff:** %g (%s)\n", m.Design.Cutoff, m.Design.Direction)
	w("- **Action:** %s\n", m.Design.Action)
	w("- **Alternative:** %s\n", m.Design.Alternative)
	w("- **Outcome:** %s — %s (%s)\n", m.Design.Outcome.Name, m.Design.Outcome.Description, m.Design.Outcome.Units)
	w("- **Estimand:** %s\n\n", m.Design.Estimand)

	// Effect.
	w("## The effect\n\n")
	e := m.Effect
	if e.Interval != nil {
		w("**%g** with a %.0f%% interval of **[%g, %g]**.\n\n", round4(e.Central), 100*e.Interval.Level, round4(e.Interval.Lower), round4(e.Interval.Upper))
	} else {
		w("**%g**.\n\n", round4(e.Central))
	}
	if ub := e.UncertaintyBudget; ub != nil {
		w("The interval width separates into two sources:\n\n")
		w("| source | standard deviation |\n|---|---|\n")
		if ub.Sampling != nil {
			w("| sampling (finite data) | %.4f |\n", math.Sqrt(*ub.Sampling))
		}
		if ub.Specification != nil {
			w("| identification (bandwidth / order / kernel choice) | %.4f |\n", math.Sqrt(*ub.Specification))
		}
		if e.StdDev != nil {
			w("| **total** | **%.4f** |\n", *e.StdDev)
		}
		w("\n")
	}

	// Validity battery.
	w("## Validity checks\n\n")
	d := m.Dossier
	w("%s\n\n", passLine("Density / manipulation", d.Density.Passed, d.Density.Method, d.Density.PValue))
	if len(d.CovariateContinuity) > 0 {
		w("**Covariate continuity at the cutoff** (covariates should not jump):\n\n")
		w("| covariate | jump | p-value | pass |\n|---|---|---|---|\n")
		for _, c := range d.CovariateContinuity {
			w("| %s | %s | %s | %s |\n", c.Name, fStat(c.Statistic), fP(c.PValue), tick(c.Passed))
		}
		w("\n")
	}
	if len(d.PlaceboCutoffs) > 0 {
		w("**Placebo cutoffs** (effect should vanish away from the real cutoff):\n\n")
		w("| placebo cutoff | estimate | indistinguishable from zero |\n|---|---|---|\n")
		for _, p := range d.PlaceboCutoffs {
			w("| %g | %.4f | %s |\n", p.Cutoff, p.Estimate, tick(p.Passed))
		}
		w("\n")
	}
	if len(d.BandwidthSweep) > 0 {
		w("**Bandwidth sweep** (estimate vs window width):\n\n")
		w("| bandwidth | estimate |\n|---|---|\n")
		for _, s := range d.BandwidthSweep {
			w("| %g | %.4f |\n", s.Param, s.Estimate)
		}
		w("\n")
	}
	if len(d.DonutRobustness) > 0 {
		w("**Donut robustness** (re-estimate after excluding units nearest the cutoff):\n\n")
		w("| donut radius | estimate |\n|---|---|\n")
		for _, s := range d.DonutRobustness {
			w("| %g | %.4f |\n", s.Param, s.Estimate)
		}
		w("\n")
	}
	if d.FirstStage != nil {
		w("**First stage** (jump in treatment probability at the cutoff): %.4f — %s\n\n", d.FirstStage.Jump, tick(d.FirstStage.Passed))
	}
	if len(d.SeamSpecificChecks) > 0 {
		w("**Seam-specific checks:**\n\n")
		for _, c := range d.SeamSpecificChecks {
			w("- **%s:** %s", c.Name, tick(c.Passed))
			if c.Method != "" {
				w(" — %s", c.Method)
			}
			w("\n")
			if c.Detail != "" {
				w("  - %s\n", c.Detail)
			}
		}
		w("\n")
	}
	if d.Notes != "" {
		w("**Notes.** %s\n\n", d.Notes)
	}

	// Data.
	w("## Data\n\n")
	w("The analysis-ready episode table (one row per unit) is published separately:\n\n")
	w("- **URL:** %s\n", m.Data.URI)
	w("- **SHA-256:** `%s`\n", m.Data.SHA256)
	w("- **Rows:** %d  ·  **Format:** %s\n", m.Data.Rows, m.Data.Format)
	if len(m.Data.Columns) > 0 {
		w("- **Columns:** %s\n", strings.Join(m.Data.Columns, ", "))
	}
	w("\n")

	// Provenance.
	w("## Provenance\n\n")
	pr := m.Provenance
	w("Point-in-time order: context as-of `%s` ≤ decision `%s` < outcome `%s`.\n\n",
		pr.ContextAsOf, pr.DecisionTimestamp, pr.OutcomeTimestamp)
	if len(pr.Sources) > 0 {
		w("**Sources:**\n\n")
		for _, s := range pr.Sources {
			w("- %s — %s. Licence: %s. SHA-256 `%s`.\n", s.Title, s.Publisher, s.Licence, s.SHA256)
		}
		w("\n")
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
		w("**Reproducibility:** %s. The mark and its data table re-mint byte-for-byte from the frozen inputs.\n", strings.Join(parts, ", "))
	}
	return b.String()
}

func passLine(name string, passed bool, method string, p *float64) string {
	s := fmt.Sprintf("**%s:** %s", name, tick(passed))
	if method != "" {
		s += " — " + method
	}
	if p != nil {
		s += fmt.Sprintf(" (p = %.3f)", *p)
	}
	return s
}

func tick(ok bool) string {
	if ok {
		return "pass"
	}
	return "FAIL"
}

func fStat(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.4f", *p)
}

func fP(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.3f", *p)
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }
