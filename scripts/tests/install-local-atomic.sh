#!/usr/bin/env sh
set -eu
ROOT="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd -P)"
INSTALLER="$ROOT/scripts/release/install-local.sh"
command -v cc >/dev/null 2>&1 || { printf 'FAIL: C compiler (cc) is required for the native CLI fixture\n' >&2; exit 1; }
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/mi-lsp-local-atomic.XXXXXX")"
running_pid=
installer_pid=
cleanup() {
  [ -z "$installer_pid" ] || { kill -KILL "$installer_pid" 2>/dev/null || true; wait "$installer_pid" 2>/dev/null || true; }
  [ -z "$running_pid" ] || { kill "$running_pid" 2>/dev/null || true; wait "$running_pid" 2>/dev/null || true; }
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

mkdir -p "$TMP_ROOT/bin" "$TMP_ROOT/dist/linux-x64/workers/linux-x64" "$TMP_ROOT/install/workers/linux-x64"
cc -std=c99 -Wall -Wextra -Werror "$ROOT/scripts/tests/install-local-cli-fixture.c" -o "$TMP_ROOT/old-mi-lsp"
for tool in go dotnet; do
  printf '#!/usr/bin/env sh\nexit 0\n' >"$TMP_ROOT/bin/$tool"
  chmod +x "$TMP_ROOT/bin/$tool"
done
cat >"$TMP_ROOT/dist/linux-x64/mi-lsp" <<'CLI'
#!/usr/bin/env sh
case "$*" in
  'daemon stop --format compact') printf 'daemon-stop\n' >>"$MI_LSP_TEST_LOG" ;;
  'worker install --rid linux-x64 --format compact') printf 'worker-install\n' >>"$MI_LSP_TEST_LOG" ;;
  'version --format toon')
    printf 'version\n' >>"$MI_LSP_TEST_LOG"
    [ "${MI_LSP_TEST_PAUSE:-0}" != 1 ] || { : >"$MI_LSP_TEST_READY"; sleep 30; }
    ;;
  'worker status --format compact')
    printf 'worker-status\n' >>"$MI_LSP_TEST_LOG"
    [ "${MI_LSP_TEST_STATUS_FAIL:-0}" != 1 ]
    ;;
  *) printf 'unexpected CLI arguments: %s\n' "$*" >&2; exit 64 ;;
esac
CLI
chmod +x "$TMP_ROOT/dist/linux-x64/mi-lsp"
printf 'new-worker\n' >"$TMP_ROOT/dist/linux-x64/workers/linux-x64/MiLsp.Worker"
chmod +x "$TMP_ROOT/dist/linux-x64/workers/linux-x64/MiLsp.Worker"
printf 'old-worker\n' >"$TMP_ROOT/install/workers/linux-x64/MiLsp.Worker"
run_installer() {
  HOME="$TMP_ROOT/home" PATH="$TMP_ROOT/bin:$PATH" MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" sh "$INSTALLER" \
    --rid linux-x64 --install-dir "$TMP_ROOT/install" --out-dir "$TMP_ROOT/dist" \
    --skip-build "$@"
}
reset_case() {
  if [ -n "$running_pid" ]; then kill "$running_pid" 2>/dev/null || true; wait "$running_pid" 2>/dev/null || true; running_pid=; fi
  rm -f "$TMP_ROOT/install/mi-lsp"
  cp "$TMP_ROOT/old-mi-lsp" "$TMP_ROOT/install/mi-lsp"
  rm -rf "$TMP_ROOT/install/workers/linux-x64"
  mkdir -p "$TMP_ROOT/install/workers/linux-x64"
  printf 'old-worker\n' >"$TMP_ROOT/install/workers/linux-x64/MiLsp.Worker"
  rm -rf "$TMP_ROOT/dist/linux-x64/workers/linux-x64"
  mkdir -p "$TMP_ROOT/dist/linux-x64/workers/linux-x64"
  printf 'new-worker\n' >"$TMP_ROOT/dist/linux-x64/workers/linux-x64/MiLsp.Worker"
  chmod +x "$TMP_ROOT/dist/linux-x64/workers/linux-x64/MiLsp.Worker"
  : >"$TMP_ROOT/cli.log"
  rm -f "$TMP_ROOT/ready"
  rm -rf "$TMP_ROOT/home"
  mkdir -p "$TMP_ROOT/home"
}
assert_old_assets() {
  cmp "$TMP_ROOT/old-mi-lsp" "$TMP_ROOT/install/mi-lsp" >/dev/null 2>&1 || fail 'previous CLI was not restored byte-for-byte'
  [ "$(cat "$TMP_ROOT/install/workers/linux-x64/MiLsp.Worker")" = old-worker ] || fail 'previous worker was lost'
}
assert_no_stages() {
  [ -z "$(find "$TMP_ROOT/install" "$TMP_ROOT/install/workers" -maxdepth 1 -name '.mi-lsp-stage.*' -print -quit)" ] || fail 'staging directory leaked'
}

# An actual running ELF stays executable while atomic rename replaces its pathname.
reset_case
MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" "$TMP_ROOT/install/mi-lsp" hold &
running_pid=$!
run_installer --skip-worker-refresh >/dev/null
[ "$(cat "$TMP_ROOT/install/workers/linux-x64/MiLsp.Worker")" = new-worker ] || fail 'worker replacement failed beside running ELF'
grep -q '^daemon stop --format compact$' "$TMP_ROOT/cli.log" || fail 'installer did not issue daemon stop protocol'
grep -q '^version$' "$TMP_ROOT/cli.log" || fail 'version protocol was not exercised'
grep -q '^worker-status$' "$TMP_ROOT/cli.log" || fail 'worker status protocol was not exercised'
kill "$running_pid" 2>/dev/null || true
wait "$running_pid" 2>/dev/null || true
running_pid=

