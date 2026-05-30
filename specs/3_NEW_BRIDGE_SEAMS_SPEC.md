# openaction2outcome — Spec 2
## Expanded landscape: datasets, joins and domains unlocked by bridge marks

Assumes Spec 1 (bridge marks) is in place. The point of bridge marks is not "more marks" — it is that the **harvesting target changes**. We no longer need an isolated clean natural experiment per policy. We need **families of identified anchors on one mechanism** that a stochadex simulator can bridge across. That single shift reopens the confounder-heavy domains (clinical, epidemiology, economic policy) that single-RDD scope excluded — because the simulator carries the mechanism's complexity while the anchors keep it honest.

This spec lays out (a) the new harvesting principle, (b) a clean data model that holds identified marks, anchor families, simulators and bridge marks coherently, and (c) the domains and cross-dataset joins now in scope, ranked by viability.

---

## 1. The new harvesting principle

**Old target:** one policy → one sharp threshold → one same-source same-unit outcome → one identified mark.

**New target:** one *mechanism* → a *family* of identified anchors at different points on its effect-curve (different cutoff levels, intensities, doses, time periods, jurisdictions) → a simulator pinned across them → bridge marks at the policy-relevant points in between.

Consequences for what we go looking for:
- **Repeat structure is now an asset.** A policy applied at *many* thresholds, or re-run across *many* areas/years with varying intensity, is no longer "several separate marks" — it is one *anchor family* feeding one bridge. Domains with naturally repeated threshold-policies leap up the priority list.
- **The identification strategy and the outcome can live in different datasets.** The anchors are still design-based (RDD/DiD/natural experiment), but the simulator's calibration data and the outcome series can be separately published and joined on entity/area/time. Joins become first-class, not a risk to avoid — because a join error now corrupts a *clearly-labelled bridge mark*, not the identified ground truth.
- **We still refuse pure selection-on-observables.** Every anchor is design-based. The simulator never manufactures an anchor; it only spans between them. The line that keeps the collection trustworthy is unchanged.

---

## 2. Clean data model

Five entities. Everything in the collection is one of these or a link between them. This model holds both the existing identified marks and the new bridge marks without special-casing.

```
Mechanism
  id, name, domain, description
  policy_variable        # what x is: cutoff level | intensity | dose | time | rank
  outcome_construct      # the single outcome these anchors all measure
  population_definition  # who/what the units are
  # coherence is defined HERE: two anchors share a mechanism iff they share
  # policy_variable + outcome_construct + population_definition + regime.

Source
  id, publisher, url, sha256, licence, retrieval_date, vintage
  # a frozen open-data input. unchanged from today. licences vary per source now.

AnchorMark   (= today's identified mark; category=identified)
  id, mechanism_id
  design            # sharp_rdd | fuzzy_rdd | did | iv | natural_experiment
  policy_point      # the x this anchor sits at on the mechanism's curve
  unit, period
  effect_posterior  # the honest interval (real, design-based truth)
  validity_dossier  # density/continuity/placebo/... + manipulation_flag
  sources[]         # join provenance: which Source(s), joined on which key
  truth_source = identified

Simulator
  id, mechanism_id
  stochadex_model_id, version, seed, input_hashes
  calibration_data_sources[]   # may differ from the anchors' outcome sources
  discrepancy_kernel           # GP covariance family + hyperparameters

BridgeMark   (category=bridge)
  id, mechanism_id, simulator_id
  anchors[]         # >=2 AnchorMark ids that BRACKET query_point (one each side)
  query_point       # the x this bridge estimates; always inside the anchor hull
  effect_posterior  # honest interval from the calibrated discrepancy posterior
                    # (bounded and pinned at both ends — interpolation only)
  loao_coverage     # leave-one-anchor-out validation result
  kernel_sensitivity
  truth_source = simulator-bridged
```

Invariants the model enforces (and the scorer/validator check):
- A `BridgeMark` references ≥2 `AnchorMark`s **of the same `mechanism_id`** that **bracket** its query point on the policy variable (anchor coherence + interpolation-only by construction). A non-bracketed query point is rejected at mint time.
- `truth_source` is immutable per category and never aggregated across categories.
- Every `AnchorMark` and `BridgeMark` is re-mintable byte-for-byte from its `Source`s + seeds + hashes (existing determinism rule, now spanning joins).
- A join is recorded as: which `Source`s, which join key, which vintages, with a point-in-time assertion (`inputs_asof ≤ decision_point < outcome_point`) — leakage discipline extended to multi-source joins.

This model is a clean superset of what the repo ships today: existing marks are `AnchorMark`s with a `Mechanism` of one anchor; nothing already published has to change except gaining a `mechanism_id` and `category=identified`.

---

## 3. Domains and joins now in scope (ranked by viability)

Ranked by: does it have anchor families (repeat threshold structure) + design-based anchors + open outcomes + a mechanism a stochadex simulator can credibly carry + low prior decision-science saturation.

### Tier 1 — strongest fit (pursue first)

