# openaction2outcome — build plans, round 2

Three further candidates, chosen to match the marks that *landed* (Berlin LEZ
NO₂ as a pre-COVID ITS with an in-zone background control; Canadian menthol as
a clean cross-province DiD). Two extend mechanism families you've already
started — pushing each toward the ≥2-anchor threshold a bridge needs — and one
opens a fresh, well-identified mechanism.

A reproducibility note that shaped these picks: the canonical UK 2007
smoke-free MI study used Hospital Episode Statistics weekly admissions for
2002–2008, but the **open** monthly NHS England activity series only begins
April 2008 — so that exact design is *not reproducible from open data*. The
smoke-free plan below routes around this via CDC sources that publish
continuously from 1999. Where a design can't be rebuilt from frozen open
inputs, it doesn't belong in the collection; that constraint is doing useful
work here.

Licence posture: EEA air quality + Open-Meteo = CC BY 4.0 (already in your
footer); CDC STATE System + WONDER = U.S. public domain (record the WONDER
"statistical analysis only, no re-identification linkage" use restriction in
`SOURCE.json`); StatCan = StatCan Open Licence (already in your footer).

---

## 6. Madrid Central → roadside NO₂ (DiD or controlled ITS) — `madrid-lez-no2`

**Status: build first this round.** Second clean anchor on the LEZ→NO₂
mechanism, same data source and licence as the Berlin mark, and a published
DiD to validate against. Reaching two LEZ anchors is what unlocks a future
LEZ→NO₂ bridge mark.

### Design
- `identification`: `did` (the published design) — or `its-controlled` to match
  the Berlin mark's shape exactly. **Recommendation: mint it as `did` to add a
  second design family to the environmental domain**, treated station vs control
  stations, parallel trends. (If you'd rather keep the LEZ family ITS-consistent
  for bridging, mint as `its-controlled` instead — see note below.)
- Estimand: ATT (DiD) or population-over-window (ITS) of Madrid Central on
  in-zone roadside NO₂.

### Sharp date
- **30 November 2018** — Madrid Central comes into effect.
- Two known later events make excellent confounder/seam material and a possible
  *reversal* anchor: **July 2019** sanction moratorium (enforcement softened),
  and **July 2020** annulment by the Madrid High Court (TSJM). Keep the primary
  post-window short and pre-COVID (e.g. Dec 2018 → mid-2019) to sidestep both
  the moratorium drift and COVID — mirroring your Berlin 2010–2011 pre-COVID
  choice.

### Treated / control construction
- **Treated**: Plaza del Carmen (the in-zone station used in the literature).
- **Control**: other Madrid municipal-network stations sharing the
  pre-intervention trend — prefer urban-background and stations away from the
  zone boundary (the published work found positive spillover at *near* stations,
  so near-boundary stations are contaminated controls — exclude or model them).
- Madrid's municipal network has ~24 stations, ample for a pre-trend-matched
  control pool.

### Open-data inputs (CC BY 4.0)
- **EEA air quality download service** — hourly/daily NO₂ per Madrid station
  (same source family as Berlin). Madrid also publishes its own open municipal
  air-quality archive (record licence).
- **Open-Meteo (ERA5/Copernicus)** — meteorology covariates, as in Berlin.

### Confounders → joins
- **Meteorology** — wind speed/direction, temperature, boundary-layer height
  (dominant short-window NO₂ driver; same join as Berlin/ULEZ plans).
- **Spillover** — near-boundary stations show attenuated treatment; this is a
  control-contamination issue, not just a confounder. Document station-distance
  to the zone and exclude near-zone stations from the control pool.
- **Enforcement drift** — the July 2019 moratorium weakens treatment intensity
  over time; keep the post-window before it, or model intensity explicitly.
- **Secular Euro-standard fleet renewal** — the ~5%/yr background NO₂ decline
  from ~2010 (Euro V/VI turnover); the DiD/ITS isolates the discontinuity from
  this trend, but record it.

### Dossier validity checks
- Parallel pre-trends (DiD) / shared pre-trend (ITS), Plaza del Carmen vs control.
- Placebo date in the pre-period.
- Meteorology-adjusted vs unadjusted.
- Control-pool sensitivity (with vs without near-boundary stations).
- Seam check: confirm the effect is estimated on the pre-moratorium,
  pre-COVID window and is robust to trimming the window.

