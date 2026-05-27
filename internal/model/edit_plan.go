package model

const (
	EditPlanVersionV1 = "edit-plan-v1"
	EditPlanVersionV2 = "edit-plan-v2"
	EditPlanVersion   = EditPlanVersionV1
)

type EditPlanRequest struct {
	Version     string              `json:"version"`
	Intent      string              `json:"intent,omitempty"`
	BaseRef     string              `json:"base_ref,omitempty"`
	Targets     []EditPlanTarget    `json:"targets"`
	Operations  []EditPlanOperation `json:"operations"`
	Constraints EditPlanConstraints `json:"constraints,omitempty"`
}

type EditPlanTarget struct {
	ID           string          `json:"id"`
	Path         string          `json:"path"`
	Language     string          `json:"language,omitempty"`
	Range        EditPlanRange   `json:"range"`
	ExpectedHash string          `json:"expected_hash,omitempty"`
	Symbol       *EditPlanSymbol `json:"symbol,omitempty"`
}

type EditPlanRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

type EditPlanSymbol struct {
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Receiver  string `json:"receiver,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type EditPlanOperation struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	TargetID        string `json:"target_id"`
	Find            string `json:"find,omitempty"`
	Replace         string `json:"replace,omitempty"`
	Content         string `json:"content,omitempty"`
	ImportPath      string `json:"import_path,omitempty"`
	ImportAlias     string `json:"import_alias,omitempty"`
	MaxReplacements int    `json:"max_replacements,omitempty"`
}

type EditPlanConstraints struct {
	RequireCleanMatch bool     `json:"require_clean_match,omitempty"`
	RequireEvidence   bool     `json:"require_evidence,omitempty"`
	DenyPaths         []string `json:"deny_paths,omitempty"`
	MaxFileBytes      int      `json:"max_file_bytes,omitempty"`
	MaxDiffChars      int      `json:"max_diff_chars,omitempty"`
}

type EditPlanResult struct {
	PatchPacket  EditPlanRequest           `json:"patch_packet"`
	Diff         string                    `json:"diff,omitempty"`
	FilesChanged int                       `json:"files_changed"`
	Operations   []EditPlanOperationResult `json:"operations"`
	Evidence     []EditPlanEvidence        `json:"evidence,omitempty"`
	Guardrails   []EditPlanGuardrail       `json:"guardrails,omitempty"`
	ApplyStatus  EditPlanApplyStatus       `json:"apply_status"`
}

type EditPlanOperationResult struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	TargetID     string `json:"target_id"`
	Path         string `json:"path,omitempty"`
	Status       string `json:"status"`
	Replacements int    `json:"replacements,omitempty"`
	Message      string `json:"message,omitempty"`
}

type EditPlanEvidence struct {
	Kind  string `json:"kind"`
	Path  string `json:"path,omitempty"`
	Value string `json:"value,omitempty"`
	Line  int    `json:"line,omitempty"`
}

type EditPlanGuardrail struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type EditPlanApplyStatus struct {
	Requested bool     `json:"requested"`
	Applied   bool     `json:"applied"`
	Rollback  bool     `json:"rollback"`
	Files     []string `json:"files,omitempty"`
	Message   string   `json:"message,omitempty"`
}
