<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md","RF-QRY-001","RF-IDX-001"]
allowed_paths: ["internal/model/**","internal/store/**","internal/service/**","internal/cli/**","internal/output/**","internal/workspace/**"]
forbidden_paths: [".git/**",".mi-lsp/**","worker-dotnet/**",".docs/wiki/_mi-lsp/read-model.toml",".env",".env.*"]
verify: ["go build ./...","go vet ./...","go test ./internal/..."]
stop_if: ["compiling would require editing a code LSP backend"]
secret_scan: "none — api_key only by env-var NAME"
-->

# Task T3: Integration spine — model + config + store + service (index hook + recall) + cli + output

## Shared Context
**Goal:** Wire the new `internal/embed` (T1) and `internal/wikichunk` (T2) packages into mi-lsp: model types, a SQLite embeddings table, a post-publish embedding step in the index job, an ungated `nav recall` command with profile auto-detection and lexical fallback, and output rendering. Additive; existing tests stay green.
**Stack:** Go 1.24, `modernc.org/sqlite` (pure-Go), `spf13/cobra`. Import `github.com/fgpaz/mi-lsp/internal/embed` and `.../internal/wikichunk`.
**Architecture:** Repo-local SQLite `.mi-lsp/index.db`. Indexing publishes docs; THIS task adds a post-publish embedding step in the service layer (NOT in the pure indexer/docgraph). `nav recall` embeds the query and ranks chunk vectors by cosine.

## Locked Decisions (do not reopen)
- Vector store = SQLite BLOB table `wiki_chunk_embeddings` + Go cosine. NO CGO, NO new go.mod deps.
- `nav recall` is UNGATED: it MUST NOT call `governanceGateEnvelope`.
- Profile auto-detected: valid `00_gobierno_documental.md` ⇒ `spec-driven`; absent/invalid ⇒ `knowledge-wiki`. Optional `[embeddings].profile` override. recall works in BOTH.
- Embedding index hook = POST-PUBLISH in the index job (service layer). DO NOT modify `internal/indexer/indexer.go` or `internal/docgraph/docgraph.go` signatures.
- No `[embeddings]` config / disabled / no endpoint ⇒ recall returns an actionable hint and calls nothing (no silent external calls). Embeddings failure during index ⇒ warn, do not fail the job.
- api_key from env var named by `[embeddings].api_key_env` (default `MI_LSP_EMBEDDINGS_API_KEY`).
- Do NOT run git. Do NOT touch `worker-dotnet/**`, `.docs/wiki/_mi-lsp/read-model.toml`, `00_gobierno_documental.md`, secrets, or any code LSP backend.

## Task Metadata
```yaml
id: T3
depends_on: [T1, T2]
agent_type: general-purpose
goal_id: G1, G2
expected_outcome: "go build ./... succeeds; mi-lsp nav recall command exists; index job embeds wiki chunks when [embeddings] is configured."
files:
  - modify: internal/model/types.go        # add EmbeddingsBlock, WikiChunkEmbedding, RecallResult; ProjectFile.Embeddings field
  - modify: internal/store/schema.go        # add wiki_chunk_embeddings DDL to EnsureSchema
  - create: internal/store/queries_embeddings.go  # Load + Replace functions
  - modify: internal/service/index_jobs.go  # call embedWorkspaceWiki post-publish
  - create: internal/service/recall.go      # embedWorkspaceWiki + recall handler + profile detect + lexical fallback
  - modify: internal/service/app.go         # dispatch case "nav.recall"
  - modify: internal/service/workspace_ops.go # report embeddings_enabled + profile in status
  - modify: internal/cli/nav.go             # recallCommand + register in AddCommand
  - modify: internal/output/formatter.go    # render []model.RecallResult in compactItems
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go vet ./internal/service/... ./internal/cli/... ./internal/store/... exits 0"
  - "mi-lsp nav recall --help shows the command (or `go run ./cmd/mi-lsp nav recall` is recognized)"
evidence_expected:
  - "go build ./... output"
  - "go test ./internal/store/... ./internal/service/... ./internal/cli/... ./internal/output/... output (must stay green)"
stop_if:
  - "compiling would require editing a code LSP backend (roslyn/tsserver/pyright/gopls)"
  - "a referenced function/struct does not exist after reading the file (report the mismatch)"
```

