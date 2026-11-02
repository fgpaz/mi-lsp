---
wiki_contract: WIKI-CONTRACT-v1
id: TP-IDX-OWNERSHIP
kind: test
status: active
title: TP-IDX-OWNERSHIP — Pruebas de ownership, fencing y exclusión
links:
  verifies:
  - RF-IDX-001
  - RF-IDX-003
  - RF-IDX-004
---

## Alcance

RF-IDX-001, RF-IDX-003, RF-IDX-004. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 1
  - RF-IDX-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: La CLI resuelve el workspace objetivo y carga `project.toml`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 2
  - RF-IDX-001 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: El indexer obtiene ignores desde defaults internos, `.gitignore`, `.milspignore` y `[ignore].extra_patterns`, respetando el orden del archivo y los re-includes negados (`!pattern`) sobre paths normalizados con `/`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 3
  - RF-IDX-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: La CLI crea un job durable y una generación candidata en `index.db`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 4
  - RF-IDX-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: Si `--clean` esta activo, fuerza recomposicion completa del modo elegido, sin borrar `index.db` antes de construir el nuevo resultado.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 5
  - RF-IDX-001 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: El walker enumera archivos y asigna ownership por `repo_id`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 6
  - RF-IDX-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: El extractor prepara `workspace_repos`, `workspace_entrypoints`, `FileRecord`, `SymbolRecord` y `WorkspaceMeta`, reportando progreso vivo durante loops largos de catalogo/docs. En Go, conserva la ruta exacta del entrypoint relativa al workspace (incluido `runtime/go.mod`). El selector que consume el observador puede ser una ruta de módulo repo-local explícita (`go.mod`) o un ID de `WorkspaceEntrypoint`; un ID se resuelve con coincidencia exacta de repo y entrypoint, se rebasa su ruta workspace-relative al root del repo seleccionado y se revalida como `go.mod` repo-local seguro y regular. Si el selector repo-local está vacío, usa el fallback al `go.mod` del root; un ID desconocido o malformado, una selección explícita insegura, inexistente o no regular, incluido un symlink final, falla cerrado y queda como omisión/diagnóstico (`""`), sin activar el fallback. `go.work` no tiene extracción de grafo en este slice y un selector explícito `go.work` se rechaza.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 7
  - RF-IDX-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: El indexador documental prepara `DocRecord`, `DocEdge` y `DocMention` a partir de `.docs/wiki`, `README*`, `docs/` y `.docs/`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 8
  - RF-IDX-001 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: El runtime publica catálogo, docs y memoria de reentrada en una única transaccion SQLite para `mode=full`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-001 / Criterio 9
  - RF-IDX-001 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-001 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: La CLI devuelve job status, stats y warnings no fatales si encontro ruido, archivos sin match de repo o problemas de docs.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-003 / Criterio 1
  - RF-IDX-003 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: El core lee `.docs/wiki/00_gobierno_documental.md`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-003 / Criterio 2
  - RF-IDX-003 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-003 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: Extrae el bloque YAML de gobernanza y valida schema comun estricto.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-003 / Criterio 3
  - RF-IDX-003 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-003 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: Compila el perfil visible (`ordered_wiki`, `spec_backend`, `spec_full`, `custom`) a `base + overlays`.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-003 / Criterio 4
  - RF-IDX-003 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-003 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: Proyecta `.docs/wiki/_mi-lsp/read-model.toml` como contrato ejecutable versionado.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-003 / Criterio 5
  - RF-IDX-003 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-003 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: Si el projection drift existe, auto-sincroniza el archivo de proyeccion.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-003 / Criterio 6
  - RF-IDX-003 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-003 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: Si cambia `00` o cambia la proyección, el workspace debe reindexarse antes de continuar con superficies docs-first.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 1
  - RF-IDX-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: un workspace_root solo puede tener una reserva mutante; la creación del job, la generación candidata y la fila de ownership ocurren en una transacción SQLite. Evidencia esperada: dos starts concurrentes producen un único job reservado y un único ActiveIndexJobError; nunca quedan dos reservas activas.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 2
  - RF-IDX-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: cada reserva conserva owner_token aleatorio y fencing_token monotónico por workspace; un token liberado no se reutiliza. Evidencia esperada: una sustitución solo pasa después de un CAS de liberación y el nuevo fencing_token es mayor que el anterior.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 3
  - RF-IDX-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: capability clearing elimina capacidades de ejecución en memoria y en control-plane cuando ownership deja de estar vigente; el control-plane no puede convertir una solicitud externa en owner. Evidencia esperada: tras clearing, el worker pierde las capacidades publish/terminal y el control-plane solo puede marcar requested_cancel mediante CAS.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 4
  - RF-IDX-004 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: PID 0 se interpreta con la semántica moderna de proceso desconocido/no verificable y no se presume vivo ni muerto sin evidencia de plataforma. Evidencia esperada: una fila PID0 moderna se conserva para inspección segura y no permite retirar un lock vivo por heurística.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 5
  - RF-IDX-004 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: las filas legacy con PID0 o formato antiguo se normalizan de forma aditiva; ningún job o generación existente se elimina o mueve durante la migración. Evidencia esperada: una base legacy queda legible, su capacidad se limita a status/recovery y solo se reserva después de stale confirmado.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 6
  - RF-IDX-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: la migración legacy concurrente usa CAS/unique workspace_root y no puede crear dos ownership activos ni perder el fence vencedor. Evidencia esperada: N migraciones concurrentes convergen en una fila vigente y todas las perdedoras devuelven conflicto recuperable.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 7
  - RF-IDX-004 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: una cancelación de un job queued pasa inmediatamente a canceled sin spawn; la reserva se libera solo con el terminal owner-bound. Evidencia esperada: no aparece proceso worker, el lock no se elimina antes de la transición válida y el job no queda failed.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 8
  - RF-IDX-004 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: la cancelación cooperativa escribe requested_cancel y el worker confirma canceled solo después de observar la solicitud y alcanzar una frontera sin publicación. Evidencia esperada: requested_cancel no es terminal y el estado final es canceled con un único ganador.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 9
  - RF-IDX-004 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: un force cancel que no confirma la salida del PID conserva requested_cancel, fase terminating, reserva y lock. Evidencia esperada: el timeout devuelve resultado operativo de terminación y no libera ownership ni marca failed.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 10
  - RF-IDX-004 — Criterio 10. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 10. Disposición: manual_evidence_pending. Evidencia esperada: un fence stale o canceled no puede escribir terminal, runtime, active generation ni memory pointer. Evidencia esperada: RowsAffected es cero o ErrStaleIndexJobOwner y todos los punteros y runtime conservan el snapshot vencedor.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 11
  - RF-IDX-004 — Criterio 11. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 11. Disposición: manual_evidence_pending. Evidencia esperada: la carrera entre cancelación y publicación termina en canceled o succeeded según el primer commit owner-bound; nunca termina failed por una cancelación válida. Evidencia esperada: existe exactamente un terminal inmutable, no hay publicación parcial y el job no se clasifica failed por perder la carrera de cancelación.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 12
  - RF-IDX-004 — Criterio 12. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 12. Disposición: manual_evidence_pending. Evidencia esperada: full, docs, catalog, graph e incremental construyen una generación candidata y publican solo con el fence vigente del job. Evidencia esperada: cada modo actualiza únicamente sus superficies declaradas y no activa una generación de otro modo.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 13
  - RF-IDX-004 — Criterio 13. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 13. Disposición: manual_evidence_pending. Evidencia esperada: la publicación y la transición terminal son una unidad durable; si falla el commit, los punteros y el runtime permanecen en el snapshot previo. Evidencia esperada: nunca se observa pointer nuevo con job failed/canceled ni job succeeded sin generación activa válida.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 14
  - RF-IDX-004 — Criterio 14. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 14. Disposición: manual_evidence_pending. Evidencia esperada: una publicación stale o cancelada no modifica active_catalog_generation_id, active_docs_generation_id, active_graph_generation_id, memory_pointer ni runtime metadata. Evidencia esperada: el snapshot anterior es byte/valor estable después del intento rechazado.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 15
  - RF-IDX-004 — Criterio 15. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 15. Disposición: manual_evidence_pending. Evidencia esperada: embeddings se ejecutan después de publicar y son best-effort; su timeout o indisponibilidad no revierte una publicación válida ni convierte el job en failed. Evidencia esperada: el job sigue succeeded, se registra warning sanitizado y los embeddings incompletos no se presentan como completos.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 16
  - RF-IDX-004 — Criterio 16. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 16. Disposición: manual_evidence_pending. Evidencia esperada: un batch del watcher realiza tres reintentos inmediatos y como máximo una reintentada diferida; un evento nuevo rearma el ciclo sin storm. Evidencia esperada: cuatro intentos totales por ciclo, una sola diferida y ningún timer duplicado para el mismo evento coalescido.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 17
  - RF-IDX-004 — Criterio 17. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 17. Disposición: manual_evidence_pending. Evidencia esperada: un lock stale se pone en quarantine antes de reclamarlo; la limpieza compara PID, owner_token y started_at y nunca retira un owner reemplazado. Evidencia esperada: el owner replacement sobrevive a cualquier callback stale, incluso después de quarantine.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-IDX-OWNERSHIP / RF-IDX-004 / Criterio 18
  - RF-IDX-004 — Criterio 18. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-IDX-004 — Criterio 18. Disposición: manual_evidence_pending. Evidencia esperada: RemoveWorkspaceIndexLockForPID nunca retira un PID vivo y trata PID0 moderno y legacy como no verificables hasta contar con evidencia segura. Evidencia esperada: un PID0 o PID vivo conserva el lock y devuelve contención/diagnóstico, no eliminación.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

