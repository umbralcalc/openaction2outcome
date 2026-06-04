# ULEZ → roadside NO₂ controlled-ITS: why it is a declared seam, not an admitted mark

**Date:** 2026-06-04
**Status:** seam SHELVED — declared but not identified. Infrastructure retained.
**Series:** `ulez-no2`. Mechanism `emission-zone-stringency-to-roadside-no2`.

## Summary

We implemented the full controlled-interrupted-time-series (ITS) machinery for the
London ULEZ → roadside-NO₂ seam and ran it on real London Air Quality Network (LAQN)
data for both the 2023 London-wide expansion and the 2019 central expansion. **Neither
yields an admitted mark.** The obstruction is not a bug and not (only) COVID; it is that
the treated-minus-control NO₂ difference carries its own non-linear secular dynamics that
a clean, reproducibly-licensed open-data confounder series cannot be found to absorb. We
record the seam as *declared but not identified* (the same status as the area-funding
mechanism) and keep the estimator/ingest/harvest infrastructure for a future cleaner
anchor (e.g. a pre-COVID European LEZ).

## What was built (retained)

- `internal/its` — controlled segmented-regression estimator on the treated−control
  difference, Newey-West (Bartlett) HAC standard errors, and a model-averaged honest
  interval (pre-window × harmonics × level/slope × meteorology), mirroring `sbi.BMAResult`.
  Validated against synthetic truth (`its_test.go`): recovers a known break, the honest
  interval covers and decomposes into sampling + identification variance, placebos clean.
- `internal/ingest/laqn.go` — frozen monthly NO₂ panel + meteorology loaders.
- `internal/series/ulezno2.go` (+helpers) — event-parametrised builder (`ulezEvent`):
  `BuildULEZNO2` → 2019 central; `BuildULEZNO22023` → 2023 London-wide.
- `scripts/ulez_harvest.py --event 2019|2023` — resumable LAQN hourly→monthly harvest +
  Open-Meteo join, per-station partial cache, broad-network-error tolerant.

## The two events and why each fails

### 2023 London-wide expansion (29 Aug 2023)
- Treated = 11 outer-London roadside/kerbside (newly covered); control = 23 urban-background.
- Effect ≈ **+1.2 µg/m³** (no reduction), honest interval spans 0. Consistent with the
  outer-London fleet being ~95 % compliant by 2023 (scrappage + natural renewal front-ran
  the policy), so the marginal NO₂ effect was negligible.
- A placebo at a fake Jan-2023 instant shows a spurious **−3.2** break: the
  roadside-minus-background gap declines **non-linearly** because the pre-window (2021–22)
  **is the post-COVID traffic rebound**. So COVID does not merely bound the analysis — it
  *creates* the curvature that defeats identification. Not admitted.

### 2019 central expansion (8 Apr 2019)
- Treated = 10 central congestion-charge-zone roadside/kerbside (Marylebone Rd, Oxford St,
  Strand, City of London…); control = 20 outer-London roadside, **untreated until 2023** —
  a clean same-road-type, never-treated-in-period control. Window 2017-04 → 2020-02, i.e.
  **entirely pre-COVID** (capped before the Mar-2020 collapse).
- Raw central−outer difference steps from a pre-mean of **32.3** to a post-mean of
  **20.9 µg/m³** (−11.4); the plug-in segmented estimate is clearly negative (≈ **−5 to
  −6 µg/m³**), in line with the literature's ~30 % central-roadside reduction.
- BUT placebos at 2017-12 / 2018-04 / 2018-08 read **+9.2 / −0.9 / −9.1** — failing in
  *opposite directions* around a clean midpoint. That is the unmistakable signature of a
  **curved (non-linear) pre-trend** a straight-line counterfactual cannot fit: central
  London was already improving on its own curved trajectory (Oct-2017 T-Charge, taxi
  delicensing, bus electrification) before the ULEZ. So here the confounder is **not**
  COVID (the window is pre-COVID) — it is the pre-existing curved fleet-renewal decline.

## Why we did not "rescue" 2019 with confounder joins

Methodologically, the fix is sound and is the project's intended approach: join the smooth
confounder that traces the curve (monthly fleet-compliance / emission-factor trajectory,
traffic volume) and let the segmented model read the ULEZ *step* against it. A step is
separable from a smooth trend given a covariate that traces the trend.

The blocker is **data availability + licensing**, not method:

