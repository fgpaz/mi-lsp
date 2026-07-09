# Task T4: Daemon-first para `nav.ask` / `nav.search` / `nav.pack` con fallback directo

## Shared Context
**Goal:** Que las queries docs-first más frecuentes sirvan desde el daemon caliente (DocRecords cacheados por generación) en vez de recargar el índice SQLite en cada invocación CLI.
**Stack:** Go, repo `C:/repos/mios/mi-lsp`.
**Architecture:** `shouldUseDaemon` en `internal/cli/root.go:431-451` excluye hoy `nav.find/search/pack/ask` del daemon; `executeOperation` (root.go:186-237) ya implementa fallback a directo si el daemon falla. El server daemon lista `nav.search`/`nav.find` en backpressure (`internal/daemon/server.go:417-426`), señal de que la capa de servicio ya puede ejecutarlas server-side.

## Locked Decisions
- **Forma exacta del cambio** (verificado 2026-07-09): `shouldUseDaemon` está en `root.go:431-441` y hoy EXCLUYE `nav.ask/search/pack` con un `return false` en la línea ~436 (lista de exclusión, no lista elegible). El cambio es: **remover `nav.ask`, `nav.search`, `nav.pack` de esa exclusión** para que queden daemon-elegibles. `shouldAutoStartDaemon` (root.go:443-452) NO se toca: estas ops usan daemon **solo si ya está vivo** (no auto-arrancan; una query barata no debe pagar el arranque). `nav.find` queda como está.
- El fallback directo existente debe quedar intacto: daemon caído/lento → misma respuesta por el camino actual. Ninguna regresión de contrato en el envelope (`backend:` puede reportar `daemon`).
- Si el dispatcher del daemon NO tiene handler para alguna de estas ops, implementarlo reusando la misma función de servicio que usa el camino directo (buscar el switch de ops en `internal/service/app.go:100-121`); si eso requiere rediseño de protocolo, STOP y reportar.
- Timeout cliente para estas ops vía daemon: el existente para ops daemon; no inventar uno nuevo.
- Tests: agregar/ajustar tests unitarios de `shouldUseDaemon` (existen tests en `internal/cli/`? localizarlos con `go test ./internal/cli/ -run Daemon -v`; si no existen para esta función, crear uno mínimo de tabla).
- No tocar el protocolo wire ni la versión `mi-lsp-v1.1`.

## Task Metadata
```yaml
id: T4
depends_on: []
agent_type: general-purpose
goal_id: G2
github_issues: []
expected_outcome: "nav ask/search/pack reusan el índice caliente del daemon cuando está vivo; sin daemon todo sigue igual"
files:
  - modify: C:/repos/mios/mi-lsp/internal/cli/root.go:431-451
  - read: C:/repos/mios/mi-lsp/internal/daemon/server.go
  - read: C:/repos/mios/mi-lsp/internal/service/app.go:100-121
complexity: medium
done_when:
  - "go build ./... exit 0"
  - "go test ./internal/... exit 0"
  - "con daemon vivo: mi-lsp nav ask \"how is this workspace organized?\" --workspace mi-lsp --format toon responde y evidencia backend daemon (o log); con daemon detenido responde igual por directo"
evidence_expected:
  - "Salidas de build/test + las dos corridas de nav ask (daemon vivo / detenido)"
stop_if:
  - "el daemon no puede ejecutar estas ops sin rediseño de protocolo"
  - "cualquier test preexistente falla por el cambio"
```

## Reference
`internal/cli/root.go:186-237` (executeOperation + fallback), `:431-451` (shouldUseDaemon/shouldAutoStartDaemon), `internal/daemon/server.go:417-426` (backpressure list).

## Prompt
Protocolo de despacho del plan aplica: primero 1-2 micro-lanes pi read-only (`pi-worker-launch.mjs --workspace C:/repos/mios/mi-lsp`) para confirmar (a) qué ops maneja hoy el dispatcher del daemon y (b) la forma exacta de `shouldUseDaemon`. Con eso, aplicá el cambio mínimo: extender el set daemon-elegible sin habilitar auto-start para estas ops. Si falta handler server-side, agregalo reusando la capa `internal/service` exactamente como las ops ya servidas. Escribí el test de tabla para `shouldUseDaemon` cubriendo: op daemon-elegible con daemon disponible, op cheap-read, y las 3 ops nuevas con y sin auto-start. Cuidado con la nota conocida: si probás una op nueva contra el daemon global viejo puede dar "unknown operation" — reiniciá el daemon local (`mi-lsp daemon stop && mi-lsp daemon start`) antes del smoke.

## Execution Procedure
1. Lanes pi read-only de confirmación de anclajes.
2. Editá `internal/cli/root.go` (y `internal/daemon/server.go` solo si falta handler).
3. `go build ./...` y `go test ./internal/...`.
4. Smoke: `go run ./cmd/mi-lsp nav ask ... --format toon` con daemon vivo y detenido (usar binario recién buildeado, no el instalado).
5. NO commitees. Reportá diffs + salidas + verdicts de lanes pi.

## Skeleton
```go
var daemonPreferredOps = map[string]bool{ /* existentes */, "nav.ask": true, "nav.search": true, "nav.pack": true }
var daemonAutoStartOps = map[string]bool{ /* sin cambios: ask/search/pack NO auto-arrancan */ }
```

## Verify
`cd C:/repos/mios/mi-lsp && go build ./... && go test ./internal/...` → exit 0

## Commit
`perf(daemon): route nav.ask/search/pack through live daemon with direct fallback`
