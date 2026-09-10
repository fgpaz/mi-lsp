# CT-NAV-WIKI

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-WIKI"
kind: "wiki-doc"
audience: "llm-first"
imports:
  - '[[RF-WIKI-001]]'
  - '[[RF-WIKI-002]]'
  - '[[RF-WIKI-003]]'
  - '[[RF-WIKI-004]]'
  - '[[RF-WIKI-005]]'
  - '[[RF-WIKI-006]]'
  - '[[RF-WIKI-007]]'
exports:
  - 'CT-NAV-WIKI'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
agent_may_edit:
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
```

## Invocacion

```
mi-lsp nav wiki search <query> [--all-workspaces] --workspace <alias> [--layer RS,RF,FL,TP,CT,TECH,DB] [--top N] [--offset N] [--include-content] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki route <task> [--all-workspaces] --workspace <alias> [--full] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki pack <task> [--all-workspaces] --workspace <alias> [--rf RF-*] [--fl FL-*] [--doc <path>] [--full] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki trace <DOC-ID|--all> [--all-workspaces] --workspace <alias> [--summary] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki map --workspace <alias> [--max-items N] [--token-budget N] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki inventory [--all-workspaces] --workspace <alias> [--with-layer-counts] [--format compact|json|text|toon|yaml]

mi-lsp nav wiki map --workspace <alias> [--format compact|json|text|toon|yaml]
mi-lsp nav wiki-root --workspace <alias> [--role producto|ecosistema|gobierno_local] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki root --workspace <alias> [--role producto|ecosistema|gobierno_local] [--format compact|json|text|toon|yaml]

mi-lsp nav wiki validate-harness --workspace <alias> [--format compact|json|text|toon|yaml]
mi-lsp nav wiki validate-source --workspace <alias> [--paths <path[,path...]>] [--ids <doc-id[,doc-id...]>] [--format compact|json|text|toon|yaml]
```

## Semantica

`nav wiki` es la puerta documental explicita para agentes. `wiki search` usa el docgraph repo-local y el scorer owner-aware para devolver candidatos wiki, mientras `wiki route`, `wiki pack` y `wiki trace` reutilizan la semantica y el shape de `nav route`, `nav pack` y `nav trace`. `wiki map` publica un catálogo compacto de hubs de una wiki de conocimiento (`wiki/` numerada y `bibliotecas/`) sin cuerpos completos y sin fan-out `--all-workspaces`. `wiki-root` (alias `wiki root`) publica la raíz portable; el envelope vive en [[CT-NAV-WIKI-ROOT]]. `wiki validate-harness` compila readiness de contratos `SDD-HARNESS-v1` sobre los docs gobernados. `wiki validate-source` compila readiness de artefactos que declaran `wiki_source_protocol: SDD-WIKI-SOURCE-v1`; los docs no migrados no son bloqueantes. `wiki search` acepta `RS` como layer outcome y `wiki trace` acepta `RS-*`, `RF-*`, `TP-*`, doc IDs tecnicos exactos (`TECH-*`, `DB-*`, `CT-*`) y source IDs exactos; para IDs tecnicos debe preferir el documento cuyo `doc_id` coincide exactamente antes de usar menciones o fallbacks RF. `--all` sigue recorriendo el set RF canonico, y cuando necesita fallback a disco debe priorizar las rutas gobernadas por `00`/`read-model` antes de caer a layouts legacy.

### Frescura de menciones literales

`doc_mentions` de tipo `doc_id` solo tiene autoridad de búsqueda cuando `workspace_meta.doc_identity_snapshot_version` coincide con la versión vigente del extractor. La publicación full o docs-only respaldada por el extractor actualiza ese marcador dentro de la misma transacción que reemplaza documentos, edges, menciones, bloques, records y bindings; cualquier escritor documental no versionado lo invalida. Si falta o es antigua, la búsqueda confirma cada candidato contra su Markdown canónico (ruta segura dentro del workspace, tamaño acotado y `content_hash` coincidente) usando la misma gramática literal; las fuentes ausentes o stale se informan como no verificadas y no se convierten en hits.

La gramática admite IDs estándar con segmentos alfanuméricos separados por guiones, puntuación terminal y `.md` en enlaces; rechaza continuaciones como `.extra` y `.md.EXTRA`, y mantiene distintos `RF-X-1`, `RF-X-1-EXTRA` y `RF-X-10`. La identidad propietaria (`doc_id`) no se deriva de las referencias.

### Límite de atomicidad documental y grafo

El marcador `workspace_meta.doc_identity_snapshot_version` y las familias documentales (Docs, edges, mentions, source blocks/records, bindings, FTS y memoria de reentrada) se publican atómicamente solo en las rutas respaldadas por el extractor vigente. Esta garantía es del snapshot documental: no equivale a una garantía de activación del puntero/generación Graph en la misma transacción para la ruta foreground. Las variantes fenced de job pueden incluir una publicación graph explícita en su transacción; la ruta foreground puede activar graph por separado y dejarlo stale. No se debe declarar que el marker vuelve atómica la activación graph ni que `docs-only` valida graph, recall o embeddings.

```toon
doc_id: CT-NAV-WIKI
block_id: ct-nav-wiki-document-snapshot-boundary
kind: atomicity-boundary
source_of_truth: this
marker: workspace_meta.doc_identity_snapshot_version
atomic_document_families: [doc_records, doc_edges, doc_mentions, doc_source_blocks, doc_source_records, doc_artifact_bindings, fts, reentry_memory]
foreground_graph_activation: separate_or_stale_allowed
fenced_job_graph_activation: same_transaction_when_explicit_graph_publication_is_supplied
not_guaranteed: [docs_and_graph_single_transaction_for_all_paths, installed_index, live_refresh, embeddings]
evidence:
  - internal/store/index_publish.go
  - internal/store/meta.go
  - internal/store/doc_snapshot.go
  - internal/indexer/indexer.go
  - internal/indexer/indexer_test.go
