---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-001
kind: requirement
status: active
title: RF-WIKI-001 - Buscar en documentacion de todos los workspaces federados
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/docgraph/docgraph.go
    role: behavior
  - path: internal/docidentity/identity.go
    role: behavior
  - path: internal/indexer/indexer.go
    role: behavior
  - path: internal/service/wiki_search.go
    role: behavior
  - path: internal/store/doc_snapshot.go
    role: behavior
  - path: internal/store/index_publish.go
    role: behavior
  - path: internal/store/meta.go
    role: behavior
  - path: internal/store/queries_docs.go
    role: behavior
  - path: internal/store/queries_incremental.go
    role: behavior
  - path: internal/wikisource/parser.go
    role: behavior
---

## Propósito

RF-WIKI-001 - Buscar en documentacion de todos los workspaces federados

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WIKI-001
- **Campo**: Título; **Valor**: Buscar en documentacion de todos los workspaces federados
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-WIKI-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Al menos un workspace registrado con `docs_ready=true`; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `~/.mi-lsp/registry.toml` accesible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Flag `--all-workspaces` presente; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `nav wiki search <pattern> --all-workspaces [--layer X] [--include-content] [--top N] [--offset M]`.
2. El core itera sobre workspaces registrados con `docs_ready=true` con semaphore=4.
3. Para cada workspace, ejecuta la busqueda local contra `.mi-lsp/index.db` (tablas `doc_records`/`doc_edges`/`doc_mentions`).
4. Para una query natural, calcula un score local y determinista por cobertura léxica de términos y evidencia de heading/passage/search_text; owner_hint, family, layer y owner-prefix solo desempatan o enrutan y no desplazan cobertura completa por sí solos. Las queries DocID/source exactas conservan su precedencia canónica.
5. Fusiona los resultados globales usando el mismo score emitido por cada workspace, ordena por score descendente y resuelve empates por path y doc_id antes de aplicar los límites globales.
6. Cada item incluye campo `workspace: <alias>` anotando su origen.
7. El truncador aplica presupuesto y devuelve envelope estable.
8. La CLI devuelve resultado en formato solicitado.
9. Las menciones de DocID se consideran confiables solo cuando el índice documental porta la versión vigente del extractor literal. Un índice heredado puede conservar referencias únicamente si el Markdown canónico existe, está dentro del workspace, no excede el límite de lectura y su hash coincide; de lo contrario, la coincidencia se omite y se informa como no verificada.

#### Gramática literal soportada

Los identificadores estándar usan `FL|RS|RF|TP|TECH|CT|DB|AE` seguido de segmentos alfanuméricos separados por guiones. Se aceptan puntuación terminal y el sufijo `.md` en enlaces Markdown; no se aceptan continuaciones arbitrarias como `.extra`, `.md.EXTRA`, ni se confunden `RF-X-1`, `RF-X-1-EXTRA` y `RF-X-10`. La identidad propietaria del documento permanece separada de sus referencias.

### 7. Casos especiales y variantes
- Si `--layer` es presente, filtrar hits por capa (e.g., `--layer 04_RF` -> solo items en RF docs).
- Si `--include-content`, adjuntar texto completo del documento a cada item (aumenta payload).
- Si `--top=0`, ignorar límite (peligroso; reservar para uso interno).
- `--offset` sin `--top` aplica default `--top=50`.
- El ranking de `nav wiki search` es search-local y no modifica el scorer compartido de `nav ask`, `nav route` o `nav pack`; su score expuesto coincide con el orden natural y el merge global.
- La evidencia de línea se lee del Markdown canónico, incluye como máximo un vecino contiguo por lado y omite rango/evidence cuando el archivo falta; `snippet` indexado queda como fallback transparente.

### 11. Decisiones cerradas
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

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `pattern`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: non-empty pattern; **RN**: RF-WIKI-001
- **Campo**: `--all-workspaces`; **Tipo**: bool; **Req.**: si; **Origen**: CLI; **Validacion**: flag presente; **RN**: RF-WIKI-001
- **Campo**: `--layer`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `01`, `02`, ..., `09`, `*` o ausente; **RN**: RF-WIKI-001
- **Campo**: `--include-content`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: include full doc content in items; **RN**: RF-WIKI-001
- **Campo**: `--top`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: 1..1000, default 50 global; **RN**: RF-WIKI-001
- **Campo**: `--offset`; **Tipo**: entero; **Req.**: no; **Origen**: CLI; **Validacion**: >= 0, default 0; **RN**: RF-WIKI-001
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-WIKI-001

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array con doc hits anotados con `workspace`
- **Campo**: `workspace`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: workspace de origen de cada item
- **Campo**: `score`; **Tipo**: número; **Destino**: usuario/skill; **Efecto observable**: score de ranking calculado
- **Campo**: `truncated`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: explícita recorte
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: workspaces fallidos con razon
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: `workspaces_queried`, `workspaces_failed`, `total_hits_before_truncation`

### 8. Impacto en el modelo de datos
- `QueryEnvelope` con array de items; cada item anotado con `workspace: string`
- Statisitcas globales de fan-out: `workspaces_queried`, `workspaces_failed`

## Errores

### 6. Errores tipados
- **Código**: `WIKI_NO_WORKSPACES`; **Causa**: ningún workspace tiene `docs_ready=true`; **Trigger**: --all-workspaces sin muestras; **Respuesta esperada**: abortar con error explícito
- **Código**: `WIKI_SEARCH_TIMEOUT`; **Causa**: workspace supera 30s timeout; **Trigger**: respuesta lenta; **Respuesta esperada**: workspace en `workspaces_failed[]`, continuar
- **Código**: `WIKI_INDEX_CORRUPTED`; **Causa**: tabla doc_records no existe o esta daña; **Trigger**: index.db inválido; **Respuesta esperada**: workspace en `workspaces_failed[]` con razon, continuar

