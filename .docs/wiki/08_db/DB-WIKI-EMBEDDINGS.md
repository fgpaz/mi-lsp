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

Detallar la tabla SQLite `wiki_chunk_embeddings` que persiste vectores de embeddings para busqueda semantica sobre la wiki canonico gobernada.

## Tabla `wiki_chunk_embeddings`

Campos canonicos:
- `id INTEGER PRIMARY KEY AUTOINCREMENT`
- `doc_path TEXT NOT NULL`: ruta relativa del documento
- `chunk_id INTEGER NOT NULL`: ordinal del chunk dentro del documento (0-indexed)
- `content_hash TEXT NOT NULL`: SHA256 del contenido del chunk para dedup incremental
- `embedding BLOB NOT NULL`: vector float32 little-endian codificado
- `embedding_model TEXT NOT NULL`: nombre del modelo usado (ej. `text-embedding-3-large`, `bge-m3`)
- `indexed_at INTEGER NOT NULL`: timestamp UNIX cuando se embedde

Indices:
- `UNIQUE(doc_path, chunk_id)`: evita duplicados
- Indice en `(doc_path, content_hash)` para lookup rapido de cambios incrementales
- Indice en `(embedding_model)` para eviction/refresh por modelo

## Estrategia de refresco

### Migracion inicial

- Tabla creada on-demand en el primer `mi-lsp index`/`index.run` que detecta `[embeddings]` activo (`base_url` + `model`, salvo `enabled=false`)
- Lazy CREATE-IF-NOT-EXISTS: no bloquea publicacion si la creacion de tabla falla
- Migrar desde absence a presencia no requiere `PRAGMA user_version`
- Un index incremental sin cambios puede ejecutar backfill para poblar filas faltantes de `wiki_chunk_embeddings`

### Cambios de contenido incremental

- Chunk se re-embeddea solo si `content_hash`, modelo o dimension difieren de la fila existente en `wiki_chunk_embeddings`
- Cambios en `doc_records` (ej. titulo, body) disparan recalculo del hash
- Query de lookup: `SELECT ... FROM wiki_chunk_embeddings` cargada por `(doc_path, chunk_id)`
- Si hit en hash/modelo/dimension, se reutiliza el BLOB; si miss, se requiere via API y se reemplaza el batch de docs indexados

### Full refresh

- `mi-lsp index` puede forzar `[embeddings].force_refresh = true` para recomputar todo
- Cambio en `[embeddings].model` dispara full refresh por modelo anterior
- Cambio en `[embeddings].provider` dispara full refresh y borra embeddings del provider anterior

## Operaciones clave

### Lectura

```sql
SELECT embedding, embedding_model 
  FROM wiki_chunk_embeddings 
 WHERE doc_path = ? AND chunk_id = ?
 LIMIT 1;
```

### Upsert incremental

```sql
INSERT INTO wiki_chunk_embeddings 
  (doc_path, chunk_id, content_hash, embedding, embedding_model, indexed_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(doc_path, chunk_id) DO UPDATE SET
  content_hash = excluded.content_hash,
  embedding = excluded.embedding,
  embedding_model = excluded.embedding_model,
  indexed_at = excluded.indexed_at
 WHERE content_hash != excluded.content_hash;
```

### Batch cleanup

```sql
DELETE FROM wiki_chunk_embeddings 
 WHERE embedding_model = ?;
```

## Persistencia y codificacion

- BLOB almacena float32 puro (4 bytes por componente) en little-endian
- Dimension configurada en `[embeddings].dim` (ej. 1536 para OpenAI, 1024 para bge-m3)
- Size = `dim * 4 bytes`; ej. 1536-dim -> 6144 bytes por chunk
- Decodificacion en Go: `binary.LittleEndian.Uint32(blob[i*4:(i+1)*4])` + conversión a float32

## Reglas de consistencia

- Todas las filas de un documento deben tener el mismo `embedding_model` (invariante observable por `nav recall` output)
- Si modelo cambia en config, se re-embeddea todo el documento
- `indexed_at` siempre UNIX seconds para compatibilidad con telemetria de indexacion
- Si la creacion de tabla falla durante `index`, la operacion sigue; warning visible en `workspace status`
- La tabla NO se copia ni snapshots en `daemon.db` global; es repo-local

## No objetivos

- Versionado historico de embeddings
- Sincronizacion remota de vectores
- Compresion de BLOB (float32 ya es compacto)
- Backup/restore automatico de embeddings (estan disponibles para recomputo via API)