## Reference
- Design: `.docs/raw/plans/2026-05-30-semantic-wiki-nav-design.md`.
- Hook map: `.docs/auditoria/2026-05-30-semantic-wiki-nav/exploration/exploration-findings.md` (line numbers are HINTS; READ each file before editing).
- Mirror the gated-handler pattern from `internal/service/ask.go` but REMOVE the gate call. Mirror envelope construction from the `search()` handler in `internal/service/app.go`. Mirror command registration from `askCommand` in `internal/cli/nav.go`.

## Prompt
Read each target file FIRST (use mi-lsp nav / Read), then apply additive edits. Concretely:

### 1. internal/model/types.go
Add:
```go
type EmbeddingsBlock struct {
    Enabled   bool   `toml:"enabled" json:"enabled,omitempty"`
    Provider  string `toml:"provider" json:"provider,omitempty"`
    BaseURL   string `toml:"base_url" json:"base_url,omitempty"`
    Model     string `toml:"model" json:"model,omitempty"`
    Dim       int    `toml:"dim" json:"dim,omitempty"`
    APIKeyEnv string `toml:"api_key_env" json:"api_key_env,omitempty"`
    Profile   string `toml:"profile" json:"profile,omitempty"` // "" | "knowledge-wiki" | "spec-driven"
    BatchSize int    `toml:"batch_size" json:"batch_size,omitempty"`
    TimeoutMS int    `toml:"timeout_ms" json:"timeout_ms,omitempty"`
}
type WikiChunkEmbedding struct {
    DocPath, ChunkID, Heading, Snippet, ContentHash, EmbeddingModel string
    StartLine, EndLine, EmbeddingDim int
    Embedding []byte // little-endian float32
    IndexedAt int64
}
type RecallResult struct {
    Query   string  `json:"query,omitempty"`
    Archivo string  `json:"archivo"`
    Heading string  `json:"heading,omitempty"`
    Score   float64 `json:"score"`
    Snippet string  `json:"snippet,omitempty"`
    StartLine int   `json:"start_line,omitempty"`
    EndLine   int   `json:"end_line,omitempty"`
    Why     []string `json:"why,omitempty"`
}
```
Add field to the existing `ProjectFile` struct: `Embeddings *EmbeddingsBlock `toml:"embeddings,omitempty" json:"embeddings,omitempty"`` (pointer so absence ⇒ nil). Find ProjectFile by reading the file; add the field, do not reorder others.

### 2. internal/store/schema.go
In `EnsureSchema`, add the table DDL (CREATE TABLE IF NOT EXISTS) exactly as in the design doc, plus the doc_path index. Append to the same statements list the file uses; follow the existing DDL-const style.

### 3. internal/store/queries_embeddings.go (NEW)
```go
package store
// LoadWikiChunkEmbeddings returns map keyed by docPath+"\x00"+chunkID.
func LoadWikiChunkEmbeddings(ctx context.Context, db *sql.DB) (map[string]model.WikiChunkEmbedding, error)
// ReplaceWikiChunkEmbeddingsForDocs deletes rows for docPaths then inserts chunks, in one tx.
func ReplaceWikiChunkEmbeddingsForDocs(ctx context.Context, db *sql.DB, docPaths []string, chunks []model.WikiChunkEmbedding) error
// AllWikiChunkEmbeddings returns every row (for recall ranking).
func AllWikiChunkEmbeddings(ctx context.Context, db *sql.DB) ([]model.WikiChunkEmbedding, error)
```
Match the prepared-statement + tx style of `internal/store/queries_docs.go`. Use `INSERT OR REPLACE`.

