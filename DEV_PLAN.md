# CAUSAL YARDSTICK v3 (working title)
## An open set of causally-validated institutional-decision reference points for testing models that make counterfactual claims

**One-line:** A curated, openly-published set of real UK public-sector decisions — each triggered when an institution's measured performance crosses a published threshold — where the true effect of the decision is recovered by regression discontinuity and shipped as an *honest interval* (central estimate + identification uncertainty + a validity dossier). Its purpose is to test whether any model that makes counterfactual claims (world models, model-based policies, digital twins, LLMs answering "what would happen if…") gets the causal effect right at points where ground truth is genuinely known.

**What it is not:** not a live system, not a recommender, not a leaderboard-first benchmark, not a training corpus. It is a small, bulletproof, fully open reference instrument.

---

## 1. Why "yardstick," and why "institutional"

**Yardstick, not benchmark.** A benchmark ranks methods and needs breadth and a leaderboard. A yardstick provides reference points so trustworthy that a model's counterfactual claims can be checked against them. This reframing makes the design's apparent weaknesses into virtues: a small mark count is fine (a ruler needs accurate marks, not many), and the local-to-cutoff estimand is the *design* (precise reference points, not a continuous field), not a caveat.

**Institutional decision units, not individuals.** A scouting pass across UK open data established a hard structural constraint: the conjunction the yardstick needs — a sharp published threshold + a publicly available running variable + a separable public outcome *at the same unit* + plausible non-manipulability — almost never holds for decisions about *individuals*, because individual-level outcomes (income, health, attainment) are locked inside the ONS Secure Research Service, whose safe-outputs rule forbids publishing the row-level dataset. It DOES hold for decisions about *institutions* (schools, NHS trusts, local authorities), whose running variables and outcomes are routinely published openly at matching resolution. The yardstick is therefore built entirely on threshold-triggered decisions about institutions. (See §7 for what was ruled out and why.)

---

## 2. The confirmed v1 seams (three domains, three unit-types, two RDD regimes)

| Seam | Domain | Unit | RDD type | Running variable (open source) | Manipulability |
|---|---|---|---|---|---|
| School floor standards | Education | School | Sharp | Performance score — DfE performance tables / Explore Education Statistics | Low–moderate |
| SHMI mortality banding | Health | NHS trust | Fuzzy | SHMI ratio + banding — NHS England / NHS Digital, monthly, open | Low |
| Area funding prioritisation (e.g. UKSPF top-20% deprivation) | Local government / regional policy | Local authority | Sharp | IMD percentile — MHCLG English Indices of Deprivation | Very low |

This spread is deliberate and is itself a strength: three distinct policy domains, three institutional unit-types, and a mix of **sharp** and **fuzzy** RDD — demonstrating the instrument works across both identification regimes. Manipulability *improves* down the table, so the area-funding seam (IMD is government-computed and cannot be self-reported or appealed across the boundary) serves as the cleanest anchor mark, with floor standards and SHMI adding domain breadth.

Each seam's "action" differs in sharpness:
- **Floor standards** — crossing the floor deterministically triggers a named intervention → sharp.
- **Area funding** — crossing the percentile boundary deterministically changes funding prioritisation → sharp.
- **SHMI** — crossing the banding boundary raises the *probability* of scrutiny/intervention ("smoke alarm"), not a certainty → fuzzy RDD. Valid and well-understood; fits the fuzzy-but-honest mark design exactly.

---

## 3. What one mark is (fuzzy-but-honest)

A mark is **not** a single number τ. It is a distribution over the true effect, whose width comes from *identification* uncertainty (bandwidth, specification), not just sampling error:

- **Context (pre-decision state):** covariates the decision-maker observed before the action, incl. the running variable. Pre-treatment only.
- **Running variable + cutoff `c`:** the published numeric threshold that (sharply or fuzzily) assigns the action.
- **Action / alternative action(s):** the policy lever and its counterfactual (the field a pure effect-estimation benchmark lacks — this is what makes it a *decision* yardstick).
- **Outcome:** later, observable in open data at the same unit.
- **The mark itself:** central effect estimate + an honest interval combining bandwidth/specification/sampling uncertainty.
- **Validity dossier (ships with every mark):** McCrary/density test (manipulation/sorting), covariate-continuity at `c`, placebo cutoffs, bandwidth sweep, donut robustness; for fuzzy seams, the first-stage discontinuity in treatment probability.
- **Provenance:** source URIs, per-source licence, IMD/data vintage, funding round, `context_asof` ≤ `decision_timestamp` < `outcome_timestamp`.

**Admission rule:** an episode is admitted if it passes the *validity* tests (no manipulation, no covariate jump, clean placebos, and — for fuzzy seams — a real first-stage jump). It is NOT rejected for a wide interval; width is information and is shipped. Rejection is only for *invalidity*, never for *imprecision*.

---

## 4. How a model is scored (distribution vs distribution), two tracks

A model under test supplies, for a mark's context + action set, its predicted effect *with its own uncertainty*. Two independently-scored tracks:

**Track A — Decision-value consistency.**
- Sign test: does the model get the direction of value(action) − value(alternative) at the cutoff right?
- Decision-regret consistent with the mark's interval (no penalty for being unsure where the mark is itself unsure).

