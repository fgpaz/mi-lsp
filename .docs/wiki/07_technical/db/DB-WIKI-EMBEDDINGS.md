---
wiki_contract: WIKI-CONTRACT-v1
id: DB-WIKI-EMBEDDINGS
kind: database
status: active
title: DB-WIKI-EMBEDDINGS
links:
  materializes:
  - data-model
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: DB-WIKI-EMBEDDINGS
kind: data-model
audience: dual
imports: [.docs/wiki/07_technical/TECH-SEMANTIC-RECALL.md]
exports: [DB-WIKI-EMBEDDINGS]
agent_must_read: [.docs/wiki/07_technical/db/DB-WIKI-EMBEDDINGS.md, internal/store/queries_embeddings.go, internal/wikichunk/]
agent_may_edit: [.docs/wiki/07_technical/db/DB-WIKI-EMBEDDINGS.md]
agent_must_not_edit: [internal/store/, .mi-lsp/index.db, .mi-lsp/project.toml]
verify: ["Leer .docs/wiki/07_technical/db/DB-WIKI-EMBEDDINGS.md y .docs/wiki/07_technical/TECH-SEMANTIC-RECALL.md; cotejar las afirmaciones con la fuente publicada internal/store/queries_embeddings.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Una migración ocurra en esta edición, embeddings se copien a daemon.db o se active proveedor oculto/credencial implícita.]
evidence: [.docs/wiki/07_technical/db/DB-WIKI-EMBEDDINGS.md, .docs/wiki/07_technical/TECH-SEMANTIC-RECALL.md, internal/store/queries_embeddings.go]
```

## Almacén

Detallar la tabla SQLite `wiki_chunk_embeddings` que persiste vectores de embeddings para recall semántico sobre la wiki canonica. La tabla es repo-local, recomputable y no transporta secretos.

### Propósito

Detallar la tabla SQLite `wiki_chunk_embeddings` que persiste vectores de embeddings para recall semántico sobre la wiki canonica. La tabla es repo-local, recomputable y no transporta secretos.

## Esquema o convención

Los campos, claves, índices y convenciones físicas se conservan en el detalle siguiente.

### Tabla `wiki_chunk_embeddings`

Campos canónicos:

- `doc_path TEXT NOT NULL`: ruta relativa del documento.
- `chunk_id TEXT NOT NULL`: id estable del chunk dentro del documento.
- `start_line INTEGER NOT NULL`: línea inicial del chunk.
- `end_line INTEGER NOT NULL`: línea final del chunk.
- `heading_text TEXT`: heading asociado.
- `snippet TEXT`: preview seguro de contenido.
- `content_hash TEXT NOT NULL`: SHA256 del texto enriquecido con metadata-prefix.
- `embedding BLOB`: vector float32 little-endian codificado.
- `embedding_model TEXT`: nombre del modelo usado.
- `embedding_dim INTEGER`: dimensión validada según el modelo elegido.
- `indexed_at INTEGER`: timestamp UNIX cuando se embedde.

Índices:

- `UNIQUE(doc_path, chunk_id)`: evita duplicados por chunk.
- `idx_wiki_chunk_embeddings_doc` sobre `doc_path`: acelera reemplazo/carga por documento.

### Estrategia de refresco

#### Migración inicial

- Tabla creada on-demand por `mi-lsp index`/`index.run` cuando `[embeddings]` esta activo (`base_url` + `model`, salvo `enabled=false`).
- Lazy CREATE-IF-NOT-EXISTS: no bloquea publicación si la creacion de tabla falla; la operación registra warning.
- Migrar desde ausencia a presencia no requiere `PRAGMA user_version`.
- Un index incremental sin cambios puede ejecutar backfill para poblar filas faltantes.

#### Cambios incrementales

- Un chunk se re-embeddea si cambia cualquiera de estos factores: metadata-prefix, texto enriquecido, `content_hash`, `embedding_model`, `embedding_dim` o ausencia de BLOB.
- El texto enriquecido incluye metadata documental (`documentKey`, `body_role`, `tags`, `path`, `title`, `layer`, `family`, `heading`) y luego contenido.
- La version de metadata-prefix actual es `qwen-metadata-v1`; si cambia, se debe reindexar para no reutilizar vectores anteriores.
- Si hay hit exacto en hash/modelo/dimension/BLOB, se reutiliza la fila.
- Si hay miss, se llama al proveedor OpenAI-compatible por batches y cada batch exitoso se persiste con `INSERT OR REPLACE`; si el proceso se interrumpe, la siguiente corrida reutiliza los batches ya guardados.
- La limpieza de stale rows por documento ocurre al cierre de una fase de embeddings completada, no antes, para no borrar progreso valido ante deadline/cancelacion.

#### Cambios de proveedor o configuración

- Cambio en `[embeddings].model` o `[embeddings].dim` inválida filas previas por comparacion de modelo/dimension.
- Cambio de `base_url`, `api_key_env`, `encoding_format` o `user_agent` requiere rerun de `mi-lsp index` si se necesita regenerar vectores con la nueva config.
- Si key/provider/config falla, no se activa un fallback local oculto. El camino documental seguro mientras se corrige config es `mi-lsp nav wiki search`.
- Cambios en `[recall.rerank_extension]` no invalidan ni reescriben `wiki_chunk_embeddings`; el hook es stateless respecto de SQLite.

### Operaciones clave

#### Lectura

```sql
SELECT embedding, embedding_model, embedding_dim, content_hash
  FROM wiki_chunk_embeddings
 WHERE doc_path = ? AND chunk_id = ?
 LIMIT 1;
```

#### Upsert incremental

```sql
INSERT OR REPLACE INTO wiki_chunk_embeddings
  (doc_path, chunk_id, start_line, end_line, heading_text, snippet,
   content_hash, embedding, embedding_model, embedding_dim, indexed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
```

#### Limpieza por documentos reindexados

```sql
DELETE FROM wiki_chunk_embeddings
 WHERE doc_path = ?
   AND chunk_id NOT IN (<chunks successfully stored in this run>);
```

### Persistencia y codificacion

- BLOB almacena float32 puro (4 bytes por componente) en little-endian.
- Dimensión configurada en `[embeddings].dim`; depende del modelo elegido.
- Size = `dim * 4 bytes`; 4096-dim -> 16384 bytes por chunk.
- Decodificacion en Go: `binary.LittleEndian.Uint32(blob[i*4:(i+1)*4])` + conversion a float32.

### Reglas de consistencia

- Todas las filas de un documento deben tener el mismo `embedding_model` y `embedding_dim` efectivo.
- Si modelo o dimensión cambia en config, se re-embeddea el documento afectado.
- Si metadata-prefix o contenido enriquecido cambia, se re-embeddea el chunk afectado.
- `indexed_at` siempre UNIX seconds para compatibilidad con telemetria de indexacion.
- La tabla NO se copia ni snapshots en `daemon.db` global; es repo-local.

## Riesgos de migración

Se conservan las reglas específicas de publicación, retención, rollback y compatibilidad. No se migra ningún almacén durante esta migración documental.

### No objetivos

- Versionado historico de embeddings.
- Sincronización remota de vectores.
- Compresion adicional de BLOB.
- Backup/restore automático de embeddings; son recomputables via API configurada.
