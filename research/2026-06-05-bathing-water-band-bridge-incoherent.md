# Bathing-water classification bands are not a coherent bridge anchor family

**Date:** 2026-06-05
**Status:** bridge path REJECTED at the design-check stage (before any build). No data
harvested; the existing `bathing-water-poor-2015` mark is unaffected.
**Mechanism:** `bathing-water-classification`.

## Why we considered it

The corpus's headline capability is the **bridge mark** — interpolate an effect-curve
across ≥2 coherent anchors that bracket a query point on one policy axis. After the
cross-country menthol bridge failed on coherence (see
`research/2026-06-05-us-flavour-ban-smoking-null.md`), the bathing-water classification
looked like a clean *same-country, same-survey, same-regime* alternative: the revised
Bathing Water Directive (rBWD) classifies each designated water Excellent / Good /
Sufficient / Poor on the same log-compliance-margin axis, so its **three band boundaries**
(Poor/Sufficient — already minted; Sufficient/Good; Good/Excellent) seemed like a natural
anchor family on one axis. The data and RDD machinery already exist; no de-risk needed on
data, licence, or dates.

## The check: does each band boundary carry a treatment?

A bridge interpolates a *treatment effect* across the policy axis, so **every anchor must
carry a comparable treatment**. The rBWD says it does not — the action is **Poor-only**:

| boundary | what crossing it triggers | treatment? |
|---|---|---|
| **Poor / Sufficient** | mandatory "advice against bathing" sign the next season **+** EA-led targeted catchment investigation/remediation | **YES** (the minted mark) |
| Sufficient / Good | a classification **label/symbol** only — "to help people make informed choices" | no |
| Good / Excellent | a classification **label/symbol** only | no |

(Sources: EA "Bathing Water Quality" guidance; gov.uk classification pages.)

So only the Poor/Sufficient boundary has a mandated action. The Sufficient/Good and
Good/Excellent boundaries are label-only, so their RD effect on a site's later compliance
margin should be **≈0**.

## Why that kills the bridge

The "effect curve" across the margin axis is therefore not a smooth dose-response — it is a
**step**: a real effect below Sufficient (where the action bites) and ≈0 above it. A bridge
between, say, the Good/Excellent anchor (effect ≈0) and the Poor anchor (effect ≠0) would
linearly interpolate a non-zero effect for an intermediate-margin query — but the true
effect there is ≈0 (no action anywhere above Sufficient). The interpolation would
**mispredict**. The anchors share a *measurement* axis (compliance margin) but not a
*treatment* axis, which is what a bridge requires. This is the same coherence failure as the
cross-country menthol bridge — caught here **before** building, by asking the one question
the de-risk-first gate had been missing.

## Silver lining: principled placebos for the existing mark

The label-only boundaries are exactly what a **placebo cutoff** should be: a real
institutional threshold on the running variable that carries **no treatment**, so a valid
design must show ≈0 effect there. The current `bathing-water-poor-2015` mark uses arbitrary
placebo cutoffs at ±0.4 log-margin; swapping in the actual Sufficient/Good and
Good/Excellent boundaries would be a stronger, more defensible manipulation/placebo check on
data we already have. (A worthwhile enhancement to the identified mark — not a bridge.)

## Disposition & lesson

- No bridge here; the `bathing-water-classification` mechanism keeps its single Poor/
  Sufficient anchor.
- A valid bridge needs a same-mechanism **dose family** — same policy, REAL treatment at
  each anchor, anchors at DIFFERENT intensities that bracket the query, and mature outcomes.
  Multi-cutoff RDD where only one cutoff carries treatment does not qualify.
- **Lesson** (reinforcing the US-flavour finding): the de-risk-first gate must include an
  **identification + coherence** check, not just data/licence/dates. Here the check cost one
  web lookup and saved an entire build + a published-but-wrong bridge. Until a coherent dose
  family with a realised outcome appears (alcohol MUP once Scotland's 2024 65p raise matures;
  a minimum-wage or sugar-tax level family), the corpus delivers clean **standalone** marks.
