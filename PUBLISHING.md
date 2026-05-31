# Publishing artifacts to object storage (Cloudflare R2)

The repo (git) holds the **instrument**: code, slim mark JSON (the metadata),
`data/raw/*/SOURCE.json` pointers, dossiers, the `datasets/*.manifest.json` dataset
pointers, and the example submission/scores. The **bulky bytes** live in **object
storage** (Cloudflare R2, chosen for zero egress fees) and are referenced by URL +
SHA-256, so changing where they are served does not affect integrity.

Only **two datasets** are published, normalised on `mark_id`: the marks (metadata, in
git) and the unified `episodes` dataset (the per-unit rows, in R2). The per-mark
episode tables under `dist/marks` are a build intermediate that feeds the episodes
reshape — they are **not** uploaded.

## What gets published where

| Artifact | Bucket key | Referenced by |
|---|---|---|
| Unified row-by-row `episodes` dataset | `datasets/episodes.parquet` | `datasets/episodes.manifest.json` (`uri` + `sha256`) |
| Frozen raw input (reproducibility; e.g. KS4 CSV, IMD xlsx) | `raw/<source_id>/<file>` (`r2_object_key` in `SOURCE.json`) | `data/raw/<id>/SOURCE.json` |

Public read URL = `publish.json:base_url` + `/` + the bucket key. (The marks are the
second dataset, but they live in git, not object storage.)

## One-time setup

1. Create an R2 bucket (e.g. `openaction2outcome`) and enable public read — either an
   `*.r2.dev` URL or, better, a custom domain.
2. Put the public base URL into [`publish.json`](publish.json):
   ```json
   { "base_url": "https://pub-8d0395b8e53947d791b1e20255172cc3.r2.dev", "bucket": "openaction2outcome" }
   ```
   Once set (and not the `REPLACE-WITH...` placeholder), `fetch` will prefer the R2
   mirror over the original gov.uk URLs — insulating reproducibility from link rot.
3. Configure an S3-compatible client for R2 (account ID + R2 API token). Examples below
   use [`rclone`](https://rclone.org/s3/#cloudflare-r2); the AWS CLI works too.

   `~/.config/rclone/rclone.conf`:
   ```ini
   [r2]
   type = s3
   provider = Cloudflare
   access_key_id = <R2_ACCESS_KEY_ID>
   secret_access_key = <R2_SECRET_ACCESS_KEY>
   endpoint = https://<ACCOUNT_ID>.r2.cloudflarestorage.com
   acl = private
   ```

## Mint → stage → upload

```sh
# 1. fetch inputs into the local cache (verifies SHA-256)
openaction2outcome fetch

# 2. mint every series: writes the slim marks to marks/ and stages each mark's
#    episode rows under dist/marks/ (a build intermediate, not published as-is)
openaction2outcome build --series floor-standards   # ... and shmi, bathing-water

# 3. reshape the staged rows into the one episodes dataset + write its manifest
openaction2outcome export        # -> dist/hf/episodes/episodes.parquet + datasets/episodes.manifest.json

# 4. upload (just these two): the episodes dataset, and the frozen raw inputs
./scripts/publish.sh             # rclone-copies episodes.parquet + data/cache, then verifies hashes
```

Because artifacts are content-addressed, re-uploading an unchanged mint is a no-op
(same bytes, same hash). The episodes manifest records the exact SHA-256, so a
consumer's download is verified on arrival.

## Consumer's view

A model author never needs the mint. They:
1. read the marks from git (the metadata),
2. download the `episodes` dataset (one Parquet, from `datasets/episodes.manifest.json`)
   and filter on `mark_id` for the rows they want to train/validate on,
3. produce a `submission.json` and run `openaction2outcome score --submission ...`.

No account, no server, nothing hosted by us beyond one static object-storage file
(plus the frozen-input mirror for reproducibility).

# Publishing to Hugging Face Datasets

Both datasets are mirrored to Hugging Face Datasets — the discovery channel for the
target audience. Git remains the source of truth; HF carries a flattened, loadable
view of the marks (one record per mark: the effect distribution + design) plus the
row-per-unit `episodes` config. The full nested marks stay in this repo.

## Generate the dataset directory

```sh
make hf            # -> dist/hf/ : README.md (Dataset Card) + one <series>.jsonl per config
```

Each series is a separate config (subset): `floor-standards`, `shmi`.

## Push it (needs a Hugging Face token)

```sh
pip install -U huggingface_hub
huggingface-cli login            # paste a token from https://huggingface.co/settings/tokens
huggingface-cli upload umbralcalc/openaction2outcome dist/hf . --repo-type dataset
```

The Dataset Card (`huggingface/README.md` in this repo) is the canonical HF
documentation; `make hf` copies it into `dist/hf/README.md`. Consumers then:

```python
from datasets import load_dataset
ds = load_dataset("umbralcalc/openaction2outcome", "floor-standards")["test"]
```

These per-series configs are the mark-level view (metadata + effect distribution).
The per-unit rows are not in them — they are in the **`episodes` config**.

`make hf` also writes `dist/hf/episodes/episodes.parquet`: every mark's rows reshaped
into one row-per-unit table, loadable as the `episodes` config
(`load_dataset(..., "episodes")`). The *same bytes* are the object-storage `episodes`
dataset (`datasets/episodes.parquet`) — content-addressed by the SHA-256 recorded in
`datasets/episodes.manifest.json`, and regenerated deterministically by `make hf` from
the staged episode rows and the marks. So Hugging Face carries the same two datasets the
project stores — the marks (the per-series configs) and `episodes` — joined on `mark_id`.
See [specs/4_EPISODES_DATASET_SPEC.md](specs/4_EPISODES_DATASET_SPEC.md).

# The documentation site (GitHub Pages)

The project ships a static website — download buttons, the schema, the per-mark dossiers,
and the docs — served by **GitHub Pages from the committed `docs/` folder**. Like the
marks, it is generated **offline and deterministically** from artifacts already in the
repo, so it can never silently drift from the data.

## Generate

```sh
make site          # -> docs/ : index, downloads, schema, dossiers/, publishing, changelog
```

`openaction2outcome site` reads the marks, dossiers, `docs/schema.md`, `CHANGELOG.md`, 
the calibration study, and `datasets/episodes.manifest.json`, and writes a
self-contained static site into `docs/`:

- **`index.html`** — the landing page; coverage cards are generated from the marks.
- **`downloads.html`** — download buttons for the `episodes` Parquet (URL + SHA-256 from
  the manifest), a generated `downloads/marks.zip` (content-addressed), the Hugging Face
  mirror, and a table of the frozen raw inputs (from each `data/raw/*/SOURCE.json`,
  preferring the object-store mirror URL when `publish.json:base_url` is configured).
- **`schema.html`** and **`changelog.html`** — the repo markdown,
  rendered (single source of truth; intra-repo links are rewritten to the site's pages or
  to GitHub).
- **`dossiers/`** — an index plus one page per mark, rendered from `dossiers/*.md`.

Re-run `make site` after a mint (or after editing the docs) and commit `docs/`. Flags let
you override the repo/Hugging Face URLs (`--repo-url`, `--hf-repo`) and any input/output
path; see `openaction2outcome site -h`.

## Enable Pages (one-time)

In the GitHub repo: **Settings → Pages → Build and deployment → Source: "Deploy from a
branch" → Branch: `main`, folder: `/docs`**. No CI is involved; pushing an updated `docs/`
to `main` republishes. (The generated `docs/.nojekyll` tells Pages to serve the files
as-is rather than running Jekyll over them.)
