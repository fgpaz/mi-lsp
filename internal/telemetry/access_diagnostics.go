package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type batchTelemetryOperation struct {
	Op     string         `json:"op"`
	Params map[string]any `json:"params"`
}

type accessDecision struct {
	SelectorType         string `json:"selector_type,omitempty"`
	SelectorPresent      bool   `json:"selector_present"`
	RepoSelectorValid    bool   `json:"repo_selector_valid"`
	PatternLen           int    `json:"pattern_len"`
	PatternHasSpaces     bool   `json:"pattern_has_spaces"`
	PatternRegexLike     bool   `json:"pattern_regex_like"`
	UsedRegex            bool   `json:"used_regex"`
	HintEmitted          bool   `json:"hint_emitted"`
	NextHintEmitted      bool   `json:"next_hint_emitted"`
	FallbackTaken        bool   `json:"fallback_taken"`
	ResultSource         string `json:"result_source,omitempty"`
	CoachPresent         bool   `json:"coach_present"`
	CoachTrigger         string `json:"coach_trigger,omitempty"`
	CoachActionCount     int    `json:"coach_action_count,omitempty"`
	ContinuationPresent  bool   `json:"continuation_present"`
	ContinuationReason   string `json:"continuation_reason,omitempty"`
	ContinuationOp       string `json:"continuation_op,omitempty"`
	MemoryPointerPresent bool   `json:"memory_pointer_present"`
	MemoryStale          bool   `json:"memory_stale,omitempty"`
	DocRanker            string `json:"doc_ranker,omitempty"`
	IntentMode           string `json:"intent_mode,omitempty"`
	RequestedBackend     string `json:"requested_backend,omitempty"`
	ResultBackend        string `json:"result_backend,omitempty"`
	BackendFallbackTaken bool   `json:"backend_fallback_taken,omitempty"`
	FallbackFrom         string `json:"fallback_from,omitempty"`
	FallbackTo           string `json:"fallback_to,omitempty"`
	RuntimeErrorCode     string `json:"runtime_error_code,omitempty"`
	PlannerPath          string `json:"planner_path,omitempty"`
	PlannerOutcome       string `json:"planner_outcome,omitempty"`
	SafeDegradeReason    string `json:"safe_degrade_reason,omitempty"`
	GuardrailTrigger     string `json:"guardrail_trigger,omitempty"`
}

func EnrichAccessEvent(event model.AccessEvent, request model.CommandRequest, envelope model.Envelope, opErr error) model.AccessEvent {
	focusOp, focusPayload := telemetryFocus(request)
	count := envelopeItemCount(envelope.Items)
	if envelope.Error != nil {
		if strings.TrimSpace(event.Error) == "" {
			event.Error = strings.TrimSpace(envelope.Error.Message)
		}
		if strings.TrimSpace(event.ErrorKind) == "" {
			event.ErrorKind = strings.TrimSpace(envelope.Error.Kind)
		}
		if strings.TrimSpace(event.ErrorCode) == "" {
			event.ErrorCode = strings.TrimSpace(envelope.Error.Code)
		}
		if strings.TrimSpace(event.FailureStage) == "" {
			event.FailureStage = strings.TrimSpace(envelope.Error.Stage)
		}
		if strings.TrimSpace(event.HintCode) == "" {
			event.HintCode = strings.TrimSpace(envelope.Error.HintCode)
		}
	}

	event.ResultCount = count
	event.Truncated = envelope.Truncated
	event.WarningCount = len(envelope.Warnings)
	if strings.TrimSpace(event.PatternMode) == "" {
		event.PatternMode = derivePatternMode(focusOp, focusPayload)
	}
	if strings.TrimSpace(event.RoutingOutcome) == "" {
		event.RoutingOutcome = deriveRoutingOutcome(event.Route, focusPayload, envelope)
	}
	if strings.TrimSpace(event.FailureStage) == "" {
		event.FailureStage = deriveFailureStage(event.Route, focusPayload, envelope, opErr)
	}
	if strings.TrimSpace(event.HintCode) == "" {
		event.HintCode = deriveHintCode(envelope)
	}
	if strings.TrimSpace(event.TruncationReason) == "" {
		event.TruncationReason = deriveTruncationReason(event.Truncated, count, request.Context)
	}
	if strings.TrimSpace(event.DecisionJSON) == "" {
		event.DecisionJSON = buildDecisionJSON(event.Route, request, focusOp, focusPayload, envelope)
	}
	return NormalizeAccessEvent(event)
}

