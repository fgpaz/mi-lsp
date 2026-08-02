#!/usr/bin/env sh
set -eu
root=$(mktemp -d)
mkdir -p "$root/src/workerdir"; printf old >"$root/old"; printf oldw >"$root/src/workerdir/file"
printf new >"$root/src/cli"; printf neww >"$root/src/workerdir/file"
run() { MI_LSP_INSTALL_TEST_MODE=activation MI_LSP_TEST_INSTALL_ROOT="$root/active" MI_LSP_TEST_SOURCE_CLI="$root/src/cli" MI_LSP_TEST_SOURCE_WORKER="$root/src/workerdir" MI_LSP_TEST_RID=linux-x64 MI_LSP_INSTALL_FAIL_PHASE="$1" sh scripts/install/install.sh >/dev/null 2>&1 || :; }
mkdir -p "$root/active/workers/linux-x64"; printf old >"$root/active/mi-lsp"; printf oldw >"$root/active/workers/linux-x64/file"
run cli-activation; test "$(cat "$root/active/mi-lsp")" = old; test "$(cat "$root/active/workers/linux-x64/file")" = oldw
find "$root/active" -type f -delete; find "$root/active" -type d -depth -empty -delete; run worker-activation; test ! -e "$root/active/mi-lsp"
MI_LSP_INSTALL_TEST_MODE=activation MI_LSP_TEST_INSTALL_ROOT="$root/active" MI_LSP_TEST_SOURCE_CLI="$root/src/cli" MI_LSP_TEST_SOURCE_WORKER="$root/src/workerdir" MI_LSP_TEST_RID=linux-x64 sh scripts/install/install.sh >/dev/null
 test "$(cat "$root/active/mi-lsp")" = new; test "$(cat "$root/active/workers/linux-x64/file")" = neww
printf 'PASS: shell rollback, first install, success
'
