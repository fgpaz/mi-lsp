# Task T0: Spec freeze — TECH-AXI-DISCOVERY + CT-CLI-AXI-MODE

## Shared Context
**Goal:** TOON-default en AXI sin --format explícito; --axi=false; stage signal anchor|preview|discovery.
**Stack:** Markdown wiki, `.docs/wiki/07_tech/TECH-AXI-DISCOVERY.md`, `.docs/wiki/09_contratos/CT-CLI-AXI-MODE.md`
**Architecture:** TECH-AXI-DISCOVERY describe reglas técnicas del modo AXI. CT-CLI-AXI-MODE define el contrato CLI. Ambos necesitan reflejar las nuevas reglas antes de implementar.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: ps-docs
files:
  - modify: .docs/wiki/07_tech/TECH-AXI-DISCOVERY.md
  - modify: .docs/wiki/09_contratos/CT-CLI-AXI-MODE.md
complexity: low
done_when: "TECH-AXI-DISCOVERY tiene regla TOON-default y stage signal; CT-CLI-AXI-MODE lista --axi=false"
```

## Reference
`.docs/wiki/07_tech/TECH-DOC-ROUTER.md` — estilo de doc técnico para referencia.
`.docs/wiki/09_contratos/CT-NAV-ROUTE.md` — estilo de contrato para referencia.

## Prompt
Actualiza dos docs técnicos para capturar las nuevas reglas de Wave 3b.

**TECH-AXI-DISCOVERY.md** — agregar/actualizar:

1. En "Reglas técnicas", agregar regla:
   ```
   10. Cuando AXI está en modo efectivo y el usuario no pasó `--format` explícito, el format
       por defecto escala a `toon`. El override explícito `--format compact` siempre gana.
   ```

2. En "Reglas técnicas", agregar regla:
   ```
   11. `--axi=false` permite anular explícitamente el default AXI de una superficie cuando
       el usuario quiere salida clásica sin escribir `--classic`.
   ```

3. Agregar sección "Stage signal en discovery lane":
   ```markdown
   ## Stage signal en discovery lane
   
   Cada `RouteDoc` en la discovery lane lleva el campo `stage` con uno de:
   - `anchor` — doc canónico de anclaje (siempre el primero)
   - `preview` — doc del mini preview pack (Tier 1 canonical)
   - `discovery` — doc de discovery advisory (Tier 2, non-authoritative)
   
   El stage permite a los agentes distinguir la fuente de cada doc sin necesidad
   de session state.
   ```

**CT-CLI-AXI-MODE.md** — agregar:

1. En "Activación y precedencia", agregar bullet:
   ```
   - `--axi=false`: anula el default AXI de la superficie actual; equivalente a `--classic` para esa invocación.
   ```

2. En "Reglas de contrato", agregar:
   ```
   - Si AXI está en modo efectivo y no hubo `--format` explícito, el format por defecto es TOON.
   ```

## Skeleton
```markdown
<!-- TECH-AXI-DISCOVERY.md — reglas adicionales -->
10. TOON-default en AXI efectivo sin --format explícito.
11. --axi=false anula default AXI de la superficie.

## Stage signal en discovery lane
- anchor: doc canónico de anclaje
- preview: mini preview pack (Tier 1)
- discovery: advisory Tier 2
```

## Verify
Leer ambos docs y confirmar que los cambios están presentes.

## Commit
`docs(axi): add toon-default rule, --axi=false, and stage signal to AXI specs`
