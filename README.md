![](docs/assets/logo.png)

# Open Action->Outcome Datasets

**Real-world reference points for testing models that predict the effects of decisions.**

If your model answers "what would happen if we did X?" (a world model, a model-based policy, a digital twin, an LLM reasoning about consequences) you need to know when it's right. 

Usually you can't: real-world counterfactual ground truth
is rarely available, so people fall back on simulators.

`openaction2outcome` is a collection of datasets that provide that ground truth; tracking real-world decisions and their measured outcomes.

📚 **Documentation site:** https://umbralcalc.github.io/openaction2outcome/ — download buttons, the schema, the per-mark dossiers, and the docs (generated offline by `make site` into [`docs/`](docs)).

In each case an institution faced a real decision — crossing a published threshold (a school's performance score, a hospital's mortality rating, an area's deprivation rank), or a policy switching on at a known date — that triggered an action whose true effect can be recovered by a **transparent quasi-experimental design**: regression discontinuity and its kink variant, difference-in-differences, or a controlled interrupted time series (see [Designs](#designs)).

Each reference point ships with an **honest interval**: a central estimate plus a range that's truthful about what's actually uncertain.

You can check two things against it: does your model get the **effect** right, and is its **confidence** honest?

## What's in it

There are only **two datasets**, normalised on `mark_id`:

- **Marks** — the reference points (the *metadata*). Small JSON files in [`marks/`](marks), in git; one per decision. Each gives the setup, the effect as a distribution, the evidence it passed, and full provenance. See the [data dictionary](docs/schema.md).
- **`episodes` dataset** — the per-unit rows behind every mark: each unit's context before the decision (`running_value` + the mark's covariates), what was done (`assigned`/`treated`), and the `outcome` that followed — the **(state, action, reward)** view for model training. Published **per mark** as one gzipped CSV each in object storage, listed (with download URL + SHA-256 + size) in [`datasets/episodes.manifest.json`](datasets/episodes.manifest.json). A mark's file *is* its rows; join them to the mark JSON on its `id` for the design and the full calibrated effect. (The same per-mark files are mirrored on Hugging Face — same schema — so `load_dataset(repo, data_files="episodes/<id>.csv.gz")` loads one mark.)

Plus, for convenience: **dossiers** — a readable write-up of each mark's validity checks, in [`dossiers/`](dossiers); and a **scorer** — a small Go package ([`pkg/score`](pkg/score)) that grades a model's predictions (nothing is hosted; you run it locally).

## Designs

A mark is **identification-agnostic** everywhere that matters — the effect
distribution, the uncertainty budget, the provenance rules, and the scorer are all
independent of *how* comparability was established. Only the `design` block and the
validity `dossier` change shape per design family, selected by the mark's
`identification` field:

| `identification` | Design | How comparability is established | Estimand |
|---|---|---|---|
| `rdd-sharp` | Sharp regression discontinuity | Units just either side of a published cutoff are comparable; crossing it deterministically triggers the action. | Local effect at the cutoff |
| `rdd-fuzzy` | Fuzzy regression discontinuity | As above, but crossing the cutoff shifts the *probability* of the action (a real first stage). | Local LATE at the cutoff |
| `rdd-kink` | Regression-kink design (RKD) | The policy is a continuous function of the running variable whose **slope** changes at a kink; the effect is the kink in the outcome's slope over the known slope change. | Marginal effect of policy intensity |
| `did` | Difference-in-differences | A treated group's pre→post change is compared to a control group's, under parallel trends. The anchor unit for dose / staggered-rollout mechanisms. | Average treatment effect on the treated |
| `its-controlled` | Controlled interrupted time series (ITS) | A treated series' break at a sharp intervention *instant* is compared to a control series sharing its pre-intervention trend. | **Population** effect over the post-intervention window |

The first three are discontinuity designs (a cutoff in a running variable); DiD and
ITS are panel designs (a treatment/control split in groups or in time). Decision
scores are comparable **within** a design family, never pooled across — an RDD
local-at-cutoff estimand and an ITS population-over-window estimand answer different
questions. Calibration scores remain comparable everywhere (they only ask whether
stated uncertainty matches realised truth).

The legacy `rdd_type` field (`sharp` / `fuzzy` / `kink` / `did`) still reads —
marks minted before `identification` migrate automatically.

## Two kinds of mark: pins and bridges

Every mark carries a `category`, and the two are **never pooled**:

