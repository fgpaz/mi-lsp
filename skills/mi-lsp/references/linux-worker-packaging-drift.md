# Linux worker packaging drift

Use this note when installing or repairing `mi-lsp` on Linux and `mi-lsp worker status --format toon` reports that the CLI exists but no worker is usable.

## Symptom

`mi-lsp` is callable from the install directory, but worker status shows:

```toon
bundled_error: bundled worker binary not found
installed_error: installed worker binary not found
dev_local_error: dotnet is not available for dev-local worker execution
install_hint: Run `mi-lsp worker install` to refresh the bundled/global worker.
```

Running `mi-lsp worker install` may fail with:

```text
worker install could not find a bundled worker and no source project was available; run from a mi-lsp distribution with workers/<rid> or publish the worker from source
```

## Cause observed

On a teslita Linux x64 install from GitHub release `v0.2.0`, the `linux-x64` release tarball contained a templated worker directory instead of the concrete RID path:

```text
workers/{{ if eq .Os "windows" }}win{{ else }}{{ .Os }}{{ end }}-{{ if eq .Arch "amd64" }}x64{{ else }}{{ .Arch }}{{ end }}/...
```

The extracted tree also contained Roslyn/MSBuild build-host files but no concrete worker executable under `workers/linux-x64` that the CLI recognized. In that state, copying only `workers/linux-x64/` next to the CLI is not sufficient.

## Diagnostic commands

```bash
export PATH="$HOME/.local/opt/mi-lsp:$PATH"
command -v mi-lsp
mi-lsp worker status --format toon
find "$HOME/.local/opt/mi-lsp/workers" -maxdepth 3 -type f \( -perm -111 -o -name '*.dll' -o -name '*.deps.json' -o -name '*.runtimeconfig.json' \) | head -120
```

Check the downloaded archive before replacing a working install:

```bash
cd /tmp
curl -fL https://github.com/fgpaz/mi-lsp/releases/download/v0.2.0/mi-lsp_0.2.0_linux-x64.tar.gz -o milsp.tgz
tar -tzf milsp.tgz | grep -Ei 'worker|mi-lsp|linux-x64' | head -80
```

If the tar listing contains a literal `{{ if eq .Os ... }}` directory, treat the release asset as packaging-drifted.

## Repair options

Prefer these in order:

1. Install a newer fixed release asset if available, then verify `mi-lsp worker status --format toon` before continuing.
2. If you control the release pipeline, rebuild/publish the Linux asset with a concrete `workers/linux-x64/` directory and the expected worker executable.
3. If source is available on the host, install dotnet and publish the worker from source, then rerun `mi-lsp worker install`.
4. Only as a last resort, inspect the templated directory and try renaming it to the concrete RID. Do not call this successful until `mi-lsp worker status --format toon` reports a selected compatible worker.

## Source-build repair recipe

Use this when the release CLI is installed but no compatible bundled/installed worker is selected. Keep all paths explicit; remote shells may not have the expected `$HOME`.

```bash
set -euo pipefail
export HOME=/home/fgpaz
INSTALL=/home/fgpaz/.local/opt/mi-lsp
SRC=/home/fgpaz/repos/mios/mi-lsp
mkdir -p /home/fgpaz/repos/mios /home/fgpaz/.dotnet

if [ ! -d "$SRC/.git" ]; then
  git clone https://github.com/fgpaz/mi-lsp.git "$SRC"
else
  git -C "$SRC" fetch --all --tags --prune
fi
git -C "$SRC" checkout v0.2.0

if [ ! -x /home/fgpaz/.dotnet/dotnet ]; then
  curl -fsSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
  bash /tmp/dotnet-install.sh --channel 10.0 --install-dir /home/fgpaz/.dotnet --no-path
fi
export DOTNET_ROOT=/home/fgpaz/.dotnet
export PATH=/home/fgpaz/.dotnet:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

rm -rf /tmp/mi-lsp-worker-linux-x64
mkdir -p /tmp/mi-lsp-worker-linux-x64
dotnet publish "$SRC/worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj" \
  -c Release -r linux-x64 --self-contained true \
  -o /tmp/mi-lsp-worker-linux-x64

mkdir -p "$INSTALL/workers"
rm -rf "$INSTALL/workers/linux-x64"
mkdir -p "$INSTALL/workers/linux-x64"
cp -a /tmp/mi-lsp-worker-linux-x64/. "$INSTALL/workers/linux-x64/"
chmod +x "$INSTALL/workers/linux-x64"/MiLsp.Worker* 2>/dev/null || true

export PATH="$INSTALL:$PATH"
mi-lsp worker status --format toon
```

Expected success fields:

```toon
bundled_compatible: true
selected_compatible: true
selected_source: bundle
selected_path: /home/fgpaz/.local/opt/mi-lsp/workers/linux-x64/MiLsp.Worker
protocol_version: mi-lsp-v1.1
```

During the v0.2.0 repair, `dotnet restore/publish` warned about `System.Security.Cryptography.Xml 9.0.0` advisories. That does not block the local worker repair, but it is an upstream maintenance item for the package.

## Completion criteria

Do not mark Linux `mi-lsp` fully installed merely because the CLI binary exists. Mark it complete only when:

- `command -v mi-lsp` points to the intended install directory.
- `mi-lsp worker status --format toon` reports a selected compatible worker, or the task explicitly only needs text-index/direct-mode features.
- Any daemon restart/warmup required by the task has been run after replacing binaries.
