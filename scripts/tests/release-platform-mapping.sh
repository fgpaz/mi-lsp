#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"
. "$ROOT/scripts/release/platform-mapping.sh"
HOST_OS="$(uname -s)"
WINDOWS_HOST=0
case "$HOST_OS" in MINGW*|MSYS*|CYGWIN*) WINDOWS_HOST=1 ;; esac

release_mode=0
allow_no_pwsh="${MI_LSP_ALLOW_NO_PWSH:-0}"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --release) release_mode=1; shift ;;
    --allow-no-pwsh) allow_no_pwsh=1; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done
if [ "${MI_LSP_RELEASE:-0}" = "1" ]; then
  release_mode=1
fi

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_mapping() {
  internal_rid="$1"
  expected_asset_rid="$2"
  actual_asset_rid="$(release_asset_rid "$internal_rid")"
  [ "$actual_asset_rid" = "$expected_asset_rid" ] || \
    fail "$internal_rid mapped to $actual_asset_rid, expected $expected_asset_rid"
}

while IFS=' ' read -r internal_rid expected_asset_rid; do
  assert_mapping "$internal_rid" "$expected_asset_rid"
done <<'EOF'
win-arm64 win-arm64
win-x64 win-x64
linux-arm64 linux-arm64
linux-x64 linux-x64
osx-arm64 darwin-arm64
osx-x64 darwin-x64
EOF

if release_asset_rid plan9-x64 >/dev/null 2>&1; then
  fail "unsupported RID unexpectedly mapped"
fi

ARCHIVE_ROOT="$ROOT/.tmp-release-archive-tests-$$"
mkdir -p "$ARCHIVE_ROOT"
cleanup_archives() { rm -rf "$ARCHIVE_ROOT"; }
trap cleanup_archives EXIT INT TERM

make_tar_fixture() {
  output="$1"
  kind="$2"
  fixture="$ARCHIVE_ROOT/input-$kind"
  rm -rf "$fixture"
  mkdir -p "$fixture/payload"
  printf 'ok' >"$fixture/payload/worker"
  case "$kind" in
    valid)
      tar -czf "$output" -C "$fixture" payload/worker
      ;;
    ../escape|/absolute|C:/drive)
      if command -v pax >/dev/null 2>&1; then
        case "$kind" in
          ../escape) replacement='../escape.txt' ;;
          /absolute) replacement='/absolute/escape.txt' ;;
          C:/drive) replacement='C:/escape.txt' ;;
        esac
        (cd "$fixture" && pax -w -f "$output" -s ",^payload/worker$,${replacement}," payload/worker)
      elif tar --version 2>/dev/null | grep -qi 'gnu tar'; then
        case "$kind" in
          ../escape) transform='s#^#../#'; absolute='--absolute-names' ;;
          /absolute) transform='s#^#/absolute/#'; absolute='--absolute-names' ;;
          C:/drive) transform='s#^#C:/#'; absolute='' ;;
        esac
        tar $absolute --transform="$transform" -czf "$output" -C "$fixture" payload/worker
      else
        echo "No portable pax or GNU tar transform support for path fixture '$kind'." >&2
        return 1
      fi
      ;;
    symlink)
      if ! ln -s '../../escape' "$fixture/link" 2>/dev/null; then
        return 2
      fi
      tar -czf "$output" -C "$fixture" link
      ;;
    hardlink)
      ln "$fixture/payload/worker" "$fixture/hard"
      tar -czf "$output" -C "$fixture" payload/worker hard
      ;;
    *)
      return 1
      ;;
  esac
}

make_tar_fixture "$ARCHIVE_ROOT/valid.tar.gz" valid
PATH="$PATH" sh "$ROOT/scripts/install/install.sh" --rid linux-x64 --validate-archive "$ARCHIVE_ROOT/valid.tar.gz" >/dev/null || fail "valid tar archive was rejected"
for kind in '../escape' '/absolute' 'C:/drive' symlink hardlink; do
  archive_name="$(printf '%s' "$kind" | tr '/:' '__').tar.gz"
  if ! make_tar_fixture "$ARCHIVE_ROOT/$archive_name" "$kind"; then
    if [ "$kind" = symlink ] && [ "$WINDOWS_HOST" -eq 1 ]; then
      echo "SKIP: Windows host refused symlink archive fixture without link privilege"
      continue
    fi
    if [ "$kind" = symlink ] && [ "$HOST_OS" = 'Darwin' ]; then
      fail "macOS refused symlink archive fixture creation"
    fi
    fail "could not construct adversarial tar fixture: $kind"
  fi
  if sh "$ROOT/scripts/install/install.sh" --validate-archive "$ARCHIVE_ROOT/$archive_name" >/dev/null 2>&1; then
    fail "unsafe tar archive was accepted: $kind"
  fi