## Criterios de aceptación (numerados)

1. Buscar en todos los workspaces: Dado al menos dos workspaces registrados con docs_ready=true Cuando ejecuto "mi-lsp nav wiki search 'governance' --all-workspaces --format toon" Entonces la respuesta contiene items de ambos workspaces Y cada item tiene campo "workspace" con alias distinto Y "stats.workspaces_queried" >= 2
2. Respetar ranking de score: Dado una busqueda que retorna hits de multiples workspaces Cuando ejecuto dos veces con el mismo pattern y --all-workspaces Entonces el order de items es identico en ambas invocaciones Y los primeros items tienen mayor score
3. Aplicar offset y top correctamente: Dado una busqueda con >100 hits globales Cuando ejecuto "nav wiki search 'doc' --all-workspaces --top 10 --offset 20" Entonces retorna exactamente 10 items Y son items 21-30 del ranking global Y "truncated=true"

4. La identidad propietaria DocID/doc_id del documento se distingue de imports, menciones, wikilinks, block_id y record_id; una referencia no adquiere autoridad de owner. Conflictos de identidad no se resuelven escogiendo silenciosamente una declaración.
5. La extracción literal distingue identificadores completos, prefijos, sufijos, fences, Unicode y referencias; no confunde RF-X-1, RF-X-1-EXTRA y RF-X-10 ni admite continuaciones arbitrarias .extra o .md.EXTRA.
6. Una query natural se admite por evidencia FTS/léxica del contenido y su cobertura; los hints solo enrutan o desempatan, sin inventar relevancia. El score emitido conserva el orden del merge global.
7. La evidencia de líneas procede del Markdown canónico con hasta un vecino por lado; si no existe el archivo no se inventa rango y el snippet indexado queda como fallback transparente.
8. El snapshot documental publica Docs, source tables y FTS sin estado parcial. Un marker literal vigente habilita confianza en menciones; un marker heredado exige confirmación acotada de fuente/hash o informa omisión. No se promete atomicidad de activación de grafo desde ese marker.

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
- Base federada: `TP-WIKI / TC-WIKI-001..004`.
- Identidad, ownership y source IDs: `TP-WIKI / TC-WIKI-044..060`.
- Relevancia natural, evidencia de líneas, diacríticos, límites literales y stale fallback: `TP-WIKI / TC-WIKI-061..068`.
- Snapshot documental, marcador de extractor y rollback: `TP-WIKI / TC-WIKI-069..070`.

- **Marcador estable**: rf-wiki-001-implementation-trace
- **RF**: RF-WIKI-001
- **Implementación**:
  - **search_admission_ranking_and_evidence**:
    - internal/service/wiki_search.go
    - internal/store/queries_docs.go
  - **identity_and_source_extraction**:
    - internal/docidentity/identity.go
    - internal/docgraph/docgraph.go
    - internal/wikisource/parser.go
  - **snapshot_publication_and_freshness**:
    - internal/store/index_publish.go
    - internal/store/meta.go
    - internal/store/doc_snapshot.go
    - internal/store/queries_incremental.go
    - internal/indexer/indexer.go
- **oracles**:
  - **service_search**: internal/service/wiki_search_test.go
  - **identity**:
    - internal/docidentity/identity_test.go
    - internal/docgraph/identity_test.go
    - internal/docgraph/reference_identity_test.go
  - **source_parser**: internal/wikisource/parser_test.go
  - **snapshot**:
    - internal/indexer/indexer_test.go
    - internal/store/doc_identity_snapshot_test.go
  - **declared_cases**:
    - .docs/wiki/06_tests/TP-WIKI.md
    - .docs/wiki/06_tests/
- **validation_basis**: attested_prior_review_without_reexecution
- **canonical_evidence**: source_and_test_paths
- **not_canonical**:
  - temporary_logs
  - installed_index
  - live_refresh
  - embeddings
- **limitations**:
  - **graph_activation_atomicity**: not_promised_by_document_snapshot_marker
  - **legacy_mentions**: bounded_source_hash_confirmation_when_marker_is_stale

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WIKI-001
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-WIKI-01.md
exports:
- RF-WIKI-001
- RF-WIKI-001 - Buscar en documentacion de todos los workspaces federados
agent_must_read:
- .docs/wiki/05_requirements/RF-WIKI-001.md
- .docs/wiki/04_flows/FL-WIKI-01.md
- .docs/wiki/06_tests/TP-WIKI.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WIKI-001.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/docgraph/docgraph.go
- internal/docidentity/identity.go
- internal/indexer/indexer.go
- internal/service/wiki_search.go
- internal/store/doc_snapshot.go
- internal/store/index_publish.go
- internal/store/meta.go
- internal/store/queries_docs.go
- internal/store/queries_incremental.go
- internal/wikisource/parser.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-WIKI.md, ejecutar el
  comando publicado: go test ./internal/service ./internal/cli ./internal/docgraph
  ./internal/indexer -count=1 -run ''TestClassifyWikiMapHubDefaultsRemainLocked|TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder|TestTrimWikiMapHubs|TestWalkWikiMapDocs|TestNavWikiMapUsesDirectExecution|TestKnowledgeWikiRoots|TestDocumentGraph''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-WIKI-001.md
- .docs/wiki/06_tests/TP-WIKI.md
```
