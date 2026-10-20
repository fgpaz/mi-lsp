---
id: RF-GPH-004
title: Actualizar el grafo incrementalmente por owner_path
status: planned
flows:
  - FL-GPH-01
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-004"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[RF-GPH-002]]'
  - '[[RF-GPH-003]]'
exports:
  - 'RF-GPH-004'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/04_RF/RF-GPH-002.md
  - .docs/wiki/04_RF/RF-GPH-003.md
  - .docs/wiki/04_RF/RF-GPH-004.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-004.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-004.md
```

# RF-GPH-004 - Actualizar el grafo incrementalmente por owner_path

## 1. Resultado requerido

Recalcular solo el conjunto de ownership afectado por create/change/delete/rename, sin dejar records stale y sin convertir un incremental incierto en PASS. Cada node, edge y evidence declara un `owner_path` primario repo-relative que permite reemplazo transaccional dentro de una generacion staged.

## 2. Entradas

- generacion activa validada;
- diff de paths con tipo de cambio y, para rename, old/new path;
- content hash por path;
- backend/extractor/config schema y sus versiones;
- dependency surface publicada por el backend;
- limites de fanout y tiempo.

## 3. Algoritmo

1. Normalizar paths con las reglas de RF-GPH-001 y aplicar hard ignores.
2. Comparar content hash, backend version y extraction-config fingerprint.
3. Construir `invalidation_set`: paths cambiados + owners de relaciones cuya evidencia o identidad depende de una superficie publica cambiada.
4. Si el backend no puede demostrar el fanout, marcar `full_rebuild_required`; no truncar y afirmar completitud.
5. Crear una generacion staged copiando por SQL los records inmutables no invalidados; no materializar el grafo entero en RAM.
6. Para cada owner invalidado, borrar/reemplazar en staging sus nodes, evidences, unresolved y edges owned; revalidar edges entrantes/salientes afectadas.
7. Tratar delete como ausencia confirmada y rename como delete + create, permitiendo conservar NodeKey solo si la identidad semantica canonica no cambia.
8. Validar la generacion completa y publicar segun RF-GPH-002.

## 4. Ownership

- El owner primario de un nodo es su archivo de declaracion o el artefacto sintetico versionado que lo crea.
- El owner de una edge es el source URI de la evidencia que observa el claim; evidencias adicionales mantienen su propio owner.
- Records de workspace/project/package usan un owner sintetico estable (`@workspace`, `@project/<id>`, `@package/<id>`).
- Un path puede poseer cero records; ese resultado tambien se sella para evitar repetir extraccion indefinidamente.

## 5. Metricas observables

`paths_total`, `paths_reused`, `paths_reextracted`, `records_copied`, `records_replaced`, `records_deleted`, `fanout_paths`, `full_rebuild_required`, `stale_record_count`, `comparable_records`, `stale_rate`, elapsed y peak RSS. `stale_rate = stale_record_count / max(1, comparable_records)` y debe medirse contra una reconstruccion clean equivalente del mismo fixture.

## 6. Invariantes

- El incremental y el clean rebuild producen el mismo snapshot canonico para el mismo source/config.
- `stale_rate` objetivo de aceptacion es `0.0`; una metrica no disponible bloquea el claim.
- Cambios de backend, schema, identity rules, governance relevante o extraction config fuerzan full rebuild.
- Un path fuera de limites, unreadable o con hash conflictivo produce bloqueo/omission explicita.
- Cancelacion o crash descarta staging y conserva el activo anterior.
- El incremental no escribe durante query ni depende del daemon.

## 7. Errores tipados

| Codigo | Causa | Resultado |
|---|---|---|
| `GPH_INCREMENTAL_BASE_MISSING` | no hay activo compatible | full rebuild requerido |
| `GPH_INCREMENTAL_PATH_INVALID` | path no canonico/fuera del repo | reject |
| `GPH_INCREMENTAL_HASH_CONFLICT` | content hash no coincide con bytes leidos | abort staging |
| `GPH_INCREMENTAL_FANOUT_UNKNOWN` | backend no prueba dependencias | full rebuild requerido |
| `GPH_INCREMENTAL_STALE_RECORD` | diferencia contra clean oracle | FAIL; no publicar |
| `GPH_INCREMENTAL_BUDGET_EXCEEDED` | fanout supera limites | degradar a full rebuild o BLOCKED |

## 8. Aceptacion y trazabilidad

- Fixtures create/change/delete/rename, cambio de firma publica, backend/config drift y cancelacion.
- Comparacion canonica incremental versus clean con `stale_rate=0.0`.
- 30 repeticiones con raw samples; reportar p95, RSS, bytes, records y determinismo.
- Gate provisional del slice: tiempo incremental <= 0.70x del full rebuild comparable, sin regresion de correctness.
- `TP-GPH / TP-GPH-003 / TC-GPH-019..022`.
