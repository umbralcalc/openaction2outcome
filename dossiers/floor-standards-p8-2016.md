# floor-standards-p8-2016

**Category:** identified (design-based truth — a pin)  ·  **Series:** floor-standards  ·  **Domain:** Education  ·  **Unit:** school  ·  **Design:** sharp RDD  ·  **Status:** ADMITTED

## The decision

- **Running variable:** progress_8_2016 — Progress 8 score, 2015/16 revised performance tables (P8 score)
- **Cutoff:** -0.5 (below-treated)
- **Action:** Below the Progress 8 floor standard (-0.5): flagged for intervention/scrutiny (support, possible academy order).
- **Alternative:** At or above the floor: no floor-triggered intervention.
- **Outcome:** progress_8_2018 — Progress 8 score two years later, 2017/18 revised performance tables (P8 score)
- **Estimand:** Sharp RD effect at -0.5 of being flagged below the 2016 Progress 8 floor on the school's 2017/18 Progress 8 (local-to-cutoff, complete cases).

## The effect

**0.0284** with a 95% interval of **[-0.0544, 0.2557]**.

The interval width separates into two sources:

| source | standard deviation |
|---|---|
| sampling (finite data) | 0.0311 |
| identification (bandwidth / order / kernel choice) | 0.0659 |
| **total** | **0.0729** |

## Validity checks

**Density / manipulation:** pass — binned-density-discontinuity (McCrary-style) (p = 0.381)

**Covariate continuity at the cutoff** (covariates should not jump):

| covariate | jump | p-value | pass |
|---|---|---|---|
| ks2_prior_attainment | 0.0555 | 0.626 | pass |
| pct_disadvantaged_fsm | 0.4044 | 0.801 | pass |
| ks4_cohort_size | 3.9518 | 0.586 | pass |

**Placebo cutoffs** (effect should vanish away from the real cutoff):

| placebo cutoff | estimate | indistinguishable from zero |
|---|---|---|
| -1.3 | 0.1243 | pass |
| 0.3 | -0.0139 | pass |

**Bandwidth sweep** (estimate vs window width):

| bandwidth | estimate |
|---|---|
| 0.3 | 0.0483 |
| 0.4 | 0.0337 |
| 0.5 | 0.0214 |
| 0.6 | 0.0139 |
| 0.7 | 0.0093 |
| 0.8 | 0.0056 |

**Donut robustness** (re-estimate after excluding units nearest the cutoff):

| donut radius | estimate |
|---|---|
| 0.05 | 0.0114 |
| 0.1 | -0.0500 |

**Notes.** SBI: Bayesian model average over 20 specs (bandwidth x order x kernel) via stochadex SMC (4000 particles, 8 rounds). Honest interval [-0.0544, 0.2557] (sd 0.0729) decomposes into sampling sd 0.0311 and identification sd 0.0659. For comparison, the plug-in local-linear interval is [-0.0627, 0.1055] (sd 0.0429) — narrower because it ignores between-spec identification uncertainty; this is the gap a model that reports only sampling SE should fail the calibration score. Design is effectively sharp: of 302 schools below -0.5, only 1 are excluded by the floor's CI condition (P8CIUPP>=0). DIFFERENTIAL ATTRITION CAVEAT: within +/-0.5 of the cutoff, 21.5% of below-floor schools vs 11.3% of above-floor schools lack a linked 2017/18 P8 (sponsored academies are re-issued a new URN). This attrition is correlated with treatment and biases the complete-case estimate; it is the dominant threat to this mark and motivates a future attrition-aware treatment.

## Data

The analysis-ready rows (one per unit) live in the single published `episodes` dataset, alongside every other mark's rows. Recover this mark's rows by filtering on `mark_id == "floor-standards-p8-2016"`. The dataset's download URL and content hash are in `datasets/episodes.manifest.json`.

- **Covariates (state):** ks2_prior_attainment, pct_disadvantaged_fsm, ks4_cohort_size

## Provenance

Point-in-time order: context as-of `2016-08-25` ≤ decision `2017-01-19` < outcome `2019-01-24`.

**Sources:**

- Key Stage 4 (KS4) final performance tables — school-level data, 2015/16 — Department for Education (DfE). Licence: Open Government Licence v3.0. SHA-256 `01288f9a4e39a9ad7c4a3dd6e88445d34664ea2dc36bb28adbb09d1c57046dd6`.
- Key Stage 4 (KS4) final performance tables — school-level data, 2017/18 — Department for Education (DfE). Licence: Open Government Licence v3.0. SHA-256 `6f0a6f5bb1154f94afe2463c25498e91700a5d561a80f19f8f2923d3f0e5bb6e`.

**Reproducibility:** go go1.25.2, openaction2outcome 0.5.0, smc particles=4000,rounds=8, stochadex v0.0.0-20260529062707-b3fa54eb7212. The mark and its data table re-mint byte-for-byte from the frozen inputs.
