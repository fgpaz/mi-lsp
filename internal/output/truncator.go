package output

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func ApplyEnvelopeLimits(env model.Envelope, opts model.QueryOptions) model.Envelope {
	paginated := false

	itemsValue := reflect.ValueOf(env.Items)
	if itemsValue.IsValid() && itemsValue.Kind() == reflect.Slice && opts.MaxItems > 0 && itemsValue.Len() > opts.MaxItems {
		env.Items = sliceToLimit(itemsValue, opts.MaxItems)
		env.Truncated = true
		paginated = true
		if env.NextHint == nil {
			env.NextHint = paginationHint(opts)
		}
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
			return env
		}
		env.Items = sliceToLimit(itemsValue, itemsValue.Len()-1)
		env.Truncated = true
	}
}

func paginationHint(opts model.QueryOptions) *string {
	if opts.MaxItems <= 0 {
		return nil
	}
	nextOffset := opts.Offset + opts.MaxItems
	hint := fmt.Sprintf("rerun with --offset %d for next page", nextOffset)
	return &hint
}

func sliceToLimit(value reflect.Value, limit int) any {
	if limit < 0 {
		limit = 0
	}
	return value.Slice(0, limit).Interface()
}
