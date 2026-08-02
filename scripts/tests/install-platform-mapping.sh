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
printf '%s\n' '{"tag_name":"latest"}'
EOF

  chmod +x "$stub_dir/uname" "$stub_dir/curl"
  for tool in wget gh; do printf "#!/usr/bin/env sh
printf '%s
' "$*" >>'$TMP_ROOT/'$tool.log
exit 97
" >"$stub_dir/$tool"; chmod +x "$stub_dir/$tool"; done
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
  assert_contains "$output" "archive=mi-lsp_latest_${expected_archive_rid}.tar.gz"
  [ ! -s "$TMP_ROOT/curl.log" ] || fail "$label invoked curl during dry-run"
  [ ! -s "$TMP_ROOT/wget.log" ] || fail "$label invoked wget during dry-run"
  [ ! -s "$TMP_ROOT/gh.log" ] || fail "$label invoked gh during dry-run"
  [ ! -e "$TMP_ROOT/install" ] || fail "$label created install files"
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

sha256_for_test() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

run_offline_install() {
  fixture="$TMP_ROOT/offline-fixture"
  bundle="$fixture/bundle"
  stub_dir="$fixture/bin"
  install_dir="$fixture/install"
  archive="$fixture/mi-lsp_latest_linux-x64.tar.gz"
  checksums="$fixture/mi-lsp_latest_checksums.txt"
  mkdir -p "$bundle/workers/linux-x64" "$stub_dir"

  cat >"$bundle/mi-lsp" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
  cat >"$bundle/workers/linux-x64/MiLsp.Worker" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
  chmod +x "$bundle/mi-lsp" "$bundle/workers/linux-x64/MiLsp.Worker"

  worker_size="$(wc -c <"$bundle/workers/linux-x64/MiLsp.Worker" | tr -d ' ')"
  worker_hash="$(sha256_for_test "$bundle/workers/linux-x64/MiLsp.Worker")"
  cat >"$bundle/workers/linux-x64/worker-manifest.json" <<EOF
{"schema":"mi-lsp-worker-manifest/v1","rid":"linux-x64","protocol":"mi-lsp-v1.1","file_count":1,"files":[{"path":"MiLsp.Worker","size":$worker_size,"sha256":"$worker_hash"}]}
EOF
  (cd "$bundle" && tar -czf "$archive" mi-lsp workers)
  printf '%s  %s\n' "$(sha256_for_test "$archive")" "$(basename "$archive")" >"$checksums"

  cat >"$stub_dir/curl" <<'EOF'
#!/usr/bin/env sh
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -H) shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
case "$url" in
  */releases/latest) printf '%s\n' '{"tag_name":"latest"}' ;;
  *.tar.gz) cp "$MI_LSP_FIXTURE_ARCHIVE" "$out" ;;
  *_checksums.txt) cp "$MI_LSP_FIXTURE_CHECKSUMS" "$out" ;;
  *) echo "unexpected URL: $url" >&2; exit 1 ;;
esac
EOF
  chmod +x "$stub_dir/curl"

  if ! output="$(PATH="$stub_dir:$PATH" \
      MI_LSP_FIXTURE_ARCHIVE="$archive" \
      MI_LSP_FIXTURE_CHECKSUMS="$checksums" \
      sh "$INSTALLER" --rid linux-x64 --install-dir "$install_dir" 2>&1)"; then
    fail "offline install failed: $output"
  fi
  [ -x "$install_dir/mi-lsp" ] || fail "offline install did not place the CLI"
  [ -x "$install_dir/workers/linux-x64/MiLsp.Worker" ] || fail "offline install did not place the worker"
}

run_offline_install

echo "PASS: platform mapping, pre-network validation, and offline archive installation"
