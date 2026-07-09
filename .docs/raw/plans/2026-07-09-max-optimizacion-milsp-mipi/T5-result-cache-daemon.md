# Task T5: Result cache en el daemon por (workspace, generación, op, query-hash)

## Shared Context
**Goal:** Que queries repetidas (los seeds/context de N workers son casi idénticos) devuelvan resultado cacheado desde el daemon sin recomputar FTS/scoring.
**Stack:** Go, repo `C:/repos/mios/mi-lsp`, paquete `internal/daemon`.
**Architecture:** El daemon ya cachea DocRecords/FTS por generación intra-proceso (`internal/service/doc_query_context.go:58-76`) y tiene un failureCache (patrón a imitar: `internal/daemon/lifecycle.go:22-74`). No existe cache de RESULTADOS de operación.

## Locked Decisions
- Nuevo archivo `internal/daemon/result_cache.go`: LRU acotado (map + lista, o slice con timestamps) de **256 entradas máx**, TTL **10 min**, key = `sha256(workspace_root + "\x00" + index_generation + "\x00" + op + "\x00" + canonical_args_json)`.
- Solo cachear ops read-only deterministas: `nav.ask`, `nav.search`, `nav.pack`, `nav.governance`, `nav.route` (si existe como op), `workspace.status` NO (reporta estado vivo).
- `index_generation`: usar el mismo identificador de generación que ya usa el doc cache (localizarlo en `doc_query_context.go`); si la generación cambia, la key cambia sola — no hace falta invalidar.
- Guardar el envelope serializado final (bytes), no estructuras — evita aliasing.
- Métrica: contador hit/miss expuesto en el status del daemon (`system.status`) como `result_cache: {hits, misses, entries}`.
- Concurrencia: `sync.Mutex` simple (el QPS del daemon no justifica sharding).
- Wiring: en el punto del server donde se despacha la operación (antes de ejecutar, después de resolver workspace + generación). Si la generación no está disponible barato en ese punto, calcularla con el mtime del index.db del workspace (stat, no query).
- Flag de escape: `MI_LSP_DAEMON_RESULT_CACHE=0` lo desactiva.
- Test unitario del cache (put/get, TTL expiry, LRU eviction, key changes con generación).

## Task Metadata
```yaml
id: T5
depends_on: []
agent_type: general-purpose
goal_id: G2
github_issues: []
expected_outcome: "Segunda ejecución de la misma query vía daemon responde desde cache (hit contabilizado)"
files:
  - create: C:/repos/mios/mi-lsp/internal/daemon/result_cache.go
  - create: C:/repos/mios/mi-lsp/internal/daemon/result_cache_test.go
  - modify: C:/repos/mios/mi-lsp/internal/daemon/server.go
complexity: medium
done_when:
  - "go build ./... && go test ./internal/daemon/... exit 0"
  - "dos nav ask idénticas vía daemon: la segunda incrementa hits en system.status"
evidence_expected:
  - "Salida de tests + status del daemon mostrando hits>=1 tras query repetida"
stop_if:
  - "el punto de despacho del server no expone workspace/op/args de forma utilizable sin refactor grande"
```

## Reference
`internal/daemon/lifecycle.go:22-74` (failureCache — patrón de cache existente en el paquete), `internal/service/doc_query_context.go:58-76` (generación).

## Prompt
Protocolo de despacho del plan aplica: micro-lane pi read-only para extraer (a) la forma exacta del dispatch en `server.go` (dónde se conoce op+args+workspace) y (b) cómo se obtiene la generación del índice. Después implementá `result_cache.go` + wiring + test según Locked Decisions. Mantené el estilo del paquete (mirá failureCache). El cache envuelve la ejecución: `if bytes, ok := cache.Get(key) { return bytes }` / `bytes := execute(); cache.Put(key, bytes); return bytes`. No cachear respuestas de error.

## Execution Procedure
1. Lane pi read-only de anclajes (server dispatch + generación).
2. Creá `result_cache.go` y su test; wiring mínimo en `server.go`.
3. `go build ./... && go test ./internal/daemon/...`.
4. Smoke local: daemon fresco (`go run ./cmd/mi-lsp daemon restart` o stop/start), dos `nav ask` idénticas, status con contadores.
5. NO commitees. Reportá diffs + salidas + verdicts de lanes.

## Skeleton
```go
type resultCache struct {
    mu      sync.Mutex
    entries map[string]*cacheEntry // + orden LRU
    maxEntries int; ttl time.Duration
    hits, misses atomic.Int64
}
func (c *resultCache) Get(key string) ([]byte, bool)
func (c *resultCache) Put(key string, payload []byte)
```

## Verify
`cd C:/repos/mios/mi-lsp && go build ./... && go test ./internal/daemon/...` → exit 0

## Commit
`perf(daemon): generation-keyed LRU result cache for read-only nav ops`
