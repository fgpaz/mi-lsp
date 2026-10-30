---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-004
kind: requirement
status: active
title: RF-WIKI-004 - Trazar evidencia de requisito a especificacion en todos los workspaces
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-WIKI-004 - Trazar evidencia de requisito a especificacion en todos los workspaces

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WIKI-004
- **Campo**: Título; **Valor**: Trazar evidencia de requisito a especificacion en todos los workspaces
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-WIKI-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Al menos un workspace registrado con `docs_ready=true`; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `~/.mi-lsp/registry.toml` accesible; **Tipo**: operativa; **Estado requerido**: obligatorio
- **Condición**: Flag `--all-workspaces` presente; **Tipo**: técnica; **Estado requerido**: obligatorio

### 4. Secuencia principal
1. La CLI recibe `nav wiki trace <task_id> --all-workspaces [--all|--summary]`.
2. El core itera sobre workspaces registrados con semaphore=4.
3. Para cada workspace, ejecuta la traza local (RS -> FL -> RF -> TP) desde el task_id.
4. Colecta caminos de dependencia documentaria sin fusionar entre wikis.
5. Cada item contiene `workspace: <alias>` explicitamente.
6. Con `--all`, incluye evidencia de código y citaciones; con `--summary`, solo caminos.
7. El envelope agrupa items por workspace pero NO fusiona trazas cross-wiki.
8. La CLI devuelve resultado en formato solicitado.

### 7. Casos especiales y variantes
- Envelope agrupa items por workspace pero **NO fusiona trazas cross-wiki** — cada workspace retorna su propia traza independiente.
- Si `--all` y `--summary` estan ambos presentes, `--summary` gana.
- Si un workspace tiene governance_blocked=true, incluir en `workspaces_failed[]`.
- Si `task_id` no se encuentra en ningún workspace, `items` esta vacio pero operación retorna `ok=true` con `stats.workspaces_with_trace=0`.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no fusionar trazas entre wikis distintos
  - no omitir campo `workspace` en items
  - no cambiar semántica de `--all` vs `--summary` sin actualizar CT
- Decisiones cerradas:
  - envelope agrupa pero no fusiona cross-workspace
  - semaphore=4 para fan-out
  - timeout 30s por workspace
- TODO explicit = 0
- Fuera de alcance:
  - sincronización de trazas cross-workspace
  - resolución automática de conflictos de traza

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `task_id`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: identificador de tarea (RF-*, TP-*, FL-*, etc.); **RN**: RF-WIKI-004
- **Campo**: `--all-workspaces`; **Tipo**: bool; **Req.**: si; **Origen**: CLI; **Validacion**: flag presente; **RN**: RF-WIKI-004
- **Campo**: `--all`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: modo verbose con evidencia completa; **RN**: RF-WIKI-004
- **Campo**: `--summary`; **Tipo**: bool; **Req.**: no; **Origen**: CLI; **Validacion**: modo compacto solo caminos; **RN**: RF-WIKI-004
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-WIKI-004

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array con trazas por workspace
- **Campo**: `workspace`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: workspace de origen
- **Campo**: `task_id`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: tarea raiz de la traza
- **Campo**: `paths`; **Tipo**: array; **Destino**: usuario/skill; **Efecto observable**: caminos de dependencia: RS->FL->RF->TP
- **Campo**: `evidence` (con --all); **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: código y citaciones para cada nodo
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: workspaces donde no se encontro tarea
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: `workspaces_queried`, `workspaces_with_trace`, `workspaces_failed`

### 8. Impacto en el modelo de datos
- `QueryEnvelope` con array de items; cada item contiene: workspace, task_id, paths[], evidence (opcional)
- Estadisticas globales: `workspaces_queried`, `workspaces_with_trace`, `workspaces_failed`

## Errores

### 6. Errores tipados
- **Código**: `WIKI_TRACE_NOT_FOUND`; **Causa**: task_id no existe; **Trigger**: tarea no mapea a documento; **Respuesta esperada**: incluir en `workspaces_failed[]` con razon
- **Código**: `WIKI_TRACE_INCOMPLETE`; **Causa**: camino incompleto (saltos sin ligadura); **Trigger**: documento falta imports/exports; **Respuesta esperada**: warning en item, continuar con otros nodos
- **Código**: `WIKI_EVIDENCE_TIMEOUT`; **Causa**: recoleccion de evidencia > 30s; **Trigger**: workspace muy grande con --all; **Respuesta esperada**: omitir evidencia, incluir solo caminos

## Criterios de aceptación (numerados)

1. Trazar requisito en multiples workspaces sin fusionar: Dado una tarea RF-WIKI-001 que existe en dos workspaces Cuando ejecuto "mi-lsp nav wiki trace RF-WIKI-001 --all-workspaces --format toon" Entonces retorna items para ambos workspaces Y cada item tiene su propio árbol de dependencias Y NO hay fusion de trazas entre workspaces
2. Incluir evidencia con --all: Dado una tarea con citaciones documentarias Cuando ejecuto "nav wiki trace <task> --all-workspaces --all" Entonces cada nodo incluye evidence.code_snippets y evidence.citations Y el payload es mayor que sin --all
3. Modo summary sin evidencia: Dado una tarea con árbol profundo Cuando ejecuto "nav wiki trace <task> --all-workspaces --summary" Entonces items contienen solo paths sin evidence Y payload es mínimo

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
- Positivo: `TP-WIKI / TC-WIKI-011`
- Positivo: `TP-WIKI / TC-WIKI-012`
- Negativo: `TP-WIKI / TC-WIKI-013`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WIKI-004
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-WIKI-01.md
exports:
- RF-WIKI-004
- RF-WIKI-004 - Trazar evidencia de requisito a especificacion en todos los workspaces
agent_must_read:
- .docs/wiki/05_requirements/RF-WIKI-004.md
- .docs/wiki/04_flows/FL-WIKI-01.md
- .docs/wiki/06_tests/TP-WIKI.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WIKI-004.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-WIKI.md, ejecutar el
  comando publicado: go test ./internal/service ./internal/cli ./internal/docgraph
  ./internal/indexer -count=1 -run ''TestClassifyWikiMapHubDefaultsRemainLocked|TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder|TestTrimWikiMapHubs|TestWalkWikiMapDocs|TestNavWikiMapUsesDirectExecution|TestKnowledgeWikiRoots|TestDocumentGraph''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- 'Detenerse y escalar si el cambio contradice el límite documentado: - Supuestos
  prohibidos:'
evidence:
- .docs/wiki/05_requirements/RF-WIKI-004.md
- .docs/wiki/06_tests/TP-WIKI.md
```
