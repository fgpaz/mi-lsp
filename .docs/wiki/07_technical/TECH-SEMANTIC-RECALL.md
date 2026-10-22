---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-SEMANTIC-RECALL
kind: technical
status: active
title: TECH-SEMANTIC-RECALL
---

## Resumen

Detallar la arquitectura técnica del backend de semantic recall: embeddings vectoriales pluggables sobre la wiki gobernada, store puro-Go, recurso sin CGO, hook local opcional de rerank y fallback documental seguro via `nav wiki search`.

### Propósito

Detallar la arquitectura técnica del backend de semantic recall: embeddings vectoriales pluggables sobre la wiki gobernada, store puro-Go, recurso sin CGO, hook local opcional de rerank y fallback documental seguro via `nav wiki search`.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

### Componentes

#### `internal/embed`

Subsistema responsable de:
- Inicializacion de cliente OpenAI-compatible con configuración de `[embeddings]` desde `project.toml`
- Resolución de endpoint base, modelo y API key opcional desde la variable nombrada en `api_key_env`
- Timeout configurado y manejo de fallas de conectividad
- Soporte pluggable de proveedores OpenAI-compatible sin cliente privado en core
- Payload OpenAI-compatible con `encoding_format`, `Accept: application/json`, `User-Agent` y validación estricta de dimensión

#### `internal/wikichunk`

Subsistema responsable de:
- Extracción de chunks de markdown desde docs canónicos gobernados
- Codificacion incremental por hash de contenido
- Persistencia y consulta de embeddings en tabla `wiki_chunk_embeddings` de `.mi-lsp/index.db`
- Fallback operacional recomendado a `nav wiki search` cuando embeddings no estan disponibles o el proveedor falla

#### `wiki_chunk_embeddings` tabla

Persistencia SQLite:
- `doc_path TEXT`: ruta relativa al workspace
- `chunk_id TEXT`: id estable del chunk dentro del documento
- `start_line INTEGER`, `end_line INTEGER`: rango de líneas original
- `heading_text TEXT`: heading del chunk
- `snippet TEXT`: preview seguro de contenido
- `content_hash TEXT`: hash SHA256 del texto enriquecido con metadata
- `embedding BLOB`: vector float32 little-endian
- `embedding_model TEXT`: nombre del modelo usado
- `embedding_dim INTEGER`: dimensión validada del vector
- `indexed_at INTEGER`: timestamp
- `UNIQUE(doc_path, chunk_id)`
- Índice en `doc_path` para lookup de documento

#### Flujo de embedding

1. Durante `mi-lsp index`/`index.run`, post-publicacion del corpus documental o como backfill incremental sin cambios
2. Activacion por `EmbeddingsBlock.Active()`: `[embeddings]` existe con `base_url` + `model`, salvo `enabled = false` explícito
3. Deteccion de cambios por metadata-prefix/content hash, modelo y dimensión respecto de `wiki_chunk_embeddings`
4. Almacenamiento incremental por batch confirmado en `wiki_chunk_embeddings`; chunks ya persistidos con mismo hash/modelo/dimension se reutilizan en corridas posteriores
5. Limpieza de filas invalidas y pruning de stale rows solo al cierre de una fase de embeddings completada, para no perder progreso si el proceso se interrumpe
6. Lazy CREATE-IF-NOT-EXISTS cuando se detecta la tabla ausente
7. Fallas del provider se reportan como warnings sanitizados y conservan lotes exitosos; cancelacion/deadline del contexto falla con stage `embeddings` y progreso `N/M chunks embedded`

#### Algoritmo de busqueda

- Encoding del query via el mismo modelo
- Cosine similarity puro-Go sobre vectores BLOB cargados en memoria
- Top-k determinista respecto de `max_items`
- Reranking por intención (`formula`, `evidence`, `route`, `explore`, `learning`) sobre metadata, path, heading y snippet
- Rerank externo opcional por comando local después del ranking semántico y antes del corte final
- Fallback recomendado a `nav wiki search` si el proveedor activo falla; hint accionable si `[embeddings]` no esta activo
- Score normalización [0, 1] con penalizacion de score bajo

#### Configuración

La configuración del workspace se declara en `.mi-lsp/project.toml`:

```toml
[embeddings]

### enabled = false  # kill switch explicito; omitido equivale a activo si base_url + model existen
provider = "openai"
base_url = "https://embeddings.example.local/v1"
model = "text-embedding-model"
dim = 1536
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
profile = "knowledge-wiki"  # o "spec-driven"
batch_size = 32
timeout_ms = 30000
index_timeout_ms = 0  # opcional; 0 usa presupuesto escalado por corpus/batches
encoding_format = "float"
user_agent = "mi-lsp-embeddings/1.0"
```

La credencial se resuelve localmente por el nombre de variable configurado o mediante `mkey run`; el valor nunca se imprime ni se guarda en docs, logs o evidencia.

