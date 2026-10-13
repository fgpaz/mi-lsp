#!/usr/bin/env bash
# Explicit future build only; never invoked by runtime/discovery/release hooks.
set -euo pipefail
if [[ $# != 2 ]]; then echo 'usage: bash worker-kotlin/build.sh PINNED_LOCAL_CHECKOUT NEW_ABSOLUTE_OUTPUT_DIR' >&2; exit 2; fi
here=$(cd -- "$(dirname -- "$0")" && pwd)
upstream=$(cd -- "$1" && pwd)
out=$2
[[ "$out" = /* && ! -e "$out" ]] || { echo 'output must be absolute and absent' >&2; exit 2; }
ref=f532bcc9c6578f5a35009a8b5f281f93d4d131ba
[[ $(git -C "$upstream" rev-parse HEAD) = "$ref" ]] || { echo 'wrong upstream ref' >&2; exit 2; }
# Metadata/hash checks are part of this future build, NOT authoring-time C2.
python3 - "$here" "$upstream" <<'PY'
import hashlib,json,pathlib,sys
here,upstream=map(pathlib.Path,sys.argv[1:])
pins=json.loads((here/'upstream-inputs.json').read_text())
for path,digest in pins['files'].items():
    if hashlib.sha256((upstream/path).read_bytes()).hexdigest()!=digest:
        raise SystemExit('upstream source hash mismatch: '+path)
if (here/'LICENSE.upstream.txt').read_bytes()!=(upstream/'LICENSE.txt').read_bytes():
    raise SystemExit('upstream license mismatch')
PY
mkdir -p -- "$out/source"
git -C "$upstream" archive "$ref" | tar -xf - -C "$out/source"
git -C "$out/source" apply "$here/upstream.patch"
cp -- "$here/src/main/kotlin/org/javacs/kt/MiLspCompilerObservations.kt" "$out/source/server/src/main/kotlin/org/javacs/kt/"
# Requires preinstalled Gradle 8.10 and JDK 11, cached exact dependencies. No wrapper download.
[[ $(gradle --version | awk '/^Gradle /{print $2}') = 8.10 ]] || { echo 'requires Gradle 8.10' >&2; exit 2; }
(cd -- "$out/source" && gradle --offline --no-daemon -Pversion=1.3.13+mi-lsp.obs2 :server:installDist -x test)
cp -a -- "$out/source/server/build/install/server" "$out/runtime"
mkdir -p -- "$out/runtime/mi-lsp-provenance"
cp -- "$here/LICENSE.upstream.txt" "$here/upstream-inputs.json" "$here/upstream.patch" "$out/runtime/mi-lsp-provenance/"
cp -- "$here/licenses/upstream-licenseReport.html" "$here/licenses/Kotlin-compiler-2.1.0-LICENSE.txt" "$here/compiler-api-inputs.json" "$out/runtime/mi-lsp-provenance/"
cp -- "$here/src/main/kotlin/org/javacs/kt/MiLspCompilerObservations.kt" "$out/runtime/mi-lsp-provenance/"
python3 - "$here" "$out/runtime" <<'PY'
import hashlib,json,pathlib,sys
here,runtime=map(pathlib.Path,sys.argv[1:]); launcher=runtime/'bin/kotlin-language-server'
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
manifest={'server_version':'1.3.13+mi-lsp.obs2','compiler_version':'2.1.0','upstream_commit':'f532bcc9c6578f5a35009a8b5f281f93d4d131ba','launcher_sha256':sha(launcher),'extension_version':'mi-lsp-compiler-observations/v2','extension_source_sha256':sha(here/'src/main/kotlin/org/javacs/kt/MiLspCompilerObservations.kt'),'extension_patch_sha256':sha(here/'upstream.patch'),'distribution_files':{str(p.relative_to(runtime)):sha(p) for p in sorted(runtime.rglob('*')) if p.is_file()}}
pathlib.Path(str(launcher)+'.mi-lsp.json').write_text(json.dumps(manifest,indent=2)+'\n')
PY
printf 'launcher=%s\n' "$out/runtime/bin/kotlin-language-server"
