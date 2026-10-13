package model

// WikiCodeSourceProjection is an opt-in query projection, not source canon or
// compiler evidence. Availability describes publication; Resolution describes
// exact selector lookup. No score can select a winner from Candidates.
type WikiCodeSourceProjection struct {
	Version           string                      `json:"version"`
	Availability      string                      `json:"availability"`
	Reason            string                      `json:"reason,omitempty"`
	Resolution        string                      `json:"resolution"`
	Omission          string                      `json:"omission,omitempty"`
	DocumentLifecycle string                      `json:"document_lifecycle,omitempty"`
	Historical        bool                        `json:"historical"`
	Candidates        []string                    `json:"candidates,omitempty"`
	Declarations      []WikiCodeSourceDeclaration `json:"declarations"`
	Truncated         bool                        `json:"truncated,omitempty"`
	DeterminismDigest string                      `json:"determinism_digest"`
}

type WikiCodeSourceDeclaration struct {
	DocBindingSource
	Resolution         string `json:"resolution"`
	Omission           string `json:"omission,omitempty"`
	ExcludedFromActive bool   `json:"excluded_from_active"`
	SupersededBy       string `json:"superseded_by,omitempty"`
}
