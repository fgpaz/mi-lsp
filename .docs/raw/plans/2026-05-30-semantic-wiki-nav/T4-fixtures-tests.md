<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md","RF-QRY-001"]
allowed_paths: ["testdata/wiki-semantic/**","internal/service/recall_test.go"]
forbidden_paths: [".git/**",".mi-lsp/**","worker-dotnet/**",".env",".env.*"]
verify: ["go test ./internal/service/ -run Recall -v"]
stop_if: ["T3 recall handler or store functions are missing"]
secret_scan: "none — fake httptest embeddings server only"
-->

# Task T4: Bilingual fixture + integration tests (recall + knowledge-wiki profile)

## Shared Context
**Goal:** Prove the semantic path end-to-end with deterministic tests: a bilingual fixture where a Spanish query retrieves an English note by meaning, and a knowledge-wiki fixture (no `00_gobierno_documental.md`) that indexes + recalls WITHOUT being governance-blocked.
**Stack:** Go 1.24 testing, `net/http/httptest` for a fake deterministic embeddings server.
**Architecture:** Tests build a fake embeddings endpoint returning fixed vectors keyed by content so cosine ranking is deterministic and offline.

## Locked Decisions
- Tests must NOT require network. Real tesla smoke is a separate manual step (V2), not a unit test.
- Fake embeddings server returns a deterministic vector per input: map known concept phrases (ES and EN) to the SAME region of vector space so the ES query ranks the EN note first.
- Do NOT run git.

## Task Metadata
```yaml
id: T4
depends_on: [T3]
agent_type: general-purpose
goal_id: G2
expected_outcome: "Integration tests prove ES query retrieves EN note by meaning and knowledge-wiki (no governance) is not blocked."
files:
  - create: testdata/wiki-semantic/notes/acidification.md       # EN note
  - create: testdata/wiki-semantic/notes/otros.md                # ES distractor note
  - create: internal/service/recall_test.go                      # integration test
complexity: medium
done_when:
  - "go test ./internal/service/ -run Recall exits 0"
  - "test asserts ES query 'acidificacion biologica' ranks the EN acidification note first"
  - "test asserts a wiki WITHOUT 00_gobierno_documental.md indexes + recalls without governance block"
evidence_expected:
  - "go test ./internal/service/ -run Recall -v output"
stop_if:
  - "the recall handler or store functions from T3 are missing (report mismatch)"
```

## Reference
- `internal/service/*_test.go` — copy the harness/setup pattern (temp workspace, store.Open, App construction) from an existing service test (e.g. `wiki_search` or `ask` tests). READ one before writing.
- T3 store API: `LoadWikiChunkEmbeddings`, `ReplaceWikiChunkEmbeddingsForDocs`, `AllWikiChunkEmbeddings`. T3 handler: `a.recall`, `a.embedWorkspaceWiki`.

## Prompt
1. Create `testdata/wiki-semantic/` as a knowledge-wiki (NO `00_gobierno_documental.md`, NO `_mi-lsp/read-model.toml`):
   - `notes/acidification.md`: an English note with `## Biological acidification` section describing how microbial fermentation lowers pH (a few sentences). Add another `## Storage` section.
   - `notes/otros.md`: a Spanish note about unrelated topics (e.g. `## Logística de transporte`, `## Facturación`).
2. Write `internal/service/recall_test.go` (package matching the other service tests):
   - Spin up `httptest.NewServer` implementing `POST /embeddings` returning, for each input string, a deterministic vector. Strategy: compute a small fixed-dim vector (e.g. dim=8) where the value is driven by presence of concept keywords. Map both `acidif`/`acidification`/`fermentation`/`ph` (EN) AND `acidificacion`/`fermentacion` (ES) to the SAME axis so the ES query vector aligns with the EN acidification chunk and NOT with the logistics/facturación chunks. Keep it simple and deterministic (no randomness).
   - Build a temp workspace pointing at a copy of `testdata/wiki-semantic` (or register it). Configure `[embeddings]` (Enabled, BaseURL=server.URL, Model="fake", Dim=8, BatchSize=4) on its ProjectFile.
   - Run the index/embibed path: call `a.embedWorkspaceWiki(ctx, root)` (after a docs index so doc_records exist) — or the full index job — and assert no fatal error and that `store.AllWikiChunkEmbeddings` returns rows for the acidification note.
   - Call `a.recall` with payload query `"acidificacion biologica"`. Assert `Ok`, items non-empty, and `items[0].Archivo` ends with `acidification.md` with `Heading` containing "acidification" — i.e. the ES query retrieved the EN note by meaning.
   - Assert profile detection returns `knowledge-wiki` (no governance doc) and that recall was NOT blocked (no governance envelope).
   - Add a sub-test: with `[embeddings]` absent/disabled, `a.recall` returns `Ok:true`, empty items, and a non-empty `Hint` (config-gated, no panic).
3. Run `go test ./internal/service/ -run Recall -v`. Fix until green. Do NOT run git.

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp`. Read an existing `internal/service/*_test.go` to copy the App/store/temp-workspace setup.
2. Create the two fixture notes, then `recall_test.go`.
3. `go test ./internal/service/ -run Recall -v`; iterate to green.
4. Report the verbose test output verbatim.

## Skeleton
```go
func newFakeEmbeddings(t *testing.T) *httptest.Server { /* deterministic concept-keyed vectors, dim=8 */ }
func TestRecall_BilingualEStoEN(t *testing.T) { /* index fixture, recall ES query, assert EN note first */ }
func TestRecall_KnowledgeWikiNoGovernance(t *testing.T) { /* profile=knowledge-wiki, not blocked */ }
func TestRecall_ConfigGatedWhenDisabled(t *testing.T) { /* empty items + hint, no panic */ }
```

## Verify
`go test ./internal/service/ -run Recall -v` → `PASS`

## Commit
(orchestrator commits) — suggested: `test(recall): bilingual ES→EN recall + knowledge-wiki-without-governance fixtures`
