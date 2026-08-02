package model

import "time"

const PreparationSchema = "mi-lsp-preparation/v1"

type PreparationPacket struct {
	Schema        string               `json:"schema"`
	Workspace     PreparationWorkspace `json:"workspace"`
	Task          PreparationTask      `json:"task"`
	Semantic      PreparationSemantic  `json:"semantic"`
	Scope         PreparationScope     `json:"scope"`
	Lineage       PreparationLineage   `json:"lineage"`
	Evidence      PreparationEvidence  `json:"evidence"`
	Status        string               `json:"status"`
	Compatibility string               `json:"compatibility"`
	PacketDigest  string               `json:"packet_digest,omitempty"`
}
type PreparationWorkspace struct {
	CanonicalRoot  string `json:"canonical_root"`
	IdentityDigest string `json:"identity_digest"`
}
type PreparationTask struct {
	Digest string `json:"digest"`
	Intent string `json:"intent"`
}
type PreparationSemantic struct {
	GovernanceDigest string `json:"governance_digest"`
	IndexDigest      string `json:"index_digest"`
	PlanDigest       string `json:"plan_digest,omitempty"`
	SeedDigest       string `json:"seed_digest,omitempty"`
}
type PreparationScope struct {
	AllowedPaths  []string `json:"allowed_paths"`
	DeniedClasses []string `json:"denied_classes"`
	ReadOnly      bool     `json:"read_only"`
}
type PreparationLineage struct {
	PreparationID       string    `json:"preparation_id"`
	ParentSession       string    `json:"parent_session,omitempty"`
	ParentPreparationID string    `json:"parent_preparation_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}
type PreparationEvidence struct {
	Root          string `json:"root"`
	ReceiptDigest string `json:"receipt_digest,omitempty"`
}
type PreparationResult struct {
	EvidenceOnly      bool               `json:"evidence_only"`
	Code              string             `json:"code"`
	Repairability     string             `json:"repairability"`
	RecommendedAction string             `json:"recommended_action"`
	Packet            *PreparationPacket `json:"packet,omitempty"`
	PacketPath        string             `json:"packet_path,omitempty"`
	Transferable      bool               `json:"transferable"`
}
