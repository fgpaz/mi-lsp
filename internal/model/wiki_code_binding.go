package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Locked relation constants.
const (
	RelationImplements = "implements"
	RelationTests      = "tests"
	RelationConfigures = "configures"
	RelationOperates   = "operates"
)

// Target kind constants.
const (
	TargetKindFile   = "file"
	TargetKindSymbol = "symbol"
	TargetKindTest   = "test"
	TargetKindConfig = "config"
)

// Authoring origin constants.
const (
	AuthoringOriginCanonical = "canonical"
	AuthoringOriginLegacy    = "legacy"
)

// Binding status constants.
const (
	BindingStatusExact  = "exact"
	BindingStatusPlanned = "planned"
)

// Doc lifecycle constants.
const (
	DocLifecycleActive   = "active"
	DocLifecycleDeprecated = "deprecated"
	DocLifecycleRetired  = "retired"
)

// Role normalization constants.
const (
	RoleImplementation = "implementation"
	RoleTest           = "test"
	RoleConfig         = "config"
	RoleCompiler       = "compiler"
	RoleSupervisor     = "supervisor"
	RoleEntrypoint     = "entrypoint"
	RoleAdapter        = "adapter"
)

// DocArtifactBinding represents an explicit wiki→artifact declaration row.
type DocArtifactBinding struct {
	DocPath          string `json:"doc_path"`
	BlockID          string `json:"block_id"`
	DocID            string `json:"doc_id,omitempty"`
	Relation         string `json:"relation"`
	Role             string `json:"role,omitempty"`
	TargetPath       string `json:"target_path"`
	TargetSymbol     string `json:"target_symbol,omitempty"`
	TargetKind       string `json:"target_kind"`
	AuthoringOrigin  string `json:"authoring_origin"`
	BindingStatus    string `json:"binding_status"`
	DocLifecycle     string `json:"doc_lifecycle"`
	SupersededBy     string `json:"superseded_by,omitempty"`
	Ordinal          int    `json:"ordinal"`
	StartLine        int    `json:"start_line"`
	EndLine          int    `json:"end_line"`
	SourceContentHash string `json:"source_content_hash,omitempty"`
	BindingRef       string `json:"binding_ref"`
	IndexedAt        int64  `json:"indexed_at"`
}

// WikiCodeBindingRef computes a stable SHA-256 digest over a length-delimited
// "MILSP-WCB/v1" payload that encodes only the declaration-identity fields.
// Mutable, evidence-version, and lifecycle fields are excluded.
func WikiCodeBindingRef(docPath, blockID, docID, relation, targetPath, targetSymbol, targetKind string) string {
	// Normalize repo-relative slashes (POSIX) before hashing.
	normalizedPath := strings.ReplaceAll(docPath, "\\", "/")
	normalizedTarget := strings.ReplaceAll(targetPath, "\\", "/")
	payload := "MILSP-WCB/v1" +
		"\x00" + normalizedPath +
		"\x00" + blockID +
		"\x00" + docID +
		"\x00" + relation +
		"\x00" + normalizedTarget +
		"\x00" + targetSymbol +
		"\x00" + targetKind
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}