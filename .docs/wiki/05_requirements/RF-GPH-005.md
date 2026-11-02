---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-005
kind: requirement
status: draft
title: RF-GPH-005 - Navegar el grafo publicado con consultas bounded
links:
  implements:
  - FL-GPH-02
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-005 - Navegar el grafo publicado con consultas bounded

## Comportamiento ordenado

### 1. Resultado requerido

Exponer navegacion read-only, generation-aware y determinista sobre SQLite adjacency mediante:

- `nav neighbors <selector>`;
- `nav callers <selector>`;
- `nav callees <selector>`;
- `nav path <from-selector> <to-selector>`;
- `nav explain <selector-or-edge>`;
- `nav graph stats`;
- `nav graph validate`.

El daemon puede acelerar la consulta, pero direct mode debe producir el mismo contenido canónico para el mismo snapshot y presupuesto.

### 2. Selectores y opciones

Un selector acepta `NodeKey`, cross-RID, symbol/document ID o nombre scoped por workspace/repo/project. Nombres con más de un match no se eligen por score: devuelven candidatos bounded y `GPH_QUERY_SELECTOR_AMBIGUOUS`.

Opciones comunes:

- **Opcion**: `--depth`; **Default v1**: 1; **Hard ceiling v1**: 6 (`path`: 12)
- **Opcion**: `--limit`; **Default v1**: 50; **Hard ceiling v1**: 500
- **Opcion**: `--token-budget`; **Default v1**: 4000; **Hard ceiling v1**: 20000
- **Opcion**: `--generation`; **Default v1**: activa al inicio; **Hard ceiling v1**: una generación publicada
- **Opcion**: `--edge`; **Default v1**: todas las registradas; **Hard ceiling v1**: allowlist de relation
- **Opcion**: `--direction`; **Default v1**: semántica del comando; **Hard ceiling v1**: `in`, `out`, `both`

`--cursor` es opaco, firmado por digest y ligado a generation, operación, selector, filtros y ordering. Cambiar cualquiera inválida el cursor.

### 3. Semántica

- `neighbors`: BFS bounded por depth/limit con filtros explicitos.
- `callers`: edges `calls` entrantes; `callees`: `calls` salientes. No mezclan `references`.
- `path`: shortest path por cantidad de edges dentro de allowlist; empates por secuencia lexicografica `(relation, from_key, to_key)`.
- `explain`: devuelve claim, endpoints y todas las evidencias bounded; `inferred` conserva regla derivativa.
- `stats`: counts por kind/relation/status/backend/generation, unresolved y omissions.
- `validate`: chequea schema, sello, pointer, NodeKey, endpoints, evidence, cross-RID y ordering sin reparar ni escribir.

Traversal se ejecuta con queries/indexes SQLite por frontier; no carga el grafo completo en memoria ni persiste closures transitivos.

### 4. Envelope

`GraphQueryEnvelope` extiende `QueryEnvelope` con:

- `operation`, `generation_id`, `schema_version`, `backend` (`sqlite-direct` o `daemon/sqlite`);
- `items` con node/edge cross-RID, relation, status y evidence refs;
- `stats` con visited/frontier/returned/depth/tokens y elapsed;
- `warnings`, `omissions`, `truncated`, `continuation` y `determinism_digest`.

Items se ordenan por distance, relation, display-name casefold + original, NodeKey y evidence digest. La serializacion canónica no incluye timings en el digest.

### 5. Invariantes

- La query fija un snapshot al inicio y no escribe ni migra `index.db`.
- Un budget agotado devuelve `truncated=true`; nunca omite silenciosamente.
- `exact`, `extracted` e `inferred` permanecen distinguibles.
- Un daemon ausente, reiniciado o evicted degrada a direct mode, no cambia el oraculo.
- Resultados vacios son válidos y explican selector/filtros/generation.
- No hay MCP, red ni store externo obligatorio.

### 8. Navegación documental

`nav.neighbors`, `nav.path` y `nav.explain` consumen las mismas aristas documentales publicadas. Los selectores aceptan `doc_id` o ruta de documento; `--edge` puede filtrar `doc_related`, `doc_depends`, `doc_supports`, `doc_contradicts` y `doc_supersedes`. El envelope conserva relación, status, ruta destino y `doc_id`; depth, limit, token budget y cursor acotan ciclos y expansiones. Un grafo stale bloquea solo la consulta graph-native y mantiene la búsqueda textual.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 6. Errores tipados

- **Código**: `GPH_QUERY_GENERATION_NOT_FOUND`; **Causa**: generation inexistente/invalid; **Respuesta**: fail sin fallback
- **Código**: `GPH_QUERY_SELECTOR_NOT_FOUND`; **Causa**: selector sin match; **Respuesta**: `items=[]` + hint
- **Código**: `GPH_QUERY_SELECTOR_AMBIGUOUS`; **Causa**: multiples matches; **Respuesta**: candidatos + refinamiento
- **Código**: `GPH_QUERY_BUDGET_INVALID`; **Causa**: valor <=0 o sobre ceiling; **Respuesta**: reject antes de leer
- **Código**: `GPH_QUERY_CURSOR_STALE`; **Causa**: cursor de otra generation/query; **Respuesta**: reject con restart hint
- **Código**: `GPH_QUERY_GRAPH_INVALID`; **Causa**: validate/sello/pointer falla; **Respuesta**: bloquear graph-native
- **Código**: `GPH_QUERY_BACKEND_UNAVAILABLE`; **Causa**: SQLite no accesible; **Respuesta**: error accionable

## Criterios de aceptación (numerados)

1. Golden envelopes y orden byte-identical en 30 repeticiones.
2. Paridad direct/daemon excluyendo timings y metadatos operativos.
3. Tests de selector ambiguo/ausente, generation retired/invalid, cursor stale y cada budget.
4. Prueba de solo lectura por hash/mtime lógico de tablas antes/despues.

### 7. Aceptacion y trazabilidad

- Golden envelopes y orden byte-identical en 30 repeticiones.
- Paridad direct/daemon excluyendo timings y metadatos operativos.
- Tests de selector ambiguo/ausente, generation retired/invalid, cursor stale y cada budget.
- Prueba de solo lectura por hash/mtime lógico de tablas antes/despues.
- `TP-GPH / TP-GPH-004 / TC-GPH-023..030`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-GPH-005
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-GPH-02.md
exports:
- RF-GPH-005
- RF-GPH-005 - Navegar el grafo publicado con consultas bounded
agent_must_read:
- .docs/wiki/05_requirements/RF-GPH-005.md
- .docs/wiki/04_flows/FL-GPH-02.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-GPH-005.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-GPH.md, ejecutar el
  comando publicado: go test ./internal/model/... ./internal/indexer/... ./internal/service/...
  ./internal/daemon/... (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-GPH-005.
evidence:
- .docs/wiki/05_requirements/RF-GPH-005.md
- .docs/wiki/06_tests/TP-GPH.md
```
