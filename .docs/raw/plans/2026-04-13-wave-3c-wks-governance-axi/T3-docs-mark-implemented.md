# Task T3: Marcar RF-WKS-004/005 como implemented

## Shared Context
**Goal:** Actualizar los docs para reflejar que RF-WKS-004 y RF-WKS-005 están implementados.
**Stack:** Markdown wiki, `.docs/wiki/04_RF/`
**Architecture:** Los RFs existen como archivos. El estado debe cambiarse de `ready` a `implemented` con notas de implementación apuntando a los archivos de código.

## Task Metadata
```yaml
id: T3
depends_on: [T0]
agent_type: ps-docs
files:
  - modify: .docs/wiki/04_RF/RF-WKS-004.md
  - modify: .docs/wiki/04_RF/RF-WKS-005.md
  - modify: .docs/wiki/04_RF.md
  - modify: .docs/wiki/06_matriz_pruebas_RF.md
complexity: low
done_when: "RF-WKS-004 y RF-WKS-005 tienen estado=implemented con notas de implementación"
```

## Reference
`.docs/wiki/04_RF/RF-QRY-014.md` — estilo de sección `## Estado` + notas de implementación.
`.docs/wiki/06_matriz_pruebas_RF.md` — formato de tabla para agregar RF-WKS-004/005.

## Prompt
Actualiza cuatro documentos.

**RF-WKS-004.md** — agregar sección al final:
```markdown
## Estado

implemented

## Notas de implementación

- `internal/cli/axi_mode.go`: `supportsAXISurface`, `defaultAXIForOperation`, `resolveAXIDecision`
- `internal/cli/root.go:52`: `MI_LSP_AXI=1` env var
- `internal/cli/root.go:101`: `--axi + --classic` mutual exclusion
- `internal/cli/root.go:117-118`: flags `--axi` y `--classic`
- Superficies AXI-default: root.home, workspace.init, workspace.status, nav.search, nav.intent, nav.pack, nav.route
- Superficie AXI-default condicional: nav.ask (orientation questions only)
```

**RF-WKS-005.md** — agregar sección al final:
```markdown
## Estado

implemented

## Notas de implementación

- `internal/service/governance.go`: `governanceGateEnvelope`, `governanceGateEnvelope`
- `internal/docgraph/governance.go`: `InspectGovernance`
- El gate se aplica en: nav.ask, nav.pack, nav.route, nav.governance
- workspace.init, workspace.status, workspace.list quedan fuera del gate por diseño (funcionan en blocked mode)
```

**04_RF.md** — actualizar tabla: cambiar `ready` a `implemented` para RF-WKS-004 y RF-WKS-005.

**06_matriz_pruebas_RF.md** — agregar filas para RF-WKS-004 y RF-WKS-005:
```
| RF-WKS-004 | FL-BOOT-01 | TP-WKS | TestWorkspaceInitAXIModePrefersAXINextSteps | - | implemented |
| RF-WKS-005 | FL-BOOT-01 | TP-WKS | TestNavPackBlockedWhenGovernanceIsInvalid, TestNavRouteBlockedWhenGovernanceIsInvalid | - | implemented |
```

## Skeleton
```markdown
## Estado

implemented

## Notas de implementación

- `internal/cli/axi_mode.go` — [descripción]
```

## Verify
Leer RF-WKS-004.md y RF-WKS-005.md — confirmar `Estado: implemented`.

## Commit
`docs(wks): mark RF-WKS-004 and RF-WKS-005 as implemented`
