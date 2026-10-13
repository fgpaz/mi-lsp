#!/usr/bin/env bash
# Explicit opt-in capture of effective main-jvm. Never called by runtime,
# discovery, build.sh, release hooks or worker startup. No wrapper, no download,
# no SDK install, no language-server bootstrap.
set -euo pipefail
if [[ "${MI_LSP_KOTLIN_EXPORT_MAIN_JVM:-}" != "1" ]]; then
  echo 'explicit_incomplete: set MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1 to capture; refusing implicit run' >&2
  exit 2
fi
if [[ $# -ne 1 || "$1" != /* || "$1" == *" "* || "$1" == *$'\t'* || "$1" == *$'\n'* ]]; then
  echo 'usage: MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1 bash worker-kotlin/config/export-main-jvm.sh /absolute/module' >&2
  exit 2
fi
module=$(cd -- "$1" && pwd -P)
[[ -f "$module/build.gradle" || -f "$module/build.gradle.kts" ]] || { echo 'explicit_incomplete: module build file missing' >&2; exit 2; }
command -v gradle >/dev/null 2>&1 || { echo 'explicit_incomplete: Gradle 8.10 must already be installed; no download' >&2; exit 2; }
observed=$(gradle --no-daemon --version | awk '/^Gradle /{print $2; exit}')
[[ "$observed" == "8.10" ]] || { echo "explicit_incomplete: requires Gradle 8.10, found ${observed:-absent}; Java 25 is not a substitute" >&2; exit 2; }
here=$(cd -- "$(dirname -- "$0")" && pwd)
init="$here/export-main-jvm.init.gradle.kts"
[[ -f "$init" && "$here" != *" "* && "$init" != *" "* ]] || { echo 'explicit_incomplete: init script path unusable' >&2; exit 2; }
export MI_LSP_KOTLIN_EXPORT_MODULE="$module"
export MI_LSP_KOTLIN_EXPORTER_PATH="$here/export-main-jvm.sh"
export MI_LSP_KOTLIN_INIT_PATH="$init"
gradle --offline --no-daemon --no-build-cache -p "$module" -I "$init" miLspExportMainJvm
[[ -f "$module/.mi-lsp/kotlin-main-v1.json" && -f "$module/.mi-lsp/kotlin-main-v1.authority.json" ]] || {
  echo 'explicit_incomplete: capture did not emit a certified main-jvm receipt' >&2
  exit 3
}
echo "captured=$module/.mi-lsp/kotlin-main-v1.authority.json"