done

manifest_parser=""
for parser_candidate in python3 python; do
  if command -v "$parser_candidate" >/dev/null 2>&1 && "$parser_candidate" -c 'import sys; raise SystemExit(0 if sys.version_info[0] == 3 else 1)' >/dev/null 2>&1; then
    manifest_parser="$(command -v "$parser_candidate")"
    break
  fi
done
[ -n "$manifest_parser" ] || fail "Python 3 is required to exercise JSON manifest validation"
MANIFEST_ROOT="$ARCHIVE_ROOT/worker-manifest"
mkdir -p "$MANIFEST_ROOT"
printf 'worker' >"$MANIFEST_ROOT/worker.bin"
printf 'hidden' >"$MANIFEST_ROOT/.hidden"
"$manifest_parser" - "$MANIFEST_ROOT/worker-manifest.json" "$MANIFEST_ROOT" <<'PY'
import hashlib
import json
import os
import sys
manifest_path, root = sys.argv[1:]
files = []
for name in ['.hidden', 'worker.bin']:
    path = os.path.join(root, name)
    with open(path, 'rb') as handle:
        data = handle.read()
    files.append({'path': name, 'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()})
with open(manifest_path, 'w', encoding='utf-8') as handle:
    json.dump({'schema': 'mi-lsp-worker-manifest/v1', 'rid': 'linux-x64', 'protocol': 'mi-lsp-v1.1', 'file_count': len(files), 'files': files}, handle)
PY
sh "$ROOT/scripts/install/install.sh" --rid linux-x64 --validate-worker-manifest "$MANIFEST_ROOT/worker-manifest.json" --worker-root "$MANIFEST_ROOT" >/dev/null || fail "valid JSON worker manifest was rejected"
for field in schema rid protocol file_count hash; do
  invalid="$MANIFEST_ROOT/invalid-$field.json"
  "$manifest_parser" - "$MANIFEST_ROOT/worker-manifest.json" "$invalid" "$field" <<'PY'
import json
import sys
source, target, field = sys.argv[1:]
with open(source, encoding='utf-8') as handle:
    document = json.load(handle)
if field == 'schema': document['schema'] = 'wrong'
elif field == 'rid': document['rid'] = 'win-x64'
elif field == 'protocol': document['protocol'] = 'wrong'
elif field == 'file_count': document['file_count'] += 1
else: document['files'][0]['sha256'] = '0' * 64
with open(target, 'w', encoding='utf-8') as handle:
    json.dump(document, handle)
PY
  if sh "$ROOT/scripts/install/install.sh" --rid linux-x64 --validate-worker-manifest "$invalid" --worker-root "$MANIFEST_ROOT" >/dev/null 2>&1; then
    fail "invalid worker manifest was accepted: $field"
  fi
done

echo "PASS: install.sh validates worker manifest JSON metadata, file_count, sizes, hashes, and hidden files"

ps_host="$(command -v pwsh 2>/dev/null || true)"
if [ -z "$ps_host" ]; then
  if [ "$release_mode" -eq 1 ] || [ "$allow_no_pwsh" != "1" ]; then
    fail "PowerShell is required for release-platform-mapping archive verification; set MI_LSP_ALLOW_NO_PWSH=1 only for an explicitly reduced local/test run"
  fi
  echo "SKIP: PowerShell unavailable; explicit local/test waiver was supplied"
else
  "$ps_host" -NoProfile -File "$ROOT/scripts/tests/archive-safety.ps1"
  "$ps_host" -NoProfile -File "$ROOT/scripts/tests/release-directory-sha256.ps1"
fi

echo "PASS: six release RIDs map to public GoReleaser assets; OSX maps to Darwin; archive guards reject adversarial members"
