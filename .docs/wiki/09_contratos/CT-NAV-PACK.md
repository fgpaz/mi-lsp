# CT-NAV-PACK

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav pack`

## Forma de invocacion

```text
mi-lsp nav pack <task> [--workspace <alias>] [--rf <id>] [--fl <id>] [--doc <path>] [--full]
```

La CLI acepta una tarea libre y produce un envelope `backend=pack`.
El comando es repo-local directo y no requiere daemon.
El envelope puede agregar `continuation` y `memory_pointer` para sostener la reentrada con pocos tokens.

## Payload logico

- `task`: string requerido
- `rf`: anchor opcional
- `fl`: anchor opcional
- `doc`: anchor opcional
- `workspace`: alias o path resoluble
- `max_items`, `token_budget`, `max_chars`: limites usuales del envelope

Cuando `workspace` se omite, el runtime resuelve primero el workspace registrado cuyo root contiene el `caller_cwd` real del invocador. Solo si no hay match puede caer a `last_workspace`, y ese caso debe quedar visible en `warnings`.

## Respuesta

Cada item de `backend=pack` contiene:
- `task`
- `family`
- `mode` (`preview|full`)
- `primary_doc`
- `docs[]`
- `why`
- `next_queries`

Cada `docs[]` contiene:
- `path`
- `title`
- `doc_id`
- `layer`
- `stage`
- `why`
- `targets`
- `slice_text` y rango de lineas solo en modo `full`

El envelope puede contener ademas:
- `continuation.reason`
- `continuation.next { op, query?, repo?, path?, symbol?, doc_id?, full? }`
- `continuation.alternate { ... }`
- `memory_pointer.doc_id`
- `memory_pointer.why`
- `memory_pointer.reentry_op`
- `memory_pointer.handoff`
- `memory_pointer.stale`

## Semantica observable

- `nav pack` entrega un reading pack canonico, no una respuesta textual explainable como `nav ask`.
- El orden del pack va de lo mas global a lo mas especifico segun familia documental y perfil local.
- El anchor del pack sale del mismo scorer owner-aware compartido con `nav route` y `nav ask`; `owner_hints` opcionales pueden sesgar ownership repo-especifico cuando no hay override explicito.
- Cuando la wiki canonica existe pero el indice documental esta vacio, el contrato devuelve warnings/hint accionables para reindexar.
- `--full` expande slices del mismo pack y no cambia `backend`.
- si varios aliases registrados comparten el mismo root, la seleccion automatica usa `project.name`, luego basename del root y deja warning visible con el alias elegido.
- en AXI preview, el contrato puede resumirse a `anchor + 2 docs` reutilizando `route core` para bajar latencia y tokens
- en AXI preview, `continuation` puede sugerir la expansion a `--full` y omite `alternate`
- Si ya existe un doc canonico positivo para la tarea, `README` y otros docs `generic` no pueden convertirse en `primary_doc`.

## Routing interno

`nav pack` usa `resolveCanonicalRoute` (RF-QRY-015) como backbone para determinar el anchor documental cuando no hay override explicito.

Precedencia de seleccion de anchor:

1. `--doc <path>` — path explicito en el payload (maxima prioridad)
2. `--rf <id>` — id de RF como anchor
3. `--fl <id>` — id de FL como anchor
4. Route core — `resolveCanonicalRoute` determina el anchor canonico segun la tarea y el perfil del workspace

Cuando el route core determina el anchor, el campo `why` del resultado incluye `"tier2=route_core"`.
