# Veredicto — reparación del instalador shell de mi-lsp

- **Fecha UTC:** 2026-07-26T19:04:14Z
- **Estado:** PASS
- **Branch:** `fix/install-archive-path`
- **Disposición autorizada:** `integrate-main`
- **Tracker:** waiver FAST; Gabriel autorizó actualizar, publicar e instalar todo el stack en Telegram.

## Hallazgo

`validate_tar_archive()` asignaba `archive="$1"` en el mismo entorno POSIX del script. Esa asignación reemplazaba el nombre de asset global por su ruta absoluta y la extracción posterior ejecutaba `tar` contra `$tmp/$archive`, duplicando el directorio temporal.

## Cambio

- Ejecutar `validate_tar_archive()` en un subshell para aislar sus variables.
- Extender `scripts/tests/install-platform-mapping.sh` con una release offline completa y portable (`sha256sum` o `shasum`) que descarga, valida, extrae e instala CLI + worker.

## Evidencia

- RED antes del fix: `sh scripts/tests/install-platform-mapping.sh` → exit 1 por ruta temporal duplicada.
- GREEN después del fix: `sh scripts/tests/install-platform-mapping.sh` → `PASS: platform mapping, pre-network validation, and offline archive installation`.
- `MI_LSP_ALLOW_NO_PWSH=1 sh scripts/tests/release-platform-mapping.sh` → PASS.
- `sh -n scripts/install/install.sh` y `sh -n scripts/tests/install-platform-mapping.sh` → PASS.
- `git diff --check` → PASS.
- `go test ./...` → PASS.
- `go test -race ./...` → PASS, incluidos `internal/service` (403.457 s) e `internal/store` (510.383 s).
- `go vet ./...` → PASS.
- `dotnet build worker-dotnet/MiLsp.Worker.sln -c Release` → PASS, 0 warnings, 0 errors.
- Worker contract tests → `PASS graph observation provenance contract`.
- Harness-first Python: 82 tests → OK.
- Target preflight `operation=read` → PASS; revalidación READ→DISPATCH reprodujo el bug conocido `ae_work_invoked_before_execution` stale; fallback seguro `operation=dispatch` fresco con `--ae-work-invoked-before-execution` → PASS. El bug queda owned por la lane de `ae-kernel`.
- Instalación real desde el script reparado sobre la release `v0.5.18` → PASS.
- PATH resuelve `/home/fgpaz/.local/bin/mi-lsp` a `/home/fgpaz/.local/opt/mi-lsp/mi-lsp`.
- CLI instalada: `v0.5.18`, protocolo `mi-lsp-v1.1`, SHA-256 `8969afe3a1328a5c0a3669aca5cba4871607d406aa62937467aa763d7b72cfe1`.
- Worker bundled/global: compatible; SHA-256 global `d69286b44062e04a8b93d76ab820f07331a189bb04dede5c13e47e69e9b967c4`.
- Ciclo real `daemon stop/start/status` → daemon `v0.5.18` activo y consulta `workspace status` exit 0.

## Riesgo residual

La matriz macOS/Windows queda a cargo de GitHub Actions. El test nuevo evita depender de `sha256sum` en macOS.
