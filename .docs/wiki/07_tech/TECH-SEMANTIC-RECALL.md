# TECH-SEMANTIC-RECALL

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-SEMANTIC-RECALL"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-SEMANTIC-RECALL]]'
exports:
  - 'TECH-SEMANTIC-RECALL'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-SEMANTIC-RECALL.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-SEMANTIC-RECALL.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-SEMANTIC-RECALL.md
```

## Proposito

Detallar la arquitectura tecnica del backend de semantic recall: embeddings vectoriales pluggables sobre la wiki gobernada, store puro-Go, recurso sin CGO, y fallback a busqueda lexical offline.

## Componentes

### `internal/embed`

Subsistema responsable de:
- Inicializacion de cliente OpenAI-compatible con configuracion de `[embeddings]` desde `project.toml`
- Resolucion de endpoint base, modelo y API key opcional desde `MI_LSP_EMBEDDINGS_API_KEY`
- Timeout configurado y manejo de fallas de conectividad
- Soporte pluggable de proveedores: OpenAI, Tesla BGE-m3, Azure OpenAI y compatibles

### `internal/wikichunk`

Subsistema responsable de:
- Extraccion de chunks de markdown desde docs canonicos gobernados
- Codificacion incremental por hash de contenido
- Persistencia y consulta de embeddings en tabla `wiki_chunk_embeddings` de `.mi-lsp/index.db`
- Fallback offline -> lexical cuando embeddings no estan disponibles

### `wiki_chunk_embeddings` tabla

Persistencia SQLite:
- `doc_path TEXT`: ruta relativa al workspace
- `chunk_id INTEGER`: orden dentro del documento
- `content_hash TEXT`: hash SHA256 del contenido
- `embedding BLOB`: vector float32 little-endian
- `embedding_model TEXT`: nombre del modelo usado
- `indexed_at INTEGER`: timestamp
- `UNIQUE(doc_path, chunk_id)`
- Indice en `(doc_path, content_hash)` para lookup incremental

### Flujo de embedding

1. Durante `mi-lsp index`/`index.run`, post-publicacion del corpus documental o como backfill incremental sin cambios
2. Activacion por `EmbeddingsBlock.Active()`: `[embeddings]` existe con `base_url` + `model`, salvo `enabled = false` explicito
3. Deteccion de cambios por `content_hash`, modelo y dimension respecto de `wiki_chunk_embeddings`
4. Almacenamiento en BLOB float32 little-endian para compacidad
5. Lazy CREATE-IF-NOT-EXISTS cuando se detecta la tabla ausente
6. No bloquea publication del indice si embedding falla

### Algoritmo de busqueda

- Encoding del query via el mismo modelo
- Cosine similarity puro-Go sobre vectores BLOB cargados en memoria
- Top-k determinista respecto de `max_items`
- Degradacion a lexical (FTS/ripgrep) si el proveedor activo falla; hint accionable si `[embeddings]` no esta activo
- Score normalizacion [0, 1] con penalizacion de score bajo

### Configuracion

En `.mi-lsp/project.toml`:

```toml
[embeddings]
# enabled = false  # kill switch explicito; omitido equivale a activo si base_url + model existen
provider = "openai"  # o "tesla", "azure-openai"
base_url = "https://api.openai.com/v1"
model = "text-embedding-3-large"  # o "bge-m3" para Tesla
dim = 1536  # o 1024 para bge-m3
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
profile = "knowledge-wiki"  # o "spec-driven"
batch_size = 100
timeout_ms = 30000
```

### Perfiles

- `knowledge-wiki`: semantica sobre wiki canonico; ranking docs-first con enriquecimiento vectorial
- `spec-driven`: optimizado para RF, CT y alineacion con especificacion; penaliza hits textuales genericos

### Degradacion offline

- Si `[embeddings]` no existe, falta `base_url`/`model`, o `enabled = false`, `nav recall` devuelve hint de configuracion sin llamar al proveedor
- Si el proveedor configurado no responde o rechaza la request, `nav recall` cae a FTS/ripgrep nativo
- Usuario puede configurar manualmente con `--no-auto-daemon` si lo prefiere
- Warnings informan sobre el cambio de backend

### Invariantes

- Pure-Go: sin CGO, sin `sqlite-vec` remoto ni dependencias C
- `sqlite-vec` fue rechazado por requerir extension C, instalacion/distribucion compleja y overhead en builds cross-platform
- Ungated: `nav recall` no requiere gobernanza valida ni index ready
- Offline: busqueda siempre funciona, vectorial cuando esta disponible, lexical en fallback
- Incremental: cambios de contenido solo re-embedden chunks afectados

## `nav recall` command

- Superficie publica: `mi-lsp nav recall <query> [--workspace <alias>] [--max-items 10] [--token-budget 2000] [--format toon|json] [--map]`
- Respuesta: `RecallResult[]` con `{archivo, heading, score, snippet, start_line}`
- Backend seleccion: `recall` si embeddings listos; `recall+lexical` si fallback
- No aguarda daemon; hot path directo
- Hint cuando embeddings no estan configurados

## Compatibilidad y migracion

- Workspace status expone `embeddings_enabled` y `recall_profile` (o `embeddings_unconfigured` en hint)
- Migration: tabla `wiki_chunk_embeddings` creada on-demand con lazy CREATE-IF-NOT-EXISTS
- `embeddings_enabled=true` cuando `[embeddings]` tiene `base_url` + `model` y no esta apagado con `enabled=false`
- Cambios en embeddings config disparan re-embedding por content hash/modelo/dimension, no full rebuild manual

## No objetivos

- Embedding de codigo C#, TS o Python (solo wiki)
- Vectores persistentes en daemon.db remoto
- Clustering o faceting de resultados
- Reranking por modelo LLM adicional
