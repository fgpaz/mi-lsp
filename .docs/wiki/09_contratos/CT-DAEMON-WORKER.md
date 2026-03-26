# CT-DAEMON-WORKER

Volver a [09_contratos_tecnicos.md](../09_contratos_tecnicos.md).

## Summary

Define el contrato interno entre core/daemon y los backends semanticos, con foco en versionado, framing, seleccion de runtime y reglas de degradacion.

## Boundary and owner

- Boundary: daemon/core -> worker Roslyn, runtime `tsserver`, o `pyright-langserver`
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

## Backends soportados

- `roslyn`
- `tsserver`
- `pyright`

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

Pyright LSP bridge:

- `get_context` (via `textDocument/hover`)
- `find_refs` (via `textDocument/references`)

## Bootstrap y seleccion de runtime

- Para Roslyn, el caller debe resolver el `tool_root` desde el ejecutable/distribucion actual o, en desarrollo, desde el repo `mi-lsp`.
- Orden canonico de candidatos Roslyn para queries: `bundle -> installed -> dev-local`, resuelto por presencia de archivos y sin probe de compatibilidad en el hot path.
- Compatibilidad minima se valida mediante probe `status` y comparacion de `protocol_version`; ese probe vive en `worker status` y diagnostico explicito.
- Si el primer candidato Roslyn falla por bootstrap/arranque, el caller puede reintentar una sola vez con el siguiente candidato determinista antes de devolver error accionable.
- Los procesos hijo no interactivos del worker y del bridge semantico deben usar la politica comun de proceso; en Windows eso significa `HideWindow + CREATE_NO_WINDOW`, y los procesos detached agregan `DETACHED_PROCESS`.
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
- Si Roslyn falla en `get_context` y el archivo existe, el caller debe preservar `slice_text` y degradar a `catalog` o `text` con warning accionable.
- Si Roslyn falla en operaciones sin fallback util, como `find_refs` o `get_deps`, el caller debe devolver error tipado y accionable.

## Versioning y deprecacion

- El protocolo CLI-daemon esta versionado por `protocol_version`.
- El protocolo daemon-worker tambien transporta `protocol_version` por request para cortar incompatibilidades temprano, aun sin handshake separado de sesion.
- Agregar campos es compatible si el receptor ignora desconocidos.
- Cambiar framing o envelopes base requiere actualizar la documentacion de contrato y el parser correspondiente.

## Related docs

- [TECH-TS-BACKEND.md](../07_tech/TECH-TS-BACKEND.md)
- [TECH-DEPENDENCY-HARDENING.md](../07_tech/TECH-DEPENDENCY-HARDENING.md)
- [CT-CLI-DAEMON-ADMIN.md](CT-CLI-DAEMON-ADMIN.md)
