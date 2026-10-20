---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-006
kind: requirement
status: draft
title: RF-GPH-006 - Calcular impacto explicable sin inflar falsos positivos
links:
  implements:
  - FL-GPH-02
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-006 - Calcular impacto explicable sin inflar falsos positivos

## Comportamiento ordenado

### 1. Resultado requerido

Evolucionar `nav affected` y `nav diff-context` hacia un analisis graph-native conservador que seleccione código, pruebas y documentos revisables. Recall adicional solo es aceptable si cada item conserva un camino explicable y no empeora el gate de precision/negative violations.

### 3. Semillas y traversal

1. Resolver paths cambiados a owners y nodes declarados en la generation.
2. Adjuntar change type, source digest y símbolos cuya declaracion/superficie cambio.
3. En `direct`, recorrer solo edges exact/extracted de un salto con semántica de impacto registrada.
4. En `transitive`, ampliar BFS hasta budget usando allowlist y reglas de direccion versionadas; `inferred` se devuelve en grupo separado y no eleva prioridad exacta.
5. Mapear tests mediante edges `tests`, imports/calls exactos y convenciones legacy declaradas; toda convencion es `heuristic`.
6. Mapear docs por `doc_mentions` y anchors canónicos sin invertir autoridad.
7. Deduplicar por cross-RID/path conservando todos los caminos bounded.

No toda edge implica impacto en ambas direcciones. La tabla de relation semantics debe definir para cada relation: direccion, status admitidos, costo y si puede atravesarse transitivamente.

### 5. Anti-inflation

- No se agrega un directorio/proyecto completo por proximidad textual.
- `references` textual no equivale a `calls`, `implements`, `tests`, `reads` o `writes`.
- Ambiguous/unresolved no produce item positivo; aparece en omissions con candidatos.
- Si el diff no se puede resolver semanticamente, se conserva el resultado legacy marcado `heuristic`, no se lo presenta como graph fact.
- Un incremento de recall con precision por debajo del baseline o `negative_violations > 0` bloquea estabilizacion.
- Resultados truncados no pueden afirmar cobertura completa.

### 6. Compatibilidad

Los flags y campos existentes de RF-QRY-017 permanecen; se agregan generation, evidence path y confidence class. Consumers legacy pueden ignorar campos nuevos. `backend` distingue `graph-native`, `graph-native+heuristic` o `git+catalog+heuristic`; nunca oculta fallback.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 2. Entradas

- paths explícitos, stdin o git diff (`--changed-ref`, staged/working/untracked);
- generation publicada o activa fijada al inicio;
- `--mode direct|transitive` (`direct` por defecto);
- `--include-tests`, `--include-docs`, relation allowlist;
- depth/limit/token budgets de RF-GPH-005;
- baseline heuristico RF-QRY-017 para compatibilidad y comparacion.

### 4. Salida

Cada `AffectedItem` incluye:

- `kind`, `path`, node/cross-RID y generation;
- `reason`, `confidence_class` (`exact|extracted|inferred|heuristic`);
- `evidence_path`: secuencia bounded de edges/evidence refs desde la semilla;
- `change_type`, `trigger_path`, `suggested_command` opcional;
- `warnings`/`omissions` locales.

El envelope informa `precision_scope`, relations recorridas, seeds, visited, returned, truncated, budgets, unresolved count y backend. Items exactos aparecen antes que extracted, inferred y heuristic; dentro de cada grupo se aplica orden estable.

## Errores

### 7. Errores tipados

- **Código**: `GPH_IMPACT_DIFF_UNAVAILABLE`; **Causa**: git/ref no resoluble; **Respuesta**: error o paths explícitos
- **Código**: `GPH_IMPACT_SEED_UNRESOLVED`; **Causa**: path sin owner/nodo; **Respuesta**: omission + fallback marcado
- **Código**: `GPH_IMPACT_RELATION_UNSUPPORTED`; **Causa**: relation sin semántica; **Respuesta**: reject filtro
- **Código**: `GPH_IMPACT_BUDGET_EXCEEDED`; **Causa**: frontier supera budget; **Respuesta**: truncation + continuation
- **Código**: `GPH_IMPACT_BASELINE_REGRESSION`; **Causa**: precision/correctness peor; **Respuesta**: FAIL del gate
- **Código**: `GPH_IMPACT_GRAPH_STALE`; **Causa**: source digest difiere; **Respuesta**: `nav.affected`: warning stale explícito y fallback heurístico; `nav.diff-context`: propagar error tipado; wrappers `change-pack` e `intent`: conservar su superficie de warning/error sanitizado, sin fallback heurístico stale

Antes de resolver seeds, `nav.affected` revisa solo los paths explícitos y, si cambiaron, refresca catálogo y `doc_records` con límite de 1 s. La actualización es síncrona, transaccional y no reconstruye la generation del grafo ni inicia escrituras de fondo. Si la generation graph-native está stale, solo `nav.affected` conserva resultados heurísticos con `confidence=0`, omite claims graph-native y emite un warning explícito `GPH_IMPACT_GRAPH_STALE` con el comando `mi-lsp index --workspace "<alias>"`. `nav.diff-context` propaga el error tipado `GPH_IMPACT_GRAPH_STALE`. `change-pack` e `intent` consumen ese fallo en su wrapper: no heredan el fallback heurístico de `nav.affected` y conservan su respuesta de warning/error sanitizado (`INTENT_DIFF_UNAVAILABLE` en intent con `graph_query_error`). Si un seed sigue sin resolverse, permanece como omission; no se infiere owner ni se presenta la heurística como evidencia graph-native. La reconciliación scoped de un archivo ausente desde `nav.find` también deja stale el grafo; hasta reindexar, ninguna respuesta debe atribuirle evidencia graph-native.

## Criterios de aceptación (numerados)

1. Goldens positive, negative, ambiguous, unresolved y not-comparable.
2. Precision, recall, F1 y negative violations por relation y global; reportar denominadores.
3. Comparacion simultanea contra RF-QRY-017 previo y Graphify fijado, 30 repeticiones.
4. Cero mutaciones del índice y paridad direct/daemon.

### 8. Aceptacion y trazabilidad

- Goldens positive, negative, ambiguous, unresolved y not-comparable.
- Precision, recall, F1 y negative violations por relation y global; reportar denominadores.
- Comparacion simultanea contra RF-QRY-017 previo y Graphify fijado, 30 repeticiones.
- Cero mutaciones del índice y paridad direct/daemon.
- `TP-GPH / TP-GPH-005 / TC-GPH-031..035`.