### Bridge potential
**High — this is the unlock.** Berlin (2010 LEZ stage) + Madrid (2018) gives ≥2
identified anchors on "LEZ stringency → NO₂", in two cities. A stochadex
dispersion-and-fleet-turnover simulator with a GP discrepancy pinned to both
can then bridge intermediate stringency/city points (`category: bridge`,
`truth_source: simulator-bridged`), with LOAO coverage reported. Keep
mechanism coherence: both are access-restriction LEZs (not charge schemes), so
they bracket the same mechanism.

> Note on design family: if you mint Madrid as `did` and Berlin is
> `its-controlled`, decision scores won't pool across them (by your rules) — but
> calibration scores still do, and the bridge operates on the *mechanism's
> effect curve*, not the design family. Minting Madrid as `its-controlled` keeps
> the LEZ family single-design and makes the bridge framing cleanest. Your call;
> I'd lean ITS for family coherence.

---

## 7. US state smoke-free indoor air laws → cardiovascular mortality (staggered DiD / controlled ITS) — `smokefree-cvd`

**Status: build second.** Fresh, strongly-identified mechanism (smoke-free
legislation is the textbook public-health natural experiment), fully open and
continuously published, with many states giving a rich staggered design.

### Design
- `identification`: `did` (staggered adoption, heterogeneity-robust estimator)
  — or `its-controlled` for a single well-chosen state vs not-yet-treated
  controls.
- Estimand: ATT of a comprehensive 100% smoke-free indoor air law (bars +
  restaurants + worksites) on age-standardised acute MI / ischaemic-heart-
  disease mortality rate.
- Unit: state × month (mortality), with event-time lags.

### Sharp dates
- Per-state **effective dates** of 100% smoke-free indoor air laws, from the
  CDC STATE System (which records venue-specific dates). Pick comprehensive-ban
  states with a clean single switch-on and a long pre-period.
- Good single-state ITS candidates from the literature: New York (2003),
  Arizona (2007) — both comprehensive, both with published AMI/stroke
  admission effects. For a DiD, use the staggered post-2003 adoption wave.

