# Data dictionary

This is the field-by-field reference for everything you download or submit. The
Go types it describes live in [`pkg/schema`](../pkg/schema); the version string is
`schema_version` (currently `0.3.0`).

There are three things to know about:

1. **A mark** — one causally-validated reference point (a JSON file in [`marks/`](../marks)).
2. **An episode table** — the per-unit data behind a mark (a gzipped CSV in object storage).
3. **A submission** — what you send to be scored, and the score you get back.

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
| `rdd_type` | string | `sharp` (crossing the cutoff always triggers the action) or `fuzzy` (it changes the probability). |
| `design` | object | What is being estimated — see below. |
| `context` | object | Pre-decision information a model is allowed to use. |
| `data` | object | Pointer to the episode table — see Episode table below. |
| `sample` | array | A small inline excerpt of episode rows nearest the cutoff, for quick inspection. |
| `effect` | object | The mark itself: the effect distribution — see below. |
| `dossier` | object | The validity checks the mark passed — see below. |
| `provenance` | object | Sources, licences, timestamps, and reproducibility metadata. |

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

## 2. Episode table

The analysis-ready data behind a mark: one row per unit. It is **not** in git —
it is a gzipped CSV in object storage, referenced from the mark's `data` field by
URL and SHA-256 so you can verify your download. You only need it if you want to
train or refit on the raw rows; scoring a model does not require it.

| `data` field | Meaning |
|---|---|
| `uri` | Download URL. |
| `sha256` | Content hash — verify after download. |
| `format` | e.g. `csv.gz`. |
| `rows` | Number of units. |
| `columns` | Column names (also below). |

Columns:

| Column | Meaning |
|---|---|
| `unit_id` | Stable identifier for the institution (e.g. school URN). |
| `unit_name` | Human-readable name. |
| `running_value` | The running variable at decision time. |
| `assigned` | `true` if the running value puts the unit on the action side of the cutoff. |
| `treated` | Whether the action was actually received (can differ from `assigned` under fuzzy designs). |
| `outcome` | The later outcome (blank if not linked). |
| *(remaining columns)* | Pre-decision covariates listed in the mark's `context.covariate_names`. |

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