### TP-IDX-OWNERSHIP — Pruebas de ownership, fencing y exclusión
  - RF-IDX-001 — Criterio 3; RF-IDX-003 — Criterio 3; RF-IDX-004 — Criterio 14. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.



Enlaces relacionados: [[RF-IDX-004]] · [[CT-INDEX-JOB-OWNERSHIP]] · [[TP-IDX]] · [[FL-IDX-01]]

### 1. Ciclo de vida y cancelación

- **Marcador estable**: TP-IDX-OWNERSHIP.lifecycle
- **Comprobación prevista**:
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(MarkIndexJobProgressUpdatesCountersStageAndTimestamp|CancelIndexJobForceTimeoutRetainsReservation|CancelIndexJobForceTerminatesProcessAndMarksCanceled|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|IndexJobCancellationWinsOverStaleFailureWriter)$'
  - go test -v -count=1 -p 1 ./internal/service -run '^TestRunIndexJob(HonorsCooperativeCancelDuringProgress|ReconcilesPublicationWinsDuringCancel)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^TestIndexJobRead(LoadersPreserveCancelRequestedAfterStaleFailureCAS|PropagatesMarkFailedDatabaseError|PropagatesReloadErrorAfterStaleCASRace)$'
- **Evidencia referenciada**:
  - internal/store/index_jobs.go
  - internal/store/index_jobs.go
  - internal/indexer/indexer.go
