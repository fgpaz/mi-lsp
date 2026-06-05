# RF-SEM-002 - Indexar y embeber semanticamente chunks de markdown por heading, incremental por hash

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-SEM-002"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-SEM-002]]'
exports:
  - 'RF-SEM-002'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-SEM-002.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-SEM-002.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-SEM-002.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-SEM-002 |
| Titulo | Indexar y embeber semanticamente chunks de markdown por heading, incremental por hash |
| Actores | Desarrollador, Skill, CLI/Core, Daemon |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-SEM-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace indexado (`.mi-lsp/index.db` existente) | funcional | obligatorio |
| Tabla `wiki_chunk_embeddings` existe o puede crearse lazy en `.mi-lsp/index.db` | tecnica | obligatorio |
| Backend embeddings accesible o fallback offline activo | operativa | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `doc_path` | string | si | indexer | archivo `.md` en `.docs/wiki` | RF-SEM-002 |
| `chunk_heading` | string | si | parser markdown | heading (e.g., `## Seccion`) | RF-SEM-002 |
| `chunk_content` | string | si | parser markdown | contenido bajo heading | RF-SEM-002 |
| `chunk_hash` | string | si | hasher | SHA256 del texto enriquecido para embedding | RF-SEM-002 |
| `embedding_batch` | lista | no | embeddings client | batch de chunks a embeber | RF-SEM-002 |

## 4. Process Steps (Happy Path)

1. Indexer detecta cambio en `.docs/wiki/**.md` (git status o file watcher).
2. Parser markdown divide contenido en chunks por H2/H3 heading.
3. Para cada chunk, construye `embedding_text` con metadata (`documentKey`, `body_role`, `tags`, `path`, `title`, `layer`, `family`, `heading`) + contenido.
4. Calcula `content_hash = SHA256(chunk.ContentHash + "\0qwen-metadata-v1\0" + embedding_text)`.
5. Consulta `wiki_chunk_embeddings` por `(doc_path, chunk_id)`; si `content_hash`, `embedding_model`, `embedding_dim` y BLOB existen y coinciden, SKIP (incremental).
6. Para chunks nuevos o invalidos, agrupa en batch usando `batch_size`.
7. Envia batch al backend OpenAI-compatible con `encoding_format = "float"` cuando aplique.
8. Si backend responde, valida dimension estrictamente contra `[embeddings].dim` e inserta `(doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding, embedding_model, embedding_dim, indexed_at)`.
9. Si backend falla/timeout, registra warning; el camino documental seguro para el usuario es `nav wiki search`, no un modelo BGE oculto.
10. Marca indice como publicado con warnings si el embedding no pudo completarse.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | CLI | operacion completada (incluso con fallback) |
| `chunks_processed` | entero | diagnostico | numero de chunks nuevos indexados |
| `chunks_skipped` | entero | diagnostico | numero de chunks sin cambios |
| `embeddings_inserted` | entero | diagnostico | numero de embeddings guardados |
| `embeddings_failed` | entero | diagnostico | numero de chunks con fallback |
| `backend` | string | diagnostico | `recall` cuando embeddings estan listos; fallback recomendado `nav wiki search` |
| `warnings` | lista | usuario | diagnostico de fallos parciales |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `SEM_INDEX_CORRUPTED` | tabla wiki_chunk_embeddings no puede crearse/leerse | `.mi-lsp/index.db` invalido | abortar con error explicito |
| `SEM_BATCH_TIMEOUT` | backend agota timeout en embedding | timeout_ms excedido | registrar warning, guardar sin embedding, continuar |
| `SEM_INVALID_CHUNK` | contenido invalido o demasiado largo | >8000 tokens | SKIP chunk con warning |
| `SEM_DIMENSION_MISMATCH` | embedding devuelto no coincide con `dim` | proveedor responde dimension distinta | warning/error de batch; requiere corregir config o reindexar |

## 7. Special Cases and Variants

- Si `batch_size=1`, cada chunk se embebe por separado (mas lento pero mas granular).
- Si embeddings no estan disponibles, usar `nav wiki search` para recuperacion documental lexical/canonica.
- Indice es incremental: SOLO procesa chunks nuevos o modificados por metadata-prefix/content hash/modelo/dimension.
- Si indice nunca fue inicializado, primera ejecucion procesa TODO (posiblemente lento).
- Daemon puede activar background re-indexing con debounce (RF-DAE-004).

## 8. Data Model Impact

- `wiki_chunk_embeddings` tabla real: `(doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding BLOB, embedding_model, embedding_dim, indexed_at)`.
- Unicidad por `(doc_path, chunk_id)` e indice por `doc_path`.
- El BLOB codifica float32 little-endian; `embedding_dim` debe coincidir con `[embeddings].dim`.

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Indexar chunks nuevos
  Given un workspace con docs en .docs/wiki/ sin embeddings
  When ejecuto "mi-lsp index --semantic"
  Then wiki_chunk_embeddings contiene N filas con embedding_model y embedding_dim
  And chunks_processed = N
  And cada fila tiene embedding_vector (o NULL si fallback)

Scenario: Detectar duplicados por chunk_hash y SKIP
  Given archivo indexado previamente con M chunks
  When modifico un chunk diferente y reindexo
  Then chunks_skipped = M-1
  And chunks_processed = 1
  And no reembeeo los M-1 chunks viejos
  And cambio de metadata-prefix, modelo o dimension fuerza reembedding

Scenario: Caer a fallback si backend no responde
  Given config con backend no disponible
  When ejecuto "mi-lsp index --semantic"
  Then warning registrado de backend timeout
  And el usuario puede continuar con `mi-lsp nav wiki search`
```

## 10. Test Traceability

- Positivo: `TP-SEM / TC-SEM-004`
- Positivo: `TP-SEM / TC-SEM-005`
- Negativo: `TP-SEM / TC-SEM-006`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir disponibilidad constante del backend
  - no eliminar embeddings viejos fuera de docs reindexados
  - no procesar chunks ya indexados por hash/modelo/dimension
- Decisiones cerradas:
  - incremental por metadata-prefix/content hash/modelo/dimension
  - fallback documental seguro via `nav wiki search`
  - batch_size segun config efectiva
- TODO explicit = 0
- Fuera de alcance:
  - soporte para formatos no-markdown (future)
