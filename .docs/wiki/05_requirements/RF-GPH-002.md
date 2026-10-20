---
wiki_contract: WIKI-CONTRACT-v1
id: RF-GPH-002
kind: requirement
status: active
title: RF-GPH-002 - Construir y publicar GraphGeneration de forma atomica
links:
  implements:
  - FL-GPH-01
  code:
  - path: internal/indexer/graph_staging.go
    role: behavior
  - path: internal/indexer/indexer.go
    role: behavior
  - path: internal/store/index_publish.go
    role: behavior
---

## Propósito

RF-GPH-002 - Construir y publicar GraphGeneration de forma atomica

### 1. Hoja de ejecución
- **Campo**: Actores; **Valor**: Indexer, Graph Validator, SQLite Publisher, recovery loop
- **Campo**: Prioridad / severidad; **Valor**: critica / critica
- **Campo**: FL origen; **Valor**: FL-GPH-01
- **Campo**: Dependencia; **Valor**: RF-GPH-001

## Comportamiento ordenado

### 2. Resultado requerido

Construir una generación inmutable fuera de la vista de lectores, validarla y mover el puntero activo en una única transaccion SQLite. Un crash, cancelacion, migración fallida o validación negativa debe dejar visible la generación valida anterior.

### 3. Identidad y estados

`generation_id` es el lowercase hex de `SHA-256` sobre una serializacion versionada de: schema de grafo, `repository_identity`, source fingerprint global, versiones de backend/extractor, configuración semántica y digests ordenados de nodes/edges/evidence/unresolved. `created_at` y el orden de extracción no participan del ID.

Estados permitidos:

- `staged`: escritura completa pero invisible para queries normales.
- `active`: única generación seleccionada por `active_graph_generation_id`.
- `retired`: generación inmutable anterior, elegible para rollback o reactivación canónica durante retencion.
- `invalid`: staging rechazado; nunca elegible para query.

Las transiciones válidas son `staged -> active`, `active -> retired`, `staged -> invalid` y `retired -> active`. Una generación canónica retirada, exacta, inmutable y estrictamente validada puede transicionar `retired -> active` mediante CAS atómico preservando su predecesor histórico inmutable (sin reescribir `previous_generation_id`), mientras que solo un nuevo snapshot `staged` recibe el puntero activo actual como su predecesor; filas retiradas arbitrarias, mutadas o corruptas nunca se reactivan ni alteran el puntero del workspace.

### 4. Secuencia de publicación

1. Abrir una transaccion de staging con schema version conocido.
2. Persistir `GraphGeneration`, nodes, edges, evidence y unresolved asociados al mismo `generation_id`.
3. Validar identidad, colisiones, endpoints, digests, cross-RIDs, completeness declarada, backend versions y source fingerprints.
4. Sellar counts y digest de la generacion.
5. En transaccion `BEGIN IMMEDIATE`, comprobar que el puntero activo no cambio desde el inicio, marcar el anterior `retired`, marcar el nuevo `active` y actualizar `workspace_meta.active_graph_generation_id`.
6. Commit; solo después devolver exito al caller.
7. Aplicar retencion/cleanup en una transaccion posterior que nunca borra el único rollback valido.

En observaciones Go, `complete` significa completo para la superficie soportada declarada: exclusiones previstas como targets externos o kinds no representables se conservan como omisiones tipadas y no degradan la observación. Un fallo real de parseo, carga de tipos, lectura de fuentes, cancelación o endpoint local elegible ausente permanece `partial`/`unresolved` y no es stageable. Un job owner-bound sin cambios deja su generación candidata en `skipped` dentro de la misma transacción y no mueve el puntero de catálogo. Si el runtime ya tiene un graph válido, conserva su puntero y binding; si requiere reparación, puede stagear/activar el snapshot graph reparado y marcar el runtime `fresh`, siempre conservando el binding del catálogo activo y las garantías de fence/CAS.

Los lectores abren una transaccion read-only y fijan un único `generation_id` al inicio. Nunca combinan filas de dos generaciones.

### Slice publicado: grafo documental docs-only

