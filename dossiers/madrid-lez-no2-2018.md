# madrid-lez-no2-2018

**Category:** identified (design-based truth — a pin)  ·  **Series:** madrid-lez-no2  ·  **Domain:** Environment  ·  **Unit:** air-quality-monitoring-station  ·  **Design:** controlled interrupted time series  ·  **Status:** ADMITTED

> The estimand is a **population** effect accumulated over the post-intervention window, not a local-at-cutoff effect. Its decision scores are never pooled with RDD marks.

## The decision

- **Action:** Madrid Central switch-on (30 Nov 2018): a ~472-ha access-restriction low-emission zone covering the Centro district barred non-resident combustion vehicles from the city core — older non-compliant vehicles banned (not charged).
- **Alternative:** Roads outside Madrid Central (the rest of the municipality): no access restriction, no emission-standard requirement.
- **Outcome:** no2_concentration — Monthly-mean NO2 concentration at the monitoring station (µg/m³)
- **Estimand:** Population effect over the post-intervention window (2018-12 to 2019-06, pre-moratorium and pre-COVID): the change in in-zone (Centro) NO2, net of a municipal urban/suburban-background control series sharing the pre-intervention trend.
- **Intervention instant:** 2018-11-30 (time axis: month, units month)
- **Pre-window:** 2017-01 → 2018-11
- **Post-window:** 2018-12 → 2019-06
- **Counterfactual:** segmented-regression on the treated-minus-control difference, seasonality: harmonic terms at the 12-month period (1–2 pairs across specs) — The difference of treated and control roadside NO2 nets out the shared regional/meteorological trend and most seasonality; a segmented regression on that difference reads the policy break (level + optional slope) against the extrapolated pre-trend, with residual seasonality carried by harmonic terms and serial correlation handled by Newey-West HAC errors.
- **Control series:** madrid-lez-no2 (parallel-trend) — Municipal urban/suburban-background stations away from the Madrid Central boundary share the in-zone station's airshed and pre-intervention trend but lie OUTSIDE the access restriction, so their series is the counterfactual for the regional/meteorological component of in-zone NO2. Traffic stations and near-boundary stations (Plaza de Espana, Escuelas Aguirre, Cuatro Caminos, Retiro) are excluded because the published Madrid Central evaluations find positive boundary spillover — near-zone stations are treatment-contaminated controls, not clean counterfactuals.

## The effect

**-11.6088** with a 95% interval of **[-17.0099, -6.432]**.

The interval width separates into two sources:

| source | standard deviation |
|---|---|
| sampling (finite data) | 2.5680 |
| identification (bandwidth / order / kernel choice) | 0.8152 |
| **total** | **2.6943** |

## Validity checks

**No anticipation (no pre-trend break / forestalling):** pass — no-anticipation placebo break 6 months before the instant (should be ≈0)

**Control parallelism (shared pre-intervention trend):** pass — pre-period parallel-trends slope of the treated-minus-control difference

**Placebo dates** (effect should vanish at fake pre-period dates):

| placebo date | estimate | indistinguishable from zero |
|---|---|---|
| 2017-09 | -1.6232 | pass |
| 2017-12 | -2.7762 | pass |
| 2018-03 | 0.2007 | pass |

**Window sweep** (estimate vs pre/post window length):

| window length | estimate |
|---|---|
| -48 | -10.7060 |
| -42 | -10.8470 |

**Transition exclusion** (re-estimate after dropping the implementation ramp):

| ramp width | estimate |
|---|---|
| 0 | -10.7060 |

**Autocorrelation modelled (Newey-West / ARMA errors):** pass — Newey-West HAC (Bartlett, lag 4)

**Notes.** Controlled ITS: model average over 8 specs (pre-window × harmonics × level/slope × meteorology) on the treated-minus-control monthly NO2 difference, Newey-West HAC (Bartlett lag 4) within each. Honest 95% interval [-17.010, -6.432] (sd 2.694) decomposes into sampling sd 2.568 and identification sd 0.815. Plug-in single-spec interval [-15.767, -5.645] (sd 2.582) is narrower because it ignores between-spec identification uncertainty. Treated = 1 in-zone (Madrid Central / Centro) station subject to the 2018 access-ban LEZ; control = 14 municipal urban/suburban-background stations away from the zone boundary kept after pre-trend matching (0 dropped: []). Effect is the extra change in treated roadside NO2 net of the control series over 7 post months (2018-12..2019-06). Meteorology-adjusted refit shifts the effect to -12.556 (Δ-1.850), confirming the control series already absorbs most short-window dispersion. Checks: control parallelism pass, no-anticipation pass, placebo dates pass. REGIME CAVEAT: Pinned to Madrid Central (30 Nov 2018, an access-BAN LEZ over the Centro core). This is a standard-BAN low-emission zone — the same regime family as the Berlin Umweltzone, kept SEPARATE from charge-type zones (London ULEZ) for any future bridge. The July 2019 sanction moratorium and July 2020 TSJM annulment fall AFTER the capped post window, so the effect is read on the fully-enforced regime. Madrid's pre-2018 APR Centro residential-priority area and episodic high-NO2 traffic protocols sit in the pre-window and are city-wide, so the controlled difference nets them out.

## Data

The analysis-ready panel rows (one per series × time bucket) live in the single published `episodes` dataset, alongside every other mark's rows. Recover this mark's rows by filtering on `mark_id == "madrid-lez-no2-2018"`; the row shape is `panel`. The dataset's download URL and content hash are in `datasets/episodes.manifest.json`.

- **Covariates (state):** wind_speed_kmh, temp_c, precip_mm

## Provenance

Point-in-time order: context as-of `2018-11-29` ≤ decision `2018-11-30` < outcome `2019-06-28`.

**Sources:**

- Madrid monthly-mean NO2 panel for the Madrid Central (LEZ) controlled-ITS (in-zone Centro station + municipal urban/suburban-background controls), 2017-01 to 2019-12, from the EEA verified (E1a) air-quality archive — European Environment Agency (EEA) — Air Quality Download Service (verified E1a); measurements reported by Spain (Madrid City Council municipal SIVPICA network). Licence: CC BY 4.0 (EEA); air quality measurements reported by Spain (Madrid City Council). SHA-256 `d3cd7480ecf930f5e5646791d433342990ad0d6e1ac173528d2167c9851d729f`.
- Central-Madrid monthly meteorology (temperature, wind speed + direction components, precipitation) for the Madrid Central controlled-ITS, 2017-01 to 2019-12 — Open-Meteo (ERA5 reanalysis, Copernicus Climate Change Service). Licence: CC BY 4.0 (Open-Meteo); contains modified Copernicus Climate Change Service information (ERA5). SHA-256 `c2f1c6806e0acdb3e6175bc507843d5639286096abb823748bb8669d71f728f1`.

**Reproducibility:** go go1.25.2, its controlled-segmented-regression,newey-west-lag=4,specs=8, openaction2outcome 0.5.0. The mark and its data table re-mint byte-for-byte from the frozen inputs.
