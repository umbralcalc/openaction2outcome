# bathing-water-poor-2015

**Series:** bathing-water  ·  **Domain:** Environment  ·  **Unit:** bathing-water  ·  **Design:** sharp RDD  ·  **Status:** ADMITTED

## The decision

- **Running variable:** compliance_margin_2015 — Base-10 log of the worst indicator's 90th-percentile statistic over its Sufficient threshold (2012-2015 rolling window); >0 means classified Poor (log10 ratio)
- **Cutoff:** 0 (above-treated)
- **Action:** Classified Poor (fails the Sufficient standard): mandatory advice-against-bathing sign the following season + EA-led catchment investigation/remediation.
- **Alternative:** Classified Sufficient or better: no Poor-triggered advice sign or investigation.
- **Outcome:** compliance_margin_2019 — Same site's log compliance margin four years later (2016-2019 rolling window) (log10 ratio)
- **Estimand:** Sharp RD effect at the Poor/Sufficient boundary of being classified Poor in 2015 (advice sign + investigation) on the site's 2019 log compliance margin (local-to-cutoff, complete cases).

## The effect

**-0.0947** with a 95% interval of **[-0.4068, 0.2451]**.

The interval width separates into two sources:

| source | standard deviation |
|---|---|
| sampling (finite data) | 0.1113 |
| identification (bandwidth / order / kernel choice) | 0.1306 |
| **total** | **0.1716** |

## Validity checks

**Density / manipulation:** pass — binned-density-discontinuity (McCrary-style) (p = 0.710)

**Covariate continuity at the cutoff** (covariates should not jump):

| covariate | jump | p-value | pass |
|---|---|---|---|
| ie_sample_count | 11.4507 | 0.152 | pass |
| is_inland | -0.0135 | 0.735 | pass |
| impacted_by_heavy_rain | -0.5609 | 0.072 | pass |

**Placebo cutoffs** (effect should vanish away from the real cutoff):

| placebo cutoff | estimate | indistinguishable from zero |
|---|---|---|
| -0.4 | -0.0023 | pass |
| 0.4 | -1.2872 | pass |

**Bandwidth sweep** (estimate vs window width):

| bandwidth | estimate |
|---|---|
| 0.15 | 0.0433 |
| 0.2 | 0.0300 |
| 0.3 | -0.0272 |
| 0.4 | -0.1029 |
| 0.5 | -0.1443 |

**Donut robustness** (re-estimate after excluding units nearest the cutoff):

| donut radius | estimate |
|---|---|
| 0.02 | -0.1231 |
| 0.05 | -0.1259 |

**Seam-specific checks:**

- **abnormal_sample_exclusion:** pass — abnormal-sample-exclusion sensitivity (re-include discounted samples, re-derive 90th-percentile margin near the cutoff)
  - 201 discountable samples in the 2012-2015 windows of near-cutoff sites; 35 of 87 near-cutoff sites reconstructed. Re-including them shifts the running variable by at most 0.350 log10 and flips Poor/Sufficient status for 9 site(s). Re-estimated effect 0.0121 vs baseline -0.0272 (delta 0.0394, within 1.96*baseline sd 0.3325 = true). mean |kept-only minus official| margin = 0.038 over 35 sites (method-fidelity check).

**Notes.** SBI: Bayesian model average over 20 specs (bandwidth x order x kernel) via stochadex SMC (4000 particles, 8 rounds). Honest interval [-0.4068, 0.2451] (sd 0.1716) decomposes into sampling sd 0.1113 and identification sd 0.1306. Plug-in local-linear interval [-0.3598, 0.3053] (sd 0.1697) is narrower because it ignores between-spec identification uncertainty. Running variable is the base-10 log compliance margin (worst of EC/IE 90th-percentile over its Sufficient threshold); margin>0 is Poor. Sharp design: of 413 decision-year sites with a usable margin, 11 are classified Poor. Checks: density/manipulation pass (p=0.710), covariate continuity pass. Decision window 2012-2015, outcome window 2016-2019 (non-overlapping; both pre-COVID). 201 discountable samples in the 2012-2015 windows of near-cutoff sites; 35 of 87 near-cutoff sites reconstructed. Re-including them shifts the running variable by at most 0.350 log10 and flips Poor/Sufficient status for 9 site(s). Re-estimated effect 0.0121 vs baseline -0.0272 (delta 0.0394, within 1.96*baseline sd 0.3325 = true). mean |kept-only minus official| margin = 0.038 over 35 sites (method-fidelity check). REGIME CAVEAT: marks are pinned to the action regime in force at the 2015 decision (automatic advice-against-bathing sign + EA investigation on a Poor classification); the 2021 amendment made 5-consecutive-Poor de-designation a Ministerial decision, shifting the downstream regime for later cohorts.

## Data

The analysis-ready episode table (one row per unit) is published separately:

- **URL:** https://pub-8d0395b8e53947d791b1e20255172cc3.r2.dev/marks/bathing-water-poor-2015/episodes.csv.gz
- **SHA-256:** `c8d2e34b886d64cc16e8f2eefc163221bb0c34c8d55b79bf675d5a302a237e02`
- **Rows:** 413  ·  **Format:** csv.gz
- **Columns:** unit_id, unit_name, running_value, assigned, treated, outcome, ie_sample_count, is_inland, impacted_by_heavy_rain

## Provenance

Point-in-time order: context as-of `2015-12-31` ≤ decision `2016-03-01` < outcome `2019-11-21`.

**Sources:**

- Bathing-water annual compliance classifications + E. coli / intestinal enterococci percentile statistics, England 2015-2024 (revised Bathing Water Directive) — Environment Agency (Defra). Licence: Open Government Licence v3.0. SHA-256 `faec1609564a1a170f62aed3846f4a33ba5e92ce8995cc1027fad5d2554bc0cc`.
- Bathing-water per-sample microbiology (E. coli / intestinal enterococci counts + discountable flag) for near-cutoff sites, England 2015-2024 — Environment Agency (Defra). Licence: Open Government Licence v3.0. SHA-256 `95a104b64d07ab5b0c69199ff00b529b2d6b03629b57e4971c093032bc9f0fe2`.

**Reproducibility:** go go1.25.2, openaction2outcome 0.3.0, smc particles=4000,rounds=8, stochadex v0.0.0-20260529062707-b3fa54eb7212. The mark and its data table re-mint byte-for-byte from the frozen inputs.
