![](assets/logo.png)

# The OpenAction2Outcome Datasets

**Real-world reference points for testing models that predict the effects of decisions.**

If your model answers "what would happen if we did X?" (a world model, a model-based policy, a digital twin, an LLM reasoning about consequences) you need to know when it's right. 

Usually you can't: real-world counterfactual ground truth
is rarely available, so people fall back on simulators.

These datasets provide that ground truth.

OpenAction2Outcome is a collection of datasets tracking real-world decisions and their measured outcomes.

In each case an institution crossed a published threshold (a school's performance score, a hospital's mortality rating, an area's deprivation rank), which triggered an action, and where the true effect of that action can be recovered — because units that *just* crossed the line are comparable to those that *just* didn't (regression discontinuity).

Each reference point ships with an **honest interval**: a central estimate plus a range that's truthful about what's actually uncertain.

You can check two things against it: does your model get the **effect** right, and is its **confidence** honest?

## What's in it

- **Marks** - the reference points. Small JSON files in [`marks/`](marks); one per decision. Each gives the setup, the effect as a distribution, the evidence it passed, and full provenance. See the [data dictionary](docs/schema.md).
- **Dossiers** — a readable write-up of each mark's validity checks, in [`dossiers/`](dossiers).
- **Episode tables** — the per-unit data behind each mark, in object storage (referenced from the mark by URL + hash). Only needed if you want the raw rows.
- **Scorer** — a small Go package ([`pkg/score`](pkg/score)) that grades a model's predictions. Nothing is hosted; you run it locally.

## The finding it's built to show

A method that reports only its *sampling* error looks confident but is wrong too often — it ignores how much the answer depends on modelling choices. A method that
accounts for that is honestly less certain, and better calibrated. 

The committed [calibration study](scores/calibration-study.json) (`make study`) shows it against
known truth — at a nominal 95% interval, the plug-in method covers the truth only 80% of the time, the model-averaged method 92%.

## Use the data

1. Read a mark from [`marks/`](marks) (and, if you want the raw rows, download its episode table from the mark's `data.uri`, verifying `data.sha256`).
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
pkg/schema   (public)    Mark + Submission types — standard library only
pkg/score    (public)    the scorer — depends only on pkg/schema
marks  dossiers  scores  examples  docs   the published outputs + reference
data/raw                 pointers (URL + hash + licence) to the frozen inputs
```

`pkg/schema` and `pkg/score` are kept dependency-light so scoring a model pulls a
tiny dependency tree; the estimation machinery stays in `internal`. How the data
is stored and published is described in [PUBLISHING.md](PUBLISHING.md).

## Coverage

Currently one series — **floor standards** (English school Progress 8 floor of
−0.5; outcome is each school's Progress 8 two years later). Two more are planned:
NHS mortality banding (a fuzzy threshold) and area-based funding.

## Licensing

Code and schema: MIT (see [LICENSE](LICENSE)). The underlying data is UK public
sector information (© Crown copyright) under the
[Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/);
each input records its own licence in `data/raw/<id>/SOURCE.json`.
