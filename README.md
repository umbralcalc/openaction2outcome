![](assets/logo.png)

# The Open Action->Outcome Datasets

**Real-world reference points for testing models that predict the effects of decisions.**

If your model answers "what would happen if we did X?" (a world model, a model-based policy, a digital twin, an LLM reasoning about consequences) you need to know when it's right. 

Usually you can't: real-world counterfactual ground truth
is rarely available, so people fall back on simulators.

These datasets provide that ground truth.

`openaction2outcome` is a collection of datasets tracking real-world decisions and their measured outcomes.

In each case an institution crossed a published threshold (a school's performance score, a hospital's mortality rating, an area's deprivation rank), which triggered an action, and where the true effect of that action can be recovered — because units that *just* crossed the line are comparable to those that *just* didn't (regression discontinuity).

Each reference point ships with an **honest interval**: a central estimate plus a range that's truthful about what's actually uncertain.

You can check two things against it: does your model get the **effect** right, and is its **confidence** honest?

## What's in it

There are only **two datasets**, normalised on `mark_id`:

- **Marks** — the reference points (the *metadata*). Small JSON files in [`marks/`](marks), in git; one per decision. Each gives the setup, the effect as a distribution, the evidence it passed, and full provenance. See the [data dictionary](docs/schema.md).
- **`episodes` dataset** — the per-unit rows behind every mark, unioned into one row-per-unit **(state, action, reward)** table for model training: each unit's context before the decision (`covariates`, `running_value`/`distance_to_cutoff`), what was done (`assigned`/`treated`), and the `outcome` that followed. A single Parquet (loadable as the Hugging Face `episodes` config; the same bytes in object storage), pointed to by [`datasets/episodes.manifest.json`](datasets/episodes.manifest.json). Each row joins back to its mark's full calibrated effect on `mark_id`.

Plus, for convenience: **dossiers** — a readable write-up of each mark's validity checks, in [`dossiers/`](dossiers); and a **scorer** — a small Go package ([`pkg/score`](pkg/score)) that grades a model's predictions (nothing is hosted; you run it locally).

## The finding it's built to show

A method that reports only its *sampling* error looks confident but is wrong too often — it ignores how much the answer depends on modelling choices. A method that
accounts for that is honestly less certain, and better calibrated. 

The committed [calibration study](scores/calibration-study.json) (`make study`) shows it against
known truth — at a nominal 95% interval, the plug-in method covers the truth only 80% of the time, the model-averaged method 92%.

## Use the data

1. Read a mark from [`marks/`](marks) (and, if you want the raw rows, download the `episodes` dataset from [`datasets/episodes.manifest.json`](datasets/episodes.manifest.json) and filter on `mark_id`, verifying the manifest `sha256`).
2. Write a `submission.json`: your predicted effect, with your own uncertainty, per mark. See [`examples/submission.example.json`](examples/submission.example.json).
3. Score it locally:

```sh
go run ./cmd/openaction2outcome score --submission submission.json --out my.scores.json
```

You get two independent scores per mark — **decision** (did you get the direction right, and what would a wrong call cost?) and **calibration** (does your stated
uncertainty match the truth?). 

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

## Layout

```
cmd/openaction2outcome   CLI: fetch, build, validate, score, study
internal/ingest          load + cache the frozen open-data inputs
internal/rdd             plug-in local-linear estimator (comparison baseline)
internal/sbi             model-averaged estimator (the honest interval)
internal/validity        manipulation / continuity / placebo / robustness checks
internal/dossier         render a mark to a readable dossier
internal/series          per-series minting
internal/publish         publishing config + episode-table writer
internal/hfexport        flatten marks to a Hugging Face dataset (per-series JSONL)
internal/sarexport       reshape episode tables into the row-by-row episodes Parquet
pkg/schema   (public)    Mark + Submission types — standard library only
pkg/score    (public)    the scorer — depends only on pkg/schema
marks  dossiers  scores  examples  docs   the published outputs + reference
data/raw                 pointers (URL + hash + licence) to the frozen inputs
datasets/                slim manifests (URL + hash) for published derived datasets
```

`pkg/schema` and `pkg/score` are kept dependency-light so scoring a model pulls a
tiny dependency tree; the estimation machinery stays in `internal`. How the data
is stored and published is described in [PUBLISHING.md](PUBLISHING.md).

## Coverage

Three series so far:

- **Floor standards** — English school Progress 8 floor of −0.5; outcome is each
  school's Progress 8 two years later.
- **SHMI** — NHS trusts publicly banded "higher than expected" mortality when
  their SHMI crosses the upper control limit; outcome is the trust's SHMI in the
  next 12-month window (a sharp intention-to-treat design, pooled trust-years).
- **Bathing water** — English designated bathing waters classified Poor when
  their E. coli / intestinal enterococci 90th-percentile statistic fails the
  Sufficient standard, mechanically triggering an advice-against-bathing sign +
  EA catchment investigation; outcome is the same site's compliance margin four
  years later (a sharp RDD on the log compliance margin, non-overlapping sample
  windows). Its dossier carries a seam-specific check: re-including the
  discretionarily-discounted "abnormal" samples and confirming the design is
  robust.

Area-based funding is planned.

## Licensing

Code and schema: MIT (see [LICENSE](LICENSE)). The underlying data is UK public
sector information (© Crown copyright) under the
[Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/);
each input records its own licence in `data/raw/<id>/SOURCE.json`.
