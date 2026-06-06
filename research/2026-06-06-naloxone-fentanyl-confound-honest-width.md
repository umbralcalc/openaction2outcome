# Naloxone access laws → opioid mortality: the honest-width showcase (plug-in confidently wrong, honest accounting → not identified)

**Date:** 2026-06-06
**Status:** seam NOT minted — kept as the documented real-data demonstration of the
project's central claim. Data is all public-domain and GREEN; the obstruction is
identification (fentanyl confounding), which is the *point*.
**Series:** PLANS.md #3 `naloxone-overdose`. Mechanism (would have been) naloxone-access → opioid mortality.

## Why we tried it — and what it's for

The project's headline methodological claim is that an **honest interval must price in
modelling-choice uncertainty, not just sampling error** — which is why a plug-in
(sampling-only) method *under-covers* the truth (see the synthetic calibration study,
`scores/calibration-study.json`). Naloxone access laws → opioid-overdose mortality was
chosen (PLANS #3) as the place this is "most visible" in **real data**: a severe,
real confounder (the fentanyl supply wave) that a naive analyst would miss.

## The de-risk was GREEN on data, licence, and dates

- **Outcome**: drug-overdose death rate (+ standard error) by US state × year, 2008–2018,
  from NCHS via data.cdc.gov Socrata **`44rk-q6r2`** ("Drug Poisoning Mortality by State"),
  filtered to All Ages / Both Sexes / All Races. Machine-readable, **public domain**.
- **Fentanyl proxy**: synthetic-opioid (T40.4) vs total-opioid death counts by state, 2015+,
  from VSRR **`xkb8-kh2a`** — the load-bearing confounder join. Public domain.
- **Treatment dates**: all 50 states + DC enacted a naloxone access law by **1 Jul 2017**,
  staggered ~2013–2017 (PDAPS). Effective dates are facts (encodable as cited constants).

So nothing failed on data/licence/dates. The new **identification + coherence gate** —
the one the US-flavour and bathing-water findings taught us to apply *before* building —
is where it stops.

## The finding: plug-in is confidently wrong; the honest reading is "not identified"

A naive early- vs late-adopter DiD (early = first naloxone law by 2013; pre 2010–12 →
post 2015–18) on the NCHS rate:

```
early-adopter states Δ = +9.0   late-adopter states Δ = +3.9   →   naive DiD = +5.15 per 100k
```

Read literally with its tight sampling SE, this says **"naloxone access laws *increased*
overdose deaths by ~5 per 100k"** — confidently significant, and obviously absurd. It is
pure confounding:

- **The early-adopter states ARE the fentanyl-wave states.** The early-minus-late gap in
  the OD rate explodes through the adoption window and after it:

  | year | 2012 | 2013 | 2014 | 2015 | 2016 | 2017 | 2018 |
  |---|---|---|---|---|---|---|---|
  | early − late (per 100k) | +1.6 | +2.5 | +2.4 | +3.5 | **+6.3** | **+7.8** | +7.2 |

  The blow-up is 2016+ — the synthetic-opioid (fentanyl) era, which hit the NE/Appalachia
  early-adopter states far harder than the Plains/Mountain late-adopters.
- **Reverse causation**: states passed naloxone laws *in response to* already-rising deaths,
  so the "treated" group was on a steeper trajectory before treatment — pre-trends already
  diverge (**early +0.50 vs late +0.35 pp/yr**, 2008–2012). Parallel trends fail.

So a rigorous design's honest interval, once it folds in the **fentanyl-adjustment choice**
(adjusted ≈ small/null; unadjusted ≈ +5), the window, and the control set, is **enormous
and spans zero** — the honest conclusion is **"the naloxone effect is not identified here;
it is confounded by fentanyl."** The validity battery's job is exactly to **reject** the
confidently-wrong plug-in that a naive analyst would publish. That contrast — tight-and-
wrong vs wide-and-honest — is the whole thesis, and naloxone is its clearest real-data
instance.

## Disposition

- **Not minted.** A clean naloxone mark is not identifiable from these data (fentanyl
  confounding + reverse causation + near-universal fast adoption with almost no never-treated
  controls). Forcing an admitted mark would require asserting parallel trends that visibly
  fail; the honest outcome is non-identification.
- **Reproducible from public-domain Socrata queries** (no frozen file committed, consistent
  with the successful-marks-only publishing rule): NCHS `44rk-q6r2` (rate by state×year) and
  VSRR `xkb8-kh2a` (synthetic-opioid share). The early/late split here is approximate
  (literature/PDAPS) and used only for the demonstration; a full build would pin per-state
  effective dates as cited constants.
- The synthetic calibration study carries the methodological claim formally; this note is the
  **real-data witness** to it.

## Lesson

The de-risk-first gate is now three-part: **data/licence/dates**, then **identification**,
then (for bridges) **coherence**. Naloxone passes the first and fails the second by design —
and that failure, honestly surfaced, is the single best real-data demonstration that
plug-in confidence is not honesty. A wide interval that says "not identified" is the correct
answer here; a tight +5.15 is the seductive wrong one.
