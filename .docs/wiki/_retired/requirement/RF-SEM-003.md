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
| Workspace con embeddings indexados (RF-SEM-002) | funcional | obligatorio para ranking vectorial |
| Config embeddings cargada (RF-SEM-001) | tecnica | obligatorio para ranking vectorial |
| Perfil knowledge-wiki efectivo (no bloqueado por gobernanza) | operativa | obligatorio |
| Sin requirement de governance gate (ungated recall) | semantica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `query` | string | si | CLI | natural language, ES o EN soportado | RF-SEM-003 |
| `--workspace` | string | si | CLI | alias workspace registrado | RF-SEM-003 |
| `--max-items` | entero | no | CLI global | limite de resultados; default global del CLI | RF-SEM-003 |
| `--token-budget` | entero | no | CLI global | presupuesto aproximado de salida | RF-SEM-003 |
| `--map` | bool | no | CLI | agrupa resultados en mapa compacto | RF-SEM-003 |
| `--intent` | enum | no | CLI | `formula`, `evidence`, `route`, `explore`, `learning`; default `explore` | RF-SEM-003 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-SEM-003 |

Guia de intenciones:

- `formula`: recuperar contratos, formulas, unidades, fixtures, rangos y stop conditions source-grounded.
- `evidence`: recuperar fuentes canonicas, source IDs, matrices de evidencia y punteros de seccion.
- `route`: recuperar perfiles o notas de ruteo para decidir quien atiende; estos hits no son fuente final.
- `explore`: explorar contexto amplio, sintesis, indices y decisiones previas.
- `learning`: recuperar aprendizajes durables, memoria operativa y reglas de mejora.

## 4. Process Steps (Happy Path)

1. CLI recibe `mi-lsp nav recall "<query>" --workspace <alias> [--intent formula|evidence|route|explore|learning]`.
2. Core carga config embeddings (RF-SEM-001).
3. Core normaliza `--intent` y enriquece el texto de query para sesgar el ranking.
4. Core solicita embedding de la query enriquecida al backend (timeout `timeout_ms`).
5. Si embedding disponible, busca TOP-K similitud cosine en tabla `wiki_chunk_embeddings`.
6. Aplica reranking por intencion sobre metadata, path, doc title, heading y snippet.
7. Si RF-SEM-004 esta configurado, puede reordenar una ventana acotada mediante hook local externo antes del corte final.
8. Si backend no disponible, emite warning/hint y la guia documental segura es `mi-lsp nav wiki search`, no modelo local oculto.
9. Aplica `--max-items` y `--token-budget`.
10. Formatea resultados como `RecallResult` con `intent`, `archivo`, `heading`, `score`, `snippet`, `start_line`, `end_line` y `why`.
11. Devuelve envelope estable (no gated por gobernanza).

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | operacion exitosa |
| `items` | lista | usuario/skill | array `RecallResult[]` con `intent`, `archivo`, `heading`, `snippet`, `score`, lineas y `why` |
| `backend` | string | usuario/skill | `recall` si usa semantic; fallback documental recomendado `nav wiki search` |
| `truncated` | bool | usuario/skill | hay mas resultados disponibles |
| `warnings` | lista | usuario/skill | diagnostico de fallback activado, slow query, etc. |
| `hint` | string | usuario/skill | explica intencion efectiva o fallback |
| `stats` | objeto | usuario/skill | conteos y presupuesto de salida |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `SEM_WORKSPACE_UNRESOLVED` | alias invalido | `--workspace` no existe | abortar con error explicito |
| `SEM_QUERY_TIMEOUT` | query embedding agota timeout | timeout_ms excedido | warning + guidance a `nav wiki search` |
| `SEM_NO_RESULTS` | ninguna seccion coincide | query muy especifica o indice vacio | retorna items=[] con hint accionable |

## 7. Special Cases and Variants

- Si query es muy corta (<3 caracteres), sugiere expansion en `next_hint`.
- Si backend disponible pero timeout, devuelve warning/hint y recomienda `nav wiki search`.
- Intent desconocido se normaliza a `explore`.
- El modelo de embeddings descubre candidatos; no convierte material route-only en fuente final.
- `intent=route` prioriza perfiles/routing para despacho, pero el agente debe volver a `formula` o `evidence` para citar fuentes finales.
- Soporta bilingual: query ES se embebe directo o se traduce segun backend capability.
- Si el hook externo de RF-SEM-004 falla, conserva el orden semantico e informa warning sanitizado.
- Sin gobernanza gate (ungated): mismo usuario puede recuperar spec docs, implementation docs, o knowledge wiki sin bloqueo.

## 8. Data Model Impact

- Query embedding temporal en memoria (no persistente)
- Consulta sobre `wiki_chunk_embeddings` con cosine similarity

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Recuperar secciones por significado con backend
  Given un workspace con embeddings indexados
  When ejecuto "mi-lsp nav recall 'como configurar embeddings' --workspace mi-lsp --max-items 5"
  Then retorna items con relevancia ordenada
  And cada item tiene "intent", "heading", "archivo", "score"
  And backend = "recall"

Scenario: Recomendar fallback wiki si backend no disponible
  Given backend embeddings no disponible
  When ejecuto "mi-lsp nav recall 'configurar embeddings' --workspace mi-lsp"
  Then warning indica provider no disponible
  And hint recomienda `mi-lsp nav wiki search`

Scenario: Soportar bilingual ES↔EN
  Given workspace con docs en Español e Ingles
  When ejecuto "mi-lsp nav recall 'embeddings' --workspace mi-lsp" (EN)
  Then retorna sections de ambos idiomas si son semanticamente relacionadas
  And cuando ejecuto misma query con termino ES equivalente, resultados similares

Scenario: Diferenciar route de formula
  Given workspace con un contrato de formula y un perfil route-only
  When ejecuto "mi-lsp nav recall 'formula hops' --workspace mi-lsp --intent formula"
  Then el contrato de formula queda por encima del perfil route-only
  When ejecuto "mi-lsp nav recall 'formula hops' --workspace mi-lsp --intent route"
  Then el perfil route-only puede subir para despacho
  And el hint indica que route no es fuente final
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
  - fallback documental seguro es `nav wiki search`
  - TOP-K cosine similarity ranking
  - query embedding temporal (no cacheado globalmente)
  - soporte ES/EN bilingual (backend-dependent)
  - embeddings descubren candidatos; source-of-truth final sigue siendo wiki canonica/source-grounded
  - rerank externo es opcional, local y fail-open hacia el orden semantico original
- TODO explicit = 0
- Fuera de alcance:
  - multi-workspace recall (future, similar a FL-WIKI-01)
  - cache de query embeddings (future)