- **Registros**:
  - **id**: TC-IDX-OWNERSHIP-001
  - RF-IDX-001 — Criterio 3; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: liveness
  - **scenario**: progress/liveness
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp$'
  - **test**: TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp
  - **setup**: t.TempDir with a local SQLite database and a cancellable index loop
  - **Oráculo**: updated_at, stage, current_path, files_total and counters advance while the worker is alive; cancellation is observed at a loop boundary
  - **Condiciones de rechazo**: progress_timestamp_stalls_before_completion=true
  - **id**: TC-IDX-OWNERSHIP-002
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 5; RF-IDX-004 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: cancellation
  - **scenario**: queued immediate cancel
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestCancelIndexJobQueuedImmediate$'
  - **test**: TestCancelIndexJobQueuedImmediate
  - **setup**: create a queued job with an ownership row but do not spawn a worker
  - **Oráculo**: cancel transitions queued to canceled without spawn; ownership is released only by the owner-bound terminal transition; result is never failed
  - **Condiciones de rechazo**: queued_cancel_spawns_worker=true
  - **id**: TC-IDX-OWNERSHIP-003
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 5; RF-IDX-004 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: cancellation
  - **scenario**: cooperative running cancel
  - **command**: go test -v -count=1 -p 1 ./internal/service -run '^TestRunIndexJobHonorsCooperativeCancelDuringProgress$'
  - **test**: TestRunIndexJobHonorsCooperativeCancelDuringProgress
  - **setup**: running worker, requested_cancel CAS and a publication barrier that has not committed
  - **Oráculo**: worker confirms canceled only after no-publication boundary; requested_cancel is not terminal and exactly one terminal winner remains
  - **Condiciones de rechazo**: canceled_written_before_worker_exit=true
  - **id**: TC-IDX-OWNERSHIP-004
  - RF-IDX-001 — Criterio 1; RF-IDX-003 — Criterio 5; RF-IDX-004 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: cancellation
  - **scenario**: force timeout
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestCancelIndexJobForceTimeoutRetainsReservation$'
  - **test**: TestCancelIndexJobForceTimeoutRetainsReservation
  - **setup**: process that ignores termination and a matching workspace lock
  - **Oráculo**: timeout leaves requested_cancel=1, phase=terminating, reservation and live lock intact; a confirmed dead PID may be cleaned safely
  - **Condiciones de rechazo**: live_lock_missing_after_timeout=true
  - **id**: TC-IDX-OWNERSHIP-005
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 3; RF-IDX-004 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: capability
  - **scenario**: capability clearing in memory/control-plane
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestControlPlaneProjectionCannotExerciseOwnerRights$'
  - **test**: TestControlPlaneProjectionCannotExerciseOwnerRights
  - **setup**: worker fence invalidated by stale, canceled or released ownership
  - **Oráculo**: in-memory publish/terminal capabilities are cleared; control-plane retains only status and cancel CAS and cannot mint or adopt a fence
  - **Condiciones de rechazo**: control_plane_publishes_without_owner=true

