---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-005
kind: requirement
status: active
title: RF-WKS-005 - Aplicar gate de gobernanza al inicio de toda tarea
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/docgraph/governance.go
    role: behavior
  - path: internal/service/app.go
    role: behavior
  - path: internal/service/governance.go
    role: behavior
  - path: internal/service/workspace_ops.go
    role: behavior
  - path: internal/workspace/registry.go
    role: behavior
---

## Propósito

RF-WKS-005 - Aplicar gate de gobernanza al inicio de toda tarea

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-005
- **Campo**: Título; **Valor**: Aplicar gate de gobernanza al inicio de toda tarea
- **Campo**: Actores; **Valor**: Desarrollador, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: `00_governance.md` presente; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Proyección ejecutable sincronizada; **Tipo**: técnica; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. Toda tarea consulta el estado de gobernanza del workspace antes de continuar.
2. `workspace status` expone `workspace_root`, `workspace_source`, perfil, sync, index sync y estado bloqueado; cuando existe snapshot repo-local de reentrada, puede exponer `memory_pointer` en preview y `memory` completo bajo expansion.
3. Bajo `--full`, si la memoria esta stale, `auto_sync` esta habilitado y la gobernanza no esta bloqueada, puede refrescar docs/memoria antes de responder. `--no-auto-sync` conserva el diagnóstico read-only y deja visible el stale.
3. Si la gobernanza es valida, el workflow normal puede seguir.
4. Si la gobernanza es inválida, el repo entra en `blocked mode`.
5. En `blocked mode` solo quedan permitidos diagnóstico y reparación de gobernanza.

### 5. Casos especiales y variantes
- El gate corre al inicio de toda tarea spec-driven, no solo de tareas documentales.
- `workspace status` y `nav governance` siguen disponibles aun en `blocked mode`.
- El policy layer (`AGENTS.md`, `CLAUDE.md`, skills) debe respetar este gate aunque el usuario no lo mencione.
- Si `--workspace <alias>` explícito apunta a un root distinto del workspace registrado que contiene el `caller_cwd`, el CLI humano (`manual-cli`) conserva el alias explícito y el warning muestra ambos roots. Un caller harness, incluido un nombre normalizado (`root`, `builtin_child`, `mi-lsp-mcp`, `pi-chief`, `grok-measure-leaf`), puede LEER (`nav.*`, `workspace.status`, `info` y demás consultas) con el mismo warning que nombra ambos roots, sin `--allow-cross-workspace`. Las operaciones que escriben estado del workspace seleccionado (`index.run|start|cancel|run-job`, `workspace.link`, `nav.prepare`, `prepare.create|refresh`) siguen recibiendo `workspace_cross_workspace_refused` salvo `--allow-cross-workspace`; ese override es manual, se marca con warning y cada uso queda registrado en `~/.mi-lsp/overrides.log` (JSONL con `ts`, `operation`, `client`, `selector`, `selected_root`, `caller_cwd`). Una lectura cross-workspace de un harness no dispara efectos de escritura en el workspace ajeno: si el catálogo no está listo, el reindex en segundo plano (RF-IDX-001) se omite con `background reindex skipped (cross-workspace read; ...)`; con `--allow-cross-workspace` se lanza y el uso queda registrado en `overrides.log` con `operation: index.autoindex`. Un canon link registrado sigue habilitando el acceso sin override. El detalle de la atribución está en `07_technical.md`.
- Decisión aprobada por Gabriel el 2026-10-03, que reemplaza el rechazo previo de lecturas cross-workspace para callers harness: leer otro workspace no requiere flag (warning con ambos roots); escribir requiere `--allow-cross-workspace` y cada uso queda registrado en `~/.mi-lsp/overrides.log`. El reindex en segundo plano que dispararía una lectura cuenta como escritura. El override es manual: un bot nunca se lo concede solo.

### Estado

`implemented`

### Notas de implementación

- Gate de gobernanza: `internal/service/governance.go` (`governanceGateEnvelope`)
- Gate activo en: `nav.ask`, `nav.pack`, `nav.route`
- `nav.governance` y workspace ops (init/status/list/add/remove) excluidos del gate por diseño — operaciones de diagnóstico y bootstrap que deben sobrevivir blocked mode
- Profile + projection: `internal/docgraph/governance.go` (`InspectGovernance`, `LoadProfile`)
- Cobertura de tests: TC-WKS-014, TC-WKS-015, TC-WKS-019, TC-WKS-020, TC-WKS-025, `TestNavPackBlockedWhenGovernanceIsInvalid`, `TestNavRouteBlockedWhenGovernanceIsInvalid`

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `GovernanceStatus`
- `WorkspaceRegistration`
- `QueryEnvelope`

## Errores

### 4. Errores tipados
- **Código**: `WKS_GOV_BLOCKED`; **Causa**: gobernanza inválida; **Trigger**: doc faltante, YAML inválido, proyección stale, índice stale; **Respuesta esperada**: devolver estado bloqueado y pasos de reparación
- **Código**: `WKS_GOV_UNCLEAR`; **Causa**: perfil o cadenas contradictorias; **Trigger**: schema semivalido pero ambiguo; **Respuesta esperada**: bloquear y listar contradicciones

## Criterios de aceptación (numerados)

1. Toda tarea consulta el estado de gobernanza del workspace antes de continuar.
2. `workspace status` expone `workspace_root`, `workspace_source`, perfil, sync, index sync y estado bloqueado; cuando existe snapshot repo-local de reentrada, puede exponer `memory_pointer` en preview y `memory` completo bajo expansion.
3. Bajo `--full`, si la memoria esta stale, `auto_sync` esta habilitado y la gobernanza no esta bloqueada, puede refrescar docs/memoria antes de responder. `--no-auto-sync` conserva el diagnóstico read-only y deja visible el stale.
4. Si la gobernanza es valida, el workflow normal puede seguir.
5. Si la gobernanza es inválida, el repo entra en `blocked mode`.
6. En `blocked mode` solo quedan permitidos diagnóstico y reparación de gobernanza.
