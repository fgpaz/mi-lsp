---
id: RF-DAE-005
doc_id: RF-DAE-005
kind: requirement
audience: llm-first
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
wiki_graph_protocol: SDD-WIKI-GRAPH-v1
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-DAE-002]]'
  - '[[TP-DAE]]'
exports:
  - 'RF-DAE-005'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-DAE-002.md
  - .docs/wiki/04_RF/RF-DAE-005.md
  - .docs/wiki/06_pruebas/TP-DAE.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-DAE-005.md
  - .docs/wiki/06_pruebas/TP-DAE.md
  - .docs/wiki/06_matriz_pruebas_RF.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-DAE-005.md
  - .docs/wiki/06_pruebas/TP-DAE.md
---

# Estadísticas de telemetría local por cliente

```toon
block_id: rf-dae-005-telemetry-stats
kind: normative
source_of_truth: this
status: implemented
purpose: resumir telemetría local por cliente sin exponer contenido de consultas ni archivos
inputs:
  command: mi-lsp stats
  flags:
    by_client: boolean, optional
    by_op: boolean, optional
    days: positive_integer, default=7
    format: json|compact, default=json
behavior:
  - Si no se selecciona --by-client ni --by-op, mostrar el desglose por cliente.
  - Leer access_events local desde daemon.db; no iniciar navegación semántica ni enviar datos a red.
  - Agrupar por client_name o 'unknown' si falta.
  - Para cada grupo, informar calls, ok_pct, p50_ms, p90_ms, p50_bytes, p90_bytes y fallback_pct.
  - Considerar fallback los eventos con route=direct_fallback o routing_outcome que indique fallback.
  - La salida solo incluye agregados; no incluir patrones, texto, argv, errores crudos ni contenido de archivos.
  - --by-op agrega un desglose análogo por operación; puede combinarse con --by-client.
outputs:
  fields: [days, total_calls, by_client?, by_op?]
  group_fields: [calls, ok_pct, p50_ms, p90_ms, p50_bytes, p90_bytes, fallback_pct]
implementation_bindings:
  - internal/cli/stats.go
  - internal/daemon/export.go
  - internal/daemon/state_store.go
verification:
  - TP-DAE/TC-DAE-027
  - TP-DAE/TC-DAE-028
stop_gates:
  - El error al abrir o consultar daemon.db se informa; no se presenta como cero actividad.
  - --days <= 0 se rechaza sin consulta.
  - Los agregados no contienen valores de consulta ni paths.
durable_evidence:
  - .docs/wiki/06_pruebas/TP-DAE.md
```
