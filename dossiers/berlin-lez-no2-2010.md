# berlin-lez-no2-2010

**Category:** identified (design-based truth — a pin)  ·  **Series:** berlin-lez-no2  ·  **Domain:** Environment  ·  **Unit:** air-quality-monitoring-station  ·  **Design:** controlled interrupted time series  ·  **Status:** ADMITTED

> The estimand is a **population** effect accumulated over the post-intervention window, not a local-at-cutoff effect. Its decision scores are never pooled with RDD marks.

## The decision

- **Action:** Berlin Umweltzone stage 2 (1 Jan 2010): a green emissions sticker (Euro-4 diesel / Euro-1 petrol) became mandatory to drive inside the S-Bahn Ring — older non-compliant vehicles banned (not charged).
- **Alternative:** Roads outside the Umweltzone (outside the S-Bahn Ring): no emissions-sticker requirement.
- **Outcome:** no2_concentration — Monthly-mean NO2 concentration at the monitoring station (µg/m³)
- **Estimand:** Population effect over the post-intervention window (2010-01 to 2011-12, pre-COVID): the change in in-zone roadside NO2, net of an in-zone urban-background control series sharing the pre-intervention trend.
- **Intervention instant:** 2010-01-01 (time axis: month, units month)
- **Pre-window:** 2008-01 → 2009-12
- **Post-window:** 2010-01 → 2011-12
- **Counterfactual:** segmented-regression on the treated-minus-control difference, seasonality: harmonic terms at the 12-month period (1–2 pairs across specs) — The difference of treated and control roadside NO2 nets out the shared regional/meteorological trend and most seasonality; a segmented regression on that difference reads the policy break (level + optional slope) against the extrapolated pre-trend, with residual seasonality carried by harmonic terms and serial correlation handled by Newey-West HAC errors.
- **Control series:** berlin-lez-no2 (parallel-trend) — In-zone urban-background stations share the treated traffic stations' airshed and pre-intervention trend but receive far weaker DIRECT kerbside-traffic treatment, so their series is the counterfactual for the regional/meteorological component of in-zone NO2.

## The effect

**-1.2429** with a 95% interval of **[-6.7346, 4.8196]**.

The interval width separates into two sources:

| source | standard deviation |
|---|---|
| sampling (finite data) | 2.6367 |
| identification (bandwidth / order / kernel choice) | 1.2665 |
| **total** | **2.9251** |

## Validity checks

**No anticipation (no pre-trend break / forestalling):** pass — no-anticipation placebo break 6 months before the instant (should be ≈0)

**Control parallelism (shared pre-intervention trend):** pass — pre-period parallel-trends slope of the treated-minus-control difference

**Placebo dates** (effect should vanish at fake pre-period dates):

| placebo date | estimate | indistinguishable from zero |
|---|---|---|
| 2008-09 | -0.1835 | pass |
| 2009-01 | -3.0165 | pass |
| 2009-05 | -3.6012 | pass |

**Window sweep** (estimate vs pre/post window length):

| window length | estimate |
|---|---|
| -156 | -2.3636 |
| -150 | -2.9170 |

**Transition exclusion** (re-estimate after dropping the implementation ramp):

| ramp width | estimate |
|---|---|
| 0 | -2.3636 |

**Autocorrelation modelled (Newey-West / ARMA errors):** pass — Newey-West HAC (Bartlett, lag 4)

**Notes.** Controlled ITS: model average over 16 specs (pre-window × harmonics × level/slope × meteorology) on the treated-minus-control monthly NO2 difference, Newey-West HAC (Bartlett lag 4) within each. Honest 95% interval [-6.735, 4.820] (sd 2.925) decomposes into sampling sd 2.637 and identification sd 1.267. Plug-in single-spec interval [-6.622, 1.895] (sd 2.173) is narrower because it ignores between-spec identification uncertainty. Treated = 2 in-zone (S-Bahn Ring) traffic stations subject to the 2010 Umweltzone stage-2 ban; control = 4 in-zone urban-background stations kept after pre-trend matching (0 dropped: []). Effect is the extra change in treated roadside NO2 net of the control series over 24 post months (2010-01..2011-12). Meteorology-adjusted refit shifts the effect to -1.147 (Δ1.217), confirming the control series already absorbs most short-window dispersion. Checks: control parallelism pass, no-anticipation pass, placebo dates pass. REGIME CAVEAT: Pinned to the Berlin Umweltzone stage 2 (green-sticker Euro-4-diesel BAN inside the S-Bahn Ring). This is a standard-BAN low-emission zone — kept SEPARATE from charge-type zones (London ULEZ) for any future bridge. Germany's 2009 Umweltprämie scrappage scheme falls in the pre-window but is national, so the controlled difference nets it out.

## Data

The analysis-ready panel rows (one per series × time bucket) live in the single published `episodes` dataset, alongside every other mark's rows. Recover this mark's rows by filtering on `mark_id == "berlin-lez-no2-2010"`; the row shape is `panel`. The dataset's download URL and content hash are in `datasets/episodes.manifest.json`.

- **Covariates (state):** wind_speed_kmh, temp_c, precip_mm

## Provenance

Point-in-time order: context as-of `2009-12-31` ≤ decision `2010-01-01` < outcome `2011-12-28`.

**Sources:**

- Berlin monthly-mean NO2 panel for the Umweltzone stage-2 controlled-ITS (in-zone traffic + in-zone urban-background stations), 2008-01 to 2011-12, from the EEA historical (AirBase) archive — European Environment Agency (EEA) — Air Quality Download Service (historical/AirBase); measurements reported by Germany (Umweltbundesamt / Berlin Senate BLUME network). Licence: CC BY 4.0 (EEA); air quality measurements reported by Germany (UBA / Berlin Senate). SHA-256 `a2aedbd13aac4065bdc08461ff0e9a978d25d83550cade5f37ee5a19f7beec67`.
- Central-Berlin monthly meteorology (temperature, wind speed + direction components, precipitation) for the Umweltzone controlled-ITS, 2008-01 to 2011-12 — Open-Meteo (ERA5 reanalysis, Copernicus Climate Change Service). Licence: CC BY 4.0 (Open-Meteo); contains modified Copernicus Climate Change Service information (ERA5). SHA-256 `54957b0b1d6ceea1a8b5910dad5fc84f802b802d3b6c33440d7bd3032f75d004`.

**Reproducibility:** go go1.25.2, its controlled-segmented-regression,newey-west-lag=4,specs=16, openaction2outcome 0.5.0. The mark and its data table re-mint byte-for-byte from the frozen inputs.
