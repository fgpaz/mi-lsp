# Resultado AORPH store — entrega tests-only

REPORT-FINAL: AORPH-RESULT-store-red.md

Estado: tests-only listo, esperando reproducción RED autorizada por parent. No se ejecutaron tests/build/gofmt/lint/validators/C2.

- Commit tests: `5cb606c` (`test(daemon): cover orphan telemetry checkpoint behavior`), sobre base `28c0a1f77ecd92246fd4679264d34b2c32e88ced`.
- Tests añadidos: `TestOrphanCheckpointIdleWAL`, `TestOrphanCheckpointPinnedReaderReportsBusy`.
- Paths tocados: `internal/daemon/state_store_orphan_test.go`; `AORPH-RESULT-store-red.md`.
- Fixtures: sqlite temporal por test, exclusivamente propio; WAL habilitado por `configureConnection`, esquema de store. El segundo test usa una segunda conexión a la misma DB temporal con transacción lectora pinneada.
- RED esperado (no ejecutado): ambas rutas pueden fallar actualmente porque `Checkpoint()` hace `Scan(&mode)` aunque `PRAGMA wal_checkpoint(TRUNCATE)` entrega tres columnas. Con lector pinneado, además debe traducir el status busy a error explícito; no considerar resultado busy como checkpoint exitoso. No se afirma que el WAL actual supere límite ni se atribuye causalidad de CPU.
- Límite de la primera entrega: las APIs actuales no exponen política de cap/backpressure ni error/estadística de escrituras async (el consumidor descarta errores). Por tanto estos tests no pueden demostrar límite de crecimiento ni visibilidad/pérdida de telemetría. Propuesta para continuación autorizada: leer las tres columnas (busy, log frames, checkpointed frames); propagar busy como `error` conservando `Checkpoint() error`; añadir al store una política explícita y acotada de admisión/backpressure antes de encolar telemetría, con diagnóstico observable de saturación y resultado fiel de escritura; nunca inferir hard cap de `wal_autocheckpoint`/`journal_size_limit`. Revisar manejo de cola/cierre en el mismo ámbito, sin cambiar APIs antes de acordarlo con parent.
- Observación: como no corrí pruebas (prohibido por encargo), compilabilidad y salida RED quedan pendientes de validación parent.

LISTO
