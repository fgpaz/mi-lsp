---
id: RF-GPH-006
title: Calcular impacto explicable sin inflar falsos positivos
status: planned
flows:
  - FL-GPH-02
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-006"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-02]]'
  - '[[RF-GPH-004]]'
  - '[[RF-GPH-005]]'
  - '[[RF-QRY-017]]'
exports:
  - 'RF-GPH-006'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-02.md
  - .docs/wiki/04_RF/RF-GPH-004.md
  - .docs/wiki/04_RF/RF-GPH-005.md
  - .docs/wiki/04_RF/RF-QRY-017.md
  - .docs/wiki/04_RF/RF-GPH-006.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-006.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-006.md
```

# RF-GPH-006 - Calcular impacto explicable sin inflar falsos positivos

## 1. Resultado requerido

Evolucionar `nav affected` y `nav diff-context` hacia un analisis graph-native conservador que seleccione codigo, pruebas y documentos revisables. Recall adicional solo es aceptable si cada item conserva un camino explicable y no empeora el gate de precision/negative violations.

## 2. Entradas

- paths explicitos, stdin o git diff (`--changed-ref`, staged/working/untracked);
- generation publicada o activa fijada al inicio;
- `--mode direct|transitive` (`direct` por defecto);
- `--include-tests`, `--include-docs`, relation allowlist;
- depth/limit/token budgets de RF-GPH-005;
- baseline heuristico RF-QRY-017 para compatibilidad y comparacion.

## 3. Semillas y traversal

1. Resolver paths cambiados a owners y nodes declarados en la generation.
2. Adjuntar change type, source digest y simbolos cuya declaracion/superficie cambio.
3. En `direct`, recorrer solo edges exact/extracted de un salto con semantica de impacto registrada.
4. En `transitive`, ampliar BFS hasta budget usando allowlist y reglas de direccion versionadas; `inferred` se devuelve en grupo separado y no eleva prioridad exacta.
5. Mapear tests mediante edges `tests`, imports/calls exactos y convenciones legacy declaradas; toda convencion es `heuristic`.
6. Mapear docs por `doc_mentions` y anchors canonicos sin invertir autoridad.
7. Deduplicar por cross-RID/path conservando todos los caminos bounded.

No toda edge implica impacto en ambas direcciones. La tabla de relation semantics debe definir para cada relation: direccion, status admitidos, costo y si puede atravesarse transitivamente.

## 4. Salida

Cada `AffectedItem` incluye:

- `kind`, `path`, node/cross-RID y generation;
- `reason`, `confidence_class` (`exact|extracted|inferred|heuristic`);
- `evidence_path`: secuencia bounded de edges/evidence refs desde la semilla;
- `change_type`, `trigger_path`, `suggested_command` opcional;
- `warnings`/`omissions` locales.

El envelope informa `precision_scope`, relations recorridas, seeds, visited, returned, truncated, budgets, unresolved count y backend. Items exactos aparecen antes que extracted, inferred y heuristic; dentro de cada grupo se aplica orden estable.

## 5. Anti-inflation

- No se agrega un directorio/proyecto completo por proximidad textual.
- `references` textual no equivale a `calls`, `implements`, `tests`, `reads` o `writes`.
- Ambiguous/unresolved no produce item positivo; aparece en omissions con candidatos.
- Si el diff no se puede resolver semanticamente, se conserva el resultado legacy marcado `heuristic`, no se lo presenta como graph fact.
- Un incremento de recall con precision por debajo del baseline o `negative_violations > 0` bloquea estabilizacion.
- Resultados truncados no pueden afirmar cobertura completa.

## 6. Compatibilidad

Los flags y campos existentes de RF-QRY-017 permanecen; se agregan generation, evidence path y confidence class. Consumers legacy pueden ignorar campos nuevos. `backend` distingue `graph-native`, `graph-native+heuristic` o `git+catalog+heuristic`; nunca oculta fallback.

## 7. Errores tipados

| Codigo | Causa | Respuesta |
|---|---|---|
| `GPH_IMPACT_DIFF_UNAVAILABLE` | git/ref no resoluble | error o paths explicitos |
| `GPH_IMPACT_SEED_UNRESOLVED` | path sin owner/nodo | omission + fallback marcado |
| `GPH_IMPACT_RELATION_UNSUPPORTED` | relation sin semantica | reject filtro |
| `GPH_IMPACT_BUDGET_EXCEEDED` | frontier supera budget | truncation + continuation |
| `GPH_IMPACT_BASELINE_REGRESSION` | precision/correctness peor | FAIL del gate |
| `GPH_IMPACT_GRAPH_STALE` | source digest difiere | bloquear graph-native |

## 8. Aceptacion y trazabilidad

- Goldens positive, negative, ambiguous, unresolved y not-comparable.
- Precision, recall, F1 y negative violations por relation y global; reportar denominadores.
- Comparacion simultanea contra RF-QRY-017 previo y Graphify fijado, 30 repeticiones.
- Cero mutaciones del indice y paridad direct/daemon.
- `TP-GPH / TP-GPH-005 / TC-GPH-031..035`.