### 2. PID y migración legacy

- **Marcador estable**: TP-IDX-OWNERSHIP.legacy
- **Comprobación prevista**:
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(LegacyPID0ModernSemantics|LegacyPID0Compatibility|IndexJobReadCompatibilityWithoutRequestedCancelColumn|ConcurrentLegacyOwnershipMigration|RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess|Open_MigratesLegacyRepoColumns)$'
- **Evidencia referenciada**:
  - internal/store/index_lock.go
  - internal/store/index_publish.go
  - internal/store/index_jobs.go
- **Registros**:
  - **id**: TC-IDX-OWNERSHIP-006
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 18. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: process-safety
  - **scenario**: PID0 moderno
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestLegacyPID0ModernSemantics$'
  - **test**: TestLegacyPID0ModernSemantics
  - **setup**: ownership/lock row with PID=0 in the current schema
  - **Oráculo**: PID0 is unknown/not-verifiable; status and recovery remain safe and no live lock is removed by PID heuristic
  - **Condiciones de rechazo**: pid0_assumed_dead=true
  - **id**: TC-IDX-OWNERSHIP-007
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 5; RF-IDX-004 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: compatibility
  - **scenario**: PID0 legacy
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestLegacyPID0Compatibility$'
  - **test**: TestLegacyPID0Compatibility
  - **setup**: pre-migration schema/row with PID0 or absent requested_cancel
  - **Oráculo**: additive migration preserves jobs and generations, exposes a conservative status, and permits reservation only after confirmed stale evidence
  - **Condiciones de rechazo**: legacy_job_deleted=true
  - **id**: TC-IDX-OWNERSHIP-008
  - RF-IDX-001 — Criterio 4; RF-IDX-003 — Criterio 3; RF-IDX-004 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: concurrency
  - **scenario**: migración legacy concurrente
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestConcurrentLegacyOwnershipMigration$'
  - **test**: TestConcurrentLegacyOwnershipMigration
  - **setup**: N goroutines open and migrate the same legacy index.db in t.TempDir
  - **Oráculo**: unique workspace_root and CAS converge to one ownership row; no duplicate active owner and no lost fence
  - **Condiciones de rechazo**: duplicate_active_ownership=true

### 3. Fencing y publicación por modo

