# RF-WIKI-005 - Empaquetar documentacion canon para tarea en todos los workspaces

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WIKI-005"
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
  - .docs/wiki/04_RF/RF-WIKI-005.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WIKI-005.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-005.md --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WIKI-005.md
  - .docs/wiki/04_RF.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WIKI-005 |
| Titulo | Empaquetar documentacion canon para tarea en todos los workspaces |
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
| `task_id` o `scope` | string | si | CLI | identificador de tarea o scope (RF-*, TP-*, etc.) | RF-WIKI-005 |
| `--all-workspaces` | bool | si | CLI | flag presente | RF-WIKI-005 |
| `--rf`, `--fl`, `--doc` | enum | no | CLI | filtros opcionales de scope | RF-WIKI-005 |
| `format` | enum | no | CLI | `compact`, `json`, `text`, `toon` o `yaml`; default `compact` | RF-WIKI-005 |

## 4. Process Steps (Happy Path)

1. La CLI recibe `nav wiki pack <task_id|scope> --all-workspaces [--rf|--fl|--doc] [--format X]`.
2. El core itera sobre workspaces registrados con semaphore=4.
3. Para cada workspace, ejecuta el packager local que colecta documentos canonicos para la tarea.
4. Packager respeta filtros opcionales (`--rf`, `--fl`, `--doc`) si estan presentes.
5. Retorna N mini-packs en el envelope, uno por workspace.
6. **Importante**: `pack --all-workspaces` devuelve N items (packs separados), NO un super-pack mergeado.
7. Hermes (consumidor) decide si los une, mantiene separados, o filtra.
8. La CLI devuelve resultado en formato solicitado.

## 5. Outputs

| Campo | Tipo | Destino | Efecto observable |
|---|---|---|---|
| `ok` | bool | usuario/skill | estado de la operacion |
| `items` | lista | usuario/skill | array con N mini-packs (uno por workspace) |
| `workspace` | string | usuario/skill | workspace de origen del pack |
| `task_id` | string | usuario/skill | tarea identificada |
| `pack` | objeto | usuario/skill | documentos coleccionados: {root_docs[], related_docs[], evidence_paths[]} |
| `pack_size_bytes` | numero | usuario/skill | tamaño aproximado del mini-pack |
| `warnings` | lista | usuario/skill | workspaces donde pack estuvo vacio o incompleto |
| `stats` | objeto | usuario/skill | `workspaces_queried`, `workspaces_with_pack`, `workspaces_failed`, `total_items` |

## 6. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WIKI_PACK_NOT_FOUND` | task_id no resolvio a documentos | tarea inexistente | incluir en `workspaces_failed[]` con razon |
| `WIKI_PACK_EMPTY` | packager colecciono cero documentos | scope muy restrictivo | warning en item, pack vacio pero continuar |
| `WIKI_PACK_TIMEOUT` | empaquetado > 30s | workspace muy grande | timeout error, workspace en `workspaces_failed[]` |

## 7. Special Cases and Variants

- Devuelve N items en el envelope, **NO un super-pack**, cada uno es independiente.
- Con `--rf`, `--fl`, `--doc`, packager filtra documentos dentro de cada workspace (no globaliza el filtro).
- Si dos workspaces tienen el mismo task_id, ambos retornan sus propios packs en el array `items[]`.
- Si un pack esta vacio, continuar con otros y reportar warning, no error fatal.
- Mini-pack payload nominal 5-50KB por workspace, segun cantidad de documentos.

## 8. Data Model Impact

- `QueryEnvelope` con array de items; cada item contiene: workspace, task_id, pack{root_docs[], related_docs[], evidence_paths[]}, pack_size_bytes
- Estadisticas globales: `workspaces_queried`, `workspaces_with_pack`, `workspaces_failed`, `total_items`

## 9. Expanded Acceptance Criteria (Gherkin)

```gherkin
Scenario: Empaquetar documentacion de N workspaces por separado
  Given una tarea RF-WIKI-001 en dos workspaces
  When ejecuto "mi-lsp nav wiki pack RF-WIKI-001 --all-workspaces --format toon"
  Then retorna items con dos packs distintos
  And cada pack contiene documentos del workspace respectivo
  And NO hay fusion ni super-pack

Scenario: Respetar filtros por scope
  Given una tarea compleja con multiples capas
  When ejecuto "nav wiki pack <task> --all-workspaces --rf"
  Then cada pack contiene solo documentos RF
  And root_docs[] incluye solo RF-*
  And related_docs[] puede incluir FL si hay imports

Scenario: Reportar pack vacio sin error
  Given una tarea con scope muy restrictivo
  When ejecuto "nav wiki pack <task> --all-workspaces --doc"
  Then si algun workspace tiene pack vacio, entra a warnings
  And stats.workspaces_with_pack refleja cuantos tuvieron contenido
```

## 10. Test Traceability

- Positivo: `TP-WIKI / TC-WIKI-014`
- Positivo: `TP-WIKI / TC-WIKI-015`
- Negativo: `TP-WIKI / TC-WIKI-016`

## 11. No Ambiguities Left

- Supuestos prohibidos:
  - no fusionar packs entre workspaces
  - no omitir campo `workspace` en items
  - no cambiar semantica de filtros sin actualizar CT
- Decisiones cerradas:
  - `pack --all-workspaces` retorna N mini-packs, nunca un super-pack
  - CT-NAV-WIKI documenta esto explicitamente
  - semaphore=4 para fan-out
  - timeout 30s por workspace
- TODO explicit = 0
- Fuera de alcance:
  - fusion de packs (responsabilidad de consumidor Hermes)
  - sincronizacion de contenido entre workspaces
