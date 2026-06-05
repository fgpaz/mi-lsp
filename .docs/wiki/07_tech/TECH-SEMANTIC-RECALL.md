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

Detallar la arquitectura tecnica del backend de semantic recall: embeddings vectoriales pluggables sobre la wiki gobernada, store puro-Go, recurso sin CGO, uso operativo Qwen3/Nan y fallback documental seguro via `nav wiki search`.

## Componentes

### `internal/embed`

Subsistema responsable de:
- Inicializacion de cliente OpenAI-compatible con configuracion de `[embeddings]` desde `project.toml`
- Resolucion de endpoint base, modelo y API key opcional desde la variable nombrada en `api_key_env`
- Timeout configurado y manejo de fallas de conectividad
- Soporte pluggable de proveedores OpenAI-compatible; Nan/Qwen3 es la referencia operativa actual
- Payload OpenAI-compatible con `encoding_format`, `Accept: application/json`, `User-Agent` y validacion estricta de dimension

### `internal/wikichunk`

Subsistema responsable de:
- Extraccion de chunks de markdown desde docs canonicos gobernados
- Codificacion incremental por hash de contenido
- Persistencia y consulta de embeddings en tabla `wiki_chunk_embeddings` de `.mi-lsp/index.db`
- Fallback operacional recomendado a `nav wiki search` cuando embeddings no estan disponibles o el proveedor falla

### `wiki_chunk_embeddings` tabla

Persistencia SQLite:
- `doc_path TEXT`: ruta relativa al workspace
- `chunk_id TEXT`: id estable del chunk dentro del documento
- `start_line INTEGER`, `end_line INTEGER`: rango de lineas original
- `heading_text TEXT`: heading del chunk
- `snippet TEXT`: preview seguro de contenido
- `content_hash TEXT`: hash SHA256 del texto enriquecido con metadata
- `embedding BLOB`: vector float32 little-endian
- `embedding_model TEXT`: nombre del modelo usado
- `embedding_dim INTEGER`: dimension validada del vector
- `indexed_at INTEGER`: timestamp
- `UNIQUE(doc_path, chunk_id)`
- Indice en `doc_path` para lookup de documento

### Flujo de embedding

1. Durante `mi-lsp index`/`index.run`, post-publicacion del corpus documental o como backfill incremental sin cambios
2. Activacion por `EmbeddingsBlock.Active()`: `[embeddings]` existe con `base_url` + `model`, salvo `enabled = false` explicito
3. Deteccion de cambios por metadata-prefix/content hash, modelo y dimension respecto de `wiki_chunk_embeddings`
4. Almacenamiento en BLOB float32 little-endian para compacidad
5. Lazy CREATE-IF-NOT-EXISTS cuando se detecta la tabla ausente
6. No bloquea publication del indice si embedding falla

### Algoritmo de busqueda

- Encoding del query via el mismo modelo
- Cosine similarity puro-Go sobre vectores BLOB cargados en memoria
- Top-k determinista respecto de `max_items`
- Reranking por intencion (`formula`, `evidence`, `route`, `explore`, `learning`) sobre metadata, path, heading y snippet
- Fallback recomendado a `nav wiki search` si el proveedor activo falla; hint accionable si `[embeddings]` no esta activo
- Score normalizacion [0, 1] con penalizacion de score bajo

### Configuracion

En `.mi-lsp/project.toml`:

```toml
[embeddings]
# enabled = false  # kill switch explicito; omitido equivale a activo si base_url + model existen
provider = "openai"
base_url = "https://api.nan.builders/v1"
model = "qwen3-embedding"
dim = 4096
api_key_env = "NAN_API_KEY"
profile = "knowledge-wiki"  # o "spec-driven"
batch_size = 32
timeout_ms = 30000
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

La key se inyecta por environment variable o `mkey run`; el valor nunca se imprime ni se guarda en docs, logs o evidencia.

### Perfiles

- `knowledge-wiki`: semantica sobre wiki canonico; ranking docs-first con enriquecimiento vectorial
- `spec-driven`: optimizado para RF, CT y alineacion con especificacion; penaliza hits textuales genericos

### Degradacion offline

- Si `[embeddings]` no existe, falta `base_url`/`model`, o `enabled = false`, `nav recall` devuelve hint de configuracion sin llamar al proveedor
- Si el proveedor configurado no responde o rechaza la request, la guidance segura para docs canonicos es `mi-lsp nav wiki search`
- Usuario puede configurar manualmente con `--no-auto-daemon` si lo prefiere
- Warnings informan sobre el cambio de backend

### Invariantes

- Pure-Go: sin CGO, sin `sqlite-vec` remoto ni dependencias C
- `sqlite-vec` fue rechazado por requerir extension C, instalacion/distribucion compleja y overhead en builds cross-platform
- Ungated: `nav recall` no requiere gobernanza valida ni index ready
- Qwen descubre candidatos; no convierte material route-only en fuente final
- Sin BGE oculto: no hay proveedor local implicito cuando Nan/key/provider falla
- Incremental: cambios de metadata-prefix, contenido, modelo o dimension re-embedden chunks afectados

## `nav recall` command

- Superficie publica: `mi-lsp nav recall <query> [--workspace <alias>] [--intent formula|evidence|route|explore|learning] [--max-items 10] [--token-budget 2000] [--format toon|json] [--map]`
- Respuesta: `RecallResult[]` con `{query, intent, archivo, heading, score, snippet, start_line, end_line, why}`
- Backend seleccion: `recall` si embeddings listos; fallback documental recomendado `nav wiki search`
- No aguarda daemon; hot path directo
- Hint cuando embeddings no estan configurados

## Compatibilidad y migracion

- Workspace status expone `embeddings_enabled` y `recall_profile` (o `embeddings_unconfigured` en hint)
- Migration: tabla `wiki_chunk_embeddings` creada on-demand con lazy CREATE-IF-NOT-EXISTS
- `embeddings_enabled=true` cuando `[embeddings]` tiene `base_url` + `model` y no esta apagado con `enabled=false`
- Cambios en metadata-prefix, texto de chunk, modelo o dimension requieren reindex/reembedding; rerun `mi-lsp index` cuando cambie cualquiera de esos factores

## No objetivos

- Embedding de codigo C#, TS o Python (solo wiki)
- Vectores persistentes en daemon.db remoto
- Clustering o faceting de resultados
- Reranking por modelo LLM adicional
