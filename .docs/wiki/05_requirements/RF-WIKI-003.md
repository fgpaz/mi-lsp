---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-003
kind: requirement
status: active
title: RF-WIKI-003 - Resolver ruta canonica de tarea en todos los workspaces
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-WIKI-003 - Resolver ruta canónica de tarea en todos los workspaces

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WIKI-003
- **Campo**: Título; **Valor**: Resolver ruta canónica de tarea en todos los workspaces
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
1. La CLI recibe `nav wiki route <task_id|brief> --all-workspaces`.
2. El core itera sobre workspaces registrados con semaphore=4.
3. Para cada workspace, ejecuta el resolutor local de ruta canónica que mapea task_id a documento.
4. Resolutor canoico consulta governance para determinar documento mínimo para la tarea.
5. Si el task_id existe en multiples workspaces, retorna N items cada uno anotado con `workspace: <alias>`.
6. Si el task_id no se encuentra, entra a warnings con razon.
7. El truncador aplica presupuesto y devuelve envelope estable.
8. La CLI devuelve resultado en formato solicitado.

### 7. Casos especiales y variantes
- Si hay candidatos en N wikis distintos, devuelve N items con `workspace` distinto.
- Si un workspace no tiene governance completa, incluir en `workspaces_failed[]` con razon.
- Si `task_id` es un pattern parcial (e.g., `RF-WIKI`), resoltor fuzzy puede retornar multiples candidatos.
- Si documento se encuentra pero es parte de contenido composite, `doc_paths` contiene el índice y el archivo componente.

### 11. Decisiones cerradas
- Supuestos prohibidos:
  - no asumir que task_id es único globalmente
  - no omitir workspace en items cuando hay multiples candidatos
  - no cambiar algoritmo de resolución sin actualizar CT y TECH
- Decisiones cerradas:
  - resolutor usa governance como source of truth
  - semaphore=4 para fan-out
  - timeout 30s por workspace
- TODO explicit = 0
- Fuera de alcance:
  - cross-wiki linking (cada workspace resuelve independientemente)
  - machine-learning ranking (solo priority-based)

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 3. Entradas
- **Campo**: `task_id` o `brief`; **Tipo**: string; **Req.**: si; **Origen**: CLI; **Validacion**: task ID (RF-*, FL-*, etc.) o descripción breve; **RN**: RF-WIKI-003
- **Campo**: `--all-workspaces`; **Tipo**: bool; **Req.**: si; **Origen**: CLI; **Validacion**: flag presente; **RN**: RF-WIKI-003
- **Campo**: `format`; **Tipo**: enum; **Req.**: no; **Origen**: CLI; **Validacion**: `compact`, `json`, `text`, `toon` o `yaml`; default `compact`; **RN**: RF-WIKI-003

### 5. Salidas
- **Campo**: `ok`; **Tipo**: bool; **Destino**: usuario/skill; **Efecto observable**: estado de la operación
- **Campo**: `items`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: array con rutas encontradas, una por workspace
- **Campo**: `workspace`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: workspace de origen
- **Campo**: `task_id`; **Tipo**: string; **Destino**: usuario/skill; **Efecto observable**: tarea identificada
- **Campo**: `doc_paths`; **Tipo**: array; **Destino**: usuario/skill; **Efecto observable**: lista de documentos canónicos por prioridad
- **Campo**: `primary_doc`; **Tipo**: path; **Destino**: usuario/skill; **Efecto observable**: documento recomendado (primero en lista)
- **Campo**: `warnings`; **Tipo**: lista; **Destino**: usuario/skill; **Efecto observable**: workspaces donde no se resolvio la tarea
- **Campo**: `stats`; **Tipo**: objeto; **Destino**: usuario/skill; **Efecto observable**: `workspaces_queried`, `workspaces_with_match`, `workspaces_failed`

### 8. Impacto en el modelo de datos
- `QueryEnvelope` con array de items; cada item contiene: workspace, task_id, doc_paths[], primary_doc
- Estadisticas globales: `workspaces_queried`, `workspaces_with_match`, `workspaces_failed`

## Errores

### 6. Errores tipados
- **Código**: `WIKI_ROUTE_NOT_FOUND`; **Causa**: tarea no mapea a documento; **Trigger**: task_id inválido o inexistente; **Respuesta esperada**: incluir en `workspaces_failed[]` con razon
- **Código**: `WIKI_GOVERNANCE_UNRESOLVED`; **Causa**: governance no permite determinismo; **Trigger**: governance_blocked=true; **Respuesta esperada**: error explícito, abortar
- **Código**: `WIKI_ROUTE_AMBIGUOUS`; **Causa**: task_id mapea a multiples documentos; **Trigger**: tarea en multiples capas; **Respuesta esperada**: retornar array de candidatos ordenado por prioridad

## Criterios de aceptación (numerados)

1. Resolver ruta en un workspace cuando existe: Dado una tarea RF-WIKI-001 registrada en dos workspaces Cuando ejecuto "mi-lsp nav wiki route RF-WIKI-001 --all-workspaces" Entonces retorna items para ambos workspaces Y cada item tiene primary_doc distinto (según workspace-local paths) Y stats.workspaces_with_match=2
2. Reportar fallo cuando tarea no existe: Dado una tarea hipotetica que no existe Cuando ejecuto "nav wiki route FAKE-999 --all-workspaces" Entonces todos los items entran a workspaces_failed[] con razon Y stats.workspaces_with_match=0
3. Manejar multiples candidatos por workspace: Dado una tarea que existe en capas distintas (e.g., FL y RF con mismo nombre) Cuando ejecuto "nav wiki route <task> --all-workspaces" Entonces doc_paths retorna candidatos en orden de prioridad Y primary_doc es el primero

### 9. Escenarios de aceptación (Gherkin)
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

### 10. Trazabilidad de pruebas
- Positivo: `TP-WIKI / TC-WIKI-008`
- Positivo: `TP-WIKI / TC-WIKI-009`
- Negativo: `TP-WIKI / TC-WIKI-010`

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WIKI-003
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-WIKI-01.md
exports:
- RF-WIKI-003
- RF-WIKI-003 - Resolver ruta canonica de tarea en todos los workspaces
agent_must_read:
- .docs/wiki/05_requirements/RF-WIKI-003.md
- .docs/wiki/04_flows/FL-WIKI-01.md
- .docs/wiki/06_tests/TP-WIKI.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WIKI-003.md
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
- .docs/wiki/05_requirements/RF-WIKI-003.md
- .docs/wiki/06_tests/TP-WIKI.md
```
