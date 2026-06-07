#!/usr/bin/env bash
#
# Upload staged artifacts to object storage (Cloudflare R2) and verify they are
# served correctly. Run after `build` has staged the episode sidecar(s) under
# dist/ and `fetch` has populated data/cache.
#
#   ./scripts/publish.sh                # upload, then verify
#   ./scripts/publish.sh --verify-only  # skip upload, just verify what's live
#
# Config comes from publish.json (bucket, base_url). The rclone remote name
# defaults to "r2"; override with RCLONE_REMOTE. Credentials live only in your
# rclone config — see PUBLISHING.md.
set -euo pipefail
cd "$(dirname "$0")/.."

REMOTE="${RCLONE_REMOTE:-r2}"
VERIFY_ONLY=0
[ "${1:-}" = "--verify-only" ] && VERIFY_ONLY=1

read_cfg() { python3 -c "import json,sys;print(json.load(open('publish.json'))['$1'])"; }
BUCKET="$(read_cfg bucket)"
BASE="$(read_cfg base_url)"
BASE="${BASE%/}"

case "$BASE" in
  *REPLACE-WITH*|"") echo "error: publish.json base_url is not set" >&2; exit 1 ;;
esac

if [ "$VERIFY_ONLY" -eq 0 ]; then
  if ! rclone listremotes | grep -qx "${REMOTE}:"; then
    echo "error: rclone remote '${REMOTE}:' not configured (see PUBLISHING.md)" >&2
    exit 1
  fi
  # The episodes dataset is published per mark: one gzipped CSV each, under
  # marks/<id>/episodes.csv.gz (the exact object key is recorded in the manifest).
  # The marks metadata itself lives in git, not R2.
  while IFS=$'\t' read -r id key; do
    [ -z "$id" ] && continue
    src="dist/marks/${id}/episodes.csv.gz"
    if [ -f "$src" ]; then
      echo ">> uploading ${id} episodes -> ${REMOTE}:${key}"
      rclone copyto "$src" "${REMOTE}:${key}" --progress
    else
      echo "warning: ${src} not found (run \`make build SERIES=${id%%-*}...\` first)" >&2
    fi
  done < <(python3 -c "import json
d=json.load(open('datasets/episodes.manifest.json')); base='$BASE'; bucket='$BUCKET'
for m in d.get('marks',[]):
    print(m['mark_id']+chr(9)+bucket+'/'+m['uri'][len(base)+1:])")
  echo ">> mirroring frozen inputs     -> ${REMOTE}:${BUCKET}/raw"
  rclone copy data/cache "${REMOTE}:${BUCKET}/raw" --progress
fi

echo ">> verifying published artifacts resolve and match recorded hashes"
# python (stdlib) only parses local JSON; all HTTP goes through curl so we use the
# system trust store and avoid macOS python TLS quirks.
fail=0
sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }

# Episodes dataset (one gzipped CSV per mark): full hash check of each file.
while IFS=$'\t' read -r uri want; do
  [ -z "$uri" ] && continue
  tmp="$(mktemp)"
  if curl -fsSL "$uri" -o "$tmp" 2>/dev/null; then
    got="$(sha256 "$tmp")"
    if [ "$got" = "$want" ]; then
      echo "  ok   $uri  ($(wc -c <"$tmp" | tr -d ' ') bytes)"
    else
      echo "  FAIL $uri  (sha256 got $got want $want)"; fail=$((fail+1))
    fi
  else
    echo "  FAIL $uri  (not reachable)"; fail=$((fail+1))
  fi
  rm -f "$tmp"
done < <(python3 -c "import glob,json
for p in sorted(glob.glob('datasets/*.manifest.json')):
    d=json.load(open(p))
    for m in d.get('marks',[]):
        if m.get('uri') and m.get('sha256'): print(m['uri']+chr(9)+m['sha256'])")

# Frozen-input mirror: HEAD check (files can be large).
while IFS= read -r url; do
  [ -z "$url" ] && continue
  code="$(curl -sS -I -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || echo 000)"
  if [ "$code" = "200" ]; then
    echo "  ok   $url"
  else
    echo "  FAIL $url  (HTTP $code)"; fail=$((fail+1))
  fi
done < <(python3 -c "import glob,json
base='$BASE'
for p in sorted(glob.glob('data/raw/*/SOURCE.json')):
    k=json.load(open(p)).get('r2_object_key')
    if k: print(base+'/'+k)")

if [ "$fail" -gt 0 ]; then
  echo; echo "$fail artifact(s) failed verification" >&2
  exit 1
fi
echo; echo "all published artifacts verified"