# Missing and incomplete worker artifacts fail before touching either installed asset.
reset_case
rm "$TMP_ROOT/dist/linux-x64/workers/linux-x64/MiLsp.Worker"
if run_installer --skip-worker-refresh >/dev/null 2>&1; then fail 'missing worker executable unexpectedly accepted'; fi
assert_old_assets
assert_no_stages
reset_case
: >"$TMP_ROOT/dist/linux-x64/workers/linux-x64/MiLsp.Worker"
if run_installer --skip-worker-refresh >/dev/null 2>&1; then fail 'empty worker executable unexpectedly accepted'; fi
assert_old_assets
assert_no_stages

# Failures at each activation boundary and the post-activation status probe restore assets.
for phase in worker-staged cli-activation worker-activation status; do
  reset_case
  if HOME="$TMP_ROOT/home" PATH="$TMP_ROOT/bin:$PATH" MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" MI_LSP_INSTALL_FAIL_PHASE="$phase" \
      sh "$INSTALLER" --rid linux-x64 --install-dir "$TMP_ROOT/install" --out-dir "$TMP_ROOT/dist" \
      --skip-build --skip-worker-refresh >/dev/null 2>&1; then
    fail "injected $phase failure unexpectedly succeeded"
  fi
  assert_old_assets
  assert_no_stages
done

# CLI status failure also rolls back; daemon-stop failure is intentionally non-fatal.
reset_case
if HOME="$TMP_ROOT/home" PATH="$TMP_ROOT/bin:$PATH" MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" MI_LSP_TEST_STATUS_FAIL=1 \
    sh "$INSTALLER" --rid linux-x64 --install-dir "$TMP_ROOT/install" --out-dir "$TMP_ROOT/dist" \
    --skip-build --skip-worker-refresh >/dev/null 2>&1; then fail 'failed worker status unexpectedly succeeded'; fi
assert_old_assets
assert_no_stages

# Hard-link backup must preserve a CLI symlink itself, not merely its referent.
reset_case
cp "$TMP_ROOT/old-mi-lsp" "$TMP_ROOT/old-cli"
rm "$TMP_ROOT/install/mi-lsp"
ln -s "$TMP_ROOT/old-cli" "$TMP_ROOT/install/mi-lsp"
if HOME="$TMP_ROOT/home" PATH="$TMP_ROOT/bin:$PATH" MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" MI_LSP_INSTALL_FAIL_PHASE=worker-activation \
    sh "$INSTALLER" --rid linux-x64 --install-dir "$TMP_ROOT/install" --out-dir "$TMP_ROOT/dist" \
    --skip-build --skip-worker-refresh >/dev/null 2>&1; then fail 'symlink rollback injection unexpectedly succeeded'; fi
[ -L "$TMP_ROOT/install/mi-lsp" ] || fail 'CLI symlink type was not restored'
[ "$(readlink "$TMP_ROOT/install/mi-lsp")" = "$TMP_ROOT/old-cli" ] || fail 'CLI symlink target changed during rollback'
cmp "$TMP_ROOT/old-mi-lsp" "$TMP_ROOT/old-cli" >/dev/null || fail 'CLI symlink referent changed'

# Existing worker symlink is also moved/restored as a link; dangling links are covered.
reset_case
rm -rf "$TMP_ROOT/install/workers/linux-x64"
ln -s "$TMP_ROOT/missing-worker-target" "$TMP_ROOT/install/workers/linux-x64"
if HOME="$TMP_ROOT/home" PATH="$TMP_ROOT/bin:$PATH" MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" MI_LSP_INSTALL_FAIL_PHASE=worker-activation \
    sh "$INSTALLER" --rid linux-x64 --install-dir "$TMP_ROOT/install" --out-dir "$TMP_ROOT/dist" \
    --skip-build --skip-worker-refresh >/dev/null 2>&1; then fail 'worker symlink rollback injection unexpectedly succeeded'; fi
[ -L "$TMP_ROOT/install/workers/linux-x64" ] || fail 'dangling worker symlink was not restored'
[ "$(readlink "$TMP_ROOT/install/workers/linux-x64")" = "$TMP_ROOT/missing-worker-target" ] || fail 'worker symlink target changed'
assert_no_stages

# A signal while post-activation CLI work is blocked must exit non-zero and roll back.
reset_case
HOME="$TMP_ROOT/home" PATH="$TMP_ROOT/bin:$PATH" MI_LSP_TEST_LOG="$TMP_ROOT/cli.log" MI_LSP_TEST_PAUSE=1 \
  MI_LSP_TEST_READY="$TMP_ROOT/ready" sh "$INSTALLER" --rid linux-x64 --install-dir "$TMP_ROOT/install" \
  --out-dir "$TMP_ROOT/dist" --skip-build --skip-worker-refresh >/dev/null 2>&1 &
installer_pid=$!
count=0
while [ ! -e "$TMP_ROOT/ready" ] && kill -0 "$installer_pid" 2>/dev/null; do
  count=$((count + 1)); [ "$count" -lt 100 ] || fail 'installer did not reach signal probe'; sleep 0.05
done
[ -e "$TMP_ROOT/ready" ] || fail 'installer exited before signal probe'
kill -TERM "$installer_pid"
if wait "$installer_pid"; then fail 'TERM did not make installer exit non-zero'; fi
installer_pid=
assert_old_assets
assert_no_stages
printf 'PASS: atomic activation, artifact validation, rollback, symlinks, signals, and cleanup\n'
