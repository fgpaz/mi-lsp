<!--
sdd_artifact: design
task_slug: semantic-wiki-nav
generated_at: 2026-05-30
status: validated (brainstorming gate passed)
secret_scan: none
-->

# Design — Semantic knowledge-wiki navigation with pluggable embeddings

## Goal

Add to `mi-lsp` the ability to index and navigate **semantically** (by meaning, multilingual) any markdown knowledge wiki — not only code repos — via a pluggable OpenAI-compatible embeddings backend. Additive: complements the existing code LSP path, never rewrites it. Existing `go test ./...` stays green.

## Locked decisions (brainstorming 2026-05-30)

| ID | Decision |
|----|----------|
| D1 | Vector store = SQLite `wiki_chunk_embeddings` BLOB table in `.mi-lsp/index.db`; brute-force cosine in pure Go. No CGO, no heavy deps (forced by `modernc.org/sqlite` + plain GOOS/GOARCH cross-compile). |
| D2 | New top-level `mi-lsp nav recall <query>`, UNGATED. `nav wiki search` stays lexical. |
| D3 | Profile auto-detected: valid `00_gobierno_documental.md` ⇒ `spec-driven`; absent/invalid ⇒ `knowledge-wiki`. Optional `[embeddings].profile` override. recall ungated in both. |
| D4 | Chunk markdown by `##`+ headings (carry `#` title as context). Re-embed only changed chunks by `content_hash` (sha256) + model + dim. |
| D5 | Endpoint offline/error ⇒ recall degrades to lexical over the same notes; never hard-fails; emits warning+hint. |
| D6 | No silent external calls: no `[embeddings]` config ⇒ recall returns actionable hint, calls nothing. tesla = documented reference only. |
| D7 | api_key from env var named by `[embeddings].api_key_env` (default `MI_LSP_EMBEDDINGS_API_KEY`); populated via `mkey run`; never committed. Bearer header sent only when a key resolves. |

## Configuration — `.mi-lsp/project.toml`

```toml
[embeddings]
enabled      = true
provider     = "openai-compatible"
base_url     = "http://tesla.tailde0a03.ts.net:8081/v1"   # reference example
model        = "bge-m3"
dim          = 1024
api_key_env  = "MI_LSP_EMBEDDINGS_API_KEY"                 # optional; mkey run injects it
profile      = ""                                          # "" = auto; or "knowledge-wiki" | "spec-driven"
batch_size   = 32                                          # embed N chunks per HTTP call
timeout_ms   = 30000
```

All fields optional with sane defaults; `enabled=false` or missing `[embeddings]` ⇒ recall is config-gated (D6).

## Architecture (data flow)

```text
mi-lsp index (knowledge-wiki or spec-driven)
  docgraph.IndexWorkspaceDocsWithSourcesWithProgress()        internal/docgraph/docgraph.go:159
    -> for each .md: chunkByHeading(content)                  NEW internal/wikichunk (or docgraph)
       -> []WikiChunkCandidate{path, startLine, endLine, heading, text, contentHash}
  indexer.IndexWorkspaceWithProgress()                         internal/indexer/indexer.go:45
    -> if embeddings.enabled: embedChunks(ctx, candidates)     NEW internal/embed (HTTP client)
       -> skip chunks whose (contentHash,model,dim) unchanged vs existing rows (incremental)
       -> POST {base_url}/embeddings {model, input:[...]}      batched
    -> store.ReplaceWorkspaceIndex(..., wikiChunkEmbeddings)   internal/store/index_publish.go:20
       -> INSERT OR REPLACE into wiki_chunk_embeddings         internal/store/queries_docs.go

mi-lsp nav recall "<query>"                                    internal/cli/nav.go:563 (recallCommand)
  app.Execute case "nav.recall" -> a.recall(ctx, req)          internal/service/app.go:59
    NO governanceGateEnvelope (ungated, D2)
    -> embed(query) via internal/embed
    -> store.LoadWikiChunkEmbeddings(db) ; cosine(q, each)     internal/store + internal/embed
    -> top-k by score, build []RecallResult                    internal/model/types.go
    -> on embed failure: lexical fallback over notes (D5)       reuse search/text path
  output formatter: case []RecallResult                        internal/output/formatter.go
```

## New SQLite table (schema.go EnsureSchema, lazy/backward-compatible)

