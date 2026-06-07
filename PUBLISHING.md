# Publishing artifacts to object storage (Cloudflare R2)

The repo (git) holds the **instrument**: code, slim mark JSON (the metadata),
`data/raw/*/SOURCE.json` pointers, dossiers, the `datasets/*.manifest.json` dataset
pointers, and the example submission/scores. The **bulky bytes** live in **object
storage** (Cloudflare R2, chosen for zero egress fees) and are referenced by URL +
SHA-256, so changing where they are served does not affect integrity.

Only **two datasets** are published, normalised on the mark `id`: the marks (metadata, in
git) and the `episodes` dataset (the per-unit rows, in R2). The episode rows are published
**per mark** — one gzipped CSV each — so `dist/marks/<id>/episodes.csv.gz` is what gets
uploaded (not a build intermediate any more); there is no unioned re-encoding anywhere.

## What gets published where

| Artifact | Bucket key | Referenced by |
|---|---|---|
| Per-mark `episodes` rows (one gzipped CSV each) | `marks/<id>/episodes.csv.gz` | `datasets/episodes.manifest.json` (`marks[]`: `uri` + `sha256` + `bytes` + `rows`) |
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
   no_check_bucket = true
   ```
   `no_check_bucket = true` is required for Cloudflare R2: without it, rclone calls
   `CreateBucket` before writing each *new* object, which a bucket-scoped R2 API token
   rejects with `403 AccessDenied` (existing objects still upload, so the failure only
   shows up the first time you publish a newly-minted mark). Add it to an existing remote
   with `rclone config update r2 no_check_bucket true`.

## Mint → stage → upload

```sh
# 1. fetch inputs into the local cache (verifies SHA-256)
openaction2outcome fetch

# 2. mint every series: writes the slim marks to marks/ and each mark's episode
#    rows to dist/marks/<id>/episodes.csv.gz (this file is what gets uploaded)
openaction2outcome build --series floor-standards   # ... and shmi, bathing-water

# 3. write the per-mark episodes manifest (URL + hash + size for each CSV)
openaction2outcome manifest      # -> datasets/episodes.manifest.json

# 4. upload: each mark's episodes.csv.gz, and the frozen raw inputs
./scripts/publish.sh             # rclone-copies dist/marks/*/episodes.csv.gz + data/cache, then verifies hashes
```

Because artifacts are content-addressed, re-uploading an unchanged mint is a no-op
(same bytes, same hash). The manifest records each per-mark file's exact SHA-256, so a
consumer's download is verified on arrival.

## Consumer's view

A model author never needs the mint. They:
1. read the marks from git (the metadata),
2. download a mark's `episodes.csv.gz` (its `uri`/`sha256` are in
   `datasets/episodes.manifest.json` under `marks[]`) for the rows they want to
   train/validate on, and verify the hash,
3. produce a `submission.json` and run `openaction2outcome score --submission ...`.

No account, no server, nothing hosted by us beyond a handful of static object-storage
files (plus the frozen-input mirror for reproducibility).

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
- **`downloads.html`** — a per-mark `episodes.csv.gz` download (URL + SHA-256 + size from
  the manifest), a generated `downloads/marks.zip` (content-addressed), and a table of the
  frozen raw inputs (from each `data/raw/*/SOURCE.json`, preferring the object-store mirror
  URL when `publish.json:base_url` is configured).
- **`schema.html`** and **`changelog.html`** — the repo markdown,
  rendered (single source of truth; intra-repo links are rewritten to the site's pages or
  to GitHub).
- **`dossiers/`** — an index plus one page per mark, rendered from `dossiers/*.md`.

Re-run `make site` after a mint (or after editing the docs) and commit `docs/`. Flags let
you override the repo URL (`--repo-url`) and any input/output path; see
`openaction2outcome site -h`.

## Enable Pages (one-time)

In the GitHub repo: **Settings → Pages → Build and deployment → Source: "Deploy from a
branch" → Branch: `main`, folder: `/docs`**. No CI is involved; pushing an updated `docs/`
to `main` republishes. (The generated `docs/.nojekyll` tells Pages to serve the files
as-is rather than running Jekyll over them.)
