# Data dictionary

This is the field-by-field reference for everything you download or submit. The
Go types it describes live in [`pkg/schema`](../pkg/schema); the version string is
`schema_version` (currently `0.5.0`).

There are three things to know about:

1. **A mark** — one causally-validated reference point (a JSON file in [`marks/`](../marks)). This is the *metadata*.
2. **The `episodes` dataset** — the per-unit rows behind every mark, in object storage as one gzipped CSV per mark, joined to the marks on the mark `id`.
3. **A submission** — what you send to be scored, and the score you get back.

There are only ever those two datasets — the marks (metadata, in git) and the
episodes dataset (rows, in object storage) — normalised on `mark_id`.

---

## 1. Mark

One mark is one real decision whose true effect is known: an institution crossed a
published threshold (or a policy switched on at a known date), that triggered an
action, and a later outcome is observable. The effect is given as a distribution (a
central estimate plus an interval whose width is honest about how much is genuinely
uncertain), not a single number.

A mark is **identification-agnostic**: the effect distribution, the uncertainty
budget, the provenance, and the scorer are the same regardless of *how*
comparability was established. Only the `design` block and the validity `dossier`
change shape per design family — selected by the `identification` field (see
[Designs](#designs-the-identification-field)).

| Field | Type | Meaning |
|---|---|---|
| `schema_version` | string | Schema version this mark was written against. |
| `id` | string | Stable unique identifier, e.g. `floor-standards-p8-2016`. |
| `series` | string | Which group of marks this belongs to: `floor-standards`, `shmi`, `bathing-water`, or `area-funding`. |
| `domain` | string | Human label, e.g. `Education`. |
| `unit_type` | string | The kind of institution, e.g. `school`, `nhs-trust`, `local-authority`. |
| `category` | string | `identified` (design-based truth — a pin) or `bridge` (simulator-bridged interpolation between anchors — a span). Empty reads as `identified`. The two are never pooled in scoring. |
| `truth_source` | string | `identified` for design-based marks, `simulator-bridged` for bridge marks. The hard provenance line. |
| `identification` | string | The design family: `rdd-sharp`, `rdd-fuzzy`, `rdd-kink`, `did`, or `its-controlled`. Selects the `design` sub-shape and the `dossier` block. Identified marks only. See [Designs](#designs-the-identification-field). |
| `rdd_type` | string | **Legacy** discriminator (`sharp` / `fuzzy` / `kink` / `did`); migrates to `identification` automatically. Optional once `identification` is set; the two must not contradict. |
| `row_shape` | string | The shape of this mark's episode rows: `cross-section` (RDD/DiD — one row per unit) or `panel` (ITS — one row per series × time bucket). Empty is derived (panel for ITS, else cross-section). |
| `design` | object | What is being estimated — see below. Identified marks only. |
| `bridge` | object | Bridge-specific fields (anchors, query point, simulator, kernel, coherence) — see below. Bridge marks only. |
| `context` | object | Pre-decision information a model is allowed to use. |
| `sample` | array | A small inline excerpt of cross-section episode rows nearest the cutoff, for quick inspection (the full rows are in the `episodes` dataset, keyed by this mark's `id`). |
| `panel_sample` | array | The ITS analogue of `sample`: a small inline excerpt of panel episode rows (treated and control series near the intervention instant). ITS marks only. |
| `effect` | object | The mark itself: the effect distribution — see below. |
| `dossier` | object | The validity checks the mark passed — see below. |
| `provenance` | object | Sources, licences, timestamps, and reproducibility metadata. |

### Designs (the `identification` field)

Every identified mark is recovered by one of four quasi-experimental designs. The
`identification` field names the family and selects the `design` sub-shape and the
`dossier` checks; everything else about the mark is identical across families.

| `identification` | Design | Comparability from | Estimand | `design` carries | `row_shape` |
|---|---|---|---|---|---|
| `rdd-sharp` | Sharp regression discontinuity | Units just either side of a cutoff | Local effect at the cutoff | `running_variable`, `cutoff`, `direction` | `cross-section` |
| `rdd-fuzzy` | Fuzzy regression discontinuity | As sharp, but the cutoff shifts the *probability* of action (real first stage) | Local LATE at the cutoff | as sharp; `dossier.first_stage` required | `cross-section` |
| `rdd-kink` | Regression-kink design (RKD) | A change in the **slope** of a continuous policy function at a kink | Marginal effect of policy intensity | as sharp + `policy_slope_change` (non-zero) | `cross-section` |
| `did` | Difference-in-differences | A treated group's pre→post change vs a control group's, under parallel trends | Average effect on the treated | the shared fields; no cutoff `direction` | `cross-section` |
| `its-controlled` | Controlled interrupted time series (ITS) | A treated series' break at a sharp intervention *instant* vs a control series sharing its pre-trend | **Population** effect over the post window | the shared fields + the `design.its` block | `panel` |

The first three are discontinuity designs (a cutoff in a running variable); `did`
and `its-controlled` are panel designs (a treatment/control split in groups or in
time). **Decision scores are comparable within a family, never pooled across** — a
local-at-cutoff RDD estimand and a population-over-window ITS estimand answer
different questions. Calibration scores remain comparable everywhere.

The legacy `rdd_type` field (`sharp` / `fuzzy` / `kink` / `did`) still reads and
migrates to `identification`; the two must not contradict.

### `bridge` (bridge marks only)

A bridge mark estimates a mechanism's effect at a `query_point` that lies strictly **between** identified anchors. Interpolation only; the boundary to identified truth is made unmissable here.

| Field | Type | Meaning |
|---|---|---|
| `mechanism` | string | The shared effect-curve mechanism id all anchors lie on. |
| `policy_variable` | string | What `x` is: `cutoff level`, `intensity`, `dose`, `time`, `rank`. |
| `query_point` | number | The `x` this mark estimates the effect τ at. Always strictly inside the anchor hull. |
| `anchors` | array | `≥2` `{mark_id, policy_point}` references to identified marks; must **bracket** `query_point` (one each side). |
| `simulator` | object | stochadex `{model_id, version, seed, input_hashes}` — re-mintable byte-for-byte. |
| `discrepancy_kernel` | object | GP covariance `{family, params, jitter}` — the trust-decay assumption, shipped openly. |
| `anchor_coherence` | object | `{same_population, same_regime, same_outcome_construct, justification}` — the mandatory argument that anchors share one mechanism. |

The bridge mark's `dossier.bridge` records the leave-one-anchor-out (LOAO) coverage (the headline credibility number), the kernel-sensitivity table, the bracketing check, and the admission verdict.

### `design`

The shared fields below apply to every design family. The discontinuity designs
(`rdd-sharp` / `rdd-fuzzy` / `rdd-kink`) carry the running-variable/cutoff triplet;
`did` omits `direction` (it has treatment groups, not a cutoff side); `its-controlled`
replaces the triplet with the `design.its` block.

| Field | Type | Meaning |
|---|---|---|
| `running_variable` | object | The measured quantity the decision depends on (`name`, `description`, `units`, `source_id`). Discontinuity designs. |
| `cutoff` | number | The published threshold. Discontinuity designs. |
| `direction` | string | `above-treated` or `below-treated` — which side of the cutoff receives the action. Discontinuity designs. |
| `policy_slope_change` | number | (RKD only) the known change in the policy function's slope at the kink, `b'(c+) − b'(c−)`; must be non-zero. The effect is the kink in the outcome's slope over this. |
| `action` | string | What happens when the action is triggered. |
| `alternative` | string | The counterfactual: what happens otherwise. |
| `outcome` | object | The later observable quantity (`name`, `description`, `units`, `source_id`). |
| `estimand` | string | Plain-language statement of exactly what the effect measures (note: a *population* effect over the post window for ITS, not a local-at-cutoff effect). |
| `its` | object | (ITS only) the controlled-interrupted-time-series design block — see below. |

#### `design.its` (ITS marks only)

The time-domain analogue of the running-variable/cutoff/direction triplet: the
forcing variable is time, the "cutoff" is a sharp intervention instant, and
comparability comes from a control series sharing the treated series' pre-trend.

| Field | Type | Meaning |
|---|---|---|
| `intervention_instant` | string | The sharp date/time the action took effect (ISO 8601) — the time-domain analogue of `cutoff`. |
| `running_time` | object | `{name, description, units, source_id}` for the time axis (e.g. units `month`). |
| `pre_window` | object | `{start, end}` — the pre-intervention period used to fit the counterfactual. |
| `post_window` | object | `{start, end}` — the period over which the effect is accumulated/averaged. |
| `transition` | object | `{start, end}` or absent — an implementation ramp excluded from both windows (the time-domain donut). |
| `counterfactual` | object | `{family, terms, seasonality, justification}` — the model of what the treated series would have done absent the action. The single biggest specification choice; recorded openly. |
| `control` | object | `{series_id, role, justification}` — the comparison series (e.g. England for a Scotland intervention). Required on an identified ITS mark; uncontrolled ITS should be a `bridge`. |

### `effect` (the distribution)

| Field | Type | Meaning |
|---|---|---|
| `central` | number | Central estimate of the effect. |
| `interval` | object | `{level, lower, upper}` — e.g. a 95% credible interval. |
| `std_dev` | number | Standard deviation of the effect distribution. |
| `quantiles` | array | `{p, value}` points of the distribution, for finer comparison. |
| `samples` | array | Representative samples of the distribution (useful for scoring). |
| `uncertainty_budget` | object | Splits the variance into `sampling` (finite data) and `specification` (which modelling choices were made). The second is what naive methods leave out. |

### `dossier` (validity)

A mark is included only if it passes these checks; it is never excluded for being
*uncertain* (a wide interval is information, not a failure). The discontinuity/DiD
checks below are the default; an ITS mark carries the time-domain `dossier.its`
block instead (same epistemic intents, see below).

| Field | Meaning |
|---|---|
| `density` | Manipulation/sorting test: did units bunch on one side of the cutoff? |
| `covariate_continuity` | Pre-decision characteristics should not jump at the cutoff. |
| `placebo_cutoffs` | The effect should vanish at fake cutoffs away from the real one. |
| `bandwidth_sweep` | How the estimate moves as the analysis window changes. |
| `donut_robustness` | Re-estimate after dropping units right at the cutoff. |
| `first_stage` | (fuzzy only) the jump in treatment probability at the cutoff. |
| `admitted` | Overall verdict. |
| `notes` | Plain-language caveats (e.g. attrition). |

#### `dossier.its` (ITS marks only)

Each check is the time-domain analogue of an RDD check, carrying the same intent.

| Field | Meaning (mirrors) |
|---|---|
| `no_anticipation` | No pre-trend break or forestalling before the intervention instant (mirrors `density`). |
| `control_parallelism` | Treated and control share a pre-intervention trend (mirrors `covariate_continuity`). |
| `placebo_dates` | The effect vanishes at fake intervention dates in the pre-period (mirrors `placebo_cutoffs`). |
| `placebo_outcomes` | A logically unaffected outcome shows no effect (mirrors `placebo_cutoffs`, second axis). |
| `window_sweep` | Estimate stability as the pre/post window lengths vary (mirrors `bandwidth_sweep`). |
| `transition_exclusion` | Re-estimate after dropping the implementation ramp (mirrors `donut_robustness`). |
| `dose_check` | The action was actually delivered (sales/price/compliance moved at the date) (mirrors fuzzy `first_stage`). |
| `autocorrelation` | Residual serial correlation modelled (Newey-West / ARMA errors); ITS-specific. |
| `admitted` / `notes` | Overall verdict and caveats. |

A rendered, human-readable version of this is in [`dossiers/`](../dossiers).

### `provenance`

`context_asof` ≤ `decision_timestamp` < `outcome_timestamp` is enforced, so no
information from after the outcome can leak in. Also records each source's
licence and SHA-256, the random seed, and tool versions — enough to re-mint the
mark byte-for-byte.

---

## 2. The `episodes` dataset

The analysis-ready rows behind every mark: one row per unit. It is **not** in git — it
lives in object storage, published **per mark** (one gzipped CSV each), pointed to by
[`datasets/episodes.manifest.json`](../datasets/episodes.manifest.json). The manifest
lists every mark's file with its download URL + SHA-256 + size, so each download is
verifiable. You only need it if you want to train or refit on the rows; scoring a model
does not require it. A mark's file *is* its rows — download `marks/<id>/episodes.csv.gz`.

Each per-mark CSV is the **(state, action, reward)** view, in this dataset's own terms —
the context before the decision, what was done, and the outcome that followed. A mark's
`row_shape` says which of two column layouts its file uses.

**`cross-section` rows** (RDD / DiD — one row per unit):

| Column | Role | Meaning |
|---|---|---|
| `unit_id` | id | Stable identifier for the institution (e.g. school URN). |
| `unit_name` | id | Human-readable name. |
| `running_value` | state | The running variable at decision time. |
| `assigned` | action | `true` if the running value puts the unit on the action side of the cutoff. |
| `treated` | action | Whether the action was actually received (can differ from `assigned` under fuzzy designs); empty if unknown. |
| `outcome` | reward | The later observed outcome; empty when the unit has no linked outcome (e.g. attrition). |
| *(covariates)* | state | One further column per pre-decision covariate (those in the mark's `context.covariate_names`; listed per mark in the manifest). |

For these rows the per-mark constants read from the mark JSON are the threshold
(`design.cutoff`), the treated side (`design.direction`), the textual
`action`/`alternative`, and the full `effect` distribution; `distance_to_cutoff` is just
`running_value − cutoff`.

**`panel` rows** (ITS — one row per series × time bucket):

| Column | Role | Meaning |
|---|---|---|
| `series_id` | id | Which series the row belongs to (treated or a control). |
| `series_name` | id | Human-readable. |
| `is_control` | state | `true` for control-series rows. |
| `period` | state | The time bucket (ISO 8601, e.g. `2018-05`). |
| `periods_since_intervention` | state | `period − intervention_instant` in `running_time.units`; negative = pre. The time analogue of `distance_to_cutoff`. |
| `is_post` | action | `true` if `period` is on/after the intervention instant (and outside any `transition`). |
| `outcome` | reward | The observed outcome value in that period for that series. |
| *(covariates)* | state | One column per `context.covariate_names` (e.g. a population denominator, seasonal index). |

For these rows the per-mark constants are the `intervention_instant`, the
`counterfactual` spec, and the `effect` distribution, joined on the mark `id`.

---

## 3. Submission and score

To be scored, send a `submission.json`: for each mark, your model's predicted
effect **with its own uncertainty**. See
[`examples/submission.example.json`](../examples/submission.example.json).

| Submission field | Meaning |
|---|---|
| `schema_version` | Must match the marks you are scoring against. |
| `model_name` | Identifier for your model. |
| `predictions[]` | One per mark. |
| `predictions[].mark_id` | Which mark this predicts. |
| `predictions[].effect` | Your predicted effect distribution (same shape as a mark's `effect`). |
| `predictions[].value_action` / `value_alternative` | Optional explicit decision values. |

You get back two independent scores per mark:

- **Decision** — did you get the direction of the effect right, and what would a
  wrong call cost? (No penalty where the reference itself is unsure of the sign.)
- **Calibration** — does your stated uncertainty match the truth? Interval
  overlap, a distribution-distance, a calibration curve, and a flag for being
  *confidently wrong* (narrow and wrong while the reference is narrow and known).
