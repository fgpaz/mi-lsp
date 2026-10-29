package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/spf13/cobra"
)

// nativeEvent contains only bounded metadata, never source, paths or prompts.
type nativeEvent struct {
	Client    string `json:"client"`
	Operation string `json:"operation"`
	Outcome   string `json:"outcome"`
	LatencyMS *int64 `json:"latency_ms"`
}

func newNativeEventCommand() *cobra.Command {
	return &cobra.Command{Use: "native-event", Short: "Record one completed native LSP operation from JSON stdin", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 4097))
		if err != nil || len(body) > 4096 {
			return errors.New("event exceeds input limit or cannot be read")
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		var in nativeEvent
		if err := dec.Decode(&in); err != nil {
			return fmt.Errorf("invalid event: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return errors.New("expected exactly one event")
		}
		if in.Client != "claude-code" && in.Client != "pi" {
			return errors.New("invalid client")
		}
		var op string
		switch in.Operation {
		case "definition":
			op = "nav.definition"
		case "references":
			op = "nav.refs"
		case "callers":
			op = "nav.callers"
		case "diagnostics":
			op = "nav.diagnostics"
		default:
			return errors.New("invalid operation")
		}
		if in.Outcome != "ok" && in.Outcome != "fallback" && in.Outcome != "error" {
			return errors.New("invalid outcome")
		}
		if in.LatencyMS == nil || *in.LatencyMS < 0 || *in.LatencyMS > 86400000 {
			return errors.New("invalid latency_ms")
		}
		event := model.AccessEvent{OccurredAt: time.Now().UTC(), ClientName: in.Client, Operation: op, Backend: "native-lsp", Route: "direct", Success: in.Outcome == "ok", LatencyMs: *in.LatencyMS, RoutingOutcome: "direct"}
		if in.Outcome == "fallback" {
			event.Route = "direct_fallback"
			event.RoutingOutcome = "direct_fallback"
		}
		if in.Outcome == "error" {
			event.ErrorKind = "native_lsp"
			event.ErrorCode = "native_error"
		}
		store, err := daemon.OpenTelemetryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.RecordAccessDirect(event); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), `{"ok":true}`)
		return err
	}}
}