```

## Integración aditiva del puente wiki ↔ código

```toon
doc_id: CT-NAV-WIKI
block_id: CT-NAV-WIKI.live-wiki-code-surfaces
kind: additive-navigation-contract
source_of_truth: this
status: implemented_slice
surfaces:
  trace: [nav.trace, nav.wiki.trace]
  pack: [nav.pack, nav.wiki.pack]
  code_context: [nav.related, nav.neighbors]
  preparation: [nav.prepare]
  impact: [nav.change-pack]
path_bearing_records:
  bindings: [doc_path, target_path, target_symbol, block_id, binding_ref]
  context: [path, doc_path, source_doc, source_block, source_line, doc_id]
  control: [direction, status, provenance, freshness, omissions, continuation, classification, cost]
semantics:
  direction: [wiki_to_code, code_to_wiki]
  freshness: bounded_fresh_per_domain
  read_your_writes: request_scoped_ram_overlay
  exact: declared_binding_before_graph_or_lexical
  tests: separate_from_direct_code
  supporting: graph_current_only
  candidates: never_direct_edges
compatibility:
  existing_envelopes: preserved
  additive_field: wiki_code_context
  no_new_public_command: true
  direct_daemon: same_canonical_items_order_omissions_and_digest
  query_writes: forbidden
authority:
  canonical_wiki: unchanged
  index_catalog_graph: derived_only
  raw_and_audit: excluded_from_primary_results
lifecycle:
  planned: nonnavigable_by_default
  retired: excluded_by_default
  historical: explicit_old_id_or_path_redirect_only
verify:
  - "FINAL_VERIFY e1835ee: go test ./... PASS (28 paquetes)"
  - "wiki-code-bridge-runner/v1: PASS; inventory=37"
evidence:
  - internal/service/app.go
  - internal/service/trace.go
  - internal/service/pack.go
  - internal/service/related.go
  - internal/service/prepare.go
  - internal/service/change_pack.go
  - internal/model/wiki_code_context.go
  - .docs/wiki/06_pruebas/TP-WIKI.md
```

## Envelope `--all-workspaces`

```toon
block_id: ct-nav-wiki-envelope-all-workspaces
description: "Extensión envelope para queries cross-workspace"
extra_item_fields:
  workspace: "alias del workspace de origen (registry)"
  host: "opcional vacío; Hermes lo setea al mergear cross-host"
extra_stats:
  workspaces_queried: "int >= 1; número total de workspaces iterados"
  workspaces_failed: "array de {alias, reason}; workspaces que fallaron"
  workspaces_count: "int; total aliases procesados exitosamente"
  truncated_per_workspace: "bool; si al menos uno fue truncado"
