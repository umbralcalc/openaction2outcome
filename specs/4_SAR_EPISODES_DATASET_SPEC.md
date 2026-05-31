# openaction2outcome — Spec 4
## A row-by-row (state, action, reward) `episodes` dataset

The collection today exposes two grains: slim **marks** (git — one per decision, carrying
the full effect distribution) and per-mark **episode tables**
(`dist/marks/<id>/episodes.csv.gz` → R2 — one row per unit). The episode tables are *already*
row-by-row (state = covariates + running value, action = assigned/treated, reward = outcome),
but they are fragmented per-mark, carry series-specific columns, and are neither packaged for
(state, action, reward) learning nor loadable from Hugging Face.

This spec defines one **unified, row-by-row dataset primed for SAR-style learning**. The key
realisation: it is a **pure reshape of data already in R2 plus the tiny mark metadata** — no
re-estimation, no change to the marks. The "mark effect" is a property of the *mark*, not the
row, so it is **never denormalised onto every row**; each row instead carries a `mark_id` join
key back to where the full effect already lives (the marks and the existing mark-level HF
configs), plus a few cheap scalar effect summaries for convenience.

**Locked decisions:** Go-native Parquet (`parquet-go`); a single unified `episodes` config
with covariates exposed as a `state` map; reward = observed outcome.

---

## 1. Outcome

- A new `episodes` Parquet artifact, content-addressed in R2 and loadable as a new Hugging
  Face config: `load_dataset("umbralcalc/openaction2outcome", "episodes")`.
- Marks unchanged; existing per-series mark-level configs unchanged.
- Stays byte-deterministic and content-addressed, consistent with the rest of the mint.

This is a deliberate exception to the current rule that episode tables are not duplicated to
Hugging Face: the SAR dataset *is* the episode rows, reshaped, and the whole point is that it
be loadable row-by-row.

---

## 2. Grain and schema

One row per unit×decision-period, all series unioned.

| field | source | role |
|---|---|---|
| `mark_id`, `series` | mark | **join key** → full effect in marks / per-series configs |
| `unit_id`, `unit_name` | episode row | id |
| `running_value`, `cutoff`, `distance_to_cutoff` (= running − cutoff, signed), `direction` | episode + `mark.Design` | **state** (structural) |
| `state` (`MAP<string,double>`) | episode covariate columns | **state** (covariates) |
| `assigned` (bool), `treated` (bool, nullable) | episode row | **action** |
| `action`, `alternative` (text) | `mark.Design` | action labels |
| `reward` (double, nullable), `reward_observed` (bool) | episode `outcome` | **reward** = observed outcome |
| `effect_central / _lower / _upper / _interval_level / _std_dev` | `mark.Effect` | inlined scalar effect summary (full posterior stays in the mark) |

The `state` map is built generically. An episode table's header is always the core set
`{unit_id, unit_name, running_value, assigned, treated, outcome}` plus series-specific
covariate columns (see `internal/series/floorstandards.go:49`); the covariates are simply the
header minus that core set. This means **zero per-series code** — it works for floor-standards,
shmi, and any future series.

`reward_observed` is `false` when a unit has no linked later outcome (e.g. floor-standards
attrition where a sponsored academy is re-issued a new URN); the `reward` is then null.
Learners filter on it.

### 2.1 Determinism caveat (must handle)

A Go `map[string]float64` iterates in random order, which would make the Parquet bytes
non-deterministic and break content-addressing. The `state` column must be built from a
**key-sorted** structure. If `parquet-go`'s MAP encoding preserves (random) Go-map order, fall
back to a `LIST<STRUCT{name, value}>` sorted by name (still a clean "state" column). Also pin
`WriterConfig.CreatedBy` to a fixed string and use a single row group. A byte-determinism test
is the acceptance gate (§4).

---

## 3. Implementation

**New `go.mod` dependency:** `github.com/parquet-go/parquet-go`, used only under `internal/` —
never in the dependency-light `pkg/schema` / `pkg/score` import graph.

