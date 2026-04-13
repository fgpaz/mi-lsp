# Task T1: Integrar resolveCanonicalRoute en pack happy path

## Shared Context
**Goal:** Usar `resolveCanonicalRoute` como backbone del happy path de pack; poblar `next_queries`.
**Stack:** Go, `internal/service/pack.go`, `internal/service/route.go`, `internal/model/types.go`
**Architecture:** `resolveCanonicalRoute(ctx, registration, task, opts, false)` devuelve `RouteResult` con `Canonical.AnchorDoc`. Pack debe usar ese anchor para seleccionar `primary_doc` en vez de `selectPackPrimary` independiente. El anchor explícito (`--rf/--fl/--doc`) se pasa como `payload` — `resolvePackAnchor` ya lo extrae; solo hay que fusionarlo con el resultado del route core.

## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/service/pack.go:60-140
  - read: internal/service/route.go:20-98
  - modify: internal/model/types.go    # solo si PackResult necesita NextQueries
complexity: medium
done_when: "go build ./... EXIT:0 && go test ./internal/service -run Pack EXIT:0"
```

## Reference
`internal/service/route.go:20-98` — `resolveCanonicalRoute` signature y shape de retorno.
`internal/service/pack.go:62-136` — happy path actual a reemplazar/extender.
`internal/service/pack.go:139-158` — `resolvePackAnchor` que extrae el anchor del payload (NO eliminar).

## Prompt
Eres un agente de implementación. Tienes que integrar `resolveCanonicalRoute` en el happy path de `nav pack`.

**Lo que debe cambiar en `pack.go`:**

1. Después de abrir el DB y cargar el profile (línea ~62), ANTES de `resolvePackAnchor`, llamar a `resolveCanonicalRoute`:
   ```go
   routeResult, _, routeWarnings, err := a.resolveCanonicalRoute(ctx, registration, task, request.Context, false)
   if err != nil {
       return model.Envelope{}, err
   }
   warnings = append(warnings, routeWarnings...)
   ```

2. Si `routeResult.Canonical.AnchorDoc.Path != ""`, usar ese path como `primary_doc` candidato:
   ```go
   if routeResult.Canonical.AnchorDoc.Path != "" {
       result.PrimaryDoc = routeResult.Canonical.AnchorDoc.Path
       result.Why = append(result.Why, "tier2=route_core")
   }
   ```

3. El `resolvePackAnchor` hardened (--rf/--fl/--doc) GANA sobre el resultado del route core cuando está presente. Si el anchor explícito resuelve un doc, sobrescribe `result.PrimaryDoc`.

4. Si `PackResult` NO tiene `NextQueries []string`:
   - Agregar el campo a `model.PackResult` en `types.go`: `NextQueries []string \`json:"next_queries,omitempty"\``
   - En `pack.go`, al final, poblar con 2-3 sugerencias basadas en la familia:
     ```go
     result.NextQueries = buildPackNextQueries(family, registration.Name)
     ```
   - Implementar `buildPackNextQueries(family, alias string) []string` al final del archivo:
     ```go
     func buildPackNextQueries(family string, alias string) []string {
         switch family {
         case "technical":
             return []string{
                 fmt.Sprintf("mi-lsp nav ask \"explain the technical baseline\" --workspace %s", alias),
                 fmt.Sprintf("mi-lsp nav search \"TECH-\" --workspace %s --format toon", alias),
             }
         default:
             return []string{
                 fmt.Sprintf("mi-lsp nav ask \"what are the main flows?\" --workspace %s", alias),
                 fmt.Sprintf("mi-lsp nav search \"RF-\" --workspace %s --format toon", alias),
             }
         }
     }
     ```

5. Asegurarte de que el test `TestNavPackWarnsWhenCanonicalWikiExistsButDocsAreNotIndexed` siga pasando — ese test usa el camino de fallback Tier 1 (no el happy path), no debe verse afectado.

**NO hacer:**
- No eliminar `resolvePackAnchor` ni `selectPackPrimary` — siguen siendo necesarios para la selección de docs secundarios
- No cambiar el shape de `PackDoc` ni `PackTarget`
- No tocar `route.go` ni `ask.go`

**Verificar:**
```bash
go build ./... 
go test ./internal/service -run Pack -v -count=1
```
Todos los tests de Pack deben pasar.

## Skeleton
```go
// En pack.go, happy path — DESPUÉS de abrir DB y cargar profile
routeResult, _, routeWarnings, err := a.resolveCanonicalRoute(ctx, registration, task, request.Context, false)
if err != nil {
    return model.Envelope{}, err
}
warnings = append(warnings, routeWarnings...)

// Anchor explícito (--rf/--fl/--doc) sobrescribe route core
hardAnchor, family := resolvePackAnchor(request.Payload, task, docs, docByPath, profile)
result.Family = family

if routeResult.Canonical.AnchorDoc.Path != "" && hardAnchor.DocPath == "" && hardAnchor.DocID == "" {
    result.PrimaryDoc = routeResult.Canonical.AnchorDoc.Path
    result.Why = append(result.Why, "tier2=route_core")
}
```

## Verify
`go build ./... && go test ./internal/service -run Pack -count=1` → EXIT:0

## Commit
`feat(pack): use resolveCanonicalRoute as routing backbone in happy path`
