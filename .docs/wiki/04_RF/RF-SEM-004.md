---
id: RF-SEM-004
title: Reordenar candidatos de recall mediante hook local externo y seguro
implements:
  - internal/model/types.go
  - internal/rerank/extension.go
  - internal/service/recall.go
  - internal/service/workspace_ops.go
tests:
  - internal/model/embeddings_test.go
  - internal/rerank/extension_test.go
  - internal/service/recall_test.go
---

# RF-SEM-004 - Reordenar candidatos de recall mediante hook local externo y seguro

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-SEM-004"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-SEM-004]]'
exports:
  - 'RF-SEM-004'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-SEM-004.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-SEM-004.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-SEM-004.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-SEM-004 |
| Titulo | Reordenar candidatos de recall mediante hook local externo y seguro |
| Actores | Usuario, Skill, Agente, CLI/Core, comando local externo |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-SEM-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Embeddings configurados e indexados | funcional | obligatorio para invocar rerank |
| `[recall.rerank_extension] enabled=true` | operativa | obligatorio |
| `command` local resuelto por el host | tecnica | obligatorio |
| Sin dependencia privada en core | arquitectura | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `enabled` | bool | si | `.mi-lsp/project.toml [recall.rerank_extension]` | default false; solo true activa hook | RF-SEM-004 |
| `command` | string | si | `.mi-lsp/project.toml` | path o binario local; no se ejecuta via shell | RF-SEM-004 |
| `args` | lista string | no | `.mi-lsp/project.toml` | argumentos estaticos, sin interpolacion de secretos | RF-SEM-004 |
| `timeout_ms` | entero | no | `.mi-lsp/project.toml` | >0; default 2000 | RF-SEM-004 |
| `candidate_count` | entero | no | `.mi-lsp/project.toml` | ventana maxima de candidatos enviados; default 50 | RF-SEM-004 |
| `top_n` | entero | no | `.mi-lsp/project.toml` | cantidad solicitada al hook; clamp a candidatos | RF-SEM-004 |
| `max_snippet_chars` | entero | no | `.mi-lsp/project.toml` | limite local de snippet por candidato; default 500 | RF-SEM-004 |

## 4. Process Steps

1. `nav recall` ejecuta primero RF-SEM-003: embedding de query, cosine similarity e intent reranking local.
2. Si embeddings estan deshabilitados o fallan, no invoca el hook y conserva el fallback a `nav wiki search`.
3. Si el hook esta activo, Core serializa stdin JSON versionado con query, `top_n`, candidatos acotados y metadata segura.
4. Core ejecuta `command` con `args` mediante subprocess local sin shell y con timeout.
5. Core parsea stdout JSON versionado con `indices` o `results[].index`.
6. Si la salida es valida, reordena la ventana de candidatos antes del corte final de `max_items`.
7. Si la salida es parcial, coloca primero los indices devueltos y completa faltantes en orden semantico original.
8. Si hay cualquier falla, conserva el orden semantico original y emite warning sanitizado.

## 5. Protocol

Stdin:

```json
{
  "protocol_version": "mi-lsp-rerank-extension-v1",
  "query": "freeform user query",
  "top_n": 5,
  "candidates": [
    {
      "index": 0,
      "archivo": ".docs/wiki/07_tech/TECH-SEMANTIC-RECALL.md",
      "heading": "Algoritmo de busqueda",
      "snippet": "bounded snippet",
      "score": 0.82,
      "start_line": 70,
      "end_line": 90,
      "why": ["semantic_match", "intent_formula"]
    }
  ],
  "metadata": {"source": "nav.recall"}
}
```

Stdout:

```json
{
  "protocol_version": "mi-lsp-rerank-extension-v1",
  "indices": [2, 0, 1],
  "warnings": ["optional provider-local warning"]
}
```

`results: [{"index": 2, "score": 0.91}]` es equivalente cuando el adaptador quiere devolver score externo. Scores no finitos o indices duplicados/fuera de rango invalidan la respuesta.

## 6. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `RecallResult.why += external_rerank` | lista | usuario/skill | indica que el hook reordeno ese candidato |
| `warnings` | lista | usuario/skill | degradaciones sanitizadas, sin payloads ni secretos |
| `workspace.status.rerank_extension_enabled` | bool | usuario/skill | diagnostico de configuracion |
| `workspace.status.rerank_extension_mode` | string | usuario/skill | `local-command` cuando activo |

## 7. Failure Semantics

- Missing command, start failure, timeout, exit no cero, stdout vacio, JSON invalido, version invalida, indices duplicados/fuera de rango o scores no finitos preservan el orden semantico original.
- Warnings no incluyen query text, snippets, stdin/stdout completo, stderr, provider responses, tokens, API keys ni rutas secretas.
- El hook nunca se invoca cuando `[embeddings]` esta inactivo o el embedding de query degrada a lexical fallback.

## 8. Data Model Impact

- Config local nueva: `[recall.rerank_extension]` dentro de `.mi-lsp/project.toml`.
- Sin tabla SQLite nueva.
- Sin persistencia de payloads de rerank.
- `AccessEvent` conserva solo conteos/warnings existentes; no persiste query/snippets/stdin/stdout del hook.

## 9. Acceptance Criteria

```gherkin
Scenario: Rerank externo reordena candidatos
  Given embeddings configurados e indexados
  And `[recall.rerank_extension] enabled=true`
  When ejecuto `mi-lsp nav recall "consulta" --workspace mi-lsp --max-items 1`
  And el hook devuelve un indice valido distinto del primer candidato semantico
  Then el resultado visible usa ese orden
  And `why` incluye `external_rerank`

Scenario: Falla del hook preserva orden
  Given un hook activo que responde JSON invalido
  When ejecuto `mi-lsp nav recall "consulta sensible" --workspace mi-lsp`
  Then el orden semantico original se conserva
  And `warnings` contiene solo una razon sanitizada
  And `warnings` no contiene query, snippet, stdout, stderr, tokens ni secretos

Scenario: Embeddings inactivos no invocan rerank
  Given `[embeddings] enabled=false`
  And `[recall.rerank_extension] enabled=true`
  When ejecuto `mi-lsp nav recall "consulta" --workspace mi-lsp`
  Then `items=[]` con hint de embeddings
  And no se ejecuta el comando externo
```

## 10. Test Traceability

- Positivo: `TP-SEM / TC-SEM-010`
- Positivo: `TP-SEM / TC-SEM-011`
- Negativo: `TP-SEM / TC-SEM-012`
- Negativo: `TP-SEM / TC-SEM-013`
- Negativo: `TP-SEM / TC-SEM-014`

## 11. No Ambiguities Left

- El core no implementa clientes privados ni proxies especificos de proveedor para rerank.
- El adaptador privado, si existe, vive fuera de este repositorio y habla el protocolo local.
- `nav search` y `nav recall` no rerankeado siguen siendo fallbacks confiables.
- Cambiar modelo o dimension de embeddings requiere reindex/reembedding completo de los chunks afectados.
