---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-001
  - RF-WIKI-002
  - RF-WIKI-003
  - RF-WIKI-004
  - RF-WIKI-005
allowed_paths:
  - .docs/wiki/04_RF.md
  - .docs/wiki/04_RF/RF-WIKI-001.md
  - .docs/wiki/04_RF/RF-WIKI-002.md
  - .docs/wiki/04_RF/RF-WIKI-003.md
  - .docs/wiki/04_RF/RF-WIKI-004.md
  - .docs/wiki/04_RF/RF-WIKI-005.md
forbidden_paths:
  - .docs/wiki/03_FL/**
  - .docs/wiki/06_pruebas/**
  - .docs/wiki/07_tech/**
  - .docs/wiki/09_contratos/**
  - internal/**
verify:
  - test -f .docs/wiki/04_RF/RF-WIKI-001.md
  - test -f .docs/wiki/04_RF/RF-WIKI-002.md
  - test -f .docs/wiki/04_RF/RF-WIKI-003.md
  - test -f .docs/wiki/04_RF/RF-WIKI-004.md
  - test -f .docs/wiki/04_RF/RF-WIKI-005.md
  - rg -n "RF-WIKI-00[1-5]" .docs/wiki/04_RF.md | wc -l -> >= 5
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-001.md,.docs/wiki/04_RF/RF-WIKI-002.md,.docs/wiki/04_RF/RF-WIKI-003.md,.docs/wiki/04_RF/RF-WIKI-004.md,.docs/wiki/04_RF/RF-WIKI-005.md --format toon -> ok=true
stop_if:
  - FL-WIKI-01.md no existe (T1 falló)
  - patrón canónico RF-QRY-001.md tiene shape incompatible con SDD-HARNESS-v1
secret_scan: clean
---

# Task T2: Crear RF-WIKI-001..005 (los cinco subcomandos federados)

## Shared Context
**Goal:** Documentar cinco requerimientos funcionales — uno por subcomando wiki federado: search (001), inventory (002), route (003), trace (004), pack (005).
**Stack:** Markdown SDD con SDD-HARNESS-v1 + SDD-WIKI-SOURCE-v1.
**Architecture:** RFs viven en `.docs/wiki/04_RF/`. Índice en `.docs/wiki/04_RF.md`. Cada RF importa de FL-WIKI-01 (parent flow) y exporta a TP-WIKI (matriz de pruebas) y CT-NAV-WIKI (contrato técnico).

## Locked Decisions
- IDs exactos:
  - `RF-WIKI-001` = `nav wiki search --all-workspaces`
  - `RF-WIKI-002` = `nav wiki inventory --all-workspaces` (nuevo subcomando)
  - `RF-WIKI-003` = `nav wiki route --all-workspaces`
  - `RF-WIKI-004` = `nav wiki trace --all-workspaces`
  - `RF-WIKI-005` = `nav wiki pack --all-workspaces`
- Cada RF describe la promesa funcional (qué hace, qué retorna, criterios de aceptación), NO el formato exacto del envelope (eso es CT) ni el algoritmo (eso es TECH).
- Audience `llm-first` con bloques toon normativos.
- Imports: `FL-WIKI-01`. Exports: `TP-WIKI`, `CT-NAV-WIKI`.

## Task Metadata
```yaml
id: T2
depends_on: [T0, T1]
agent_type: ps-docs
goal_id: G1
github_issues: []
expected_outcome: "Cinco archivos RF-WIKI-00[1-5].md existen con harness válido y 04_RF.md tiene cinco filas nuevas."
files:
  - create: .docs/wiki/04_RF/RF-WIKI-001.md
  - create: .docs/wiki/04_RF/RF-WIKI-002.md
  - create: .docs/wiki/04_RF/RF-WIKI-003.md
  - create: .docs/wiki/04_RF/RF-WIKI-004.md
  - create: .docs/wiki/04_RF/RF-WIKI-005.md
  - modify: .docs/wiki/04_RF.md
  - read: .docs/wiki/04_RF/RF-QRY-001.md   # patrón canónico
  - read: .docs/wiki/04_RF.md              # estructura del índice
  - read: .docs/wiki/03_FL/FL-WIKI-01.md   # parent flow
complexity: high
done_when:
  - "los cinco archivos RF-WIKI-*.md existen"
  - "cada RF tiene SDD-HARNESS-v1 frontmatter con id correcto, kind=requirement, audience=llm-first"
  - "cada RF tiene al menos un bloque toon con block_id=rf-wiki-00X-accepts"
  - "04_RF.md tiene cinco filas nuevas siguiendo el formato existente"
  - "mi-lsp nav wiki validate-source contra los cinco paths returns ok=true"
evidence_expected:
  - "Output de mi-lsp nav wiki validate-harness y validate-source contra los cinco RF"
  - "Diff de 04_RF.md"
stop_if:
  - "FL-WIKI-01.md no existe (T1 falló) — el RF necesita el FL como imports"
  - "el patrón RF-QRY-001 tiene shape distinto a SDD-HARNESS-v1 (escalado a governance repair)"
```

## Reference
- Patrón a seguir: `.docs/wiki/04_RF/RF-QRY-001.md` (consulta single-workspace). Replicar la estructura, no el contenido.
- Índice: `.docs/wiki/04_RF.md`.
- Parent flow: `.docs/wiki/03_FL/FL-WIKI-01.md` (creado en T1).
- Decisiones de alcance: `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md`.

## Prompt

Sos el ejecutor de T2 (ps-docs). Tu trabajo es crear cinco archivos RF y agregar cinco entradas al índice.

Para cada RF (RF-WIKI-001 hasta RF-WIKI-005), seguir esta plantilla literal:

**Contenido por RF** (sustituí `XXX` con el ID y `SUBCMD` con search/inventory/route/trace/pack):

1. Frontmatter SDD-HARNESS-v1: `id: RF-WIKI-XXX`, `kind: requirement`, `audience: llm-first`, `imports: [FL-WIKI-01]`, `exports: [TP-WIKI, CT-NAV-WIKI]`, más todos los campos obligatorios.
2. `doc_id: RF-WIKI-XXX`.
3. **Bloque toon `rf-wiki-XXX-promise`**: la promesa funcional en una línea ("Federar `nav wiki SUBCMD` sobre todos los workspaces registrados con un solo flag `--all-workspaces`, devolviendo un envelope con un item por hit anotado con su workspace").
4. **Bloque toon `rf-wiki-XXX-accepts`**: criterios de aceptación verificables (al menos 4 por RF, ej: "el flag --all-workspaces existe y es opcional; cuando está ausente el comportamiento es idéntico al actual; el envelope retorna ok=true incluso si N workspaces fallaron; el campo stats.workspaces_queried >= 1").
5. **Bloque toon `rf-wiki-XXX-out-of-scope`**: qué NO entra en este RF (ej: para RF-WIKI-005 pack, "no se mergea en un super-pack global; devuelve N mini-packs por workspace").
6. Sección con Obsidian-style links a FL-WIKI-01, TP-WIKI, CT-NAV-WIKI.

**Reglas específicas por RF:**

- **RF-WIKI-001 (search)**: aceptar `--all-workspaces`, ranking compatible con el actual `nav ask` (score = doc_evidence*10 + code_evidence*5). Top global default 50. Soporta `--layer`, `--include-content`, `--top`, `--offset` igual que single-workspace.
- **RF-WIKI-002 (inventory)**: comando **nuevo**. Default ligero (~2-3KB / 50 wikis). Flag opt-in `--with-layer-counts` agrega conteos por capa (~5KB). Item shape: `alias`, `root`, `wiki_root`, `governance_blocked`, `docs_ready`, `doc_count`, `last_indexed_at`. Con flag: agregar `layers: {RS,FL,RF,TP,TECH,DB,CT}`.
- **RF-WIKI-003 (route)**: federar el resolutor de ruta canónica por tarea. Cuando hay candidatos en N wikis, devuelve N items con `workspace`.
- **RF-WIKI-004 (trace)**: federar la traza RS/RF/TP a evidencia. `--all` y `--summary` siguen siendo válidos. El envelope global agrupa items por workspace pero no fusiona trazas entre wikis distintas.
- **RF-WIKI-005 (pack)**: federar `pack`. Devuelve N mini-packs (uno por workspace), NO un super-pack. CT documentará esto explícitamente.

**Índice 04_RF.md**: Después de crear los cinco archivos, abrir `.docs/wiki/04_RF.md` y agregar cinco entradas siguiendo el formato existente (ver las filas de RF-QRY-* para el shape). Insertar respetando el orden actual (alfabético o por dominio — replicar lo que ya hay).

**Validar:**
```powershell
mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-001.md,.docs/wiki/04_RF/RF-WIKI-002.md,.docs/wiki/04_RF/RF-WIKI-003.md,.docs/wiki/04_RF/RF-WIKI-004.md,.docs/wiki/04_RF/RF-WIKI-005.md --format toon
mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/04_RF/RF-WIKI-001.md,.docs/wiki/04_RF/RF-WIKI-002.md,.docs/wiki/04_RF/RF-WIKI-003.md,.docs/wiki/04_RF/RF-WIKI-004.md,.docs/wiki/04_RF/RF-WIKI-005.md --format toon
```

Commitear: `docs(wiki): add RF-WIKI-001..005 for federated nav wiki subcommands`.

## Execution Procedure
1. Leer RF-QRY-001.md y 04_RF.md.
2. Crear los cinco archivos siguiendo plantilla.
3. Actualizar índice 04_RF.md.
4. Validar harness + source.
5. Commit.
6. Reportar.

## Skeleton

```markdown
---
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: RF-WIKI-001
doc_id: RF-WIKI-001
kind: requirement
audience: llm-first
imports: [FL-WIKI-01]
exports: [TP-WIKI, CT-NAV-WIKI]
agent_must_read: [...]
agent_may_edit: false
agent_must_not_edit: [...]
verify: [...]
stop_if: [...]
evidence: [...]
---

# RF-WIKI-001 — nav wiki search --all-workspaces

```toon
block_id: rf-wiki-001-promise
promise: "Federar nav wiki search sobre todos los workspaces registrados con --all-workspaces"
```

```toon
block_id: rf-wiki-001-accepts
accepts:
  - "El flag --all-workspaces es opcional y compatible con todos los flags actuales (--layer, --include-content, --top, --offset)."
  - "El envelope retorna ok=true aún si workspaces individuales fallan (registrados en stats.workspaces_failed[])."
  - "Cada item tiene el campo workspace:<alias> con el alias del registry."
  - "El score es comparable cross-workspace (score = doc_evidence*10 + code_evidence*5)."
  - "Top global default = 50; se puede override con --top-global."
```

```toon
block_id: rf-wiki-001-out-of-scope
out_of_scope:
  - "Merge cross-máquina (vive en Hermes wrapper, no en mi-lsp)"
  - "Cache distribuido del fan-out"
```

## Links
- [[FL-WIKI-01]]
- [[TP-WIKI]]
- [[CT-NAV-WIKI]]
```

## Verify
`mi-lsp nav wiki validate-source --workspace mi-lsp --paths <los cinco RFs> --format toon` -> `ok=true`

## Commit
`docs(wiki): add RF-WIKI-001..005 for federated nav wiki subcommands`
