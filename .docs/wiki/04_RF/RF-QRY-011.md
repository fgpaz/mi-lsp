# RF-QRY-011 - Resolver intencion en modo hibrido docs|code con scope opcional de repo

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-011 |
| Titulo | Resolver intencion en modo hibrido docs|code con scope opcional de repo |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| Catalogo repo-local disponible | tecnica | obligatorio |
| Pregunta no vacia | funcional | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI recibe `mi-lsp nav intent <question>`.
2. El core clasifica la pregunta en `mode=docs` o `mode=code`.
3. Si el usuario envio `--repo`, el core valida el selector; en `mode=code` acota el universo al repo hijo seleccionado del workspace `container`, y en `mode=docs` puede ignorarlo con warning visible.
4. En `mode=docs`, el sistema usa el scorer owner-aware documental compartido con `nav route/ask/pack`.
5. En `mode=code`, el sistema mantiene el ranking BM25 actual sobre `search_text` enriquecido del catalogo con boosts por nombre/kind.
6. Devuelve un envelope `backend=intent` con `mode=docs|code`.
7. Como `nav intent` pertenece a la superficie AXI-default, la primera page puede ser mas estrecha por default y debe incluir guidance de expansion via `--full` salvo `--classic`.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_INTENT_QUESTION_REQUIRED` | falta pregunta | argumento vacio | abortar con error explicito |
| `QRY_INTENT_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_INTENT_SCOPE_UNKNOWN` | repo selector invalido | `--repo` no coincide con un repo hijo | devolver `backend=router`, candidatos y `next_hint` |

## 5. Special Cases and Variants

- Si la normalizacion no produce tokens utiles, responde `ok=true`, `items=[]` y warning.
- Si `mode=docs` y no hay docs indexados fuertes, puede responder desde Tier 1 canonical route con items documentales owner-aware.
- Si `mode=code` y no hay simbolos compatibles, responde `ok=true`, `items=[]` y warning.
- La operacion es catalog-first y directa; no depende del daemon.
- En workspaces `container`, `--repo` acota el resultado solo en `mode=code`; en `mode=docs` se valida pero no cambia el lane documental.
- En AXI efectivo, la semantica del ranking no cambia; solo cambia la disclosure inicial y el guidance de expansion.

## 6. Data Model Impact

- `SymbolRecord`
- `QueryEnvelope`
- `QueryOptions`
