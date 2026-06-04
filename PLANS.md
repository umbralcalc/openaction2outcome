# openaction2outcome — build plans for five new mark series

Five candidate series, each written to drop into the existing
`make fetch` → `make build` → `make validate` flow. Ordered by how cleanly
they fit the current schema. Every plan specifies: the design family and
estimand, the sharp date(s), the treated/control construction, the open-data
inputs with licence, the confounder set to mitigate via a causal data model
or extra joins, the validity checks the dossier must carry, and the bridge
potential (whether the series yields ≥2 anchors on one mechanism).

Licence posture, up front:

- **TfL / London Datastore / LAQN / Breathe London** — Open Government Licence
  v3.0 (© Crown copyright). Same posture as your existing inputs; record per
  input in `data/raw/<id>/SOURCE.json`.
- **CDC BRFSS / WONDER / data.cdc.gov** — U.S. Government works, public domain
  (17 U.S.C. § 105). One caveat to record: NCHS micro-data and WONDER carry a
  "statistical reporting and analysis only; no linkage to re-identify
  individuals" use restriction. This does not impede aggregate ITS/DiD use but
  belongs in `SOURCE.json` and the dossier.
- **PDAPS (Prescription Drug Abuse Policy System)** — policy-date tables;
  check and record the specific licence (typically open/CC-style) per pull.
- **Health Canada / provincial sources & European city LEZ data** — licence is
  per-source; record each in `SOURCE.json`. Do not assume OGL/PD outside UK/US.

A recurring structural note: a controlled ITS needs a control series that
*shares the pre-intervention trend*; a DiD needs a treated vs. control split
under *parallel pre-trends*. For every plan the parallel-trends / shared-trend
check is the load-bearing validity artefact, and the confounder joins exist
specifically to defend it.

---

## 1. ULEZ → roadside NO₂ (controlled ITS) — `ulez-no2`

**Status: BUILT, then SHELVED (2026-06-04) — declared seam, not identified.**
The full controlled-ITS machinery was implemented (`internal/its`,
`internal/series/ulezno2.go`, `scripts/ulez_harvest.py`) and run on real LAQN data
for both the 2023 and 2019 events. Neither yields an admitted mark: the
treated−control NO₂ difference carries non-linear secular dynamics (2023: the
pre-window IS the post-COVID rebound; 2019: a pre-existing curved fleet-renewal
decline from the 2017 T-Charge). The fix — joining a monthly fleet-compliance curve
— is methodologically sound but blocked by data: that series exists only in
scanned-image GLA PDFs and is not under a clean OGL grant, so it fails this corpus's
reproducible-open-data bar. Infrastructure retained for a future pre-COVID European
LEZ anchor (series 5), which reuses the same ITS estimator. See
`research/2026-06-04-ulez-no2-its-covid-confound.md`. Original plan below.

**Original status: build first.** Cleanest sharp instant, open data on your existing
licence, published designs to validate against, and a multi-anchor mechanism.

### Design
- `identification`: `its-controlled`
- Estimand: **population effect over the post-intervention window** — the
  change in roadside NO₂ in the newly-covered zone relative to a control
  series sharing the pre-intervention trend.
- Unit of analysis: monitoring station × time (hourly or daily-aggregated to
  weekly/monthly for the ITS; keep hourly in `episodes` rows so the seam is
  inspectable).

### Sharp dates (three anchors on one mechanism)
- **8 April 2019** — central London ULEZ switches on (Congestion Charge Zone).
- **25 October 2021** — inner London expansion, to (not incl.) the North/South
  Circular.
- **29 August 2023** — London-wide expansion to the GLA boundary (~M25).

