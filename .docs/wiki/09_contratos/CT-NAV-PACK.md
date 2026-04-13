# CT-NAV-PACK

## Boundary

Usuario/agente -> CLI publica `mi-lsp nav pack`

## Forma de invocacion

```text
mi-lsp nav pack <task> [--workspace <alias>] [--rf <id>] [--fl <id>] [--doc <path>] [--full]
```

La CLI acepta una tarea libre y produce un envelope `backend=pack`.
El comando es repo-local directo y no requiere daemon.

## Payload logico

- `task`: string requerido
- `rf`: anchor opcional
- `fl`: anchor opcional
- `doc`: anchor opcional
- `workspace`: alias o path resoluble
- `max_items`, `token_budget`, `max_chars`: limites usuales del envelope

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

## Semantica observable

- `nav pack` entrega un reading pack canonico, no una respuesta textual explainable como `nav ask`.
- El orden del pack va de lo mas global a lo mas especifico segun familia documental y perfil local.
- Cuando la wiki canonica existe pero el indice documental esta vacio, el contrato devuelve warnings/hint accionables para reindexar.
- `--full` expande slices del mismo pack y no cambia `backend`.

## Routing interno

`nav pack` usa `resolveCanonicalRoute` (RF-QRY-015) como backbone para determinar el anchor documental cuando no hay override explicito.

Precedencia de seleccion de anchor:

1. `--doc <path>` — path explicito en el payload (maxima prioridad)
2. `--rf <id>` — id de RF como anchor
3. `--fl <id>` — id de FL como anchor
4. Route core — `resolveCanonicalRoute` determina el anchor canonico segun la tarea y el perfil del workspace

Cuando el route core determina el anchor, el campo `why` del resultado incluye `"tier2=route_core"`.
