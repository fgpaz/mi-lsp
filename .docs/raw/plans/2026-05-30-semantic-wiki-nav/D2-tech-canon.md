<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md"]
allowed_paths: [".docs/wiki/07_baseline_tecnica.md",".docs/wiki/07_tech/**",".docs/wiki/08_modelo_fisico_datos.md",".docs/wiki/08_db/**",".docs/wiki/09_contratos_tecnicos.md",".docs/wiki/09_contratos/**"]
forbidden_paths: [".docs/wiki/00_gobierno_documental.md",".docs/wiki/_mi-lsp/read-model.toml",".git/**"]
verify: ["mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon"]
stop_if: ["frontmatter shape cannot be matched from a sibling"]
secret_scan: "none"
-->

# Task D2: Technical canon sync — 07/TECH, 08/DB, 09/CT

## Shared Context
**Goal:** Sync the technical wiki canon for the new semantic recall path: technical baseline + a TECH detail doc, physical data model + a DB detail doc, technical contracts + a CT detail doc.
**Stack:** Markdown with `SDD-HARNESS-v1` YAML frontmatter (copy the shape from a sibling doc).
**Architecture:** Root 07/08/09 stay short/canonical; detail lives in `07_tech/TECH-*`, `08_db/DB-*`, `09_contratos/CT-*`.

## Locked Decisions
- New surface: `nav recall` command, `[embeddings]` config, `wiki_chunk_embeddings` BLOB table, pure-Go cosine, knowledge-wiki profile, offline⇒lexical.
- Only touch the listed wiki files. Do NOT edit `00_gobierno_documental.md` or `_mi-lsp/read-model.toml`. Do NOT run git.

## Task Metadata
```yaml
id: D2
depends_on: [T3]
agent_type: ps-worker
goal_id: G3
expected_outcome: "07/08/09 root docs + new TECH/DB/CT detail docs describe the embeddings backend, vector store, and recall contract."
files:
  - modify: .docs/wiki/07_baseline_tecnica.md
  - create: .docs/wiki/07_tech/TECH-SEMANTIC-RECALL.md
  - modify: .docs/wiki/08_modelo_fisico_datos.md
  - create: .docs/wiki/08_db/DB-WIKI-EMBEDDINGS.md
  - modify: .docs/wiki/09_contratos_tecnicos.md
  - create: .docs/wiki/09_contratos/CT-NAV-RECALL.md
complexity: medium
done_when:
  - "07 lists a semantic recall component and links TECH-SEMANTIC-RECALL"
  - "08 documents wiki_chunk_embeddings and links DB-WIKI-EMBEDDINGS"
  - "09 lists nav recall + [embeddings] surface and links CT-NAV-RECALL"
  - "mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon does not regress (new docs carry valid frontmatter)"
evidence_expected:
  - "diffs + new files; validate-harness output"
stop_if:
  - "frontmatter shape cannot be matched from a sibling (report)"
```

## Reference
- Frontmatter + section style: copy from `.docs/wiki/07_tech/TECH-GOVERNANCE-PROFILES.md`, `.docs/wiki/08_db/DB-*` (pick one), `.docs/wiki/09_contratos/CT-NAV-ASK.md`.
- Design doc: `.docs/raw/plans/2026-05-30-semantic-wiki-nav-design.md` (table schema, envelope, config).
- Known validator false positives (per repo memory): cross-kind imports and evidence paths to Go files or `path:N-M` ranges may warn — keep frontmatter conservative (mirror a passing sibling exactly).

## Prompt
1. `TECH-SEMANTIC-RECALL.md` (new, `07_tech/`): purpose = pluggable OpenAI-compatible embeddings backend + semantic recall over markdown wikis. Document: pure-Go vector store (BLOB + Go cosine, NO CGO, why sqlite-vec was rejected), `internal/embed` + `internal/wikichunk` packages, post-publish embedding hook in the index job, incremental re-embedding by content hash, knowledge-wiki vs spec-driven profile, offline⇒lexical degradation, `[embeddings]` config + `MI_LSP_EMBEDDINGS_API_KEY` via mkey. Copy the harness frontmatter from TECH-GOVERNANCE-PROFILES.md (change id/exports/evidence to TECH-SEMANTIC-RECALL).
2. `07_baseline_tecnica.md`: add a component row ("Semantic recall / embeddings backend" — Subsistema Go — Query/runtime — semantic wiki recall via embeddings), add 2-3 invariants (pure-Go vector store; recall ungated; offline⇒lexical), and add `[TECH-SEMANTIC-RECALL.md](07_tech/TECH-SEMANTIC-RECALL.md)` to "Documentos detalle". Add a change trigger line for embeddings/recall.
3. `DB-WIKI-EMBEDDINGS.md` (new, `08_db/`): document `wiki_chunk_embeddings` (columns, types, UNIQUE(doc_path,chunk_id), index, BLOB float32 LE encoding, content_hash incrementality, lazy CREATE-IF-NOT-EXISTS migration). Copy frontmatter from a sibling DB doc.
4. `08_modelo_fisico_datos.md`: add `wiki_chunk_embeddings` to the repo-local table inventory and link DB-WIKI-EMBEDDINGS.
5. `CT-NAV-RECALL.md` (new, `09_contratos/`): document `mi-lsp nav recall <query>` — flags (`--workspace`, `--max-items`, `--token-budget`, `--format`, `--map`), ungated (no governance gate), `RecallResult` envelope shape ({archivo, heading, score, snippet, start_line}), backends `recall` / `recall+lexical`, hint when unconfigured, profile reporting in `workspace status` (`embeddings_enabled`, `recall_profile`). Copy frontmatter from CT-NAV-ASK.md.
6. `09_contratos_tecnicos.md`: add `nav recall` to the public CLI surface bullets + "Operaciones adicionales", note it is ungated, and link `[CT-NAV-RECALL.md](09_contratos/CT-NAV-RECALL.md)` in "Documentos detalle". Add an `[embeddings]` config + `MI_LSP_EMBEDDINGS_API_KEY` env mention. Add a change trigger.
7. Run `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon` and report; do NOT run git.

## Execution Procedure
1. Read the reference sibling docs to copy exact frontmatter/section conventions.
2. Create the 3 new docs, then edit the 3 root docs.
3. Run validate-harness; ensure no NEW BLOCKED beyond known false positives.
4. Report diffs + new file paths + validate-harness output.

## Verify
`mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon` → no new BLOCKED attributable to these docs.

## Commit
(orchestrator commits) — suggested: `docs(tech): sync 07/08/09 + TECH/DB/CT for semantic recall`
