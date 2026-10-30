---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-004
kind: requirement
status: draft
title: RF-GPH-004 - Actualizar el grafo incrementalmente por owner_path
links:
  implements:
  - FL-GPH-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-GPH-004 - Actualizar el grafo incrementalmente por owner_path

## Comportamiento ordenado

### 1. Resultado requerido

Recalcular solo el conjunto de ownership afectado por create/change/delete/rename, sin dejar records stale y sin convertir un incremental incierto en PASS. Cada node, edge y evidence declara un `owner_path` primario repo-relative que permite reemplazo transaccional dentro de una generación staged.

### 3. Algoritmo

1. Normalizar paths con las reglas de RF-GPH-001 y aplicar hard ignores.
2. Comparar content hash, backend version y extraction-config fingerprint.
3. Construir `invalidation_set`: paths cambiados + owners de relaciones cuya evidencia o identidad depende de una superficie publica cambiada.
4. Si el backend no puede demostrar el fanout, marcar `full_rebuild_required`; no truncar y afirmar completitud.
5. Crear una generación staged copiando por SQL los records inmutables no invalidados; no materializar el grafo entero en RAM.
6. Para cada owner invalidado, borrar/reemplazar en staging sus nodes, evidences, unresolved y edges owned; revalidar edges entrantes/salientes afectadas.
7. Tratar delete como ausencia confirmada y rename como delete + create, permitiendo conservar NodeKey solo si la identidad semántica canónica no cambia.
8. Validar la generación completa y publicar según RF-GPH-002.

### 4. Ownership

- El owner primario de un nodo es su archivo de declaracion o el artefacto sintetico versionado que lo crea.
- El owner de una edge es el source URI de la evidencia que observa el claim; evidencias adicionales mantienen su propio owner.
- Records de workspace/project/package usan un owner sintetico estable (`@workspace`, `@project/<id>`, `@package/<id>`).
- Un path puede poseer cero records; ese resultado también se sella para evitar repetir extracción indefinidamente.

### 5. Metricas observables

`paths_total`, `paths_reused`, `paths_reextracted`, `records_copied`, `records_replaced`, `records_deleted`, `fanout_paths`, `full_rebuild_required`, `stale_record_count`, `comparable_records`, `stale_rate`, elapsed y peak RSS. `stale_rate = stale_record_count / max(1, comparable_records)` y debe medirse contra una reconstruccion clean equivalente del mismo fixture.

### 6. Invariantes

- El incremental y el clean rebuild producen el mismo snapshot canónico para el mismo source/config.
- `stale_rate` objetivo de aceptacion es `0.0`; una metrica no disponible bloquea el claim.
- Cambios de backend, schema, identity rules, governance relevante o extraction config fuerzan full rebuild.
- Un path fuera de límites, unreadable o con hash conflictivo produce bloqueo/omission explicita.
- Cancelacion o crash descarta staging y conserva el activo anterior.
- El incremental no escribe durante query ni depende del daemon.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 2. Entradas

- generación activa validada;
- diff de paths con tipo de cambio y, para rename, old/new path;
- content hash por path;
- backend/extractor/config schema y sus versiones;
- dependency surface publicada por el backend;
- límites de fanout y tiempo.

## Errores

### 7. Errores tipados

- **Código**: `GPH_INCREMENTAL_BASE_MISSING`; **Causa**: no hay activo compatible; **Resultado**: full rebuild requerido
- **Código**: `GPH_INCREMENTAL_PATH_INVALID`; **Causa**: path no canonico/fuera del repo; **Resultado**: reject
- **Código**: `GPH_INCREMENTAL_HASH_CONFLICT`; **Causa**: content hash no coincide con bytes leidos; **Resultado**: abort staging
- **Código**: `GPH_INCREMENTAL_FANOUT_UNKNOWN`; **Causa**: backend no prueba dependencias; **Resultado**: full rebuild requerido
- **Código**: `GPH_INCREMENTAL_STALE_RECORD`; **Causa**: diferencia contra clean oracle; **Resultado**: FAIL; no publicar
- **Código**: `GPH_INCREMENTAL_BUDGET_EXCEEDED`; **Causa**: fanout supera límites; **Resultado**: degradar a full rebuild o BLOCKED

## Criterios de aceptación (numerados)

1. Fixtures create/change/delete/rename, cambio de firma publica, backend/config drift y cancelacion.
2. Comparacion canónica incremental versus clean con `stale_rate=0.0`.
3. 30 repeticiones con raw samples; reportar p95, RSS, bytes, records y determinismo.
4. Gate provisional del slice: tiempo incremental <= 0.70x del full rebuild comparable, sin regresion de correctness.

### 8. Aceptacion y trazabilidad

- Fixtures create/change/delete/rename, cambio de firma publica, backend/config drift y cancelacion.
- Comparacion canónica incremental versus clean con `stale_rate=0.0`.
- 30 repeticiones con raw samples; reportar p95, RSS, bytes, records y determinismo.
- Gate provisional del slice: tiempo incremental <= 0.70x del full rebuild comparable, sin regresion de correctness.
- `TP-GPH / TP-GPH-003 / TC-GPH-019..022`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-GPH-004
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-GPH-01.md
exports:
- RF-GPH-004
- RF-GPH-004 - Actualizar el grafo incrementalmente por owner_path
agent_must_read:
- .docs/wiki/05_requirements/RF-GPH-004.md
- .docs/wiki/04_flows/FL-GPH-01.md
- .docs/wiki/06_tests/TP-GPH.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-GPH-004.md
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
  más allá de RF-GPH-004.
evidence:
- .docs/wiki/05_requirements/RF-GPH-004.md
- .docs/wiki/06_tests/TP-GPH.md
```
