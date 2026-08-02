#!/usr/bin/env sh
set -eu

REPO="${MI_LSP_REPO:-fgpaz/mi-lsp}"
RID="${MI_LSP_RID:-}"
INSTALL_DIR="${MI_LSP_INSTALL_DIR:-$HOME/.local/bin}"
DRY_RUN=0
SKIP_WORKER_INSTALL=0
VALIDATE_ARCHIVE=""
VALIDATE_MANIFEST=""
VALIDATE_WORKER_ROOT=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --rid) RID="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --skip-worker-install) SKIP_WORKER_INSTALL=1; shift ;;
    --validate-archive) VALIDATE_ARCHIVE="$2"; shift 2 ;;
    --validate-worker-manifest) VALIDATE_MANIFEST="$2"; shift 2 ;;
    --worker-root) VALIDATE_WORKER_ROOT="$2"; shift 2 ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

detect_rid() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux) os_part="linux" ;;
    Darwin) os_part="darwin" ;;
    *) echo "Unsupported OS '$os'. Supported: Linux, macOS. Use install.ps1 on Windows." >&2; exit 1 ;;
  esac
  case "$arch" in
    x86_64|amd64) arch_part="x64" ;;
    aarch64|arm64) arch_part="arm64" ;;
    *) echo "Unsupported architecture '$arch'." >&2; exit 1 ;;
  esac
  echo "${os_part}-${arch_part}"
}

if [ -z "$RID" ] && [ "${MI_LSP_INSTALL_TEST_MODE:-}" = activation ]; then
  RID="${MI_LSP_TEST_RID:-}"
fi
if [ -z "$RID" ]; then
  RID="$(detect_rid)"
fi

case "$RID" in
  linux-x64|linux-arm64|darwin-x64|darwin-arm64|osx-x64|osx-arm64) ;;
  *) echo "Unsupported RID '$RID' for install.sh. Supported values: linux-x64, linux-arm64, darwin-x64, darwin-arm64, osx-x64, osx-arm64." >&2; exit 1 ;;
esac

archive_rid="$RID"
worker_rid="$RID"
case "$RID" in
  darwin-x64) worker_rid="osx-x64" ;;
  darwin-arm64) worker_rid="osx-arm64" ;;
  osx-x64) archive_rid="darwin-x64" ;;
  osx-arm64) archive_rid="darwin-arm64" ;;
esac


require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command missing: $1" >&2
    exit 1
  fi
}

require_cmd tar

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  echo "Required command missing: sha256sum or shasum" >&2
  exit 1
}

retry() {
  attempts=0
  while :; do
    if "$@"; then
      return 0
    fi
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 5 ]; then
      return 1
    fi
    sleep 1
  done
}

canonical_existing_path() {
  path="$1"
  [ -e "$path" ] || [ -L "$path" ] || return 1
  dir="$(dirname "$path")"
  base="$(basename "$path")"
  canonical_dir="$(CDPATH= cd -P -- "$dir" 2>/dev/null && pwd -P)" || return 1
  if [ -d "$canonical_dir/$base" ]; then
    (CDPATH= cd -P -- "$canonical_dir/$base" 2>/dev/null && pwd -P)
  else
    printf '%s/%s\n' "$canonical_dir" "$base"
  fi
}

