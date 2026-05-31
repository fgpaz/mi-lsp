# Exploration findings — semantic-wiki-nav

Source: Workflow `semantic-wiki-explore` (run wf_248d2e19-6f3), 5 read-only Explore agents, 142 tool-uses. 2026-05-30.
Baseline `go test ./...` = green (EXIT=0) before any change.

## Decisive constraint
`go.mod:13` uses `modernc.org/sqlite v1.37.1` = **pure-Go SQLite, NO CGO**. `Makefile:25-37` and `scripts/release/build-dist.ps1:39-42` cross-compile the 4 RIDs with plain `GOOS`/`GOARCH` and zero CGO. → A C extension like `sqlite-vec`/`mattn/go-sqlite3` WOULD BREAK the cross-compile. Vector store MUST stay pure-Go.

## 1. Index pipeline (where embeddings hook in)
- `internal/indexer/indexer.go:45 IndexWorkspaceWithProgress` — entry: `buildCatalog()` (symbols/files) → `docgraph.IndexWorkspaceDocsWithSourcesWithProgress()` (docs) → `store.ReplaceWorkspaceIndex()` (atomic publish).
- `internal/docgraph/docgraph.go:159` — markdown enumeration: `collectDocCandidates()` (glob from profile families), reads content, extracts title/snippet/search_text, calls `wikisource.Parse()` (~:212).
- `internal/wikisource/parser.go:58 Parse` — ONLY parses ```toon fence blocks; **no heading-based chunking exists yet**. `DocSourceBlock` already carries `StartLine/EndLine` (line-level chunking precedent).
- `internal/store/schema.go:189 EnsureSchema` — DDL statements array + `ensureColumn()` (:291) lazy ALTER-TABLE migrations. NEW `wiki_chunk_embeddings` table goes here.
- `internal/store/db.go:26 Open` → `configureWorkspaceDB` (WAL + pragmas) → `EnsureSchema`.
- `internal/store/index_publish.go:20 ReplaceWorkspaceIndex` + `internal/store/queries_docs.go:33 replaceDocsWithSourcesTx` — atomic publish (DELETE all doc tables, re-INSERT). New embeddings INSERT goes alongside source blocks.
- `internal/model/types.go` — add `WikiChunkEmbedding` model near DocSourceBlock (:342).

## 2. nav dispatch + governance gate
- `internal/cli/nav.go:563 command.AddCommand(...)` — registers all nav subcommands; define `recallCommand` near :560 (mirror `askCommand` :129-146), add to AddCommand.
- `internal/cli/nav.go:833` — `nav wiki` subcommands (search/route/pack/trace/validate-*/inventory).
- `internal/service/app.go:59` switch — add `case "nav.recall": envelope, err = a.recall(ctx, request)` (after nav.intent ~:148).
- `internal/service/governance.go:41 governanceGateEnvelope` — the gate. Calls `docgraph.InspectGovernance(root, true)`; if `status.Blocked` returns blocked envelope. Applied to: nav.ask, nav.pack, nav.route, nav.wiki.search, nav.wiki.validate-harness, nav.wiki.validate-source. NOT applied to nav.search/find/symbols/refs/context/deps.
- `internal/docgraph/governance.go:30 InspectGovernance` — requires `.docs/wiki/00_gobierno_documental.md` (+ fenced YAML) and synced `read-model.toml`.
- `internal/docgraph/governance.go:257 resolveGovernanceProfile` — maps profile string → base+overlays (spec_backend → ordered_wiki + spec_core + technical).
- BYPASS approach: `recall()` simply does NOT call `governanceGateEnvelope` (it is an additive cheap-read-like semantic surface). `index` itself is not gated, so a knowledge-wiki without `00_gobierno` already indexes its markdown into `doc_records`.

## 3. Workspace config + fixtures
- `internal/model/types.go:546 WorkspaceRegistration`, `:574 RegistryFile`, `:579 ProjectBlock`, `:591 ProjectFile`. Add `EmbeddingsBlock` + `Embeddings` field → `[embeddings]` section in `.mi-lsp/project.toml`.
- `internal/workspace/registry.go:597 LoadProjectFile` / `:609 SaveProjectFile` — auto TOML encode/decode the new block.
- `internal/workspace/topology.go:25 DetectWorkspaceLayout`, `buildProjectFile` (~:490) — initialize defaults.
- `internal/service/workspace_ops.go:369 workspaceStatus` (:385 InspectGovernance, :388 governance_profile) — add embeddings/profile status fields.
- `testdata/wiki-fanout/`: `ws-alpha` (00_gobierno spec_backend + RF-DUMMY-001.md ES), `ws-bravo` (00_gobierno + FL-DUMMY-01.md ES), `ws-charlie` (no wiki). All notes Spanish. Need a NEW bilingual fixture (ES query → EN note) for multilingual recall test.

## 4. HTTP client + output envelopes
- No production net/http client exists (daemon uses custom frame protocol `internal/daemon/client.go:17`). New embeddings client = standard `net/http.Client` + timeout + `encoding/json`. Error classification pattern at `internal/cli/root.go:294-335`.
- `internal/model/types.go:118-135 Envelope` = {ok, workspace, backend, mode, items, error, stats, truncated, warnings, hint, next_hint, coach, continuation, memory_pointer}. `Stats` (:32-40) = {symbols, files, ms, tokens_est, ...}.
- `internal/output/formatter.go:14 Render` (compact/json/text/toon/yaml); `compactItems` (~:369-491) type switch — add `case []model.RecallResult`.
- Flags flow: `internal/cli/root.go:168-182 QueryOptions` {workspace, format, token_budget, max_items, max_chars, offset, ...} → CommandRequest → app.Execute → handler → envelope → formatter. Mirror `search()` at `internal/service/app.go:417-530`.
- Closest result struct to mirror = `AskResult` (:503). New `RecallResult` = {query, archivo, heading, score, snippet, line?, why[], next_queries[]}.

## Risks flagged by explorers
- Heading chunking must handle nested headings, content between levels, fenced code blocks inside sections.
- Schema migration must be backward-compatible (lazy add table; old generations lack embeddings until first re-index).
- Embeddings endpoint latency/offline → must degrade to lexical (locked by prompt).
- Secret handling: api_key must not live in plaintext-committed config; use env var.
- token_budget is computed post-serialization today; recall truncation should cap top-k before format.
