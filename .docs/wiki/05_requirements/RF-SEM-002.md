---
wiki_contract: WIKI-CONTRACT-v1
id: RF-SEM-002
kind: requirement
status: active
title: RF-SEM-002 - Indexar y embeber semanticamente chunks de markdown por heading, incremental por hash
links:
  implements:
  - FL-SEM-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-SEM-002 - Indexar y embeber semanticamente chunks de markdown por heading, incremental por hash

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-SEM-002
- **Campo**: Título; **Valor**: Indexar y embeber semanticamente chunks de markdown por heading, incremental por hash
- **Campo**: Actores; **Valor**: Desarrollador, Skill, CLI/Core, Daemon
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-SEM-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace indexado (`.mi-lsp/index.db` existente); **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Tabla `wiki_chunk_embeddings` existe o puede crearse lazy en `.mi-lsp/index.db`; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Backend embeddings accesible o fallback offline activo; **Tipo**: operativa; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. Indexer detecta cambio en `.docs/wiki/**.md` (git status o file watcher).
2. Parser markdown divide contenido en chunks por H2/H3 heading.
3. Para cada chunk, construye `embedding_text` con metadata (`documentKey`, `body_role`, `tags`, `path`, `title`, `layer`, `family`, `heading`) + contenido.
4. Calcula `content_hash = SHA256(chunk.ContentHash + "\0qwen-metadata-v1\0" + embedding_text)`.
5. Consulta `wiki_chunk_embeddings` por `(doc_path, chunk_id)`; si `content_hash`, `embedding_model`, `embedding_dim` y BLOB existen y coinciden, SKIP (incremental).
6. Para chunks nuevos o invalidos, agrupa en batch usando `batch_size`.
7. Envia batch al backend OpenAI-compatible con `encoding_format = "float"` cuando aplique.
8. Si backend responde, valida dimensión estrictamente contra `[embeddings].dim` e inserta `(doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding, embedding_model, embedding_dim, indexed_at)`.
9. Si backend falla/timeout, registra warning; el camino documental seguro para el usuario es `nav wiki search`, no un modelo local oculto.
10. Marca índice como publicado con warnings si el embedding no pudo completarse.

### 7. Casos especiales y variantes
- Si `batch_size=1`, cada chunk se embebe por separado (más lento pero más granular).
- Si embeddings no estan disponibles, usar `nav wiki search` para recuperacion documental lexical/canonica.
- Índice es incremental: SOLO procesa chunks nuevos o modificados por metadata-prefix/content hash/modelo/dimension.
- Si índice nunca fue inicializado, primera ejecución procesa TODO (posiblemente lento).
- Daemon puede activar background re-indexing con debounce (RF-DAE-004).

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir disponibilidad constante del backend
  - no eliminar embeddings viejos fuera de docs reindexados
  - no procesar chunks ya indexados por hash/modelo/dimension
- Decisiones cerradas:
  - incremental por metadata-prefix/content hash/modelo/dimension
  - fallback documental seguro via `nav wiki search`
  - batch_size según config efectiva
- TODO explicit = 0
- Fuera de alcance:
  - soporte para formatos no-markdown (future)

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `doc_path`; **Tipo**: string; **Req.**: si; **Origen**: indexer; **Validacion**: archivo `.md` en `.docs/wiki`; **RN**: RF-SEM-002
- **Campo**: `chunk_heading`; **Tipo**: string; **Req.**: si; **Origen**: parser markdown; **Validacion**: heading (e.g., `## Seccion`); **RN**: RF-SEM-002
- **Campo**: `chunk_content`; **Tipo**: string; **Req.**: si; **Origen**: parser markdown; **Validacion**: contenido bajo heading; **RN**: RF-SEM-002
- **Campo**: `chunk_hash`; **Tipo**: string; **Req.**: si; **Origen**: hasher; **Validacion**: SHA256 del texto enriquecido para embedding; **RN**: RF-SEM-002
- **Campo**: `embedding_batch`; **Tipo**: lista; **Req.**: no; **Origen**: embeddings client; **Validacion**: batch de chunks a embeber; **RN**: RF-SEM-002

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: CLI; **Efecto observable**: operación completada (incluso con fallback)
- **Campo**: `chunks_processed`; **Tipo**: entero; **Destino**: diagnóstico; **Efecto observable**: número de chunks nuevos indexados
- **Campo**: `chunks_skipped`; **Tipo**: entero; **Destino**: diagnóstico; **Efecto observable**: número de chunks sin cambios
- **Campo**: `embeddings_inserted`; **Tipo**: entero; **Destino**: diagnóstico; **Efecto observable**: número de embeddings guardados
- **Campo**: `embeddings_failed`; **Tipo**: entero; **Destino**: diagnóstico; **Efecto observable**: número de chunks con fallback
- **Campo**: `backend`; **Tipo**: string; **Destino**: diagnóstico; **Efecto observable**: `recall` cuando embeddings estan listos; fallback recomendado `nav wiki search`
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario; **Efecto observable**: diagnóstico de fallos parciales

