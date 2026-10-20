---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-011
kind: requirement
status: draft
title: RF-GPH-011 - Optimizar contexto y packs derivados con gates reproducibles
links:
  implements:
  - FL-GPH-02
  - FL-GPH-03
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-011 - Optimizar contexto y packs derivados con gates reproducibles

## Comportamiento ordenado

### 1. Resultado requerido

Seleccionar el contexto mínimo suficiente para una tarea y habilitar packs opcionales sin mover identidad, schema, generación, seguridad, provenance o autoridad fuera del core. Cada slice estable debe superar al baseline previo de mi-lsp y a Graphify en su alcance medible, sin regresion de correctness/wiki authority/determinismo.

### 2. Context Optimizer

Entrada: intent/selector, generation(s), authority chain, depth/result/token budget, required evidence classes y caller scope.

Proceso determinista:

1. fijar documentos/nodos obligatorios por gobernanza y operación;
2. generar candidatos por graph paths bounded;
3. calcular cobertura marginal de claims/evidence frente a costo canónico de tokens;
4. seleccionar greedily con tie-break estable por authority, confidence class, distance, NodeKey/doc ID;
5. deduplicar snippets/evidence sin eliminar precondiciones;
6. emitir omissions, unresolved y razones include/exclude.

Salida `ContextPack` con generation, primary/required items, optional items, evidence refs, token budget/used, coverage vector, omissions, truncated y determinism digest.

### 3. Familias de packs

- **Pack**: `Visual`; **Propósito**: export/layout derivativo y navegacion humana; **Estado inicial**: optional MILX
- **Pack**: `Languages-LSP`; **Propósito**: adapters semánticos tsserver/Pyright y futuros; **Estado inicial**: gated por RF-GPH-008
- **Pack**: `Languages-AST`; **Propósito**: parsers estructurales adicionales; **Estado inicial**: extracted, no compiler authority
- **Pack**: `Documents`; **Propósito**: formatos/document links adicionales; **Estado inicial**: derivativo, docs authority intacta
- **Pack**: `Algorithms`; **Propósito**: SCC/ciclos/centralidad/comunidades; **Estado inicial**: SCC/ciclos bounded pueden core; heavy via MILX
- **Pack**: `Stores`; **Propósito**: export/cache externo opcional; **Estado inicial**: nunca autoridad ni dependencia de query core
- **Pack**: `Graphify Import`; **Propósito**: importar snapshot externo advisory; **Estado inicial**: namespace `external:graphify`
- **Pack**: `Remote Snapshot`; **Propósito**: snapshot firmado/configurado explicitamente; **Estado inicial**: off por defecto; sin red implicita

Promover un pack exige manifest/version, security review, deterministic fixtures, resource budget, rollback y release-distribution evidence.

### 4. Cache e invalidacion

`GraphAnalysis`/pack cache se keyea por generation(s), pack ID/version/executable digest, operation, parameters, authority/profile digest y output schema. Cambiar cualquiera invalida. Cache corrupta o ausente se recomputa o reporta omission; nunca modifica active generation.

### 5. Gate de victoria

Por slice relevante, 30 repeticiones por variante con misma fixture/protocolo y comparadores fijados:

- Graphify `9bf14a4931658152969586ace39eb965c010f0d1`;
- baseline mi-lsp `a251ab1f8db4e96f029926fbef275b078a20a111`.

Metricas obligatorias: correctness, precision, recall, negative violations, determinism, token count, warm p95, peak RSS e incrementality. Objetivos provisionales del producto: tokens <= 0.70x, warm p95 <= 0.80x, peak RSS <= 0.50x e incremental <= 0.70x del comparador aplicable. Un target no medido o no comparable es `BLOCKED/NOT_COMPARABLE`, nunca PASS. Hot paths preexistentes conservan guard rail baseline x1.10 + 25 ms.

### 6. Invariantes

- Core nativo mantiene identidad, schema, generation, governance, security, provenance, cache keys y publicacion.
- Packs son aislados/derivativos y no escriben `index.db` primario.
- No se usa un LLM como juez único; goldens y oraculos deterministas son obligatorios.
- Precision de edges precede comunidades, visualizacion avanzada y long-tail.
- No se declara superioridad global desde un subset o metrica unavailable.
- CLI/direct/daemon siguen sin MCP ni red obligatoria.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 7. Errores tipados

- **Código**: `GPH_CONTEXT_BUDGET_UNSATISFIABLE`; **Causa**: obligatorios exceden budget; **Respuesta**: BLOCKED/truncated explícito
- **Código**: `GPH_CONTEXT_AUTHORITY_MISSING`; **Causa**: falta chain canónica; **Respuesta**: fail closed
- **Código**: `GPH_PACK_VERSION_UNSUPPORTED`; **Causa**: manifest/schema incompatible; **Respuesta**: reject pack
- **Código**: `GPH_PACK_CACHE_INVALID`; **Causa**: key/digest no coincide; **Respuesta**: discard/recompute
- **Código**: `GPH_PACK_RESULT_UNTRUSTED`; **Causa**: provenance/security incompleta; **Respuesta**: discard
- **Código**: `GPH_VICTORY_BASELINE_MISSING`; **Causa**: falta comparador/fixture/raw data; **Respuesta**: BLOCKED
- **Código**: `GPH_VICTORY_THRESHOLD_FAILED`; **Causa**: regression/target no cumplido; **Respuesta**: FAIL del slice
- **Código**: `GPH_VICTORY_NONDETERMINISTIC`; **Causa**: output digest difiere; **Respuesta**: FAIL

## Criterios de aceptación (numerados)

1. Goldens de selection, authority preservation, tie-break, token accounting y cache invalidation.
2. Sandbox/no-write/no-MCP/no-network para cada pack.
3. Raw 30-run evidence y report con commits, fixture digest, entorno, comando y unavailable fields.
4. Cross-RID y cross-OS para core/packs publicados por RID.

### 8. Aceptacion y trazabilidad

- Goldens de selection, authority preservation, tie-break, token accounting y cache invalidation.
- Sandbox/no-write/no-MCP/no-network para cada pack.
- Raw 30-run evidence y report con commits, fixture digest, entorno, comando y unavailable fields.
- Cross-RID y cross-OS para core/packs publicados por RID.
- `TP-GPH / TP-GPH-006 / TC-GPH-039..048` y `TP-GPH-007 / TC-GPH-049..058`.
