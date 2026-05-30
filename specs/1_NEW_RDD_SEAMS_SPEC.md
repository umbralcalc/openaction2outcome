# openaction2outcome — v2 seam integration spec
## Adding two new series: bathing-water (safe) + IPC food security (ambitious)

Conservation/IUCN was scouted and **deferred** (outcome circular with the assessment, discretionary designation, and the action→outcome question already worked via selection-on-observables — it would dilute rather than strengthen the collection). Area-based funding remains "planned" as before. The two seams below are the recommended v2 additions: one clean/safe, one high-consequence/ambitious. Together they take the collection from 2 → 4 series and, more importantly, demonstrate the instrument across the full identification-difficulty range.

Repo conventions assumed (from current `main`): each series is minted by `internal/series`, loaded by `internal/ingest`, estimated by `internal/rdd` (plug-in baseline) + `internal/sbi` (honest interval), checked by `internal/validity`, rendered by `internal/dossier`; published marks live in `marks/`, dossiers in `dossiers/`, frozen-input pointers in `data/raw/<id>/SOURCE.json`; `pkg/schema` + `pkg/score` stay dependency-light.

---

## SERIES 1 — Bathing water (safe seam)

**Story.** An English designated bathing water is classified annually (Excellent/Good/Sufficient/Poor) from a percentile statistic of the log-normal distribution of E. coli + intestinal enterococci over a rolling 4-year sample. Crossing into **Poor** mechanically triggers an action: a mandatory "advice against bathing" sign the following year + EA-led targeted catchment investigation/remediation. Outcome: the site's classification statistic in a subsequent season.

**Why safe.** Sharp formula-based threshold; EA-measured running variable (not self-reported/appealable → low manipulation); same-unit, same-source, open annual outcome (Swimfo / bathing-water data API, OGL); under-modelled in the decision frame (existing literature is all *forecasting* of exceedance, not *effect of the triggered action*).

### Slots into the repo

- **`data/raw/bathing-water/SOURCE.json`** — frozen pointers (URL + SHA-256 + OGL licence) to: the annual classifications dataset, and the underlying per-sample microbiological series needed to reconstruct the percentile running variable. Freeze specific years' vintages (point-in-time).
- **`internal/ingest`** — loader normalising (site_id, season_year, running_variable_percentile_stat, classification_band, sample_count, abnormal_samples_excluded_flag). Pure functions, unit-tested on a committed sample.
- **`internal/series`** — `bathingwater` minting: running variable = the percentile compliance statistic relative to the **Poor/Sufficient** boundary (the action-triggering cutoff); treatment = crossing into Poor; outcome = next-classification-window statistic at the same site.
- **`internal/rdd` + `internal/sbi`** — sharp RDD; SBI posterior over bandwidth/polynomial/kernel → honest interval. Same machinery as floor-standards (this is deliberately the easy case to validate the new ingest path).
- **`internal/validity`** — standard battery (McCrary density, covariate continuity, placebo cutoffs, donut) **plus one seam-specific check**: document the **abnormal-sample-exclusion** rule (extreme-rainfall samples disregarded from classification). This is a discretionary data step, not party-manipulation, but the dossier must record how many samples were excluded near the cutoff and test sensitivity to their inclusion. This is the bathing-water analogue of a manipulation check.
- **`marks/` + `dossiers/`** — one mark per qualifying site-year crossing; dossier notes the formula, the rolling-4-year window, and the exclusion sensitivity.

### Honest caveats to record in the dossier
- The 4-year rolling window means treatment status is **autocorrelated** across years for the same site — pool carefully and avoid double-counting a single crossing across overlapping windows.
- Recent regulatory change: 5-consecutive-Poor de-designation is now a Ministerial decision, not automatic — so the *downstream* action regime shifts post-amendment; pin marks to the regime in force at the decision year.

---

## SERIES 2 — IPC food security (ambitious seam)

**Story.** Subnational areas are classified into IPC Acute Food Insecurity Phases 1–5. The **Phase 2→3 ("Stressed"→"Crisis") boundary is explicitly meant to trigger deployment of humanitarian resources** (>$6bn/yr allocated with reference to IPC). Outcome: the area's food-security status / malnutrition / mortality in the next analysis window.

**Why ambitious — and why it's the most valuable seam in the collection.** Highest-consequence action space available; genuinely open and global (ipcinfo.org, World Bank Data360, HDX); almost entirely unmodelled in the causal-decision frame. **The catch is the contribution:** a Dec 2025 *Nature Food* study found **bunching and under-classification right at the threshold that triggers aid** — i.e. documented manipulation of the running variable at the cutoff. A naïve RDD here is invalid for the same reason SBRR was. openaction2outcome's honest-interval + validity-dossier framing is *uniquely* suited to ship this not as a clean mark but as a **manipulation-flagged mark whose dossier surfaces the bunching as a headline finding**: "the aid-triggering threshold is gamed, here is the density evidence, and here is what that does to the effect estimate and its honest interval."

