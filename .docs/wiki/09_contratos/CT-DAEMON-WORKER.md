# CT-DAEMON-WORKER

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: "CT-DAEMON-WORKER"
id: "CT-DAEMON-WORKER"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-DAEMON-WORKER]]'
exports:
  - 'CT-DAEMON-WORKER'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
```

Volver a [09_contratos_tecnicos.md](../09_contratos_tecnicos.md).

## Summary

Define el contrato interno entre core/daemon y los backends semanticos, con foco en versionado, framing, seleccion de runtime y reglas de degradacion.

## Boundary and owner

- Boundary: daemon/core -> worker Roslyn, runtime `tsserver`, `pyright-langserver` o `gopls`
- Owner logico: Semantic backends
- Scope: framing, envelopes, lifecycle, payloads derivados y bootstrap de runtimes

## Contract family inventory

### Framing canonico

Roslyn worker:

- transporte por `stdio`
- mensajes JSON con length-prefix
- request envelope actual:
  - `protocol_version`
  - `method`
  - `workspace`
  - `backend_type`
  - `payload`
- response envelope actual:
  - `ok`
  - `backend`
  - `items`
  - `warnings`
  - `error`
  - `stats`

TS semantic bridge:

- transporte via `node <tsserver.js>`
- framing `Content-Length` propio de `tsserver`
- sin handshake separado de `mi-lsp`; el bridge opera request/response por comando

Pyright y Go LSP bridge:

- transporte por `stdio` hacia `pyright-langserver` o `gopls`
- framing LSP JSON-RPC 2.0 con `Content-Length`
- lifecycle y documentos abiertos gestionados por el cliente LSP generico

## Backends soportados

- `roslyn`
- `tsserver`
- `pyright`
- `gopls`

## Operaciones minimas actuales

Roslyn:

- `find_symbol`
- `find_refs`
- `get_overview`
- `get_context`
- `get_deps`
- `status`

TS semantic bridge:

- `get_context`
- `find_refs`

Pyright y gopls LSP bridge:

- `get_context` (via `textDocument/hover`)
- `find_refs` (via `textDocument/references`)

## Ciclo CLI-daemon y deadlines

```toon
doc_id: CT-DAEMON-WORKER
block_id: CT-DAEMON-WORKER.request-lifecycle
kind: request-lifecycle-contract
source_of_truth: this
verify:
  - go test ./internal/daemon/... ./internal/service/...
evidence:
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
  - internal/daemon/client.go
routing:
  no_daemon:
    flag: --no-daemon
    behavior: no_connect_and_no_start
  no_auto_daemon:
    flag: --no-auto-daemon
    behavior: skip_auto_start_only
    existing_daemon: connect_allowed
  execute:
    method: ExecuteWithDialTimeout
    short_timeout_scope: dial_only
    after_dial: original_context
    operations: [write, read, response_processing, cancellation]
```

`--no-daemon` fuerza la ejecucion local y no conecta ni inicia el daemon. `--no-auto-daemon` solo omite el auto-start; si la operacion usa daemon, aun puede conectar a una instancia existente. `ExecuteWithDialTimeout` deriva un contexto breve para establecer la conexion y lo libera al terminar el dial; write, read, procesamiento de respuesta y cancelacion vuelven a obedecer el contexto original de la request.

## Lock de arranque y frontera de observacion graph-native

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-DAEMON-WORKER
block_id: CT-DAEMON-WORKER.start-lock-and-graph-observation
kind: daemon-worker-contract
audience: llm-first
source_of_truth: this
imports:
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
  - .docs/wiki/07_tech/TECH-DEPENDENCY-HARDENING.md
  - .docs/wiki/06_pruebas/TP-GPH.md
exports:
  - daemon_start_lock_contract
  - graph_observation_worker_boundary
evidence:
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
  - internal/daemon/state_store.go
  - internal/daemon/start_lock.go
  - internal/daemon/start_lock_windows.go
  - internal/daemon/start_lock_unix.go
  - internal/daemon/process_liveness_windows.go
  - internal/service/graph_observer.go
  - internal/model/graph_observation.go
  - worker-dotnet/MiLsp.Worker/RoslynService.cs
verify:
  - go test ./internal/daemon/... ./internal/service/... ./internal/model/... ./internal/indexer/...
  - dotnet run --project worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj
stop_if:
  - live_owner_lock_reclaimed=true
  - unknown_metadata_lock_reclaimed=true
  - ambiguous_windows_liveness_reclaimed=true
  - start_guard_removed=true
  - start_lock_operation_outside_guard=true
  - lock_owner_file_replaced=true
  - noncanonical_batch_accepted=true
  - batch_not_ready_for_staging=true
start_lock:
  guard:
    path: start.guard
    persistence: persistent_never_removed
    os_exclusive_lock:
      windows: LockFileEx
      unix: flock
    serializes: [create, inspect, reclaim, Close]
  create:
    flags: O_CREATE|O_EXCL|O_RDWR
    mode: 0600
    metadata:
      version: 1
      fields: [pid, nonce]
      descriptor_close: after_versioned_metadata_sync
  pid:
    valid_range: 1..math.MaxInt32
  owner:
    live: preserve
    dead: reclaim
    metadata_unknown: preserve
    errors: fail_closed
    windows_liveness:
      ERROR_INVALID_PARAMETER: nonexistent
      ACCESS_DENIED: alive
      ambiguous_error: alive
      exit_code_error: alive
  legacy_empty:
    reclaim_only_after: 5m
  close:
    under_guard: true
    remove_only_when_pid_and_nonce_match: true
    replacement_with_different_metadata: preserve
graph_observation:
  worker_output: canonical_unsealed_batch
  core_sequence: [ValidateCanonical, SealGraphObservationBatch, ReadyForStaging]
```

