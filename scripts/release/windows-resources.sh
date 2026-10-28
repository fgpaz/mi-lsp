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
make windows-resources WINDOWS_FILE_VERSION="$1.$2.$3.0"
