# mi-lsp release/distribution prep — 2026-10-30

## Alcance y estado

Preparación documental/de solo lectura para empaquetar en Tesla y VPS cuando exista un source SHA aceptado. **No** corrí builds, tests, `dotnet publish`, instalación/activación, daemon queries en VPS, creación de tags ni publicación. No usé Yoga como dependencia de esta preparación.

Persona consultada: `persona:arranque-con-punteros-no-con-contenido-20261025`. `mi-decidir consultar` no devolvió IDs de decisiones aplicables; no se deriva una versión o un source binding de esa consulta.

## Release público y colisión de tag

Consulta a GitHub API el 2026-10-30:

- Release más reciente: **v0.16.7**, publicado 2026-10-30.
- Source commit del tag oficial existente: `efb88b3041f6d1100d272e5e3e84fb8f8eb95db1`.
- `v0.16.8`: no existe ref de tag ni release en el API consultado. Esto es solo una comprobación de colisión; **no** selecciona ni autoriza ese tag como versión siguiente.
- Assets públicos observados: `mi-lsp_0.16.7_{darwin-arm64,darwin-x64,linux-arm64,linux-x64,win-arm64,win-x64}.{tar.gz|zip}`, `mi-lsp-host_0.16.7_win-{arm64,x64}.exe` y `mi-lsp_0.16.7_checksums.txt` (SHA-256). Son 6 archives de CLI/bundles, 2 host executables y un archivo de checksums.

El `HEAD` local actual es ese commit base, pero la rama `feat/native-overlay-roslyn-delta-20261030` tiene cambios y archivos untracked. **No** es un checkout limpio y no se debe presentar como source de una nueva release. Para la release futura quedan sin resolver, intencionalmente: `<SOURCE_SHA>`, `<TAG>` y `<VERSION>`.

## Matriz y contratos de artefactos

Fuentes leídas: `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`, `scripts/release/ae-release-binaries.ps1`, `scripts/release/build-dist.ps1`, `scripts/release/build-workers.ps1`, `scripts/release/verify-worker-bundles.ps1`, `scripts/release/worker-manifest.ps1`, `scripts/release/platform-mapping.sh`, `.goreleaser.yaml` y `scripts/release.sh`.

- Seis RID lógicos de CLI y workers: `win-arm64`, `win-x64`, `linux-arm64`, `linux-x64`, `osx-arm64`, `osx-x64`.
- Dos RIDs adicionales del ejecutable host: `win-x64`, `win-arm64`. Por tanto, el GoReleaser configura 8 build tuples en total (6 CLI + 2 host), mientras el worker .NET se empaqueta para los seis RIDs lógicos.
- Los assets Darwin se nombran `darwin-arm64` / `darwin-x64`; dentro del bundle el worker conserva `workers/osx-arm64` / `workers/osx-x64`.
- Cada worker lleva `worker-manifest.json`: schema `mi-lsp-worker-manifest/v1`, protocolo `mi-lsp-v1.1`, RID, lista/cantidad de archivos, tamaños y SHA-256. El verificador exige el allowlist exacto y puede hacer probe del worker compatible con el host.
- `.goreleaser.yaml` crea los 6 CLI, 2 host Windows y checksums SHA-256. El build de workers se hace en el hook con `pwsh ... build-workers.ps1`; `scripts/release.sh` además corre el guard `MI_LSP_RELEASE=1 ... release-platform-mapping.sh`, `dotnet build ... -c Release`, GoReleaser y materialización/verificación de los nueve nombres esperados.
- La firma Authenticode/code-signing está diferida en el contrato (SEC-11 / FD3); checksums no equivalen a firma.

### Comando de empaquetado futuro (plantilla, NO ejecutar ahora)

El mismo build cross-platform puede prepararse desde Tesla o VPS en un checkout separado, limpio y fijado al SHA que finalmente se acepte. Reemplazar los tres placeholders solo cuando exista el binding real:

```bash
SOURCE_SHA="<SOURCE_SHA>"
TAG="<TAG>"
VERSION="<VERSION>" # semver sin prefijo v

test "$TAG" = "v$VERSION"
test "$(git rev-parse HEAD)" = "$SOURCE_SHA"
test "$(git rev-parse "${TAG}^{commit}")" = "$SOURCE_SHA"
test -z "$(git status --porcelain)"
MI_LSP_ROOT="$PWD" sh scripts/release.sh "$VERSION"
```

El comando sin `--upload` solo prepara `dist/`; no crea tag, no sube release ni instala/activa el CLI. Si GoReleaser no está disponible, `scripts/release.sh` instala la versión fijada `v2.13.1` mediante `go install`. El script sí ejecuta el guard de plataforma, `dotnet build`, build de workers/CLI/host y validación/materialización de assets; nada de eso se ejecutó en esta preparación. La salida de Tesla y VPS debe permanecer separada por checkout/`dist` hasta comparar SHA y checksums.

## Entornos observados (solo lectura)

- Tesla: `tesla-desktop`, usuario `tesla`, UID `1000`, Linux x86_64. Encontrados `pwsh`, `dotnet`, `go`, `python`; `goreleaser` no está en PATH.
- VPS solicitado: host `tesla-vps-ThinkPad-T14`, usuario `tesla`, UID `1000`, Linux x86_64. SSH de solo lectura encontró `pwsh`, `dotnet`, `go`, `python`; `goreleaser` no está en PATH.
- CLI local Tesla: wrapper `/home/tesla/.local/bin/mi-lsp`; ejecutable `/home/tesla/.local/share/mi-lsp-runtime/versions/v0.16.6/linux-x64/mi-lsp`. `mi-lsp version` reportó versión `0.16.7`, revisión `efb88b3041f6d1100d272e5e3e84fb8f8eb95db1`, `vcs_modified=false`. El directorio rotulado `v0.16.6` no coincide con la versión reportada; no se usó como artefacto de build ni se modificó.

## Discrepancias y límites de publicación

- El doc/script local dicen que `-Publish -Tag` empuja el tag y activa `.github/workflows/release.yml`; sin embargo, ese archivo no existe en este checkout y la API de `main` también devolvió 404 para la ruta; el endpoint de workflows no mostró workflows. La release v0.16.7 sí existe, pero el mecanismo actual de publicación no queda confirmado por estas fuentes. No ejecutar `-Publish` ni `--upload` hasta resolver esta discrepancia con el flujo vigente.
- El contrato `Graph-Native Release Gate` en `AE-RELEASE-DISTRIBUTION.md` declara publicación prohibida hasta G10 `ae-close` aprobado. La tarea de perfil/wiring descrita en la sesión conserva su revisión C0 independiente en curso; este informe no la declara aceptada ni sustituye esa evidencia. La preparación de builds es independiente y no espera una respuesta de Yoga.

## Siguiente acción

Continuar el cierre independiente de source/doc profile wiring. Cuando exista source SHA limpio y aceptado, volver a comprobar el tag previsto y los nombres/checksums de release; antes de publicar, resolver la discrepancia del workflow y cumplir el gate Graph-Native/G10. Mantener `<SOURCE_SHA>`, `<TAG>` y `<VERSION>` como placeholders hasta entonces.

## Paths

- Creado: `RELEASE-DISTRIBUTION-PREP-RESULTADO-20261030.md`
- Sin cambios de código, scripts, instalaciones ni archivos de host.

REPORTE-FINAL: /home/tesla/repos/mios-wt/mi-lsp-overlay-roslyn-delta-20261030/RELEASE-DISTRIBUTION-PREP-RESULTADO-20261030.md
LISTO
