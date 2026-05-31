# RF-SEM-003 - Recuperar secciones por significado con nav recall (ungated, knowledge-wiki)

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-SEM-003"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-SEM-003]]'
exports:
  - 'RF-SEM-003'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-SEM-003.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-SEM-003.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-SEM-003.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-SEM-003 |
| Titulo | Recuperar secciones por significado con nav recall (ungated, knowledge-wiki) |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-SEM-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace con embeddings indexados (RF-SEM-002) | funcional | obligatorio |
| Config embeddings cargada (RF-SEM-001) | tecnica | obligatorio |
| Perfil knowledge-wiki efectivo (no bloqueado por gobernanza) | operativa | obligatorio |
| Sin requirement de governance gate (ungated recall) | semantica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `query` | string | si | CLI | natural language, ES o EN soportado | RF-SEM-003 |
| `--workspace` | string | si | CLI | alias workspace registrado | RF-SEM-003 |
| `--top` | entero | no | CLI | 1..50, default 5 | RF-SEM-003 |
| `--offset` | entero | no | CLI | >= 0, default 0 | RF-SEM-003 |
| `--layer` | string | no | CLI | e.g. `04_RF`, `07_tech` o `*` | RF-SEM-003 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-SEM-003 |

## 4. Process Steps (Happy Path)

1. CLI recibe `mi-lsp nav recall "<query>" --workspace <alias> [--top N] [--layer X]`.
2. Core carga config embeddings (RF-SEM-001).
3. Si query es ES, opcionalmente traduce a EN (o embebe ES directo si backend soporta).
4. Core solicita embedding de la query al backend (timeout `timeout_ms`).
5. Si embedding disponible, busca TOP-K similitud cosine en tabla `wiki_chunk_embeddings`.
6. Si backend no disponible, cae a fallback: busca lexical full-text sobre `chunk_content` (offline).
7. Filtro por `--layer` si esta presente (e.g., solo `04_RF/` docs).
8. Aplica paginacion: `--offset` y `--top`.
9. Formatea resultados con `heading`, `doc_path`, `relevance_score` (cosine o BM25 en fallback).
10. Devuelve envelope estable (no gated por gobernanza).

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | operacion exitosa |
| `items` | lista | usuario/skill | array de chunks con `heading`, `doc_path`, `content_preview`, `relevance_score` |
| `backend` | string | usuario/skill | `embeddings` si usa semantic, `text-index` si es fallback |
| `truncated` | bool | usuario/skill | hay mas resultados disponibles |
| `warnings` | lista | usuario/skill | diagnostico de fallback activado, slow query, etc. |
| `stats` | objeto | usuario/skill | `chunks_matched`, `search_time_ms`, `backend_latency_ms` |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `SEM_WORKSPACE_UNRESOLVED` | alias invalido | `--workspace` no existe | abortar con error explicito |
| `SEM_QUERY_TIMEOUT` | query embedding agota timeout | timeout_ms excedido | cae a fallback lexical |
| `SEM_NO_RESULTS` | ninguna seccion coincide | query muy especifica o indice vacio | retorna items=[] con hint accionable |

## 7. Special Cases and Variants

- Si query es muy corta (<3 caracteres), sugiere expansion en `next_hint`.
- Si backend disponible pero timeout, cae silenciosamente a fallback con warning.
- Si `--layer` es presente pero ningun resultado coincide, retorna items=[] + hint "no matches in layer X".
- Soporta bilingual: query ES se embebe directo o se traduce segun backend capability.
- Sin gobernanza gate (ungated): mismo usuario puede recuperar spec docs, implementation docs, o knowledge wiki sin bloqueo.

## 8. Data Model Impact

- Query embedding temporal en memoria (no persistente)
- Consulta sobre `wiki_chunk_embeddings` con cosine similarity

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Recuperar secciones por significado con backend
  Given un workspace con embeddings indexados
  When ejecuto "mi-lsp nav recall 'como configurar embeddings' --workspace mi-lsp --top 5"
  Then retorna items con relevancia ordenada
  And cada item tiene "heading", "doc_path", "relevance_score"
  And backend = "embeddings"

Scenario: Caer a fallback lexical si backend no disponible
  Given backend embeddings no disponible
  When ejecuto "mi-lsp nav recall 'configurar embeddings' --workspace mi-lsp"
  Then retorna items usando busqueda full-text (offline)
  And backend = "text-index"
  And warning indica "backend unavailable; using offline lexical"

Scenario: Soportar bilingual ES↔EN
  Given workspace con docs en Español e Ingles
  When ejecuto "mi-lsp nav recall 'embeddings' --workspace mi-lsp" (EN)
  Then retorna sections de ambos idiomas si son semanticamente relacionadas
  And cuando ejecuto misma query con termino ES equivalente, resultados similares

Scenario: Respetar layer filtering
  Given workspace con docs en multiples capas (04_RF/, 07_tech/)
  When ejecuto "mi-lsp nav recall 'recall' --workspace mi-lsp --layer 04_RF"
  Then solo retorna chunks de archivos en .docs/wiki/04_RF/
  And chunks de 07_tech/ son excluidos
```

## 10. Test Traceability

- Positivo: `TP-SEM / TC-SEM-007`
- Positivo: `TP-SEM / TC-SEM-008`
- Negativo: `TP-SEM / TC-SEM-009`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir gobernanza gate (ungated recall)
  - no exponer tokens de embeddings raw en output
  - no filtrar resultados por RF existencia (knowledge-wiki es agnóstico)
- Decisiones cerradas:
  - fallback offline-lexical es automatico
  - TOP-K cosine similarity ranking
  - query embedding temporal (no cacheado globalmente)
  - soporte ES/EN bilingual (backend-dependent)
- TODO explicit = 0
- Fuera de alcance:
  - multi-workspace recall (future, similar a FL-WIKI-01)
  - cache de query embeddings (future)
