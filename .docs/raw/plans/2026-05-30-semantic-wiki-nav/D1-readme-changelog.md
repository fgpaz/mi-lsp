<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md"]
allowed_paths: ["README.md","CHANGELOG.md"]
forbidden_paths: [".git/**","worker-dotnet/**",".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["grep -n \"nav recall\" README.md"]
stop_if: ["README/CHANGELOG structure differs from described"]
secret_scan: "none"
-->

# Task D1: Docs sync — README.md + CHANGELOG.md

## Shared Context
**Goal:** Document the new `nav recall` command, the `[embeddings]` config block, and the knowledge-wiki profile in the public-facing README and CHANGELOG.
**Stack:** Markdown.
**Architecture:** README is the public entrypoint; CHANGELOG follows Keep a Changelog.

## Locked Decisions
- Command: `mi-lsp nav recall <query>` (ungated). Config: `[embeddings]` in `.mi-lsp/project.toml`. Profile: knowledge-wiki auto-detected when no `00_gobierno_documental.md`.
- Secret via env var `MI_LSP_EMBEDDINGS_API_KEY`, populated by `mkey run`. tesla bge-m3 is the documented REFERENCE example, not a baked default.
- Only touch README.md and CHANGELOG.md. Do NOT run git.

## Task Metadata
```yaml
id: D1
depends_on: [T3]
agent_type: ps-worker
goal_id: G3
expected_outcome: "README and CHANGELOG describe nav recall, [embeddings], and the knowledge-wiki profile."
files:
  - modify: README.md
  - modify: CHANGELOG.md
complexity: low
done_when:
  - "README mentions 'nav recall' and an [embeddings] config example"
  - "CHANGELOG [Unreleased] Added section lists nav recall + embeddings backend"
evidence_expected:
  - "git diff of README.md and CHANGELOG.md (do not commit)"
stop_if:
  - "README/CHANGELOG structure differs from what is described (adapt, do not invent a new file)"
```

## Reference
README.md current sections: "Use the right command for the job" table (~line 113), "Docs-First Search" (~144), "Core Capabilities" (~226), "Current v0.1.0 Scope" with "Out of scope" listing "Embeddings or remote semantic search services" (~328). CHANGELOG.md `## [Unreleased]` (line 8).

## Prompt
1. README.md:
   - Add a row to the "You want to... / Run this" table: `| Find a wiki note by meaning (multilingual) | mi-lsp nav recall "<query>" --workspace myapp |`.
   - Add a short subsection "Semantic recall over knowledge wikis" after "Docs-First Search": explain `nav recall` embeds the query and ranks markdown sections by cosine similarity (multilingual, e.g. a Spanish query finds an English note); it is ungated and works on a knowledge-wiki without `00_gobierno_documental.md`. Show the `[embeddings]` config block (provider, base_url, model, dim, api_key_env, profile) with tesla bge-m3 as the reference example, and note the key is injected via `mkey run` / `MI_LSP_EMBEDDINGS_API_KEY` and is never committed. Mention offline ⇒ degrades to lexical.
   - Add `nav ... recall` to the `mi-lsp nav ...` line in "Core Capabilities".
   - In "Out of scope", REMOVE or amend the "Embeddings or remote semantic search services" bullet to reflect that pluggable embeddings recall is now IN scope (move it to scope or delete the out-of-scope line).
2. CHANGELOG.md: under `## [Unreleased]`, add an `### Added` section listing: `nav recall` semantic search over markdown knowledge wikis; pluggable OpenAI-compatible `[embeddings]` backend (reference: tesla bge-m3); `wiki_chunk_embeddings` repo-local vector store with incremental re-embedding by content hash; `knowledge-wiki` profile that bypasses the spec-driven governance gate; offline ⇒ lexical fallback.
3. Do NOT run git. Report the diffs.

## Execution Procedure
1. Read README.md and CHANGELOG.md.
2. Apply the edits above, matching existing tone/format.
3. Report `git --no-pager diff -- README.md CHANGELOG.md` (read-only diff view; do not commit).

## Verify
`grep -n "nav recall" README.md` returns ≥1; `grep -n "recall" CHANGELOG.md` returns ≥1.

## Commit
(orchestrator commits) — suggested: `docs: document nav recall + [embeddings] + knowledge-wiki profile`
