#!/usr/bin/env sh
# GoReleaser hook: generate ignored PE resources for both Windows binaries.
set -eu
version=${1%%[-+]*}
case "$version" in
  *[!0-9.]*|'') echo 'invalid numeric PE version' >&2; exit 1 ;;
esac
old_ifs=$IFS
IFS=.
set -- $version
IFS=$old_ifs
[ "$#" -eq 3 ] || { echo 'expected major.minor.patch for PE version' >&2; exit 1; }
file_version="$1.$2.$3.0"
for cmd in mi-lsp mi-lsp-host; do
  go run github.com/tc-hib/go-winres@v0.3.3 make \
    --in "cmd/$cmd/winres.json" --out "cmd/$cmd/rsrc" \
    --arch amd64,arm64 --file-version "$file_version" --product-version "$file_version"
done