Build the **2023 outer-London expansion** as the first mark: it has the
largest pool of credible never-treated-until-then control stations (outer
boroughs that only entered the zone in Aug 2023, versus stations well outside,
plus urban-background stations). The 2021 and 2019 events give you the second
and third anchors on the *same mechanism* — this is the series most likely to
support a future **bridge mark** (≥2 bracketing anchors on "LEZ stringency →
NO₂").

### Treated / control construction
- **Treated series**: roadside (kerbside) stations inside the boroughs newly
  covered by the relevant expansion.
- **Control series**: (a) urban-background stations (same airshed, far weaker
  direct treatment), and (b) roadside stations outside the newly-covered area
  for that expansion. Pick controls by *pre-period trend match*, not by
  geography alone.
- Population-over-window estimand → aggregate to the zone level after
  station-level modelling.

### Open-data inputs (all OGL v3.0)
- **LAQN API** (`londonair.org.uk/Londonair/API/`) — hourly NO₂/PM per station,
  station metadata (roadside vs background, lat/long).
- **Breathe London** (`breathelondon.edf.org`) — additional sensor network,
  OGL; missing-data sentinel is `-999` (handle in ingest).
- **London Datastore LAEI** (`data.london.gov.uk`) — emissions inventory for
  baseline context.
- **TfL** — ULEZ compliance %, traffic-flow counts (for the compliance/traffic
  correlate joins).

### Confounders → causal data model / joins
The seam is that there is no perfect concurrent control (a London-wide policy
contaminates all London stations). Mitigate by joining time-varying correlates
and modelling them explicitly:
- **Meteorology** (dominant short-window confounder): wind speed/direction,
  temperature, boundary-layer height, precipitation. Source: Met Office /
  Open-Meteo (record licence). NO₂ at a roadside site is hugely sensitive to
  dispersion conditions — this join is non-negotiable.
- **Fleet-compliance trend**: TfL publishes compliance % rising over the
  window; the policy effect should be attributed to the *discontinuity*, not
  the smooth secular compliance rise.
- **COVID traffic baseline**: 2020–2021 traffic collapse and recovery overlaps
  the 2021 expansion. Join mobility/traffic-flow series; consider excluding or
  flagging lockdown windows.
- **Scrappage scheme timing** (£61m 2021, doubled for 2023): a co-intervention
  on the same mechanism — document, don't double-count.
- **LEZ tightening (1 March 2021)**: heavy-vehicle standards tightened — a
  *separate* co-intervention near the 2021 ULEZ event. Critical to disentangle
  for the 2021 anchor; cleaner for the 2023 anchor.

### Dossier validity checks
- Pre-trend overlap between treated and control series (the shared-trend
  assumption).
- Placebo: a fake intervention date in the pre-period should show no break.
- Meteorology-adjusted vs unadjusted estimate (show the meteorology join is
  load-bearing).
- Sensitivity to control-set choice (background-only vs outside-zone-roadside
  vs both).
- Seam-specific check (in the spirit of the bathing-water dossier):
  re-estimate excluding COVID-affected months; confirm the break survives.

### Bridge potential
**High.** Three anchors (2019/2021/2023) on "zone stringency → NO₂", plus
European LEZ anchors (series 5) on the same mechanism. Once ≥2 are minted, a
stochadex dispersion-and-fleet-turnover simulator can bridge intermediate
stringency points, GP-discrepancy pinned to the anchors.

---

## 2. Flavoured-tobacco bans → adult smoking prevalence (DiD) — `flavour-ban-smoking`

**Status: build second.** Already published as clean BRFSS DiD studies, with
the confounder structure explicitly characterised — which directly tells you
which joins your causal data model needs.

### Design
- `identification`: `did`
- Estimand: **ATT** — average treatment effect on the treated (the banning
  state) on adult current-smoking prevalence.
- Unit: BRFSS respondent (individual-level), aggregated to state × year for
  the panel; keep individual rows in `episodes` with survey weights.

### Sharp dates (two anchors on one mechanism)
- **1 June 2020** — Massachusetts comprehensive flavoured-tobacco ban
  (incl. menthol) takes effect.
- **21 December 2022** — California comprehensive flavoured-tobacco ban
  takes effect.

Two states → two anchors on the "comprehensive flavour ban → smoking
prevalence" mechanism.

### Treated / control construction
- **Treated**: MA (mark A), CA (mark B).
- **Control**: states with **no** flavour ban *and* no significant local
  (municipal/county) menthol pre-emption. This control-purity point is the
  single biggest driver of estimate validity (see confounders).
- Pre-period from 2017; post-period through latest BRFSS wave.

### Open-data inputs (public domain)
- **BRFSS annual files** (`cdc.gov/brfss/annual_data/annual_data.htm`,
  mirrored on data.cdc.gov) — current-smoking status, age, sex, race/ethnicity,
  income, education, plus survey weights (`_LLCPWT`) and strata.
- **Policy dates / local pre-emption matrix**: PDAPS + the NBER replication set
  (e.g. w32535) for state and large-jurisdiction flavour-restriction dates.

### Confounders → causal data model / joins
The published studies hand you the confounder list directly:
- **Local-level menthol bans in control states**: including these biases the
  estimate. Join a municipal/county pre-emption matrix and *exclude or
  down-weight* contaminated control states. (Published MA estimate was biased
  when local-ban states were left in the control pool.)
- **COVID-19 wave**: the MA ban (Jun 2020) lands in the pandemic. Estimate is
  sensitive to dropping the first 6 months of 2020 BRFSS — so model a COVID
  indicator and run the leave-out as a robustness check.
- **Tobacco-21 laws**: age-of-sale changes — restrict to 21+ or join the
  Tobacco-21 adoption dates.
- **Baseline smoking-rate heterogeneity**: control for state fixed effects and
  consider dropping the highest-baseline states as a robustness check.
- **Substitution**: bans shift menthol → other flavours / cigarettes → vaping.
  If you want the *net* smoking effect this is fine; if you want a mechanism
  decomposition, join e-cig/vaping outcome variables (note BRFSS vaping
  coverage gaps by state-wave).

### Dossier validity checks
- Parallel pre-trends, treated vs control, 2017→ban.
- Event-study / leads-and-lags plot (no pre-trend, effect after switch-on).
- Control-purity sensitivity (with vs without locally-pre-empted states).
- COVID leave-out (drop Mar–Aug 2020).
- Goodman-Bacon / modern DiD diagnostic if you ever pool toward a staggered
  design (see series 3).

### Bridge potential
**Medium.** MA + CA + Canadian provincial menthol bans (series 4) form an
anchor family on "menthol/flavour restriction → smoking", across two
countries and partial-vs-comprehensive intensity — a credible bridge target.

---

## 3. State naloxone access laws → opioid overdose mortality (staggered DiD) — `naloxone-overdose`

**Status: build third — higher methodological care.** The "instant" is fuzzier
than ULEZ and the dominant confounder (fentanyl supply) is severe. Best treated
as a staggered-adoption DiD with an event-study lag structure, not a sharp ITS.

### Design
- `identification`: `did` (staggered adoption; use a heterogeneity-robust
  estimator, not naïve two-way fixed effects).
- Estimand: **ATT** of naloxone access law on opioid-overdose mortality rate,
  with explicit event-time lags.
- Unit: state × quarter (or county × quarter where mortality cell sizes allow;
  WONDER suppresses cells < 10).

### Sharp date(s)
- Per-state **effective dates** of pharmacy-access / standing-order naloxone
  laws. Caveat for the dossier: there are documented lags between a law's
  effective date and the implementing directive, so the treatment "instant" is
  itself uncertain — model an implementation lag window.

### Treated / control construction
- **Treated**: states at their law effective date.
- **Control**: not-yet-treated and never-treated states (staggered design).
- Because adoption is near-universal eventually, lean on *timing* variation;
  never-treated controls are scarce, so the not-yet-treated comparison and a
  modern estimator (Callaway–Sant'Anna / Sun–Abraham style) matter.

### Open-data inputs
- **Outcome**: opioid-overdose mortality via **CDC WONDER multiple-cause-of-
  death** / data.cdc.gov (public domain; honour the no-linkage use
  restriction). ICD-10 drug-poisoning codes (X40–X44, X60–X64, X85, Y10–Y14),
  opioid-specific contributory codes.
- **Policy dates**: PDAPS naloxone-access-law tables.
- **Co-policy matrix**: PDAPS for PDMP, Good Samaritan, Medicaid expansion,
  MOUD/buprenorphine access.

### Confounders → causal data model / joins
This is the confounder-heaviest series; the joins *are* the project:
- **Fentanyl supply wave** (dominant time-varying confounder, post-2015):
  the illicit-supply shift swamps policy effects. Join a fentanyl-penetration
  proxy (e.g. share of deaths involving synthetic opioids) as a time-varying
  covariate — and be honest in the interval about residual confounding.
- **Policy co-adoption / clustering**: naloxone laws travel with PDMPs, Good
  Samaritan laws, Medicaid expansion. Effects are driven by *bundles*, not
  single laws — join the full co-policy matrix and consider a bundle treatment
  definition.
- **Implementation lag**: effective date ≠ on-the-ground access; model a lag
  window and test sensitivity.
- **Rural/urban & shortage-area composition**: dispensing access varies; join
  primary-care-shortage-area designation and rurality.
- **Reporting / coding changes** in cause-of-death over time.

### Dossier validity checks
- Event-study with pre-trends (the parallel-trends defence under staggering).
- Heterogeneity-robust estimator vs naïve TWFE (show the Goodman-Bacon
  contamination if present).
- Fentanyl-adjusted vs unadjusted (demonstrate the join is load-bearing and
  state residual confounding honestly).
- Implementation-lag sensitivity.
- Bundle vs single-law treatment definition.

### Bridge potential
**Low–medium.** Many anchors exist (many states, many dates) but they sit on a
confounded, bundled mechanism — bridging is harder to defend. Treat as
identified pins first; only attempt a bridge if a clean sub-family emerges.

### Honesty note
Given fentanyl confounding, this series is where the project's central finding
— that honest intervals must price in modelling-choice uncertainty, not just
sampling error — will be most visible. The interval should be conspicuously
wider than a plug-in method would suggest.

---

## 4. Canadian provincial menthol bans → smoking (DiD, non-US anchor) — `ca-menthol-smoking`

**Status: scope after #2.** A clean non-US anchor on the *same menthol
mechanism* as series 2 — its main value is feeding a cross-country bridge.

### Design
- `identification`: `did` (staggered across provinces).
- Estimand: **ATT** of provincial menthol ban on menthol/total smoking
  prevalence.
- Unit: survey respondent → province × year.

### Sharp dates
- Provincial menthol cigarette bans rolled out **2015–2017** (incl. the more
  populous Alberta, Ontario, Quebec). Confirm exact per-province effective
  dates at build time and record each in `SOURCE.json`.

### Treated / control construction
- **Treated**: each province at its ban date.
- **Control**: not-yet-banning provinces (staggered).
- Published DiD found ~2.4 pp reduction in youth menthol smoking and ~3.1 pp in
  adults — a sanity target for your central estimate.

### Open-data inputs — **verify licence per source (do not assume PD/OGL)**
- **Outcome**: Canadian Community Health Survey (CCHS) / Canadian Tobacco
  surveys via Statistics Canada. Record the StatCan Open Licence terms in
  `SOURCE.json`.
- **Policy dates**: Health Canada / provincial legislation records.

### Confounders → joins
- Federal vs provincial timing overlap (federal menthol restriction came
  later — disentangle from provincial bans).
- Cross-province contraband/substitution flows.
- Differing baseline prevalence and tax regimes (join provincial tobacco-tax
  schedules).

### Dossier validity checks
- Parallel pre-trends by province.
- Event-study leads/lags.
- Substitution check (menthol → non-menthol).

### Bridge potential
**This series exists for the bridge.** MA (comprehensive, 2020) + CA
(comprehensive, 2022) + Canadian provincial (menthol-only, 2015–17) span a
*restriction-intensity* axis across two countries — a defensible interpolation
target for "flavour-restriction intensity → smoking prevalence".

---

## 5. European city low-emission zones → NO₂ (controlled ITS, non-UK anchors) — `eu-lez-no2`

**Status: FIRST ANCHOR DELIVERED (2026-06-04) — `berlin-lez-no2-2010`, ADMITTED.**
The Berlin Umweltzone stage-2 (1 Jan 2010, green-sticker Euro-4-diesel BAN inside
the S-Bahn Ring) is minted and admitted as the first identified anchor on a new
`lez-ban-stringency-to-roadside-no2` mechanism (kept SEPARATE from the charge-type
ULEZ). Effect −1.2 µg/m³, honest 95% interval [−6.7, +4.8] — a near-null with a wide
interval, the honest reading that a standard-ban LEZ cut roadside NO₂ little (LEZs cut
particulates/soot far more than NOx). Crucially it ADMITS where London did not: the
Berlin in-zone-traffic-minus-background difference is flat (pre-trend slope ≈0), so the
placebo battery is clean — versus London's curved pre-trend. Data: EEA historical
(AirBase) archive, hourly NO₂ → monthly, CC BY 4.0; pre-COVID (2008-2011). LIMITATION:
only two in-zone traffic NO₂ stations have historical EEA coverage (thin treated side →
wide interval). Reuses the `internal/its` estimator and the event-parametrised
`ulezEvent` builder. Build: `make build SERIES=berlin-lez-no2`. Original plan below.

**Original status: scope after #1.** Same mechanism as ULEZ in other cities; main value
is broadening the LEZ→NO₂ anchor family for a bridge.

### Design
- `identification`: `its-controlled`
- Estimand: population NO₂ effect over post-intervention window.
- Unit: station × time per city.

### Candidate sharp dates (confirm and pin at build time)
- **Berlin Umweltzone** stage 2 (stricter standard) — staged introduction.
- **Milan Area C** congestion/pollution charge — sharp introduction date.
- Other staged European LEZ tightenings as anchors.

### Treated / control construction
- Same template as series 1: treated = in-zone roadside; control = background
  + out-of-zone roadside matched on pre-trend.

### Open-data inputs — **verify licence per source**
- **European Environment Agency (EEA)** air-quality download service / national
  networks (UBA for Germany, ARPA Lombardia for Milan). Record each licence in
  `SOURCE.json`; EEA data is generally reusable but confirm terms per dataset.
- City traffic/charge data per municipality.

### Confounders → joins
- Meteorology (as series 1) — wind/dispersion dominates.
- Concurrent fleet-renewal and national diesel policies.
- Charge-vs-ban mechanism differences (Milan Area C is a charge; Berlin is a
  standard ban) — keep mechanism-consistent when feeding a bridge.

### Dossier validity checks
- Shared pre-trend, treated vs control.
- Placebo date.
- Meteorology-adjusted vs unadjusted.

### Bridge potential
**High, combined with series 1.** ULEZ (2019/2021/2023) + EU city LEZs form a
multi-city, multi-stringency anchor family on "LEZ → NO₂". Keep
charge-mechanisms and ban-mechanisms separable so the bridge interpolates
within a coherent mechanism.

---

## Suggested build order and rationale

1. **`ulez-no2`** — cleanest sharp instant, existing licence posture, three
   same-mechanism anchors, highest bridge value. Build the 2023 expansion mark
   first.
2. **`flavour-ban-smoking`** — published DiD to validate against, confounders
   explicitly characterised, two anchors.
3. **`naloxone-overdose`** — high value but confounder-heavy; the showcase for
   honest-interval width. Use a heterogeneity-robust staggered estimator.
4. **`ca-menthol-smoking`** — non-US anchor; build to enable the flavour-ban
   bridge.
5. **`eu-lez-no2`** — non-UK anchors; build to enable the LEZ→NO₂ bridge.

Series 1 and 2 each become a `make build` target with a mark JSON + dossier;
series 4 and 5 are explicitly anchor-family members for future bridge marks
(`category: bridge`, `truth_source: simulator-bridged`) once each mechanism has
≥2 bracketing identified anchors.

## Per-series setup checklist (applies to all)

- [ ] `data/raw/<id>/SOURCE.json` — URL + SHA-256 + licence per input.
- [ ] Confirm and pin exact intervention date(s); record source.
- [ ] Wire `internal/ingest` loader (handle LAQN `-999`, BRFSS weights/strata,
      WONDER cell suppression as relevant).
- [ ] Treated/control construction by **pre-trend match**, recorded.
- [ ] Confounder joins implemented as covariate columns in `episodes` rows.
- [ ] Mark JSON: `identification`, `design` block, effect distribution,
      uncertainty budget, provenance.
- [ ] Dossier: shared-trend/parallel-trend check, placebo, confounder-adjusted
      vs unadjusted, control-set sensitivity, seam-specific check.
- [ ] `make validate` passes against the schema.
- [ ] For anchor-family members: note bracketing anchors for future bridge.