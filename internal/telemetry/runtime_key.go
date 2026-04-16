package telemetry

import (
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func RuntimeKeyForOperation(request model.CommandRequest, response model.Envelope) string {
	backendType := firstNonEmpty(response.Backend, request.Context.BackendHint, payloadString(request.Payload, "backend_type"))
	if backendType == "" {
		backendType = "catalog"
	}
	workspaceName := firstNonEmpty(response.Workspace, request.Context.Workspace, "-")
	entrypoint := firstNonEmpty(
		payloadString(request.Payload, "entrypoint"),
		payloadString(request.Payload, "solution"),
		payloadString(request.Payload, "project_path"),
		payloadString(request.Payload, "repo"),
		"default",
	)
	return backendType + "::" + workspaceName + "::" + entrypoint
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}
