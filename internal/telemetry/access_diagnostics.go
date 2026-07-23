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

var decisionFieldKinds = map[string]byte{
	"selector_type": 's', "selector_present": 'b', "repo_selector_valid": 'b',
	"pattern_len": 'i', "pattern_has_spaces": 'b', "pattern_regex_like": 'b',
	"used_regex": 'b', "hint_emitted": 'b', "next_hint_emitted": 'b',
	"fallback_taken": 'b', "result_source": 's', "coach_present": 'b',
	"coach_trigger": 's', "coach_action_count": 'i', "continuation_present": 'b',
	"continuation_reason": 's', "continuation_op": 's', "memory_pointer_present": 'b',
	"memory_stale": 'b', "doc_ranker": 's', "intent_mode": 's',
	"requested_backend": 's', "result_backend": 's', "backend_fallback_taken": 'b',
	"fallback_from": 's', "fallback_to": 's', "runtime_error_code": 's',
	"planner_path": 's', "planner_outcome": 's', "safe_degrade_reason": 's',
	"guardrail_trigger": 's',
}

var decisionCodeFields = map[string]struct{}{
	"coach_trigger":       {},
	"continuation_reason": {},
	"guardrail_trigger":   {},
	"planner_outcome":     {},
	"runtime_error_code":  {},
	"safe_degrade_reason": {},
}

const maxStableTelemetryCodeLength = 64

// stableTelemetryCode is the single gate for values that can become persisted
// telemetry codes. Keep this set closed: code-shaped input is not evidence that
// a value is an approved ErrorCode, HintCode, warning, or decision code.
var stableTelemetryCodeAllowlist = map[string]struct{}{
	"GPH_BACKEND_EMPTY":                         {},
	"GPH_BACKEND_MISMATCH":                      {},
	"GPH_BACKEND_UNAVAILABLE":                   {},
	"GPH_IMPACT_BUDGET_EXCEEDED":                {},
	"GPH_IMPACT_BUDGET_INVALID":                 {},
	"GPH_IMPACT_GRAPH_STALE":                    {},
	"GPH_IMPACT_RELATION_UNSUPPORTED":           {},
	"GPH_IMPACT_SEED_BUDGET_EXCEEDED":           {},
	"GPH_IMPACT_SEED_UNRESOLVED":                {},
	"GPH_MILX_CAPABILITY_DENIED":                {},
	"GPH_MILX_EXECUTABLE_DIGEST_MISMATCH":       {},
	"GPH_MILX_FRAME_INVALID":                    {},
	"GPH_MILX_MANIFEST_INVALID":                 {},
	"GPH_MILX_OUTPUT_INVALID":                   {},
	"GPH_QUERY_BACKEND_UNAVAILABLE":             {},
	"GPH_QUERY_BUDGET_INVALID":                  {},
	"GPH_QUERY_CURSOR_STALE":                    {},
	"GPH_QUERY_GENERATION_NOT_FOUND":            {},
	"GPH_QUERY_GRAPH_INVALID":                   {},
	"GPH_QUERY_SELECTOR_AMBIGUOUS":              {},
	"GPH_QUERY_SELECTOR_INVALID":                {},
	"GPH_QUERY_UTILITY_INVALID":                 {},
	"GPH_WIKI_BUDGET_EXCEEDED":                  {},
	"GPH_WIKI_CODE_AMBIGUOUS":                   {},
	"GPH_WIKI_CODE_MISSING":                     {},
	"GPH_WIKI_GRAPH_STALE":                      {},
	"GPH_WIKI_GRAPH_UNAVAILABLE":                {},
	"GPH_WIKI_PRIMARY_GRAPH_MISSING":            {},
	"INTENT_AFFECTED_UNAVAILABLE":               {},
	"INTENT_ARGUMENT_MISSING":                   {},
	"INTENT_CALLERS_OMITTED":                    {},
	"INTENT_CALLEES_OMITTED":                    {},
	"INTENT_DIFF_UNAVAILABLE":                   {},
	"INTENT_GENERATION_MISMATCH":                {},
	"INTENT_GRAPH_OMISSION":                     {},
	"INTENT_GRAPH_UNAVAILABLE":                  {},
	"INTENT_NO_CHANGED_PATHS":                   {},
	"INTENT_SELECTOR_AMBIGUOUS":                 {},
	"INTENT_TESTS_OMITTED":                      {},
	"INTENT_WIKI_EVIDENCE_OMITTED":              {},
	"active_pointer_invalid":                    {},
	"anchor_drift":                              {},
	"apply_clean_git":                           {},
	"backpressure_busy":                         {},
	"char_budget":                               {},
	"daemon_request_failed":                     {},
	"daemon_transport_failed":                   {},
	"daemon_unavailable":                        {},
	"dotnet_global_json_mismatch":               {},
	"dotnet_sdk_missing":                        {},
	"dry_run_default":                           {},
	"embeddings_failed":                         {},
	"embeddings_timeout":                        {},
	"evidence_inventory_truncated":              {},
	"expand_preview":                            {},
	"file_not_found":                            {},
	"follow_doc":                                {},
	"go_ast_only":                               {},
	"governance_blocked":                        {},
	"gopls_generic":                             {},
	"invalid_range":                             {},
	"language_not_supported":                    {},
	"low_confidence":                            {},
	"low_evidence":                              {},
	"max_chars":                                 {},
	"max_items":                                 {},
	"missing_expected_hash":                     {},
	"missing_symbol":                            {},
	"nav_generic":                               {},
	"narrow_scope":                              {},
	"no_matches":                                {},
	"no_matches_refinable":                      {},
	"no_stage_commit_format":                    {},
	"not_found":                                 {},
	"omitted_ranges":                            {},
	"operation_error":                           {},
	"operation_failed":                          {},
	"operation_required":                        {},
	"outside_workspace":                         {},
	"path_denylist":                             {},
	"plan_invalid":                              {},
	"planner_generic":                           {},
	"preview_trimmed":                           {},
	"process_spawn_access_denied":               {},
	"process_spawn_failed":                      {},
	"pyright_generic":                           {},
	"qry_edit_plan_apply_requires_experimental": {},
	"qry_edit_plan_dirty_git":                   {},
	"qry_edit_plan_generic":                     {},
	"qry_edit_plan_hash_mismatch":               {},
	"qry_edit_plan_invalid_packet":              {},
	"qry_edit_plan_language_not_supported":      {},
	"qry_edit_plan_overlap":                     {},
	"qry_edit_plan_unsafe_path":                 {},
	"read_error":                                {},
	"regex_auto_healed":                         {},
	"regex_suspected":                           {},
	"repo_selector_invalid":                     {},
	"repository_identity_missing":               {},
	"roslyn_generic":                            {},
	"roslyn_worker_bootstrap":                   {},
	"runtime_state_unavailable":                 {},
	"scope_narrowing_available":                 {},
	"scope_narrowing_required":                  {},
	"scope_preview":                             {},
	"search_failed":                             {},
	"search_timeout":                            {},
	"symbol_query_detected":                     {},
	"text_fallback":                             {},
	"text_generic":                              {},
	"token_budget":                              {},
	"tsserver_generic":                          {},
	"validation_failed":                         {},
	"warning_present":                           {},
	"workspace_cross_workspace_refused":         {},
	"workspace_resolution_failed":               {},
	"workspace_unresolved":                      {},
}

