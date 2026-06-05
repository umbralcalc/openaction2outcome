# ca-menthol-smoking-2016

**Category:** identified (design-based truth — a pin)  ·  **Series:** ca-menthol-smoking  ·  **Domain:** Health  ·  **Unit:** province  ·  **Design:** difference-in-differences  ·  **Status:** ADMITTED

> The estimand is the **ATT** — the average effect on the treated group, identified by comparing its pre→post change to a control group under parallel trends. Not a local-at-cutoff effect; never pooled with RDD marks.

## The decision

- **Action (treated):** Provincial menthol-cigarette ban (2015-2017): the province prohibited the sale of menthol-flavoured cigarettes ahead of the federal ban.
- **Alternative (control):** No provincial menthol ban — menthol cigarettes remained legal until the federal ban took effect on 2 Oct 2017.
- **Outcome:** current_smoking_prevalence — Adult (12+) current-smoker (daily or occasional) prevalence, CCHS (percent)
- **Time axis:** year — Calendar year (the DiD time axis)
- **Estimand:** ATT of a provincial menthol-cigarette ban on adult current-smoking prevalence (treated provinces vs federal-only control provinces, 2007-2014 pre vs 2016-2017 post), under parallel trends.

## The effect

**-1.7889** with a 95% interval of **[-3.0876, -0.4901]**.

The interval width separates into two sources:

| source | standard deviation |
|---|---|
| sampling (unit-clustered) | 0.5523 |
| specification (pre/post window choice) | 0.3661 |
| **total** | **0.6626** |

## Validity checks

**parallel_trends:** pass — cross-group pre-period trend slope of (treated − control) smoking prevalence (should be ≈0) — pre-trend slope -0.802 pp/yr (se 1.689): trends are statistically parallel

**placebo_pre_period_ban:** pass — fake menthol ban at 2012 using pre-period data only (should be ≈0) — placebo ATT 1.631 pp (sd 0.996): pass

**leave_one_province_out:** pass — drop each treated province in turn; the ATT sign should be stable — leave-one-out ATT ranges [-2.106, -1.431] vs full -1.789; sign stable=true

**Placebo (fake pre-period treatment date)** (effect should vanish):

| placebo year | estimate | indistinguishable from zero |
|---|---|---|
| 2012 | 1.6310 | pass |

**Window sweep** (ATT vs pre/post half-width):

| half-width | estimate |
|---|---|
| 2 | -0.9000 |
| 3 | -1.7889 |
| 4 | -1.3595 |
| 5 | -1.4448 |

**Notes.** Difference-in-differences (unit-clustered, window sweep folded into the interval): 7 treated provinces with a provincial menthol ban (NS/PE/AB 2015, NB/QC 2016, ON/NL 2017) vs 3 control provinces covered only by the federal 2 Oct 2017 ban (BC, MB, SK). ATT = -1.789 pp on adult current-smoking prevalence, honest 95% interval [-3.088, -0.490] (sd 0.663 = sampling 0.552 + specification 0.366). Pre-period 2007-2014 (spans and differences out the 2015 CCHS redesign), post window 2016-2017 (before the federal ban universalises treatment); 2015 dropped as the rollout ramp. Checks: parallel pre-trends pass (cross-group pre-slope -0.802), placebo pre-period ban pass. CAVEATS: the federal Oct-2017 ban caps the clean post window at 2017; the effect is on TOTAL smoking, diluted by substitution to non-menthol products, so it is smaller than the menthol-specific reductions in the literature; with only 3 control provinces the unit-clustered interval is wide.

## Data

The analysis-ready panel rows (one per unit × period) live in the single published `episodes` dataset, alongside every other mark's rows. Recover this mark's rows by filtering on `mark_id == "ca-menthol-smoking-2016"`; the row shape is `panel`. The dataset's download URL and content hash are in `datasets/episodes.manifest.json`.

## Provenance

Point-in-time order: context as-of `2014-12-31` ≤ decision `2015-05-31` < outcome `2017-12-31`.

**Sources:**

- Adult (12+) current-smoking prevalence by province and year, Canada 2007-2019 (CCHS, stitched across the 2015 redesign) — Statistics Canada. Licence: Statistics Canada Open Licence. SHA-256 `8d87bdd566bfa0a77bec28cf18f61c71b2178457d2df03627e956463ad0f6299`.

**Reproducibility:** did unit-clustered-2x2,split=2015,windows=[2 3 4 5], go go1.25.2, openaction2outcome 0.5.0. The mark and its data table re-mint byte-for-byte from the frozen inputs.
