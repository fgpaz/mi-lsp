---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-004
kind: requirement
status: active
title: RF-WKS-004 - Exponer AXI selectivo por superficie para onboarding y discovery del CLI
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/cli/axi_mode.go
    role: behavior
  - path: internal/cli/root.go
    role: behavior
  - path: internal/cli/workspace.go
    role: behavior
  - path: internal/service/workspace_ops.go
    role: behavior
  - path: internal/workspace/registry.go
    role: behavior
---

## Propósito

RF-WKS-004 - Exponer AXI selectivo por superficie para onboarding y discovery del CLI

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-WKS-004
- **Campo**: Título; **Valor**: Exponer AXI selectivo por superficie para onboarding y discovery del CLI
- **Campo**: Actores; **Valor**: Desarrollador, Skill, Agente, CLI/Core
- **Campo**: Prioridad; **Valor**: media
- **Campo**: Severidad; **Valor**: media
- **Campo**: FL origen; **Valor**: FL-BOOT-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: CLI publica disponible; **Tipo**: técnica; **Estado requerido**: obligatorio
- **Condición**: Resolución centralizada del modo efectivo por superficie; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Si se usa `--full`, el comando debe quedar en modo AXI efectivo; **Tipo**: funcional; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. La CLI resuelve el modo efectivo combinando defaults por superficie, `--axi`, `--classic` y `MI_LSP_AXI=1`.
2. `mi-lsp` sin subcomando entra en AXI por default y devuelve un home content-first salvo `--classic`.
3. El home intenta resolver el workspace por `--workspace`, `cwd` o ultimo workspace registrado.
4. `init`, `workspace status`, `nav search` y `nav intent` pertenecen a la superficie AXI-default y arrancan en preview-first si no hubo `--classic`.
5. `nav ask` entra en AXI por default solo para preguntas cortas de onboarding/orientacion; las preguntas con señales de implementacion quedan clasicas salvo `--axi`.
6. `nav workspace-map` y el resto de la CLI quedan en modo clasico por default; pueden entrar en AXI solo por `--axi` o `MI_LSP_AXI=1`.
7. Si el usuario define `--format`, `--max-items`, `--max-chars` o `--token-budget`, esos overrides ganan sobre defaults AXI.

### 5. Casos especiales y variantes
- `--axi=false` debe poder anular `MI_LSP_AXI=1`.
- `--classic` debe prevalecer sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos deben fallar con error claro.
- La version actual no usa hooks ni contexto ambiente fuera del proceso CLI.
- El home AXI puede mostrar readiness barata (daemon/worker) pero no debe mutar runtime solo para renderizar el overview.
- `workspace list` sin flags sigue preservando todos los aliases registrados.
- `workspace list --group-by-root` solo agrega una vista diagnostica por root exacto; no deduplica ni modifica el registry.
- `workspace doctor` es diagnóstico no mutante para aliases duplicados, familias de worktrees, paths stale, colisiones de paths Git que difieren solo por casing y shadowing de binario.
- `workspace hygiene` es la superficie agent-first de higiene del registry: por default diagnostica sin mutar; con `--apply-safe` solo delega en la poda segura de aliases con root inexistente y limpia defaults invalidos. No borra directorios, worktrees, índices, ramas ni procesos.
- `workspace prune --stale` conserva el contrato de limpieza puntual de registry: en dry-run no muta; con `--apply` remueve solo aliases con root inexistente y no toca archivos ni worktrees.

### Estado

`implemented`

### Notas de implementación

- Resolución del modo efectivo: `internal/cli/axi_mode.go` (`resolveAXIDecision`)
- Seed desde env `MI_LSP_AXI`: `internal/cli/root.go:52-62`
- TOON default en PersistentPreRunE: `internal/cli/root.go:103-106`
- Mutual exclusion `--axi + --classic`: `internal/cli/root.go:101-103`
- `--axi=false` hard disable: `internal/cli/axi_mode.go:65-69`
- Cobertura de tests: TC-WKS-011, TC-WKS-012, TC-WKS-013, TC-WKS-016, TC-WKS-017, TC-WKS-018, TC-WKS-021, TC-WKS-022, TC-WKS-023, TC-WKS-024, TC-WKS-031, TC-WKS-032, TC-WKS-034

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `QueryOptions`
- `QueryEnvelope`

## Errores

### 4. Errores tipados
- **Código**: `WKS_AXI_UNSUPPORTED_SURFACE`; **Causa**: expansion fuera de alcance; **Trigger**: `--full` en un comando no cubierto; **Respuesta esperada**: ignorar o degradar sin romper el comando base
- **Código**: `WKS_AXI_WORKSPACE_UNRESOLVED`; **Causa**: sin contexto de workspace; **Trigger**: root AXI no puede resolver workspace actual/default; **Respuesta esperada**: devolver home con sugerencias de bootstrap, sin side effects

## Criterios de aceptación (numerados)

1. La CLI resuelve el modo efectivo combinando defaults por superficie, `--axi`, `--classic` y `MI_LSP_AXI=1`.
2. `mi-lsp` sin subcomando entra en AXI por default y devuelve un home content-first salvo `--classic`.
3. El home intenta resolver el workspace por `--workspace`, `cwd` o ultimo workspace registrado.
4. `init`, `workspace status`, `nav search` y `nav intent` pertenecen a la superficie AXI-default y arrancan en preview-first si no hubo `--classic`.
5. `nav ask` entra en AXI por default solo para preguntas cortas de onboarding/orientacion; las preguntas con señales de implementacion quedan clasicas salvo `--axi`.
6. `nav workspace-map` y el resto de la CLI quedan en modo clasico por default; pueden entrar en AXI solo por `--axi` o `MI_LSP_AXI=1`.
7. Si el usuario define `--format`, `--max-items`, `--max-chars` o `--token-budget`, esos overrides ganan sobre defaults AXI.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-WKS-004
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-BOOT-01.md
exports:
- RF-WKS-004
- RF-WKS-004 - Exponer AXI selectivo por superficie para onboarding y discovery del
  CLI
agent_must_read:
- .docs/wiki/05_requirements/RF-WKS-004.md
- .docs/wiki/04_flows/FL-BOOT-01.md
- .docs/wiki/06_tests/TP-WKS.md
- .docs/wiki/06_tests/TP-WKS-PROBE.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-WKS-004.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/cli/axi_mode.go
- internal/cli/root.go
- internal/cli/workspace.go
- internal/service/workspace_ops.go
- internal/workspace/registry.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-WKS.md, ejecutar el
  comando publicado: go test -count=1 ./internal/service -run ''^TestExecuteAutoRegisterSkipsIndexWithoutCommits$''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-WKS-004.
evidence:
- .docs/wiki/05_requirements/RF-WKS-004.md
- .docs/wiki/06_tests/TP-WKS.md
- .docs/wiki/06_tests/TP-WKS-PROBE.md
```
