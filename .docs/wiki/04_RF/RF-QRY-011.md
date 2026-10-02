---
doc_id: RF-QRY-011
---

# RF-QRY-011 - Resolver intencion en modo hibrido docs|code con scope opcional de repo

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-011"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-011]]'
exports:
  - 'RF-QRY-011'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-011.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-011.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-011.md
```

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
3. Si la pregunta trae señales de código (ver sección 5) y el catálogo contiene símbolos o archivos que coinciden con sus tokens, el modo es `mixed`: los matches de código fuertes van primero, luego los documentos y al final los matches de código débiles, todo acotado a `top`.
4. Si el usuario envio `--repo`, el core valida el selector; en `mode=code` acota el universo al repo hijo seleccionado del workspace `container`, y en `mode=docs` puede ignorarlo con warning visible.
5. En `mode=docs`, el sistema usa el scorer owner-aware documental compartido con `nav route/ask/pack`.
6. En `mode=code`, el sistema mantiene el ranking BM25 actual sobre `search_text` enriquecido del catalogo con boosts por nombre/kind.
7. Devuelve un envelope `backend=intent` con `mode=docs|code|mixed`.
8. Como `nav intent` pertenece a la superficie AXI-default, la primera page puede ser mas estrecha por default y debe incluir guidance de expansion via `--full` salvo `--classic`.
9. Para una salida implícita consumida por agentes, la CLI presenta el workspace elegido primero y una forma compacta; `--verbose` amplía el detalle. Un `--format` explícito conserva el formato solicitado.
10. Para los candidatos Markdown de `nav.intent`, se permite refrescar hasta cinco paths bajo presupuesto de 500 ms. Si se publica el catálogo, se recarga y vuelve a puntuar una sola vez; ante error/deadline se devuelven los resultados calculados en memoria con warning. El refresco no reconstruye el grafo.

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
- `mode=mixed` (aditivo; `docs` y `code` se mantienen): se activa cuando la pregunta tiene señales léxicas de código (camelCase, snake_case, `a.b`, rutas o archivos `*.go|cs|ts|tsx|js|jsx|py|rs|java`, sustantivos como `function|func|method|struct|class|handler|interface|type`, o patrones `where|donde ... implemented|defined|called|implementa|define|llama`) o cuando un token de la pregunta coincide con un nombre de símbolo o de archivo del catálogo. Sin señales ni coincidencia de catálogo, o sin catálogo, la respuesta de `mode=docs` queda intacta.
- Cada item de `nav.intent` lleva `result_kind=code|doc`; `kind` conserva el tipo de símbolo (`function`, `struct`, etc.) y `origin` es `catalog` para código y `wiki` para documentos. Un match de código es fuerte si su nombre iguala un identificador de la pregunta, o si es `name_match` y cubre al menos dos tokens en nombre, padre o ruta; los fuertes ocupan como máximo `max(1, 2/3 de top)` posiciones antes de los documentos.
- Tokenización de lenguaje natural para código: separa identificadores camelCase y snake_case, descarta stopwords en inglés y español y palabras de menos de tres caracteres, y agrega un stem liviano (`ing`, `ed`, `es`, `s`) como token adicional sin reemplazar la palabra original.
- En AXI efectivo, la semantica del ranking no cambia; solo cambia la disclosure inicial y el guidance de expansion.

## 6. Data Model Impact

- `SymbolRecord`
- `QueryEnvelope`
- `QueryOptions`

## 7. Test Traceability

- Positivo: `TP-QRY / TC-QRY-040`
- Positivo: `TP-QRY / TC-QRY-044`
- Positivo: `TP-QRY / TC-QRY-045`
- Positivo: `TP-QRY / TC-QRY-179`
- Positivo: `TP-QRY / TC-QRY-181`
- Negativo: `TP-QRY / TC-QRY-041`
- Negativo: `TP-QRY / TC-QRY-180`