`start.guard` es persistente y nunca se elimina. Su lock exclusivo del sistema operativo (`LockFileEx` en Windows, `flock` en Unix) cubre toda operación de crear, inspeccionar, recuperar y `Close`; por ello no se libera el guard entre recuperar `start.lock` y crear su reemplazo. `start.lock` conserva `O_CREATE|O_EXCL`, se escribe con metadata versionada `pid+nonce`, se sincroniza y solo entonces se cierra el descriptor.

Un PID válido está en `1..math.MaxInt32`. Un owner vivo, una metadata desconocida o cualquier error de liveness ambiguo se preservan; en Windows, `ACCESS_DENIED` y todo error distinto de `ERROR_INVALID_PARAMETER` se tratan como owner vivo, y únicamente `ERROR_INVALID_PARAMETER` prueba inexistencia. Un lock legacy vacío solo se recupera cuando tiene más de cinco minutos. `Close` actúa bajo `start.guard` y elimina únicamente si coinciden el PID y el nonce que adquirieron el lock; los errores son fail-closed.

## Bootstrap y seleccion de runtime

- Para Roslyn, el caller debe resolver el `tool_root` desde el ejecutable/distribucion actual o, en desarrollo, desde el repo `mi-lsp`.
- Orden canonico de candidatos Roslyn para queries: `bundle -> installed -> dev-local`, resuelto por presencia de archivos y sin probe de compatibilidad en el hot path.
- Compatibilidad minima se valida mediante probe `status` y comparacion de `protocol_version`; ese probe vive en `worker status` y diagnostico explicito.
- Si el primer candidato Roslyn falla por bootstrap/arranque, el caller puede reintentar una sola vez con el siguiente candidato determinista antes de devolver error accionable.
- `gopls` es opcional y se resuelve primero desde `PATH`, luego desde `bin/gopls(.exe)` o `.bin/gopls(.exe)` dentro del repo; su ausencia no invalida el catalogo AST nativo de Go.
- Los procesos hijo no interactivos del worker y del bridge semantico deben usar la politica comun de proceso; en Windows eso significa `HideWindow + CREATE_NO_WINDOW`, y los procesos detached agregan `DETACHED_PROCESS`.
- El runtime pool debe respetar presupuesto de memoria/proceso observable por daemon: working set, private bytes, handles y cantidad de runtimes activos son parte del contrato de `daemon status`/`perf-smoke`.
- El worker instalado por RID vive en `~/.mi-lsp/workers/<rid>/`.
- Un repo de desarrollo no debe tratar `bin/workers/<rid>` como bundle de distribucion canonico para consultas regulares.

## Payload, error y compatibilidad

- Los payloads son siempre derivados; no enviar ASTs ni blobs completos por defecto.
- Toda respuesta debe poder incluir:
  - `warnings`
  - `stats`
  - `backend`
- Los errores del worker deben mapearse a mensajes accionables para el CLI.
- Si `tsserver` falla o no existe, el caller debe poder degradar a catalog/text con warning.
- Si `pyright` falla o no existe, el caller debe poder degradar a catalog/text con warning.
- Si `gopls` falla o no existe, el caller debe conservar el catalogo AST nativo de Go y degradar a catalog/text con warning.
- Si Roslyn falla en `get_context` y el archivo existe, el caller debe preservar `slice_text` y degradar a `catalog` o `text` con warning accionable.
- Si Roslyn falla en operaciones sin fallback util, como `find_refs` o `get_deps`, el caller debe devolver error tipado y accionable.
- Si el SLO de memoria/proceso se excede durante `perf-smoke`, el envelope debe ser `ok=false` con `error.kind=daemon` o `backend_runtime`, `error.code` estable y warning accionable; no debe esconderse como warning benigno.

## Versioning y deprecacion

- El protocolo CLI-daemon esta versionado por `protocol_version`.
- El protocolo daemon-worker tambien transporta `protocol_version` por request para cortar incompatibilidades temprano, aun sin handshake separado de sesion.
- Agregar campos es compatible si el receptor ignora desconocidos.
- Cambiar framing o envelopes base requiere actualizar la documentacion de contrato y el parser correspondiente.

## Related docs

- [TECH-TS-BACKEND.md](../07_tech/TECH-TS-BACKEND.md)
- [TECH-DEPENDENCY-HARDENING.md](../07_tech/TECH-DEPENDENCY-HARDENING.md)
- [CT-CLI-DAEMON-ADMIN.md](CT-CLI-DAEMON-ADMIN.md)
