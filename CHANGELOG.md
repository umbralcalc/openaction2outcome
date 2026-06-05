# Changelog

All notable changes to this project are recorded here. Versions refer to the
published dataset + tooling release (the wire-format `schema_version` is tracked
separately inside each mark).

## v1.10.0 — 2026-06-05

Lands the **first difference-in-differences mark**: `ca-menthol-smoking-2016` — Canada's
staggered provincial menthol-cigarette bans (NS/PE/AB 2015, NB/QC 2016, ON/NL 2017) vs the
provinces covered only by the federal 2 Oct 2017 ban (BC, MB, SK), on adult current-smoking
prevalence. **ATT −1.79 pp, honest 95% interval [−3.09, −0.49]** — the corpus's first mark
whose interval **excludes zero**: a real, design-identified reduction, with parallel
pre-trends holding and a fake pre-period ban showing no effect. The ~1.8 pp on *total*
smoking is the literature's ~3 pp *menthol-specific* effect diluted by substitution. First
anchor on a new `menthol-restriction-to-smoking` mechanism — the cross-country bridge
partner for US comprehensive flavour bans.

- **`internal/did` put to work** (the estimator existed but nothing used it): unit-clustered
  2×2 with the pre/post window sweep folded into the honest interval (sampling + specification).
- **First DiD schema-fit**: `internal/dossier` gains a `renderDiD` path (treated/control,
  parallel trends, placebo year, window sweep, leave-one-province-out — not RDD
  manipulation/continuity checks); panel episode rows (province × year).
- **Data**: adult current-smoking prevalence by province × year from the Canadian Community
  Health Survey, stitched across the 2015 CCHS redesign from StatCan tables 13-10-0451
  (2007–2014) + 13-10-0096 (2015+) — the redesign is a common shock the DiD differences out.
  **Statistics Canada Open Licence** (a third open licence in the corpus, alongside OGL v3.0
  and CC BY 4.0). Frozen + hash-pinned via `scripts/menthol_harvest.py`.
- Validity caveats in the dossier: the federal Oct-2017 ban caps the clean post window at
  2017; only 3 control provinces, so the interval is wide; the effect is on total (not
  menthol-specific) smoking.

## v1.9.0 — 2026-06-04

Lands the **first controlled-ITS mark**: `berlin-lez-no2-2010` — the Berlin Umweltzone
stage-2 (1 Jan 2010, green-sticker Euro-4-diesel **ban** inside the S-Bahn Ring) on
roadside NO₂. It is the first identified anchor on a new `lez-ban-stringency-to-roadside-no2`
mechanism, kept deliberately **separate** from charge-type zones (London ULEZ) so a future
bridge interpolates a coherent mechanism. Effect **−1.2 µg/m³, honest 95% interval
[−6.7, +4.8]** — a near-null with a wide interval: the honest reading that a standard-ban
LEZ moved roadside NO₂ little (the Euro-standard upgrades cut particulates/soot far more
than NOₓ). It is admitted because its validity battery is clean (a flat in-zone-traffic-
minus-background pre-trend → clean placebos), demonstrating that a wide honest interval
around a null is information, not a failure.

- **`internal/its`**: new controlled-ITS estimator — a segmented regression on the
  treated−control monthly difference with Newey-West (Bartlett) HAC standard errors and a
  model-averaged honest interval (pre-window × seasonal harmonics × level/slope ×
  meteorology), mirroring the RDD marks' bandwidth×order×kernel averaging. Synthetic-truth
  tested (recovers a known break; the interval covers and decomposes into sampling +
  identification variance; placebos clean).
- **`internal/series`**: an event-parametrised LEZ→NO₂ builder (`ulezEvent`) driving the
  Berlin mark, with pre-trend control matching, a placebo-adequacy guard, and the ITS
  validity battery (parallelism, no-anticipation, placebo dates, window sweep,
  meteorology-adjusted-vs-unadjusted, autocorrelation). Admission gates on the
  manufactured-break guards (placebos + no-anticipation), not on a flat pre-trend.
- **`internal/ingest/laqn.go`**: a monthly NO₂-panel + meteorology loader (shared by the
  London and Berlin harvests).