- The monthly central-London **fleet-compliance series exists** (TfL ANPR, ~39 % in Feb
  2017 rising through 2019) — but **only inside scanned-image PDF evaluation reports**
  (GLA london.gov.uk / TfL). The London Datastore ULEZ dataset carries **only boundary
  geometry** (OGL), no compliance/traffic series. DfT open road-traffic data is **annual**
  AADF, not monthly.
- **Licensing:** the GLA evaluation reports are **not** under an OGL grant (london.gov.uk
  terms assert plain copyright; unlike the Datastore datasets or GOV.UK). The numeric
  values are facts (not copyrightable, so transcribable), but we could not honestly label a
  hand-transcribed-from-a-PDF series as "OGL v3" the way every other corpus input is. TfL
  originates the data and publishes open data under OGL, but not this historical monthly
  series in machine-readable form.

So the one series that would rescue identification fails the corpus bar on **two** counts:
non-reproducible provenance (hand-transcribed off a scanned chart) and no clean open
licence. Putting that at the load-bearing centre of a mark would undercut exactly what
makes the corpus trustworthy. We declined to do so.

## Source data (recorded here only — NOT published to R2)

Because this seam yields no admitted mark, its frozen inputs are deliberately kept
OUT of `data/raw/` and the object-store mirror — only successful marks' data is
published. The inputs are recorded here for reproducibility; re-create them with
`scripts/ulez_harvest.py --event 2019|2023` (LAQN hourly NO2 → monthly + Open-Meteo),
which produces byte-identical files matching these hashes:

| id | publisher | licence | bytes | sha256 |
|----|-----------|---------|-------|--------|
| `ulez-no2-laqn` (2023 window) | London Air Quality Network (Imperial ERG) | OGL v3.0 | 160176 | `3a00628c0fe94c323803cbad32167dffc7d52242e209220bca1c0eaf48611345` |
| `ulez-no2-laqn-2019` | London Air Quality Network (Imperial ERG) | OGL v3.0 | 110610 | `9ae84a6c95c4647e3fb42bdb5245003460f1d96ec58327859acf513a6992fb60` |
| `ulez-no2-meteo` (2023 window) | Open-Meteo (ERA5/Copernicus) | CC BY 4.0 | 2551 | `5fbed8e25ef9c00ada9d557195ad10ae469eb360268af311c152129062fa8d81` |
| `ulez-no2-meteo-2019` | Open-Meteo (ERA5/Copernicus) | CC BY 4.0 | 1944 | `f4b7f8f8323a9c1fd5ea792a5bb3144a692e44af4a97a2b83c9fcacb55252076` |

- LAQN NO2: `https://api.erg.ic.ac.uk/AirQuality/Data/SiteSpecies/` (hourly per station/year → monthly).
- Open-Meteo: `https://archive-api.open-meteo.com/v1/archive` (central-London point).
- Station panels (treated/control codes) are pinned in `scripts/ulez_harvest.py`'s `EVENTS` table.

## Disposition

Because this seam produced no admitted mark, it is documented HERE only — not carried in
the active corpus:

- **No `data/raw` pointers, no R2 publishing** — the frozen inputs are recorded in the
  table above, not staged for the object-store mirror (only successful marks' data is
  published). Re-create with `scripts/ulez_harvest.py` to reproduce the finding.
- **No build wiring / no mechanism registry entry** — the `--series ulez-no2` commands and
  the charge-type `emission-zone-stringency-to-roadside-no2` mechanism were removed once the
  seam was shelved. The shared, generic LEZ→NO2 controlled-ITS machinery
  (`internal/its`, the event-parametrised builder in `internal/series/ulezno2.go`,
  `scripts/ulez_harvest.py`) is RETAINED — it is what mints the admitted Berlin anchor.
- **Re-entry path**: a future charge-type LEZ with clean pre-COVID data (e.g. Milan Area C,
  with the confounder caveats noted) would re-introduce a charge mechanism and reuse this
  same machinery. The delivered LEZ anchor today is the BAN-type Berlin Umweltzone
  (`berlin-lez-no2-2010`, mechanism `lez-ban-stringency-to-roadside-no2`).

## Lesson

The honest-interval discipline did its job: a naïve plug-in ITS would have reported a
confident ULEZ effect (−5 to −6 µg/m³); the placebo battery + model averaging revealed the
identification is not robust to the counterfactual's trend specification, and the clean
open-data needed to fix that is not available. A wide interval is information; an
unadmitted seam, honestly documented, is also information.