### 8. Impacto en el modelo de datos
- `wiki_chunk_embeddings` tabla real: `(doc_path, chunk_id, start_line, end_line, heading_text, snippet, content_hash, embedding BLOB, embedding_model, embedding_dim, indexed_at)`.
- Unicidad por `(doc_path, chunk_id)` e índice por `doc_path`.
- El BLOB codifica float32 little-endian; `embedding_dim` debe coincidir con `[embeddings].dim`.

## Errores

### 6. Errores tipados
- **Código**: `SEM_INDEX_CORRUPTED`; **Causa**: tabla wiki_chunk_embeddings no puede crearse/leerse; **Trigger**: `.mi-lsp/index.db` inválido; **Respuesta esperada**: abortar con error explícito
- **Código**: `SEM_BATCH_TIMEOUT`; **Causa**: backend agota timeout en embedding; **Trigger**: timeout_ms excedido; **Respuesta esperada**: registrar warning, guardar sin embedding, continuar
- **Código**: `SEM_INVALID_CHUNK`; **Causa**: contenido inválido o demasiado largo; **Trigger**: >8000 tokens; **Respuesta esperada**: SKIP chunk con warning
- **Código**: `SEM_DIMENSION_MISMATCH`; **Causa**: embedding devuelto no coincide con `dim`; **Trigger**: proveedor responde dimensión distinta; **Respuesta esperada**: warning/error de batch; requiere corregir config o reindexar

## Criterios de aceptación (numerados)

1. Indexar chunks nuevos: Dado un workspace con docs en .docs/wiki/ sin embeddings Cuando ejecuto "mi-lsp index --semantic" Entonces wiki_chunk_embeddings contiene N filas con embedding_model y embedding_dim Y chunks_processed = N Y cada fila tiene embedding_vector (o NULL si fallback)
2. Detectar duplicados por chunk_hash y SKIP: Dado archivo indexado previamente con M chunks Cuando modifico un chunk diferente y reindexo Entonces chunks_skipped = M-1 Y chunks_processed = 1 Y no reembeeo los M-1 chunks viejos Y cambio de metadata-prefix, modelo o dimensión fuerza reembedding
3. Caer a fallback si backend no responde: Dado config con backend no disponible Cuando ejecuto "mi-lsp index --semantic" Entonces warning registrado de backend timeout Y el usuario puede continuar con `mi-lsp nav wiki search`

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
- Positivo: `TP-SEM / TC-SEM-004`
- Positivo: `TP-SEM / TC-SEM-005`
- Negativo: `TP-SEM / TC-SEM-006`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-SEM-002
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-SEM-01.md
exports:
- RF-SEM-002
- RF-SEM-002 - Indexar y embeber semanticamente chunks de markdown por heading, incremental
  por hash
agent_must_read:
- .docs/wiki/05_requirements/RF-SEM-002.md
- .docs/wiki/04_flows/FL-SEM-01.md
- .docs/wiki/06_tests/TP-SEM.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-SEM-002.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/06_tests/TP-SEM.md sin inferir que
  fueron ejecutados.
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-SEM-002.md
- .docs/wiki/06_tests/TP-SEM.md
```
