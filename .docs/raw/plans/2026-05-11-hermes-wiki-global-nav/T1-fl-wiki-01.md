---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - FL-WIKI-01
allowed_paths:
  - .docs/wiki/03_FL.md
  - .docs/wiki/03_FL/FL-WIKI-01.md
forbidden_paths:
  - .docs/wiki/04_RF/**
  - .docs/wiki/06_pruebas/**
  - .docs/wiki/07_tech/**
  - .docs/wiki/09_contratos/**
  - internal/**
verify:
  - test -f .docs/wiki/03_FL/FL-WIKI-01.md
  - rg -n "FL-WIKI-01" .docs/wiki/03_FL.md -> match
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/03_FL/FL-WIKI-01.md --format toon -> ok=true
stop_if:
  - .docs/wiki/03_FL/FL-WIKI-01.md already exists with conflicting content
  - existing FL-* docs use a different harness contract version (SDD-HARNESS-v1 not present)
secret_scan: clean
---

# Task T1: Crear FL-WIKI-01 (federación wiki global cross-workspace)

## Shared Context
**Goal:** Documentar el flujo funcional FL-WIKI-01: cómo una consulta wiki se federa cross-workspace dentro de una máquina, y cómo Hermes la extiende cross-máquina vía SSH/Tailscale.
**Stack:** Markdown SDD con SDD-HARNESS-v1 + SDD-WIKI-SOURCE-v1 (fenced toon blocks, doc_id, block_id).
**Architecture:** mi-lsp profile = ordered_wiki + spec_backend. FL docs viven en `.docs/wiki/03_FL/`. Índice de FLs en `.docs/wiki/03_FL.md`.

## Locked Decisions
- ID: `FL-WIKI-01`. No usar `FL-QRY-02` ni otro dominio.
- El FL describe el flujo end-to-end pero **NO** entra al detalle del envelope (eso vive en CT-NAV-WIKI) ni al detalle del scoring (eso vive en TECH-WIKI-FANOUT).
- Audience: `llm-first`. Toda decisión normativa va en bloques `toon` cercados; prosa solo para narrar el flujo y los anchors.
- Imports: `FL-QRY-01` (consulta single-workspace, base), `RF-WIKI-001..005` (los cinco RF que materializan).
- Exports: anchors para `TP-WIKI`, `TECH-WIKI-FANOUT`, `CT-NAV-WIKI`.

## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: ps-docs
goal_id: G1
github_issues: []
expected_outcome: "FL-WIKI-01.md existe en .docs/wiki/03_FL/ con harness contract y wiki source blocks válidos; 03_FL.md lista el nuevo flow."
files:
  - create: .docs/wiki/03_FL/FL-WIKI-01.md
  - modify: .docs/wiki/03_FL.md  # agregar entrada al índice
  - read: .docs/wiki/03_FL/FL-QRY-01.md   # patrón de referencia
  - read: .docs/wiki/03_FL.md             # estructura del índice
complexity: medium
done_when:
  - "FL-WIKI-01.md contiene SDD-HARNESS-v1 frontmatter con id=FL-WIKI-01, kind=flow, audience=llm-first"
  - "FL-WIKI-01.md contiene al menos un bloque toon con block_id=fl-wiki-01-overview"
  - "03_FL.md tiene una nueva fila/entrada apuntando a FL-WIKI-01"
  - "mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/03_FL/FL-WIKI-01.md returns ok=true"
evidence_expected:
  - "Output de mi-lsp nav wiki validate-harness y validate-source contra FL-WIKI-01"
  - "Diff capturado del cambio en 03_FL.md"
stop_if:
  - "FL-QRY-01.md no existe o tiene shape distinto a SDD-HARNESS-v1 (reportar para repair de governance antes de continuar)"
```

## Reference
- Patrón a seguir: `.docs/wiki/03_FL/FL-QRY-01.md` (consulta single-workspace). Replicar la estructura de secciones, harness contract y bloques toon. **No copiar contenido** — solo la arquitectura del documento.
- Índice: `.docs/wiki/03_FL.md`. Mirar las filas existentes y respetar el orden alfabético / por dominio.
- Plan principal: `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md` (referencia para alcance del flujo).

## Prompt

Sos el ejecutor de T1 (ps-docs). Tu trabajo es crear UN archivo nuevo de flujo SDD y actualizar el índice. No tocar código ni otros docs.

1. Leer `.docs/wiki/03_FL/FL-QRY-01.md` completo para entender la estructura del FL canónico en este repo.
2. Leer `.docs/wiki/03_FL.md` completo para entender el shape del índice.
3. Leer el plan principal (`.docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md`) para extraer el alcance del flujo.
4. Crear `.docs/wiki/03_FL/FL-WIKI-01.md` siguiendo exactamente la estructura del FL-QRY-01.md pero con contenido propio. Debe incluir:
   - **Frontmatter YAML con SDD-HARNESS-v1**: `id: FL-WIKI-01`, `kind: flow`, `audience: llm-first`, `imports: [FL-QRY-01]`, `exports: [TP-WIKI, TECH-WIKI-FANOUT, CT-NAV-WIKI]`, `agent_must_read`, `agent_may_edit`, `agent_must_not_edit`, `verify`, `stop_if`, `evidence`.
   - **doc_id**: `FL-WIKI-01`.
   - **Bloque toon `block_id: fl-wiki-01-overview`** con el flujo en pasos (entrada del agente → fan-out intra-máquina → merge en cliente → opcional fan-out cross-máquina vía Hermes/SSH).
   - **Bloque toon `block_id: fl-wiki-01-actors`** listando: cliente CLI, registry local, daemon (opcional), Hermes, hosts Tailscale.
   - **Bloque toon `block_id: fl-wiki-01-invariants`**: mi-lsp NO sabe de hosts; merge cross-máquina vive en Hermes; envelope `--all-workspaces` lleva `workspace` por item y `host:""` opcional; fallo de workspace NO aborta el fan-out.
   - **Sección de Obsidian-style links** a RF-WIKI-001..005, TP-WIKI, TECH-WIKI-FANOUT, CT-NAV-WIKI.
   - **No usar Markdown tables para contenido normativo**. Si necesitás tablas, marcarlas como "comparison" o "historical snapshot" con nota explícita.
5. Actualizar `.docs/wiki/03_FL.md` para incluir una fila/entrada de FL-WIKI-01 siguiendo el formato existente. **No reordenar las demás entradas.**
6. Ejecutar `mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/03_FL/FL-WIKI-01.md --format toon` y verificar `ok=true`.
7. Ejecutar `mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/03_FL/FL-WIKI-01.md --format toon` y verificar `ok=true`.
8. Commitear: `docs(wiki): add FL-WIKI-01 federation flow for global wiki nav`.
9. Reportar diff de ambos archivos al orquestador.

**No hacer:**
- No reordenar otros FLs.
- No tocar `.docs/wiki/02_arquitectura.md` (eso es T6).
- No describir el envelope ni el scoring (eso vive en CT/TECH).

## Execution Procedure
1. Leer FL-QRY-01.md y 03_FL.md.
2. Crear FL-WIKI-01.md con la estructura listada arriba.
3. Editar 03_FL.md insertando la entrada nueva.
4. Validar con `mi-lsp nav wiki validate-harness` y `validate-source`.
5. `git add` y commit semantic.
6. Reportar.

## Skeleton

```markdown
---
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: FL-WIKI-01
doc_id: FL-WIKI-01
kind: flow
audience: llm-first
imports:
  - FL-QRY-01
exports:
  - TP-WIKI
  - TECH-WIKI-FANOUT
  - CT-NAV-WIKI
agent_must_read: [...]
agent_may_edit: false
agent_must_not_edit: [...]
verify: [...]
stop_if: [...]
evidence: [...]
---

# FL-WIKI-01 — Federación wiki global cross-workspace

## Overview

Flujo de federación de consultas wiki cross-workspace dentro de una máquina,
con extensión opcional cross-máquina orquestada por un cliente externo (Hermes).

```toon
block_id: fl-wiki-01-overview
steps:
  - "cliente CLI invoca `mi-lsp nav wiki <op> ... --all-workspaces`"
  - "mi-lsp itera workspaces del registry con semaphore=4"
  - "..."
```

## Actors

```toon
block_id: fl-wiki-01-actors
actors:
  - { name: cli, role: entry-point }
  - { name: registry, role: workspace inventory }
  - ...
```

## Invariants

```toon
block_id: fl-wiki-01-invariants
invariants:
  - "mi-lsp no conoce hosts ni transporte de red"
  - "merge cross-máquina ocurre en Hermes; mi-lsp es CLI-puro per-máquina"
  - "fallo de workspace -> entra en workspaces_failed[], NO aborta el fan-out"
```

## Links
- [[RF-WIKI-001]] (search federada)
- [[RF-WIKI-002]] (inventory federada)
- ...
- [[TECH-WIKI-FANOUT]]
- [[CT-NAV-WIKI]]
```

## Verify
`mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/03_FL/FL-WIKI-01.md --format toon` -> `ok=true` AND `mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/03_FL/FL-WIKI-01.md --format toon` -> `ok=true`

## Commit
`docs(wiki): add FL-WIKI-01 federation flow for global wiki nav`