- **data**: `data/raw/berlin-lez-no2` (EEA historical/AirBase hourly NO₂ → monthly,
  **CC BY 4.0**) + `data/raw/berlin-lez-meteo` (Open-Meteo ERA5, CC BY 4.0). Harvested by
  `scripts/berlin_lez_harvest.py` (EEA Air Quality Download Service → per-station parquet),
  hash-pinned and frozen. Window 2008–2011, **pre-COVID**.
- **London ULEZ → NO₂ shelved** as a declared seam (`emission-zone-stringency-to-roadside-no2`,
  no admitted anchor): the 2023 and 2019 events were built and run, but COVID and a curved
  pre-trend defeat clean identification, and the confounder series that would rescue it is
  only in non-OGL scanned PDFs. See `research/2026-06-04-ulez-no2-its-covid-confound.md`.
  The build is wired (`--series ulez-no2`) and reproducible but excluded from `build-all`.

## v1.8.0 — 2026-06-03

Adds **controlled interrupted time series (ITS)** as a second identification strategy
alongside regression discontinuity — the design family behind the dose / rollout
price-instrument seams (Scotland alcohol minimum-unit pricing, London emission zones)
where the policy lands at a sharp *instant in time* and comparability is a control
series sharing the treated series' pre-trend. An ITS estimand is a **population** effect
over a post-intervention window, not a local-at-cutoff effect, so its decision scores
never pool with RDD marks (the same firewall already used for identified vs bridge).
Purely **additive** — `schema_version` stays 0.5.0; every mark, episode file, and scorer
minted before this keeps validating unchanged.

- **`schema`**: a new `identification` discriminator (`rdd-sharp` / `rdd-fuzzy` /
  `rdd-kink` / `did` / `its-controlled`) that selects which `design` sub-shape and which
  `dossier` block a reader expects. Legacy `rdd_type` migrates via
  `EffectiveIdentification`; a contradiction between the two is rejected.
- **`schema`**: `Design.ITS` carries the ITS-only design fields (intervention instant,
  pre/post windows, transition ramp, counterfactual model, control series) in place of
  the RDD running_variable/cutoff/direction; `Dossier.ITS` holds the time-domain validity
  battery (no-anticipation, control-parallelism, placebo dates/outcomes, window sweep,
  transition exclusion, dose check, autocorrelation). New `row_shape` (`cross-section` /
  `panel`) and a `PanelObservation` row type for the ITS panel. `Validate` enforces the
  mandatory ITS fields, requires a control on an *identified* ITS mark (uncontrolled ITS
  belongs in a bridge), and keeps panel rows' post-flag consistent with their distance.
- **`internal/dossier`**: a distinct ITS dossier renderer (the time-domain analogue of the
  RDD dossier; shared effect/provenance sections), so a reader can never mistake a
  population-over-window estimand for a local-at-cutoff one.
- **`internal/episodes`** / **`internal/hfexport`**: the episodes manifest gains a per-mark
  `row_shape`, and the Hugging Face record gains an `identification` column.
- Executes `research/schema-its-addendum.md` (the 0.5.0 proposal).

## v1.7.0 — 2026-06-02

Adds the **difference-in-differences (DiD)** design — the gating prerequisite for the
dose / staggered-rollout bridge seams (alcohol minimum-unit pricing, emission zones,
minimum wage), where each anchor is one treated-vs-control, pre-vs-post comparison and
a *family* of such anchors at different policy intensities is what a bridge interpolates
across. This is the DiD analogue of the regression-kink work, and it unlocks all three
top dose/rollout candidates, not just one.

- **`internal/did`** — a clean 2×2 (unit-first-difference) DiD around a single event:
  per-unit post-mean minus pre-mean, compared across the treated and control groups,
  with **unit-clustered** inference and the same window-swept honest interval (sampling
  + specification variance) as the RDD/RKD estimators. Deliberately a single-event 2×2,
  which sidesteps the staggered-adoption / two-way-fixed-effects bias of a pooled
  regression. Ships **`PreTrend`** — the parallel-trends diagnostic (the cross-group
  pre-period trend, ≈0 when credible) — the DiD analogue of the RDD manipulation check.
  Validated on synthetic panels: exact effect recovery, pre-trend-violation detection,
  and spec-spread folding under a dynamic effect.
