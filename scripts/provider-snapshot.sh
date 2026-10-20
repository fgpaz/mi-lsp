#!/usr/bin/env sh
# Generate only from a binary built from this checkout; run by the goal owner.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
binary=${MI_LSP_BINARY:-mi-lsp}
target="$root/integrations/mi-mcp/provider-manifest.json"
scratch=$(mktemp "$target.XXXXXX")
trap 'rm -f "$scratch"' EXIT HUP INT TERM
"$binary" provider-manifest --format json > "$scratch"
mv "$scratch" "$target"
