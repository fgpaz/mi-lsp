# Trazabilidad y auditoria - mi-lsp next wave

Fecha: 2026-05-11
Rama: `feature/daemon-go-ergonomics-wave`
Workspace: `C:\repos\mios\mi-lsp`

## Alcance ejecutado

- `daemon status` detecta daemon/binario stale con metadata de ejecutable (`path`, `size`, `mtime`, `sha256`) y guidance `mi-lsp daemon restart`.
- `gopls` queda integrado como backend Go opcional via LSP generico para `nav context`/refs, con fallback catalog/text si no esta instalado.
- `nav service` es language-aware para paquetes Go: detecta `net/http`, routers tipo chi/gin/fiber, Cobra y senales worker, evitando patrones .NET y falsos positivos de tests/fixtures/literales.
- `nav context` acepta `file.go:123` y conserva el formato anterior `file.go 123`, con guidance corregida ante sintaxis invalida.
- Lecturas SQLite documentales agregan retry/backoff breve ante locks transitorios sin ocultar locks persistentes.

## Sincronizacion documental

- `.docs/wiki/06_pruebas/TP-DAE.md`
- `.docs/wiki/06_pruebas/TP-QRY.md`
- `.docs/wiki/07_baseline_tecnica.md`
- `.docs/wiki/07_tech/TECH-SERVICE-EXPLORATION.md`
- `.docs/wiki/08_modelo_fisico_datos.md`
- `.docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md`
- `.docs/wiki/09_contratos_tecnicos.md`

## Evidencia

| Comando | Resultado |
|---|---|
| `go test ./...` | PASS |
| `go run ./cmd/mi-lsp daemon restart --format toon` | PASS |
| `go run ./cmd/mi-lsp daemon status --format toon --max-chars 1200` | PASS, sin warning stale despues de restart con mismo hash |
| `go run ./cmd/mi-lsp index --workspace mi-lsp --clean --format toon` | PASS, `files=178`, `symbols=2027`, `docs=85` |
| `go run ./cmd/mi-lsp workspace status mi-lsp --format toon` | PASS, `governance_blocked=false`, `governance_sync=in_sync`, `index_ready=true` |
| `go run ./cmd/mi-lsp nav governance --workspace mi-lsp --format toon` | PASS, `blocked=false`, `sync=in_sync` |
| `go run ./cmd/mi-lsp nav workspace-map --workspace mi-lsp --axi --full --format toon` | PASS, `services=16`, `total_symbols=2053` |
| `go run ./cmd/mi-lsp nav service internal/service --workspace mi-lsp --format toon` | PASS, `profile=go-package`, sin endpoint falso desde tests/raw strings |
| `go run ./cmd/mi-lsp nav context internal/service/context.go:42 --workspace mi-lsp --format toon --max-chars 500` | PASS inicial con fallback catalog cuando `gopls` aun no estaba instalado localmente |
| `go run ./cmd/mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon` | PASS, `harness_verdict=PASS`, `harness_contracts_reviewed=84` antes de este doc |
| `go run ./cmd/mi-lsp nav wiki validate-source --workspace mi-lsp --format toon --full` | PASS, `wiki_source_verdict=PASS`, `navigation_readiness=ready` |
| `git diff --check` | PASS; solo warnings CRLF de Git |

## Distribucion local y mirror

| Superficie | Resultado |
|---|---|
| `skills/mi-lsp/*` | Actualizado con `nav context <file>:<line>`, `gopls`, `go-package` y `executable_sha256` |
| `C:\Users\fgpaz\.agents\skills\mi-lsp` | Sincronizado desde `skills/mi-lsp`; `fc /B SKILL.md` PASS |
| `C:\repos\buho\assets\skills\mi-lsp` | Sincronizado desde `skills/mi-lsp`; `fc /B SKILL.md` PASS |
| `pwsh -NoProfile -Command "& { ./scripts/release/build-dist.ps1 -Rids @('win-arm64','win-x64','linux-x64') -Clean }"` | PASS |
| `pwsh -NoProfile -File scripts/release/install-local.ps1 -Rid win-arm64 -InstallDir C:\Users\fgpaz\bin -OutDir .\dist -SkipBuild` | PASS |
| `C:\Users\fgpaz\bin\mi-lsp.exe daemon restart --format toon` | PASS, daemon corre desde `C:\Users\fgpaz\bin\mi-lsp.exe` con `executable_sha256=3628bc536b7af29946b2e46633d1cde78dc65ff9a76e00d486d1d0903e98d78f` |
| `C:\Users\fgpaz\bin\mi-lsp.exe nav context internal/service/context.go:42 --workspace mi-lsp --format toon --max-chars 1200` | PASS posterior con `backend: gopls` tras instalar `gopls v0.21.1` en `C:\Users\fgpaz\bin\gopls.exe` |
| `C:\Users\fgpaz\bin\mi-lsp.exe worker status --format toon` | PASS, `selected_source=installed`, `selected_compatible=true`, `protocol_version=mi-lsp-v1.1` |
| `go version -m C:\Users\fgpaz\bin\mi-lsp.exe` | PASS, `GOOS=windows`, `GOARCH=arm64`, `vcs.revision=b63b17493e54eee7fea41553360a7defc9dbda86`, `vcs.modified=true` |
| `go version -m C:\Users\fgpaz\.agents\skills\mi-lsp\bin\mi-lsp-win-x64.exe` | PASS, `GOOS=windows`, `GOARCH=amd64`, misma revision dirty |
| `go version -m C:\repos\buho\assets\skills\mi-lsp\bin\mi-lsp-win-x64.exe` | PASS, `GOOS=windows`, `GOARCH=amd64`, misma revision dirty |
| `go version -m C:\repos\buho\assets\skills\mi-lsp\bin\mi-lsp-linux-x64` | PASS, `GOOS=linux`, `GOARCH=amd64`, misma revision dirty |

## Auditoria

- Gobernanza inicial y final: PASS, no bloqueada, `read-model.toml` en sync.
- Trazabilidad docs-code: PASS, cambios con docs 07/08/09 y TP actualizados.
- Riesgo residual: primer arranque de `gopls` puede tardar mientras prepara cache, pero la ruta real ya fue probada con `backend: gopls` y el fallback catalog/text queda cubierto por tests.
- Riesgo residual: `nav service` Go sigue siendo evidencia heuristica, no score de completitud.
