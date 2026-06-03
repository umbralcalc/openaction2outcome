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

// Render returns the Markdown dossier for a mark. Bridge marks get a distinct
// dossier (pin/span picture, kernel, coherence, LOAO) so a reader can never
// mistake a simulator-bridged estimate for an identified one.
func Render(m schema.Mark) string {
	if m.EffectiveCategory() == schema.CategoryBridge {
		return renderBridge(m)
	}
	if m.EffectiveIdentification() == schema.IDITSControlled {
		return renderITS(m)
	}
	return renderIdentified(m)
}

// renderIdentified renders the dossier for a design-based (RDD) mark.
func renderIdentified(m schema.Mark) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# %s\n\n", m.ID)
	verdict := "ADMITTED"
	if !m.Dossier.Admitted {
		verdict = "NOT ADMITTED"
	}
	w("**Category:** identified (design-based truth — a pin)  ·  **Series:** %s  ·  **Domain:** %s  ·  **Unit:** %s  ·  **Design:** %s RDD  ·  **Status:** %s\n\n",
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
	writeEffect(&b, m)

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
	w("The analysis-ready rows (one per unit) live in the single published `episodes` "+
		"dataset, alongside every other mark's rows. Recover this mark's rows by filtering "+
		"on `mark_id == %q`. The dataset's download URL and content hash are in "+
		"`datasets/episodes.manifest.json`.\n\n", m.ID)
	if len(m.Context.CovariateNames) > 0 {
		w("- **Covariates (state):** %s\n\n", strings.Join(m.Context.CovariateNames, ", "))
	}

	// Provenance.
	writeProvenance(&b, m)
	return b.String()
}

// writeEffect renders the shared "## The effect" section (central + interval and
// the two-source uncertainty budget). Identical for every identified design.
func writeEffect(b *strings.Builder, m schema.Mark) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
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
}

// writeProvenance renders the shared "## Provenance" section (point-in-time order,
// sources, reproducibility). Identical for every identified design.
func writeProvenance(b *strings.Builder, m schema.Mark) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
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
}

// renderITS renders the dossier for a controlled-interrupted-time-series mark.
// The design and validity sections are the time-domain analogues of the RDD
// dossier; the effect and provenance sections are shared. The header flags the
// population-over-window estimand so a reader never pools it with RDD marks.
func renderITS(m schema.Mark) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# %s\n\n", m.ID)
	verdict := "ADMITTED"
	if !m.Dossier.Admitted {
		verdict = "NOT ADMITTED"
	}
	w("**Category:** identified (design-based truth — a pin)  ·  **Series:** %s  ·  **Domain:** %s  ·  **Unit:** %s  ·  **Design:** controlled interrupted time series  ·  **Status:** %s\n\n",
		m.Series, m.Domain, m.UnitType, verdict)
	w("> The estimand is a **population** effect accumulated over the post-intervention window, not a local-at-cutoff effect. Its decision scores are never pooled with RDD marks.\n\n")

	// Design.
	d := m.Design
	w("## The decision\n\n")
	w("- **Action:** %s\n", d.Action)
	w("- **Alternative:** %s\n", d.Alternative)
	w("- **Outcome:** %s — %s (%s)\n", d.Outcome.Name, d.Outcome.Description, d.Outcome.Units)
	w("- **Estimand:** %s\n", d.Estimand)
	if its := d.ITS; its != nil {
		w("- **Intervention instant:** %s (time axis: %s, units %s)\n", its.InterventionInstant, its.RunningTime.Name, its.RunningTime.Units)
		w("- **Pre-window:** %s → %s\n", its.PreWindow.Start, its.PreWindow.End)
		w("- **Post-window:** %s → %s\n", its.PostWindow.Start, its.PostWindow.End)
		if its.Transition != nil {
			w("- **Transition (excluded ramp):** %s → %s\n", its.Transition.Start, its.Transition.End)
		}
		w("- **Counterfactual:** %s", its.Counterfactual.Family)
		if its.Counterfactual.Seasonality != "" {
			w(", seasonality: %s", its.Counterfactual.Seasonality)
		}
		w(" — %s\n", its.Counterfactual.Justification)
		if its.Control != nil {
			w("- **Control series:** %s (%s) — %s\n", its.Control.SeriesID, its.Control.Role, its.Control.Justification)
		}
	}
	w("\n")

	// Effect.
	writeEffect(&b, m)

	// Validity battery.
	w("## Validity checks\n\n")
	if its := m.Dossier.ITS; its != nil {
		w("%s\n\n", passLine("No anticipation (no pre-trend break / forestalling)", its.NoAnticipation.Passed, its.NoAnticipation.Method, its.NoAnticipation.PValue))
		w("%s\n\n", passLine("Control parallelism (shared pre-intervention trend)", its.ControlParallelism.Passed, its.ControlParallelism.Method, its.ControlParallelism.PValue))
		if len(its.PlaceboDates) > 0 {
			w("**Placebo dates** (effect should vanish at fake pre-period dates):\n\n")
			w("| placebo date | estimate | indistinguishable from zero |\n|---|---|---|\n")
			for _, p := range its.PlaceboDates {
				w("| %s | %.4f | %s |\n", p.Date, p.Estimate, tick(p.Passed))
			}
			w("\n")
		}
		if len(its.PlaceboOutcomes) > 0 {
			w("**Placebo outcomes** (a logically unaffected outcome should show no effect):\n\n")
			w("| outcome | p-value | pass |\n|---|---|---|\n")
			for _, c := range its.PlaceboOutcomes {
				w("| %s | %s | %s |\n", c.Name, fP(c.PValue), tick(c.Passed))
			}
			w("\n")
		}
		if len(its.WindowSweep) > 0 {
			w("**Window sweep** (estimate vs pre/post window length):\n\n")
			w("| window length | estimate |\n|---|---|\n")
			for _, s := range its.WindowSweep {
				w("| %g | %.4f |\n", s.Param, s.Estimate)
			}
			w("\n")
		}
		if len(its.TransitionExclusion) > 0 {
			w("**Transition exclusion** (re-estimate after dropping the implementation ramp):\n\n")
			w("| ramp width | estimate |\n|---|---|\n")
			for _, s := range its.TransitionExclusion {
				w("| %g | %.4f |\n", s.Param, s.Estimate)
			}
			w("\n")
		}
		if its.DoseCheck != nil {
			w("**Dose check** (the action was actually delivered — sales/price/compliance moved at the date): %.4f — %s\n\n", its.DoseCheck.Jump, tick(its.DoseCheck.Passed))
		}
		w("%s\n\n", passLine("Autocorrelation modelled (Newey-West / ARMA errors)", its.Autocorrelation.Passed, its.Autocorrelation.Method, its.Autocorrelation.PValue))
		if its.Notes != "" {
			w("**Notes.** %s\n\n", its.Notes)
		}
	}

	// Data.
	w("## Data\n\n")
	w("The analysis-ready panel rows (one per series × time bucket) live in the single "+
		"published `episodes` dataset, alongside every other mark's rows. Recover this mark's "+
		"rows by filtering on `mark_id == %q`; the row shape is `panel`. The dataset's download "+
		"URL and content hash are in `datasets/episodes.manifest.json`.\n\n", m.ID)
	if len(m.Context.CovariateNames) > 0 {
		w("- **Covariates (state):** %s\n\n", strings.Join(m.Context.CovariateNames, ", "))
	}

	// Provenance.
	writeProvenance(&b, m)
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
