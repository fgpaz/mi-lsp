package milx

import "encoding/json"

const (
	ManifestSchema         = "milx-manifest/v1"
	EnvelopeSchema         = "milx-envelope/v1"
	ProtocolVersion uint32 = 1
	MaxFrameBytes          = 1 << 20
	MaxRequestID           = 128
)

type Manifest struct {
	Schema           string            `json:"schema"`
	ExtensionID      string            `json:"extension_id"`
	ExtensionVersion string            `json:"extension_version"`
	ExecutableSHA256 string            `json:"executable_sha256"`
	ProtocolMin      uint32            `json:"protocol_min"`
	ProtocolMax      uint32            `json:"protocol_max"`
	Operations       []string          `json:"operations"`
	InputSchemas     []string          `json:"input_schemas"`
	OutputSchemas    []string          `json:"output_schemas"`
	Capabilities     []string          `json:"capabilities"`
	Deterministic    bool              `json:"deterministic"`
	ResourceHints    map[string]uint64 `json:"resource_hints,omitempty"`
	PackFamilies     []string          `json:"pack_families,omitempty"`
}

type Envelope struct {
	Schema          string          `json:"schema"`
	RequestID       string          `json:"request_id"`
	Operation       string          `json:"operation"`
	ProtocolVersion uint32          `json:"protocol_version"`
	Status          string          `json:"status,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

type Pack struct {
	Schema       string          `json:"schema"`
	GenerationID string          `json:"generation_id"`
	Selection    json.RawMessage `json:"selection"`
	Provenance   Provenance      `json:"provenance"`
	Digest       string          `json:"digest"`
	Omissions    []Omission      `json:"omissions"`
}

type Result struct {
	Schema       string          `json:"schema"`
	Result       json.RawMessage `json:"result"`
	ResultDigest string          `json:"result_digest"`
	Provenance   Provenance      `json:"provenance"`
	Omissions    []Omission      `json:"omissions"`
}

type Provenance struct {
	GenerationID     string `json:"generation_id"`
	ExtensionID      string `json:"extension_id"`
	ExtensionVersion string `json:"extension_version"`
	ParametersDigest string `json:"parameters_digest"`
}
type Omission struct {
	Code    string `json:"code"`
	Count   uint32 `json:"count"`
	Summary string `json:"summary,omitempty"`
}
type ErrorResponse struct {
	Code             string `json:"code"`
	Stage            string `json:"stage"`
	Retryable        bool   `json:"retryable"`
	Hint             string `json:"hint,omitempty"`
	SanitizedSummary string `json:"sanitized_summary"`
}
type MILXError struct {
	Code, Stage            string
	Retryable              bool
	Hint, SanitizedSummary string
}

func (e *MILXError) Error() string { return e.Code + ": " + e.SanitizedSummary }

type LifecycleState string

const (
	StateSpawned   LifecycleState = "spawned"
	StateDescribed LifecycleState = "described"
	StatePrepared  LifecycleState = "prepared"
	StateExecuting LifecycleState = "executing"
	StateShutdown  LifecycleState = "shutdown"
)

func ValidTransition(from LifecycleState, operation string) bool {
	switch operation {
	case "describe":
		return from == StateSpawned
	case "prepare":
		return from == StateDescribed
	case "execute":
		return from == StatePrepared
	case "cancel":
		return from == StateExecuting
	case "health":
		return from != StateShutdown
	case "shutdown":
		return from != StateShutdown
	}
	return false
}
