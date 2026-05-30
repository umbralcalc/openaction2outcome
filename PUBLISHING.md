# Publishing artifacts to object storage (Cloudflare R2)

The repo (git) holds only the **instrument**: code, slim mark JSON, `data/raw/*/SOURCE.json`
pointers, dossiers, and the example submission/scores. The **bulky bytes** — the
frozen open-data inputs and the per-mark analysis-ready episode tables — live in
**object storage** (Cloudflare R2, chosen for zero egress fees) and are referenced
from marks and manifests by URL + SHA-256. Changing where they are served does not
affect integrity, because every reference is content-addressed.

## What gets published where

| Artifact | Bucket key | Referenced by |
|---|---|---|
| Frozen raw input (e.g. KS4 CSV, IMD xlsx) | `raw/<source_id>/<file>` (`r2_object_key` in `SOURCE.json`) | `data/raw/<id>/SOURCE.json` |
| Per-mark episode table | `marks/<mark_id>/episodes.csv.gz` | the mark's `data.uri` |

Public read URL = `publish.json:base_url` + `/` + the bucket key.

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

# 2. mint: writes the slim mark to marks/ and stages the episode table under dist/
openaction2outcome build --seam floor-standards

# 3. upload the staged per-mark artifacts
rclone copy dist/marks r2:openaction2outcome/marks --progress

# 4. (one-time per input) mirror the frozen raw inputs, preserving r2_object_key layout
#    e.g. data/cache/ks4-2015-2016-final/england_ks4final_2015-2016.csv
#         -> raw/ks4-2015-2016-final/england_ks4final_2015-2016.csv
rclone copy data/cache r2:openaction2outcome/raw --progress
```

Because artifacts are content-addressed, re-uploading an unchanged mint is a no-op
(same bytes, same hash). Marks reference exact SHA-256s, so a consumer's download is
verified on arrival.

## Consumer's view

A model author never needs the mint. They:
1. read a slim mark from git (or the dataset mirror),
2. download its `data.uri` episode table (one gzipped CSV) to train/validate on,
3. produce a `submission.json` and run `openaction2outcome score --submission ...`.

No account, no server, nothing hosted by us beyond static object storage.
