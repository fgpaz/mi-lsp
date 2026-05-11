---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - CT-NAV-WIKI
allowed_paths:
  - .docs/wiki/09_contratos_tecnicos.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
forbidden_paths:
  - .docs/wiki/03_FL/**
  - .docs/wiki/04_RF/**
  - .docs/wiki/06_pruebas/**
  - .docs/wiki/07_tech/**
  - internal/**
verify:
  - rg -n "--all-workspaces" .docs/wiki/09_contratos/CT-NAV-WIKI.md | wc -l -> >= 5
  - rg -n "workspaces_queried" .docs/wiki/09_contratos/CT-NAV-WIKI.md -> match
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-NAV-WIKI.md --format toon -> ok=true
stop_if:
  - CT-NAV-WIKI.md no existe (esperado: ya existe)
  - el archivo tiene un harness contract distinto a SDD-HARNESS-v1
secret_scan: clean
---

# Task T5: Extender CT-NAV-WIKI con --all-workspaces y nuevo subcomando inventory

## Shared Context
**Goal:** Actualizar el contrato técnico existente `CT-NAV-WIKI.md` para incluir el flag `--all-workspaces` en search/route/trace/pack, agregar inventory como subcomando nuevo, y documentar la forma del envelope merge-friendly.
**Stack:** Markdown SDD con SDD-HARNESS-v1 + SDD-WIKI-SOURCE-v1.
**Architecture:** CT vive en `.docs/wiki/09_contratos/CT-NAV-WIKI.md` (ya existe). Resumen en `.docs/wiki/09_contratos_tecnicos.md`.

## Locked Decisions
- NO crear `CT-NAV-WIKI-SEARCH`, `CT-NAV-WIKI-INVENTORY`, etc. — todo vive en `CT-NAV-WIKI.md`.
- Cada subcomando se documenta con un bloque toon `ct-nav-wiki-<subcmd>-contract`.
- Envelope merge-friendly se documenta en un bloque toon compartido `ct-nav-wiki-envelope-all-workspaces`.
- Inventory es el ÚNICO subcomando nuevo: agregar su bloque.
- Imports: `RF-WIKI-001..005`. Exports: ninguno (terminal CT).

## Task Metadata
```yaml
id: T5
depends_on: [T0, T2]
agent_type: ps-docs
goal_id: G1
github_issues: []
expected_outcome: "CT-NAV-WIKI.md documenta los cinco subcomandos federados con --all-workspaces y el shape del envelope; 09_contratos_tecnicos.md cita el contrato actualizado."
files:
  - modify: .docs/wiki/09_contratos/CT-NAV-WIKI.md
  - modify: .docs/wiki/09_contratos_tecnicos.md
  - read: .docs/wiki/09_contratos/CT-NAV-ASK.md   # referencia: cómo describe --all-workspaces
complexity: medium
done_when:
  - "CT-NAV-WIKI.md tiene 5 bloques toon ct-nav-wiki-<search|inventory|route|trace|pack>-contract"
  - "CT-NAV-WIKI.md tiene 1 bloque toon ct-nav-wiki-envelope-all-workspaces con campos workspace, host, workspaces_queried, workspaces_failed, truncated_per_workspace"
  - "Para inventory: documentar modo default (ligero) y --with-layer-counts opt-in"
  - "Para pack: documentar explícitamente 'N mini-packs, no super-pack mergeado'"
  - "09_contratos_tecnicos.md menciona la extensión"
  - "validate-source contra CT-NAV-WIKI.md returns ok=true"
evidence_expected:
  - "Diff de CT-NAV-WIKI.md mostrando los bloques nuevos"
  - "validate-source output"
stop_if:
  - "CT-NAV-WIKI.md no es SDD-HARNESS-v1 (governance repair antes de continuar)"
```

## Reference
- Archivo a extender: `.docs/wiki/09_contratos/CT-NAV-WIKI.md` (ya existe).
- Referencia de cómo documentar `--all-workspaces`: `.docs/wiki/09_contratos/CT-NAV-ASK.md` (ya tiene el flag).
- Índice: `.docs/wiki/09_contratos_tecnicos.md`.

## Prompt

Sos el ejecutor de T5 (ps-docs). Tu trabajo es extender un archivo existente, no crear uno nuevo.

1. Leer `.docs/wiki/09_contratos/CT-NAV-WIKI.md` completo.
2. Leer `.docs/wiki/09_contratos/CT-NAV-ASK.md` para ver cómo documentaron `--all-workspaces` ahí. Replicar terminología.
3. Modificar `CT-NAV-WIKI.md`:
   - **Actualizar `imports`** del frontmatter para incluir `RF-WIKI-001..005`.
   - **Agregar bloque toon `ct-nav-wiki-envelope-all-workspaces`**: describir los campos extras del envelope cuando `--all-workspaces=true`:
     ```toon
     block_id: ct-nav-wiki-envelope-all-workspaces
     extra_item_fields:
       workspace: "alias del workspace de origen (registry)"
       host: "opcional vacío; Hermes lo setea al mergear cross-host"
     extra_stats:
       workspaces_queried: "int >= 1"
       workspaces_failed: "array de {alias, reason}"
       truncated_per_workspace: "bool"
     backward_compat: "cuando --all-workspaces=false, el envelope es idéntico al actual"
     ```
   - **Para cada subcomando existente (search, route, trace, pack), agregar/actualizar un bloque `ct-nav-wiki-<subcmd>-contract`** con:
     - `flag: --all-workspaces (opcional)`
     - `flags_preserved: [--layer, --top, --offset, --include-content, etc.]` (los que ya tiene)
     - `envelope_extension: ct-nav-wiki-envelope-all-workspaces`
     - Para pack agregar `result_shape: N mini-packs por workspace, NO super-pack mergeado`.
   - **Agregar bloque nuevo `ct-nav-wiki-inventory-contract`** (subcomando NUEVO):
     ```toon
     block_id: ct-nav-wiki-inventory-contract
     subcommand: "nav wiki inventory"
     flag_all_workspaces: required_or_optional  # decidir: yo recomiendo "opcional, default = --all-workspaces=true" (porque single-workspace inventory es trivial)
     default_mode: "light"
     light_item_shape:
       - alias
       - root
       - wiki_root
       - governance_blocked
       - docs_ready
       - doc_count
       - last_indexed_at
     extended_flag: "--with-layer-counts"
     extended_item_shape_adds:
       - "layers: {RS, FL, RF, TP, TECH, DB, CT}"
     envelope_extension: ct-nav-wiki-envelope-all-workspaces
     ```
4. Actualizar `.docs/wiki/09_contratos_tecnicos.md`: si tiene una lista de contratos con resumen por línea, agregar/actualizar la fila de CT-NAV-WIKI mencionando la extensión `--all-workspaces` y el nuevo inventory.
5. Validar:
   ```powershell
   mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-NAV-WIKI.md --format toon
   mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-NAV-WIKI.md --format toon
   ```
6. Commit: `docs(wiki): extend CT-NAV-WIKI with --all-workspaces and inventory contract`.
7. Reportar diff.

## Execution Procedure
1. Leer CT-NAV-WIKI.md y CT-NAV-ASK.md.
2. Editar CT-NAV-WIKI.md con los bloques nuevos.
3. Actualizar índice 09_contratos_tecnicos.md.
4. Validar.
5. Commit.
6. Reportar.

## Skeleton

(Edit-style — agregar al archivo existente, no rewrite completo)

```markdown
## Envelope con --all-workspaces

```toon
block_id: ct-nav-wiki-envelope-all-workspaces
extra_item_fields:
  workspace: "..."
  host: "..."
extra_stats:
  workspaces_queried: "..."
  workspaces_failed: "..."
  truncated_per_workspace: "..."
```

## Subcomandos federados

```toon
block_id: ct-nav-wiki-search-contract
subcommand: "nav wiki search"
flag_all_workspaces: optional
...
```

(repetir para inventory, route, trace, pack)
```

## Verify
`mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/09_contratos/CT-NAV-WIKI.md --format toon` -> `ok=true` AND `rg -n "--all-workspaces" .docs/wiki/09_contratos/CT-NAV-WIKI.md | wc -l` >= 5

## Commit
`docs(wiki): extend CT-NAV-WIKI with --all-workspaces and inventory contract`