- **Marcador estable**: TP-IDX-OWNERSHIP.publication
- **Comprobación prevista**:
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(IndexJobStaleFenceCannotWinAfterTerminalRelease|IndexJobTerminalRaceHasSingleImmutableWinner|FencedPublicationModeMatrix|ControlPlaneProjectionCannotExerciseOwnerRights|FencedGraphPointerActivation|FencedGraphExpectedPriorActivation|StaleOrCanceledGraphFencePreservesPointerAndRuntime|PublicationAndTerminalCommitAtomically|IncrementalPublicationCanceledFencePreservesFilesAndSymbols|IncrementalPublicationStaleFencePreservesFilesAndSymbols)$'
  - go test -v -count=1 -p 1 ./internal/indexer -run '^Test(FullIndexPublishesLocalGoGraph|IncrementalIndexPublishesNewGraphGenerationAfterFileChange|IncrementalIndexObservationFailureLeavesGraphStale)$'
  - go test -v -count=1 -p 1 ./internal/service -run '^Test(EmbeddingFailureAfterPublicationLeavesSucceededGeneration|EmbeddingIndexPersistsSuccessfulBatchesAndResumes|Recall_BilingualEStoEN|Recall_EmbeddingsImplicitlyActiveWithoutEnabled)$'
- **Evidencia referenciada**:
  - internal/store/index_jobs.go
  - internal/store/index_publish.go
  - internal/store/index_publish.go
  - internal/indexer/graph_pipeline.go
  - internal/indexer/incremental.go
  - internal/service/recall.go
  - internal/service/index_jobs.go
- **Registros**:
  - **id**: TC-IDX-OWNERSHIP-009
  - RF-IDX-001 — Criterio 8; RF-IDX-003 — Criterio 5; RF-IDX-004 — Criterio 12. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: publication
  - **scenario**: publicación fenced full/docs/catalog/graph/incremental
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestFencedGraphPointerActivation$'
  - **test**: TestFencedGraphPointerActivation
  - **setup**: candidate generations for modes full, docs, catalog, graph and incremental with independent fences
  - **Oráculo**: each mode publishes only its declared surfaces and rejects a fence belonging to another mode
  - **Condiciones de rechazo**: cross_mode_pointer_changed=true
  - **id**: TC-IDX-OWNERSHIP-010
  - RF-IDX-001 — Criterio 8; RF-IDX-003 — Criterio 6; RF-IDX-004 — Criterio 10. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: fencing
  - **scenario**: stale/canceled fence sin cambios de pointers/runtime
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestStaleOrCanceledGraphFencePreservesPointerAndRuntime$'
  - **test**: TestStaleOrCanceledGraphFencePreservesPointerAndRuntime
  - **setup**: snapshot with active catalog/docs/graph pointers and a stale or canceled candidate
  - **Oráculo**: ErrStaleIndexJobOwner or zero RowsAffected; active_*_generation_id, memory_pointer, runtime metadata, files and symbols remain unchanged
  - **Condiciones de rechazo**: stale_writer_mutated_runtime=true
  - **id**: TC-IDX-OWNERSHIP-011
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 11. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: atomicity
  - **scenario**: atomicidad publication-terminal
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestPublicationAndTerminalCommitAtomically$'
  - **test**: TestPublicationAndTerminalCommitAtomically
  - **setup**: inject a commit failure between candidate activation and terminal transition
  - **Oráculo**: transaction rollback leaves prior pointers and terminal state; no succeeded job points to an invisible generation and no failed job points to a new active generation
  - **Condiciones de rechazo**: partial_publication_visible=true
  - **id**: TC-IDX-OWNERSHIP-012
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 3; RF-IDX-004 — Criterio 15. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: best-effort
  - **scenario**: embeddings post-publicación
  - **command**: go test -v -count=1 -p 1 ./internal/service -run '^TestEmbeddingFailureAfterPublicationLeavesSucceededGeneration$'
  - **test**: TestEmbeddingFailureAfterPublicationLeavesSucceededGeneration
  - **setup**: valid published generation and an embedding provider that times out or returns partial batches
  - **Oráculo**: publication remains succeeded, warning is sanitized, and incomplete embeddings are not advertised as complete
  - **Condiciones de rechazo**: embedding_failure_rolled_back_valid_publication=true

### 4. Watcher, locks y carreras

