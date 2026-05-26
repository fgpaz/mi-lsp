---
id: RF-WKS-004
title: Exponer AXI selectivo por superficie para onboarding y discovery del CLI
implements:
  - internal/cli/workspace.go
  - internal/service/workspace_ops.go
  - internal/workspace/registry.go
tests:
  - internal/service/app_test.go
  - internal/workspace/registry_test.go
---

# RF-WKS-004 - Exponer AXI selectivo por superficie para onboarding y discovery del CLI

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-WKS-004"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-WKS-004]]'
exports:
  - 'RF-WKS-004'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-WKS-004.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-WKS-004.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-WKS-004.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-004 |
| Titulo | Exponer AXI selectivo por superficie para onboarding y discovery del CLI |
| Actores | Desarrollador, Skill, Agente, CLI/Core |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| CLI publica disponible | tecnica | obligatorio |
| Resolucion centralizada del modo efectivo por superficie | funcional | obligatorio |
| Si se usa `--full`, el comando debe quedar en modo AXI efectivo | funcional | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI resuelve el modo efectivo combinando defaults por superficie, `--axi`, `--classic` y `MI_LSP_AXI=1`.
2. `mi-lsp` sin subcomando entra en AXI por default y devuelve un home content-first salvo `--classic`.
3. El home intenta resolver el workspace por `--workspace`, `cwd` o ultimo workspace registrado.
4. `init`, `workspace status`, `nav search` y `nav intent` pertenecen a la superficie AXI-default y arrancan en preview-first si no hubo `--classic`.
5. `nav ask` entra en AXI por default solo para preguntas cortas de onboarding/orientacion; las preguntas con señales de implementacion quedan clasicas salvo `--axi`.
6. `nav workspace-map` y el resto de la CLI quedan en modo clasico por default; pueden entrar en AXI solo por `--axi` o `MI_LSP_AXI=1`.
7. Si el usuario define `--format`, `--max-items`, `--max-chars` o `--token-budget`, esos overrides ganan sobre defaults AXI.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_AXI_UNSUPPORTED_SURFACE` | expansion fuera de alcance | `--full` en un comando no cubierto | ignorar o degradar sin romper el comando base |
| `WKS_AXI_WORKSPACE_UNRESOLVED` | sin contexto de workspace | root AXI no puede resolver workspace actual/default | devolver home con sugerencias de bootstrap, sin side effects |

## 5. Special Cases and Variants

- `--axi=false` debe poder anular `MI_LSP_AXI=1`.
- `--classic` debe prevalecer sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos deben fallar con error claro.
- La version actual no usa hooks ni contexto ambiente fuera del proceso CLI.
- El home AXI puede mostrar readiness barata (daemon/worker) pero no debe mutar runtime solo para renderizar el overview.
- `workspace list` sin flags sigue preservando todos los aliases registrados.
- `workspace list --group-by-root` solo agrega una vista diagnostica por root exacto; no deduplica ni modifica el registry.
- `workspace doctor` es diagnostico no mutante para aliases duplicados, familias de worktrees, paths stale y shadowing de binario.
- `workspace hygiene` es la superficie agent-first de higiene del registry: por default diagnostica sin mutar; con `--apply-safe` solo delega en la poda segura de aliases con root inexistente y limpia defaults invalidos. No borra directorios, worktrees, indices, ramas ni procesos.
- `workspace prune --stale` conserva el contrato de limpieza puntual de registry: en dry-run no muta; con `--apply` remueve solo aliases con root inexistente y no toca archivos ni worktrees.

## 6. Data Model Impact

- `QueryOptions`
- `QueryEnvelope`

## Estado

`implemented`

## Notas de implementación

- Resolución del modo efectivo: `internal/cli/axi_mode.go` (`resolveAXIDecision`)
- Seed desde env `MI_LSP_AXI`: `internal/cli/root.go:52-62`
- TOON default en PersistentPreRunE: `internal/cli/root.go:103-106`
- Mutual exclusion `--axi + --classic`: `internal/cli/root.go:101-103`
- `--axi=false` hard disable: `internal/cli/axi_mode.go:65-69`
- Cobertura de tests: TC-WKS-011, TC-WKS-012, TC-WKS-013, TC-WKS-016, TC-WKS-017, TC-WKS-018, TC-WKS-021, TC-WKS-022, TC-WKS-023, TC-WKS-024
