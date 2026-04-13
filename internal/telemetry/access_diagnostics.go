package telemetry

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

type batchTelemetryOperation struct {
	Op     string         `json:"op"`
	Params map[string]any `json:"params"`
}

type accessDecision struct {
	SelectorType      string `json:"selector_type,omitempty"`
	SelectorPresent   bool   `json:"selector_present"`
	RepoSelectorValid bool   `json:"repo_selector_valid"`
	PatternLen        int    `json:"pattern_len"`
	PatternHasSpaces  bool   `json:"pattern_has_spaces"`
	PatternRegexLike  bool   `json:"pattern_regex_like"`
	UsedRegex         bool   `json:"used_regex"`
	HintEmitted       bool   `json:"hint_emitted"`
	NextHintEmitted   bool   `json:"next_hint_emitted"`
	FallbackTaken     bool   `json:"fallback_taken"`
	ResultSource      string `json:"result_source,omitempty"`
}

func EnrichAccessEvent(event model.AccessEvent, request model.CommandRequest, envelope model.Envelope, opErr error) model.AccessEvent {
	focusOp, focusPayload := telemetryFocus(request)
	count := envelopeItemCount(envelope.Items)

	event.ResultCount = count
	maxItems := request.Context.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	event.Truncated = envelope.Truncated || (count > 0 && count >= maxItems)
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
		event.DecisionJSON = buildDecisionJSON(event.Route, focusPayload, envelope)
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
	if strings.EqualFold(strings.TrimSpace(envelope.Backend), "router") {
		if strings.TrimSpace(payloadStr(payload, "repo")) != "" {
			return "selector_validation"
		}
		return "router"
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
	parts := make([]string, 0, len(envelope.Warnings)+2)
	if strings.TrimSpace(envelope.Hint) != "" {
		parts = append(parts, envelope.Hint)
	}
	if envelope.NextHint != nil && strings.TrimSpace(*envelope.NextHint) != "" {
		parts = append(parts, *envelope.NextHint)
	}
	parts = append(parts, envelope.Warnings...)
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

func buildDecisionJSON(route string, payload map[string]any, envelope model.Envelope) string {
	pattern := payloadStr(payload, "pattern")
	selectorPresent := strings.TrimSpace(payloadStr(payload, "repo")) != ""
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
	}
	body, err := json.Marshal(decision)
	if err != nil {
		return ""
	}
	return string(body)
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