backward_compat: |
  Cuando --all-workspaces=false (default), el envelope es idéntico al actual.
  Los clientes legacy que ignoren los nuevos campos siguen recibiendo respuesta válida.
semantics:
  mergeable: true
  precedence: "workspace-local primero; Hermes puede mergear order cross-host"
  item_deduplication: "por doc_id + workspace; linaje preservado en why"
```

## Envelope `wiki search`

```json
{
  "ok": true,
  "backend": "wiki.search",
  "workspace": "alias",
  "items": [
    {
      "doc_id": "RF-QRY-016",
      "path": ".docs/wiki/04_RF/RF-QRY-016.md",
      "title": "RF-QRY-016 - Explorar la wiki con una superficie dedicada para agentes",
      "layer": "RF",
      "family": "functional",
      "stage": "requirements",
      "score": 120,
      "line_start": 1,
      "line_end": 40,
      "why": ["doc_id=RF-QRY-016", "canonical_match"],
      "lookup_status": {
        "query": "RF-QRY-016",
        "workspace": "alias",
        "index_freshness": "current",
        "governance_sync": "in_sync",
        "match_kind": "canonical_indexed_id",
        "doc_id": "RF-QRY-016",
        "path": ".docs/wiki/04_RF/RF-QRY-016.md",
        "layer": "RF",
        "stage": "requirements",
        "rank_reason": "doc_id=RF-QRY-016,canonical_match",
        "total_matches": 1,
        "shown_matches": 1
      },
      "next_queries": [
        "mi-lsp nav wiki pack \"wiki agentes\" --workspace alias --doc .docs/wiki/04_RF/RF-QRY-016.md --format toon",
        "mi-lsp nav wiki trace RF-QRY-016 --workspace alias --format toon",
        "mi-lsp nav multi-read .docs/wiki/04_RF/RF-QRY-016.md:1-120 --workspace alias --format toon"
      ]
    }
  ],
  "warnings": [],
  "stats": {"files": 1},
  "truncated": false
}
```

## Contract `wiki search`

```toon
block_id: ct-nav-wiki-search-contract
subcommand: "nav wiki search"
flag_all_workspaces: optional, default false
flags_preserved:
  - "--layer RS,RF,FL,TP,CT,TECH,DB"
  - "--top N"
  - "--offset N"
  - "--include-content"
  - "--format compact|json|text|toon|yaml"
envelope_extension: "ct-nav-wiki-envelope-all-workspaces"
semantics: |
  Sin --all-workspaces: busca en workspace específico (default actual).
  Con --all-workspaces: itera todos los aliases registrados, ejecuta query en paralelo,
  agrega workspace a cada item, mergea items, mantiene truncated_per_workspace.
  En cada workspace, la secuencia elegible se ensambla completa y de forma determinista:
  primero el DocRecord cuyo doc_id coincide exactamente, después las coincidencias exactas
  de doc_source_blocks/doc_source_records y, por último, el orden owner-aware existente.
  La deduplicación es por path, el filtro de capa se aplica antes de offset/top y la
  paginación se aplica una sola vez. En el workspace único, total_matches cuenta toda
  la secuencia elegible y shown_matches los items emitidos por el servicio antes de
  cualquier límite posterior del envelope; el fanout no promete esos campos por
  workspace. Una declaración source exacta no promociona referencias textuales ni
  reemplaza una autoridad documental ambigua.
  La admisión de relevancia es exclusiva de search y no cambia route, ask o pack:
  una query con forma de identificador conserva únicamente el DocRecord cuyo doc_id
  coincide de forma completa, declaraciones source exactas y referencias que contienen
  el identificador completo con límites de identidad (incluye [[ID]], enlaces ID.md y
  puntuación de frase como `RF-X-1.` o `RF-X-1,`, pero bloquea RF-X-1 frente a
  RF-X-1-EXTRA, RF-X-1_EXTRA, RF-X-1.extra, RF-X-1.md.EXTRA o RF X 1).
  La igualdad exacta de un DocRecord.DocID declarado activa este modo aun para IDs
  no software, con puntos, guiones bajos o Unicode; no se inventa una gramática para
  IDs desconocidos fuera de las formas soportadas. Una query natural exige evidencia
  FTS o léxica real en doc_id, title, search_text o path; familia, layer, owner-prefix
  y owner_hint son señales de ordenación, no evidencia de admisión.
  El esquema vigente no declara alias de búsqueda explícitos en DocsReadProfile; por
  tanto, los alias de route Tier1 siguen orientando consultas sin índice o sin hit, pero
  no fuerzan resultados en una búsqueda literal. El soporte Markdown sin doc_id se
  conserva mediante title/search_text/path y su identidad de evidencia. En queries
  naturales, la comparación léxica local descompone Unicode mediante NFD y elimina
  marcas diacríticas: cache y caché son equivalentes para cobertura, ranking y selección
  de evidencia. Esta normalización no modifica la igualdad literal de DocID/source IDs
  ni el ranking compartido de route, ask o pack.
  Search aplica después de la admisión un score local y determinista:
  cobertura de tokens, heading, passage/snippet y search_text prevalecen sobre owner_hint,
  family, layer y owner-prefix cuando estos solo expresan routing; el score emitido es el
  mismo usado para ordenar y fusionar workspaces. Los empates se resuelven por path y luego
  doc_id. La evidencia de salida se selecciona de líneas reales del path canónico, prioriza
  la mayor cobertura de query y texto significativo. A igual cobertura, el cuerpo prevalece
  sobre frontmatter y metadatos Harness; los bloques TOON del cuerpo siguen siendo evidencia
  válida y los metadatos se conservan como fallback si no hay un pasaje útil. Incluye como
  máximo una línea contigua antes/después y omite el rango si el archivo falta o está stale; snippet/index permanece
  como fallback transparente sin inventar autoridad ni frescura.