func sanitizePersistedAccessFields(repo string, warnings []string, errorText, errorCode, hintCode, backend string) (string, []string, string, string, string) {
	if strings.TrimSpace(repo) != "" {
		repo = "selected"
	}

	safeErrorCode := stableTelemetryCode(errorCode)
	safeHintCode := stableTelemetryCode(hintCode)
	codes := make([]string, 0, len(warnings))
	seen := make(map[string]struct{}, len(warnings))
	for _, warning := range warnings {
		code := firstStableCode(errorCode, hintCode)
		if code == "" {
			code = stableTelemetryCode(ClassifyErrorInfo(backend, "", []string{warning}).Code)
		}
		if code == "" {
			code = stableTelemetryCode("warning_present")
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}

	safeError := ""
	if strings.TrimSpace(errorText) != "" {
		safeError = safeErrorCode
		if safeError == "" {
			safeError = stableTelemetryCode("operation_error")
		}
	}
	return repo, codes, safeError, safeErrorCode, safeHintCode
}

func firstStableCode(values ...string) string {
	for _, value := range values {
		if code := stableTelemetryCode(value); code != "" {
			return code
		}
	}
	return ""
}

func stableTelemetryCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxStableTelemetryCodeLength {
		return ""
	}
	if _, ok := stableTelemetryCodeAllowlist[value]; !ok {
		return ""
	}
	return value
}