**A. Epidemiology / public-health intervention thresholds (vaccination, screening, stewardship).**
- *Anchor family:* the same intervention triggered at many thresholds / rolled out across many areas and times (age-eligibility cutoffs for screening or vaccination are classic sharp RDDs; staggered area rollouts are DiD anchors). Naturally repeated → rich anchor families.
- *Mechanism for the simulator:* disease/transmission or screening-detection dynamics — **exactly** the stochadex epidemiology wheelhouse (your AMR/epi background). The simulator carries herd-effects, spillovers, and dynamics no single RDD captures; anchors keep it honest.
- *Joins:* eligibility/rollout policy (one source) × outcome surveillance (another source) × population denominators (a third), joined on area/age-band/time.
- *Why under-modelled in this frame:* heavily modelled for *prediction*, almost never as a calibrated bridge between design-based anchors with honest intervals.

**B. Environmental regulation intensity (emissions limits, water-quality bands, abstraction limits).**
- *Anchor family:* the same regulatory threshold at different stringency levels / regions / revisions; bathing-water (already a seam) generalises — many sites, repeated annually, is an anchor family on one mechanism.
- *Mechanism:* catchment / pollutant-transport / ecological-response dynamics — stochadex environmental modelling (your flood/freshwater background).
- *Joins:* regulatory threshold + enforcement action (regulator register) × environmental outcome (monitoring API) × emissions/discharge (operator returns), on facility/site/catchment.
- *Why under-modelled:* existing work is compliance-forecasting; the calibrated-effect-of-the-regulation question is open.

**C. Economic policy with eligibility thresholds applied at many levels (relief bands, support tiers, tax kinks).**
- *Anchor family:* a policy with multiple band boundaries, or repeated across budgets/years, gives anchors along the intensity axis.
- *Mechanism:* a behavioural/firm-dynamics or household-response simulator bridging between the band-boundary RDD anchors to estimate effects mid-band where no cutoff exists.
- *Joins:* eligibility rule (policy doc) × entity outcome (open registry) on entity id + period. Caveat: many running variables here are self-reported (manipulation risk at the anchor) — anchors must pass the existing manipulation check or ship flagged.

### Tier 2 — viable but harder

**D. Clinical dose/eligibility thresholds (guideline cutoffs that change recommended treatment).**
- *Anchor family:* guideline thresholds (e.g. a biomarker cutoff that flips recommended action) act as RDDs; revised guidelines over time give anchors at different cutoff values.
- *Mechanism:* pharmacological/clinical-progression simulator bridging between guideline-cutoff anchors.
- *Hard part:* individual-level outcomes are often behind controlled access (the SRS-equivalent wall) — restrict to settings with open aggregate outcomes, or the seam fails the open requirement. Anchor families exist but open outcomes are the binding constraint, exactly as in the gov scout.

**E. Education intensity beyond the single floor (multiple accountability thresholds, funding tiers).**
- *Anchor family:* floor-standards (already a seam) plus other accountability/funding cutoffs on the same school-performance mechanism → an anchor family bridging the performance axis.
- *Mechanism:* a school-improvement-response simulator.
- *Why Tier 2:* the mechanism simulator is softer (school behavioural response is less mechanistically constrained than disease or pollutant transport), so the bridge leans harder on the discrepancy GP than on simulator structure — wider, less defensible intervals.

### Tier 3 — defer (named so we don't re-litigate)
- Finance / index reconstitution / credit / sports — saturated, as established; bridge marks don't help because the gap was never identification, it was novelty.
- Conservation/IUCN — deferred (circular outcome, discretionary designation, active SoO literature).
- Pure observational domains with no design-based anchor — out by principle; the simulator cannot manufacture an anchor.

---

## 4. What to build, in order

1. **Backfill `mechanism_id` + `category` onto the existing four seams** (floor-standards, SHMI, bathing-water, IPC). Cheap, and it makes the current collection a clean instance of the new data model. Bathing-water immediately reveals an anchor family (many sites/years on one mechanism) — the natural first bridge.
2. **First bridge mark on bathing-water** — it already has the most repeat structure and an environmental mechanism stochadex can carry; lowest-risk place to validate `internal/bridge` on real data after the synthetic test from Spec 1.
3. **Tier-1 epidemiology seam** — the highest-value expansion and the best stochadex fit; pursue once the bridge machinery is proven on bathing-water. Start with an age-eligibility screening/vaccination cutoff that has open aggregate outcomes (anchor) and a transmission/detection mechanism (simulator).
4. **Tier-1 environmental and economic seams** as capacity allows, each following the same pattern: harvest the anchor family first, build the mechanism simulator second, bridge third.

## 5. Standing principles (unchanged, restated because joins/simulators stress them)
- Design-based anchors only; the simulator never makes an anchor.
- Pin/span provenance is absolute; categories never pool.
- Every artefact re-mintable byte-for-byte, joins included.
- Open + redistributable end-to-end; verify licence per source (no longer uniform OGL).
- A small set of bulletproof, coherent anchor families beats a large set of isolated marks — the yardstick's value is trust, not volume.