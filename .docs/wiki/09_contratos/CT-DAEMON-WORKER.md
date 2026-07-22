# CT-DAEMON-WORKER

```yaml
harness_protocol: SDD-HARNESS-v1
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
