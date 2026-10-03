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

paths_overlap() {
  case "$1" in "$2"|"$2"/*) return 0 ;; esac
  case "$2" in "$1"/*) return 0 ;; esac
  return 1
}

resolve_path() {
  path="$1"
  case "$path" in /*) ;; *) path="$PWD/$path" ;; esac
  suffix=""
  while [ ! -e "$path" ] && [ ! -L "$path" ]; do
    name=${path##*/}
    parent=${path%/*}
    [ -n "$parent" ] || parent=/
    suffix="/$name$suffix"
    path="$parent"
  done
  if [ -d "$path" ]; then
    path="$(CDPATH= cd "$path" && pwd -P)"
  else
    name=${path##*/}
    parent=${path%/*}
    [ -n "$parent" ] || parent=/
    parent="$(CDPATH= cd "$parent" && pwd -P)"
    path="$parent/$name"
  fi
  printf '%s%s\n' "$path" "$suffix"
}

cleanup_staging() {
  [ -z "${stage_root:-}" ] || rm -rf "$stage_root"
  [ -z "${worker_stage_root:-}" ] || rm -rf "$worker_stage_root"
  [ -z "${global_stage_root:-}" ] || rm -rf "$global_stage_root"
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
worker_binary="$(find "$source_worker" -type f -name MiLsp.Worker -print -quit)"
if [ -z "$worker_binary" ] || [ ! -s "$worker_binary" ]; then
  echo "Built worker is incomplete; a non-empty MiLsp.Worker executable was not found in '$source_worker'." >&2
  exit 1
fi

if [ "$SKIP_WORKER_REFRESH" -eq 0 ]; then
  pre_global_home="$(resolve_path "$HOME")"
  pre_global_worker="$(resolve_path "$pre_global_home/.mi-lsp/workers/$RID")"
  pre_install_root="$(resolve_path "$INSTALL_DIR")"
  pre_local_cli="$(resolve_path "$pre_install_root/mi-lsp")"
  pre_local_worker="$(resolve_path "$pre_install_root/workers/$RID")"
  if paths_overlap "$pre_local_cli" "$pre_global_worker" || paths_overlap "$pre_local_worker" "$pre_global_worker"; then
    echo "Refusing overlapping install destinations: local assets '$pre_local_cli' and '$pre_local_worker' conflict with global worker '$pre_global_worker'." >&2
    exit 1
  fi
fi

mkdir -p "$INSTALL_DIR/workers"
install_root="$(CDPATH= cd "$INSTALL_DIR" && pwd -P)"
workers_root="$(CDPATH= cd "$INSTALL_DIR/workers" && pwd -P)"
target="$install_root/mi-lsp"
target_worker="$workers_root/$RID"
case "$target_worker" in
  "$workers_root"/*) ;;
  *) echo "Refusing to replace worker directory outside install workers root: $target_worker" >&2; exit 1 ;;
esac

stage_root=""
worker_stage_root=""
global_stage_root=""
global_worker_dir=""
trap cleanup_staging EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
if [ "$SKIP_WORKER_REFRESH" -eq 0 ]; then
  global_home="$(CDPATH= cd "$HOME" && pwd -P)"
  global_workers_root="$global_home/.mi-lsp/workers"
  if [ -d "$global_home/.mi-lsp" ]; then
    global_dot_dir="$(CDPATH= cd "$global_home/.mi-lsp" && pwd -P)"
    global_workers_root="$global_dot_dir/workers"
  fi
  if [ -d "$global_workers_root" ]; then
    global_workers_root="$(CDPATH= cd "$global_workers_root" && pwd -P)"
  fi
  global_worker_dir="$global_workers_root/$RID"
  if paths_overlap "$target" "$global_worker_dir" || paths_overlap "$target_worker" "$global_worker_dir"; then
    echo "Refusing overlapping install destinations: local assets '$target' and '$target_worker' conflict with global worker '$global_worker_dir'." >&2
    exit 1
  fi
  mkdir -p "$global_workers_root"
  global_workers_root="$(CDPATH= cd "$global_workers_root" && pwd -P)"
  global_worker_dir="$global_workers_root/$RID"
  if paths_overlap "$target" "$global_worker_dir" || paths_overlap "$target_worker" "$global_worker_dir"; then
    echo "Refusing overlapping install destinations: local assets '$target' and '$target_worker' conflict with global worker '$global_worker_dir'." >&2
    exit 1
  fi
fi
# Stage every replacement beside its destination: rename stays on the same
# filesystem, so replacing a running executable never truncates it in place.
stage_root="$(mktemp -d "$install_root/.mi-lsp-stage.XXXXXX")"
worker_stage_root="$(mktemp -d "$workers_root/.mi-lsp-stage.$RID.XXXXXX")"
if [ "$SKIP_WORKER_REFRESH" -eq 0 ]; then
  global_stage_root="$(mktemp -d "$global_workers_root/.mi-lsp-stage.$RID.XXXXXX")"
fi
staged_cli="$stage_root/mi-lsp"
staged_worker="$worker_stage_root/worker"
cp "$source_cli" "$staged_cli"
chmod +x "$staged_cli"
cp -R "$source_worker" "$staged_worker"
find "$staged_worker" -type f -name 'MiLsp.Worker' -exec chmod +x {} \; 2>/dev/null || true

backup_root="$stage_root/backup"
worker_backup_root="$worker_stage_root/backup"
global_worker_backup="$global_stage_root/worker"
mkdir "$backup_root" "$worker_backup_root"
old_cli=0
old_worker=0
old_global_worker=0
new_cli=0
new_worker=0
new_global_worker=0
committed=0
rollback() {
  status=$?
  restore_failed=0
  trap - EXIT HUP INT TERM
  if [ "$committed" -eq 0 ]; then
    if [ "$new_cli" -eq 1 ] && { [ -e "$target" ] || [ -L "$target" ]; }; then rm -f "$target" || restore_failed=1; fi
    if [ "$new_worker" -eq 1 ] && { [ -e "$target_worker" ] || [ -L "$target_worker" ]; }; then rm -rf "$target_worker" || restore_failed=1; fi
    if [ "$new_global_worker" -eq 1 ] && { [ -e "$global_worker_dir" ] || [ -L "$global_worker_dir" ]; }; then rm -rf "$global_worker_dir" || restore_failed=1; fi
    if [ "$old_cli" -eq 1 ] && [ "$new_cli" -eq 1 ]; then mv "$backup_root/mi-lsp" "$target" || restore_failed=1; fi
    if [ "$old_worker" -eq 1 ]; then mv "$worker_backup_root/worker" "$target_worker" || restore_failed=1; fi
    if [ "$old_global_worker" -eq 1 ] && { [ -e "$global_worker_backup" ] || [ -L "$global_worker_backup" ]; }; then mv "$global_worker_backup" "$global_worker_dir" || restore_failed=1; fi
  fi
  if [ "$restore_failed" -eq 0 ]; then
    cleanup_staging || status=1
  else
    echo "Rollback was incomplete; previous assets may remain in '$backup_root', '$worker_backup_root', and '$global_stage_root'." >&2
    status=1
  fi
  return "$status"
}
trap rollback EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -x "$target" ]; then
  "$target" daemon stop --format compact >/dev/null 2>&1 || true
fi
# Preserve old worker contents until both replacements are fully prepared.
if [ -e "$target_worker" ] || [ -L "$target_worker" ]; then mv "$target_worker" "$worker_backup_root/worker"; old_worker=1; fi
if [ -e "$target" ] || [ -L "$target" ]; then ln -P "$target" "$backup_root/mi-lsp"; old_cli=1; fi
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = worker-staged ] && exit 1
new_cli=1
mv "$staged_cli" "$target"
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = cli-activation ] && exit 1
new_worker=1
mv "$staged_worker" "$target_worker"
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = worker-activation ] && exit 1

if [ "$SKIP_WORKER_REFRESH" -eq 0 ]; then
  if [ -e "$global_worker_dir" ] || [ -L "$global_worker_dir" ]; then
    old_global_worker=1
    mv "$global_worker_dir" "$global_worker_backup"
  fi
  new_global_worker=1
  (cd "$install_root" && "$target" worker install --rid "$RID" --format compact)
fi
(cd "$install_root" && "$target" version --format toon && "$target" worker status --format compact)
[ "${MI_LSP_INSTALL_FAIL_PHASE:-}" = status ] && exit 1

committed=1
trap - EXIT HUP INT TERM
cleanup_staging
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Add mi-lsp to PATH with:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

echo "mi-lsp local build installed at $target"