### Treated / control construction
- **Treated**: state at its comprehensive-ban effective date.
- **Control**: not-yet-treated and never-(yet)-treated states sharing the
  pre-trend. With staggered adoption, lean on a modern estimator
  (Callaway–Sant'Anna / Sun–Abraham) to avoid TWFE contamination.

### Open-data inputs (U.S. public domain)
- **Outcome**: CDC WONDER multiple-cause-of-death, monthly, 1999→ present.
  AMI = ICD-10 I21; IHD = I20–I25. Age-standardised rates; honour cell
  suppression (<10) and the no-linkage use restriction.
- **Policy dates**: CDC STATE System smoke-free indoor air law tables (venue-
  level effective dates).

### Confounders → joins
- **Cigarette taxes / tax changes** — the major co-policy on the same outcome;
  join state cigarette-excise-tax schedules and treat as a time-varying
  covariate (smoke-free-air effects are routinely estimated *net* of taxes).
- **Pre-existing smoking-prevalence trend** — join BRFSS state smoking
  prevalence.
- **Secular CVD-mortality decline** — AMI mortality fell ~4%/yr nationally for
  decades; the design isolates the break from this strong trend (record it; it
  is the reason a *controlled* design beats a bare before/after).
- **Local (sub-state) ordinances pre-empting the state law** — partial local
  bans before the statewide date attenuate the measured switch; join local-
  ordinance coverage where available, or restrict to states without major prior
  local coverage.
- **Population age-structure shifts** — use age-standardised rates.

### Dossier validity checks
- Event-study pre-trends (parallel-trends defence under staggering).
- Heterogeneity-robust estimator vs naïve TWFE (show Goodman-Bacon
  contamination if present).
- Tax-adjusted vs unadjusted (demonstrate the tax join is load-bearing).
- Placebo outcome (a cause of death not plausibly affected by secondhand
  smoke, e.g. an external-cause baseline).
- Single-state ITS robustness as a cross-check on the pooled DiD.

### Bridge potential
**Medium–high.** Many states × dates on one clean mechanism, plus international
anchors in the literature (Chile 2013 national ban, Italy 2005, Uruguay,
Scotland 2006) if you later want a cross-country smoke-free→CVD bridge. Strong
candidate for a second mechanism family with ≥2 anchors quickly.

---

## 8. Canadian provincial smoke-free / tobacco-tax change → smoking prevalence (DiD) — `ca-smokefree-smoking`

**Status: scope third.** Builds directly on the StatCan + CCHS pipeline you
already stood up for the menthol mark — lowest marginal ingest cost — and adds
a second Canadian anchor on the broader "tobacco-control intensity → smoking"
mechanism.

### Design
- `identification`: `did` (staggered across provinces).
- Estimand: ATT of a provincial smoke-free public-places law (or a sharp
  tobacco-tax increase) on adult current-smoking prevalence.
- Unit: CCHS respondent → province × year.

### Sharp dates
- Provincial comprehensive smoke-free public-place laws rolled out across the
  mid-2000s at differing dates (confirm exact per-province effective dates at
  build time). Alternatively, sharp provincial tobacco-tax step changes give a
  cleaner "intensity" treatment for an RKD/DiD.

### Treated / control construction
- **Treated**: each province at its law/tax date.
- **Control**: not-yet-treated provinces (staggered), pre-trend matched.
- Reuse the menthol mark's province×year CCHS panel and survey-weight handling.

### Open-data inputs (StatCan Open Licence)
- **Canadian Community Health Survey (CCHS)** — current-smoking status, demo-
  graphics, weights (same source as your menthol mark).
- **Policy dates** — provincial legislation; provincial tobacco-tax schedules.

### Confounders → joins
- **Federal co-timing** — federal tobacco measures overlapping provincial dates;
  disentangle.
- **Tax vs smoke-free-air confounding** — if studying smoke-free laws, join tax
  schedules (and vice versa); the two co-move.
- **Cross-province contraband flows** — affects tax-based designs especially.
- **Baseline prevalence heterogeneity** — province fixed effects.

### Dossier validity checks
- Parallel pre-trends by province (you already have the tooling from the
  menthol mark).
- Event-study leads/lags.
- Tax/smoke-free cross-adjustment.

### Bridge potential
**Medium.** Menthol ban + smoke-free law + tax step changes form a Canadian
"tobacco-control intensity → smoking" anchor family; combined with the US
smoke-free→prevalence evidence, a cross-country intensity bridge becomes
plausible. Keep the *outcome* consistent (smoking prevalence) so anchors bracket
one effect curve.

---

## Build order and rationale

1. **`madrid-lez-no2`** — second LEZ→NO₂ anchor, same data/licence as Berlin,
   published design, unlocks the first environmental bridge. Mint as
   `its-controlled` for family coherence (or `did` to diversify design families).
2. **`smokefree-cvd`** — fresh, textbook-clean mechanism, fully open and
   continuous via CDC WONDER + STATE System, rich staggered design, fast route
   to a second multi-anchor family.
3. **`ca-smokefree-smoking`** — lowest ingest cost (reuses the menthol CCHS
   pipeline), extends the Canadian tobacco-control family.

## Cross-cutting reproducibility flags

- **UK HES open series starts Apr 2008** — rules out reproducing the England
  2007 smoke-free MI design from open data. Note for anyone proposing UK pre-2008
  health-admission ITS marks.
- **CDC WONDER** — public domain but carries the no-linkage use restriction;
  record in `SOURCE.json` and dossier. Cell suppression (<10) must be handled in
  ingest.
- **Pre-COVID windows preferred** — both environmental marks you've landed used
  pre-2020 windows; keep Madrid's primary window pre-moratorium and pre-COVID,
  and prefer pre-2020 switch-ons for `smokefree-cvd` single-state ITS variants.

## Per-series setup checklist (as before)

- [ ] `data/raw/<id>/SOURCE.json` — URL + SHA-256 + licence + attribution per input.
- [ ] Confirm and pin exact intervention date(s); record source.
- [ ] Ingest loader (EEA station NO₂ + Open-Meteo; or WONDER monthly + STATE
      System dates; or CCHS province×year).
- [ ] Treated/control by pre-trend match; near-zone/contaminated controls
      excluded and documented.
- [ ] Confounder joins as covariate columns in `episodes` rows.
- [ ] Mark JSON: `identification`, `design` block, effect distribution,
      uncertainty budget, provenance.
- [ ] Dossier: pre-trend/parallel-trend check, placebo, confounder-adjusted vs
      unadjusted, control-set sensitivity, seam-specific check.
- [ ] `make validate` passes.
- [ ] Note bracketing anchors for the relevant bridge (LEZ→NO₂;
      tobacco-control→smoking; smoke-free→CVD).