### The critical methodological decision (settle before harvesting)
IPC phase is a **consensus, multi-indicator expert judgement**, NOT a formula — so the holistic phase is not a clean running variable (this is why it's fuzzy/ambitious, not safe). **Anchor the RDD on a specific quantitative sub-indicator**: the **percentage of population in Phase 3+** (the "20% threshold" the Nature Food paper used and where the bunching was found). Be explicit in the dossier that the design is RDD *on that sub-indicator*, not on the holistic phase, and that assignment is fuzzy (crossing raises the probability of the Crisis classification + aid, not a certainty). This mirrors how SHMI already ships as a fuzzy intention-to-treat design.

### Slots into the repo

- **`data/raw/ipc/SOURCE.json`** — frozen pointers to IPC subnational analyses (the % population per phase per area-period) from ipcinfo.org / World Bank Data360 / HDX, with each source's licence recorded (verify redistribution terms per source — IPC/FEWS NET/HDX licences differ and must each be checked before mirroring).
- **`internal/ingest`** — loader normalising (area_id, analysis_period, pct_phase3plus_running_variable, assigned_phase, aid_triggered_flag where recoverable, humanitarian-assistance-adjustment notes). Note IPC explicitly flags areas that *would be worse without assistance* — capture that field; it bears on outcome interpretation.
- **`internal/series`** — `ipc` minting: running variable = % population in Phase 3+ relative to the Crisis-trigger boundary; treatment = crossing into the aid-triggering classification; outcome = next-period food-security indicator at the same area.
- **`internal/rdd` + `internal/sbi`** — **fuzzy** RDD (first-stage = jump in aid-triggering probability at the cutoff), two-stage like SHMI; SBI propagates both stages' uncertainty.
- **`internal/validity`** — standard battery **plus the load-bearing addition**: a **McCrary/density manipulation test that is expected to FAIL or flag**, with the result shipped prominently. Cite the Nature Food bunching evidence. The mark is **admitted as manipulation-flagged**, not rejected — its honest interval widens to reflect the sorting, and the dossier makes the manipulation the centrepiece rather than a hidden caveat. This is a deliberate departure from the standard "reject on manipulation" rule, justified because *documenting the manipulation at a life-and-death aid threshold is itself the decision-science contribution*.
- **`marks/` + `dossiers/`** — manipulation-flagged marks; dossier leads with the density evidence and explains the widened interval.

### Honest caveats to record in the dossier
- **Confounding by assistance:** outcomes are contaminated by the very aid the threshold triggers (IPC even classifies "would-be-worse-without-assistance" areas). The effect you recover is entangled with assistance intensity — state this explicitly; it may be the *intended* causal quantity (effect of crossing → aid → outcome) but must be framed as ITT-through-aid, not a clean biophysical effect.
- **Sub-indicator ≠ phase:** the dossier must be unambiguous that RDD is on %Phase3+, and that holistic phase assignment involves other indicators.
- **Geopolitical data integrity:** IPC classifications are sometimes contested by host governments (e.g. documented disputes); provenance must record the governance/contestation status of each analysis.
- **This seam will draw expert scrutiny** (humanitarian-data and food-security communities are active) — the manipulation-forward honesty is the defence, not a liability.

---

## What these two add to the collection's identity

- **Range demonstration:** floor-standards + SHMI + bathing-water are the "clean/fuzzy but well-behaved" end; IPC is the "contested, manipulated, high-stakes" end. Shipping IPC *honestly* (manipulation-flagged, not hidden) is the strongest possible demonstration of *why* the honest-interval framing exists — it's the seam that justifies the whole project.
- **Breadth:** education + health + environment + humanitarian/global. Moves the collection from "UK public services" toward "threshold-triggered action on institutions/areas, anywhere, openly measured" — the broader, more shareable identity, without abandoning gov data.
- **Count:** 2 → 4 marks-series, which crosses the "enough to share more widely" bar you set.

---

## Build order (slots into existing phase model)
1. **Bathing water first** — reuses the sharp-RDD + SBI path almost unchanged; validates the new ingest/series plumbing on an easy case. Lowest risk, fastest to a shipped 3rd series.
2. **IPC second** — reuses the fuzzy two-stage path from SHMI, but requires the sub-indicator decision, the manipulation-flagged admission rule, and per-source licence checks. Higher effort; ship once bathing water proves the plumbing.
3. Update README **Coverage** + the Hugging Face card splits (`floor-standards`, `shmi`, `bathing-water`, `ipc`); add the IPC manipulation finding to the "finding it's built to show" narrative as a second, complementary headline.

## New cross-cutting decisions to record
- **Manipulation-flagged admission** is a new mark *status* (alongside admitted/rejected). Add it to `pkg/schema` as an explicit field so consumers can filter clean vs flagged marks. This generalises beyond IPC and is a genuine schema improvement.
- **Licence-per-source** matters more now: IPC's mirror redistribution terms must be verified per source before pushing to Hugging Face (unlike the uniform OGL of the gov seams).