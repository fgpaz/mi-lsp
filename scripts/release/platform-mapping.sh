#!/usr/bin/env sh

# Map internal bundle RIDs to the public GoReleaser asset suffix.
# Worker directories intentionally keep the internal osx-* names.
release_asset_rid() {
  case "$1" in
    win-arm64|win-x64|linux-arm64|linux-x64) printf '%s\n' "$1" ;;
    osx-arm64) printf '%s\n' 'darwin-arm64' ;;
    osx-x64) printf '%s\n' 'darwin-x64' ;;
    *)
      echo "Unsupported release RID '$1'." >&2
      return 1
      ;;
  esac
}
