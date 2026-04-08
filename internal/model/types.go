package model

import "time"

const ProtocolVersion = "mi-lsp-v1.1"

const (
	WorkspaceKindSingle    = "single"
	WorkspaceKindContainer = "container"
	EntrypointKindSolution = "solution"
	EntrypointKindProject  = "project"
)

type QueryOptions struct {
	Workspace   string `json:"workspace,omitempty"`
	Format      string `json:"format,omitempty"`
	TokenBudget int    `json:"token_budget,omitempty"`
	MaxItems    int    `json:"max_items,omitempty"`
	MaxChars    int    `json:"max_chars,omitempty"`
	Offset      int    `json:"offset,omitempty"`
	Verbose     bool   `json:"verbose,omitempty"`
	ClientName  string `json:"client_name,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	BackendHint string `json:"backend_hint,omitempty"`
	Compress    bool   `json:"compress,omitempty"`
}

type Stats struct {
	Symbols        int   `json:"symbols,omitempty"`
	Files          int   `json:"files,omitempty"`
	Ms             int64 `json:"ms,omitempty"`
	TokensEstimate int   `json:"tokens_est,omitempty"`
}

type Envelope struct {
	Ok        bool     `json:"ok"`
	Workspace string   `json:"workspace,omitempty"`
	Backend   string   `json:"backend,omitempty"`
	Items     any      `json:"items"`
	Truncated bool     `json:"truncated"`
	Stats     Stats    `json:"stats,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	Hint      string   `json:"hint,omitempty"`
	NextHint  *string  `json:"next_hint,omitempty"`
}

// QueryEnvelope is a semantic alias of Envelope for traceability with 05_modelo_datos.md.
type QueryEnvelope = Envelope

type ServiceSurfaceSummary struct {
	Service          string           `json:"service"`
	Path             string           `json:"path"`
	Profile          string           `json:"profile,omitempty"`
	Sources          []string         `json:"sources,omitempty"`
	Symbols          map[string]int   `json:"symbols,omitempty"`
	HTTPEndpoints    []map[string]any `json:"http_endpoints,omitempty"`
	EventConsumers   []map[string]any `json:"event_consumers,omitempty"`
	EventPublishers  []map[string]any `json:"event_publishers,omitempty"`
	Entities         []map[string]any `json:"entities,omitempty"`
	Infrastructure   map[string]any   `json:"infrastructure,omitempty"`
	ArchetypeMatches []string         `json:"archetype_matches,omitempty"`
	NextQueries      []string         `json:"next_queries,omitempty"`
}

type SymbolRecord struct {
	ID            int64  `json:"id,omitempty"`
	FilePath      string `json:"file_path"`
	RepoID        string `json:"repo_id,omitempty"`
	RepoName      string `json:"repo,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	StartLine     int    `json:"line"`
	EndLine       int    `json:"end_line,omitempty"`
	Parent        string `json:"parent,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
	Signature     string `json:"signature,omitempty"`
	SignatureHash string `json:"signature_hash,omitempty"`
	Scope         string `json:"scope,omitempty"`
	Language      string `json:"language"`
	FileHash      string `json:"file_hash,omitempty"`
	Implements    string `json:"implements,omitempty"`
	SearchText    string `json:"search_text,omitempty"`
}

type FileRecord struct {
	FilePath    string `json:"file_path"`
	RepoID      string `json:"repo_id,omitempty"`
	RepoName    string `json:"repo,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	IndexedAt   int64  `json:"indexed_at,omitempty"`
	Language    string `json:"language"`
}

type DocRecord struct {
	Path        string `json:"path"`
	Title       string `json:"title,omitempty"`
	DocID       string `json:"doc_id,omitempty"`
	Layer       string `json:"layer,omitempty"`
	Family      string `json:"family,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	SearchText  string `json:"search_text,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	IndexedAt   int64  `json:"indexed_at,omitempty"`
}

type DocEdge struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path,omitempty"`
	ToDocID  string `json:"to_doc_id,omitempty"`
	Kind     string `json:"kind"`
	Label    string `json:"label,omitempty"`
}

type DocMention struct {
	DocPath      string `json:"doc_path"`
	MentionType  string `json:"mention_type"`
	MentionValue string `json:"mention_value"`
}

type DocsReadProfile struct {
	Version     int                 `toml:"version"`
	Families    []DocsReadFamily    `toml:"family"`
	GenericDocs DocsGenericFallback `toml:"generic_docs"`
}

type DocsReadFamily struct {
	Name           string   `toml:"name"`
	IntentKeywords []string `toml:"intent_keywords"`
	Paths          []string `toml:"paths"`
}

