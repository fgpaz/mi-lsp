package model

// FindFilesResult contains filenames only, never file contents. Counts refer to
// the listing snapshot, not a semantic index or a live filesystem transaction.
type FindFilesResult struct {
	Schema    string           `json:"schema"`
	Items     []FindFilesItem  `json:"items"`
	Matched   int              `json:"matched"`
	Truncated bool             `json:"truncated"`
	Reason    string           `json:"truncation_reason,omitempty"`
	NextHint  string           `json:"next_hint,omitempty"`
	Budget    FindFilesBudget  `json:"budget"`
	Listing   FindFilesListing `json:"listing"`
}

type FindFilesItem struct {
	Path   string `json:"path"`
	Rank   int    `json:"rank"`
	Reason string `json:"reason"`
}

type FindFilesBudget struct {
	Top      int `json:"top"`
	MaxBytes int `json:"max_bytes"`
}

type FindFilesListing struct {
	Backend  string `json:"backend"`
	Cached   bool   `json:"cached"`
	AgeMS    int64  `json:"age_ms"`
	MaxAgeMS int64  `json:"max_age_ms"`
}

// FindFilesError deliberately excludes subprocess stderr and absolute paths.
type FindFilesError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *FindFilesError) Error() string { return e.Code + ": " + e.Message }