func telemetryFocus(request model.CommandRequest) (string, map[string]any) {
	if request.Operation != "nav.batch" {
		return request.Operation, request.Payload
	}
	rawOps, _ := request.Payload["operations"].(string)
	if strings.TrimSpace(rawOps) == "" {
		return request.Operation, request.Payload
	}
	var ops []batchTelemetryOperation
	if err := json.Unmarshal([]byte(rawOps), &ops); err != nil {
		return request.Operation, request.Payload
	}
	for _, op := range ops {
		switch op.Op {
		case "nav.search", "nav.find", "nav.intent":
			return op.Op, op.Params
		}
	}
	return request.Operation, request.Payload
}

func envelopeItemCount(items any) int {
	if rv := reflect.ValueOf(items); rv.IsValid() && rv.Kind() == reflect.Slice {
		return rv.Len()
	}
	return 0
}

func derivePatternMode(operation string, payload map[string]any) string {
	if operation != "nav.search" {
		return "none"
	}
	if payloadBool(payload, "regex") {
		return "regex"
	}
	if strings.TrimSpace(payloadStr(payload, "pattern")) != "" {
		return "literal"
	}
	return "none"
}

func deriveRoutingOutcome(route string, payload map[string]any, envelope model.Envelope) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(route), "direct_fallback"):
		return "direct_fallback"
	case strings.EqualFold(strings.TrimSpace(envelope.Backend), "router"):
		return "router_error"
	case strings.TrimSpace(payloadStr(payload, "repo")) != "":
		return "narrowed_repo"
	default:
		return "direct"
	}
}

func deriveFailureStage(route string, payload map[string]any, envelope model.Envelope, opErr error) string {
	if envelope.Error != nil {
		if stage := strings.TrimSpace(envelope.Error.Stage); stage != "" {
			return stage
		}
		switch strings.TrimSpace(envelope.Error.Kind) {
		case "transport":
			return "transport"
		case "workspace", "validation":
			return "selector_validation"
		}
		if !envelope.Ok {
			return "backend"
		}
	}
	if strings.EqualFold(strings.TrimSpace(envelope.Backend), "router") {
		if strings.TrimSpace(payloadStr(payload, "repo")) != "" {
			return "selector_validation"
		}
		return "router"
	}
	if info := ClassifyErrorInfo(envelope.Backend, "", envelope.Warnings); info.Kind == "backend_runtime" {
		return "backend_runtime"
	}
	if opErr == nil {
		return "none"
	}
	message := strings.ToLower(strings.TrimSpace(opErr.Error()))
	if strings.Contains(message, "dial") ||
		strings.Contains(message, "connect") ||
		strings.Contains(message, "connection") ||
		strings.Contains(message, "transport") ||
		strings.Contains(message, "daemon is not running") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "pipe has been ended") ||
		strings.EqualFold(strings.TrimSpace(route), "direct_fallback") {
		return "transport"
	}
	return "backend"
}