```sql
CREATE TABLE IF NOT EXISTS wiki_chunk_embeddings (
    doc_path        TEXT NOT NULL,
    chunk_id        TEXT NOT NULL,
    start_line      INTEGER NOT NULL,
    end_line        INTEGER NOT NULL,
    heading_text    TEXT,
    snippet         TEXT,
    content_hash    TEXT NOT NULL,
    embedding       BLOB,            -- float32 little-endian, dim floats
    embedding_model TEXT,
    embedding_dim   INTEGER,
    indexed_at      INTEGER,
    UNIQUE(doc_path, chunk_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_chunk_embeddings_doc ON wiki_chunk_embeddings(doc_path);
```

BLOB encoding: `dim` × float32 little-endian (`encoding/binary`). Decode + cosine in Go. Vectors assumed normalized (bge-m3) but cosine normalizes anyway for safety.

## Components & ownership (allowed paths)

- `internal/model/types.go` — `EmbeddingsBlock`, `WikiChunkEmbedding`, `RecallResult`.
- `internal/embed/` (NEW) — OpenAI-compatible HTTP client (`Embed(ctx, texts) ([][]float32, error)`), cosine, BLOB encode/decode. Pure stdlib (`net/http`, `encoding/json`, `encoding/binary`).
- `internal/wikichunk/` (NEW) OR a function in `internal/docgraph` — `ChunkByHeading(path, content) []WikiChunkCandidate`. Handles nested headings, fenced code blocks (no chunk split inside ``` fences), front-matter/title context.
- `internal/store/schema.go`, `queries_docs.go`, `index_publish.go` — table + insert + load.
- `internal/indexer/indexer.go` — embedding orchestration step (incremental skip by hash).
- `internal/docgraph/docgraph.go` — produce chunk candidates during doc walk.
- `internal/service/recall.go` (NEW) — `recall()` handler (ungated), profile detection, lexical fallback.
- `internal/service/app.go` — dispatch `case "nav.recall"`.
- `internal/service/workspace_ops.go` — report `embeddings`/`profile` in workspace status.
- `internal/cli/nav.go`, `internal/cli/root.go` — `recallCommand` + flags.
- `internal/output/formatter.go` — render `[]RecallResult`.
- `internal/workspace/topology.go`, `registry.go` — default `[embeddings]` block, load/save.
- `testdata/wiki-fanout/` — NEW bilingual fixture (ES query ⇒ EN note).

## Edge cases / safeguards

- Offline endpoint, missing key, missing config ⇒ degrade or hint; never panic.
- Index without embeddings config ⇒ docs index normally, no vectors; recall hints.
- Schema migration is `CREATE TABLE IF NOT EXISTS` ⇒ old index.db upgrades lazily; first re-index populates.
- Incremental: unchanged chunks (same hash+model+dim) are not re-embedded ⇒ cheap re-index.
- Determinism in tests: a fake embeddings server (httptest) returns deterministic vectors keyed by text; bilingual fixture asserts ES query retrieves EN note. Real tesla smoke is separate, network-gated.

## Testing strategy

1. Unit: `ChunkByHeading` (headings, code fences, nesting), cosine/BLOB roundtrip, embed client against `httptest.Server`.
2. Integration: index a fixture wiki with a fake embeddings server ⇒ `nav recall` returns the right section; ES-query→EN-note multilingual assertion via a deterministic fake that maps known synonyms.
3. Profile: index+recall a knowledge-wiki fixture WITHOUT `00_gobierno_documental.md` ⇒ not blocked.
4. Non-regression: full `go test ./...` green.
5. Smoke (manual/network): real tesla bge-m3 via `mkey run teslita <env> -- mi-lsp nav recall "..."`.

## Docs sync (after implementation)

- README.md: new `nav recall` + `[embeddings]` config + knowledge-wiki profile.
- CHANGELOG.md: Unreleased / Added.
- 07_baseline_tecnica.md + new TECH-SEMANTIC-RECALL.md (embeddings backend, vector store, profiles).
- 08_modelo_fisico_datos.md + new DB-WIKI-EMBEDDINGS.md (table schema).
- 09_contratos_tecnicos.md + new CT-NAV-RECALL.md (command + envelope).
- 03_FL.md (+ FL-SEM-01), 04_RF.md (+ RF-SEM family), 06 test matrix (+ TP-SEM).
- AE-RELEASE-DISTRIBUTION: binary refresh waived this cycle (code merged via PR; no publish requested).

## Source Map generalization (prompt item 5)

`nav recall` IS the generalized Source-Map navigator: for any sub-topic it returns `archivo § heading + score + snippet + why` so an agent knows which note/section to read. A `--map` mode (or `nav recall --map "<topic>"`) can emit a compact sub-topic→note→section→when-to-use list reusing the same ranking. Implement as a thin output variant over the same recall ranking.