```

## Filtros de capa

| Layer | Docs incluidos |
|---|---|
| `RS` | `02_resultados_soluciones_usuario.md`, `02_resultados/*`, `doc_id=RS-*` |
| `FL` | `03_FL*`, `doc_id=FL-*` |
| `RF` | `04_RF*`, `doc_id=RF-*` |
| `TP` | `06_pruebas*`, `doc_id=TP-*` |
| `TECH` | `07_*`, `07_tech/*`, `doc_id=TECH-*` |
| `DB` | `08_*`, `08_db/*`, `doc_id=DB-*` |
| `CT` | `09_*`, `09_contratos/*`, `doc_id=CT-*` |

## Contract `wiki route`

```toon
block_id: ct-nav-wiki-route-contract
subcommand: "nav wiki route"
flag_all_workspaces: optional, default false
flags_preserved:
  - "--full"
  - "--format compact|json|text|toon|yaml"
envelope_extension: "ct-nav-wiki-envelope-all-workspaces"
result_shape: "N mini-routes, uno por workspace; NO super-route mergeado"
semantics: |
  Con --all-workspaces, devuelve array de rutas documentales, cada una
  con su workspace origen. No hay consolidación de campos; cada item preserva
  su doc_id y layer originales.
```

## Roots declarados e identidad de packs

- Un canon declarado puede usar `wiki/`, una ruta relativa externa u otro layout, sin exigir carpetas `03_FL` ni reclasificar documentos `generic` para que el índice sea utilizable.
- Un selector que coincide exactamente con `DocRecord.DocID` y con la identidad declarada del documento elige ese propietario antes de rankings, referencias o fallbacks a `README.md`. Un ID legacy inferido de una mención no desplaza al agregado gobernado que contiene el registro solicitado. Dos paths propietarios del mismo ID producen ambigüedad explícita; `--doc` permite seleccionar el path. Un `--rf`/`--fl` sin propietario no debe transformarse en un candidato rankeado ajeno.
- En Tier 1, el frontmatter y los metadatos de identidad declarados prevalecen sobre menciones del cuerpo. Una declaración vacía o conflictiva no autoriza fallback a una mención. Los registros embebidos legacy de `.docs/wiki` conservan compatibilidad.
- Las rutas del read-model propio de un canon se interpretan desde su raíz, tanto en Tier 1 como en la selección canónica del servicio; la proyección local conserva precedencia. No se reindexa, mueve canon ni cambia autoridad para resolver navegación.
- Un ancla gobernada no indexada conserva su lugar en route/pack; los documentos indexados que sólo la mencionan son apoyo, no propietarios. El pack devuelve el fallback de gobierno con `tier1=anchor_not_indexed` y diagnóstico explícito: no promete un slice indexado completo. La regla también aplica a canons externos con read-model relativo.
- El contrato del documento de gobierno resoluble y su ausencia explícita vive en [[CT-NAV-WIKI-ROOT]].

## Contract `wiki pack`

```toon
block_id: ct-nav-wiki-pack-contract
subcommand: "nav wiki pack"
flag_all_workspaces: optional, default false
flags_preserved:
  - "--rf RF-*"
  - "--fl FL-*"
  - "--doc <path>"
  - "--full"
  - "--format compact|json|text|toon|yaml"
envelope_extension: "ct-nav-wiki-envelope-all-workspaces"
result_shape: "N mini-packs, uno por workspace; NO super-pack mergeado"
semantics: |
  Con --all-workspaces, devuelve array de reading packs, cada uno con su
  workspace origen. Preserva independencia por workspace; cliente es responsable
  de mergear si necesario.
```

## Contract `wiki trace`

```toon
block_id: ct-nav-wiki-trace-contract
subcommand: "nav wiki trace"
flag_all_workspaces: optional, default false
flags_preserved:
  - "--summary"
  - "--format compact|json|text|toon|yaml"
envelope_extension: "ct-nav-wiki-envelope-all-workspaces"
semantics: |
  Sin --all-workspaces: busca DOC-ID en workspace específico.
  Con --all-workspaces: busca DOC-ID en todos los workspaces, agrega
  workspace a cada resultado. Si DOC-ID existe en múltiples workspaces,
  devuelve uno por workspace.
```

## Envelope `wiki validate-harness`

```json
{
  "ok": true,
  "backend": "harness",
  "workspace": "alias",
  "items": [
    {
      "harness_protocol": "SDD-HARNESS-v1",
      "harness_readiness": "ready",
      "harness_verdict": "PASS",
      "harness_blockers": [],
      "harness_warnings": [],
      "harness_contracts_reviewed": 1,
      "harness_links_reviewed": 2,
      "harness_evidence_required": ["artifacts/harness/evidence.md"],
      "harness_evidence_found": ["artifacts/harness/evidence.md"],
      "harness_docs_missing_contract": [],
      "harness_docs_unknown_audience": []
    }
  ]
}
```

Veredictos:

- `PASS`: no hay blockers.
- `WARN`: no hay blockers y existen warnings no bloqueantes; por ejemplo contratos `human` o `dual` con `verify`, `stop_if` o `evidence` vacios.
- `BLOCKED`: faltan contratos requeridos, hay imports/links rotos, conflictos `agent_may_edit` vs `agent_must_not_edit`, audience desconocida o faltan `verify`/`stop_if`/`evidence` en docs `llm-first`.

## Envelope `wiki validate-source`

```json
{
  "ok": true,
  "backend": "wiki.source",
  "workspace": "alias",
  "items": [
    {
      "wiki_source_protocol": "SDD-WIKI-SOURCE-v1",
      "wiki_source_readiness": "ready",
      "wiki_source_verdict": "PASS",
      "wiki_source_artifacts_reviewed": 1,
      "wiki_source_blocks_reviewed": 2,
      "wiki_source_records_reviewed": 5,
      "wiki_source_tables_reviewed": 0,
      "navigation_readiness": "ready"
    }
  ]
}
```

Veredictos:

- `PASS`: los artefactos fuente declarados tienen `doc_id`, fences `toon`, `block_id` y filas typed publicadas.
- `WARN`: no hay blockers, pero quedan warnings no bloqueantes.
- `BLOCKED`: falta `doc_id`, hay `doc_id` duplicado, falta fence `toon`, falta `block_id`, hay tabla Markdown normativa sin excepcion o faltan filas de navegacion en `doc_source_blocks`.

```toon
block_id: ct-nav-wiki-validate-source-scope
kind: validator-contract
source_of_truth: this
verify:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --paths <path> --format toon
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --ids <doc-id> --format toon
scope:
  paths: "comma-separated existing markdown paths"
  ids: "comma-separated document/source IDs"
  selection: "intersection with governed SDD-WIKI-SOURCE-v1 artifacts"
no_match:
  ok: true
  wiki_source_verdict: BLOCKED
  wiki_source_readiness: blocked
  navigation_readiness: blocked
  navigation_blockers: [scope=no_match]
readiness:
  exact_claims: "only source-declared artifacts"
  non_source_docs: "not implicitly promoted and not corpus blockers"
  diagnostics: "hint and blocker are visible"
```

## Diagnosticos

- Si `governance_blocked=true`, `wiki search` devuelve `backend=governance` y no ejecuta ranking documental.
- Si `doc_records` está vacío, `wiki search` devuelve `backend=wiki.search`, `items=[]` y un hint hacia `mi-lsp index --workspace <alias> --docs-only`.
- Si el catálogo Graph está stale (estado de runtime o generaciones de catálogo divergentes), `nav graph stats|status|validate` falla cerrado con `error.code=GPH_QUERY_GRAPH_INVALID`; las consultas de vecinos pueden degradar a un envelope explícito `mode=query_only` sin claims graph. En ambos casos el diagnóstico incluye `next_hint` hacia `mi-lsp index --workspace <alias> --docs-only` (grafo documental) o `mi-lsp index --workspace <alias>` (grafo completo). No se ejecuta un rebuild automático; `nav wiki search` y `nav wiki map` siguen disponibles.
- Si `--layer` contiene valores desconocidos, se ignoran y se devuelven warnings con los layers validos.
- `--repo` no pertenece a `nav wiki`; para compatibilidad, `nav ask|route|pack --repo <x>` lo acepta, lo ignora para docs y sugiere `nav wiki`.
- `nav wiki trace RS-*` devuelve identidad documental (`doc_id`, `layer=RS`, `stage=outcome`) y no rellena el campo legacy `rf`; `nav wiki trace --all` permanece RF-only.
- `nav wiki validate-harness` aplica el gate de gobernanza, lee el docgraph existente, abre los markdown gobernados y valida YAML frontmatter o fenced YAML con `harness_protocol: SDD-HARNESS-v1`.
- `nav wiki validate-harness` resuelve imports, evidencia y links Obsidian links Obsidian de ejemplo contra `DocRecord`, `doc_id`, exports y paths del workspace.
- `nav wiki validate-harness` debe usar todo el docgraph gobernado para resolver referencias, aunque la validacion este acotada a contratos `SDD-HARNESS-v1`; si un record agregado apunta al mismo ID que un contrato canonico, el agregado no debe generar falso `missing contract`.
- `--ids <lista>` combina DocRecord.DocID, Title y basename del path; DocRecord.DocID se llena preferentemente con identidad declarada en frontmatter/Harness/SDD y solo después con la compatibilidad legacy acotada (H1 ID-leading o basename exacto), nunca con el primer ID encontrado en el cuerpo.
- `nav wiki validate-source` aplica el gate de gobernanza, lee `doc_source_blocks`/`doc_source_records`, abre solo markdowns que declaran `SDD-WIKI-SOURCE-v1` y no bloquea el resto del corpus.
- `nav wiki validate-source --paths <path[,path...]>` limita el scope a paths existentes; `--ids <doc-id[,doc-id...]>` limita el scope a IDs documentales/source. Los filtros son aditivos al workspace y no convierten un documento dual sin `SDD-WIKI-SOURCE-v1` en source artifact; si el scope solo coincide con documentos no fuente, se reporta `scope=no_match`.
- Un scope sin coincidencias no es una lista vacía ambigua: devuelve `ok=true`, `wiki_source_verdict=BLOCKED`, `wiki_source_readiness=blocked`, `navigation_readiness=blocked` y `navigation_blockers` con `scope=no_match`, junto con un hint accionable.
- `nav wiki search`, `route`, `pack` y `trace` son superficies dirigidas y owner-aware. Una preview puede devolver `next_queries`/`next_hint`, pero no debe ocultar truncation, omisiones, stale index ni graph context no disponible.
- `wiki search` aplica admisión de relevancia local: las queries con forma de ID requieren identidad completa o referencia completa con límites (se aceptan enlaces Markdown y el sufijo `.md`); las queries naturales requieren FTS o solapamiento léxico real en `doc_id`, título, texto o path. `family`, `layer`, `owner_prefix` y `owner_hint` no bastan por sí solos y un no-hit devuelve `items=[]` con su diagnóstico/hint existente.
- No existe un campo de alias de búsqueda explícito en el esquema actual de `DocsReadProfile`; los alias de orientación Tier1 de `wiki route` no se inyectan en resultados literales de `wiki search`.
- `nav wiki search <id>` resuelve coincidencias exactas en `doc_source_blocks.doc_id`, `doc_source_blocks.block_id` y `doc_source_records.record_id` antes del ranking textual.
- `nav wiki search` debe exponer evidencia de linea cuando esta disponible: `line_start`/`line_end` en el item o rangos equivalentes dentro de `snippet/content`; los rangos deben apuntar al markdown canonico devuelto en `path`.
- `nav wiki trace <id>` puede devolver evidencia `wiki-source` para source IDs exactos aunque no sean `RS-*`, `RF-*` o `TP-*`.
- `nav wiki search|route|pack|trace` expone `lookup_status` de forma aditiva con `query`, `workspace`, `index_freshness`, `governance_sync`, `match_kind`, IDs exactos (`doc_id`, `block_id`, `record_id`), `path`, `layer`, `stage`, `rank_reason`, totales, razon y `next_hint` valido cuando la preview no muestra todo.
- `match_kind` distingue `canonical_indexed_id`, `alias_read_model_routing`, `mentions_content_fallback`, `content_fallback` y `true_absence`; no debe reportar ausencia si encontro identidad canonica pero la traza downstream queda incompleta.
- `TraceResult` puede agregar `confidence`, `confidence_reason` y `status_reason` de forma aditiva para diferenciar evidencia fuerte, fallback a disco, cobertura parcial y ausencia real; estos campos explican el veredicto pero no reemplazan `status`, `lookup_status` ni la evidencia `wiki-source`.
- La propiedad de `DocRecord.doc_id` se extrae con precedencia documental: YAML frontmatter o bloque YAML Harness inicial con `doc_id` (y `id` como fallback compatible), seguido del encabezado explícito `SDD-WIKI-SOURCE-v1`; solo los documentos legacy con un H1 que comienza por un ID conocido o con basename exactamente igual conservan esa compatibilidad. Imports, wikilinks, menciones, cuerpos y `doc_source_records.record_id` son referencias, no propietarios.
- Un `doc_id` no estándar o Unicode declarado es válido. Declaraciones de propietario contradictorias dejan el ID vacío, sin elegir silenciosamente una de ellas. Un Markdown enlazado sin declaración propia conserva identidad por `path/title/search_text`, no por el ID enlazado.
- La corrección de un índice ya publicado requiere el comando explícito soportado `mi-lsp index --workspace <alias> --docs-only`; la navegación no reindexa automáticamente. Esa operación vuelve a extraer el documento y publica el snapshot derivado (Docs, edges/mentions, source blocks/records y FTS) mediante reemplazo atómico. No se afirma una migración global ni se incluyen embeddings.

## Ejemplo documental sin embeddings

Para una pregunta sobre decisiones, fuentes o sustituciones, el flujo textual bounded es:

```text
mi-lsp nav wiki search "motivo de la decisión" --workspace <alias> --top 5 --format toon
mi-lsp nav wiki map --workspace <alias> --max-items 12 --format toon
mi-lsp nav wiki search "sustitución" --workspace <alias> --include-content --top 5 --format toon
mi-lsp nav multi-read .docs/wiki/04_RF/RF-WIKI-006.md:1-120 --workspace <alias> --format toon
```

La búsqueda es textual y el mapa es un catálogo de hubs; v1 no implementa embeddings ni `nav recall`. Si Graph está stale, se conserva este flujo y se repara de forma explícita con `mi-lsp index --workspace <alias> --docs-only` o con un index completo según la necesidad.

## Contract `wiki inventory`

```toon
block_id: ct-nav-wiki-inventory-contract
subcommand: "nav wiki inventory"
flag_all_workspaces: optional, default true
default_mode: light
light_item_shape:
  - alias
  - root
  - wiki_root
  - governance_blocked
  - docs_ready
  - doc_count
  - last_indexed_at
extended_flag: "--with-layer-counts"
extended_item_shape_adds:
  - layers: "{RS, FL, RF, TP, TECH, DB, CT} con counts"
envelope_extension: "ct-nav-wiki-envelope-all-workspaces"
semantics: |
  Sin argumentos requeridos, por defecto itera --all-workspaces.
  Light mode: listado simple de alias + metadata básica.
  Con --with-layer-counts: expande cada item con layer-specific doc counts.
  Úsease para auditoría de cobertura documental cross-workspace.
  Regla de prioridad: si --workspace o Context.Workspace es no vacío, el
  modo single-workspace gana siempre sobre el fan-out de --all-workspaces; la
  API no hace fan-out cuando hay un alias objetivo.
```

## Contract `wiki map`

```toon
doc_id: CT-NAV-WIKI
block_id: ct-nav-wiki-map-contract
kind: cli-contract
source_of_truth: this
subcommand: "nav wiki map"
workspace: required_single_alias
flag_all_workspaces: forbidden
backend: wiki.map
execution: direct_only
item_shape:
  - id: persona|proyectos|sistema|materia|custom
  - title: string
  - total_docs: int
  - docs: [{path, title}]
default_hub_rules:
  persona: wiki/00-09 markdown de primer nivel
  proyectos: wiki/10-19
  sistema: wiki/20-30 de primer nivel o 30-dashboard
  materia: bibliotecas/**
configuration:
  source: .docs/wiki/_mi-lsp/read-model.toml
  roots_default: [wiki/, bibliotecas/]
  roots_custom: adicionales, ordenadas y deduplicadas después de defaults
  custom_hubs: declaración ordenada; primer pattern coincidente gana; no mezcla defaults
  enabled_false: mapa e indexado automático deshabilitados
excluded: [wiki/31-workers, wiki/32-contratos, yaml, old, archive, deprecated, historico, legacy, .docs/wiki]
sources:
  preferred: SELECT path,title filtrado por roots configuradas
  fallback: walk bounded con ignore, cancelación y symlink/reparse guard
budgets:
  allocation: round-robin estable por conteo
  token_fit: búsqueda binaria
  truncation: Envelope.Truncated + files=total_returned + total_docs + total_returned + truncation_reason + next_hint
verify:
  - mi-lsp nav wiki map --workspace <alias> --format toon
  - go test ./internal/cli ./internal/service -count=1 -run 'TestNavWikiMapUsesDirectExecution|TestClassifyWikiMapHubDefaultsRemainLocked|TestTrimWikiMapHubs|TestWalkWikiMapDocs'
stop_if:
  - map_returns_full_bodies=true
evidence:
  - internal/service/wiki_map.go
  - .docs/wiki/04_RF/RF-WIKI-006.md
semantics: |
  Catálogo compacto para Momento/D-TEDI-017. No federar. No inventar grafo.
  El grafo de vecinos lo publica el Graph Kernel tras index --docs-only.
```

## Estado

implemented (search, route, pack, trace, validate-harness, validate-source, inventory, map, wiki-root)

## RF asociado

RF-QRY-016, RF-WIKI-001, RF-WIKI-002, RF-WIKI-003, RF-WIKI-004, RF-WIKI-005, RF-WIKI-006, RF-WIKI-007

## Grafo documental explícito

Las listas Markdown pueden declarar relaciones documentales sin frontmatter:

```markdown
- related: [[RF-DG-DECISION]]
- depends: [decisión](decision.md)
- supports: [[decision.md]]
- contradicts: [[alternativa.md]]
- supersedes: [[historia.md]]
```

`nav.neighbors` reutiliza la generación publicada y acepta como selector un `doc_id` o una ruta (`.docs/wiki/...`, `wiki/...` o `bibliotecas/...`). `--edge doc_related` (y las otras cuatro relaciones) filtra la consulta. Cada item conserva `relation`, `status`, `owner_path` (destino) y `doc_id` cuando existe; los targets faltantes o ambiguos aparecen como omisiones, nunca como inferencias. La consulta respeta `--depth`, `--limit`, `--token-budget` y `--cursor`; ciclos quedan acotados por el presupuesto. Si el catálogo está stale, el contrato conserva el fallback textual de `nav wiki search`.

Flujo recomendado: `nav wiki search "concepto"` → `nav neighbors RF-DG-DECISION --edge doc_related` → `nav multi-read .docs/wiki/04_RF/RF-GPH-003.md ...`.

Contrato de escritura para futuras skills: declarar `doc_id`, `summary` y `status`; escribir relaciones explícitas en listas; actualizar antes de crear otro documento con el mismo ID; `supersedes` preserva el historial y no lo oculta.