func deriveHintCode(envelope model.Envelope) string {
	if envelope.Error != nil && strings.TrimSpace(envelope.Error.HintCode) != "" {
		return strings.TrimSpace(envelope.Error.HintCode)
	}
	parts := make([]string, 0, len(envelope.Warnings)+2)
	if strings.TrimSpace(envelope.Hint) != "" {
		parts = append(parts, envelope.Hint)
	}
	if envelope.NextHint != nil && strings.TrimSpace(*envelope.NextHint) != "" {
		parts = append(parts, *envelope.NextHint)
	}
	parts = append(parts, envelope.Warnings...)
	if info := ClassifyErrorInfo(envelope.Backend, "", envelope.Warnings); info.Code != "" {
		return info.Code
	}
	message := strings.ToLower(strings.Join(parts, "\n"))
	switch {
	case strings.Contains(message, "unknown repo selector") || strings.Contains(message, "--repo <name>"):
		return "repo_selector_invalid"
	case strings.Contains(message, "search timed out"):
		return "search_timeout"
	case strings.Contains(message, "--regex") || strings.Contains(message, "regex-like"):
		return "regex_suspected"
	case strings.Contains(message, "0 matches"):
		return "no_matches"
	default:
		if envelope.Coach != nil {
			return strings.TrimSpace(envelope.Coach.Trigger)
		}
		return ""
	}
}

func deriveTruncationReason(truncated bool, count int, opts model.QueryOptions) string {
	if !truncated {
		return "none"
	}
	switch {
	case opts.MaxItems > 0 && count >= opts.MaxItems:
		return "max_items"
	case opts.MaxChars > 0:
		return "max_chars"
	case opts.TokenBudget > 0:
		return "token_budget"
	default:
		return "none"
	}
}

func buildDecisionJSON(route string, request model.CommandRequest, focusOp string, payload map[string]any, envelope model.Envelope) string {
	pattern := payloadStr(payload, "pattern")
	selectorPresent := strings.TrimSpace(payloadStr(payload, "repo")) != ""
	requestedBackend := deriveRequestedBackend(request, focusOp, payload)
	resultBackend := strings.TrimSpace(envelope.Backend)
	runtimeError := ClassifyErrorInfo(resultBackend, "", envelope.Warnings)
	decision := accessDecision{
		SelectorType:      selectorType(payload),
		SelectorPresent:   selectorPresent,
		RepoSelectorValid: selectorPresent && !strings.EqualFold(strings.TrimSpace(envelope.Backend), "router"),
		PatternLen:        len(pattern),
		PatternHasSpaces:  strings.Contains(pattern, " "),
		PatternRegexLike:  looksRegexLike(pattern),
		UsedRegex:         payloadBool(payload, "regex"),
		HintEmitted:       strings.TrimSpace(envelope.Hint) != "",
		NextHintEmitted:   envelope.NextHint != nil && strings.TrimSpace(*envelope.NextHint) != "",
		FallbackTaken:     strings.EqualFold(strings.TrimSpace(route), "direct_fallback"),
		ResultSource:      firstNonEmpty(strings.TrimSpace(envelope.Backend), "unknown"),
		DocRanker:         currentDocRankerMode(),
		RequestedBackend:  requestedBackend,
		ResultBackend:     resultBackend,
		RuntimeErrorCode:  runtimeError.Code,
		PlannerPath:       derivePlannerPath(envelope),
		PlannerOutcome:    derivePlannerOutcome(envelope),
	}
	if requestedBackend != "" && resultBackend != "" && !strings.EqualFold(requestedBackend, resultBackend) {
		decision.BackendFallbackTaken = true
		decision.FallbackFrom = requestedBackend
		decision.FallbackTo = resultBackend
	} else if runtimeError.Code != "" && resultBackend != "" {
		decision.BackendFallbackTaken = true
		decision.FallbackTo = resultBackend
	}
	if strings.EqualFold(strings.TrimSpace(envelope.Backend), "intent") && strings.TrimSpace(envelope.Mode) != "" {
		decision.IntentMode = strings.TrimSpace(envelope.Mode)
	}
	if envelope.Coach != nil {
		decision.CoachPresent = true
		decision.CoachTrigger = strings.TrimSpace(envelope.Coach.Trigger)
		decision.CoachActionCount = len(envelope.Coach.Actions)
		decision.GuardrailTrigger = strings.TrimSpace(envelope.Coach.Trigger)
	}
	if envelope.Continuation != nil {
		decision.ContinuationPresent = true
		decision.ContinuationReason = strings.TrimSpace(envelope.Continuation.Reason)
		decision.ContinuationOp = strings.TrimSpace(envelope.Continuation.Next.Op)
		decision.SafeDegradeReason = strings.TrimSpace(envelope.Continuation.Reason)
	}
	if decision.SafeDegradeReason == "" && envelope.Coach != nil {
		decision.SafeDegradeReason = strings.TrimSpace(envelope.Coach.Trigger)
	}
	if envelope.MemoryPointer != nil {
		decision.MemoryPointerPresent = true
		decision.MemoryStale = envelope.MemoryPointer.Stale
	}
	body, err := json.Marshal(decision)
	if err != nil {
		return ""
	}
	return string(body)
}