type DocsGenericFallback struct {
	Paths []string `toml:"paths"`
}

type AskDocEvidence struct {
	Path    string `json:"path"`
	Title   string `json:"title,omitempty"`
	DocID   string `json:"doc_id,omitempty"`
	Layer   string `json:"layer,omitempty"`
	Family  string `json:"family,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type AskCodeEvidence struct {
	Type    string `json:"type"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Name    string `json:"name,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type AskResult struct {
	Question     string            `json:"question,omitempty"`
	Summary      string            `json:"summary"`
	PrimaryDoc   AskDocEvidence    `json:"primary_doc"`
	DocEvidence  []AskDocEvidence  `json:"doc_evidence,omitempty"`
	CodeEvidence []AskCodeEvidence `json:"code_evidence,omitempty"`
	Why          []string          `json:"why,omitempty"`
	NextQueries  []string          `json:"next_queries,omitempty"`
}

type WorkspaceRegistration struct {
	Name      string   `json:"name,omitempty" toml:"-"`
	Root      string   `json:"root" toml:"root"`
	Languages []string `json:"languages,omitempty" toml:"languages"`
	Kind      string   `json:"kind,omitempty" toml:"kind,omitempty"`
	Solution  string   `json:"sln,omitempty" toml:"sln,omitempty"`
}

type WorkspaceRepo struct {
	ID                string   `json:"id" toml:"id"`
	Name              string   `json:"name" toml:"name"`
	Root              string   `json:"root" toml:"root"`
	Languages         []string `json:"languages,omitempty" toml:"languages"`
	DefaultEntrypoint string   `json:"default_entrypoint,omitempty" toml:"default_entrypoint,omitempty"`
}

type WorkspaceEntrypoint struct {
	ID      string `json:"id" toml:"id"`
	RepoID  string `json:"repo_id" toml:"repo_id"`
	Path    string `json:"path" toml:"path"`
	Kind    string `json:"kind" toml:"kind"`
	Default bool   `json:"default,omitempty" toml:"default,omitempty"`
}

type RegistryDefaults struct {
	LastWorkspace string `toml:"last_workspace,omitempty"`
}

type RegistryFile struct {
	Defaults   RegistryDefaults                 `toml:"defaults"`
	Workspaces map[string]WorkspaceRegistration `toml:"workspaces"`
}

type ProjectBlock struct {
	Name              string   `toml:"name"`
	Languages         []string `toml:"languages"`
	Kind              string   `toml:"kind,omitempty"`
	DefaultRepo       string   `toml:"default_repo,omitempty"`
	DefaultEntrypoint string   `toml:"default_entrypoint,omitempty"`
}

type IgnoreBlock struct {
	ExtraPatterns []string `toml:"extra_patterns"`
}

type ProjectFile struct {
	Project     ProjectBlock          `toml:"project"`
	Ignore      IgnoreBlock           `toml:"ignore"`
	Repos       []WorkspaceRepo       `toml:"repo"`
	Entrypoints []WorkspaceEntrypoint `toml:"entrypoint"`
}

type CommandRequest struct {
	ProtocolVersion string         `json:"protocol_version,omitempty"`
	Operation       string         `json:"operation"`
	Context         QueryOptions   `json:"context"`
	Payload         map[string]any `json:"payload,omitempty"`
}

type WorkerRequest struct {
	ProtocolVersion string         `json:"protocol_version,omitempty"`
	Method          string         `json:"method"`
	Workspace       string         `json:"workspace"`
	WorkspaceName   string         `json:"workspace_name,omitempty"`
	BackendType     string         `json:"backend_type,omitempty"`
	RepoID          string         `json:"repo_id,omitempty"`
	RepoName        string         `json:"repo_name,omitempty"`
	RepoRoot        string         `json:"repo_root,omitempty"`
	EntrypointID    string         `json:"entrypoint_id,omitempty"`
	EntrypointPath  string         `json:"entrypoint_path,omitempty"`
	EntrypointType  string         `json:"entrypoint_type,omitempty"`
	Payload         map[string]any `json:"payload,omitempty"`
}

type WorkerResponse struct {
	Ok       bool             `json:"ok"`
	Backend  string           `json:"backend,omitempty"`
	Items    []map[string]any `json:"items,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
	Error    string           `json:"error,omitempty"`
	Stats    Stats            `json:"stats,omitempty"`
}

