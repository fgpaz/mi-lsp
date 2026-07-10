# Wave 5 — mi-lsp routing, telemetría y release

Plan coordinador: `C:/wt/mi-pi-wave5/.docs/raw/plans/2026-07-09-wave5-full-optimization.md`.

## Alcance mi-lsp

1. Tomar baseline de `admin export` por operación, cliente, hint, failure stage y route outcome.
2. Convertir los errores históricos en casos reproducibles; no optimizar solo por conteo agregado.
3. Reparar los top root causes acotados de `nav.route`, `nav.edit-plan`, `workspace.status` e `index.start`.
4. Añadir tests y telemetría que distingan error real, warning/hint esperado y fallback válido.
5. Ejecutar carga concurrente/benchmark de routing y comparar p50/p95/p99, error rate y warnings.
6. Construir los cuatro RIDs, verificar worker bundle/checksums/installers/mirrors y preparar `v0.5.11`.
7. Publicar únicamente tras auditoría, pre-push y CI verdes, mediante gate irreversible explícito.

## Archivos candidatos

- `internal/cli/**`, `internal/nav/**`, `internal/service/**`, `internal/telemetry/**`, `internal/workspace/**`
- tests asociados
- `.github/workflows/**`, `.goreleaser.yml`, `scripts/release/**`, `scripts/install/**`
- `.docs/wiki/07*`, `.docs/wiki/09*`, `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`

## Verificación

- `go build ./...`
- `go vet ./...`
- `go test ./...`
- admin-export baseline vs final
- benchmark concurrente
- `scripts/ae/pre-push-guard.ps1`
- build/install/checksum de win-arm64, win-x64, linux-arm64, linux-x64
- PR CI Windows/Linux

## Stop conditions

- error histórico sin reproducción local
- reducción de warnings mediante ocultamiento de información útil
- publish con worktree sucio, CI rojo o tag no apuntando a HEAD