func derivePlannerPath(envelope model.Envelope) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(envelope.Backend), "planner"):
		return "planner"
	case strings.EqualFold(strings.TrimSpace(envelope.Backend), "router"):
		return "router"
	case strings.TrimSpace(envelope.Mode) != "":
		return strings.TrimSpace(envelope.Backend) + ":" + strings.TrimSpace(envelope.Mode)
	default:
		return ""
	}
}

func derivePlannerOutcome(envelope model.Envelope) string {
	mode := strings.TrimSpace(envelope.Mode)
	if strings.EqualFold(strings.TrimSpace(envelope.Backend), "planner") && mode != "" {
		return strings.ReplaceAll(mode, "-", "_")
	}
	if envelope.Coach != nil && strings.TrimSpace(envelope.Coach.Trigger) != "" {
		return strings.TrimSpace(envelope.Coach.Trigger)
	}
	return ""
}

func deriveRequestedBackend(request model.CommandRequest, focusOp string, payload map[string]any) string {
	if explicit := strings.TrimSpace(request.Context.BackendHint); explicit != "" {
		return explicit
	}
	switch focusOp {
	case "nav.context", "nav.refs":
		file := payloadStr(payload, "file")
		lower := strings.ToLower(file)
		switch {
		case strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".tsx") || strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".jsx") || strings.HasSuffix(lower, ".mts") || strings.HasSuffix(lower, ".cts"):
			return "tsserver"
		case strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".pyi"):
			return "pyright"
		case strings.HasSuffix(lower, ".cs"):
			return "roslyn"
		}
	}
	return ""
}

func currentDocRankerMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MI_LSP_DOC_RANKING"))) {
	case "legacy":
		return "legacy"
	default:
		return "owner"
	}
}

func selectorType(payload map[string]any) string {
	if strings.TrimSpace(payloadStr(payload, "repo")) != "" {
		return "repo"
	}
	return ""
}

func looksRegexLike(pattern string) bool {
	return strings.ContainsAny(pattern, "|()[]{}+?^\\")
}

func payloadStr(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func payloadBool(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	value, _ := payload[key].(bool)
	return value
}

// RedactAccessEventPaths redacts absolute paths in an AccessEvent for export (SEC-05).
// If redactPaths is true, replaces workspace_root with a sha256 hash prefix.
// Used when exporting telemetry to untrusted destinations.
func RedactAccessEventPaths(event model.AccessEvent, redactPaths bool) model.AccessEvent {
	if !redactPaths {
		return event
	}
	if strings.TrimSpace(event.WorkspaceRoot) != "" {
		hash := sha256.Sum256([]byte(event.WorkspaceRoot))
		event.WorkspaceRoot = "root_" + hex.EncodeToString(hash[:])[:12]
	}
	// Note: EntrypointPath is not currently in AccessEvent struct, but if added,
	// it should be redacted similarly.
	return event
}
