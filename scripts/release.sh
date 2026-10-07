#!/usr/bin/env bash
# Local release path for mi-lsp. Mirrors .github/workflows/release.yml up to
# dist/ artifacts. Does not create git tags. Does not print secrets.
#
# goreleaser lookup:
#   1. $GORELEASER if it is an executable file
#   2. goreleaser on PATH
#   3. ${GOBIN:-$HOME/go/bin}/goreleaser
#   4. install pin v2.13.1 into that GOBIN (go toolchain may download a newer
#      Go only to compile goreleaser; the project build still uses the host go)
set -euo pipefail

usage() {
  echo "Usage: scripts/release.sh <version> [--upload]" >&2
  exit 2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
fi

version="$1"
upload=0
if [[ $# -eq 2 ]]; then
  [[ "$2" == "--upload" ]] || usage
  upload=1
fi

if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z]+)?$ ]]; then
  echo "version must look like 0.10.4 (no v prefix)" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

find_goreleaser() {
  if [[ -n "${GORELEASER:-}" && -x "${GORELEASER}" ]]; then
    printf '%s\n' "$GORELEASER"
    return 0
  fi
  if command -v goreleaser >/dev/null 2>&1; then
    command -v goreleaser
    return 0
  fi
  local gobin="${GOBIN:-$HOME/go/bin}"
  if [[ -x "$gobin/goreleaser" ]]; then
    printf '%s\n' "$gobin/goreleaser"
    return 0
  fi
  mkdir -p "$gobin"
  echo "installing goreleaser v2.13.1 into $gobin" >&2
  GOBIN="$gobin" go install github.com/goreleaser/goreleaser/v2@v2.13.1
  printf '%s\n' "$gobin/goreleaser"
}

expected_names=(
  "mi-lsp_${version}_checksums.txt"
  "mi-lsp_${version}_darwin-arm64.tar.gz"
  "mi-lsp_${version}_darwin-x64.tar.gz"
  "mi-lsp_${version}_linux-arm64.tar.gz"
  "mi-lsp_${version}_linux-x64.tar.gz"
  "mi-lsp_${version}_win-arm64.zip"
  "mi-lsp_${version}_win-x64.zip"
)

if [[ "$upload" -eq 0 ]]; then
  echo "release.sh: build ${version} without publish" >&2
  MI_LSP_RELEASE=1 sh scripts/tests/release-platform-mapping.sh

  arch="$(uname -m)"
  if [[ "$arch" != "aarch64" && "$arch" != "arm64" ]]; then
    go test -race ./...
  else
    echo "release.sh: skip -race on ${arch}" >&2
  fi

  dotnet build worker-dotnet/MiLsp.Worker.sln -c Release

  goreleaser_bin="$(find_goreleaser)"
  # Reuse the tag name the workflow would see. Do not create a tag.
  export GORELEASER_CURRENT_TAG="v${version}"
  "$goreleaser_bin" release --clean --skip=publish

  missing=0
  for name in "${expected_names[@]}"; do
    if [[ ! -f "dist/${name}" ]]; then
      echo "missing dist/${name}" >&2
      missing=1
    fi
  done
  if [[ "$missing" -ne 0 ]]; then
    echo "dist/ contents:" >&2
    ls -1 dist >&2 || true
    exit 1
  fi
  echo "release.sh: dist artifacts ok" >&2
  ls -1 dist
  exit 0
fi

tag="v${version}"
if ! git rev-parse --verify --quiet "refs/tags/${tag}" >/dev/null; then
  echo "refusing upload: tag ${tag} does not exist (this script does not create tags)" >&2
  exit 1
fi
head="$(git rev-parse HEAD)"
tagged="$(git rev-parse "${tag}^{commit}")"
if [[ "$head" != "$tagged" ]]; then
  echo "refusing upload: HEAD ${head} is not ${tag} (${tagged})" >&2
  exit 1
fi

files=()
for name in "${expected_names[@]}"; do
  if [[ ! -f "dist/${name}" ]]; then
    echo "refusing upload: dist/${name} is missing; run without --upload first" >&2
    exit 1
  fi
  files+=("dist/${name}")
done

if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" --clobber "${files[@]}"
else
  gh release create "$tag" --verify-tag --title "mi-lsp ${tag}" "${files[@]}"
fi
echo "release.sh: uploaded ${#files[@]} assets to ${tag}" >&2
