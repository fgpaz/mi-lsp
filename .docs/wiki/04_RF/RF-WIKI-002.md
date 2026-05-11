# RF-WIKI-002 - Inventariar documentacion disponible en todos los workspaces

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WIKI-002"
kind: "requirement"
audience: "llm-first"
imports:
  - '[[FL-WIKI-01]]'
  - '[[00_gobierno_documental]]'
exports:
  - '[[TP-WIKI]]'
  - '[[CT-NAV-WIKI]]'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
  - .docs/wiki/04_RF/RF-WIKI-002.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-002.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-002.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WIKI-002.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WIKI-002 |
| Titulo | Inventariar documentacion disponible en todos los workspaces |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-WIKI-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Al menos un workspace registrado | funcional | obligatorio |
| `~/.mi-lsp/registry.toml` accesible | operativa | obligatorio |
| Flag `--all-workspaces` presente | tecnica | obligatorio |

## 3. Inputs

| Campo | Tipo | Req. | Origen | Validacion | RN |
|---|---|---|---|---|---|
| `--all-workspaces` | bool | si | CLI | flag presente | RF-WIKI-002 |
| `--with-layer-counts` | bool | no | CLI | opt-in para conteos por capa | RF-WIKI-002 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-WIKI-002 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `nav wiki inventory --all-workspaces [--with-layer-counts]`.
2. El core itera sobre todos los workspaces registrados en `~/.mi-lsp/registry.toml`.
3. Para cada workspace, carga su metadatos: `alias`, `root`, `wiki_root`, `governance_blocked`, `docs_ready`, `doc_count`, `last_indexed_at`.
4. Si `--with-layer-counts` esta presente, cuenta documentos por capa (01-09) desde `.mi-lsp/index.db` con semaphore=4.
5. Agrupa items por workspace en el envelope respetando orden de registro.
6. Sin flag, payload nominal es ~2-3KB (50 workspaces); con flag, ~5KB.
7. La CLI devuelve resultado en formato solicitado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `items` | lista | usuario/skill | array con inventario per workspace |
| `alias` | string | usuario/skill | identificador del workspace |
| `root` | path | usuario/skill | raiz absoluta del workspace |
| `wiki_root` | path | usuario/skill | raiz de documentacion (normalmente `.docs/wiki`) |
| `governance_blocked` | bool | usuario/skill | si el workspace esta bloqueado por governance |
| `docs_ready` | bool | usuario/skill | si la documentacion existe y puede indexarse |
| `doc_count` | numero | usuario/skill | total de documentos cuando docs_ready=true |
| `last_indexed_at` | timestamp | usuario/skill | cuando se hizo la ultima indexacion |
| `layers` (con flag) | objeto | usuario/skill | conteo por capa: `{RS: 0, FL: 2, RF: 15, TP: 10, TECH: 8, DB: 5, CT: 3}` |
| `warnings` | lista | usuario/skill | workspaces con metadatos faltantes |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WIKI_REGISTRY_EMPTY` | ningun workspace registrado | registry.toml vacio o inexistente | abortar con error explicito |
| `WIKI_INDEX_NOT_FOUND` | workspace sin .mi-lsp/index.db | docs_ready=false | incluir en items con doc_count=0, sin error |
| `WIKI_LAYER_COUNT_TIMEOUT` | layer count toma > 30s | workspace muy grande | warning en `warnings`, omitir conteos pero continuar |

## 7. Special Cases and Variants

- Si un workspace no tiene `.docs/wiki/`, incluir en resultado con `docs_ready=false` y `doc_count=0`.
- Si `governance_blocked=true`, marcar explicitamente pero incluir en resultado.
- Ignorar `last_indexed_at=null` (workspace nunca indexado); reportar con timestamp especial "never".
- Si `--with-layer-counts` y el conteo falla para un workspace, omitir ese objeto `layers` pero continuar con otros.
- Payload nominales: default ~2-3KB para 50 workspaces; con flag ~5KB.

## 8. Data Model Impact

- `QueryEnvelope` con array de `InventoryItem`: alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at
- Con flag: agregar campo `layers: {RS: int, FL: int, RF: int, TP: int, TECH: int, DB: int, CT: int}`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Listar inventario de todos los workspaces sin conteos
  Given al menos dos workspaces registrados
  When ejecuto "mi-lsp nav wiki inventory --all-workspaces --format toon"
  Then la respuesta contiene items para todos los workspaces
  And cada item tiene: alias, root, wiki_root, governance_blocked, docs_ready, doc_count, last_indexed_at
  And el payload es menor que 3KB

Scenario: Incluir conteos por capa cuando se solicita
  Given un workspace con documentacion completa
  When ejecuto "nav wiki inventory --all-workspaces --with-layer-counts --format toon"
  Then cada item contiene campo "layers" con conteos por capa
  And layers.RF es mayor que cero si el workspace tiene RF docs

Scenario: Manejar workspaces sin documentacion
  Given un workspace sin .docs/wiki/
  When ejecuto "nav wiki inventory --all-workspaces"
  Then aparece en items con docs_ready=false
  And doc_count=0
  And no error se genera
```

## 10. Test Traceability

- Positivo: `TP-WIKI / TC-WIKI-005`
- Positivo: `TP-WIKI / TC-WIKI-006`
- Negativo: `TP-WIKI / TC-WIKI-007`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que todos los workspaces tienen index.db
  - no omitir workspaces con docs_ready=false
  - no hacer conteos por capa sin flag explicito
- Decisiones cerradas:
  - `nav wiki inventory` es comando nuevo bajo subgrupo wiki
  - Default: metadatos basicos sin conteos
  - Con flag: agregar conteos por capa
- TODO explicit = 0
- Fuera de alcance:
  - indexacion asincona (solo metadatos existentes)
  - sincronizacion de indices entre workspaces
