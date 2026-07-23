---
id: RF-GPH-005
title: Navegar el grafo publicado con consultas bounded
status: planned
flows:
  - FL-GPH-02
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-005"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-02]]'
  - '[[RF-GPH-001]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-003]]'
exports:
  - 'RF-GPH-005'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-003.md
  - .docs/wiki/04_RF/RF-GPH-005.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-005.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-005.md
```

# RF-GPH-005 - Navegar el grafo publicado con consultas bounded

## 1. Resultado requerido

Exponer navegacion read-only, generation-aware y determinista sobre SQLite adjacency mediante:

- `nav neighbors <selector>`;
- `nav callers <selector>`;
- `nav callees <selector>`;
- `nav path <from-selector> <to-selector>`;
- `nav explain <selector-or-edge>`;
- `nav graph stats`;
- `nav graph validate`.

El daemon puede acelerar la consulta, pero direct mode debe producir el mismo contenido canonico para el mismo snapshot y presupuesto.

## 2. Selectores y opciones

Un selector acepta `NodeKey`, cross-RID, symbol/document ID o nombre scoped por workspace/repo/project. Nombres con mas de un match no se eligen por score: devuelven candidatos bounded y `GPH_QUERY_SELECTOR_AMBIGUOUS`.

Opciones comunes:

| Opcion | Default v1 | Hard ceiling v1 |
|---|---:|---:|
| `--depth` | 1 | 6 (`path`: 12) |
| `--limit` | 50 | 500 |
| `--token-budget` | 4000 | 20000 |
| `--generation` | activa al inicio | una generacion publicada |
| `--edge` | todas las registradas | allowlist de relation |
| `--direction` | semantica del comando | `in`, `out`, `both` |

`--cursor` es opaco, firmado por digest y ligado a generation, operacion, selector, filtros y ordering. Cambiar cualquiera invalida el cursor.

## 3. Semantica

- `neighbors`: BFS bounded por depth/limit con filtros explicitos.
- `callers`: edges `calls` entrantes; `callees`: `calls` salientes. No mezclan `references`.
- `path`: shortest path por cantidad de edges dentro de allowlist; empates por secuencia lexicografica `(relation, from_key, to_key)`.
- `explain`: devuelve claim, endpoints y todas las evidencias bounded; `inferred` conserva regla derivativa.
- `stats`: counts por kind/relation/status/backend/generation, unresolved y omissions.
- `validate`: chequea schema, sello, pointer, NodeKey, endpoints, evidence, cross-RID y ordering sin reparar ni escribir.

Traversal se ejecuta con queries/indexes SQLite por frontier; no carga el grafo completo en memoria ni persiste closures transitivos.

## 4. Envelope

`GraphQueryEnvelope` extiende `QueryEnvelope` con:

- `operation`, `generation_id`, `schema_version`, `backend` (`sqlite-direct` o `daemon/sqlite`);
- `items` con node/edge cross-RID, relation, status y evidence refs;
- `stats` con visited/frontier/returned/depth/tokens y elapsed;
- `warnings`, `omissions`, `truncated`, `continuation` y `determinism_digest`.

Items se ordenan por distance, relation, display-name casefold + original, NodeKey y evidence digest. La serializacion canonica no incluye timings en el digest.

## 5. Invariantes

- La query fija un snapshot al inicio y no escribe ni migra `index.db`.
- Un budget agotado devuelve `truncated=true`; nunca omite silenciosamente.
- `exact`, `extracted` e `inferred` permanecen distinguibles.
- Un daemon ausente, reiniciado o evicted degrada a direct mode, no cambia el oraculo.
- Resultados vacios son validos y explican selector/filtros/generation.
- No hay MCP, red ni store externo obligatorio.

## 6. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_QUERY_GENERATION_NOT_FOUND` | generation inexistente/invalid | fail sin fallback |
| `GPH_QUERY_SELECTOR_NOT_FOUND` | selector sin match | `items=[]` + hint |
| `GPH_QUERY_SELECTOR_AMBIGUOUS` | multiples matches | candidatos + refinamiento |
| `GPH_QUERY_BUDGET_INVALID` | valor <=0 o sobre ceiling | reject antes de leer |
| `GPH_QUERY_CURSOR_STALE` | cursor de otra generation/query | reject con restart hint |
| `GPH_QUERY_GRAPH_INVALID` | validate/sello/pointer falla | bloquear graph-native |
| `GPH_QUERY_BACKEND_UNAVAILABLE` | SQLite no accesible | error accionable |

## 7. Aceptacion y trazabilidad

- Golden envelopes y orden byte-identical en 30 repeticiones.
- Paridad direct/daemon excluyendo timings y metadatos operativos.
- Tests de selector ambiguo/ausente, generation retired/invalid, cursor stale y cada budget.
- Prueba de solo lectura por hash/mtime lógico de tablas antes/despues.
- `TP-GPH / TP-GPH-004 / TC-GPH-023..030`.
