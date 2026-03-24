package output

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func ApplyEnvelopeLimits(env model.Envelope, opts model.QueryOptions) model.Envelope {
	itemsValue := reflect.ValueOf(env.Items)
	if itemsValue.IsValid() && itemsValue.Kind() == reflect.Slice && opts.MaxItems > 0 && itemsValue.Len() > opts.MaxItems {
		env.Items = sliceToLimit(itemsValue, opts.MaxItems)
		env.Truncated = true
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
		if !itemsValue.IsValid() || itemsValue.Kind() != reflect.Slice || itemsValue.Len() == 0 {
			hint := fmt.Sprintf("response exceeded %d chars; narrow the query or raise the limits", maxChars)
			env.NextHint = &hint
			env.Truncated = true
			return env
		}
		env.Items = sliceToLimit(itemsValue, itemsValue.Len()-1)
		env.Truncated = true
	}
}

func sliceToLimit(value reflect.Value, limit int) any {
	if limit < 0 {
		limit = 0
	}
	return value.Slice(0, limit).Interface()
}
