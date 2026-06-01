#!/usr/bin/env sh
set -eu

REPO="${MI_LSP_REPO:-fgpaz/mi-lsp}"
RID="${MI_LSP_RID:-}"
INSTALL_DIR="${MI_LSP_INSTALL_DIR:-$HOME/.local/bin}"
DRY_RUN=0
SKIP_WORKER_INSTALL=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --rid) RID="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --skip-worker-install) SKIP_WORKER_INSTALL=1; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

detect_rid() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux) ;;
    Darwin) echo "macOS assets are not published yet. Supported Linux RIDs: linux-x64, linux-arm64." >&2; exit 1 ;;
    *) echo "Unsupported OS '$os'. Supported OS: Linux. Use install.ps1 on Windows." >&2; exit 1 ;;
  esac
  case "$arch" in
    x86_64|amd64) echo "linux-x64" ;;
    aarch64|arm64) echo "linux-arm64" ;;
    *) echo "Unsupported Linux architecture '$arch'." >&2; exit 1 ;;
  esac
}

if [ -z "$RID" ]; then
  RID="$(detect_rid)"
fi

case "$RID" in
  linux-x64|linux-arm64) ;;
  *) echo "Unsupported RID '$RID' for install.sh. Supported values: linux-x64, linux-arm64." >&2; exit 1 ;;
esac

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command missing: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd tar
require_cmd sha256sum

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
archive="mi-lsp_${version}_${RID}.tar.gz"
checksums="mi-lsp_${version}_checksums.txt"
base_url="https://github.com/$REPO/releases/download/$tag"

if [ "$DRY_RUN" -eq 1 ]; then
  printf 'repo=%s\nversion=%s\nrid=%s\narchive=%s\nchecksums=%s\ninstall_dir=%s\n' \
    "$REPO" "$tag" "$RID" "$archive" "$checksums" "$INSTALL_DIR"
  exit 0
fi

tmp="${TMPDIR:-/tmp}/mi-lsp-install-$$"
rm -rf "$tmp"
mkdir -p "$tmp"
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
actual="$(sha256sum "$tmp/$archive" | awk '{print $1}')"
if [ "$actual" != "$expected" ]; then
  echo "Checksum mismatch for $archive. Expected $expected, got $actual." >&2
  exit 1
fi

mkdir -p "$tmp/extract"
tar -xzf "$tmp/$archive" -C "$tmp/extract"
source_cli="$(find "$tmp/extract" -type f -name mi-lsp | head -n 1)"
source_worker="$(find "$tmp/extract" -type d -path "*/workers/$RID" | head -n 1)"
if [ -z "$source_cli" ]; then
  echo "Extracted archive did not contain mi-lsp." >&2
  exit 1
fi
if [ -z "$source_worker" ]; then
  echo "Extracted archive did not contain workers/$RID." >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR/workers"
target="$INSTALL_DIR/mi-lsp"
if [ -x "$target" ]; then
  "$target" daemon stop --format compact >/dev/null 2>&1 || true
fi
workers_root="$(cd "$INSTALL_DIR/workers" && pwd -P)"
target_worker="$workers_root/$RID"
case "$target_worker" in
  "$workers_root"/*) ;;
  *) echo "Refusing to replace worker directory outside install workers root: $target_worker" >&2; exit 1 ;;
esac
retry rm -rf "$target_worker"
retry cp "$source_cli" "$target"
retry cp -R "$source_worker" "$workers_root/"
chmod +x "$target"
"$target" daemon stop --format compact >/dev/null 2>&1 || true

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Add mi-lsp to PATH with:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

if [ "$SKIP_WORKER_INSTALL" -eq 0 ]; then
  "$target" worker install --rid "$RID" --format compact
fi
"$target" version --format toon
"$target" worker status --format compact
echo "mi-lsp $tag installed at $target"