assert_lexically_under() {
  parent="$1"
  child="$2"
  case "$parent" in
    /*) ;;
    *) parent="$(CDPATH= cd -L -- "$parent" 2>/dev/null && pwd -L)" || { echo "Could not resolve lexical confinement parent: $1" >&2; return 1; } ;;
  esac
  case "$child" in
    "$parent"|"$parent"/*) ;;
    *) echo "Path escaped temporary root: $child" >&2; return 1 ;;
  esac
}

file_identity() {
  path="$1"
  stat_cmd="$(command -v stat 2>/dev/null || true)"
  [ -n "$stat_cmd" ] || { echo "Required command missing: stat; refusing hardlink validation." >&2; return 1; }
  identity="$("$stat_cmd" -c '%d:%i:%h' "$path" 2>/dev/null || true)"
  case "$identity" in
    *[!0-9:]*|'') identity="$("$stat_cmd" -f '%d:%i:%l' "$path" 2>/dev/null || true)" ;;
  esac
  case "$identity" in
    *[!0-9:]*|'') echo "Could not obtain portable file identity for '$path'; refusing hardlink validation." >&2; return 1 ;;
  esac
  printf '%s\n' "$identity"
}

validate_tar_archive() (
  archive="$1"
  [ -f "$archive" ] || { echo "Archive not found: $archive" >&2; return 1; }
  listing="$(tar -tzf "$archive")" || { echo "Could not list archive: $archive" >&2; return 1; }
  printf '%s\n' "$listing" | awk '
    (length($0) == 0 || $0 == "." || $0 == ".." || index($0, "\\") || $0 ~ /^\// || $0 ~ /^[A-Za-z]:/ || $0 ~ /(^|\/)\.\.(\/|$)/ || $0 ~ /[\r\n]/) { bad = 1 }
    END { exit bad }
  ' || { echo "Archive contains an unsafe path member: $archive" >&2; return 1; }
  verbose="$(tar -tvzf "$archive")" || { echo "Could not inspect archive metadata: $archive" >&2; return 1; }
  printf '%s\n' "$verbose" | awk '
    (substr($0, 1, 1) != "-" && substr($0, 1, 1) != "d") { bad = 1 }
    END { exit bad }
  ' || { echo "Archive contains a symlink, hardlink, or special member: $archive" >&2; return 1; }
)

assert_confined_tree() {
  lexical_root="$1"
  [ -d "$lexical_root" ] || { echo "Extraction root is not a directory: $lexical_root" >&2; return 1; }
  [ ! -L "$lexical_root" ] || { echo "Extraction root is a symlink or reparse point: $lexical_root" >&2; return 1; }
  physical_root="$(canonical_existing_path "$lexical_root")" || { echo "Could not resolve extraction root: $lexical_root" >&2; return 1; }
  [ -d "$physical_root" ] || { echo "Extraction root is not a directory: $lexical_root" >&2; return 1; }
  if find "$physical_root" -type l -print | grep -q .; then
    echo "Extraction produced a symlink inside confined root: $physical_root" >&2
    return 1
  fi

  identity_file="$(mktemp "${TMPDIR:-/tmp}/mi-lsp-identities.XXXXXX")" || {
    echo "mktemp is required for portable hardlink validation; refusing extraction." >&2
    return 1
  }
  : >"$identity_file"
  paths="$(find "$physical_root" -print)"
  while IFS= read -r path; do
    [ -n "$path" ] || continue
    case "$path" in
      "$physical_root"|"$physical_root"/*) ;;
      *) echo "Extraction escaped confined root: $path" >&2; rm -f "$identity_file"; return 1 ;;
    esac
    if [ -f "$path" ]; then
      identity="$(file_identity "$path")" || { rm -f "$identity_file"; return 1; }
      link_count="${identity##*:}"
      case "$link_count" in
        ''|*[!0-9]*) echo "Invalid link count for '$path'; refusing extraction." >&2; rm -f "$identity_file"; return 1 ;;
      esac
      if [ "$link_count" -gt 1 ] || grep -F -x "${identity%:*}" "$identity_file" >/dev/null 2>&1; then
        echo "Extraction produced a hardlinked file: $path" >&2
        rm -f "$identity_file"
        return 1
      fi
      printf '%s\n' "${identity%:*}" >>"$identity_file"
    fi
  done <<EOF
$paths
EOF
  rm -f "$identity_file"
}

validate_worker_manifest() {
  manifest="$1"
  worker_root="$2"
  worker_rid="$3"
  manifest_parser=""
  for parser_candidate in python3 python; do
    if command -v "$parser_candidate" >/dev/null 2>&1 && "$parser_candidate" -c 'import sys; raise SystemExit(0 if sys.version_info[0] == 3 else 1)' >/dev/null 2>&1; then
      manifest_parser="$(command -v "$parser_candidate")"
      break
    fi
  done
  [ -n "$manifest_parser" ] || { echo "Python 3 is required to parse worker manifests; refusing installation." >&2; return 1; }
  [ -f "$manifest" ] || { echo "Worker manifest not found: $manifest" >&2; return 1; }
  [ -d "$worker_root" ] || { echo "Worker root is not a directory: $worker_root" >&2; return 1; }
  assert_confined_tree "$worker_root" || return 1
  "$manifest_parser" - "$manifest" "$worker_root" "$worker_rid" <<'PY'
import hashlib
import json
import os
import re
import sys

manifest_path, worker_root, expected_rid = sys.argv[1:]
def fail(message):
    raise SystemExit(message)

try:
    with open(manifest_path, 'r', encoding='utf-8') as handle:
        document = json.load(handle)
except (OSError, UnicodeError, json.JSONDecodeError) as exc:
    fail('Worker manifest is not valid UTF-8 JSON: %s' % exc)
if not isinstance(document, dict):
    fail('Worker manifest root must be a JSON object.')
if document.get('schema') != 'mi-lsp-worker-manifest/v1':
    fail('Worker manifest schema is invalid.')
if document.get('rid') != expected_rid:
    fail('Worker manifest RID does not match the selected worker RID.')
if document.get('protocol') != 'mi-lsp-v1.1':
    fail('Worker manifest protocol is invalid.')
entries = document.get('files')
file_count = document.get('file_count')
if not isinstance(file_count, int) or isinstance(file_count, bool) or file_count < 1:
    fail('Worker manifest file_count must be a positive integer.')
if not isinstance(entries, list) or file_count != len(entries):
    fail('Worker manifest file_count does not match files.')

root = os.path.abspath(worker_root)
expected = {}
for entry in entries:
    if not isinstance(entry, dict):
        fail('Worker manifest contains a non-object file entry.')
    path = entry.get('path')
    size = entry.get('size')
    digest = entry.get('sha256')
    if not isinstance(path, str) or not path or '\\' in path or path.startswith('/') or re.match(r'^[A-Za-z]:', path):
        fail('Worker manifest contains an unsafe file path.')
    parts = path.split('/')
    if any(part in ('', '.', '..') for part in parts) or path == 'worker-manifest.json':
        fail('Worker manifest contains an unsafe file path.')
    candidate = os.path.abspath(os.path.join(root, *parts))
    if os.path.commonpath((root, candidate)) != root:
        fail('Worker manifest file escaped its worker root.')
    if not isinstance(size, int) or isinstance(size, bool) or size < 0:
        fail('Worker manifest file size is invalid.')
    if not isinstance(digest, str) or not re.fullmatch(r'[0-9a-fA-F]{64}', digest):
        fail('Worker manifest file hash is invalid.')
    if path in expected:
        fail('Worker manifest contains duplicate file paths.')
    if not os.path.isfile(candidate) or os.path.islink(candidate):
        fail('Worker manifest references a missing or linked file: %s' % path)
    actual_size = os.path.getsize(candidate)
    if actual_size != size:
        fail('Worker manifest file size mismatch: %s' % path)
    digest_actual = hashlib.sha256()
    with open(candidate, 'rb') as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b''):
            digest_actual.update(chunk)
    if digest_actual.hexdigest().lower() != digest.lower():
        fail('Worker manifest file hash mismatch: %s' % path)
    expected[path] = candidate

actual = {}
for dirpath, dirnames, filenames in os.walk(root, topdown=True, followlinks=False):
    for dirname in list(dirnames):
        full = os.path.join(dirpath, dirname)
        if os.path.islink(full):
            fail('Worker root contains a linked directory.')
    for filename in filenames:
        full = os.path.join(dirpath, filename)
        if os.path.islink(full):
            fail('Worker root contains a linked file.')
        relative = os.path.relpath(full, root).replace(os.sep, '/')
        if relative != 'worker-manifest.json':
            actual[relative] = full
if set(actual) != set(expected):
    fail('Worker manifest file list does not match the extracted worker bundle.')
PY
}

download() {
  url="$1"
  out="$2"
  name="$(basename "$out")"
  dir="$(dirname "$out")"
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    if curl -fL -H 'User-Agent: mi-lsp-installer' -H "Authorization: Bearer $GITHUB_TOKEN" "$url" -o "$out"; then
      return 0
    fi
  else
    if curl -fL -H 'User-Agent: mi-lsp-installer' "$url" -o "$out"; then
      return 0
    fi
  fi
  if command -v gh >/dev/null 2>&1; then
    gh release download "$tag" --repo "$REPO" --pattern "$name" --dir "$dir" --clobber
    return $?
  fi
  return 1
}

activate_confined() {
  test_root="$1"; test_cli="$2"; test_worker="$3"; test_rid="$4"
  mkdir -p "$test_root/workers"; test_target="$test_root/mi-lsp"; test_worker_target="$test_root/workers/$test_rid"
  case "$test_worker_target" in "$test_root/workers"/*) ;; *) return 1 ;; esac
  test_backup="$test_root/.mi-lsp-backup.$$"; mkdir "$test_backup"; test_old_cli=0; test_old_worker=0; test_committed=0
  test_rollback() { status=$?; if [ "$test_committed" -eq 0 ]; then [ ! -e "$test_target" ] || rm -f "$test_target"; [ ! -e "$test_worker_target" ] || find "$test_worker_target" -depth -type f -delete; [ "$test_old_cli" -eq 1 ] && mv "$test_backup/mi-lsp" "$test_target"; [ "$test_old_worker" -eq 1 ] && mv "$test_backup/worker" "$test_worker_target"; fi; find "$test_backup" -depth -type f -delete; find "$test_backup" -depth -type d -empty -delete; return "$status"; }
  trap test_rollback EXIT INT TERM
  [ -e "$test_target" ] && { mv "$test_target" "$test_backup/mi-lsp"; test_old_cli=1; }; [ -e "$test_worker_target" ] && { mv "$test_worker_target" "$test_backup/worker"; test_old_worker=1; }
  cp "$test_cli" "$test_target"; chmod +x "$test_target"; [ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = cli-activation ] && return 1
  cp -R "$test_worker" "$test_worker_target"; [ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = worker-activation ] && return 1; [ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = status ] && return 1
  test_committed=1; trap - EXIT INT TERM; find "$test_backup" -depth -type f -delete; find "$test_backup" -depth -type d -empty -delete
}
if [ "${MI_LSP_INSTALL_TEST_MODE:-}" = activation ]; then
  : "${MI_LSP_TEST_INSTALL_ROOT:?required}"; : "${MI_LSP_TEST_SOURCE_CLI:?required}"; : "${MI_LSP_TEST_SOURCE_WORKER:?required}"; : "${MI_LSP_TEST_RID:?required}"
  activate_confined "$MI_LSP_TEST_INSTALL_ROOT" "$MI_LSP_TEST_SOURCE_CLI" "$MI_LSP_TEST_SOURCE_WORKER" "$MI_LSP_TEST_RID"; echo 'PASS: activation test mode'; exit 0
fi

if [ -n "$VALIDATE_ARCHIVE" ]; then
  validate_tar_archive "$VALIDATE_ARCHIVE"
  echo "PASS: tar archive members are confined and link-free"
  exit 0
fi

if [ -n "$VALIDATE_MANIFEST" ]; then
  [ -n "$VALIDATE_WORKER_ROOT" ] || { echo "--worker-root is required with --validate-worker-manifest." >&2; exit 2; }
  validate_worker_manifest "$VALIDATE_MANIFEST" "$VALIDATE_WORKER_ROOT" "$worker_rid"
  echo "PASS: worker manifest JSON, schema, RID, protocol, file_count, sizes, and hashes are valid"
  exit 0
fi

if [ "$DRY_RUN" -eq 1 ]; then
  version="latest"
  archive="mi-lsp_${version}_${archive_rid}.tar.gz"
  checksums="mi-lsp_${version}_checksums.txt"
  printf "repo=%s
version=%s
rid=%s
archive_rid=%s
worker_rid=%s
archive=%s
checksums=%s
install_dir=%s
" "$REPO" "$version" "$RID" "$archive_rid" "$worker_rid" "$archive" "$checksums" "$INSTALL_DIR"
  exit 0
fi

require_cmd curl

api="https://api.github.com/repos/$REPO/releases/latest"
if [ -n "${GITHUB_TOKEN:-}" ]; then
  release_json="$(curl -fsSL -H 'User-Agent: mi-lsp-installer' -H "Authorization: Bearer $GITHUB_TOKEN" "$api")"
else
  release_json="$(curl -fsSL -H 'User-Agent: mi-lsp-installer' "$api")"
fi
tag="$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
if [ -z "$tag" ]; then
  echo "Could not resolve latest release for $REPO." >&2
  exit 1
fi
version="${tag#v}"
archive="mi-lsp_${version}_${archive_rid}.tar.gz"
checksums="mi-lsp_${version}_checksums.txt"
base_url="https://github.com/$REPO/releases/download/$tag"

if [ "$DRY_RUN" -eq 1 ]; then
  printf 'repo=%s\nversion=%s\nrid=%s\narchive_rid=%s\nworker_rid=%s\narchive=%s\nchecksums=%s\ninstall_dir=%s\n' \
    "$REPO" "$tag" "$RID" "$archive_rid" "$worker_rid" "$archive" "$checksums" "$INSTALL_DIR"
  exit 0
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/mi-lsp-install.XXXXXX")" || {
  echo "Could not create an atomic temporary extraction root." >&2
  exit 1
}
[ ! -L "$tmp" ] || { echo "Temporary extraction root is a symlink or reparse point: $tmp" >&2; exit 1; }
tmp_physical="$(canonical_existing_path "$tmp")" || { echo "Could not resolve temporary extraction root: $tmp" >&2; exit 1; }
assert_lexically_under "$tmp" "$tmp/extract"
cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

download "$base_url/$archive" "$tmp/$archive"
download "$base_url/$checksums" "$tmp/$checksums"

expected="$(grep " $archive\$" "$tmp/$checksums" | awk '{print $1}' | head -n 1 || true)"
if [ -z "$expected" ]; then
  expected="$(grep "$archive" "$tmp/$checksums" | awk '{print $1}' | head -n 1 || true)"
fi
if [ -z "$expected" ]; then
  echo "Checksum for $archive was not found in $checksums." >&2
  exit 1
fi
actual="$(sha256_file "$tmp/$archive")"
if [ "$actual" != "$expected" ]; then
  echo "Checksum mismatch for $archive. Expected $expected, got $actual." >&2
  exit 1
fi

mkdir -p "$tmp/extract"
assert_lexically_under "$tmp" "$tmp/extract"
[ ! -L "$tmp/extract" ] || { echo "Extraction root is a symlink or reparse point: $tmp/extract" >&2; exit 1; }
extract_physical="$(canonical_existing_path "$tmp/extract")" || { echo "Could not resolve extraction root: $tmp/extract" >&2; exit 1; }
case "$extract_physical" in
  "$tmp_physical"|"$tmp_physical"/*) ;;
  *) echo "Extraction root escaped temporary root: $tmp/extract" >&2; exit 1 ;;
esac
validate_tar_archive "$tmp/$archive"
tar -xzf "$tmp/$archive" -C "$tmp/extract"
assert_confined_tree "$tmp/extract"
source_cli="$(find "$tmp/extract" -type f -name mi-lsp -print | head -n 1)"
source_worker="$(find "$tmp/extract" -type d -path "*/workers/$worker_rid" -print | head -n 1)"
if [ -z "$source_cli" ]; then
  echo "Extracted archive did not contain mi-lsp." >&2
  exit 1
fi
if [ -z "$source_worker" ]; then
  echo "Extracted archive did not contain workers/$worker_rid." >&2
  exit 1
fi
worker_manifest="$source_worker/worker-manifest.json"
validate_worker_manifest "$worker_manifest" "$source_worker" "$worker_rid"

mkdir -p "$INSTALL_DIR/workers"
workers_root="$(cd "$INSTALL_DIR/workers" && pwd -P)"
target="$INSTALL_DIR/mi-lsp"
target_worker="$workers_root/$worker_rid"
case "$target_worker" in "$workers_root"/*) ;; *) echo "Refusing worker path" >&2; exit 1 ;; esac
install_root="$(CDPATH= cd -P -- "$INSTALL_DIR" && pwd -P)"
backup_root="$install_root/.mi-lsp-backup.$$"
mkdir "$backup_root"
old_cli=0; old_worker=0; activated=1
safe_remove() { p="$1"; case "$p" in "$install_root"/*) ;; *) return 1 ;; esac; if [ -d "$p" ] && [ ! -L "$p" ]; then find "$p" -depth -type f -delete; find "$p" -depth -type d -delete; else rm -f "$p"; fi; }
rollback() { status=$?; [ "$activated" -eq 1 ] && { [ ! -e "$target" ] || safe_remove "$target"; [ ! -e "$target_worker" ] || safe_remove "$target_worker"; [ "$old_cli" -eq 1 ] && mv "$backup_root/mi-lsp" "$target"; [ "$old_worker" -eq 1 ] && mv "$backup_root/worker" "$target_worker"; }; rmdir "$backup_root" 2>/dev/null || true; return "$status"; }
trap rollback EXIT INT TERM
if [ -x "$target" ]; then "$target" daemon stop --format compact >/dev/null 2>&1 || true; fi
if [ -e "$target" ]; then mv "$target" "$backup_root/mi-lsp"; old_cli=1; fi
if [ -e "$target_worker" ]; then mv "$target_worker" "$backup_root/worker"; old_worker=1; fi
cp "$source_cli" "$target"; chmod +x "$target"
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = cli-activation ] && exit 1
cp -R "$source_worker" "$target_worker"
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = worker-activation ] && exit 1
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = worker-install ] && exit 1
"$target" daemon stop --format compact >/dev/null 2>&1 || true

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add mi-lsp to PATH with:"; echo "  export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

if [ "$SKIP_WORKER_INSTALL" -eq 0 ]; then "$target" worker install --rid "$worker_rid" --format compact; fi
"$target" version --format toon
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = status ] && exit 1
"$target" worker status --format compact
safe_remove "$backup_root/mi-lsp" 2>/dev/null || true
safe_remove "$backup_root/worker" 2>/dev/null || true
rmdir "$backup_root"
activated=0
trap - EXIT INT TERM
echo "mi-lsp $tag installed at $target"