### 4. internal/service/recall.go (NEW)
- `func (a *App) embedWorkspaceWiki(ctx context.Context, root string) (warnings []string)`: load ProjectFile via `workspace.LoadProjectFile(root)`; if `Embeddings == nil || !Embeddings.Enabled || BaseURL == "" || Model == ""` ⇒ return nil (no-op). Open the repo-local store the same way other service ops do (read app.go/workspace_ops.go for the exact `store.Open`/db-resolve pattern). Query doc paths from `doc_records` (or use an existing store helper). For each doc path under `.docs`/governed wiki, read the file, `wikichunk.ChunkByHeading`. Build candidates. Load existing embeddings; for each candidate reuse the existing row's Embedding when `ContentHash`+model+dim match (skip HTTP); collect the rest. Embed the rest via `embed.New(embed.Config{...from block...}).Embed`. Build `[]model.WikiChunkEmbedding` (snippet = first ~200 chars of chunk text). `store.ReplaceWikiChunkEmbeddingsForDocs(ctx, db, indexedDocPaths, all)`. On ANY error, append a warning and return (never panic, never fail the caller).
- `func (a *App) recall(ctx context.Context, request model.CommandRequest) (model.Envelope, error)`: DO NOT call governanceGateEnvelope. Resolve workspace+root (copy the resolve pattern from `ask`/`search`). Detect profile: `knowledge-wiki` if `docgraph.InspectGovernance(root,false).Blocked` OR no governance doc, else `spec-driven`; `[embeddings].profile` overrides. Read query from `request.Payload["query"]`. If embeddings not configured/enabled ⇒ return envelope `Ok:true, Backend:"recall", Items:[]` with `Hint` telling the user to configure `[embeddings]` or use `nav search`. Else: build client, `EmbedOne(query)`. If embed FAILS ⇒ lexical fallback: run the existing text search over the wiki and map to `[]RecallResult` (Score 0), envelope `Backend:"recall+lexical"`, Warning `embeddings_unavailable; served lexical`, Hint. On success: `store.AllWikiChunkEmbeddings`, compute `embed.Cosine(q, embed.DecodeVector(row.Embedding))`, sort desc, take `request.Context.MaxItems` (default 10 if ≤0), build `[]RecallResult{Archivo:DocPath, Heading, Score, Snippet, StartLine, EndLine, Why:["semantic match"]}`. Envelope `Backend:"recall"`, `Stats{Files:len(items)}`, `Mode:"semantic"`.
- Support an optional `--map` payload flag: when `request.Payload["map"]==true`, group top results into a compact "sub-topic → archivo § heading → why" list (still []RecallResult, just Why explains when-to-use). Keep it thin.

### 5. internal/service/index_jobs.go
Find where `indexer.IndexWorkspaceWithProgress` is called in `runIndexJob` and the publish succeeds. AFTER successful publish (full/docs modes), call `warns := a.embedWorkspaceWiki(ctx, registration.Root)` and append `warns` to the job's warnings. Do not change job status on embedding warnings. (catalog-only mode: skip.)

### 6. internal/service/app.go
In the `Execute` switch, add `case "nav.recall": envelope, err = a.recall(ctx, request)`. Place it near `nav.intent`/`nav.search`.

### 7. internal/service/workspace_ops.go
In `workspaceStatus`, after the `governance_profile` assignment, add `item["embeddings_enabled"] = <bool from ProjectFile.Embeddings != nil && .Enabled>` and `item["recall_profile"] = <"knowledge-wiki"|"spec-driven" via same detection as recall>`. Read the file to match the exact item-map variable name.

### 8. internal/cli/nav.go
Define `recallCommand` mirroring `askCommand` (read it). `Use: "recall <query>"`, `Short: "Semantic recall over markdown knowledge wiki (no governance gate)"`. RunE: require 1 arg, join args as query, payload `map[string]any{"query": query}`; if a `--map` bool flag is set add `payload["map"]=true`. Call the same `executeOperation`/state helper askCommand uses with operation `"nav.recall"`. Add `recallCommand` to the `command.AddCommand(...)` list for nav. Add a local `--map` flag on recallCommand. Reuse global `--max-items`/`--token-budget`/`--format` (no new global flags).

### 9. internal/output/formatter.go
In the `compactItems` type switch (and any toon/yaml item-shaping switch), add `case []model.RecallResult:` producing compact maps `{arch, h, s, snip, l}` (l = StartLine). Follow the neighboring cases' style.

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp`. Confirm `internal/embed` and `internal/wikichunk` exist (from T1/T2); if missing, STOP and report.
2. Edit files in the order 1→9 above. READ each file before editing; treat line numbers as hints.
3. After model + store + service + cli + output edits, run `go build ./...`. Fix compile errors iteratively (additive only; never edit a code LSP backend — if that seems required, STOP and report).
4. Run `go test ./internal/store/... ./internal/service/... ./internal/cli/... ./internal/output/... ./internal/model/... ./internal/workspace/...`. They MUST stay green. Fix regressions you introduced.
5. Run `go vet ./...`.
6. Do NOT run git. Report: `go build ./...` output, the targeted `go test` output, and a 1-line summary per file changed.

## Skeleton
(See per-file Prompt above; signatures are the contract. Keep everything additive.)

## Verify
`go build ./...` → exit 0; `go test ./internal/...` for touched packages → `ok`.

## Commit
(orchestrator commits) — suggested: `feat(nav): add semantic recall, embeddings index hook, and wiki_chunk_embeddings store`
