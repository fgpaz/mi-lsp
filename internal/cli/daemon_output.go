package cli

import "github.com/fgpaz/mi-lsp/internal/model"

const redactedAdminToken = "[REDACTADO:token]"

func daemonStateForOutput(state model.DaemonState) model.DaemonState {
	if state.AdminToken != "" {
		state.AdminToken = redactedAdminToken
	}
	return state
}

func redactDaemonStateOutput(envelope model.Envelope) model.Envelope {
	redactItems := func(items []map[string]any) {
		for _, item := range items {
			redactDaemonStateItem(item)
		}
	}
	switch items := envelope.Items.(type) {
	case []map[string]any:
		redactItems(items)
	case []any:
		for _, value := range items {
			if item, ok := value.(map[string]any); ok {
				redactDaemonStateItem(item)
			}
		}
	}
	return envelope
}

func redactDaemonStateItem(item map[string]any) {
	state, ok := item["state"]
	if !ok {
		return
	}
	switch value := state.(type) {
	case model.DaemonState:
		item["state"] = daemonStateForOutput(value)
	case *model.DaemonState:
		if value != nil {
			item["state"] = daemonStateForOutput(*value)
		}
	case map[string]any:
		if token, exists := value["admin_token"]; exists && token != nil && token != "" {
			value["admin_token"] = redactedAdminToken
		}
	}
}
