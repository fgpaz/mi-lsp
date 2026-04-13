# Task T0: Gap analysis — pack.go vs CT-NAV-PACK

## Shared Context
**Goal:** Integrar resolveCanonicalRoute en el happy path de nav pack y poblar next_queries.
**Stack:** Go, `internal/service/pack.go`, `internal/service/route.go`, `internal/model/types.go`
**Architecture:** Wave 2 creó `resolveCanonicalRoute` en `route.go`. `pack.go` usa `resolvePackAnchor + selectPackPrimary` en happy path; solo usa Tier 1 como fallback. CT-NAV-PACK define el contrato pero puede estar desalineado.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: ps-explorer
files:
  - read: internal/service/pack.go
  - read: internal/service/route.go
  - read: internal/model/types.go
  - read: .docs/wiki/09_contratos/CT-NAV-PACK.md
  - read: .docs/wiki/04_RF/RF-QRY-012.md
complexity: low
done_when: "Gaps documentados: qué campos faltan en PackResult, si next_queries existe, si resolveCanonicalRoute es compatible con el anchor de pack"
```

## Reference
`internal/service/pack.go:104-158` — `resolvePackAnchor` y `selectPackPrimary` para entender el flujo actual.
`internal/service/route.go:20-98` — `resolveCanonicalRoute` para entender el shape del RouteResult.
`internal/model/types.go` — buscar `PackResult`, `PackDoc`, `RouteResult`, `RouteCanonicalLane`.

## Prompt
Eres un agente de exploración read-only. Tu tarea es documentar los gaps entre la implementación actual de `nav pack` y el contrato `CT-NAV-PACK` + spec `RF-QRY-012`.

Pasos:
1. Leer `internal/service/pack.go` completo. Identificar:
   - El shape de `PackResult` (campos actuales)
   - Si existe `NextQueries []string` o equivalente en `PackResult`
   - Cómo fluye el anchor `--rf/--fl/--doc` actualmente
   - Por qué `resolveCanonicalRoute` NO se usa en el happy path

2. Leer `internal/service/route.go:20-98`. Identificar:
   - El shape de `RouteResult.Canonical.AnchorDoc`
   - Si `resolveCanonicalRoute` puede recibir el anchor explícito (rf/fl/doc) o si necesita que pack lo resuelva primero

3. Leer `internal/model/types.go`. Anotar si `PackResult` tiene campo `NextQueries` o `Family`.

4. Leer `CT-NAV-PACK.md` y `RF-QRY-012.md`. Identificar:
   - Campos del envelope esperados que no están implementados
   - Si `next_queries` está en el contrato

5. Producir un reporte de gaps con:
   - **Gap 1**: ¿Falta `NextQueries` en `PackResult`?
   - **Gap 2**: ¿`resolveCanonicalRoute` puede integrarse en happy path con el anchor actual?
   - **Gap 3**: ¿Hay campos del envelope `CT-NAV-PACK` no implementados?
   - **Recomendación**: qué cambios mínimos necesita T1

NO modifiques ningún archivo. Solo reporta.

## Skeleton
```
# Gap Report: nav pack vs CT-NAV-PACK

## PackResult campos actuales
- task, mode, primary_doc, docs[], why, family
- NextQueries: [presente/ausente]

## resolveCanonicalRoute compatibility
- AnchorDoc.Path compatible con resolvePackAnchor: [sí/no, cómo]
- Anchor explícito (--rf/--fl): puede pasarse como: [...]

## CT-NAV-PACK campos no implementados
- next_queries: [sí/no]
- [otros gaps]

## Recomendación T1
[descripción concisa de cambios mínimos]
```

## Verify
Reporte de gaps presente. Sin cambios en ningún archivo.

## Commit
No aplica (task read-only)
