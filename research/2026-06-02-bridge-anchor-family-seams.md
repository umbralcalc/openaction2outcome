# Bridge anchor-family seams: dose-response & staggered-rollout candidates

**Date:** 2026-06-02
**Question:** What open data could we focus the SCM/bridge approach on first? Find and rank
candidate "seams" — anchor families a deterministic structural causal model + GP-discrepancy
bridge could interpolate across.
**Method:** Deep-research method (decompose → search → verify → synthesize), run in the main
loop (subagents have no network in this environment), via WebSearch/WebFetch. Sources cited
inline.

---

## 1. The problem this research had to beat

Prior scouting (business rates, SHMI, bathing water, area-funding/IMD) kept failing the same
way: we hunted **multiple sharp design-based cutoffs on one axis (RDD/RKD)**, and those
families barely exist in open data — threshold/banding systems almost always have **one**
actionable cutoff (the "bad" side triggers a treatment; the other boundaries are
non-actionable labels), and the multi-cutoff structures that do exist are contaminated
(discretionary allocation), closed-access (individual microdata), or manipulable
(self-reported tax bases).

## 2. The reframe that worked

Stop hunting multi-cutoff RDD. Hunt **dose-response / staggered-rollout families identified by
difference-in-differences (DiD)** on **price-instrument policies**. Key coherence-solver:

> **A single jurisdiction that ratchets a policy intensity up in steps over time** is a
> coherent dose family — *same population, same regime, multiple intensities*, each
> DiD-identifiable, with a *well-motivated* bridge: interpolate the effect at an intensity
> that has not been tried and cannot be experimentally set.

Every strong candidate below is **SCM-tractable**: the mechanism is price→response or
emissions→concentration, with linear-ish elasticities and additive/structured (e.g.
meteorological) noise the discrepancy term absorbs. **None is bifurcating** (no epidemic-takeoff
/ tipping-point dynamics), so **none needs the deferred Monte-Carlo sampling rung.** This is
exactly the deterministic-SCM-with-enriched-noise regime.

## 3. Candidate landscape (ranked)

| # | Seam | Dose / anchor axis | Outcome openness | SCM fit | Verdict |
|---|------|--------------------|------------------|---------|---------|
| 1 | **London emission zones → NO₂** | LEZ'08 → T-charge'17 → ULEZ central'19 → inner'21 → M25'23 | **Open** (LAQN, Breathe London OGL, London Datastore, Defra AURN) | emissions→dispersion, deterministic | **Top** |
| 2 | **Alcohol minimum unit pricing → harm** | Scotland 50p'18 → 65p'24; Wales 50p; England control | **Open** (NRS/ONS deaths; MESAS sales) | price→consumption, linear | **Top** |
| 3 | **Minimum wage → employment** | many US states/cities × levels × time | **Open, CC0** (QCEW county, 1990+) | labour demand, ~linear | **Top (international fallback)** |
| 4 | Carbon pricing → emissions | BC, EU ETS, national taxes at €/tCO₂ | Open but coarse/aggregate | price→abatement | Viable, blunt |
| 5 | SSB / soda taxes → purchases | Berkeley 1¢, Philadelphia 1.5¢, Mexico, UK SDIL tiers | **Often Nielsen scanner = proprietary** | price→demand | Weak (data) |
| 6 | Congestion charging → traffic / NO₂ | London / Stockholm / Milan price levels | Open-ish (traffic, AQ) | price→demand | Viable |
| 7 | Tobacco taxes → prevalence | many jurisdictions × rates | Survey-based (open, noisy, lagged) | price→demand | Viable |
| 8 | Multi-cutoff RDD (business rates, etc.) | rateable-value kinks | Open (OGL) but underpowered | deterministic | Known weak |

