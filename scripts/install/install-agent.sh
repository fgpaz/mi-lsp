#!/usr/bin/env sh
set -eu

REPO="${MI_LSP_REPO:-fgpaz/mi-lsp}"
RID="${MI_LSP_RID:-}"
INSTALL_DIR="${MI_LSP_INSTALL_DIR:-$HOME/.local/bin}"
SKILL_REPO="${MI_LSP_SKILL_REPO:-fgpaz/mi-lsp}"
SKILL_NAME="${MI_LSP_SKILL_NAME:-mi-lsp}"
AGENTS="${MI_LSP_AGENTS:-codex claude-code}"
DRY_RUN=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --rid) RID="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --skill-repo) SKILL_REPO="$2"; shift 2 ;;
    --skill) SKILL_NAME="$2"; shift 2 ;;
    --agent)
      if [ "$AGENTS" = "codex claude-code" ]; then AGENTS=""; fi
      AGENTS="${AGENTS:+$AGENTS }$2"
      shift 2
      ;;
    --dry-run) DRY_RUN=1; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

get_script_dir() {
  case "$0" in
    */*) dirname "$0" ;;
    *) echo "" ;;
  esac
}

script_dir="$(get_script_dir)"
if [ -n "$script_dir" ] && [ -f "$script_dir/install.sh" ]; then
  install_script="$script_dir/install.sh"
else
  tmp="${TMPDIR:-/tmp}/mi-lsp-install-agent-$$"
  rm -rf "$tmp"
  mkdir -p "$tmp"
  trap 'rm -rf "$tmp"' EXIT INT TERM
  install_script="$tmp/install.sh"
  curl -fsSL https://raw.githubusercontent.com/fgpaz/mi-lsp/main/scripts/install/install.sh -o "$install_script"
fi

if [ -n "$RID" ]; then
  if [ "$DRY_RUN" -eq 1 ]; then
    sh "$install_script" --repo "$REPO" --install-dir "$INSTALL_DIR" --rid "$RID" --dry-run
  else
    sh "$install_script" --repo "$REPO" --install-dir "$INSTALL_DIR" --rid "$RID"
  fi
else
  if [ "$DRY_RUN" -eq 1 ]; then
    sh "$install_script" --repo "$REPO" --install-dir "$INSTALL_DIR" --dry-run
  else
    sh "$install_script" --repo "$REPO" --install-dir "$INSTALL_DIR"
  fi
fi

set -- skills add "$SKILL_REPO" --skill "$SKILL_NAME" -g
for agent in $AGENTS; do
  set -- "$@" -a "$agent"
done
set -- "$@" -y

printf 'npx'
for arg in "$@"; do
  printf ' %s' "$arg"
done
printf '\n'

if [ "$DRY_RUN" -eq 1 ]; then
  exit 0
fi

if ! command -v npx >/dev/null 2>&1; then
  echo "install-agent requires npx. Install Node.js/npm, then rerun this script. No direct skill-copy fallback is used." >&2
  exit 1
fi

npx "$@"
echo "Installed skill '$SKILL_NAME' for agents: $AGENTS"
