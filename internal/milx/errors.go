package milx

import "fmt"

var errorCodes = map[string]bool{"GPH_MILX_PROTOCOL_UNSUPPORTED": true, "GPH_MILX_MANIFEST_INVALID": true, "GPH_MILX_EXECUTABLE_DIGEST_MISMATCH": true, "GPH_MILX_CAPABILITY_DENIED": true, "GPH_MILX_STATE_INVALID": true, "GPH_MILX_FRAME_INVALID": true, "GPH_MILX_OUTPUT_INVALID": true, "GPH_MILX_TIMEOUT": true, "GPH_MILX_PROCESS_FAILED": true, "GPH_MILX_CLEANUP_FAILED": true, "GPH_MILX_NETWORK_FORBIDDEN": true, "GPH_MILX_MCP_FORBIDDEN": true}

func NewError(code, stage string, retryable bool, hint, summary string) *MILXError {
	if !errorCodes[code] {
		code = "GPH_MILX_OUTPUT_INVALID"
	}
	return &MILXError{Code: code, Stage: stage, Retryable: retryable, Hint: hint, SanitizedSummary: summary}
}
func (e *MILXError) Response() ErrorResponse {
	return ErrorResponse{Code: e.Code, Stage: e.Stage, Retryable: e.Retryable, Hint: e.Hint, SanitizedSummary: e.SanitizedSummary}
}
func ValidateEnvelope(e Envelope, response bool) error {
	if e.Schema != EnvelopeSchema || e.RequestID == "" || len(e.RequestID) > MaxRequestID || !allowedOperations[e.Operation] || e.ProtocolVersion != ProtocolVersion || len(e.Payload) == 0 {
		return NewError("GPH_MILX_OUTPUT_INVALID", "envelope", false, "", "envelope required fields are invalid")
	}
	if response && !map[string]bool{"ok": true, "rejected": true, "canceled": true, "timeout": true, "failed": true}[e.Status] {
		return NewError("GPH_MILX_OUTPUT_INVALID", "envelope", false, "", "response status is invalid")
	}
	return nil
}
func ValidateOperationPayload(operation string, p []byte) error {
	if !allowedOperations[operation] {
		return fmt.Errorf("unknown operation")
	}
	var value any
	if err := DecodeCanonical(p, &value); err != nil {
		return fmt.Errorf("invalid operation payload: %w", err)
	}
	if operation == "describe" || operation == "health" || operation == "shutdown" {
		if string(p) != "{}" && string(p) != "null" {
			return fmt.Errorf("operation payload must be empty")
		}
	} else if value == nil {
		return fmt.Errorf("operation payload is required")
	}
	return nil
}
