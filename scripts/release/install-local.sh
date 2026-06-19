#!/usr/bin/env sh
set -eu

RID="${MI_LSP_RID:-}"
INSTALL_DIR="${MI_LSP_INSTALL_DIR:-$HOME/.local/bin}"
OUT_DIR="${MI_LSP_DIST_DIR:-}"
SKIP_BUILD=0
SKIP_WORKER_REFRESH=0
DRY_RUN=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --rid) RID="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --skip-worker-refresh) SKIP_WORKER_REFRESH=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

script_dir="$(CDPATH= cd "$(dirname "$0")" && pwd -P)"
repo_root="$(CDPATH= cd "$script_dir/../.." && pwd -P)"
if [ -z "$OUT_DIR" ]; then
  OUT_DIR="$repo_root/dist"
fi

detect_rid() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux) os_part="linux" ;;
    Darwin) os_part="osx" ;;
    *) echo "Unsupported OS '$os'. Supported: Linux, macOS." >&2; exit 1 ;;
  esac
  case "$arch" in
    x86_64|amd64) arch_part="x64" ;;
    aarch64|arm64) arch_part="arm64" ;;
    *) echo "Unsupported architecture '$arch'." >&2; exit 1 ;;
  esac
  echo "${os_part}-${arch_part}"
}

normalize_rid() {
  case "$1" in
    darwin-x64) echo "osx-x64" ;;
    darwin-arm64) echo "osx-arm64" ;;
    osx-x64|osx-arm64|linux-x64|linux-arm64) echo "$1" ;;
    win-*) echo "install-local.sh does not install Windows RIDs. Use scripts/release/install-local.ps1." >&2; exit 1 ;;
    *) echo "Unsupported RID '$1'. Supported values: linux-x64, linux-arm64, osx-x64, osx-arm64." >&2; exit 1 ;;
  esac
}

rid_to_goos() {
  case "$1" in
    linux-*) echo "linux" ;;
    osx-*) echo "darwin" ;;
    *) echo "Unsupported RID '$1'." >&2; exit 1 ;;
  esac
}

rid_to_goarch() {
  case "$1" in
    *-x64) echo "amd64" ;;
    *-arm64) echo "arm64" ;;
    *) echo "Unsupported RID '$1'." >&2; exit 1 ;;
  esac
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command missing: $1" >&2
    exit 1
  fi
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

if [ -z "$RID" ]; then
  RID="$(detect_rid)"
fi
RID="$(normalize_rid "$RID")"
GOOS_VALUE="$(rid_to_goos "$RID")"
GOARCH_VALUE="$(rid_to_goarch "$RID")"

dist_root="$OUT_DIR/$RID"
source_cli="$dist_root/mi-lsp"
source_worker="$dist_root/workers/$RID"

if [ "$DRY_RUN" -eq 1 ]; then
  printf 'repo=%s\nrid=%s\ngoos=%s\ngoarch=%s\nout_dir=%s\ninstall_dir=%s\n' \
    "$repo_root" "$RID" "$GOOS_VALUE" "$GOARCH_VALUE" "$OUT_DIR" "$INSTALL_DIR"
  exit 0
fi

require_cmd go
require_cmd dotnet

if [ "$SKIP_BUILD" -eq 0 ]; then
  mkdir -p "$dist_root" "$source_worker"
  (
    cd "$repo_root"
    CGO_ENABLED=0 GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" go build -ldflags="-s -w" -o "$source_cli" ./cmd/mi-lsp
    dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r "$RID" --self-contained true -o "$source_worker"
  )
fi

if [ ! -x "$source_cli" ]; then
  echo "Built CLI was not found or is not executable at '$source_cli'." >&2
  exit 1
fi
if [ ! -d "$source_worker" ]; then
  echo "Built worker directory was not found at '$source_worker'." >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR/workers"
target="$INSTALL_DIR/mi-lsp"
if [ -x "$target" ]; then
  "$target" daemon stop --format compact >/dev/null 2>&1 || true
fi

workers_root="$(CDPATH= cd "$INSTALL_DIR/workers" && pwd -P)"
target_worker="$workers_root/$RID"
case "$target_worker" in
  "$workers_root"/*) ;;
  *) echo "Refusing to replace worker directory outside install workers root: $target_worker" >&2; exit 1 ;;
esac

retry rm -rf "$target_worker"
retry cp "$source_cli" "$target"
retry cp -R "$source_worker" "$workers_root/"
chmod +x "$target"
find "$target_worker" -type f -name 'MiLsp.Worker' -exec chmod +x {} \; 2>/dev/null || true

if [ "$SKIP_WORKER_REFRESH" -eq 0 ]; then
  (
    cd "$INSTALL_DIR"
    "$target" worker install --rid "$RID" --format compact
  )
fi
(
  cd "$INSTALL_DIR"
  "$target" version --format toon
  "$target" worker status --format compact
)

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Add mi-lsp to PATH with:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

echo "mi-lsp local build installed at $target"
