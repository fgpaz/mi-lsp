# Task T1: TOON default en AXI mode + --axi=false support

## Shared Context
**Goal:** Cuando AXI está activo y no hubo --format explícito, el format default escala a toon. --axi=false anula el default AXI de la superficie.
**Stack:** Go, `internal/cli/root.go`, `internal/cli/axi_mode.go`
**Architecture:** `root.go:52` lee `MI_LSP_AXI`; `PersistentPreRunE` valida format. `effectiveAXI()` devuelve bool. El TOON default se aplica en `PersistentPreRunE` antes de que el comando corra: si `effectiveAXI(cmd, operation, nil)` es true y `!flagChanged(cmd, "format")`, entonces `state.format = "toon"`.

## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/cli/root.go:84-105   # PersistentPreRunE
  - modify: internal/cli/axi_mode.go      # --axi=false handling
complexity: low
done_when: "go build ./... EXIT:0"
```

## Reference
`internal/cli/root.go:84-105` — `PersistentPreRunE` donde vive la validación de flags.
`internal/cli/axi_mode.go:53-76` — `resolveAXIDecision` que ya maneja `--classic`.
`internal/cli/root.go:101-103` — mutual exclusion --axi + --classic existente.

## Prompt
Dos cambios pequeños en `root.go` + uno en `axi_mode.go`.

**Cambio 1 — TOON default en PersistentPreRunE (`root.go:84-105`)**

Después de la validación de `state.backendHint` (línea ~100) y ANTES del return nil, agregar:

```go
// TOON default when AXI is effective and --format was not explicit
if !flagChanged(cmd, "format") {
    // Resolve the effective operation for this command
    // Use "root.home" as fallback if we can't determine the operation
    op := cmd.CommandPath()
    if s.effectiveAXI(cmd, operationFromCmd(cmd), nil) {
        s.format = "toon"
    }
}
```

Nota: `operationFromCmd` puede no existir. Usa una approach más simple:
```go
if !flagChanged(cmd, "format") && s.axi {
    s.format = "toon"
}
```

Esto es suficiente: si el usuario pasó `--axi` o `MI_LSP_AXI=1`, el format escala a toon. Si no pasó `--format`, aplica el override. Si pasó `--format compact` explícito, `flagChanged` lo detecta y no escala.

**Cambio 2 — --axi=false en axi_mode.go**

En `resolveAXIDecision`, cuando `flagChanged(cmd, "axi")` es true pero `s.axi` es false (el usuario pasó `--axi=false`), tratar como classic:

```go
func (s *rootState) resolveAXIDecision(cmd *cobra.Command, operation string, payload map[string]any) axiDecision {
    if !supportsAXISurface(operation) {
        return axiDecision{}
    }
    if isClassicRequested(cmd, s.classic) {
        return axiDecision{Supported: true, Enabled: false}
    }
    // --axi=false explicit override: disable even default AXI surfaces
    if flagChanged(cmd, "axi") && !s.axi {
        return axiDecision{Supported: true, Enabled: false}
    }
    // ... resto igual
```

**NO hacer:**
- No cambiar el default de `state.format` en la inicialización (línea ~55) — solo en `PersistentPreRunE`
- No tocar los tests de axi_mode que ya pasan

## Skeleton
```go
// root.go PersistentPreRunE — añadir al final antes del return nil
if !flagChanged(cmd, "format") && s.axi {
    s.format = "toon"
}

// axi_mode.go resolveAXIDecision — añadir después del isClassicRequested check
if flagChanged(cmd, "axi") && !s.axi {
    return axiDecision{Supported: true, Enabled: false}
}
```

## Verify
`go build ./...` → EXIT:0

## Commit
`feat(axi): toon default when AXI active, --axi=false explicit override`