type WorkerStatus struct {
	Workspace      string    `json:"workspace"`
	WorkspaceRoot  string    `json:"workspace_root,omitempty"`
	BackendType    string    `json:"backend_type,omitempty"`
	RuntimeKey     string    `json:"runtime_key,omitempty"`
	RepoID         string    `json:"repo_id,omitempty"`
	RepoName       string    `json:"repo,omitempty"`
	RepoRoot       string    `json:"repo_root,omitempty"`
	EntrypointID   string    `json:"entrypoint_id,omitempty"`
	EntrypointPath string    `json:"entrypoint_path,omitempty"`
	EntrypointType string    `json:"entrypoint_type,omitempty"`
	PID            int       `json:"pid,omitempty"`
	MemoryBytes    uint64    `json:"memory_bytes,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	LastUsedAt     time.Time `json:"last_used_at,omitempty"`
}

type DaemonState struct {
	RunID           int64     `json:"run_id,omitempty"`
	PID             int       `json:"pid"`
	Endpoint        string    `json:"endpoint"`
	AdminURL        string    `json:"admin_url,omitempty"`
	RepoRoot        string    `json:"repo_root,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	Version         string    `json:"version,omitempty"`
	ProtocolVersion string    `json:"protocol_version,omitempty"`
	MaxWorkers      int       `json:"max_workers,omitempty"`
	IdleTimeout     string    `json:"idle_timeout,omitempty"`
	AlreadyRunning  bool      `json:"already_running,omitempty"`
}

type AccessEvent struct {
	ID             int64     `json:"id,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
	ClientName     string    `json:"client_name,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	Seq            int       `json:"seq,omitempty"`
	Workspace      string    `json:"workspace,omitempty"`
	WorkspaceInput string    `json:"workspace_input,omitempty"`
	WorkspaceRoot  string    `json:"workspace_root,omitempty"`
	WorkspaceAlias string    `json:"workspace_alias,omitempty"`
	Repo           string    `json:"repo,omitempty"`
	Operation      string    `json:"operation"`
	Backend        string    `json:"backend,omitempty"`
	Success        bool      `json:"success"`
	LatencyMs      int64     `json:"latency_ms,omitempty"`
	Warnings       []string  `json:"warnings,omitempty"`
	RuntimeKey     string    `json:"runtime_key,omitempty"`
	EntrypointID   string    `json:"entrypoint_id,omitempty"`
	Error          string    `json:"error,omitempty"`
	ErrorKind      string    `json:"error_kind,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	Truncated      bool      `json:"truncated,omitempty"`
	ResultCount    int       `json:"result_count,omitempty"`
}

// TraceLink represents a spec-to-code link, either explicit (wiki marker) or inferred (heuristic).
type TraceLink struct {
	File       string  `json:"file"`
	Symbol     string  `json:"symbol,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	Source     string  `json:"source"`                // "wiki-marker" | "heuristic"
	Verified   bool    `json:"verified"`
	Confidence float64 `json:"confidence,omitempty"`
}

// TraceDrift represents a detected divergence between spec and code (v2 stub).
type TraceDrift struct {
	Rule     string `json:"rule"`
	Actual   string `json:"actual"`
	Severity string `json:"severity"` // "info" | "warn" | "error"
}

// TraceResult represents the traceability result for a single RF requirement.
type TraceResult struct {
	RF       string       `json:"rf"`
	Title    string       `json:"title"`
	Status   string       `json:"status"`   // "implemented" | "partial" | "missing"
	Coverage float64      `json:"coverage"` // 0.0 - 1.0
	Explicit []TraceLink  `json:"explicit"`
	Inferred []TraceLink  `json:"inferred"`
	Tests    []TraceLink  `json:"tests"`
	Drift    []TraceDrift `json:"drift"`
}

// ProjectConfig is a semantic alias of ProjectFile for traceability with 05_modelo_datos.md.
type ProjectConfig = ProjectFile

// WorkspaceMeta represents index totals and metadata for a repo-local workspace.
type WorkspaceMeta struct {
	TotalSymbols int    `json:"total_symbols"`
	TotalFiles   int    `json:"total_files"`
	LastIndexed  int64  `json:"last_indexed,omitempty"`
	SchemaVer    string `json:"schema_version,omitempty"`
}

// DaemonRun represents a historical daemon run for telemetry.
type DaemonRun struct {
	ID        int64     `json:"id"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt time.Time `json:"stopped_at,omitempty"`
	Version   string    `json:"version,omitempty"`
}

// RuntimeSnapshot represents the state of a runtime at a point in time.
type RuntimeSnapshot struct {
	ID          int64     `json:"id"`
	DaemonRunID int64     `json:"daemon_run_id"`
	RuntimeKey  string    `json:"runtime_key"`
	BackendType string    `json:"backend_type"`
	PID         int       `json:"pid"`
	MemoryBytes uint64    `json:"memory_bytes,omitempty"`
	SnapshotAt  time.Time `json:"snapshot_at"`
}
