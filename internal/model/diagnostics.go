package model

// Diagnostic positions are one-based UTF-16 coordinates, matching the semantic protocols.
type Diagnostic struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
	Code     any    `json:"code,omitempty"`
}

type DiagnosticsSummary struct {
	Errors         int            `json:"errors"`
	Warnings       int            `json:"warnings"`
	Information    int            `json:"information"`
	Hints          int            `json:"hints"`
	FilesRequested int            `json:"files_requested"`
	FilesChecked   int            `json:"files_checked"`
	Returned       int            `json:"returned"`
	Backends       map[string]int `json:"backends"`
}
