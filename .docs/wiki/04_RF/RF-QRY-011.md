# RF-QRY-011 - Buscar simbolos por intencion con scope opcional de repo

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-011 |
| Titulo | Buscar simbolos por intencion con scope opcional de repo |
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
2. El core tokeniza la pregunta y consulta el `search_text` enriquecido del catalogo.
3. Si el usuario envio `--repo`, el core acota el universo al repo hijo seleccionado del workspace `container`.
4. El sistema rankea candidatos por BM25 y boosts por nombre/kind.
5. Devuelve un envelope compacto con `file`, `line`, `symbol`, `kind`, `qualified_name`, `score`, `evidence` y `snippet`.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_INTENT_QUESTION_REQUIRED` | falta pregunta | argumento vacio | abortar con error explicito |
| `QRY_INTENT_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con `ok=false` |
| `QRY_INTENT_SCOPE_UNKNOWN` | repo selector invalido | `--repo` no coincide con un repo hijo | devolver `backend=router`, candidatos y `next_hint` |

## 5. Special Cases and Variants

- Si la normalizacion no produce tokens utiles, responde `ok=true`, `items=[]` y warning.
- Si no hay simbolos compatibles, responde `ok=true`, `items=[]` y warning.
- La operacion es catalog-first y directa; no depende del daemon.
- En workspaces `container`, `--repo` acota el resultado pero no cambia el backend a semantico.

## 6. Data Model Impact

- `SymbolRecord`
- `QueryEnvelope`
