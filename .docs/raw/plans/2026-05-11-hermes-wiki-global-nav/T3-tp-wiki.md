---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - TP-WIKI
allowed_paths:
  - .docs/wiki/06_matriz_pruebas_RF.md
  - .docs/wiki/06_pruebas/TP-WIKI.md
forbidden_paths:
  - .docs/wiki/03_FL/**
  - .docs/wiki/04_RF/**
  - .docs/wiki/07_tech/**
  - .docs/wiki/09_contratos/**
  - internal/**
verify:
  - test -f .docs/wiki/06_pruebas/TP-WIKI.md
  - rg -n "TP-WIKI" .docs/wiki/06_matriz_pruebas_RF.md -> match
  - mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/06_pruebas/TP-WIKI.md --format toon -> ok=true
stop_if:
  - RF-WIKI-001..005 no existen (T2 falló)
  - patrón canónico TP-QRY.md tiene shape incompatible con SDD-HARNESS-v1
secret_scan: clean
---

# Task T3: Crear TP-WIKI (matriz de pruebas para los cinco subcomandos federados)

## Shared Context
**Goal:** Documentar la matriz de pruebas para los cinco RF-WIKI-001..005 con casos verificables.
**Stack:** Markdown SDD con SDD-HARNESS-v1 + SDD-WIKI-SOURCE-v1.
**Architecture:** TPs viven en `.docs/wiki/06_pruebas/`. Índice en `.docs/wiki/06_matriz_pruebas_RF.md`. TP-WIKI cubre los cinco RFs; cada RF tiene al menos 4 test cases.

## Locked Decisions
- ID: `TP-WIKI` (siguiendo el patrón TP-QRY, TP-WKS, TP-IDX). No `TP-WIKI-001` por subcomando — todos viven en el mismo TP, segmentados por bloque toon.
- Cada RF (001-005) tiene su propio bloque toon dentro de TP-WIKI con sus test cases.
- Test cases nombrados `TC-WIKI-XXX` con numeración global creciente.
- Imports: `RF-WIKI-001..005`. Exports: `CT-NAV-WIKI` (referencia inversa).

## Task Metadata
```yaml
id: T3
depends_on: [T0, T2]
agent_type: ps-docs
goal_id: G1
github_issues: []
expected_outcome: "TP-WIKI.md existe con al menos 20 test cases (4 por RF) y 06_matriz_pruebas_RF.md tiene la entrada."
files:
  - create: .docs/wiki/06_pruebas/TP-WIKI.md
  - modify: .docs/wiki/06_matriz_pruebas_RF.md
  - read: .docs/wiki/06_pruebas/TP-QRY.md   # patrón
  - read: .docs/wiki/06_pruebas/TP-WKS.md   # patrón complementario
  - read: .docs/wiki/04_RF/RF-WIKI-001.md   # RFs base
  - read: .docs/wiki/04_RF/RF-WIKI-002.md
  - read: .docs/wiki/04_RF/RF-WIKI-003.md
  - read: .docs/wiki/04_RF/RF-WIKI-004.md
  - read: .docs/wiki/04_RF/RF-WIKI-005.md
complexity: medium
done_when:
  - "TP-WIKI.md tiene SDD-HARNESS-v1 frontmatter con id=TP-WIKI, kind=test-plan"
  - "TP-WIKI.md tiene cinco bloques toon (uno por RF) con block_id=tp-wiki-rf-001..005-cases"
  - "Total >= 20 test cases (TC-WIKI-001..020+)"
  - "Cada test case tiene id, rf, given/when/then verificables"
  - "06_matriz_pruebas_RF.md incluye TP-WIKI con sus cinco RF cubiertos"
  - "mi-lsp nav wiki validate-source contra TP-WIKI.md returns ok=true"
evidence_expected:
  - "Output de mi-lsp nav wiki validate-source --paths TP-WIKI.md"
  - "Diff de 06_matriz_pruebas_RF.md"
stop_if:
  - "alguno de los RF-WIKI-00X.md no existe (T2 incompleto)"
```

## Reference
- Patrón a seguir: `.docs/wiki/06_pruebas/TP-QRY.md` (matriz de pruebas para queries single-workspace). Replicar estructura, no contenido.
- Patrón complementario: `.docs/wiki/06_pruebas/TP-WKS.md` (tiene casos para workspace status que son similares a inventory en estilo).
- Índice: `.docs/wiki/06_matriz_pruebas_RF.md`.

## Prompt

Sos el ejecutor de T3 (ps-docs). Tu trabajo es crear UN archivo TP-WIKI con los test cases para los cinco RFs.

1. Leer TP-QRY.md y TP-WKS.md para entender estructura y estilo de test cases.
2. Leer los cinco RF-WIKI-001..005.md para extraer los criterios de aceptación.
3. Crear `.docs/wiki/06_pruebas/TP-WIKI.md` con:
   - Frontmatter SDD-HARNESS-v1: `id: TP-WIKI`, `kind: test-plan`, `audience: llm-first`, `imports: [RF-WIKI-001, RF-WIKI-002, RF-WIKI-003, RF-WIKI-004, RF-WIKI-005]`, exports a `CT-NAV-WIKI`.
   - `doc_id: TP-WIKI`.
   - Cinco bloques toon `block_id: tp-wiki-rf-001-cases` ... `tp-wiki-rf-005-cases`, cada uno con al menos 4 test cases.
4. **Test cases mínimos a incluir** (uno por bloque/RF como mínimo):

   Para **RF-WIKI-001 (search)**:
   - TC-WIKI-001: search "" sin --all-workspaces devuelve idéntico shape que single-workspace (back-compat).
   - TC-WIKI-002: search "governance" --all-workspaces devuelve items con `workspace<>''` por entrada.
   - TC-WIKI-003: search --all-workspaces con un workspace con governance_blocked=true sigue retornando ok=true; ese workspace aparece en items con flag o en stats.
   - TC-WIKI-004: search --all-workspaces con --top-global 10 limita correctamente el envelope final.

   Para **RF-WIKI-002 (inventory)**:
   - TC-WIKI-005: inventory --all-workspaces sin --with-layer-counts devuelve items con shape mínimo (alias/root/wiki_root/governance_blocked/docs_ready/doc_count/last_indexed_at) y SIN campo `layers`.
   - TC-WIKI-006: inventory --all-workspaces --with-layer-counts agrega el campo `layers` con conteos por capa.
   - TC-WIKI-007: inventory retorna `stats.workspaces_queried >= 1` y `workspaces_failed[]` (puede ser vacío).
   - TC-WIKI-008: inventory cubre tanto workspaces con docs_ready=true como false.

   Para **RF-WIKI-003 (route)**:
   - TC-WIKI-009: route "<query>" --all-workspaces devuelve N items con workspace cuando hay candidatos en múltiples wikis.
   - TC-WIKI-010: route --all-workspaces con cero matches devuelve items=[] y hint.
   - TC-WIKI-011: route --all-workspaces preserva flags actuales (--top, --tier).
   - TC-WIKI-012: route --all-workspaces no introduce ambigüedad cuando un mismo task slug aparece en dos wikis.

   Para **RF-WIKI-004 (trace)**:
   - TC-WIKI-013: trace --all-workspaces agrupa items por workspace; no fusiona trazas cross-wiki.
   - TC-WIKI-014: trace --all --all-workspaces vuelca todas las trazas RF de todos los workspaces.
   - TC-WIKI-015: trace --summary --all-workspaces respeta el modo summary per-workspace.
   - TC-WIKI-016: trace --all-workspaces con un workspace sin TPs aún incluye el workspace en stats.

   Para **RF-WIKI-005 (pack)**:
   - TC-WIKI-017: pack --all-workspaces devuelve N mini-packs (uno por workspace), no un super-pack global.
   - TC-WIKI-018: pack --all-workspaces con --rf <id> aplica el filtro per-workspace; workspaces sin ese RF entran a items=[] o se omiten con hint.
   - TC-WIKI-019: pack --all-workspaces respeta --doc.
   - TC-WIKI-020: pack --all-workspaces marca cada mini-pack con su workspace y su doc_count local.

5. Actualizar `.docs/wiki/06_matriz_pruebas_RF.md` agregando una fila TP-WIKI con sus cinco RFs cubiertos.
6. Validar:
   ```powershell
   mi-lsp nav wiki validate-harness --workspace mi-lsp --paths .docs/wiki/06_pruebas/TP-WIKI.md --format toon
   mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/06_pruebas/TP-WIKI.md --format toon
   ```
7. Commitear: `docs(wiki): add TP-WIKI test plan for federated nav wiki subcommands`.
8. Reportar diff al orquestador.

## Execution Procedure
1. Leer TP-QRY.md, TP-WKS.md, los cinco RFs.
2. Crear TP-WIKI.md.
3. Actualizar índice 06_matriz_pruebas_RF.md.
4. Validar harness + source.
5. Commit.
6. Reportar.

## Skeleton

```markdown
---
harness_protocol: SDD-HARNESS-v1
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: TP-WIKI
doc_id: TP-WIKI
kind: test-plan
audience: llm-first
imports: [RF-WIKI-001, RF-WIKI-002, RF-WIKI-003, RF-WIKI-004, RF-WIKI-005]
exports: [CT-NAV-WIKI]
agent_must_read: [...]
agent_may_edit: false
agent_must_not_edit: [...]
verify: [...]
stop_if: [...]
evidence: [...]
---

# TP-WIKI — Matriz de pruebas para subcomandos wiki federados

```toon
block_id: tp-wiki-rf-001-cases
rf: RF-WIKI-001
cases:
  - id: TC-WIKI-001
    given: "registry con N workspaces docs_ready=true; flag --all-workspaces ausente"
    when: "mi-lsp nav wiki search 'governance' --workspace mi-lsp --format toon"
    then: "envelope shape idéntico al actual single-workspace; stats.workspaces_queried no presente o = 1"
  - id: TC-WIKI-002
    given: "registry con >= 3 workspaces docs_ready=true"
    when: "mi-lsp nav wiki search 'governance' --all-workspaces --format toon"
    then: "items[N>0] cada uno con workspace<>''; stats.workspaces_queried >= 3"
  - ...
```

```toon
block_id: tp-wiki-rf-002-cases
rf: RF-WIKI-002
cases:
  - id: TC-WIKI-005
    ...
```

(continuar para 003, 004, 005)
```

## Verify
`mi-lsp nav wiki validate-source --workspace mi-lsp --paths .docs/wiki/06_pruebas/TP-WIKI.md --format toon` -> `ok=true` AND `rg -n "TC-WIKI-0[12][0-9]" .docs/wiki/06_pruebas/TP-WIKI.md | wc -l` >= 20

## Commit
`docs(wiki): add TP-WIKI test plan for federated nav wiki subcommands`
