#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"
INSTALLER="$ROOT/scripts/install/install.sh"
TMP_ROOT="${TMPDIR:-/tmp}/mi-lsp-install-platform-test-$$"

cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  haystack="$1"
  needle="$2"
  case "$haystack" in
    *"$needle"*) ;;
    *) fail "expected output to contain: $needle" ;;
  esac
}

make_stubs() {
  stub_dir="$1"
  os="$2"
  arch="$3"
  mkdir -p "$stub_dir"

  cat >"$stub_dir/uname" <<EOF
#!/usr/bin/env sh
case "\${1:-}" in
  -s) printf '%s\n' '$os' ;;
  -m) printf '%s\n' '$arch' ;;
  *) printf '%s\n' '$os' ;;
esac
EOF

  cat >"$stub_dir/curl" <<EOF
#!/usr/bin/env sh
printf '%s\n' "\$*" >>'$TMP_ROOT/curl.log'
printf '%s\n' '{"tag_name":"v0.0.0-test"}'
EOF

  chmod +x "$stub_dir/uname" "$stub_dir/curl"
}

run_supported() {
  label="$1"
  stub_dir="$2"
  expected_rid="$3"
  expected_archive_rid="$4"
  expected_worker_rid="$5"
  shift 5
  : >"$TMP_ROOT/curl.log"

  output="$(PATH="$stub_dir:$PATH" sh "$INSTALLER" --dry-run --install-dir "$TMP_ROOT/install" "$@" 2>&1)"
  assert_contains "$output" "rid=$expected_rid"
  assert_contains "$output" "archive_rid=$expected_archive_rid"
  assert_contains "$output" "worker_rid=$expected_worker_rid"
  assert_contains "$output" "archive=mi-lsp_0.0.0-test_${expected_archive_rid}.tar.gz"
  [ -s "$TMP_ROOT/curl.log" ] || fail "$label did not resolve release metadata"
}

mkdir -p "$TMP_ROOT"
make_stubs "$TMP_ROOT/darwin-bin" Darwin arm64
make_stubs "$TMP_ROOT/linux-bin" Linux x86_64

run_supported "auto-detected Darwin" "$TMP_ROOT/darwin-bin" darwin-arm64 darwin-arm64 osx-arm64
run_supported "explicit darwin-x64" "$TMP_ROOT/linux-bin" darwin-x64 darwin-x64 osx-x64 --rid darwin-x64
run_supported "explicit osx-x64" "$TMP_ROOT/linux-bin" osx-x64 darwin-x64 osx-x64 --rid osx-x64
run_supported "auto-detected Linux" "$TMP_ROOT/linux-bin" linux-x64 linux-x64 linux-x64

: >"$TMP_ROOT/curl.log"
if invalid_output="$(PATH="$TMP_ROOT/linux-bin:$PATH" sh "$INSTALLER" --rid plan9-x64 2>&1)"; then
  fail "invalid RID unexpectedly succeeded"
fi
assert_contains "$invalid_output" "darwin-x64, darwin-arm64, osx-x64, osx-arm64"
[ ! -s "$TMP_ROOT/curl.log" ] || fail "invalid RID invoked curl before validation"

echo "PASS: Darwin archives map to OSX workers, Linux remains supported, and invalid RIDs fail before network access"
