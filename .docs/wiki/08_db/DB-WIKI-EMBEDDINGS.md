# DB-WIKI-EMBEDDINGS

```yaml
harness_protocol: SDD-HARNESS-v1
id: "DB-WIKI-EMBEDDINGS"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[DB-WIKI-EMBEDDINGS]]'
exports:
  - 'DB-WIKI-EMBEDDINGS'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/08_db/DB-WIKI-EMBEDDINGS.md
agent_may_edit:
  - .docs/wiki/08_db/DB-WIKI-EMBEDDINGS.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/08_db/DB-WIKI-EMBEDDINGS.md
```

## Proposito

Detallar la tabla SQLite `wiki_chunk_embeddings` que persiste vectores de embeddings para recall semantico sobre la wiki canonica. La tabla es repo-local, recomputable y no transporta secretos.

## Tabla `wiki_chunk_embeddings`

Campos canonicos:

- `doc_path TEXT NOT NULL`: ruta relativa del documento.
- `chunk_id TEXT NOT NULL`: id estable del chunk dentro del documento.
- `start_line INTEGER NOT NULL`: linea inicial del chunk.
- `end_line INTEGER NOT NULL`: linea final del chunk.
- `heading_text TEXT`: heading asociado.
- `snippet TEXT`: preview seguro de contenido.
- `content_hash TEXT NOT NULL`: SHA256 del texto enriquecido con metadata-prefix.
- `embedding BLOB`: vector float32 little-endian codificado.
- `embedding_model TEXT`: nombre del modelo usado, por ejemplo `qwen3-embedding`.
- `embedding_dim INTEGER`: dimension validada, `4096` para `qwen3-embedding`.
- `indexed_at INTEGER`: timestamp UNIX cuando se embedde.

Indices:

- `UNIQUE(doc_path, chunk_id)`: evita duplicados por chunk.
- `idx_wiki_chunk_embeddings_doc` sobre `doc_path`: acelera reemplazo/carga por documento.

## Estrategia de refresco

### Migracion inicial

- Tabla creada on-demand por `mi-lsp index`/`index.run` cuando `[embeddings]` esta activo (`base_url` + `model`, salvo `enabled=false`).
- Lazy CREATE-IF-NOT-EXISTS: no bloquea publicacion si la creacion de tabla falla; la operacion registra warning.
- Migrar desde ausencia a presencia no requiere `PRAGMA user_version`.
- Un index incremental sin cambios puede ejecutar backfill para poblar filas faltantes.

### Cambios incrementales

- Un chunk se re-embeddea si cambia cualquiera de estos factores: metadata-prefix, texto enriquecido, `content_hash`, `embedding_model`, `embedding_dim` o ausencia de BLOB.
- El texto enriquecido incluye metadata documental (`documentKey`, `body_role`, `tags`, `path`, `title`, `layer`, `family`, `heading`) y luego contenido.
- La version de metadata-prefix actual es `qwen-metadata-v1`; si cambia, se debe reindexar para no reutilizar vectores anteriores.
- Si hay hit exacto en hash/modelo/dimension/BLOB, se reutiliza la fila.
- Si hay miss, se llama al proveedor OpenAI-compatible y se reemplazan los chunks de los docs reindexados.

### Cambios de proveedor o configuracion

- Cambio en `[embeddings].model` o `[embeddings].dim` invalida filas previas por comparacion de modelo/dimension.
- Cambio de `base_url`, `api_key_env`, `encoding_format` o `user_agent` requiere rerun de `mi-lsp index` si se necesita regenerar vectores con la nueva config.
- Si Nan/key/provider falla, no se activa un fallback BGE oculto. El camino documental seguro mientras se corrige config es `mi-lsp nav wiki search`.

## Operaciones clave

### Lectura

```sql
SELECT embedding, embedding_model, embedding_dim, content_hash
  FROM wiki_chunk_embeddings
 WHERE doc_path = ? AND chunk_id = ?
 LIMIT 1;
```

### Upsert incremental

```sql
INSERT INTO wiki_chunk_embeddings
  (doc_path, chunk_id, start_line, end_line, heading_text, snippet,
   content_hash, embedding, embedding_model, embedding_dim, indexed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(doc_path, chunk_id) DO UPDATE SET
  start_line = excluded.start_line,
  end_line = excluded.end_line,
  heading_text = excluded.heading_text,
  snippet = excluded.snippet,
  content_hash = excluded.content_hash,
  embedding = excluded.embedding,
  embedding_model = excluded.embedding_model,
  embedding_dim = excluded.embedding_dim,
  indexed_at = excluded.indexed_at
 WHERE content_hash != excluded.content_hash
    OR embedding_model != excluded.embedding_model
    OR embedding_dim != excluded.embedding_dim;
```

### Limpieza por documentos reindexados

```sql
DELETE FROM wiki_chunk_embeddings
 WHERE doc_path IN (<docs reindexed>);
```

## Persistencia y codificacion

- BLOB almacena float32 puro (4 bytes por componente) en little-endian.
- Dimension configurada en `[embeddings].dim`; `qwen3-embedding` usa 4096.
- Size = `dim * 4 bytes`; 4096-dim -> 16384 bytes por chunk.
- Decodificacion en Go: `binary.LittleEndian.Uint32(blob[i*4:(i+1)*4])` + conversion a float32.

## Reglas de consistencia

- Todas las filas de un documento deben tener el mismo `embedding_model` y `embedding_dim` efectivo.
- Si modelo o dimension cambia en config, se re-embeddea el documento afectado.
- Si metadata-prefix o contenido enriquecido cambia, se re-embeddea el chunk afectado.
- `indexed_at` siempre UNIX seconds para compatibilidad con telemetria de indexacion.
- La tabla NO se copia ni snapshots en `daemon.db` global; es repo-local.

## No objetivos

- Versionado historico de embeddings.
- Sincronizacion remota de vectores.
- Compresion adicional de BLOB.
- Backup/restore automatico de embeddings; son recomputables via API configurada.