La configuración personal opcional del nodo está en `$XDG_CONFIG_HOME/mi-lsp/config.toml`; si `XDG_CONFIG_HOME` no está definido, se usa `~/.config/mi-lsp/config.toml` bajo el home del usuario que ejecuta la CLI. No se guarda en el repositorio. Admite proveedores `openai-compatible` con `base_urls` ordenadas, `model`, `dimensions` y credencial por `api_key_env` o `api_key_command`, además de declaraciones de complementos. Los valores existentes `MI_LSP_EMBEDDINGS_*` conservan precedencia sobre esta configuración; la precedencia completa y la compatibilidad con `[embeddings]` legado del workspace están descritas en `docs/configuracion.md`. Ninguna configuración personal introduce endpoint, modelo o credencial predeterminados; si no hay proveedor activo, el comportamiento sigue siendo léxico.

Los ejemplos publicados usan dominios y paths genéricos, no rutas absolutas, hosts privados, nombres de máquina ni valores de secretos de un nodo. El endpoint de embeddings depende de la configuración explícita del consumidor; el contrato de gateway y su transporte no forman parte de esta configuración personal.

Rerank externo opcional en `.mi-lsp/project.toml`:

```toml
[recall.rerank_extension]
enabled = true
command = "mi-lsp-rerank-local"
args = ["--profile", "default"]
timeout_ms = 2000
candidate_count = 50
top_n = 10
max_snippet_chars = 500
```

El comando se ejecuta localmente sin shell. El core no implementa cliente HTTP de rerank, no conoce proveedores privados y no interpola secretos en `args`.

#### Perfiles

- `knowledge-wiki`: semántica sobre wiki canónico; ranking docs-first con enriquecimiento vectorial
- `spec-driven`: optimizado para RF, CT y alineacion con especificacion; penaliza hits textuales genericos

#### Degradacion offline

- Si `[embeddings]` no existe, falta `base_url`/`model`, o `enabled = false`, `nav recall` devuelve hint de configuración sin llamar al proveedor
- Si el proveedor configurado no responde o rechaza la request, la guidance segura para docs canónicos es `mi-lsp nav wiki search`
- Si el hook de rerank falta, tarda, sale con código no cero o responde JSON inválido, `nav recall` preserva el orden semántico original
- Usuario puede configurar manualmente con `--no-auto-daemon` si lo prefiere
- Warnings informan sobre el cambio de backend o falla del hook con texto sanitizado

#### Invariantes

- Pure-Go: sin CGO, sin `sqlite-vec` remoto ni dependencias C
- `sqlite-vec` fue rechazado por requerir extension C, instalacion/distribucion compleja y overhead en builds cross-platform
- Ungated: `nav recall` no requiere gobernanza valida ni index ready
- El modelo de embeddings descubre candidatos; no convierte material route-only en fuente final
- Sin proveedor oculto: no hay modelo local implicito cuando key/provider/config falla
- Rerank extension no persiste query, snippets, stdin/stdout, stderr, provider responses, tokens ni secretos
- Incremental: cambios de metadata-prefix, contenido, modelo o dimensión re-embedden chunks afectados

### `nav recall` command

- Superficie publica: `mi-lsp nav recall <query> [--workspace <alias>] [--intent formula|evidence|route|explore|learning] [--max-items 10] [--token-budget 2000] [--format toon|json] [--map]`
- Respuesta: `RecallResult[]` con `{query, intent, archivo, heading, score, snippet, start_line, end_line, why}`; si aplica el hook externo, `why` incluye `external_rerank`
- Backend selección: `recall` si embeddings listos; fallback documental recomendado `nav wiki search`
- No aguarda daemon; hot path directo
- Hint cuando embeddings no estan configurados

### Compatibilidad y migración

- Workspace status expone `embeddings_enabled` y `recall_profile` (o `embeddings_unconfigured` en hint)
- Workspace status expone `rerank_extension_enabled` y `rerank_extension_mode=local-command` cuando el hook esta activo
- Migration: tabla `wiki_chunk_embeddings` creada on-demand con lazy CREATE-IF-NOT-EXISTS
- `embeddings_enabled=true` cuando `[embeddings]` tiene `base_url` + `model` y no esta apagado con `enabled=false`
- Cambios en metadata-prefix, texto de chunk, modelo o dimensión requieren reindex/reembedding; rerun `mi-lsp index` cuando cambie cualquiera de esos factores
- Cambios en `[recall.rerank_extension]` no requieren migración ni reindex, salvo que cambie el modelo/dimension de embeddings upstream

### No objetivos

- Embedding de código C#, TS o Python (solo wiki)
- Vectores persistentes en daemon.db remoto
- Clustering o faceting de resultados
- Cliente privado de rerank dentro del core
