# RF-WIKI-001 - Buscar en documentacion de todos los workspaces federados

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WIKI-001"
kind: "requirement"
audience: "llm-first"
imports:
  - '[[FL-WIKI-01]]'
exports:
  - '[[TP-WIKI]]'
  - '[[CT-NAV-WIKI]]'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/04_RF/RF-WIKI-001.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-001.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-001.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WIKI-001.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WIKI-001 |
| Titulo | Buscar en documentacion de todos los workspaces federados |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-WIKI-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Al menos un workspace registrado con `docs_ready=true` | funcional | obligatorio |
| `~/.mi-lsp/registry.toml` accesible | operativa | obligatorio |
| Flag `--all-workspaces` presente | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `pattern` | string | si | CLI | non-empty pattern | RF-WIKI-001 |
| `--all-workspaces` | bool | si | CLI | flag presente | RF-WIKI-001 |
| `--layer` | enum | no | CLI | `01`, `02`, ..., `09`, `*` o ausente | RF-WIKI-001 |
| `--include-content` | bool | no | CLI | include full doc content in items | RF-WIKI-001 |
| `--top` | entero | no | CLI | 1..1000, default 50 global | RF-WIKI-001 |
| `--offset` | entero | no | CLI | >= 0, default 0 | RF-WIKI-001 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-WIKI-001 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `nav wiki search <pattern> --all-workspaces [--layer X] [--include-content] [--top N] [--offset M]`.
2. El core itera sobre workspaces registrados con `docs_ready=true` con semaphore=4.
3. Para cada workspace, ejecuta la busqueda local contra `.mi-lsp/index.db` (tablas `doc_records`/`doc_edges`/`doc_mentions`).
4. Para una query natural, calcula un score local y determinista por cobertura léxica de términos y evidencia de heading/passage/search_text; owner_hint, family, layer y owner-prefix solo desempatan o enrutan y no desplazan cobertura completa por sí solos. Las queries DocID/source exactas conservan su precedencia canónica.
5. Fusiona los resultados globales usando el mismo score emitido por cada workspace, ordena por score descendente y resuelve empates por path y doc_id antes de aplicar los límites globales.
6. Cada item incluye campo `workspace: <alias>` anotando su origen.
7. El truncador aplica presupuesto y devuelve envelope estable.
8. La CLI devuelve resultado en formato solicitado.
9. Las menciones de DocID se consideran confiables solo cuando el índice documental porta la versión vigente del extractor literal. Un índice heredado puede conservar referencias únicamente si el Markdown canónico existe, está dentro del workspace, no excede el límite de lectura y su hash coincide; de lo contrario, la coincidencia se omite y se informa como no verificada.

### Gramática literal soportada

