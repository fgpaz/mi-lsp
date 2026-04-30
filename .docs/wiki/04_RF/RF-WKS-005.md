---
id: RF-WKS-005
title: Aplicar gate de gobernanza al inicio de toda tarea
implements:
  - internal/service/app.go
  - internal/service/workspace_ops.go
  - internal/workspace/registry.go
tests:
  - internal/service/workspace_resolution_test.go
  - internal/workspace/registry_test.go
---

# RF-WKS-005 - Aplicar gate de gobernanza al inicio de toda tarea

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-005"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-005]]'
exports:
  - 'RF-WKS-005'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-005.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-005.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-005.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-005 |
| Titulo | Aplicar gate de gobernanza al inicio de toda tarea |
| Actores | Desarrollador, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| `00_gobierno_documental.md` presente | funcional | obligatorio |
| Proyeccion ejecutable sincronizada | tecnica | obligatorio |

## 3. Process Steps (Happy Path)

1. Toda tarea consulta el estado de gobernanza del workspace antes de continuar.
2. `workspace status` expone `workspace_root`, `workspace_source`, perfil, sync, index sync y estado bloqueado; cuando existe snapshot repo-local de reentrada, puede exponer `memory_pointer` en preview y `memory` completo bajo expansion.
3. Si la gobernanza es valida, el workflow normal puede seguir.
4. Si la gobernanza es invalida, el repo entra en `blocked mode`.
5. En `blocked mode` solo quedan permitidos diagnostico y reparacion de gobernanza.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_GOV_BLOCKED` | gobernanza invalida | doc faltante, YAML invalido, proyeccion stale, indice stale | devolver estado bloqueado y pasos de reparacion |
| `WKS_GOV_UNCLEAR` | perfil o cadenas contradictorias | schema semivalido pero ambiguo | bloquear y listar contradicciones |

## 5. Special Cases and Variants

- El gate corre al inicio de toda tarea spec-driven, no solo de tareas documentales.
- `workspace status` y `nav governance` siguen disponibles aun en `blocked mode`.
- El policy layer (`AGENTS.md`, `CLAUDE.md`, skills) debe respetar este gate aunque el usuario no lo mencione.
- Si `--workspace <alias>` explicito apunta a un root distinto del workspace registrado que contiene el `caller_cwd`, el alias explicito gana y el warning debe mostrar el desvio para evitar respuestas desde otro worktree.

## 6. Data Model Impact

- `GovernanceStatus`
- `WorkspaceRegistration`
- `QueryEnvelope`

## Estado

`implemented`

## Notas de implementación

- Gate de gobernanza: `internal/service/governance.go` (`governanceGateEnvelope`)
- Gate activo en: `nav.ask`, `nav.pack`, `nav.route`
- `nav.governance` y workspace ops (init/status/list/add/remove) excluidos del gate por diseño — operaciones de diagnóstico y bootstrap que deben sobrevivir blocked mode
- Profile + projection: `internal/docgraph/governance.go` (`InspectGovernance`, `LoadProfile`)
- Cobertura de tests: TC-WKS-014, TC-WKS-015, TC-WKS-019, TC-WKS-020, `TestNavPackBlockedWhenGovernanceIsInvalid`, `TestNavRouteBlockedWhenGovernanceIsInvalid`
