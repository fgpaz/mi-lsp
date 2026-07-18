---
id: RF-GPH-002
title: Construir y publicar GraphGeneration de forma atomica
status: planned
flows:
  - FL-GPH-01
tests:
  - .docs/wiki/06_pruebas/TP-GPH.md
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-GPH-002"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-GPH-01]]'
  - '[[RF-GPH-001]]'
  - '[[05_modelo_datos]]'
exports:
  - 'RF-GPH-002'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-GPH-01.md
  - .docs/wiki/04_RF/RF-GPH-001.md
  - .docs/wiki/05_modelo_datos.md
  - .docs/wiki/04_RF/RF-GPH-002.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-GPH-002.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-GPH-002.md
```

# RF-GPH-002 - Construir y publicar GraphGeneration de forma atomica

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| Actores | Indexer, Graph Validator, SQLite Publisher, recovery loop |
| Prioridad / severidad | critica / critica |
| FL origen | FL-GPH-01 |
| Dependencia | RF-GPH-001 |

## 2. Resultado requerido

Construir una generacion inmutable fuera de la vista de lectores, validarla y mover el puntero activo en una unica transaccion SQLite. Un crash, cancelacion, migracion fallida o validacion negativa debe dejar visible la generacion valida anterior.

## 3. Identidad y estados

`generation_id` es el lowercase hex de `SHA-256` sobre una serializacion versionada de: schema de grafo, `repository_identity`, source fingerprint global, versiones de backend/extractor, configuracion semantica y digests ordenados de nodes/edges/evidence/unresolved. `created_at` y el orden de extraccion no participan del ID.

Estados permitidos:

- `staged`: escritura completa pero invisible para queries normales.
- `active`: unica generacion seleccionada por `active_graph_generation_id`.
- `retired`: generacion inmutable anterior, elegible para rollback durante retencion.
- `invalid`: staging rechazado; nunca elegible para query.

Las transiciones validas son `staged -> active`, `active -> retired` y `staged -> invalid`. No se reactiva una generacion alterando sus filas: rollback mueve el puntero a un snapshot inmutable ya validado.

## 4. Secuencia de publicacion

1. Abrir una transaccion de staging con schema version conocido.
2. Persistir `GraphGeneration`, nodes, edges, evidence y unresolved asociados al mismo `generation_id`.
3. Validar identidad, colisiones, endpoints, digests, cross-RIDs, completeness declarada, backend versions y source fingerprints.
4. Sellar counts y digest de la generacion.
5. En transaccion `BEGIN IMMEDIATE`, comprobar que el puntero activo no cambio desde el inicio, marcar el anterior `retired`, marcar el nuevo `active` y actualizar `workspace_meta.active_graph_generation_id`.
6. Commit; solo despues devolver exito al caller.
7. Aplicar retencion/cleanup en una transaccion posterior que nunca borra el unico rollback valido.

Los lectores abren una transaccion read-only y fijan un unico `generation_id` al inicio. Nunca combinan filas de dos generaciones.

## 5. Migracion y compatibilidad

- El schema graph-native se agrega sin destruir tablas legacy.
- Toda migracion tiene `from_version`, `to_version`, preflight, backup metadata, checksum y estado durable.
- Durante la ventana de compatibilidad se permite dual-write transaccional y dual-read explicito; cada respuesta declara el backend efectivo. No se mezclan resultados legacy y graph-native como si fueran una misma evidencia.
- La generacion nueva se stagea desde datos fuente, se valida y recien entonces se activa.
- La migracion nunca corre durante una query.
- Retirar escritura legacy exige evidencia de compatibilidad, rollback ejercitado y gate de release; no ocurre por timeout implicito.

## 6. Crash recovery y cancelacion

Al abrir `index.db`, el recovery loop inspecciona migraciones y generaciones no terminales. Si encuentra staging sin sello, pointer swap incompleto o owner PID muerto, marca la generacion `invalid`, limpia solo sus filas y conserva/restaura el puntero validado anterior. Una cancelacion cooperativa sigue la misma regla. Si no puede probar un puntero valido, bloquea queries graph-native con error accionable; no elige la generacion mas reciente por timestamp.

## 7. Invariantes

- Existe como maximo una generacion `active` por workspace/schema.
- Ningun reader observa staging, filas parciales o pointer sin snapshot sellado.
- La publicacion es idempotente para el mismo `generation_id` y payload.
- Un mismo ID con digest/counts diferentes es corrupcion y bloquea.
- `index.db` SQLite es la autoridad de adjacency; no se requiere una copia completa en RAM.
- El rollback no depende de NetworkX, MCP, red ni daemon.

## 8. Errores tipados

| Codigo | Causa | Resultado |
|---|---|---|
| `GPH_GENERATION_VALIDATION_FAILED` | identidad, endpoint, digest o cross-RID invalido | staged -> invalid |
| `GPH_GENERATION_POINTER_CONFLICT` | otro publisher cambio el activo | rollback transaccion y retry explicito |
| `GPH_GENERATION_WRITE_FAILED` | fallo SQLite | conservar activo anterior |
| `GPH_GENERATION_CORRUPT` | ID reutilizado con payload distinto | bloquear graph-native |
| `GPH_MIGRATION_PREFLIGHT_FAILED` | schema/source incompatibles | no escribir |
| `GPH_MIGRATION_ROLLBACK_FAILED` | no se prueba restauracion | bloquear release y queries nuevas |
| `GPH_CRASH_RECOVERY_REQUIRED` | staging/pointer no terminal | recuperar antes de servir graph-native |
| `GPH_GENERATION_NOT_FOUND` | selector no existe o fue limpiado | error, sin fallback inventado |

## 9. Aceptacion y trazabilidad

- Pruebas de pointer atomicity, readers concurrentes, crash en cada ventana, cancellation, retry e idempotencia.
- Pruebas de migracion additive, dual-read/write, downgrade y rollback con schema viejo intacto.
- Tras cualquier fallo, lectores ven la generacion valida anterior o un bloqueo explicito; nunca una mezcla.
- `TP-GPH / TP-GPH-002 / TC-GPH-007..014`.