Los identificadores estándar usan `FL|RS|RF|TP|TECH|CT|DB|AE` seguido de segmentos alfanuméricos separados por guiones. Se aceptan puntuación terminal y el sufijo `.md` en enlaces Markdown; no se aceptan continuaciones arbitrarias como `.extra`, `.md.EXTRA`, ni se confunden `RF-X-1`, `RF-X-1-EXTRA` y `RF-X-10`. La identidad propietaria del documento permanece separada de sus referencias.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `items` | lista | usuario/skill | array con doc hits anotados con `workspace` |
| `workspace` | string | usuario/skill | workspace de origen de cada item |
| `score` | numero | usuario/skill | score de ranking calculado |
| `truncated` | bool | usuario/skill | explicita recorte |
| `warnings` | lista | usuario/skill | workspaces fallidos con razon |
| `stats` | objeto | usuario/skill | `workspaces_queried`, `workspaces_failed`, `total_hits_before_truncation` |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WIKI_NO_WORKSPACES` | ningun workspace tiene `docs_ready=true` | --all-workspaces sin muestras | abortar con error explicito |
| `WIKI_SEARCH_TIMEOUT` | workspace supera 30s timeout | respuesta lenta | workspace en `workspaces_failed[]`, continuar |
| `WIKI_INDEX_CORRUPTED` | tabla doc_records no existe o esta daña | index.db invalido | workspace en `workspaces_failed[]` con razon, continuar |

## 7. Special Cases and Variants

- Si `--layer` es presente, filtrar hits por capa (e.g., `--layer 04_RF` -> solo items en RF docs).
- Si `--include-content`, adjuntar texto completo del documento a cada item (aumenta payload).
- Si `--top=0`, ignorar limite (peligroso; reservar para uso interno).
- `--offset` sin `--top` aplica default `--top=50`.
- El ranking de `nav wiki search` es search-local y no modifica el scorer compartido de `nav ask`, `nav route` o `nav pack`; su score expuesto coincide con el orden natural y el merge global.
- La evidencia de línea se lee del Markdown canónico, incluye como máximo un vecino contiguo por lado y omite rango/evidence cuando el archivo falta; `snippet` indexado queda como fallback transparente.

## 8. Data Model Impact

- `QueryEnvelope` con array de items; cada item anotado con `workspace: string`
- Statisitcas globales de fan-out: `workspaces_queried`, `workspaces_failed`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Buscar en todos los workspaces
  Given al menos dos workspaces registrados con docs_ready=true
  When ejecuto "mi-lsp nav wiki search 'governance' --all-workspaces --format toon"
  Then la respuesta contiene items de ambos workspaces
  And cada item tiene campo "workspace" con alias distinto
  And "stats.workspaces_queried" >= 2

Scenario: Respetar ranking de score
  Given una busqueda que retorna hits de multiples workspaces
  When ejecuto dos veces con el mismo pattern y --all-workspaces
  Then el order de items es identico en ambas invocaciones
  And los primeros items tienen mayor score

Scenario: Aplicar offset y top correctamente
  Given una busqueda con >100 hits globales
  When ejecuto "nav wiki search 'doc' --all-workspaces --top 10 --offset 20"
  Then retorna exactamente 10 items
  And son items 21-30 del ranking global
  And "truncated=true"
```

## 10. Test Traceability

- Base federada: `TP-WIKI / TC-WIKI-001..004`.
- Identidad, ownership y source IDs: `TP-WIKI / TC-WIKI-044..060`.
- Relevancia natural, evidencia de líneas, diacríticos, límites literales y stale fallback: `TP-WIKI / TC-WIKI-061..068`.
- Snapshot documental, marcador de extractor y rollback: `TP-WIKI / TC-WIKI-069..070`.

```toon
doc_id: RF-WIKI-001
block_id: rf-wiki-001-implementation-trace
kind: implementation-trace
source_of_truth: this
requirement: RF-WIKI-001
implementation:
  search_admission_ranking_and_evidence:
    - internal/service/wiki_search.go
    - internal/store/queries_docs.go
  identity_and_source_extraction:
    - internal/docidentity/identity.go
    - internal/docgraph/docgraph.go
    - internal/wikisource/parser.go
  snapshot_publication_and_freshness:
    - internal/store/index_publish.go
    - internal/store/meta.go
    - internal/store/doc_snapshot.go
    - internal/store/queries_incremental.go
    - internal/indexer/indexer.go
oracles:
  service_search: internal/service/wiki_search_test.go
  identity: [internal/docidentity/identity_test.go, internal/docgraph/identity_test.go, internal/docgraph/reference_identity_test.go]
  source_parser: internal/wikisource/parser_test.go
  snapshot: [internal/indexer/indexer_test.go, internal/store/doc_identity_snapshot_test.go]
  declared_cases: [.docs/wiki/06_pruebas/TP-WIKI.md, .docs/wiki/06_matriz_pruebas_RF.md]
validation_basis: attested_prior_review_without_reexecution
canonical_evidence: source_and_test_paths
not_canonical: [temporary_logs, installed_index, live_refresh, embeddings]
limitations:
  graph_activation_atomicity: not_promised_by_document_snapshot_marker
  legacy_mentions: bounded_source_hash_confirmation_when_marker_is_stale
```

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que todos los workspaces responden rapido
  - no omitir campo `workspace` en items
  - no cambiar funcion de scoring sin actualizar RF y CT
- Decisiones cerradas:
  - semaphore=4 como default para fan-out
  - timeout 30s por workspace
  - ranking: score search-local determinista por cobertura y evidencia de contenido; no hereda owner_hint como autoridad
- TODO explicit = 0
- Fuera de alcance:
  - transporte de red (SSH, HTTP); CLI puro
  - cache cross-workspace (cada invocacion consulta registry fresca)
