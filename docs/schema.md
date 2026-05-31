# Data dictionary

This is the field-by-field reference for everything you download or submit. The
Go types it describes live in [`pkg/schema`](../pkg/schema); the version string is
`schema_version` (currently `0.4.0`).

There are three things to know about:

1. **A mark** — one causally-validated reference point (a JSON file in [`marks/`](../marks)). This is the *metadata*.
2. **The `episodes` dataset** — the per-unit rows behind every mark, in object storage as one gzipped CSV per mark, joined to the marks on the mark `id`.
3. **A submission** — what you send to be scored, and the score you get back.

There are only ever those two datasets — the marks (metadata, in git) and the
episodes dataset (rows, in object storage) — normalised on `mark_id`.

---

## 1. Mark

One mark is one real decision whose true effect is known: an institution crossed a
published threshold, that triggered an action, and a later outcome is observable.
The effect is given as a distribution (a central estimate plus an interval whose
width is honest about how much is genuinely uncertain), not a single number.

| Field | Type | Meaning |
|---|---|---|
| `schema_version` | string | Schema version this mark was written against. |
| `id` | string | Stable unique identifier, e.g. `floor-standards-p8-2016`. |
| `series` | string | Which group of marks this belongs to: `floor-standards`, `shmi`, or `area-funding`. |
| `domain` | string | Human label, e.g. `Education`. |
| `unit_type` | string | The kind of institution, e.g. `school`, `nhs-trust`, `local-authority`. |
| `category` | string | `identified` (design-based truth — a pin) or `bridge` (simulator-bridged interpolation between anchors — a span). Empty reads as `identified`. The two are never pooled in scoring. |
| `truth_source` | string | `identified` for design-based marks, `simulator-bridged` for bridge marks. The hard provenance line. |
| `rdd_type` | string | `sharp` (crossing the cutoff always triggers the action) or `fuzzy` (it changes the probability). Identified marks only. |
| `design` | object | What is being estimated — see below. Identified marks only. |
| `bridge` | object | Bridge-specific fields (anchors, query point, simulator, kernel, coherence) — see below. Bridge marks only. |
| `context` | object | Pre-decision information a model is allowed to use. |
| `sample` | array | A small inline excerpt of episode rows nearest the cutoff, for quick inspection (the full rows are in the `episodes` dataset, keyed by this mark's `id`). |
| `effect` | object | The mark itself: the effect distribution — see below. |
| `dossier` | object | The validity checks the mark passed — see below. |
| `provenance` | object | Sources, licences, timestamps, and reproducibility metadata. |

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

| Field | Type | Meaning |
|---|---|---|
| `running_variable` | object | The measured quantity the decision depends on (`name`, `description`, `units`, `source_id`). |
| `cutoff` | number | The published threshold. |
| `direction` | string | `above-treated` or `below-treated` — which side of the cutoff receives the action. |
| `action` | string | What happens when the cutoff is crossed. |
| `alternative` | string | The counterfactual: what happens otherwise. |
| `outcome` | object | The later observable quantity (`name`, `description`, `units`, `source_id`). |
| `estimand` | string | Plain-language statement of exactly what the effect measures. |

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
*uncertain* (a wide interval is information, not a failure).

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
(The same per-mark files are also mirrored into the Hugging Face dataset at
`episodes/<id>.csv.gz` — same schema — so an HF user can load one mark's rows with
`load_dataset(repo, data_files="episodes/<id>.csv.gz")`.)

Each per-mark CSV is the **(state, action, reward)** view, in this dataset's own terms —
the unit's context before the decision, what was done, and the outcome that followed:

| Column | Role | Meaning |
|---|---|---|
| `unit_id` | id | Stable identifier for the institution (e.g. school URN). |
| `unit_name` | id | Human-readable name. |
| `running_value` | state | The running variable at decision time. |
| `assigned` | action | `true` if the running value puts the unit on the action side of the cutoff. |
| `treated` | action | Whether the action was actually received (can differ from `assigned` under fuzzy designs); empty if unknown. |
| `outcome` | reward | The later observed outcome; empty when the unit has no linked outcome (e.g. attrition). |
| *(covariates)* | state | One further column per pre-decision covariate (those in the mark's `context.covariate_names`; listed per mark in the manifest). |

These are the per-unit, per-mark columns. Everything else is **constant for the mark** and
is read from the mark JSON, joined on the mark `id`: the threshold (`design.cutoff`), the
treated side (`design.direction`), the textual `action`/`alternative`, and the full
`effect` distribution. `distance_to_cutoff` is just `running_value − cutoff`.

> Hugging Face carries the **same per-mark CSVs** (mirrored at `episodes/<id>.csv.gz`),
> not a separate unioned table — one row shape everywhere. The mark-level metadata + effect
> distribution are the per-series configs (`floor-standards`, `shmi`, `bathing-water`).

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
