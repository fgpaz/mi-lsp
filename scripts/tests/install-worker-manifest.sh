#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
TMPDIR=${TMPDIR:-/tmp}; export TMPDIR
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir "$tmp/worker"
printf 'worker-bytes\n' >"$tmp/worker/app"
size=$(wc -c <"$tmp/worker/app" | tr -d ' ')
if command -v sha256sum >/dev/null 2>&1; then hash=$(sha256sum "$tmp/worker/app" | awk '{print $1}'); else hash=$(shasum -a 256 "$tmp/worker/app" | awk '{print $1}'); fi
printf '{"schema":"mi-lsp-worker-manifest/v1","rid":"linux-x64","protocol":"mi-lsp-v1.1","file_count":1,"files":[{"path":"app","size":%s,"sha256":"%s"}]}' "$size" "$hash" >"$tmp/manifest.json"
node "$ROOT/scripts/install/validate-worker-manifest.js" "$tmp/manifest.json" "$tmp/worker" linux-x64
printf 'PASS: valid worker manifest accepted\n'
printf '0' >>"$tmp/manifest.json"
if node "$ROOT/scripts/install/validate-worker-manifest.js" "$tmp/manifest.json" "$tmp/worker" linux-x64 >/dev/null 2>&1; then
  echo 'FAIL: malformed manifest accepted' >&2
  exit 1
fi
printf 'PASS: malformed worker manifest rejected\n'
