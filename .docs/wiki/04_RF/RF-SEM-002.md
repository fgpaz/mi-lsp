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
| Tabla `wiki_chunk_embeddings` existe en `.mi-lsp/index.db` | tecnica | obligatorio |
| Backend embeddings accesible o fallback offline activo | operativa | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `doc_path` | string | si | indexer | archivo `.md` en `.docs/wiki` | RF-SEM-002 |
| `chunk_heading` | string | si | parser markdown | heading (e.g., `## Seccion`) | RF-SEM-002 |
| `chunk_content` | string | si | parser markdown | contenido bajo heading | RF-SEM-002 |
| `chunk_hash` | string | si | hasher | SHA256 del contenido | RF-SEM-002 |
| `embedding_batch` | lista | no | embeddings client | batch de chunks a emebeer | RF-SEM-002 |

## 4. Process Steps (Happy Path)

1. Indexer detecta cambio en `.docs/wiki/**.md` (git status o file watcher).
2. Parser markdown divide contenido en chunks por H2/H3 heading.
3. Para cada chunk, calcula `chunk_hash = SHA256(heading + content)`.
4. Consulta `wiki_chunk_embeddings` con `chunk_hash`; si existe y `content_hash` coincide, SKIP (incremental).
5. Para chunks nuevos, agrupa en batch (default 10 por `batch_size` config).
6. Envia batch a embeddings backend (OpenAI-compatible o offline).
7. Si backend responde, inserta `(chunk_hash, doc_path, heading, content, embedding_vector, dim)` en tabla.
8. Si backend falla/timeout, registra warning y cae a fallback: guarda chunk SIN embedding (vector=NULL).
9. Marca indice como `last_semantic_index_ts = now()`.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | CLI | operacion completada (incluso con fallback) |
| `chunks_processed` | entero | diagnostico | numero de chunks nuevos indexados |
| `chunks_skipped` | entero | diagnostico | numero de chunks sin cambios |
| `embeddings_inserted` | entero | diagnostico | numero de embeddings guardados |
| `embeddings_failed` | entero | diagnostico | numero de chunks con fallback |
| `backend` | string | diagnostico | `embeddings` o `fallback` |
| `warnings` | lista | usuario | diagnostico de fallos parciales |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `SEM_INDEX_CORRUPTED` | tabla wiki_chunk_embeddings no existe | `.mi-lsp/index.db` invalido | abortar con error explicito |
| `SEM_BATCH_TIMEOUT` | backend agota timeout en embedding | timeout_ms excedido | registrar warning, guardar sin embedding, continuar |
| `SEM_INVALID_CHUNK` | contenido invalido o demasiado largo | >8000 tokens | SKIP chunk con warning |

## 7. Special Cases and Variants

- Si `batch_size=1`, cada chunk se embebe por separado (mas lento pero mas granular).
- Si embeddings son NULL (fallback), `nav recall` aun puede usar texto (lexical fallback offline).
- Indice es incremental: SOLO procesa chunks nuevos o modificados por `chunk_hash`.
- Si indice nunca fue inicializado, primera ejecucion procesa TODO (posiblemente lento).
- Daemon puede activar background re-indexing con debounce (RF-DAE-004).

## 8. Data Model Impact

- `WikiChunkEmbedding` tabla: `(chunk_hash, doc_path, heading, content, embedding_vector BLOB, dim, inserted_at, last_updated_at)`
- Indice por `chunk_hash` para lookup O(1) de duplicados

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Indexar chunks nuevos
  Given un workspace con docs en .docs/wiki/ sin embeddings
  When ejecuto "mi-lsp index --semantic"
  Then wiki_chunk_embeddings contiene N filas
  And chunks_processed = N
  And cada fila tiene embedding_vector (o NULL si fallback)

Scenario: Detectar duplicados por chunk_hash y SKIP
  Given archivo indexado previamente con M chunks
  When modifico un chunk diferente y reindexo
  Then chunks_skipped = M-1
  And chunks_processed = 1
  And no reembeeo los M-1 chunks viejos

Scenario: Caer a fallback si backend no responde
  Given config con backend no disponible
  When ejecuto "mi-lsp index --semantic"
  Then embeddings_failed > 0
  And embedding_vector = NULL para esos chunks
  And warning registrado de backend timeout
```

## 10. Test Traceability

- Positivo: `TP-SEM / TC-SEM-004`
- Positivo: `TP-SEM / TC-SEM-005`
- Negativo: `TP-SEM / TC-SEM-006`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir disponibilidad constante del backend
  - no eliminar embeddings viejos
  - no procesar chunks ya indexados por hash
- Decisiones cerradas:
  - incremental por `chunk_hash`
  - fallback sin embedding (NULL vector)
  - batch_size default 10
- TODO explicit = 0
- Fuera de alcance:
  - re-embedding de chunks viejos (update estrategia: future)
  - soporte para formatos no-markdown (future)
