# RF-IDX-002 - Indexar incrementalmente usando git status para detectar archivos modificados

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-IDX-002"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-IDX-002]]'
exports:
  - 'RF-IDX-002'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-IDX-002.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-IDX-002.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-IDX-002.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-IDX-002 |
| Titulo | Indexar incrementalmente usando git status para detectar archivos modificados |
| Actores | Daemon, Skill, CLI |
| Prioridad | alta |
| Severidad | media |
| FL origen | FL-IDX-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Git disponible en workspace | tecnica | preferido |
| `index.db` existente | funcional | obligatorio |
| Workspace resoluble | funcional | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `workspace` | string | si | CLI | nombre o path | RF-IDX-002 |
| `--clean` | booleano | no | CLI | si true, fuerza full re-index | RF-IDX-002 |
| `--mode` | enum | no | CLI | `full`, `docs`, `catalog` en `index start` | RF-IDX-002 |

## 4. Process Steps (Happy Path)

1. La CLI recibe workspace name.
2. El indexador verifica disponibilidad de git.
3. Si git disponible, ejecuta `git status --porcelain` para detectar archivos changed/added/deleted.
4. Si el delta toca `.docs/wiki`, `README*`, `docs/`, `.docs/` o `.docs/wiki/_mi-lsp/read-model.toml`, aborta el modo incremental y hace full re-index.
5. Si no, re-indexa solo archivos de codigo en la lista de cambios.
6. Archivos deletados se eliminan del indice.
7. Actualiza `index.db`, cierra el job y devuelve stats de cambios.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la indexacion |
| `stats` | objeto | usuario/skill | files_changed, files_skipped, files_deleted, duration |
| `mode` | string | usuario/skill | `incremental` o `full` (fallback) |
| `warnings` | lista | usuario/skill | git unavailable o docs change con fallback notice |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `IDX_GIT_UNAVAILABLE` | git no disponible | git command falla | degrade a full index con warning |
| `IDX_DATABASE_ERROR` | error escribiendo index.db | lock o I/O error | abortar con error explicito |
| `IDX_WORKSPACE_UNRESOLVED` | workspace no encontrado | nombre invalido | error explicito |

## 7. Special Cases and Variants

- Si git no esta disponible, fallback a full index con warning.
- `--clean` siempre hace full re-index sin usar git status.
- Archivos nuevos se indexan normalmente.
- Archivos deletados en git se eliminan del indice.
- Cambios en docs o en el `read-model` no intentan incremental parcial; fuerzan full re-index para evitar grafo inconsistente.
- `index start --mode full` puede usar incremental si no hay `--clean`; si no publica generacion full, marca la generacion candidata como `skipped` y conserva los punteros activos previos.
- `index start --mode catalog` recompone solo catalogo y publica `active_catalog_generation_id`.
- `index start --mode docs` recompone solo docs + memoria y publica `active_docs_generation_id` / `active_memory_generation_id`.
- En `mode docs`, el skip-reparse por `content_hash` aplica solo cuando el job no trae `--clean`; con `--clean` se reparsea todo el contenido documental del alcance sin reutilizar filas previas por hash, manteniendo la publicacion atomica y sin borrar el canon en disco.

### Contrato observable del reparse documental

```toon
block_id: RF-IDX-002-DOCS-OBSERVABLE
kind: normative
source_of_truth: normative
requirement_id: RF-IDX-002
resumen_publico:
  campo: warnings
  formato: docs_skip_reparse parsed=<entero> skipped=<entero>
  disponibilidad: toda lectura documental completada, incluso con skipped=0 o sin documentos
  parsed: cantidad de rutas con parse exitoso en esta lectura
  skipped: cantidad de rutas reutilizadas del snapshot previo por hash e identidad coincidentes
  exclusiones: errores de lectura o parse y rutas sin documento no incrementan ninguno; conservan sus advertencias
  alcance: resumen existente de docgraph, también visible en otros modos que lo consumen; no es un contador de símbolos ni bloques
  salida_docs: index start --mode docs --wait e index --docs-only devuelven el resumen en warnings
  no_persistencia: index status expone el job, no conserva este resumen terminal; guardar la salida de la ejecución que espera
  clean: omite el snapshot previo; los contadores nunca se deducen del flag ni de docs=N
punteros_publicos:
  nav_git_frontier: generations.catalog y generations.docs leen los punteros activos de catálogo y docs; generations.graph es una identidad distinta
  ausencia: valor null con generation_reasons cuando no está disponible; no equivale a publicación
  index_status: generation_id identifica la generación candidata del job; succeeded no es lectura independiente del puntero activo
  wiki_code_context: doc_generation_id y provenance.docs_generation cuando la superficie documental devuelve este contexto; code_generation_id identifica grafo, no catálogo
  workspace_status_full: doc_count, docs_index_ready, index_symbols e index_ready; memory y memory_pointer pueden aparecer, sin ID de generación de memoria
limite_memoria:
  contrato_interno: el publisher activa active_docs_generation_id y active_memory_generation_id en la misma transacción documental y conserva active_catalog_generation_id
  no_observable: active_memory_generation_id no está expuesto por estas superficies; memory_pointer es un puntero de reentrada, no de generación
  aceptacion: comprobar publicación docs y preservación del catálogo en cada corrida mediante nav git-frontier y navegación del mismo símbolo; no declarar acreditado el puntero interno de memoria
  pendiente: evidencia independiente de generación de memoria, dueño parent_final_verify, vencimiento antes del cierre del objetivo docs-clean-reparse-20261013
```

## 8. Data Model Impact

- `FileRecord`
- `DocRecord`
- `DocEdge`
- `DocMention`
- `index.db` schema

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Indexar incrementalmente con git disponible
  Given git disponible y archivos modificados
  When ejecuto "mi-lsp index --workspace myapp"
  Then re-indexo solo archivos changed/added/deleted
  And "mode" es "incremental"
  And stats reportan numeros correctos

Scenario: Fallback a full index si git no disponible
  Given git no disponible
  When ejecuto la indexacion
  Then degrade a full re-index con warning
  And "mode" es "full"
  And operacion completa normalmente

Scenario: Forzar full index por cambios documentales
  Given el delta incluye cambios en `.docs/wiki` o `read-model.toml`
  When ejecuto la indexacion incremental
  Then hago full re-index
  And "mode" es "full"
```

## 10. Test Traceability

- Positivo: `TP-IDX / TC-IDX-007`
- Positivo: `TP-IDX / TC-IDX-008`
- Manual pendiente: `TP-IDX / TC-IDX-028` (contadores y punteros públicos; memoria interna no acreditada)
- Positivo: `TP-IDX / TC-IDX-009`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que git siempre disponible
  - no fallar si git no esta
- Decisiones cerradas:
  - incremental con git, fallback full
  - docs/profile cambian -> full re-index
  - `--clean` fuerza full sin condiciones
- TODO explicit = 0