- **`schema`**: new `rdd_type=did`; `Validate` admits it without a cutoff Direction
  (DiD has treatment groups, not a cutoff side) and forbids the kink-only
  policy_slope_change. Additive — `schema_version` stays 0.5.0.

## v1.6.0 — 2026-06-02

Adds the **regression-kink design (RKD)** as a new identified-anchor design — the
prerequisite for tiered-relief anchor families (e.g. small-business-rate-relief
tapers), where the policy bends rather than jumps. Where a sharp/fuzzy RDD reads a
*level* discontinuity at a cutoff, an RKD reads a *slope* discontinuity at a kink and
divides it by the known kink in the policy function's slope, identifying the marginal
effect of the policy intensity.

- **`internal/rdd`** gains the plug-in RKD estimator: a kernel-weighted local-
  *quadratic* slope fit per side (the RKD convention for boundary derivatives),
  `FitKink` / `EstimateKink` with the same bandwidth-sweep honest interval as the RDD
  level estimator, and `LevelDiscontinuity` — the RKD validity check that the
  conditional mean is continuous at the kink (a non-zero level jump means a notch
  contaminates the design). Validated on synthetic data with a known marginal effect.
- **`schema`**: new `rdd_type=kink` and a required `design.policy_slope_change`
  (b'(c+) − b'(c−)); `Validate` enforces a non-zero value for kink designs and forbids
  it on level designs. Additive — `schema_version` stays 0.5.0.

## v1.5.0 — 2026-06-01

Introduces the **Mechanism** entity, making the collection a clean instance of the
bridge-marks data model. A mechanism is where anchor coherence is *defined* — two
anchors share a mechanism iff they share the policy variable, outcome construct,
population definition, and regime — and it is the unit a bridge spans. Grouping
marks under a mechanism is what turns "several isolated marks" into an anchor
family. This is the groundwork the bridge-seams expansion needs.

- **`schema.Mechanism`** + a registry of the declared seams (`area-funding-eligibility`,
  `floor-standards-p8`, `shmi-mortality-banding`, `bathing-water-classification`),
  exposed via `CanonicalMechanisms()` / `MechanismByID()`.
- New required **`mechanism_id`** field on every mark; `Mark.Validate` rejects a
  missing or unknown id, and requires a bridge mark's `mechanism_id` to equal its
  bridge block's mechanism. Each existing seam is a one-anchor mechanism today; the
  data model is identical when a family arrives — more anchors simply share the id.
- The three committed identified marks gain `mechanism_id` (values unchanged).
- Additive field — `schema_version` stays 0.5.0.

## v1.4.0 — 2026-06-01

Adds the **deterministic causal layer** — the first major extension on top of the
bridge foundation. It augments causal-modelling power while staying *determinism-first*:
the honest interval is computed by closed-form / deterministic moment propagation, never
by sampling the discrepancy, and an explicit tractability gate certifies that a
deterministic interval is honest for a given mechanism before one is minted. No
breaking wire-format change — the new provenance fields are additive and optional, so
`schema_version` stays 0.5.0 and existing marks are unchanged.

### Deterministic inference (rungs 1 & 2)
- **`CalibrateMoment` / `CalibrateDeterministic`** (`internal/bridge`) — a calibrator with
  no Monte Carlo: θ is fit by deterministic Gauss–Newton (a Laplace posterior) and pushed
  through the prediction map by the **unscented transform**. It is **exact in the
  linear-Gaussian limit** (verified against a closed-form analytic reference to round-off)
  and re-mints byte-for-byte (no RNG).

### The tractability gate (the Axis-B detector)
- **`TractabilityGate`** tests whether a mechanism stays in the deterministic regime and
  ships its verdict as provenance. Its load-bearing statistic is the **Laplace misfit**
  (the true negative-log-posterior vs its quadratic approximation, in nats): ~0 for a
  linear-Gaussian θ posterior, large for a genuinely non-Gaussian one. A failed gate is
  flagged and routed to the deferred sampling path rather than given a (miscalibrated)
  deterministic interval. Determinism is earned, not assumed.