**New package `internal/sarexport`:**
- `LoadEpisodeTable(mark, distDir) (header []string, rows [][]string, err)` — reads the staged
  `dist/marks/<id>/episodes.csv.gz`, **verifies SHA-256 against `mark.Data.SHA256`**, gunzips
  and parses. Offline-first: requires `build` to have staged the table; clear error + hint if
  absent. Reuse the hashing pattern from `internal/publish/publish.go:113`.
- `SARRow` struct with `parquet:"..."` tags per §2; `BuildSAR(marks, distDir) []SARRow`
  mapping every episode row, sorted by `(series, mark_id, unit_id)`.
- `WriteParquet(path, rows) (publish.WrittenArtifact, error)` — single row group, fixed schema,
  pinned `CreatedBy`; hashes the file for the manifest.
- `Manifest` struct + writer for the slim git-tracked pointer (below).

**`internal/publish/publish.go`:** add `DatasetsPrefix` to `Config` (default `datasets`) plus
`DatasetObjectKey` / `DatasetArtifactURL` helpers, mirroring the existing
`MarkObjectKey` / `MarkArtifactURL` (publish.go:49-56).

**`cmd/openaction2outcome/main.go`:** extend `cmdExport` (so `make hf` covers it) to, after the
mark-level JSONL:
1. `sarexport.BuildSAR(marks, distDir)`;
2. write Parquet to `dist/hf/episodes/episodes.parquet` (the HF config path);
3. write the slim manifest `datasets/episodes.manifest.json` (committed to git) recording the
   R2 URI (`cfg.DatasetArtifactURL`), SHA-256, row count, column list, series list, and the
   join/reward semantics — the same "git holds the pointer, R2 holds the bytes" pattern as marks.

**`huggingface/README.md`** (Dataset Card YAML at README.md:14-22): add the config —
```yaml
- config_name: episodes
  data_files:
  - split: train
    path: episodes/episodes.parquet
```
plus a prose section: row-by-row SAR; `state` is a map; reward = observed outcome
(`reward_observed=false` when no linked outcome); the full calibrated effect is one `mark_id`
join away in the per-series configs / marks.

**`scripts/publish.sh`:** upload `dist/hf/episodes/episodes.parquet` →
`r2:${BUCKET}/datasets/episodes.parquet`, and add it to the hash-verify loop (read URI + SHA
from `datasets/episodes.manifest.json`; full-hash check like the per-mark tables).

**`Makefile`:** `make hf` already runs `export`; no new target required (optionally add a `sar`
alias). Update the `hf` help text.

**Docs:** add a row to the PUBLISHING.md artifact table
(`datasets/episodes.parquet` ← `datasets/episodes.manifest.json`); note the R2 + HF
duplication rationale; add to the README layout block.

---

## 4. Verification

`internal/sarexport` unit tests:
- stage a tiny synthetic episode table via `publish.WriteEpisodesCSVGz`, build a minimal
  `schema.Mark` pointing at it; assert `BuildSAR` maps `state` = covariates, `reward` =
  outcome, `reward_observed`, signed `distance_to_cutoff`, and the inlined effect scalars;
- **byte-determinism:** `WriteParquet` twice → identical SHA-256;
- round-trip: read the Parquet back with `parquet-go` → same rows;
- behavioural: every row's `mark_id` ∈ loaded marks; `state` keys ⊆ `mark.Context.CovariateNames`.

End-to-end:
```sh
make fetch && make build-all && make hf
```
then
```python
from datasets import load_dataset
ds = load_dataset("<local dist/hf>", "episodes")["train"]   # row-by-row SAR loads
```
confirm `state` / `reward` / `mark_id` are present and joinable to the `floor-standards` config
on `id == mark_id`. `make test` green; `make verify` (after a real upload) hashes the published
Parquet against the manifest.

---

## 5. Out of scope

- No change to marks, estimation, or the validity battery.
- No effect-as-per-row-reward variant — the full posterior stays at mark grain and reward =
  observed outcome. It can be added later behind the same builder without reshaping the schema.