- **Marcador estable**: RF-GPH-002.docs-only-knowledge-wiki
- **Estado del comportamiento**: implemented
- **command**: mi-lsp index --docs-only
- **canonical_markdown**:
  - wiki/**
  - bibliotecas/**
  - .docs/wiki/**
  - docs/**
  - README*.md
- **excluded**:
  - .docs/raw/
  - .docs/auditoria/
  - old
  - archive
  - deprecated
  - historico
  - legacy
- **Comportamiento**:
  - si hay docs canónicos, se publica GraphGeneration documental aunque no haya batches de compilador
  - jobs fenced reusan CatalogGeneration activa y no la pisan con la generation de docs
  - el catálogo de código no se reemplaza
  - wiki map no publica el grafo; solo el indexer/publisher
- **Comprobación prevista**:
  - go test ./internal/indexer -count=1 -run 'TestCanonicalGraphDocAcceptsKnowledgeWikiPaths|TestDocsOnly'
- **Condiciones de rechazo**:
  - docs_only_replaces_code_catalog=true
  - missing_canonical_docs_silently_claimed_as_graph_fresh=true
- **Evidencia referenciada**:
  - internal/indexer/indexer.go
  - internal/indexer/graph_staging.go
  - internal/store/index_publish.go

### 5. Migración y compatibilidad

- El schema graph-native se agrega sin destruir tablas legacy.
- Toda migración tiene `from_version`, `to_version`, preflight, backup metadata, checksum y estado durable.
- Durante la ventana de compatibilidad se permite dual-write transaccional y dual-read explícito; cada respuesta declara el backend efectivo. No se mezclan resultados legacy y graph-native como si fueran una misma evidencia.
- La generación nueva se stagea desde datos fuente, se valida y recien entonces se activa.
- La migración nunca corre durante una query.
- Retirar escritura legacy exige evidencia de compatibilidad, rollback ejercitado y gate de release; no ocurre por timeout implicito.

### 6. Crash recovery y cancelacion

Al abrir `index.db`, el recovery loop inspecciona migraciones y generaciones no terminales. Si encuentra staging sin sello, pointer swap incompleto o owner PID muerto, marca la generación `invalid`, limpia solo sus filas y conserva/restaura el puntero validado anterior. Una cancelacion cooperativa sigue la misma regla. Si no puede probar un puntero valido, bloquea queries graph-native con error accionable; no elige la generación más reciente por timestamp.

### 7. Invariantes

- Existe como maximo una generación `active` por workspace/schema.
- Ningún reader observa staging, filas parciales o pointer sin snapshot sellado.
- La publicación es idempotente para el mismo `generation_id` y payload.
- Un mismo ID con digest/counts diferentes es corrupcion y bloquea.
- `index.db` SQLite es la autoridad de adjacency; no se requiere una copia completa en RAM.
- El rollback no depende de NetworkX, MCP, red ni daemon.

## Superficies

- mi-lsp index --docs-only

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### 8. Errores tipados

- **Código**: `GPH_GENERATION_VALIDATION_FAILED`; **Causa**: identidad, endpoint, digest o cross-RID inválido; **Resultado**: staged -> invalid
- **Código**: `GPH_GENERATION_POINTER_CONFLICT`; **Causa**: otro publisher cambio el activo; **Resultado**: rollback transaccion y retry explícito
- **Código**: `GPH_GENERATION_WRITE_FAILED`; **Causa**: fallo SQLite; **Resultado**: conservar activo anterior
- **Código**: `GPH_GENERATION_CORRUPT`; **Causa**: ID reutilizado con payload distinto; **Resultado**: bloquear graph-native
- **Código**: `GPH_MIGRATION_PREFLIGHT_FAILED`; **Causa**: schema/source incompatibles; **Resultado**: no escribir
- **Código**: `GPH_MIGRATION_ROLLBACK_FAILED`; **Causa**: no se prueba restauracion; **Resultado**: bloquear release y queries nuevas
- **Código**: `GPH_CRASH_RECOVERY_REQUIRED`; **Causa**: staging/pointer no terminal; **Resultado**: recuperar antes de servir graph-native
- **Código**: `GPH_GENERATION_NOT_FOUND`; **Causa**: selector no existe o fue limpiado; **Resultado**: error, sin fallback inventado

## Criterios de aceptación (numerados)

1. Pruebas de pointer atomicity, readers concurrentes, crash en cada ventana, cancellation, retry e idempotencia.
2. Pruebas de migración additive, dual-read/write, downgrade y rollback con schema viejo intacto.
3. Tras cualquier fallo, lectores ven la generación valida anterior o un bloqueo explícito; nunca una mezcla.

### 9. Aceptacion y trazabilidad

- Pruebas de pointer atomicity, readers concurrentes, crash en cada ventana, cancellation, retry e idempotencia.
- Pruebas de migración additive, dual-read/write, downgrade y rollback con schema viejo intacto.
- Tras cualquier fallo, lectores ven la generación valida anterior o un bloqueo explícito; nunca una mezcla.
- `TP-GPH / TP-GPH-002 / TC-GPH-007..014`.
