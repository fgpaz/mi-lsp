---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - TECH-WIKI-FANOUT
  - RF-WIKI-001
  - RF-WIKI-002
  - RF-WIKI-003
  - RF-WIKI-004
  - RF-WIKI-005
allowed_paths:
  - internal/nav/**
  - internal/service/**
  - internal/cli/nav.go
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
  - .mi-lsp/**
verify:
  - go build ./... -> exit 0
  - go vet ./internal/nav/... -> no errors
  - rg -n "FanOutWiki" internal/nav/ -> match
stop_if:
  - internal/service/ask.go no contiene la función AllWorkspaces ni un patrón equivalente
  - go module dependencies inconsistencies block build
secret_scan: clean
---

# Task T7: Implementar FanOutWiki primitive en internal/nav/

## Shared Context
**Goal:** Crear el helper `FanOutWiki(ctx, op, opts)` que abstrae el fan-out con semaphore=4 sobre `workspace list`, listo para ser invocado por los cinco subcomandos wiki.
**Stack:** Go 1.22+ (módulo mi-lsp).
**Architecture:** `internal/nav/` contiene primitivas de navegación. `internal/service/ask.go:465-564` tiene el patrón `AllWorkspaces` con semaphore=4 — esta tarea lo EXTRAE o lo REUSA (no copy-paste).

## Locked Decisions
- Vivir en `internal/nav/fanout_wiki.go` (archivo nuevo).
- Reusar el iterator de registry y el semaphore de `internal/service/ask.go` — NO duplicar la lógica del semaphore.
- Signatura objetivo (adaptar al tipo real del repo):
  ```go
  type WikiFanOutOptions struct {
      WorkspaceFilter []string  // si vacío, todos
      Timeout         time.Duration  // default 30s per workspace
      Parallel        int       // default 4
  }
  
  type WikiFanOutItem struct {
      Workspace string         // alias
      Items     []any          // resultado del subcomando per-workspace
      Stats     map[string]any // stats per-workspace
      Err       error
  }
  
  type WikiFanOutResult struct {
      Items             []WikiFanOutItem
      WorkspacesQueried int
      WorkspacesFailed  []WorkspaceFailure
      TruncatedPerWS    bool
  }
  
  func FanOutWiki(ctx context.Context, opts WikiFanOutOptions, fn func(ctx context.Context, ws WorkspaceRegistration) (items []any, stats map[string]any, err error)) (*WikiFanOutResult, error)
  ```
- Cada subcomando wiki construye su propia closure `fn` que sabe cómo consultar el doc-index per-workspace.
- Timeout=30s default heredado de `nav ask`.

## Task Metadata
```yaml
id: T7
depends_on: [T0, T4]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "Existe internal/nav/fanout_wiki.go con FanOutWiki implementado y testeable, reusando AllWorkspaces pattern; go build PASS."
files:
  - create: internal/nav/fanout_wiki.go
  - read: internal/service/ask.go:465-564
  - read: internal/cli/nav.go:546-689
  - read: internal/registry/   # para entender WorkspaceRegistration shape
complexity: high
done_when:
  - "internal/nav/fanout_wiki.go compila"
  - "go build ./... exit 0"
  - "go vet ./internal/nav/... no errors"
  - "rg -n 'func FanOutWiki' internal/nav/fanout_wiki.go returns match"
  - "el helper NO contiene SSH ni transporte de red"
evidence_expected:
  - "Output de go build y go vet"
  - "Output de rg -n 'FanOutWiki' internal/"
stop_if:
  - "AllWorkspaces pattern en ask.go fue renombrado o eliminado — alertar y pedir nuevo audit antes de seguir"
  - "Registry no expone iterator de WorkspaceRegistration usable desde internal/nav/"
```

## Reference
- Patrón a copiar (NO refactorizar): `internal/service/ask.go:465-564` (función o método AllWorkspaces; conserva su semáforo y política de error).
- WorkspaceRegistration type: `internal/registry/` (mirar definición).
- Para inspirarse en cómo `nav wiki search` accede al doc-index per-workspace: `internal/cli/nav.go:546-689` y/o `internal/service/wiki.go` (si existe).
- **OBLIGATORIO: usar `mi-lsp nav search "AllWorkspaces" --workspace mi-lsp --format toon --include-content`** para localizar el patrón actual antes de implementar. No usar Grep raw sin antes intentar con mi-lsp.

## Prompt

Sos el ejecutor de T7 (ps-worker). Tu trabajo es escribir Go nuevo SIN duplicar el patrón existente.

1. **Navegar el código con mi-lsp**: ejecutar:
   ```powershell
   mi-lsp nav search "AllWorkspaces" --workspace mi-lsp --include-content --format toon
   mi-lsp nav refs AllWorkspaces --workspace mi-lsp --format toon
   mi-lsp nav context AllWorkspaces --workspace mi-lsp --format toon
   ```
2. Leer `internal/service/ask.go` entre las líneas que devuelve la consulta (esperado ~465-564) para entender la signatura real del helper actual y cómo se invoca el semaphore.
3. Leer la definición de `WorkspaceRegistration` o equivalente en `internal/registry/`.
4. Decidir entre dos rutas (cualquiera válida — documentar la elegida en un comentario corto en el archivo nuevo):
   - **Ruta A** (preferida): exportar el patrón actual desde `internal/service/ask.go` a un helper genérico en `internal/nav/fanout.go` (si no existe) o `internal/nav/fanout_wiki.go`, y refactor `ask.go` para usar el helper.
   - **Ruta B**: dejar `ask.go` intacto y crear un helper paralelo en `internal/nav/fanout_wiki.go` que reusa la misma semántica (mismo timeout, mismo semaphore, misma política de error). Documentar con `// derived from internal/service/ask.go AllWorkspaces pattern`.
5. Implementar la signatura listada en "Locked Decisions" (adaptar tipos a los del repo real — si `WorkspaceRegistration` tiene otro nombre, usar el real).
6. **Comportamiento esperado**:
   - Iterar `registry.List()` (o equivalente).
   - Para cada workspace, lanzar goroutine bounded por semaphore=4.
   - Cada goroutine corre `fn(ctx, ws)` con timeout context (30s default).
   - Capturar errores en `WorkspacesFailed[]`. NUNCA abortar el global por un workspace fallido.
   - Devolver el resultado agregado.
7. **NO** mezclar este helper con código que conoce subcomandos específicos — debe ser una primitiva neutral. Los subcomandos (T8-T12) le pasan la closure.
8. Ejecutar:
   ```powershell
   go build ./...
   go vet ./internal/nav/...
   ```
   Ambos deben exit 0.
9. Commit: `feat(nav): add FanOutWiki primitive for federated wiki commands`.
10. Reportar al orquestador con: ruta elegida (A o B), output de go build/vet, y signatura final del helper.

**No hacer:**
- No agregar SSH, hosts, transporte de red.
- No implementar los subcomandos wiki (eso es T8-T12).
- No tocar `worker-dotnet/`.

## Execution Procedure
1. mi-lsp search/refs/context sobre AllWorkspaces.
2. Leer ask.go alrededor del patrón.
3. Decidir Ruta A o B y documentar.
4. Crear `internal/nav/fanout_wiki.go`.
5. go build + go vet.
6. Commit.
7. Reportar.

## Skeleton

```go
package nav

import (
    "context"
    "time"

    "github.com/<owner>/mi-lsp/internal/registry"  // ajustar al import path real
)

// FanOutWiki itera workspaces del registry y aplica fn por cada uno
// con semaphore=4 y timeout per workspace. Derived from internal/service/ask.go
// AllWorkspaces pattern (semaphore=4, no-abort-on-failure policy).
//
// Esta primitiva no conoce hosts, SSH, ni transporte de red. mi-lsp permanece
// CLI puro per-máquina; el merge cross-host vive en clientes externos.
type WikiFanOutOptions struct {
    WorkspaceFilter []string
    Timeout         time.Duration
    Parallel        int
}

type WorkspaceFailure struct {
    Alias  string
    Reason string
}

type WikiFanOutItem struct {
    Workspace string
    Items     []any
    Stats     map[string]any
    Err       error
}

type WikiFanOutResult struct {
    Items             []WikiFanOutItem
    WorkspacesQueried int
    WorkspacesFailed  []WorkspaceFailure
    TruncatedPerWS    bool
}

func FanOutWiki(ctx context.Context, opts WikiFanOutOptions,
    fn func(ctx context.Context, ws registry.WorkspaceRegistration) (items []any, stats map[string]any, err error)) (*WikiFanOutResult, error) {
    // ...
}
```

## Verify
`go build ./...` -> exit 0 AND `go vet ./internal/nav/...` -> no errors AND `rg -n "func FanOutWiki" internal/nav/fanout_wiki.go` -> match

## Commit
`feat(nav): add FanOutWiki primitive for federated wiki commands`
