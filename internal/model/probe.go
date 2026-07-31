package model

import "time"

type ProbeStatus string

const (
	ProbeStatusAbsent  ProbeStatus = "absent"
	ProbeStatusCurrent ProbeStatus = "current"
	ProbeStatusStale   ProbeStatus = "stale"
	ProbeStatusUnknown ProbeStatus = "unknown"
	ProbeStatusPartial ProbeStatus = "partial"
)

type ProbeOptions struct {
	Selector  string
	CallerCWD string
}

type ProbeWorkspace struct {
	Selector         string `json:"selector,omitempty" yaml:"selector,omitempty"`
	SelectorKind     string `json:"selector_kind,omitempty" yaml:"selector_kind,omitempty"`
	ResolutionSource string `json:"resolution_source,omitempty" yaml:"resolution_source,omitempty"`
	DisplayRoot      string `json:"display_root,omitempty" yaml:"display_root,omitempty"`
	CanonicalRoot    string `json:"canonical_root,omitempty" yaml:"canonical_root,omitempty"`
}

type ProbeState struct {
	Config          string `json:"config,omitempty" yaml:"config,omitempty"`
	Operational     string `json:"operational,omitempty" yaml:"operational,omitempty"`
	Database        string `json:"database,omitempty" yaml:"database,omitempty"`
	MigrationStatus string `json:"migration_status,omitempty" yaml:"migration_status,omitempty"`
	PortablePath    string `json:"portable_path,omitempty" yaml:"portable_path,omitempty"`
	OperationalPath string `json:"operational_path,omitempty" yaml:"operational_path,omitempty"`
	LegacyPath      string `json:"legacy_path,omitempty" yaml:"legacy_path,omitempty"`
}

type ProbeEvidence struct {
	Files    []string             `json:"files,omitempty" yaml:"files,omitempty"`
	Mtimes   map[string]time.Time `json:"mtimes,omitempty" yaml:"mtimes,omitempty"`
	DBMode   string               `json:"db_mode,omitempty" yaml:"db_mode,omitempty"`
	Warnings []string             `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

type ProbeReport struct {
	Status      ProbeStatus    `json:"status" yaml:"status"`
	SideEffects bool           `json:"side_effects" yaml:"side_effects"`
	Workspace   ProbeWorkspace `json:"workspace" yaml:"workspace"`
	State       ProbeState     `json:"state" yaml:"state"`
	Evidence    ProbeEvidence  `json:"evidence" yaml:"evidence"`
	Provenance  map[string]any `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Warnings    []string       `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

type ProbeEnvelope struct {
	Schema          string         `json:"schema" yaml:"schema"`
	Command         string         `json:"command" yaml:"command"`
	ProtocolVersion string         `json:"protocol_version" yaml:"protocol_version"`
	OK              bool           `json:"ok" yaml:"ok"`
	SideEffects     bool           `json:"side_effects" yaml:"side_effects"`
	Backend         string         `json:"backend" yaml:"backend"`
	Items           []ProbeReport  `json:"items" yaml:"items"`
	Warnings        []string       `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Error           *EnvelopeError `json:"error,omitempty" yaml:"error,omitempty"`
}
