# US comprehensive flavour bans → smoking: a null, and why the cross-country menthol bridge isn't viable

**Date:** 2026-06-05
**Status:** seam NOT minted — US flavour-ban → total-smoking is a null; the intended
cross-country menthol bridge does not hold. Harvest script retained; data documented
here only (not staged under `data/raw` or published).
**Series:** PLANS.md #2 `flavour-ban-smoking`. Mechanism `menthol-restriction-to-smoking`.

## Why we tried it

The just-delivered Canadian mark (`ca-menthol-smoking-2016`, ATT **−1.79 pp**) is one
anchor on `menthol-restriction-to-smoking`. The US comprehensive flavour bans —
Massachusetts (1 Jun 2020) and California (21 Dec 2022) — were meant to be the
**higher-intensity** anchors, so that menthol-only (Canada) ↔ comprehensive (US) would
span a **restriction-intensity axis** and bracket an intermediate query — the corpus's
first **bridge mark**.

## The de-risk was GREEN on all three gates

Unlike ULEZ/CQC, every gate passed cleanly:

- **Outcome data**: adult current-smoking by US state × year, **2011–2024**, with 95%
  confidence limits (→ SEs) and sample sizes, from the CDC BRFSS via the data.cdc.gov
  Socrata dataset **`dttw-5yxu`** ("Current Smoker Status" = Yes, break-out "Overall").
  Machine-readable API, **public domain** (17 U.S.C. § 105; BRFSS no-linkage use
  restriction noted, irrelevant to aggregate DiD). Frozen panel (51 states incl DC):
  `sha256 c976077e89d0d0f061a87493be49c9c467efa8a9893ae7dbd7dd0abe493ba43a`,
  30456 bytes, 709 rows — reproducible via `scripts/usflavour_harvest.py`.
- **Treatment dates**: MA 1 Jun 2020, CA 21 Dec 2022 — the only statewide comprehensive
  menthol-cigarette bans in window (DC 2022 also; NJ's is 2026). Authoritative
  (Tobacco-Free Kids / Public Health Law Center / Canada Gazette analogues).
- **Control purity**: clean pool of ~45 states after excluding treated (MA, CA, DC) and
  the few with notable *local* menthol bans (MN, CO, IL).

So the obstruction below is **identification**, not data or licence.

## The finding: US comprehensive flavour bans → TOTAL smoking is a null

**Naive DiD (treated vs all clean states) is confounded** by a floor effect. MA and CA
are low-smoking states (~13 % / 11 % in 2016) versus the control average (~18 %). In the
COVID era (2020+) the high-smoking control states fell *faster* (more room; secular
convergence) — controls dropped ~5.9 pp 2016→2024 vs ~3–5 pp in MA/CA. So the naive DiD
reads a spurious **positive** "effect" (treated fell *less*): MA ATT +0.6 pp (perm
p=0.46), CA ATT +2.0 pp (perm p=0.02, wrong-signed). That is differential state-level
convergence, not the ban.

**Baseline-matched controls fix the confound** — and the answer is null. Matching MA/CA
to other low-smoking non-ban states (UT, CT, NJ, NY, MD, WA, HI, NH, RI, VT; matched-avg
14.1 %), with permutation (placebo-state) inference:

| event | matched-control DiD ATT | perm p | pre-trend slope | reading |
|---|---|---|---|---|
| **MA** (2020) | **+0.22 pp** | 0.73 | −0.04 pp/yr (parallel) | clean **null** |
| **CA** (2022) | +1.55 pp | 0.091 | −0.21 pp/yr | null-ish (2 post yrs, mild pre-divergence) |

So a comprehensive flavour ban produced **no detectable change in total adult smoking
prevalence** — the substitution story (menthol/flavoured → non-flavoured, little net
quitting). The literature's larger effects are **menthol-specific**, which BRFSS total
current-smoking does not isolate.

## Why the cross-country menthol bridge isn't viable

The bridge premise was that restriction intensity maps monotonically to a smoking effect.
The anchors say otherwise:

- Canada **menthol-only** → **−1.8 pp**
- US **comprehensive** → **≈ 0**

A *stronger* restriction shows a *smaller* effect — **non-monotonic**, so the anchors do
not lie on one coherent effect-curve. And mechanism **coherence** fails independently:
different surveys (CCHS 12+ vs BRFSS 18+), populations (provinces vs states), and eras
(2016–17 vs COVID-era 2021–24). A bridge requires anchors agreeing on policy variable,
outcome construct, population, and regime; these agree on none of the last three. The
cross-country framing was the flaw — bridges need a coherent **same-country, same-survey**
anchor family.

## Disposition

- **Not minted.** US flavour-ban → total smoking is a null, and it does not enable the
  intended bridge. The two US events *are* potentially valid **standalone honest nulls**
  (MA especially), but minting them needs **single-treated-unit inference** (matched /
  synthetic controls + permutation — `internal/did`'s unit-clustered SE is invalid with
  one treated unit) — new estimator machinery — for a null that advances nothing.
- **Data not published.** The frozen BRFSS panel lives only in `data/cache` (gitignored);
  no `data/raw` pointer, nothing staged for R2 — consistent with the "successful marks
  only" publishing rule. `scripts/usflavour_harvest.py` is retained for reproducibility.
- The Canadian anchor (`ca-menthol-smoking-2016`) stands on its own on the mechanism.

## Lesson

The de-risk-first discipline worked perfectly on **data, licence, and dates** — all green,
fast, clean. But it does not test **identification** or **bridge coherence**, which only
the estimate reveals. A clean dataset can still yield a confounded or premise-breaking
result. The honest negative — "comprehensive flavour bans did not move total smoking, and
a cross-country intensity bridge is not coherent" — is itself a finding worth recording.
