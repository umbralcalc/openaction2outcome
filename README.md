# openaction2outcome

**An open causal yardstick mapping institutional decisions to verified outcomes.**

A small, bulletproof, fully open set of real UK public-sector decisions — each
triggered when an institution's measured performance crosses a published
threshold — where the true effect of the decision is recovered by regression
discontinuity (RDD) and shipped as an *honest interval* (central estimate +
identification uncertainty + a validity dossier). Its purpose is to test whether
any model that makes counterfactual claims gets the causal effect right at points
where ground truth is genuinely known. See [BRIEF.md](BRIEF.md) for the full
rationale and [DEV_PLAN.md](DEV_PLAN.md) for the build plan.

This is an **offline dataset you mint locally and publish as static files** — no
server, no database, no scheduled jobs. You run a Go pipeline by hand; it
produces the marks *and* the scores. A consumer scores their own model with the
same public Go package, locally. Recurring cost: £0.

**Storage model.** Git holds the *instrument* — code, slim mark JSON (~20 KB),
source pointers, dossiers, example scores. The *bulky bytes* — the frozen
open-data inputs and the per-mark analysis-ready episode tables model authors
train on — live in object storage (Cloudflare R2, zero egress) and are referenced
from marks and manifests by URL + SHA-256. Inputs are **not** vendored into git:
`data/raw/<id>/SOURCE.json` is a pointer (canonical URL + mirror key + hash), and
`openaction2outcome fetch` downloads the bytes into a gitignored cache, verifying
the hash. See [PUBLISHING.md](PUBLISHING.md).

## Status — Phase 0 (schema + first real mark)

| Piece | State |
|---|---|
| Public module skeleton (`pkg/schema`, `pkg/score`, `cmd`, `internal/*`) | ✅ |
| `Mark` + `Submission` schema with point-in-time / leakage guards | ✅ |
| Dependency-light scoring (Track A + Track B) — imports only `pkg/schema` + stdlib | ✅ |
| First genuine, outcome-bearing mark on **real open data** | ✅ floor standards |
| stochadex / SBI estimation, second + third seams | ⏳ Phase 1+ |

The first mark is the **education floor-standards** seam: a sharp RDD on the 2016
Progress 8 floor of **−0.5**, with the outcome being each school's Progress 8 two
years later (2017/18), linked by URN. (The area-funding seam's running variable —
IMD 2019 — is frozen under `data/raw/imd2019`, but its sharp form, UKSPF 2025-26,
is too recent to have a realized downstream outcome yet; see the project notes.)

## Repository layout

```
cmd/openaction2outcome   CLI: fetch, build, validate, score
internal/ingest          input fetch-to-cache + loaders (KS4 performance tables)
internal/rdd             local-linear RDD estimator (honest interval)
internal/validity        density / covariate-continuity / placebo / donut battery
internal/publish         publish config + deterministic episode-table writer
internal/seam            per-seam mint orchestration
pkg/schema   (PUBLIC)    Mark + Submission types — stdlib only
pkg/score    (PUBLIC)    Track A + Track B scoring — imports only pkg/schema
data/raw                 SOURCE.json pointers (URL + mirror key + SHA-256 + licence)
data/cache               fetched input bytes (gitignored; `fetch` populates)
dist                     staged episode sidecars for upload (gitignored)
marks                    published slim mark files (JSON)
scores                   published / example scores
examples                 example submission
publish.json             object-store base URL (Cloudflare R2)
```

`pkg/schema` and `pkg/score` are deliberately dependency-light so a consumer who
only wants to score a model pulls a tiny dependency tree; all heavy numerics stay
in `internal`.

## Reproduce the marks and scores

```sh
go run ./cmd/openaction2outcome fetch              # download frozen inputs into data/cache (verifies SHA-256)
go test ./...                                       # unit + real-data integration tests (skips if not fetched)
go run ./cmd/openaction2outcome build --seam floor-standards   # mints the slim mark + stages the episode sidecar
go run ./cmd/openaction2outcome validate            # checks every mark against the schema
```

Minting is **deterministic**: re-running `build` reproduces the mark *and* its
episode sidecar byte-for-byte (seeds, tool versions, and input SHA-256s are
recorded in each mark's provenance, and the cached inputs are integrity-checked at
mint time). `go test` is offline — the integration test skips cleanly until you
have run `fetch`.

## Score your own model

1. (Optional) Download a mark's analysis-ready episode table from its `data.uri`
   (a single gzipped CSV, content-addressed by `data.sha256`) to train/validate
   your model on. You never need to run the mint or re-derive anything.
2. Produce a `submission.json` to the published schema (`pkg/schema`,
   `Submission`): for each mark, your predicted effect *with your own
   uncertainty*. See [examples/submission.example.json](examples/submission.example.json).
3. Run it locally — nothing is hosted:

```sh
go run ./cmd/openaction2outcome score --submission submission.json --out my.scores.json
```

The committed [examples/submission.example.json](examples/submission.example.json)
and its [expected scores](scores/example.scores.json) let you confirm you are
using the tool correctly.

### How a model is scored (two independent tracks)

- **Track A — decision-value consistency:** does the model get the *sign* of the
  effect right, and what is its decision regret? No penalty where the mark itself
  is unsure of the sign.
- **Track B — calibration against truth:** does the model's interval overlap the
  mark's honest interval; a CRPS-style distribution distance; a PIT calibration
  curve; and a *confidently-wrong* detector (flagged only when a narrow model is
  wrong **and** the mark is itself narrow-and-known).

## What one mark carries

Running variable + published cutoff; the action and its counterfactual; the later
open outcome; the effect as a **distribution** (central estimate + honest interval
whose width folds in bandwidth/specification uncertainty, not just sampling SE); a
**validity dossier** (density/manipulation, covariate continuity, placebo cutoffs,
bandwidth sweep, donut robustness); and full **provenance** (sources, licences,
SHA-256s, and `context_asof ≤ decision < outcome` point-in-time timestamps). A
mark is admitted iff it passes the validity tests — never rejected for a wide
interval.

## Licensing

Schema, scoring, and evaluator code: MIT (see [LICENSE](LICENSE)). Source data
licences are passed through with attribution — the DfE and MHCLG inputs used here
are © Crown copyright, licensed under the
[Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/).
Each frozen input records its own licence in `data/raw/<id>/SOURCE.json`.
