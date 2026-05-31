<!--
linear_parent: not_applicable
linear_child: not_applicable
anchors: [".docs/raw/plans/2026-05-30-semantic-wiki-nav.md","RF-IDX-001"]
allowed_paths: ["internal/embed/**"]
forbidden_paths: [".git/**",".mi-lsp/**","worker-dotnet/**",".env",".env.*"]
verify: ["go test ./internal/embed/..."]
stop_if: ["a non-stdlib import would be required"]
secret_scan: "none — only env-var NAME referenced"
-->

# Task T1: internal/embed — OpenAI-compatible embeddings client + cosine + BLOB codec

## Shared Context
**Goal:** New pure-Go package providing embeddings via an OpenAI-compatible endpoint, plus cosine similarity and float32↔BLOB encoding.
**Stack:** Go 1.24, stdlib only (`net/http`, `encoding/json`, `encoding/binary`, `math`, `context`, `os`). NO new go.mod deps. NO CGO.
**Architecture:** Leaf package; nothing else depends on it yet. `internal/indexer` and `internal/service/recall` will import it later (other tasks).

## Locked Decisions
- OpenAI-compatible POST `{base_url}/embeddings` with body `{"model": <model>, "input": [<texts>]}`; response `{"data":[{"embedding":[...]},...]}`.
- api_key resolved from env var (name passed in config); `Authorization: Bearer <key>` header sent ONLY when key non-empty.
- BLOB encoding = little-endian float32 sequence (`encoding/binary.LittleEndian`), `dim` floats.
- Cosine normalizes both vectors (do not assume pre-normalized).
- Do NOT run git. Do NOT edit files outside `internal/embed/`.

## Task Metadata
```yaml
id: T1
depends_on: []
agent_type: general-purpose
goal_id: G1
expected_outcome: "internal/embed package compiles and unit tests pass against an httptest fake server."
files:
  - create: internal/embed/client.go
  - create: internal/embed/vector.go
  - create: internal/embed/client_test.go
  - create: internal/embed/vector_test.go
complexity: medium
done_when:
  - "go test ./internal/embed/... exits 0"
evidence_expected:
  - "go test ./internal/embed/... output"
stop_if:
  - "a non-stdlib import would be required"
```

## Reference
Design doc: `.docs/raw/plans/2026-05-30-semantic-wiki-nav-design.md`. Match existing repo error/style conventions (read one neighbor like `internal/daemon/client.go` for naming feel, but DO NOT copy its frame protocol — this is plain HTTP).

## Prompt
Create a new Go package `internal/embed` (package name `embed`) at `C:/repos/mios/mi-lsp/internal/embed/`. It must NOT import any non-stdlib package. Implement exactly:

`vector.go`:
- `func EncodeVector(v []float32) []byte` — little-endian float32 sequence.
- `func DecodeVector(b []byte) []float32` — inverse; len(b)/4 floats. If len(b)%4 != 0, return the floats that fit (ignore trailing bytes).
- `func Cosine(a, b []float32) float64` — returns 0 if either is zero-length or zero-norm or lengths differ; else dot/(normA*normB).

`client.go`:
- `type Config struct { Provider, BaseURL, Model, APIKeyEnv string; Dim, BatchSize, TimeoutMS int }`
- `type Client struct { ... }` with `func New(cfg Config) *Client`. Default BatchSize=32, TimeoutMS=30000 when ≤0. Uses an `*http.Client{ Timeout: ... }`.
- `func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error)` — batches `texts` by BatchSize, POSTs to `strings.TrimRight(BaseURL,"/")+"/embeddings"` with JSON `{"model":Model,"input":batch}`, sets `Content-Type: application/json` and, if `os.Getenv(APIKeyEnv)` non-empty (and APIKeyEnv non-empty), `Authorization: Bearer <key>`. Parse `{"data":[{"embedding":[]float32}]}` preserving order. Returns one []float32 per input text. On non-2xx, return an error including status code and a short body excerpt. Respect ctx cancellation.
- `func (c *Client) EmbedOne(ctx context.Context, text string) ([]float32, error)` — convenience over Embed.

Edge cases: empty `texts` ⇒ return empty slice, no HTTP call. A batch whose response has fewer embeddings than inputs ⇒ error.

Tests (`*_test.go`, package `embed`):
- `vector_test.go`: round-trip EncodeVector/DecodeVector for a known vector; Cosine of identical normalized vectors ≈ 1.0; orthogonal ⇒ 0; mismatched length ⇒ 0.
- `client_test.go`: spin up `httptest.NewServer` returning a deterministic embeddings payload for given inputs; assert Embed returns correct count and values; assert Authorization header present when env set (use `t.Setenv`), absent when env empty; assert batching issues the expected number of requests for BatchSize.

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp`. Create `internal/embed/`.
2. Write `vector.go`, then `client.go`, then the two test files following the Prompt exactly.
3. Run `go test ./internal/embed/...`.
4. If it fails, read the compiler/test output and fix WITHIN `internal/embed/` only. Do not touch other packages. Do not run git.
5. Report the final `go test ./internal/embed/...` output verbatim.

## Skeleton
```go
package embed

// vector.go
func EncodeVector(v []float32) []byte { /* binary.LittleEndian.PutUint32 per float */ }
func DecodeVector(b []byte) []float32 { /* inverse */ }
func Cosine(a, b []float32) float64  { /* guard zero/len; dot/(na*nb) */ }

// client.go
type Config struct {
    Provider, BaseURL, Model, APIKeyEnv string
    Dim, BatchSize, TimeoutMS           int
}
type Client struct { cfg Config; http *http.Client }
func New(cfg Config) *Client { /* defaults */ }
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) { /* batched POST */ }
func (c *Client) EmbedOne(ctx context.Context, text string) ([]float32, error) { /* wrap */ }
```

## Verify
`go test ./internal/embed/...` → `ok  github.com/fgpaz/mi-lsp/internal/embed`

## Commit
(orchestrator commits; agent does NOT run git) — suggested message: `feat(embed): OpenAI-compatible embeddings client + cosine + BLOB codec`