**Track B — Calibration against truth (headline; the SBI/Bayesian edge).**
- Consistency: does the model's predicted interval overlap the mark's honest interval?
- Calibration curve: when the model claims X% confidence, is it right X% of the time across marks, *accounting for each mark's own width*?
- Confidently-wrong detector: flagged only when the model is narrow-and-wrong AND the mark is narrow-and-known — the fair version of catching hallucinated counterfactuals.
- Scoring rule: a proper scoring rule (e.g. CRPS-style) comparing model distribution to mark distribution.

**Predicted, publishable finding:** plug-in RDD methods reporting only sampling SE will systematically *fail* Track-B calibration because they understate the identification uncertainty the marks correctly include; principled Bayesian/SBI methods that propagate specification uncertainty should pass. That contrast is a result, not a leaderboard row.

---

## 5. The central intellectual risk (decide deliberately)

Comparing a model's *predictive* uncertainty to a mark's *identification* uncertainty requires stating how the two are treated as commensurable — they are not the same animal. The scoring rule (§4) must make this explicit (interval coverage vs CRPS vs aggregated calibration curve). This is the part a reviewer probes hardest and the part the author's Bayesian-model-comparison background is built to argue. It is a genuine intellectual risk, not a detail: if the two uncertainties turn out not to be cleanly comparable, the headline calibration measurement weakens.

---

## 6. Leakage / point-in-time integrity

Every mark must satisfy and expose: `context_asof` ≤ `decision_timestamp` < `outcome_timestamp`; the running variable pinned to its decision-time vintage (critical for IMD/SHMI, which are periodically revised); no post-treatment variable in the admissible control set. For percentile-based cutoffs (area funding), each mark is pinned to a specific funding round + IMD vintage because the boundary moves when the index is recomputed.

---

## 7. What was ruled out, and why (honest scoping — belongs in the paper)

- **Individual-level thresholds** (FSM/pupil premium £7,400 income; any person-level policy): outcomes locked in ONS SRS, whose safe-outputs rule forbids open publication of the dataset. Incompatible with an open artefact.
- **Small Business Rate Relief (£12k/£15k RV):** sharp threshold, but the running variable (rateable value) is *appealable* → manipulation/sorting at the cutoff; and no individual-level open outcome join (property→firm→survival has no open key).
- **Ofsted ratings; CQC special measures:** triggered by a *judgement*, not a numeric cutoff → no continuous running variable → no RDD.
- **Company audit-exemption thresholds:** "two of three" multi-criteria rule (no single discontinuity) + two-year hysteresis + self-reported, highly manipulable financials.

Stating these rejections is part of the contribution: it documents *why* open-data causal yardsticks must be institutional.

---

## 8. Deliverable shape
- **Corpus:** versioned, immutable-append; parquet + JSON marks + per-mark validity dossiers; one record per institution × decision-period. MIT for schema + evaluator; source data licences passed through with attribution.
- **API (serves past marks only):** `GET /marks`, `GET /marks/{id}` (full spec + provenance + dossier), `POST /score` (evaluate a submission on Tracks A and B), snapshot dumps. Read-only.
- **Evaluator:** scores Track A and Track B independently; ships a plug-in local-linear RDD baseline and one principled Bayesian/SBI reference method as the Track-B frontier.
- **Companion post** ("Engineering Smart Actions in Practice"): the institutional-threshold harvest, the validity protocol, the open-vs-SRS scoping decision, and the plug-in-vs-Bayesian calibration finding.

---

## 9. v1 scope
- Three seams above; sharp RDD for floor standards + area funding, fuzzy RDD for SHMI.
- Target a tight set of fully-dossiered marks per seam (quality over count); a small total is acceptable and on-brand for a yardstick.
- Both tracks; plug-in baseline + one Bayesian/SBI reference method.
- Explicit limitations: local-to-cutoff estimand, single defensible spec per mark, periodic-vintage running variables, fuzzy-seam first-stage dependence, and the commensurability question (§5).

---

## 10. Novelty statement (scoped, defensible)
The first **open** causal yardstick that scores counterfactual-claiming models against **real-world, quasi-experimentally-identified institutional-decision reference points carrying honest identification uncertainty**. The field currently validates counterfactual world models almost exclusively against *simulators* (exact but synthetic), because real-world counterfactual ground truth is assumed unavailable; this supplies exactly that, via RDD, openly. Claim the wiring (RDD ground truth + honest-uncertainty marks + counterfactual-model calibration scoring + open institutional sourcing), not the individual components, each of which is mature.

---

## 11. Honest risk register
- **Mark yield per seam** is the binding empirical constraint — density/continuity tests will reject candidates; resolved only during build, not by more scoping.
- **Commensurability of the two uncertainties** (§5) is the real intellectual risk.
- **Periodic-vintage running variables** (IMD, SHMI revisions) require careful point-in-time pinning to avoid leakage.
- **Fast-moving field** — counterfactual world-model evaluation is active (multiple 2026 papers); the open-real-world-ground-truth lane is open today but should be claimed promptly and scoped tightly.
- **Novelty is recombination at an intersection**, not a new primitive — pitch accordingly.