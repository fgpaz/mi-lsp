package model

const DocBindingSourceVersion = "MILSP-DBS/v1"
const DocBindingSourceAvailable = "available"
const DocBindingSourceIncomplete = "explicit_incomplete"

// GraphV1ArtifactBinding is exactly the source object, without observed or
// owner metadata. Symbol is optional; no bridge defaults enter this object.
type GraphV1ArtifactBinding struct {
	BindingID        string `json:"binding_id"`
	Kind             string `json:"kind"`
	Role             string `json:"role"`
	Path             string `json:"path"`
	Surface          string `json:"surface"`
	BindingLifecycle string `json:"binding_lifecycle"`
	Symbol           string `json:"symbol,omitempty"`
}

// DocBindingSource wraps one declared source object. BindingRef is an optional
// many-to-one association to MILSP-WCB/v1, never this declaration's identity.
// Resolution is deliberately absent: consumers derive it at query time.
type DocBindingSource struct {
	DocPath           string                 `json:"doc_path"`
	DocID             string                 `json:"doc_id"`
	BlockID           string                 `json:"block_id,omitempty"`
	DocumentLifecycle string                 `json:"document_lifecycle"`
	Binding           GraphV1ArtifactBinding `json:"binding"`
	Provenance        string                 `json:"provenance"`
	BindingRef        string                 `json:"binding_ref,omitempty"`
	SourceContentHash string                 `json:"source_content_hash,omitempty"`
	StartLine         int                    `json:"start_line,omitempty"`
	EndLine           int                    `json:"end_line,omitempty"`
}

// DocBindingSourceCollection distinguishes a parsed empty collection from an
// unavailable legacy snapshot. Truncated is independent of availability.
type DocBindingSourceCollection struct {
	Version           string             `json:"version"`
	Availability      string             `json:"availability"`
	Reason            string             `json:"reason,omitempty"`
	DocumentLifecycle string             `json:"document_lifecycle,omitempty"`
	Declarations      []DocBindingSource `json:"declarations"`
	Truncated         bool               `json:"truncated,omitempty"`
}
