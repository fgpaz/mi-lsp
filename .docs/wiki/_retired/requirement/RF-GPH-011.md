---
id: RF-GPH-011
title: Optimizar contexto y packs derivados con gates reproducibles
status: planned
flows:
  - FL-GPH-02
  - FL-GPH-03
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-011"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-02]]'
  - '[[FL-GPH-03]]'
  - '[[RF-GPH-005]]'
  - '[[RF-GPH-007]]'
  - '[[RF-GPH-009]]'
  - '[[RF-GPH-010]]'
exports:
  - 'RF-GPH-011'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/03_FL/FL-GPH-03.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-GPH-007.md
  - .docs/wiki/04_RF/RF-GPH-009.md
  - .docs/wiki/04_RF/RF-GPH-010.md
  - .docs/wiki/04_RF/RF-GPH-011.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-011.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-011.md
```

# RF-GPH-011 - Optimizar contexto y packs derivados con gates reproducibles

## 1. Resultado requerido

Seleccionar el contexto minimo suficiente para una tarea y habilitar packs opcionales sin mover identidad, schema, generacion, seguridad, provenance o autoridad fuera del core. Cada slice estable debe superar al baseline previo de mi-lsp y a Graphify en su alcance medible, sin regresion de correctness/wiki authority/determinismo.

## 2. Context Optimizer

Entrada: intent/selector, generation(s), authority chain, depth/result/token budget, required evidence classes y caller scope.

Proceso determinista:

1. fijar documentos/nodos obligatorios por gobernanza y operacion;
2. generar candidatos por graph paths bounded;
3. calcular cobertura marginal de claims/evidence frente a costo canonico de tokens;
4. seleccionar greedily con tie-break estable por authority, confidence class, distance, NodeKey/doc ID;
5. deduplicar snippets/evidence sin eliminar precondiciones;
6. emitir omissions, unresolved y razones include/exclude.

Salida `ContextPack` con generation, primary/required items, optional items, evidence refs, token budget/used, coverage vector, omissions, truncated y determinism digest.

## 3. Familias de packs

| Pack | Proposito | Estado inicial |
|---|---|---|
| `Visual` | export/layout derivativo y navegacion humana | optional MILX |
| `Languages-LSP` | adapters semanticos tsserver/Pyright y futuros | gated por RF-GPH-008 |
| `Languages-AST` | parsers estructurales adicionales | extracted, no compiler authority |
| `Documents` | formatos/document links adicionales | derivativo, docs authority intacta |
| `Algorithms` | SCC/ciclos/centralidad/comunidades | SCC/ciclos bounded pueden core; heavy via MILX |
| `Stores` | export/cache externo opcional | nunca autoridad ni dependencia de query core |
| `Graphify Import` | importar snapshot externo advisory | namespace `external:graphify` |
| `Remote Snapshot` | snapshot firmado/configurado explicitamente | off por defecto; sin red implicita |

Promover un pack exige manifest/version, security review, deterministic fixtures, resource budget, rollback y release-distribution evidence.

## 4. Cache e invalidacion

`GraphAnalysis`/pack cache se keyea por generation(s), pack ID/version/executable digest, operation, parameters, authority/profile digest y output schema. Cambiar cualquiera invalida. Cache corrupta o ausente se recomputa o reporta omission; nunca modifica active generation.

## 5. Gate de victoria

Por slice relevante, 30 repeticiones por variante con misma fixture/protocolo y comparadores fijados:

- Graphify `9bf14a4931658152969586ace39eb965c010f0d1`;
- baseline mi-lsp `a251ab1f8db4e96f029926fbef275b078a20a111`.

Metricas obligatorias: correctness, precision, recall, negative violations, determinism, token count, warm p95, peak RSS e incrementality. Objetivos provisionales del producto: tokens <= 0.70x, warm p95 <= 0.80x, peak RSS <= 0.50x e incremental <= 0.70x del comparador aplicable. Un target no medido o no comparable es `BLOCKED/NOT_COMPARABLE`, nunca PASS. Hot paths preexistentes conservan guard rail baseline x1.10 + 25 ms.

## 6. Invariantes

- Core nativo mantiene identidad, schema, generation, governance, security, provenance, cache keys y publicacion.
- Packs son aislados/derivativos y no escriben `index.db` primario.
- No se usa un LLM como juez unico; goldens y oraculos deterministas son obligatorios.
- Precision de edges precede comunidades, visualizacion avanzada y long-tail.
- No se declara superioridad global desde un subset o metrica unavailable.
- CLI/direct/daemon siguen sin MCP ni red obligatoria.

## 7. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_CONTEXT_BUDGET_UNSATISFIABLE` | obligatorios exceden budget | BLOCKED/truncated explicito |
| `GPH_CONTEXT_AUTHORITY_MISSING` | falta chain canonica | fail closed |
| `GPH_PACK_VERSION_UNSUPPORTED` | manifest/schema incompatible | reject pack |
| `GPH_PACK_CACHE_INVALID` | key/digest no coincide | discard/recompute |
| `GPH_PACK_RESULT_UNTRUSTED` | provenance/security incompleta | discard |
| `GPH_VICTORY_BASELINE_MISSING` | falta comparador/fixture/raw data | BLOCKED |
| `GPH_VICTORY_THRESHOLD_FAILED` | regression/target no cumplido | FAIL del slice |
| `GPH_VICTORY_NONDETERMINISTIC` | output digest difiere | FAIL |

## 8. Aceptacion y trazabilidad

- Goldens de selection, authority preservation, tie-break, token accounting y cache invalidation.
- Sandbox/no-write/no-MCP/no-network para cada pack.
- Raw 30-run evidence y report con commits, fixture digest, entorno, comando y unavailable fields.
- Cross-RID y cross-OS para core/packs publicados por RID.
- `TP-GPH / TP-GPH-006 / TC-GPH-039..048` y `TP-GPH-007 / TC-GPH-049..058`.
