# Task T0: Audit — governanceGateEnvelope coverage

## Shared Context
**Goal:** Identificar qué operaciones nav usan governanceGateEnvelope y cuáles no.
**Stack:** Go, `internal/service/*.go`
**Architecture:** `governanceGateEnvelope` está en `governance.go`. Cada handler de operación debería llamarlo al inicio. El gap audit determina si hay handlers que lo omiten.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: ps-explorer
files:
  - read: internal/service/governance.go
  - read: internal/service/app.go
  - read: internal/service/ask.go
  - read: internal/service/pack.go
  - read: internal/service/route.go
  - read: internal/service/search.go
  - read: internal/service/intent.go
  - read: internal/service/workspace_ops.go
  - read: internal/service/workspace_map.go
complexity: low
done_when: "Reporte: qué operaciones tienen gate, cuáles no, y si las omisiones son por diseño"
```

## Reference
`internal/service/governance.go` — `governanceGateEnvelope` signature.
`internal/service/route.go:160-165` — ejemplo de cómo se usa el gate en nav.route.

## Prompt
Eres un agente read-only. Audita qué operaciones usan `governanceGateEnvelope`.

1. Leer `governance.go` para entender la signature de `governanceGateEnvelope`.

2. Para cada archivo de service (`ask.go`, `pack.go`, `route.go`, `search.go`, `intent.go`, `workspace_ops.go`, `workspace_map.go`), identificar:
   - Nombre de la función handler
   - Si llama `governanceGateEnvelope` al inicio
   - Si deliberadamente no lo llama (y por qué)

3. Leer `app.go` para ver el dispatch completo y listar todas las operaciones registradas.

4. Producir reporte:
   - **Con gate**: list de operaciones
   - **Sin gate — probablemente por diseño**: workspace.init, workspace.list, workspace.status, nav.governance (estas son operaciones que deben funcionar incluso en blocked mode)
   - **Sin gate — posible gap**: cualquier nav.* que debería tener gate pero no lo tiene

NO modificar nada.

## Skeleton
```
# Gate Coverage Report

## Operaciones CON governanceGateEnvelope
- nav.ask ✓
- nav.pack ✓
- nav.route ✓
- [...]

## Operaciones SIN gate — por diseño (blocked mode OK)
- workspace.init
- workspace.status
- nav.governance
- [...]

## Posibles gaps (nav ops sin gate)
- [lista o "ninguno"]
```

## Verify
Reporte presente. Sin cambios.

## Commit
No aplica (read-only)