### The causal mechanism + structured kernels
- **`LinearSCMMechanism`** — a directed structural causal model (a confounded
  treatment→outcome graph) built as a genuine stochadex deterministic graph (value-function
  nodes + upstream edges, run on the same simulator engine the identified marks use), with
  **analytic `do(·)` interventions**. The interventional slope differs from the confounded
  observational association — the directed graph carries interventional content a covariance
  alone cannot.
- New discrepancy kernels: an **Ornstein–Uhlenbeck / latent-force** kernel (ODE-derived,
  encoding a first-order linearised mechanism) and an **intrinsic-coregionalisation**
  multi-output kernel (PSD `B = WWᵀ + diag κ`, with joint multi-output Gram assembly) to
  bridge correlated outputs or jurisdictions.

### Provenance + validity
- New optional `inference` record on a bridge mark (and in the dossier): the **rung** used
  (`closed-form` | `deterministic-moment` | `sampled`) and the **tractability-gate verdict**
  with its statistics + tolerances, so a consumer sees exactly which method produced the
  interval and that determinism was certified.
- The bridge validity battery runs the tractability gate; a failed gate is surfaced
  prominently. The dossier renders the inference rung and the gate verdict.

### Validation artifact
- **`study --bridge --layer`** runs the deterministic-causal-layer study on the structural
  causal mechanism with a *known* interventional truth: the moment calibrator coincides with
  the SMC closed-form joint to within Monte-Carlo error while carrying no sampling noise,
  re-mints byte-for-byte, earns the closed-form rung at every problem, and recovers the
  known τ* between the anchors.

### Still deferred (data-gated)
- The first *real* bridge mark (needs a mechanism with ≥2 bracketing identified anchors) and
  the gated sampling rung remain deferred. The modelling layer raised the ceiling; coherent
  anchor families remain the binding constraint.

## v1.3.0 — 2026-05-31

