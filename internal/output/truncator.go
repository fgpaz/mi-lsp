package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func ApplyEnvelopeLimits(env model.Envelope, opts model.QueryOptions) model.Envelope {
	paginated := false
	omittedItems := 0

	itemsValue := reflect.ValueOf(env.Items)
	if itemsValue.IsValid() && itemsValue.Kind() == reflect.Slice && opts.MaxItems > 0 && itemsValue.Len() > opts.MaxItems {
		omittedItems += itemsValue.Len() - opts.MaxItems
		env.Items = sliceToLimit(itemsValue, opts.MaxItems)
		env.Truncated = true
		paginated = true
		if env.NextHint == nil {
			env.NextHint = paginationHint(opts)
		}
		env.Omissions = append(env.Omissions, model.EnvelopeOmission{
			Reason:    fmt.Sprintf("max_items omitted %d item(s)", omittedItems),
			ErrorCode: "max_items",
		})
	}

	maxChars := opts.MaxChars
	if maxChars == 0 && opts.TokenBudget > 0 {
		maxChars = opts.TokenBudget * 4
	}
	if maxChars <= 0 {
		return env
	}

	for {
		payload, _ := json.Marshal(env)
		if len(payload) <= maxChars {
			return env
		}
		itemsValue = reflect.ValueOf(env.Items)
		if !itemsValue.IsValid() || itemsValue.Kind() != reflect.Slice || itemsValue.Len() <= 1 {
			env.Truncated = true
			if !paginated && env.NextHint == nil {
				hint := fmt.Sprintf("response exceeded %d chars; raise --token-budget or --max-chars, or use --format compact", maxChars)
				env.NextHint = &hint
			}
			if omittedItems == 0 {
				env.Omissions = append(env.Omissions, model.EnvelopeOmission{
					Reason:    fmt.Sprintf("response exceeded %d chars", maxChars),
					ErrorCode: "char_budget",
				})
			}
			return env
		}
		omittedItems++
		env.Items = sliceToLimit(itemsValue, itemsValue.Len()-1)
		env.Truncated = true
		if !hasOmissionCode(env.Omissions, "char_budget") {
			env.Omissions = append(env.Omissions, model.EnvelopeOmission{
				Reason:    "char budget removed trailing item(s)",
				ErrorCode: "char_budget",
			})
		}
	}
}

func hasOmissionCode(omissions []model.EnvelopeOmission, code string) bool {
	for _, omission := range omissions {
		if omission.ErrorCode == code {
			return true
		}
	}
	return false
}

func paginationHint(opts model.QueryOptions) *string {
	if opts.MaxItems <= 0 {
		return nil
	}
	nextOffset := opts.Offset + opts.MaxItems
	hint := fmt.Sprintf("rerun with --offset %d for next page", nextOffset)
	return &hint
}

// CapRendered bounds already-rendered CLI text. maxChars <= 0, or text that
// already fits, is returned unchanged. Over budget, the continuation value is
// copied verbatim and a truncation marker is inserted. Rendered bytes are not
// unquoted, so Windows paths keep backslash-r and backslash-m as written.
func CapRendered(rendered []byte, maxChars int) []byte {
	if maxChars <= 0 || len(rendered) <= maxChars {
		return rendered
	}
	start, end, ok := continuationSpan(rendered)
	if !ok {
		return prefixWithin(rendered, maxChars)
	}
	block := rendered[start:end]
	marker := []byte(fmt.Sprintf("...[truncated %d chars; continuation preserved]...", len(rendered)-maxChars))
	budget := maxChars - len(marker) - len(block)
	head := prefixWithin(rendered[:start], budget)
	out := make([]byte, 0, len(head)+len(marker)+len(block))
	out = append(out, head...)
	out = append(out, marker...)
	out = append(out, block...)
	return out
}

