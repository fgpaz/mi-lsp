package model

// WikiGraphNode is the stable public projection of a node in the unified
// wiki-and-code graph.
type WikiGraphNode struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	DocID  string `json:"doc_id,omitempty"`
	Path   string `json:"path,omitempty"`
	Symbol string `json:"symbol,omitempty"`
}

// WikiGraphEdge projects one persisted graph edge. Provenance is one of
// declared, observed, derived, or candidate.
type WikiGraphEdge struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	Relation   string `json:"relation"`
	To         string `json:"to"`
	Provenance string `json:"provenance"`
}

type WikiGraphMatrixRow struct {
	RFID  string   `json:"rf_id"`
	TPIDs []string `json:"tp_ids"`
}

// WikiGraphUnresolved identifies an authored target that was not resolved to
// a unique graph node. It remains a candidate, never a synthetic edge.
type WikiGraphUnresolved struct {
	OwnerPath      string   `json:"owner_path,omitempty"`
	SourceDocument string   `json:"source_document,omitempty"`
	SourceBlock    string   `json:"source_block,omitempty"`
	TargetKind     string   `json:"target_kind,omitempty"`
	TargetValue    string   `json:"target_value,omitempty"`
	Candidates     []string `json:"candidates,omitempty"`
	ReasonCode     string   `json:"reason_code"`
	Provenance     string   `json:"provenance"`
}

// WikiGraphMermaid is derived for the current response only; it is not stored
// in the wiki or graph generation.
type WikiGraphMermaid struct {
	Format       string   `json:"format"`
	Source       string   `json:"source"`
	GenerationID string   `json:"generation_id"`
	NodeIDs      []string `json:"node_ids"`
	EdgeIDs      []string `json:"edge_ids"`
}

type WikiGraphRoadmapEntry struct {
	RSID    string `json:"rs_id"`
	Horizon string `json:"horizon,omitempty"`
	Status  string `json:"status,omitempty"`
}

type WikiGraphRoadmap struct {
	Canonical bool                    `json:"canonical"`
	Entries   []WikiGraphRoadmapEntry `json:"entries"`
}

type WikiContractValidationCheck struct {
	DocID    string                      `json:"doc_id,omitempty"`
	Path     string                      `json:"path"`
	Kind     string                      `json:"kind"`
	Sections []WikiContractSection       `json:"sections,omitempty"`
	Criteria []WikiContractCriterion     `json:"criteria,omitempty"`
	Cases    []WikiContractCaseCriterion `json:"cases,omitempty"`
	Findings []WikiContractFinding       `json:"findings,omitempty"`
}