Adds a second, clearly-separated category of mark — the **bridge mark** — and the
machinery to mint and score it. Bridge marks estimate a mechanism's effect at a
point bracketed by real identified anchors, using a stochadex simulator plus a
Gaussian-process discrepancy pinned to those anchors (Kennedy–O'Hagan calibration).
Interpolation only; the honest interval is the posterior, bounded and pinned at
both ends. Existing marks are unchanged in value — they gain a `category` field.

### Schema (`schema_version` 0.4.0 → 0.5.0)
- New mark fields: `category` (`identified` | `bridge`, empty reads as
  `identified`) and `truth_source` (`identified` | `simulator-bridged`) — the hard
  pin/span provenance line, never pooled across categories.
- New `bridge` block on bridge marks: `mechanism`, `policy_variable`, `query_point`,
  `anchors[]` (≥2, must bracket the query), `simulator`, `discrepancy_kernel`,
  `anchor_coherence`. **Bracketing is enforced in the data model** at validation
  time — a non-bracketed query is rejected (no extrapolation path).
- New `dossier.bridge` block: anchor-coherence echo, LOAO coverage (headline),
  kernel-sensitivity table, bracketing flag, admission verdict.
- The three committed identified marks are re-minted at 0.5.0 (byte-identical
  values; only the new `category`/`truth_source` fields + version string change).

### New machinery
- **`internal/bridge`** — the simulator interface, two GP covariance kernels
  (squared-exponential, Matérn-5/2), GP-discrepancy conditioning (reusing the
  dense linear-algebra idiom from `internal/sbi`), SMC over the mechanism
  parameters (the full weighted particle cloud), leave-one-anchor-out validation,
  and kernel sensitivity. Validated against a synthetic mechanism with a *known*
  effect curve (`study --bridge`): recovery and LOAO coverage track nominal.
- **`internal/validity`** gains the bridge battery (anchor coherence + bracketing
  as the hard gates; LOAO + kernel sensitivity as reported credibility numbers).
- **`internal/dossier`** renders a distinct bridge dossier — pin/span picture, the
  load-bearing kernel, the coherence justification, and LOAO as the headline.

### Determinism: closed-form, not sampled
- The discrepancy GP is conditioned in **closed form**, never sampled — the honest
  interval is exact and re-mintable. `internal/bridge` ships three calibrators:
  the modular cut (default), `CalibrateMarginal` (the *exact* joint — GP marginal
  likelihood for θ plus analytic δ conditioning), and a stochadex-*sampled* joint.
- New `study --bridge --compare` runs all three on identical known-truth problems
  and is the committed evidence for the closed-form choice: the two closed-form
  calibrators nearly coincide and track nominal coverage (recovery ≈ 1.0 at 95%),
  while the sampled joint degenerates (recovery ≈ 0.25, intervals several-fold too
  narrow) as its data-free discrepancy latents collapse under resampling. Sampling
  the GP adds only Monte-Carlo error and degeneracy.

### Scoring
- `pkg/score` now reports **per-category, never pooled** (so strong identified
  coverage cannot mask weak bridge coverage). New `--category identified|bridge|both`
  flag on `score`; excluded marks are reported, never silently dropped.

## v1.2.0 — 2026-05-31

Reworks the `episodes` distribution to match what is actually served: the rows are
published **per mark** as one gzipped CSV each, not as a single unified file. The marks
and the row *values* are unchanged.

### Storage / dataset (per-mark `episodes`)
- The `episodes` dataset is published as **one gzipped CSV per mark** in object storage at
  `marks/<id>/episodes.csv.gz` (columns: `unit_id`, `unit_name`, `running_value`,
  `assigned`, `treated`, `outcome`, + the mark's covariates). A mark's file *is* its rows;
  the per-mark design (`cutoff`/`direction`/`action`) and the full effect distribution are
  read from the mark JSON, joined on the mark `id`.
- **`datasets/episodes.manifest.json`** is now a per-mark listing: `marks[]` with each
  file's `uri` + `sha256` + `bytes` + `rows` + `covariates`, so every download is
  individually verifiable. (Previously it pointed at a single unified Parquet URL that was
  never actually served.)
- `scripts/publish.sh` uploads and verifies each `marks/<id>/episodes.csv.gz`.
- **Removed the unioned `episodes` Parquet** (and the `parquet-go` dependency, with its
  whole indirect chain). It was a convenience mirror carrying a second, denormalised
  schema; everything it held is derivable from the per-mark CSV + the mark JSON. Hugging
  Face now mirrors the **same per-mark CSVs** at `episodes/<id>.csv.gz` — one row shape
  everywhere; load one mark with `load_dataset(repo, data_files="episodes/<id>.csv.gz")`
  (or `huggingface_hub.hf_hub_download`). The `episodes` config is gone; the per-series
  mark configs are unchanged.

## v1.1.0 — 2026-05-31

Adds a documentation website. No dataset, schema, or scorer changes — the marks and the
`episodes` dataset are byte-identical to v1.0.0.

### Documentation site
- **Static GitHub Pages site** (`make site` → `openaction2outcome site`): generates a
  self-contained website into the committed `docs/` folder — a landing page (coverage
  cards generated from the marks), a downloads page (per-mark `episodes.csv.gz` with their
  SHA-256, a content-addressed `marks.zip`, the Hugging Face mirror, and the frozen
  raw-input table), the rendered schema and changelog docs, and a page per mark dossier.
  Generated offline and deterministically from artifacts already in the repo, so it can't
  drift from the data. Served by Pages from `main` → `/docs`. See
  [PUBLISHING.md](PUBLISHING.md).

## v1.0.0 — 2026-05-31

First stable release. Restructures distribution around the dataset model: there
are now exactly **two datasets**, normalised on `mark_id` — the marks (metadata,
in git) and a single row-per-unit `episodes` dataset (the rows, in object
storage).

### New dataset
- **`episodes`** — every mark's per-unit rows, unioned into one table for model
  training: each unit's context before the decision (`covariates`,
  `running_value`/`distance_to_cutoff`/`direction`), what was done (`assigned`/`treated`,
  `action`/`alternative`), and the `outcome` that followed (`outcome_observed` flags
  attrition). The (state, action, reward) view, in the dataset's own terms. Published as
  one deterministic, content-addressed Parquet (object storage) and loadable as the
  Hugging Face `episodes` config; pointed to by `datasets/episodes.manifest.json`. Each
  row carries `mark_id` (+ scalar `effect_*` summaries) to join back to the mark's full
  effect distribution.

### Schema (`schema_version` 0.3.0 → 0.4.0)
- **Breaking:** removed the per-mark `data` artifact (`DataArtifact`). A mark is now pure
  metadata; its rows live in the `episodes` dataset, recovered by filtering on the mark
  `id`. Per-mark episode tables are no longer published — they are a build intermediate
  feeding the episodes reshape.

### Storage
- Object storage now holds just the `episodes` dataset plus the frozen raw-input mirror
  (reproducibility). The mark-level Hugging Face records drop the `episode_table_*`
  fields. (Superseded by v1.2.0: the `episodes` dataset is published per mark, not as a
  single unified file.)

## v0.2.0 — 2026-05-31

Adds a third series and the first environmental-domain mark, taking the
collection to three domains (education + health + environment). Introduces
seam-specific validity checks beyond the standard battery, debuting with the
bathing-water abnormal-sample-exclusion sensitivity.

### Marks (admitted reference points)
- **bathing-water-poor-2015** — sharp RDD on the English bathing-water Poor/Sufficient
  classification boundary (revised Bathing Water Directive). Running variable is the
  base-10 log compliance margin (worst of the E. coli / intestinal enterococci
  90th-percentile statistic over its Sufficient threshold, coastal vs inland);
  crossing into Poor triggers a mandatory advice-against-bathing sign + EA catchment
  investigation. Outcome is the same site's compliance margin four years later
  (2015 → 2019, non-overlapping rolling sample windows). Effect −0.095, 95% interval
  [−0.407, 0.245] (the bandwidth sweep flips sign — genuine identification
  uncertainty, honestly reported).

### Schema
- New `bathing-water` series value, and an optional `seam_specific_checks` field on
  the validity dossier for series-specific tests outside the standard battery.

### New validity check
- **Abnormal-sample-exclusion sensitivity** (the bathing-water manipulation analogue):
  re-includes the discretionarily-discounted extreme-weather samples, re-derives the
  log-normal 90th-percentile margin near the cutoff, and confirms the effect estimate
  is robust. The mark records how many samples were discounted near the boundary and
  how many sites flip Poor/Sufficient under re-inclusion.

### Data
- Bathing-water inputs are harvested from the Defra linked-data API (no bulk CSV
  upstream) by `scripts/bathingwater_harvest.py` into two frozen, hash-pinned CSVs
  (annual compliance + percentile statistics; per-sample microbiology for near-cutoff
  windows), mirroring the existing derived-snapshot pattern.

## v0.1.0 — 2026-05-30

First public release: a working causal yardstick on two domains, with both
identification regimes implemented and the headline calibration finding
demonstrated.

### Marks (admitted reference points)
- **floor-standards-p8-2016** — sharp RDD on the 2016 English school Progress 8
  floor (−0.5); outcome is each school's Progress 8 two years later. Effect
  0.028, 95% interval [−0.054, 0.256].
- **shmi-higher-than-expected-banding** — sharp intention-to-treat RDD on the NHS
  SHMI "higher than expected" mortality banding; outcome is the trust's SHMI in
  the next 12-month window (pooled trust-years). Effect −0.013, 95% interval
  [−0.066, 0.018].

### Estimation
- Model-averaged SBI estimator (stochadex SMC across a bandwidth × order × kernel
  grid; marginal-likelihood weighting within a bandwidth, uniform across). The
  honest interval splits into sampling vs identification variance.
- Fuzzy two-stage (Wald LATE) estimator with a first-stage admission gate.
- Plug-in local-linear estimator kept as the comparison baseline.

### Headline finding
- Calibration study against known truth: at a nominal 95% interval the plug-in
  method covers the truth 80% of the time, the model-averaged method 92%.

### Tooling & distribution
- CLI: `fetch`, `build`, `validate`, `score`, `study`, `export`.
- Two public, dependency-light Go packages: `pkg/schema` and `pkg/score`.
- Deterministic, re-mintable marks; per-mark validity dossiers.
- Storage: git holds the instrument; frozen inputs and episode tables live in
  object storage (Cloudflare R2), referenced by URL + SHA-256.
- Hugging Face dataset export (one config per series) + Dataset Card.

### Honest scoping
- Documented why a clean area-funding mark is deferred (UKSPF too recent for an
  outcome; the Neighbourhood Renewal Fund running variable is not openly
  reconstructable; the CQC-instrumented fuzzy SHMI first stage is too weak to
  admit). The fuzzy estimator runs on this real data but the mark is not admitted.
