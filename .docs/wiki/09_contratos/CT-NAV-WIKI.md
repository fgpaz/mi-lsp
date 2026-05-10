# CT-NAV-WIKI

```yaml
harness_protocol: SDD-HARNESS-v1
id: "CT-NAV-WIKI"
kind: "wiki-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-NAV-WIKI]]'
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
mi-lsp nav wiki search <query> --workspace <alias> [--layer RS,RF,FL,TP,CT,TECH,DB] [--top N] [--offset N] [--include-content] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki route <task> --workspace <alias> [--full] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki pack <task> --workspace <alias> [--rf RF-*] [--fl FL-*] [--doc <path>] [--full] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki trace <DOC-ID|--all> --workspace <alias> [--summary] [--format compact|json|text|toon|yaml]
mi-lsp nav wiki validate-harness --workspace <alias> [--format compact|json|text|toon|yaml]
mi-lsp nav wiki validate-source --workspace <alias> [--format compact|json|text|toon|yaml]
```

## Semantica

`nav wiki` es la puerta documental explicita para agentes. `wiki search` usa el docgraph repo-local y el scorer owner-aware para devolver candidatos wiki, mientras `wiki route`, `wiki pack` y `wiki trace` reutilizan la semantica y el shape de `nav route`, `nav pack` y `nav trace`. `wiki validate-harness` compila readiness de contratos `SDD-HARNESS-v1` sobre los docs gobernados. `wiki validate-source` compila readiness de artefactos que declaran `wiki_source_protocol: SDD-WIKI-SOURCE-v1`; los docs no migrados no son bloqueantes. `wiki search` acepta `RS` como layer outcome y `wiki trace` acepta `RS-*`, `RF-*`, `TP-*` y source IDs exactos; `--all` sigue recorriendo el set RF canonico, y cuando necesita fallback a disco debe priorizar las rutas gobernadas por `00`/`read-model` antes de caer a layouts legacy.

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

## Diagnosticos

- Si `governance_blocked=true`, `wiki search` devuelve `backend=governance` y no ejecuta ranking documental.
- Si `doc_records` esta vacio, `wiki search` devuelve `backend=wiki.search`, `items=[]` y un hint hacia `mi-lsp index --workspace <alias> --docs-only`.
- Si `--layer` contiene valores desconocidos, se ignoran y se devuelven warnings con los layers validos.
- `--repo` no pertenece a `nav wiki`; para compatibilidad, `nav ask|route|pack --repo <x>` lo acepta, lo ignora para docs y sugiere `nav wiki`.
- `nav wiki trace RS-*` devuelve identidad documental (`doc_id`, `layer=RS`, `stage=outcome`) y no rellena el campo legacy `rf`; `nav wiki trace --all` permanece RF-only.
- `nav wiki validate-harness` aplica el gate de gobernanza, lee el docgraph existente, abre los markdown gobernados y valida YAML frontmatter o fenced YAML con `harness_protocol: SDD-HARNESS-v1`.
- `nav wiki validate-harness` resuelve imports, evidencia y links Obsidian links Obsidian de ejemplo contra `DocRecord`, `doc_id`, exports y paths del workspace.
- `nav wiki validate-harness` debe usar todo el docgraph gobernado para resolver referencias, aunque la validacion este acotada a contratos `SDD-HARNESS-v1`; si un record agregado apunta al mismo ID que un contrato canonico, el agregado no debe generar falso `missing contract`.
- `nav wiki validate-source` aplica el gate de gobernanza, lee `doc_source_blocks`/`doc_source_records`, abre solo markdowns que declaran `SDD-WIKI-SOURCE-v1` y no bloquea el resto del corpus.
- `nav wiki search <id>` resuelve coincidencias exactas en `doc_source_blocks.doc_id`, `doc_source_blocks.block_id` y `doc_source_records.record_id` antes del ranking textual.
- `nav wiki trace <id>` puede devolver evidencia `wiki-source` para source IDs exactos aunque no sean `RS-*`, `RF-*` o `TP-*`.
- `nav wiki search|route|pack|trace` expone `lookup_status` de forma aditiva con `query`, `workspace`, `index_freshness`, `governance_sync`, `match_kind`, IDs exactos (`doc_id`, `block_id`, `record_id`), `path`, `layer`, `stage`, `rank_reason`, totales, razon y `next_hint` valido cuando la preview no muestra todo.
- `match_kind` distingue `canonical_indexed_id`, `alias_read_model_routing`, `mentions_content_fallback`, `content_fallback` y `true_absence`; no debe reportar ausencia si encontro identidad canonica pero la traza downstream queda incompleta.

## Estado

implemented

## RF asociado

RF-QRY-016
