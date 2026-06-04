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

## Disposition

- Mechanism `emission-zone-stringency-to-roadside-no2` kept in the registry as a **declared
  seam, no admitted anchor yet** (see `pkg/schema/mechanism.go`).
- ULEZ build is wired (`--series ulez-no2`) and reproducible but **excluded from
  `make build-all`** (it reports NOT ADMITTED, never written to `marks/`).
- Re-entry path when/if clean data appears, or for a pre-COVID European LEZ (Berlin
  Umweltzone, Milan Area C) which would reuse this exact ITS machinery with a quieter
  outcome series.

## Lesson

The honest-interval discipline did its job: a naïve plug-in ITS would have reported a
confident ULEZ effect (−5 to −6 µg/m³); the placebo battery + model averaging revealed the
identification is not robust to the counterfactual's trend specification, and the clean
open-data needed to fix that is not available. A wide interval is information; an
unadmitted seam, honestly documented, is also information.