func normalizeDecisionJSON(raw string, event model.AccessEvent) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return ""
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		switch decisionFieldKinds[key] {
		case 'b':
			var typed bool
			if json.Unmarshal(value, &typed) == nil {
				output[key] = typed
			}
		case 'i':
			var typed int
			if json.Unmarshal(value, &typed) == nil {
				output[key] = typed
			}
		case 's':
			var typed string
			if json.Unmarshal(value, &typed) != nil {
				continue
			}
			if _, isCode := decisionCodeFields[key]; isCode {
				if code := stableTelemetryCode(typed); code != "" {
					output[key] = code
				}
				continue
			}
			if decisionTokenAllowed(typed, event) {
				output[key] = strings.TrimSpace(typed)
			}
		}
	}
	if len(output) == 0 {
		return ""
	}
	body, err := json.Marshal(output)
	if err != nil {
		return ""
	}
	return string(body)
}

func decisionTokenAllowed(value string, event model.AccessEvent) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, candidate := range []string{
		event.Operation, event.Intent, event.Backend, event.Route, event.Format,
		event.ErrorKind, event.PatternMode, event.RoutingOutcome,
		event.FailureStage, event.TruncationReason, event.WorkspaceAlias,
	} {
		if value == strings.TrimSpace(candidate) {
			return true
		}
	}
	switch value {
	case "repo", "unknown", "owner", "legacy", "none", "direct", "daemon", "direct_fallback", "router_error", "narrowed_repo", "router", "text", "roslyn", "tsserver", "pyright", "planner", "intent", "catalog", "worker", "governance", "docs", "code", "ask", "literal", "regex", "present", "low_evidence", "scope_preview", "nav.search", "nav.find", "nav.intent", "nav.refs", "nav.context", "nav.related", "nav.workspace-map", "nav.wiki.pack":
		return true
	default:
		return value == "intent:docs" || value == "intent:code"
	}
}

// IntentFromRequestEnvelope extracts only the allowlisted intent token. It handles
// both in-process typed plans and JSON-decoded cache envelopes.
func IntentFromRequestEnvelope(request model.CommandRequest, envelope model.Envelope) string {
	if explicit := model.SanitizeUtilityIntent(payloadStr(request.Payload, "intent")); explicit != "unknown" {
		return explicit
	}
	if intent := intentFromItems(envelope.Items); intent != "" {
		return model.SanitizeUtilityIntent(intent)
	}
	return "unknown"
}

func intentFromItems(items any) string {
	switch typed := items.(type) {
	case []model.IntentPlan:
		if len(typed) > 0 {
			return typed[0].Intent
		}
	case model.IntentPlan:
		return typed.Intent
	case []any:
		for _, item := range typed {
			if intent := intentFromItems(item); intent != "" {
				return intent
			}
		}
	case []map[string]any:
		for _, item := range typed {
			if intent := intentFromItems(item); intent != "" {
				return intent
			}
		}
	case map[string]any:
		if intent, ok := typed["intent"].(string); ok {
			return intent
		}
	}
	return ""
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
		return stableTelemetryCode(envelope.Error.HintCode)
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
		return stableTelemetryCode(info.Code)
	}
	message := strings.ToLower(strings.Join(parts, "\n"))
	switch {
	case strings.Contains(message, "unknown repo selector") || strings.Contains(message, "--repo <name>"):
		return stableTelemetryCode("repo_selector_invalid")
	case strings.Contains(message, "search timed out"):
		return stableTelemetryCode("search_timeout")
	case strings.Contains(message, "--regex") || strings.Contains(message, "regex-like"):
		return stableTelemetryCode("regex_suspected")
	case strings.Contains(message, "0 matches"):
		return stableTelemetryCode("no_matches")
	default:
		if envelope.Coach != nil {
			return stableTelemetryCode(envelope.Coach.Trigger)
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
	runtimeError := stableTelemetryCode(ClassifyErrorInfo(resultBackend, "", envelope.Warnings).Code)
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
		RuntimeErrorCode:  runtimeError,
		PlannerPath:       derivePlannerPath(envelope),
		PlannerOutcome:    derivePlannerOutcome(envelope),
	}
	if requestedBackend != "" && resultBackend != "" && !strings.EqualFold(requestedBackend, resultBackend) {
		decision.BackendFallbackTaken = true
		decision.FallbackFrom = requestedBackend
		decision.FallbackTo = resultBackend
	} else if runtimeError != "" && resultBackend != "" {
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
