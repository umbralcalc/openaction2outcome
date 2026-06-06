# Madrid Central → in-zone NO₂: the second LEZ-ban anchor (bridge unlock)

*2026-06-06 · delivered mark `madrid-lez-no2-2018` (PLANS round-2, series 6)*

## What landed

`madrid-lez-no2-2018` — **Madrid Central** (30 Nov 2018, a ~472-ha access-**ban**
low-emission zone over the Centro district) on in-zone NO₂, minted as a controlled
ITS on the same machinery as Berlin (`ulezEvent`/`buildULEZ`, unchanged).

- **Effect −11.6 µg/m³, honest 95% [−17.0, −6.4]** (sampling sd 2.57, identification
  sd 0.82). The interval **excludes zero** — a large, design-identified in-zone
  reduction. Meteorology-adjusted refit lands at −12.6 (Δ−1.85), so the control series
  already absorbs most short-window dispersion and the effect is not a weather artefact.
- **Validity battery clean**: flat pre-trend (slope 0.006 µg/m³/month, n_pre=23),
  three null placebo dates (−1.6, −2.8, +0.2), no anticipation (−1.67, within noise),
  window sweep stable (−10.7 / −10.8). Admitted.
- **Data**: EEA **verified (E1a, 2013+)** parquet archive — the recent-data sibling of
  Berlin's historical (AirBase) archive — CC BY 4.0; measurements from the Madrid City
  Council SIVPICA municipal network. Frozen + hash-pinned by
  `scripts/madrid_lez_harvest.py`. Open-Meteo (ERA5) meteorology join, CC BY 4.0.

## Design choices that mattered

- **Treated = a single in-zone monitor** (`28079035` Plaza del Carmen, Centro) — the
  station the published Madrid Central evaluations use. One station makes the treated
  aggregate thin and the interval sampling-led, but the effect is large enough to
  exclude zero cleanly. This is honest, not a weakness.
- **Control = 14 municipal urban/suburban-background stations away from the boundary.**
  Traffic stations and **near-boundary** stations (Plaza de España, Escuelas Aguirre,
  Cuatro Caminos, Retiro) are **excluded**: the literature finds positive boundary
  spillover (traffic displaced to the ring), so near-zone stations are
  treatment-contaminated controls, not clean counterfactuals. 0 controls were dropped
  by the pre-trend anti-correlation guard — the background network is tightly parallel.

## The enforcement-moratorium seam (confirmed in the data)

The post window is **capped at 2019-06**, before the **July 2019 sanction moratorium**
softened enforcement and long before COVID. This is not just prudence — it is visible
in the raw treated-minus-control in-zone excess:

```
pre  (2017-01..2018-11):  ~12.0 µg/m³ excess
post (2018-12..2019-06):  ~1.6  µg/m³ excess   ← Madrid Central enforced
then (2019-07..2019-12):  creeps back to ~9-13 ← moratorium weakens treatment
```

A naïve analyst who ran the post window through 2019-12 (or to COVID) would dilute the
effect with the de-enforced regime and misread the policy. The short, fully-enforced
window is the identifying choice; the July 2020 TSJM annulment is a later, separate
regime (a possible future *reversal* anchor).

## Bridge status: the LEZ→NO₂ family now has two anchors

This is the round-2 unlock. The `lez-ban-stringency-to-roadside-no2` mechanism now has
**two identified anchors in the same design family** (`its-controlled`), bracketing the
same access-**ban** mechanism (not charge schemes):

| anchor | year | stringency | effect (µg/m³) | reading |
|---|---|---|---|---|
| Berlin Umweltzone stage 2 | 2010 | city-ring Euro-4-diesel sticker ban | −1.2 [−6.7, +4.8] | honest **null** |
| Madrid Central | 2018 | central-core access ban | −11.6 [−17.0, −6.4] | strong **negative** |

The two bracket a plausible **stringency → NO₂** curve: a modest ring-wide
Euro-standard tightening barely moved NOₓ (it cut soot/PM more), while a hard
central-core access restriction cut in-zone NO₂ sharply. A stochadex
dispersion-and-fleet-turnover simulator with a GP discrepancy pinned to both anchors
can now bridge intermediate stringency/city points (`category: bridge`,
`truth_source: simulator-bridged`), with LOAO coverage reported — see
[[bridge-gp-closed-form]] and [[bridge-deterministic-causal-layer]]. Mechanism
coherence holds because both are bans; charge-type zones (London ULEZ) stay separate.

**Caveat for the bridge build:** the two anchors differ in *what* they restrict (ring
Euro-standard vs central access) and the treated panels differ in thickness (2 vs 1
station). The bridge interpolates the *effect curve* of the shared ban mechanism, and
both calibration scores and the discrepancy GP operate on that curve regardless of the
single-station sampling width — but the simulator's stringency axis must encode "share
of in-zone km restricted × fleet non-compliance", not a single scalar, to place the two
anchors honestly.