func prefixWithin(b []byte, budget int) []byte {
	if budget <= 0 || len(b) == 0 {
		return nil
	}
	if len(b) <= budget {
		return append([]byte(nil), b...)
	}
	cut := budget
	for cut > 0 && !utf8.RuneStart(b[cut]) {
		cut--
	}
	if cut <= 0 {
		return nil
	}
	return append([]byte(nil), b[:cut]...)
}

func continuationSpan(rendered []byte) (int, int, bool) {
	trimmed := bytes.TrimLeft(rendered, " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if start, end, ok := jsonContinuationSpan(rendered); ok {
			return start, end, true
		}
	}
	return textContinuationSpan(rendered)
}

func jsonContinuationSpan(src []byte) (int, int, bool) {
	start, end := 0, 0
	found := false
	i := 0
	for i < len(src) {
		if src[i] != '"' {
			i++
			continue
		}
		stringEnd := jsonStringEnd(src, i)
		if stringEnd <= i {
			i++
			continue
		}
		if string(src[i+1:stringEnd-1]) == "continuation" && nextNonWSByte(src, stringEnd) == ':' {
			valueStart := skipWS(src, stringEnd)
			if valueStart < len(src) && src[valueStart] == ':' {
				valueStart = skipWS(src, valueStart+1)
				start, end, found = i, jsonValueEnd(src, valueStart), true
			}
		}
		i = stringEnd
	}
	return start, end, found
}

func textContinuationSpan(src []byte) (int, int, bool) {
	const key = "continuation:"
	from := 0
	for from < len(src) {
		rel := bytes.Index(src[from:], []byte(key))
		if rel < 0 {
			return 0, 0, false
		}
		start := from + rel
		if start != 0 && src[start-1] != '\n' {
			from = start + len(key)
			continue
		}
		lineStart := start
		for lineStart < len(src) {
			lineEnd := lineStart
			for lineEnd < len(src) && src[lineEnd] != '\n' {
				lineEnd++
			}
			if lineStart != start {
				if lineEnd == lineStart || (src[lineStart] != ' ' && src[lineStart] != '\t') {
					return start, lineStart, true
				}
			}
			if lineEnd >= len(src) {
				return start, len(src), true
			}
			lineStart = lineEnd + 1
		}
		return start, len(src), true
	}
	return 0, 0, false
}

func jsonValueEnd(src []byte, i int) int {
	i = skipWS(src, i)
	if i >= len(src) {
		return len(src)
	}
	switch src[i] {
	case '"':
		return jsonStringEnd(src, i)
	case '{', '[':
		return jsonContainerEnd(src, i)
	default:
		j := i
		for j < len(src) {
			switch src[j] {
			case ',', '}', ']', ' ', '\n', '\r', '\t':
				return j
			default:
				j++
			}
		}
		return j
	}
}

func jsonContainerEnd(src []byte, i int) int {
	open := src[i]
	close := byte('}')
	if open == '[' {
		close = ']'
	}
	depth := 0
	for i < len(src) {
		switch src[i] {
		case '"':
			i = jsonStringEnd(src, i)
		case open:
			depth++
			i++
		case close:
			depth--
			i++
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return len(src)
}

func jsonStringEnd(src []byte, i int) int {
	if i >= len(src) || src[i] != '"' {
		return i
	}
	i++
	for i < len(src) {
		if src[i] == '\\' {
			if i+1 >= len(src) {
				return len(src)
			}
			if src[i+1] == 'u' && i+6 <= len(src) {
				i += 6
				continue
			}
			i += 2
			continue
		}
		if src[i] == '"' {
			return i + 1
		}
		i++
	}
	return len(src)
}

func skipWS(src []byte, i int) int {
	for i < len(src) {
		switch src[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func nextNonWSByte(src []byte, i int) byte {
	i = skipWS(src, i)
	if i >= len(src) {
		return 0
	}
	return src[i]
}

func sliceToLimit(value reflect.Value, limit int) any {
	if limit < 0 {
		limit = 0
	}
	return value.Slice(0, limit).Interface()
}
