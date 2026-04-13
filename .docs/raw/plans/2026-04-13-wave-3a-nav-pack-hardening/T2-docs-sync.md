# Task T2: Docs sync — CT-NAV-PACK + RF-QRY-012

## Shared Context
**Goal:** Alinear CT-NAV-PACK y RF-QRY-012 con la implementación de Wave 3a.
**Stack:** Markdown wiki bajo `.docs/wiki/`
**Architecture:** CT-NAV-PACK ya existe como contrato. RF-QRY-012 ya tiene spec. Ambos necesitan reflejar: route core como backbone, next_queries en envelope, estado `implemented`.

## Task Metadata
```yaml
id: T2
depends_on: [T0]
agent_type: ps-docs
files:
  - modify: .docs/wiki/09_contratos/CT-NAV-PACK.md
  - modify: .docs/wiki/04_RF/RF-QRY-012.md
  - modify: .docs/wiki/09_contratos_tecnicos.md
complexity: low
done_when: "CT-NAV-PACK refleja next_queries y route core; RF-QRY-012 estado=ready (no cambiar a implemented hasta que T1 esté done)"
```

## Reference
`.docs/wiki/09_contratos/CT-NAV-ROUTE.md` — seguir el mismo estilo de contrato que CT-NAV-ROUTE.
`.docs/wiki/04_RF/RF-QRY-014.md` — estilo de RF ya implementado para referencia de formato.

## Prompt
Eres un agente de documentación. Actualiza los docs de Wave 3a.

**CT-NAV-PACK.md** — agregar/actualizar:
1. En la sección "Respuesta", añadir `next_queries[]` a los campos de cada item:
   ```
   - next_queries: lista de 2-3 comandos sugeridos según familia
   ```
2. Agregar sección "Routing interno":
   ```markdown
   ## Routing interno
   
   `nav pack` llama a `resolveCanonicalRoute` como primer paso del happy path para resolver el anchor canónico (Tier 1 + Tier 2). El anchor explícito (`--rf`, `--fl`, `--doc`) gana sobre el route core cuando está presente.
   ```
3. Mantener `Estado: ready` (no cambiar a implemented hasta que T1 esté done).

**RF-QRY-012.md** — actualizar sección "Special Cases":
1. Agregar el punto: "El core de routing canonico (`resolveCanonicalRoute`) se invoca como backbone del happy path; los flags `--rf/--fl/--doc` hardean la selección final sobre el resultado del route core."
2. En Data Model Impact, agregar `RouteResult` si no está listado.
3. Mantener estado `ready`.

**09_contratos_tecnicos.md** — verificar que CT-NAV-PACK está listado; si no, agregar entrada.

NO cambiar ningún código. Solo docs.

## Skeleton
```markdown
<!-- CT-NAV-PACK.md — sección a agregar -->
## Routing interno

`nav pack` invoca `resolveCanonicalRoute` (Tier 1 + Tier 2) como primer paso del happy path.
El anchor explícito (`--rf`, `--fl`, `--doc`) gana sobre el route core cuando está presente.
El resultado incluye `next_queries[]` sugeridos según la familia documental detectada.
```

## Verify
Leer CT-NAV-PACK.md y verificar que `next_queries` y sección "Routing interno" están presentes.

## Commit
`docs(ct-nav-pack): add next_queries and route core routing notes`