**The SSB result is the cautionary one:** dose-response is *explicitly documented* ("a
dose–response relationship with the size of the SSB tax",
[meta-analysis](https://pmc.ncbi.nlm.nih.gov/articles/PMC9285619/)), and rates span Berkeley
(~21.9% AVE), Philadelphia (~33.3%), Mexico, the UK SDIL's two-tier structure
([Obesity Evidence Hub](https://www.obesityevidencehub.org.au/collections/prevention/countries-that-have-implemented-taxes-on-sugar-sweetened-beverages-ssbs)) —
but the purchase outcome usually comes from
[proprietary Nielsen scanner data](https://pmc.ncbi.nlm.nih.gov/articles/PMC8912695/), which
fails the open-data requirement. This confirms **outcome-openness is the binding filter**, not
whether a dose axis exists.

## 4. Top 3 — concrete go-paths

For each: (a) mechanism & SCM-tractability, (b) the axis, (c) the anchors, (d) the bracketed
bridge query, (e) data/licences/joins, (f) risks.

### 4.1 London emission zones → air quality *(strongest: open end-to-end + environmental mechanism)*

- **(a)** Vehicle-emissions restriction → NO₂ concentration via deterministic atmospheric
  dispersion + emissions accounting. Meteorological variation (wind, temperature) is
  additive/structured noise the discrepancy GP/SCM absorbs — the enriched-deterministic regime,
  non-bifurcating.
- **(b)** Scheme stringency/coverage ratcheted in discrete steps:
  [LEZ 2008 → T-charge 2017 → ULEZ central 2019 → inner (N/S Circular) Oct 2021 → M25 Aug 2023](https://en.wikipedia.org/wiki/Ultra_Low_Emission_Zone).
  Also a **staggered area rollout** (each ring treated at a different date).
- **(c)** A DiD effect per wave (treated zone/ring vs outside, pre/post). The
  [congestion-charge / ULEZ pollution-DiD literature already exists](https://www.sciencedirect.com/science/article/abs/pii/S0166046220302581);
  ULEZ is reported to have cut NO₂ ~27% London-wide.
- **(d)** Effect of an intermediate stringency, or in an area/period between treated waves —
  not directly estimable (you cannot run the un-tried scheme).
- **(e)** Outcome NO₂: [LAQN / Imperial College](https://www.londonair.org.uk/) (free public
  access), [Breathe London — OGL](https://breathelondon.edf.org/methodology.html), ~1,900
  borough NO₂ diffusion tubes on the [London Datastore](https://data.london.gov.uk/air-quality/),
  [Defra AURN](https://uk-air.defra.gov.uk/networks/network-info?view=aln). Treatment
  (scheme dates/boundaries) is public TfL/GLA. Join on monitoring-site location × date.
- **(f)** Risks: the steps **conflate area + stringency** (not a clean 1-D dose); **COVID 2020–21
  traffic collapse overlaps the 2021 expansion** (a real confound — but meteorological/mobility
  normalisation is exactly what the SCM is for); spatial spillover; fleet-turnover trend.

### 4.2 Alcohol minimum unit pricing → alcohol harm *(cleanest coherent dose axis)*

- **(a/b)** Price → consumption → harm (linear-ish price elasticity, SCM-tractable).
  Within-Scotland dose axis:
  [**50p (May 2018) → 65p (Sep 2024)**](https://www.gov.scot/policies/alcohol-and-drugs/minimum-unit-pricing/)
  — same population, two intensities = coherent. Plus Wales 50p (2020) and England (control)
  for cross-jurisdiction.
- **(c)** Controlled interrupted time series / DiD (Scotland vs England) — peer-reviewed,
  [~268 fewer deaths/yr](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC10154457/).
- **(d)** Effect at an un-implemented price (e.g. **60p**), bracketed by the 50p and 65p anchors.
- **(e)** Outcome **open**: alcohol-specific deaths/hospitalisations from
  [NRS Scotland](https://www.gov.scot/news/decrease-in-alcohol-specific-deaths/) + ONS. **Use
  MESAS/PHS alcohol *sales* as the proximal outcome** — both 50p and 65p effects are observable
  *now*, whereas deaths lag years.
- **(f)** Risks: deaths are chronic-lagged (→ use sales); COVID overlap; the 65p is recent, so
  its *deaths* anchor is still maturing (sales sidesteps this).

### 4.3 Minimum wage → employment *(international fallback; maximally open outcome)*

- **(a/b)** Labour-demand response, elasticity
  [~−0.1 to −0.2](https://www.nber.org/system/files/working_papers/w32878/w32878.pdf) (linear,
  SCM-tractable). Dose axis = the minimum-wage level, with large cross-state/time variation;
  within-state stepped schedules (e.g. California $10→$15) are coherent.
- **(c)** Staggered-DiD across states/borders — the canonical
  [minimum-wage literature](https://irle.berkeley.edu/wp-content/uploads/2010/11/Minimum-Wage-Effects-Across-State-Borders.pdf).
- **(d)** The employment effect at an **un-tried wage ($20)** — a live policy debate, not
  directly estimable.
- **(e)** Outcome **CC0 public domain**:
  [QCEW](https://www.bls.gov/cew/additional-resources/open-data/home.htm), county-level, 1990+.
  Minimum-wage rates are public policy facts (assembled separately).
- **(f)** Risks: US not UK; most-studied (some saturation — but the *calibrated-bridge* framing
  is novel); elasticity heterogeneity by labour-market concentration (mild nonlinearity →
  moment propagation handles it).

## 5. Conclusion

**The bridge is not blocked on data.** Genuine, open, SCM-tractable dose/rollout anchor families
exist; the unlock was abandoning the multi-cutoff-RDD hunt for **price-instrument policies
ratcheted in steps over time**, identified by DiD, with a within-jurisdiction stepping that
satisfies coherence. **None of the top candidates needs the sampling rung.**

**Recommended first real bridge: London emission zones → air quality.** It is UK/OGL-open
end-to-end, the mechanism is the deterministic-environmental wheelhouse, and it provides both a
staggered-rollout *and* a stringency anchor family. **Alcohol MUP** is the cleanest coherent
dose axis and a strong second. **Minimum wage** is the maximally-open international fallback.

**Next step:** take the London air-quality seam into a concrete harvest + mint scope (data pull,
per-wave DiD anchors, the SCM mechanism, the bracketed query, validity/coherence/leakage risks).

## 6. Sources

- ULEZ / London emission-zone timeline — [Wikipedia: Ultra Low Emission Zone](https://en.wikipedia.org/wiki/Ultra_Low_Emission_Zone)
- London congestion charge & pollution DiD — [ScienceDirect](https://www.sciencedirect.com/science/article/abs/pii/S0166046220302581)
- LAQN — [londonair.org.uk](https://www.londonair.org.uk/); Breathe London (OGL) — [methodology](https://breathelondon.edf.org/methodology.html); [London Datastore air quality](https://data.london.gov.uk/air-quality/); [Defra AURN](https://uk-air.defra.gov.uk/networks/network-info?view=aln)
- Alcohol MUP policy & 50p→65p — [gov.scot MUP](https://www.gov.scot/policies/alcohol-and-drugs/minimum-unit-pricing/); MUP CITS evaluation — [PMC10154457](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC10154457/); NRS alcohol deaths — [gov.scot news](https://www.gov.scot/news/decrease-in-alcohol-specific-deaths/)
- Minimum wage — [Dube, NBER w32878](https://www.nber.org/system/files/working_papers/w32878/w32878.pdf); [Dube/Lester/Reich border design](https://irle.berkeley.edu/wp-content/uploads/2010/11/Minimum-Wage-Effects-Across-State-Borders.pdf); [QCEW open data](https://www.bls.gov/cew/additional-resources/open-data/home.htm)
- SSB taxes — [meta-analysis (dose-response), PMC9285619](https://pmc.ncbi.nlm.nih.gov/articles/PMC9285619/); [multi-city Nielsen comparison, PMC8912695](https://pmc.ncbi.nlm.nih.gov/articles/PMC8912695/); [Obesity Evidence Hub: SSB taxes by country](https://www.obesityevidencehub.org.au/collections/prevention/countries-that-have-implemented-taxes-on-sugar-sweetened-beverages-ssbs)
- Carbon pricing — [BC carbon tax, Springer](https://link.springer.com/article/10.1007/s10640-022-00679-w); [ex-post review, IOPscience](https://iopscience.iop.org/article/10.1088/1748-9326/abdae9)
- Low emission zones across Europe (review) — [SpringerOpen](https://link.springer.com/content/pdf/10.1186/s12544-025-00749-2.pdf)