- **`identified`** — the design-based marks above, recovered by one of the four
  quasi-experimental [designs](#designs) (RDD, RKD, DiD, ITS). Real counterfactual
  truth from a transparent design. These are the **pins**. `truth_source` =
  `identified`.
- **`bridge`** — a calibrated estimate of a mechanism's effect at a point on its
  effect-curve where *no clean cutoff exists*, but which is **bracketed by real
  identified anchors on both sides**. A stochadex simulator carries the mechanism;
  a Gaussian-process discrepancy term, pinned to the anchors, spans between them.
  The honest interval is the posterior: narrow at the pins, wider between them,
  always **bounded** because the query is bracketed (interpolation only — no
  extrapolation, by design). These are the **span**. `truth_source` =
  `simulator-bridged`.

**The pin/span discipline is absolute.** A bridge mark's provenance makes the
boundary unmissable: its dossier ships the bracketing anchors, the load-bearing
covariance kernel (the trust-decay assumption), the anchor-coherence justification,
and a headline **leave-one-anchor-out (LOAO) coverage** number. The simulator is
never the source of truth — the anchors are. A consumer can always filter the
collection to `category == identified` and never see a simulated quantity laundered
as ground truth.

Bridge marks change *what we harvest*: not one isolated experiment per policy, but
a **family of identified anchors on one mechanism** that a simulator can bridge —
reopening confounder-heavy domains (epidemiology, environmental regulation,
economic/clinical thresholds) that single-RDD scope excluded. The bridge machinery
(`internal/bridge`) is validated against synthetic mechanisms with a *known* effect
curve (`make study` → `study --bridge`); the first real bridge mark lands once a
mechanism in the collection has ≥2 bracketing anchors.

## The finding it's built to show

A method that reports only its *sampling* error looks confident but is wrong too often — it ignores how much the answer depends on modelling choices. A method that
accounts for that is honestly less certain, and better calibrated. 

The committed [calibration study](scores/calibration-study.json) (`make study`) shows it against
known truth — at a nominal 95% interval, the plug-in method covers the truth only 80% of the time, the model-averaged method 92%.

## Use the data

1. Read a mark from [`marks/`](marks) (and, if you want the raw rows, download that mark's `episodes.csv.gz` — its URL + `sha256` are in [`datasets/episodes.manifest.json`](datasets/episodes.manifest.json) — and verify the hash).
2. Write a `submission.json`: your predicted effect, with your own uncertainty, per mark. See [`examples/submission.example.json`](examples/submission.example.json).
3. Score it locally:

```sh
go run ./cmd/openaction2outcome score --submission submission.json --out my.scores.json
```

You get two independent scores per mark — **decision** (did you get the direction right, and what would a wrong call cost?) and **calibration** (does your stated
uncertainty match the truth?). Results are **broken out per category and never pooled** — so strong coverage on the identified pins can't mask weak coverage on the bridges. Use `--category identified` for a maximally-defensible test, `--category bridge` for the simulator-bridged estimates, or `both` (default).

The committed
[example submission](examples/submission.example.json) and its [expected scores](scores/example.scores.json) let you confirm your setup.

## Reproduce the marks

Everything is minted offline and deterministically — same inputs, same bytes out.

```sh
make fetch      # download the frozen open-data inputs (verifies hashes)
make build      # mint the floor-standards mark + dossier
make validate   # check every mark against the schema
make study      # re-run the calibration study
```

The bridge machinery has its own known-truth study — recovery of a synthetic
mechanism's effect curve between anchors, plus leave-one-anchor-out coverage:

```sh
go run ./cmd/openaction2outcome study --bridge --out scores/bridge-study.json
```

The discrepancy GP is conditioned in **closed form**, not sampled. `--compare`
shows why: it runs three calibrators on identical problems — the modular cut
(shipped default), the *exact* closed-form joint (GP marginal likelihood for θ +
analytic δ conditioning), and a stochadex-*sampled* joint. The first two nearly
coincide and track nominal coverage; the sampled joint degenerates (the data-free
query-innovation latent collapses under SMC resampling) and badly under-covers —
the empirical case for the closed form.

```sh
go run ./cmd/openaction2outcome study --bridge --compare --out scores/bridge-compare.json
```

## Layout

```
cmd/openaction2outcome   CLI: fetch, build, validate, score, study, export, site
internal/ingest          load + cache the frozen open-data inputs
internal/rdd             local-linear RDD + regression-kink (RKD) estimators (sharp/fuzzy/kink)
internal/did             difference-in-differences estimator (treated-vs-control, parallel-trends)
internal/sbi             model-averaged estimator (the honest interval)
internal/bridge          simulator + GP-discrepancy bridge calibration (bridge marks)
internal/validity        manipulation / continuity / placebo / robustness + bridge checks
internal/dossier         render a mark to a readable dossier
internal/series          per-series minting
internal/publish         publishing config + per-mark episode-table writer
internal/episodes        per-mark episodes manifest + mirror the CSVs into the HF dir
internal/hfexport        flatten marks to a Hugging Face dataset (per-series JSONL)
internal/site            generate the static GitHub Pages site (docs/)
pkg/schema   (public)    Mark + Submission types — standard library only
pkg/score    (public)    the scorer — depends only on pkg/schema
marks  dossiers  scores  examples  docs   the published outputs + reference
data/raw                 pointers (URL + hash + licence) to the frozen inputs
datasets/                slim manifests (URL + hash) for published derived datasets
```

`pkg/schema` and `pkg/score` are kept dependency-light so scoring a model pulls a
tiny dependency tree; the estimation machinery stays in `internal`. How the data
is stored and published is described in [PUBLISHING.md](PUBLISHING.md).

## Licensing

Code and schema: MIT (see [LICENSE](LICENSE)). The underlying data is open but
licensed per source: UK public sector information (© Crown copyright) under the
[Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/),
and European Environment Agency air-quality data plus Open-Meteo (ERA5/Copernicus)
weather data under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). Each
input records its own licence and required attribution in `data/raw/<id>/SOURCE.json`.
