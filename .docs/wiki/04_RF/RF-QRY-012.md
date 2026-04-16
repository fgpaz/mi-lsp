# RF-QRY-012 - Construir un reading pack canonico docs-first para una tarea

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-012 |
| Titulo | Construir un reading pack canonico docs-first para una tarea |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Indice repo-local disponible o construible | tecnica | obligatorio |
| Corpus documental canonico accesible | funcional | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI recibe `mi-lsp nav pack <task>`.
2. El core resuelve el workspace y carga el `read-model` del proyecto o el default embebido.
3. El core invoca `resolveCanonicalRoute` (RF-QRY-015) para clasificar la tarea y obtener el anchor canonico; si el payload incluye `--rf`, `--fl` o `--doc`, ese anchor explicito sobreescribe el resultado del route core.
4. El core construye un reading pack ordenado de lo mas global a lo mas especifico segun la ladder canonica, reutilizando el mismo scorer owner-aware compartido por `nav route` y `nav ask`.
5. En modo preview devuelve paths, stages, razones y targets sugeridos.
6. En modo `--full` expande slices legibles de los documentos seleccionados sin cambiar el envelope base.
7. Si la wiki existe pero el indice documental esta vacio o stale, devuelve warnings accionables para reindexar en vez de degradar silenciosamente a `README`.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_PACK_TASK_REQUIRED` | falta tarea | argumento vacio | abortar con error explicito |
| `QRY_PACK_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_PACK_DOC_INDEX_UNAVAILABLE` | store repo-local no accesible | `index.db` no abrible | abortar con error explicito |

## 5. Special Cases and Variants

- El input principal es tarea libre; el route core (`resolveCanonicalRoute`, RF-QRY-015) determina el anchor canonico por defecto.
- `--doc <path>`, `--rf <id>` y `--fl <id>` endurecen la seleccion con un anchor explicito que sobreescribe el route core (precedencia: `--doc` > `--rf` > `--fl` > route core).
- `nav pack` pertenece a la superficie AXI-default y usa preview-first por default; `--full` expande slices sin cambiar la semantica del comando.
- La ladder base prioriza documentos raiz antes de bajar a `FL`, `RF`, `TECH-*`, `CT-*` o capas UX segun la familia.
- Si existe `.docs/wiki/_mi-lsp/read-model.toml`, el bloque `reading_pack` puede ajustar `max_docs` y el orden de stages por familia.
- Cuando existen `owner_hints` proyectados desde `00`, esos hints pueden empujar el anchor canonico sin reemplazar un override explicito.

## 6. Data Model Impact

- `DocRecord`
- `DocEdge`
- `DocsReadProfile`
- `PackResult`
- `PackDoc`
- `PackTarget`
- `QueryEnvelope`
