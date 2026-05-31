<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md"]
allowed_paths: [".docs/wiki/03_FL.md",".docs/wiki/04_RF.md",".docs/wiki/04_RF/**",".docs/wiki/06_matriz_pruebas_RF.md"]
forbidden_paths: [".docs/wiki/00_gobierno_documental.md",".docs/wiki/_mi-lsp/read-model.toml",".git/**"]
verify: ["grep -n \"RF-SEM-001\" .docs/wiki/04_RF.md"]
stop_if: ["the RF index format differs from described"]
secret_scan: "none"
-->

# Task D3: Functional canon sync — 03_FL, 04_RF, 06 test matrix

## Shared Context
**Goal:** Add functional traceability for semantic recall: a flow entry, a small RF family, and test-matrix rows.
**Stack:** Markdown with frontmatter; index tables that already exist.
**Architecture:** 03_FL = flows, 04_RF = requirements (one row per RF in the index table), 06 = RF↔test matrix.

## Locked Decisions
- New RF family `RF-SEM-*` under a new flow `FL-SEM-01` (semantic knowledge-wiki recall). recall is ungated; knowledge-wiki profile; pluggable embeddings.
- Only touch the listed files (index-level rows; do NOT author full per-RF subdocs unless the repo requires individual files — if `04_RF/` requires one file per RF, create minimal `RF-SEM-00X.md` stubs matching a sibling). Do NOT run git.

## Task Metadata
```yaml
id: D3
depends_on: [T3]
agent_type: ps-worker
goal_id: G3
expected_outcome: "FL/RF/TP canon references semantic recall with consistent IDs."
files:
  - modify: .docs/wiki/03_FL.md
  - modify: .docs/wiki/04_RF.md
  - modify: .docs/wiki/06_matriz_pruebas_RF.md
  - create: .docs/wiki/04_RF/RF-SEM-001.md   # only if 04_RF/ uses one-file-per-RF; else skip
  - create: .docs/wiki/04_RF/RF-SEM-002.md   # idem
  - create: .docs/wiki/04_RF/RF-SEM-003.md   # idem
complexity: medium
done_when:
  - "04_RF.md catalog table has RF-SEM-001..003 rows mapped to FL-SEM-01"
  - "03_FL.md lists FL-SEM-01"
  - "06 matrix references the RF-SEM family"
evidence_expected:
  - "diffs + any new RF files"
stop_if:
  - "the RF index format differs (adapt to the actual table columns)"
```

## Reference
- `04_RF.md` catalog table columns: `ID | Titulo | Actor principal | FL origen | Estado | TP asociado`.
- Read `03_FL.md` for the flow entry format and an existing `04_RF/RF-*.md` for the per-RF doc shape (only if individual files are required).

## Prompt
1. `03_FL.md`: add `FL-SEM-01` — "Recall semantico de wikis de conocimiento" (actor: Usuario/Skill/Agente). Match the existing flow-entry format.
2. `04_RF.md`: add catalog rows:
   - `RF-SEM-001 | Configurar backend de embeddings pluggable OpenAI-compatible por workspace | Desarrollador / Skill | FL-SEM-01 | ready | TP-SEM`
   - `RF-SEM-002 | Indexar y embeber semanticamente chunks de markdown por heading, incremental por hash | Desarrollador / Skill | FL-SEM-01 | ready | TP-SEM`
   - `RF-SEM-003 | Recuperar secciones por significado con nav recall (ungated, knowledge-wiki) | Usuario / Skill / Agente | FL-SEM-01 | ready | TP-SEM`
3. If `04_RF/` uses one file per RF (check: do RF-QRY-001.md etc. exist?), create minimal `RF-SEM-001.md`, `RF-SEM-002.md`, `RF-SEM-003.md` mirroring a sibling's frontmatter + sections. If `04_RF/` is empty or aggregated, SKIP file creation and only update the index table.
4. `06_matriz_pruebas_RF.md`: add a `TP-SEM` row/section covering RF-SEM-001..003 (bilingual recall ES→EN, knowledge-wiki without governance, offline⇒lexical), pointing at `internal/service/recall_test.go` and `internal/embed`/`internal/wikichunk` tests as evidence. Match the matrix format.
5. Run `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon`; report. Do NOT run git.

## Execution Procedure
1. Read `03_FL.md`, `04_RF.md`, `06_matriz_pruebas_RF.md`, and one `04_RF/RF-*.md` (to decide if per-RF files are needed).
2. Apply index edits; create RF-SEM files only if the per-file convention is in use.
3. Run validate-harness; report diffs + new files + output.

## Verify
`grep -n "RF-SEM-001" .docs/wiki/04_RF.md` returns ≥1; `grep -n "FL-SEM-01" .docs/wiki/03_FL.md` returns ≥1.

## Commit
(orchestrator commits) — suggested: `docs(spec): add FL-SEM-01 + RF-SEM family + TP-SEM for semantic recall`
