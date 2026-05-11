# RF-WIKI-004 - Trazar evidencia de requisito a especificacion en todos los workspaces

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WIKI-004"
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
  - .docs/wiki/04_RF/RF-WIKI-004.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-004.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-004.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WIKI-004.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WIKI-004 |
| Titulo | Trazar evidencia de requisito a especificacion en todos los workspaces |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | media |
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
| `task_id` | string | si | CLI | identificador de tarea (RF-*, TP-*, FL-*, etc.) | RF-WIKI-004 |
| `--all-workspaces` | bool | si | CLI | flag presente | RF-WIKI-004 |
| `--all` | bool | no | CLI | modo verbose con evidencia completa | RF-WIKI-004 |
| `--summary` | bool | no | CLI | modo compacto solo caminos | RF-WIKI-004 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-WIKI-004 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `nav wiki trace <task_id> --all-workspaces [--all|--summary]`.
2. El core itera sobre workspaces registrados con semaphore=4.
3. Para cada workspace, ejecuta la traza local (RS -> FL -> RF -> TP) desde el task_id.
4. Colecta caminos de dependencia documentaria sin fusionar entre wikis.
5. Cada item contiene `workspace: <alias>` explicitamente.
6. Con `--all`, incluye evidencia de codigo y citaciones; con `--summary`, solo caminos.
7. El envelope agrupa items por workspace pero NO fusiona trazas cross-wiki.
8. La CLI devuelve resultado en formato solicitado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `items` | lista | usuario/skill | array con trazas por workspace |
| `workspace` | string | usuario/skill | workspace de origen |
| `task_id` | string | usuario/skill | tarea raiz de la traza |
| `paths` | array | usuario/skill | caminos de dependencia: RS->FL->RF->TP |
| `evidence` (con --all) | objeto | usuario/skill | codigo y citaciones para cada nodo |
| `warnings` | lista | usuario/skill | workspaces donde no se encontro tarea |
| `stats` | objeto | usuario/skill | `workspaces_queried`, `workspaces_with_trace`, `workspaces_failed` |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WIKI_TRACE_NOT_FOUND` | task_id no existe | tarea no mapea a documento | incluir en `workspaces_failed[]` con razon |
| `WIKI_TRACE_INCOMPLETE` | camino incompleto (saltos sin ligadura) | documento falta imports/exports | warning en item, continuar con otros nodos |
| `WIKI_EVIDENCE_TIMEOUT` | recoleccion de evidencia > 30s | workspace muy grande con --all | omitir evidencia, incluir solo caminos |

## 7. Special Cases and Variants

- Envelope agrupa items por workspace pero **NO fusiona trazas cross-wiki** — cada workspace retorna su propia traza independiente.
- Si `--all` y `--summary` estan ambos presentes, `--summary` gana.
- Si un workspace tiene governance_blocked=true, incluir en `workspaces_failed[]`.
- Si `task_id` no se encuentra en ningun workspace, `items` esta vacio pero operacion retorna `ok=true` con `stats.workspaces_with_trace=0`.

## 8. Data Model Impact

- `QueryEnvelope` con array de items; cada item contiene: workspace, task_id, paths[], evidence (opcional)
- Estadisticas globales: `workspaces_queried`, `workspaces_with_trace`, `workspaces_failed`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Trazar requisito en multiples workspaces sin fusionar
  Given una tarea RF-WIKI-001 que existe en dos workspaces
  When ejecuto "mi-lsp nav wiki trace RF-WIKI-001 --all-workspaces --format toon"
  Then retorna items para ambos workspaces
  And cada item tiene su propio arbol de dependencias
  And NO hay fusion de trazas entre workspaces

Scenario: Incluir evidencia con --all
  Given una tarea con citaciones documentarias
  When ejecuto "nav wiki trace <task> --all-workspaces --all"
  Then cada nodo incluye evidence.code_snippets y evidence.citations
  And el payload es mayor que sin --all

Scenario: Modo summary sin evidencia
  Given una tarea con arbol profundo
  When ejecuto "nav wiki trace <task> --all-workspaces --summary"
  Then items contienen solo paths sin evidence
  And payload es minimo
```

## 10. Test Traceability

- Positivo: `TP-WIKI / TC-WIKI-011`
- Positivo: `TP-WIKI / TC-WIKI-012`
- Negativo: `TP-WIKI / TC-WIKI-013`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no fusionar trazas entre wikis distintos
  - no omitir campo `workspace` en items
  - no cambiar semantica de `--all` vs `--summary` sin actualizar CT
- Decisiones cerradas:
  - envelope agrupa pero no fusiona cross-workspace
  - semaphore=4 para fan-out
  - timeout 30s por workspace
- TODO explicit = 0
- Fuera de alcance:
  - sincronizacion de trazas cross-workspace
  - resolucion automatica de conflictos de traza
