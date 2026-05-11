---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - TECH-WIKI-FANOUT
allowed_paths:
  - .docs/wiki/07_baseline_tecnica.md
  - .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
forbidden_paths:
  - .docs/wiki/03_FL/**
  - .docs/wiki/04_RF/**
  - .docs/wiki/06_pruebas/**
  - .docs/wiki/09_contratos/**
  - internal/**
verify:
  - test -f .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
  - rg -n "TECH-WIKI-FANOUT" .docs/wiki/07_baseline_tecnica.md -> match
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-WIKI-FANOUT.md --format toon -> ok=true
stop_if:
  - patrón TECH-AXI-DISCOVERY.md o TECH-DAEMON-GOBERNANZA.md incompatible con SDD-HARNESS-v1
secret_scan: clean
---

# Task T4: Crear TECH-WIKI-FANOUT (arquitectura técnica del fan-out wiki)

## Shared Context
**Goal:** Documentar la arquitectura técnica del fan-out: reuse del patrón `AllWorkspaces` con semaphore=4, política de timeout/fallo, shape del envelope, scoring, cache, invariantes.
**Stack:** Markdown SDD con SDD-HARNESS-v1 + SDD-WIKI-SOURCE-v1.
**Architecture:** TECH-* docs viven en `.docs/wiki/07_tech/`. Resumen en `.docs/wiki/07_baseline_tecnica.md`.

## Locked Decisions
- ID: `TECH-WIKI-FANOUT`. Audience `llm-first`.
- Imports: `FL-WIKI-01`. Exports: `CT-NAV-WIKI`.
- Documenta arquitectura: semaphore=4, timeout=30s per workspace, política "no aborta", score cross-workspace, dónde vive el merge (cliente, no servidor), referencia a internal/service/ask.go como patrón.
- NO documentar contratos exactos (eso es CT). NO documentar test cases (eso es TP).

## Task Metadata
```yaml
id: T4
depends_on: [T0, T1]
agent_type: ps-docs
goal_id: G1
github_issues: []
expected_outcome: "TECH-WIKI-FANOUT.md existe con bloques toon normativos y 07_baseline_tecnica.md tiene la sección y el link."
files:
  - create: .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
  - modify: .docs/wiki/07_baseline_tecnica.md
  - read: .docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md  # patrón
  - read: .docs/wiki/07_tech/TECH-AXI-DISCOVERY.md      # patrón complementario
  - read: .docs/wiki/07_baseline_tecnica.md             # estructura
complexity: medium
done_when:
  - "TECH-WIKI-FANOUT.md tiene harness frontmatter con id=TECH-WIKI-FANOUT, kind=tech-spec"
  - "Bloques toon: tech-wiki-fanout-architecture, tech-wiki-fanout-failure-policy, tech-wiki-fanout-envelope-shape, tech-wiki-fanout-reuse-pattern"
  - "07_baseline_tecnica.md tiene una nueva sección 'Fan-out de comandos wiki' que enlaza a TECH-WIKI-FANOUT"
  - "mi-lsp nav wiki validate-source returns ok=true contra TECH-WIKI-FANOUT.md"
evidence_expected:
  - "Output de validate-harness y validate-source"
  - "Diff de 07_baseline_tecnica.md"
stop_if:
  - "FL-WIKI-01.md no existe (T1 falló)"
```

## Reference
- Patrón: `.docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md` (TECH spec docs governance/runtime).
- Patrón secundario: `.docs/wiki/07_tech/TECH-AXI-DISCOVERY.md`.
- Código a referenciar (NO leer en profundidad — solo citar paths): `internal/service/ask.go` (líneas 465-564 donde vive AllWorkspaces con semaphore=4).

## Prompt

Sos el ejecutor de T4 (ps-docs). Tu trabajo es crear UN archivo TECH y actualizar el baseline.

1. Leer TECH-DAEMON-GOBERNANZA.md y TECH-AXI-DISCOVERY.md para entender estructura.
2. Leer 07_baseline_tecnica.md.
3. Crear `.docs/wiki/07_tech/TECH-WIKI-FANOUT.md` con:
   - Frontmatter SDD-HARNESS-v1: `id: TECH-WIKI-FANOUT`, `kind: tech-spec`, `audience: llm-first`, `imports: [FL-WIKI-01]`, `exports: [CT-NAV-WIKI]`.
   - `doc_id: TECH-WIKI-FANOUT`.
   - Bloque `tech-wiki-fanout-architecture`: describir el iterator que recorre `workspace list`, semaphore=4, paralelismo bounded, timeout=30s per workspace (default heredado de `nav ask`).
   - Bloque `tech-wiki-fanout-failure-policy`: workspace que falla -> entra a `workspaces_failed[]`; el fan-out global NO aborta; se devuelve `ok=true` con stats parciales.
   - Bloque `tech-wiki-fanout-envelope-shape`: enumerar los campos agregados al envelope cuando `--all-workspaces=true`: `items[].workspace`, `items[].host` opcional vacío, `stats.workspaces_queried`, `stats.workspaces_failed[]`, `stats.truncated_per_workspace`.
   - Bloque `tech-wiki-fanout-reuse-pattern`: referenciar `internal/service/ask.go:465-564` como fuente del patrón; documentar que el nuevo helper vive en `internal/nav/fanout_wiki.go` y reusa la misma política sin duplicar.
   - Bloque `tech-wiki-fanout-scoring`: score cross-workspace = score per-workspace devuelto por el index local (FTS5 ranking estable); merge en cliente ordena por `(score DESC, workspace ASC, doc_id ASC)`.
   - Bloque `tech-wiki-fanout-non-goals`: mi-lsp NO sabe de hosts/SSH/Tailscale; cross-máquina vive en Hermes wrapper; no hay cache distribuido.
4. Actualizar `.docs/wiki/07_baseline_tecnica.md`: agregar una nueva sección "Fan-out de comandos wiki" con uno o dos párrafos cortos y un link `[[TECH-WIKI-FANOUT]]`. Insertar respetando el orden actual del baseline.
5. Validar:
   ```powershell
   mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-WIKI-FANOUT.md --format toon
   mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-WIKI-FANOUT.md --format toon
   ```
6. Commit: `docs(wiki): add TECH-WIKI-FANOUT for federated nav wiki architecture`.
7. Reportar diff.

## Execution Procedure
1. Leer los dos TECHs de referencia y el baseline.
2. Crear TECH-WIKI-FANOUT.md.
3. Actualizar 07_baseline_tecnica.md.
4. Validar.
5. Commit.
6. Reportar.

## Skeleton

```markdown
---
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: TECH-WIKI-FANOUT
doc_id: TECH-WIKI-FANOUT
kind: tech-spec
audience: llm-first
imports: [FL-WIKI-01]
exports: [CT-NAV-WIKI]
agent_must_read: [...]
agent_may_edit: false
agent_must_not_edit: [...]
verify: [...]
stop_if: [...]
evidence: [...]
---

# TECH-WIKI-FANOUT — Arquitectura técnica del fan-out wiki

```toon
block_id: tech-wiki-fanout-architecture
iterator: "workspace list (registry.toml)"
semaphore: 4
timeout_per_workspace: "30s"
parallel: true
```

```toon
block_id: tech-wiki-fanout-failure-policy
on_workspace_failure: "register in stats.workspaces_failed[]; continue"
abort_global: false
return_partial: true
```

(continuar con los demás bloques)
```

## Verify
`mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/07_tech/TECH-WIKI-FANOUT.md --format toon` -> `ok=true`

## Commit
`docs(wiki): add TECH-WIKI-FANOUT for federated nav wiki architecture`