- **Marcador estable**: TP-IDX-OWNERSHIP.watcher-lock-races
- **Comprobación prevista**:
  - go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherBatchRetry(UsesThreeImmediateRetries|StopsAfterOneDeferredFailure|NewEventRearmsExhaustedCycle)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(WithWorkspaceIndexLockRejectsConcurrentIndexRun|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|WorkspaceIndexLockDoesNotRemoveReplacementOwner|WorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine|RemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine|RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(CancelPublicationRaceNeverEndsFailed|IndexJobCancellationWinsOverStaleFailureWriter)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether$'
  - go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether$'
- **Evidencia referenciada**:
  - internal/daemon/file_watcher.go
  - internal/store/index_lock.go
  - internal/store/index_jobs.go
- **Registros**:
  - **id**: TC-IDX-OWNERSHIP-013
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 16. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: watcher
  - **scenario**: watcher tres inmediatos y una única diferida sin storm
  - **command**: go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherBatchRetryUsesThreeImmediateRetries$'
  - **test**: TestWatcherBatchRetryUsesThreeImmediateRetries
  - **setup**: coalesced watcher event with deterministic contention and one new event after exhaustion
  - **Oráculo**: three immediate retries plus one deferred retry, exactly one timer, and a new event rearms one fresh cycle without storm
  - **Condiciones de rechazo**: retry_timer_count > 1
  - **id**: TC-IDX-OWNERSHIP-014
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 17. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: lock
  - **scenario**: lock quarantine y replacement owner-bound
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestWorkspaceIndexLockDoesNotRemoveReplacementOwner$'
  - **test**: TestWorkspaceIndexLockDoesNotRemoveReplacementOwner
  - **setup**: stale lock quarantined, replacement owner claims the same path, then the stale callback runs
  - **Oráculo**: cleanup compares PID, owner_token and started_at; replacement lock survives and a live PID is never removed
  - **Condiciones de rechazo**: replacement_owner_lock_removed=true
  - **id**: TC-IDX-OWNERSHIP-015
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 11. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: race
  - **scenario**: carrera cancelación/publicación que no puede acabar failed
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestCancelPublicationRaceAtCommitBoundary$'
  - **test**: TestCancelPublicationRaceAtCommitBoundary
  - **setup**: synchronize cancellation CAS, publication commit and stale failure writer at the same barrier
  - **Oráculo**: exactly one terminal canceled or succeeded wins; a valid cancellation loser is stale/recoverable and the final job is never failed
  - **Condiciones de rechazo**: cancellation_publication_race_failed=true
  - **id**: TC-IDX-OWNERSHIP-016
  - RF-IDX-001 — Criterio 8; RF-IDX-003 — Criterio 6; RF-IDX-004 — Criterio 10. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: atomicity
  - **scenario**: incremental cancelado no deriva files/symbols
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestIncrementalPublicationCanceledFencePreservesFilesAndSymbols$'
  - **test**: TestIncrementalPublicationCanceledFencePreservesFilesAndSymbols
  - **setup**: catálogo incremental previo, cancel_requested confirmado y cambios de files/symbols preparados en memoria
  - **Oráculo**: el fence cancelado se rechaza antes de escribir y files/symbols permanecen byte/row-equivalentes al snapshot previo
  - **Condiciones de rechazo**: canceled_incremental_catalog_drift=true
  - **id**: TC-IDX-OWNERSHIP-017
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 10. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: atomicity
  - **scenario**: incremental stale no deriva files/symbols
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestIncrementalPublicationStaleFencePreservesFilesAndSymbols$'
  - **test**: TestIncrementalPublicationStaleFencePreservesFilesAndSymbols
  - **setup**: catálogo incremental previo y fence liberado por un terminal succeeded antes del intento stale
  - **Oráculo**: el owner stale se rechaza antes de escribir y files/symbols permanecen equivalentes al snapshot previo
  - **Condiciones de rechazo**: stale_incremental_catalog_drift=true
  - **id**: TC-IDX-OWNERSHIP-018
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 11. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: race
  - **scenario**: publication-wins reconciliada por runIndexJob
  - **command**: go test -v -count=1 -p 1 ./internal/service -run '^TestRunIndexJobReconcilesPublicationWinsDuringCancel$'
  - **test**: TestRunIndexJobReconcilesPublicationWinsDuringCancel
  - **setup**: cancel_requested y publicación terminal succeeded sincronizados antes del CAS de cancelación owner-bound
  - **Oráculo**: MarkIndexJobCanceled stale se reconcilia como succeeded; runIndexJob no propaga ErrStaleIndexJobOwner ni marca failed
  - **Condiciones de rechazo**: publication_wins_propagates_stale_error=true
  - **id**: TC-IDX-OWNERSHIP-019
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 6; RF-IDX-004 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: atomicity
  - **scenario**: publicaci�n incremental foreground
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether$'
  - **test**: TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether
  - **setup**: cambios de files y symbols preparados y publicados por la ruta foreground sin job
  - **Oráculo**: content_hash y symbols reflejan el mismo cambio y el runtime graph queda stale de forma coherente cuando no hay graph publication
  - **Condiciones de rechazo**: foreground_files_symbols_diverge=true
  - **id**: TC-IDX-OWNERSHIP-020
  - RF-IDX-001 — Criterio 1; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: atomicity
  - **scenario**: publicaci�n incremental del watcher
  - **command**: go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether$'
  - **test**: TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether
  - **setup**: workspace TypeScript real, cambio detectado por git y callback watcher que usa la ruta incremental foreground
  - **Oráculo**: el callback actualiza content_hash y symbols juntos; el s�mbolo anterior no permanece y el nuevo s�
  - **Condiciones de rechazo**: watcher_files_symbols_diverge=true
  - **id**: TC-IDX-OWNERSHIP-021
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 5; RF-IDX-004 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: terminal-truth
  - **scenario**: stale read con cancel_requested
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestIndexJobReadLoadersPreserveCancelRequestedAfterStaleFailureCAS$'
  - **test**: TestIndexJobReadLoadersPreserveCancelRequestedAfterStaleFailureCAS
  - **setup**: fila moderna con PID muerto y cancel_requested confirmado antes de la reconciliación de lectura
  - **Oráculo**: MarkIndexJobFailed puede perder el CAS, pero GetIndexJob, LatestIndexJob y ActiveIndexJob recargan el estado SQLite real y nunca sintetizan failed
  - **Condiciones de rechazo**: stale_cancel_read_synthesizes_failed=true
  - **id**: TC-IDX-OWNERSHIP-022
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 1; RF-IDX-004 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: error-propagation
  - **scenario**: error de MarkIndexJobFailed durante reconciliación
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestIndexJobReadPropagatesMarkFailedDatabaseError$'
  - **test**: TestIndexJobReadPropagatesMarkFailedDatabaseError
  - **setup**: seam de MarkIndexJobFailed que devuelve un error de base de datos
  - **Oráculo**: el loader devuelve el error envuelto y la fila conserva su estado no terminal en SQLite
  - **Condiciones de rechazo**: mark_failed_database_error_is_ignored=true
  - **id**: TC-IDX-OWNERSHIP-023
  - RF-IDX-001 — Criterio 5; RF-IDX-003 — Criterio 6; RF-IDX-004 — Criterio 11. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
  - **type**: api-boundary
  - **scenario**: publicación incremental sin bypass por commit independiente
  - **command**: go test -v -count=1 -p 1 ./internal/store -run '^TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether$'
  - **test**: TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether
  - **setup**: cambios de files y symbols publicados mediante PublishIncrementalGenerationWithChanges
  - **Oráculo**: las APIs públicas de publicación mantienen files y symbols en una transacción atómica; ReplaceFileSymbols y DeleteFileSymbols no son APIs exportadas ni tienen callers productivos
  - **Condiciones de rechazo**: incremental_private_query_bypass=true

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-IDX-OWNERSHIP
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-IDX-001.md
  - .docs/wiki/05_requirements/RF-IDX-003.md
  - .docs/wiki/05_requirements/RF-IDX-004.md
exports:
  - TP-IDX-OWNERSHIP
agent_must_read:
  - .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/
  - internal/store/
verify:
  - .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - atomicity_or_fencing_oracle_is_changed
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md
  - .docs/wiki/05_requirements/RF-IDX-001.md
  - .docs/wiki/05_requirements/RF-IDX-003.md
  - .docs/wiki/05_requirements/RF-IDX-004.md
```
