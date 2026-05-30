# shmi-higher-than-expected-banding

**Series:** shmi  ·  **Domain:** Health  ·  **Unit:** nhs-trust  ·  **Design:** sharp RDD  ·  **Status:** ADMITTED

## The decision

- **Running variable:** shmi_minus_od_upper — SHMI minus the overdispersed upper control limit (positive => banded 'higher than expected') (SHMI ratio)
- **Cutoff:** 0 (above-treated)
- **Action:** Publicly banded 'higher than expected' mortality (SHMI above the overdispersed upper control limit) — a 'smoke alarm' that raises scrutiny.
- **Alternative:** Banded 'as expected' (SHMI within the control limits): no higher-than-expected flag.
- **Outcome:** shmi_next_window — The trust's SHMI in the following non-overlapping 12-month window (SHMI ratio)
- **Estimand:** Sharp RD (intention-to-treat) effect at the upper control limit of being banded 'higher than expected' on the trust's SHMI in the following 12-month window (pooled trust-years, local-to-cutoff).

## The effect

**-0.0131** with a 95% interval of **[-0.0658, 0.0177]**.

The interval width separates into two sources:

| source | standard deviation |
|---|---|
| sampling (finite data) | 0.0136 |
| identification (bandwidth / order / kernel choice) | 0.0195 |
| **total** | **0.0238** |

## Validity checks

**Density / manipulation:** pass — binned-density-discontinuity (McCrary-style) (p = 0.623)

**Covariate continuity at the cutoff** (covariates should not jump):

| covariate | jump | p-value | pass |
|---|---|---|---|
| expected_deaths | -48.3127 | 0.855 | pass |

**Placebo cutoffs** (effect should vanish away from the real cutoff):

| placebo cutoff | estimate | indistinguishable from zero |
|---|---|---|
| -0.2 | -0.0074 | pass |
| -0.1 | 0.0056 | pass |

**Bandwidth sweep** (estimate vs window width):

| bandwidth | estimate |
|---|---|
| 0.08 | -0.0299 |
| 0.12 | -0.0249 |
| 0.15 | -0.0244 |
| 0.2 | -0.0265 |
| 0.25 | -0.0258 |

**Donut robustness** (re-estimate after excluding units nearest the cutoff):

| donut radius | estimate |
|---|---|
| 0.02 | -0.0325 |
| 0.04 | -0.0473 |

**Notes.** Sharp intention-to-treat RDD on the SHMI 'higher than expected' banding, pooled over 3 non-overlapping reporting-window pairs (trust-years). Effect of being publicly flagged (SHMI above the overdispersed upper control limit) on the trust's SHMI in the following 12-month window. SBI honest interval [-0.0658, 0.0177] (sd 0.0238) splits into sampling sd 0.0136 and identification sd 0.0195; the plug-in interval is [-0.0616, 0.0128] (sd 0.0190). SMALL-N CAVEAT: only ~120 acute trusts per window, so the interval is wide by design. The RDD nets out SHMI's mean reversion (smooth through the cutoff); the flagging effect is the discontinuity. Pooling assumes a stable effect across windows and treats trust-years as units (within-trust serial correlation is not modelled, which understates the sampling component). COVID windows (Apr 2019 - Mar 2022 decisions) are excluded. This is intention-to-treat on the banding label, not the effect of any specific downstream intervention.

## Data

The analysis-ready episode table (one row per unit) is published separately:

- **URL:** https://pub-8d0395b8e53947d791b1e20255172cc3.r2.dev/marks/shmi-higher-than-expected-banding/episodes.csv.gz
- **SHA-256:** `bdf9ef119fff440dd07684f6dad139abfc9fd8f47d9ad5cc35d77476605f915c`
- **Rows:** 368  ·  **Format:** csv.gz
- **Columns:** unit_id, unit_name, running_value, assigned, treated, outcome, expected_deaths, decision_shmi

## Provenance

Point-in-time order: context as-of `2018-04-01` ≤ decision `2024-03-31` < outcome `2025-03-31`.

**Sources:**

- Summary Hospital-level Mortality Indicator (SHMI) — historical trust-level data (release Oct 2024 - Sep 2025) — NHS England (NHS Digital). Licence: Open Government Licence v3.0. SHA-256 `0e67d902cab0084c9f7a61a67de4e8b94f21b06b45cfeb1c50982f48f45e2a5f`.

**Reproducibility:** go go1.25.2, openaction2outcome 0.3.0, smc particles=4000,rounds=8, stochadex v0.0.0-20260529062707-b3fa54eb7212. The mark and its data table re-mint byte-for-byte from the frozen inputs.
