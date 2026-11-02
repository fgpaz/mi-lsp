---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-012
kind: requirement
status: active
title: RF-QRY-012 - Construir un reading pack canonico docs-first para una tarea
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-012 - Construir un reading pack canónico docs-first para una tarea

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-012
- **Campo**: Título; **Valor**: Construir un reading pack canónico docs-first para una tarea
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Índice repo-local disponible o construible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Corpus documental canónico accesible; **Tipo**: funcional; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. La CLI recibe `mi-lsp nav pack <task>`.
2. El core resuelve el workspace y carga el `read-model` del proyecto o el default embebido.
3. El core invoca `resolveCanonicalRoute` (RF-QRY-015) para clasificar la tarea y obtener el anchor canónico; si el payload incluye `--rf`, `--fl` o `--doc`, ese anchor explícito sobreescribe el resultado del route core.
4. El core construye un reading pack ordenado de lo más global a lo más especifico según la ladder canónica, reutilizando el mismo scorer owner-aware compartido por `nav route` y `nav ask`.
5. En modo preview devuelve paths, stages, razones y targets sugeridos.
6. En modo `--full` expande slices legibles de los documentos seleccionados sin cambiar el envelope base.
7. Si la wiki existe pero el índice documental esta vacio o stale, devuelve warnings accionables para reindexar en vez de degradar silenciosamente a `README`.

### 5. Casos especiales y variantes
- El input principal es tarea libre; el route core (`resolveCanonicalRoute`, RF-QRY-015) determina el anchor canónico por defecto.
- `--doc <path>`, `--rf <id>` y `--fl <id>` endurecen la selección con un anchor explícito que sobreescribe el route core (precedencia: `--doc` > `--rf` > `--fl` > route core).
- `nav pack` pertenece a la superficie AXI-default y usa preview-first por default; `--full` expande slices sin cambiar la semántica del comando.
- La ladder base prioriza documentos raiz antes de bajar a `FL`, `RF`, `TECH-*`, `CT-*` o capas UX según la familia.
- Si existe `.docs/wiki/_mi-lsp/read-model.toml`, el bloque `reading_pack` puede ajustar `max_docs` y el orden de stages por familia.
- Cuando existen `owner_hints` proyectados desde `00`, esos hints pueden empujar el anchor canónico sin reemplazar un override explicito.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `DocRecord`
- `DocEdge`
- `DocsReadProfile`
- `PackResult`
- `PackDoc`
- `PackTarget`
- `QueryEnvelope`

## Errores

### 4. Errores tipados
- **Código**: `QRY_PACK_TASK_REQUIRED`; **Causa**: falta tarea; **Trigger**: argumento vacio; **Respuesta esperada**: abortar con error explícito
- **Código**: `QRY_PACK_WORKSPACE_NOT_FOUND`; **Causa**: workspace inválido; **Trigger**: alias/path no resoluble; **Respuesta esperada**: abortar con `ok=false`
- **Código**: `QRY_PACK_DOC_INDEX_UNAVAILABLE`; **Causa**: store repo-local no accesible; **Trigger**: `index.db` no abrible; **Respuesta esperada**: abortar con error explícito

## Criterios de aceptación (numerados)

1. La CLI recibe `mi-lsp nav pack <task>`.
2. El core resuelve el workspace y carga el `read-model` del proyecto o el default embebido.
3. El core invoca `resolveCanonicalRoute` (RF-QRY-015) para clasificar la tarea y obtener el anchor canónico; si el payload incluye `--rf`, `--fl` o `--doc`, ese anchor explícito sobreescribe el resultado del route core.
4. El core construye un reading pack ordenado de lo más global a lo más especifico según la ladder canónica, reutilizando el mismo scorer owner-aware compartido por `nav route` y `nav ask`.
5. En modo preview devuelve paths, stages, razones y targets sugeridos.
6. En modo `--full` expande slices legibles de los documentos seleccionados sin cambiar el envelope base.
7. Si la wiki existe pero el índice documental esta vacio o stale, devuelve warnings accionables para reindexar en vez de degradar silenciosamente a `README`.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-QRY-012
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-QRY-01.md
exports:
- RF-QRY-012
- RF-QRY-012 - Construir un reading pack canonico docs-first para una tarea
agent_must_read:
- .docs/wiki/05_requirements/RF-QRY-012.md
- .docs/wiki/04_flows/FL-QRY-01.md
- .docs/wiki/06_tests/TP-QRY.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-QRY-012.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/service/app.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-QRY.md, ejecutar el
  comando publicado: go test ./internal/model ./internal/service ./internal/cli ./internal/indexer
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-QRY-012.
evidence:
- .docs/wiki/05_requirements/RF-QRY-012.md
- .docs/wiki/06_tests/TP-QRY.md
- .docs/wiki/06_tests/TP-QRY-PREPARATION.md
```
