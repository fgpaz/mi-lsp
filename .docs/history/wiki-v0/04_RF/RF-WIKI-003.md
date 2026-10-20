# RF-WIKI-003 - Resolver ruta canonica de tarea en todos los workspaces

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WIKI-003"
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
  - .docs/wiki/04_RF/RF-WIKI-003.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-003.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-003.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WIKI-003.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WIKI-003 |
| Titulo | Resolver ruta canonica de tarea en todos los workspaces |
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
| `task_id` o `brief` | string | si | CLI | task ID (RF-*, FL-*, etc.) o descripcion breve | RF-WIKI-003 |
| `--all-workspaces` | bool | si | CLI | flag presente | RF-WIKI-003 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-WIKI-003 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `nav wiki route <task_id|brief> --all-workspaces`.
2. El core itera sobre workspaces registrados con semaphore=4.
3. Para cada workspace, ejecuta el resolutor local de ruta canonica que mapea task_id a documento.
4. Resolutor canoico consulta governance para determinar documento minimo para la tarea.
5. Si el task_id existe en multiples workspaces, retorna N items cada uno anotado con `workspace: <alias>`.
6. Si el task_id no se encuentra, entra a warnings con razon.
7. El truncador aplica presupuesto y devuelve envelope estable.
8. La CLI devuelve resultado en formato solicitado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `items` | lista | usuario/skill | array con rutas encontradas, una por workspace |
| `workspace` | string | usuario/skill | workspace de origen |
| `task_id` | string | usuario/skill | tarea identificada |
| `doc_paths` | array | usuario/skill | lista de documentos canonicos por prioridad |
| `primary_doc` | path | usuario/skill | documento recomendado (primero en lista) |
| `warnings` | lista | usuario/skill | workspaces donde no se resolvio la tarea |
| `stats` | objeto | usuario/skill | `workspaces_queried`, `workspaces_with_match`, `workspaces_failed` |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WIKI_ROUTE_NOT_FOUND` | tarea no mapea a documento | task_id invalido o inexistente | incluir en `workspaces_failed[]` con razon |
| `WIKI_GOVERNANCE_UNRESOLVED` | governance no permite determinismo | governance_blocked=true | error explicito, abortar |
| `WIKI_ROUTE_AMBIGUOUS` | task_id mapea a multiples documentos | tarea en multiples capas | retornar array de candidatos ordenado por prioridad |

## 7. Special Cases and Variants

- Si hay candidatos en N wikis distintos, devuelve N items con `workspace` distinto.
- Si un workspace no tiene governance completa, incluir en `workspaces_failed[]` con razon.
- Si `task_id` es un pattern parcial (e.g., `RF-WIKI`), resoltor fuzzy puede retornar multiples candidatos.
- Si documento se encuentra pero es parte de contenido composite, `doc_paths` contiene el indice y el archivo componente.

## 8. Data Model Impact

- `QueryEnvelope` con array de items; cada item contiene: workspace, task_id, doc_paths[], primary_doc
- Estadisticas globales: `workspaces_queried`, `workspaces_with_match`, `workspaces_failed`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Resolver ruta en un workspace cuando existe
  Given una tarea RF-WIKI-001 registrada en dos workspaces
  When ejecuto "mi-lsp nav wiki route RF-WIKI-001 --all-workspaces"
  Then retorna items para ambos workspaces
  And cada item tiene primary_doc distinto (segun workspace-local paths)
  And stats.workspaces_with_match=2

Scenario: Reportar fallo cuando tarea no existe
  Given una tarea hipotetica que no existe
  When ejecuto "nav wiki route FAKE-999 --all-workspaces"
  Then todos los items entran a workspaces_failed[] con razon
  And stats.workspaces_with_match=0

Scenario: Manejar multiples candidatos por workspace
  Given una tarea que existe en capas distintas (e.g., FL y RF con mismo nombre)
  When ejecuto "nav wiki route <task> --all-workspaces"
  Then doc_paths retorna candidatos en orden de prioridad
  And primary_doc es el primero
```

## 10. Test Traceability

- Positivo: `TP-WIKI / TC-WIKI-008`
- Positivo: `TP-WIKI / TC-WIKI-009`
- Negativo: `TP-WIKI / TC-WIKI-010`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no asumir que task_id es unico globalmente
  - no omitir workspace en items cuando hay multiples candidatos
  - no cambiar algoritmo de resolucion sin actualizar CT y TECH
- Decisiones cerradas:
  - resolutor usa governance como source of truth
  - semaphore=4 para fan-out
  - timeout 30s por workspace
- TODO explicit = 0
- Fuera de alcance:
  - cross-wiki linking (cada workspace resuelve independientemente)
  - machine-learning ranking (solo priority-based)
