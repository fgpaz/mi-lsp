package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/spf13/cobra"
)

var directQSessions = service.NewSessionState()

func newQCommand(state *rootState) *cobra.Command {
	var budget, maxBytes, timeoutMS int
	var page string
	var fresh, dedupe bool
	command := &cobra.Command{
		Use: "q <pipeline>", Short: "Run a bounded q-v1 semantic query pipeline",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{"q": args[0], "page": page, "fresh": fresh, "dedupe": dedupe}
			if cmd.Flags().Changed("budget") {
				payload["budget"] = budget
			}
			if cmd.Flags().Changed("max-bytes") {
				payload["max_bytes"] = maxBytes
			}
			if cmd.Flags().Changed("timeout-ms") {
				payload["timeout_ms"] = timeoutMS
			}
			opts := state.queryOptions(cmd, "q", payload)
			if opts.SessionID != "" {
				payload["session_id"] = opts.SessionID
			}
			request := model.CommandRequest{ProtocolVersion: model.ProtocolVersion, Operation: "q", Context: opts, Payload: payload}
			deadline := time.Duration(timeoutMS) * time.Millisecond
			if deadline <= 0 {
				deadline = 10 * time.Second
			}
			if deadline > 30*time.Second {
				deadline = 30 * time.Second
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), deadline)
			defer cancel()
			if !state.noDaemon {
				if !state.noAutoDaemon {
					ensure := daemon.EnsureDaemon
					if state.ensureDaemon != nil {
						ensure = state.ensureDaemon
					}
					_ = ensure(state.repoRoot)
				}
				if envelope, err := daemon.NewClient().Execute(ctx, request); err == nil {
					return state.printEnvelope(envelope, opts)
				}
			}
			envelope, err := state.app.ExecuteQ(ctx, request, directQSessions)
			if err != nil {
				return fmt.Errorf("q: %w", err)
			}
			if envelope.FallbackUsed == "" {
				if state.noDaemon {
					envelope.FallbackUsed = "direct"
				} else {
					envelope.FallbackUsed = "direct_fallback"
				}
			}
			return state.printEnvelope(envelope, opts)
		},
	}
	command.Flags().IntVar(&budget, "budget", 2000, "Approximate token budget for q items (max 12000)")
	command.Flags().IntVar(&maxBytes, "max-bytes", 262144, "Maximum serialized q-v1 envelope bytes")
	command.Flags().IntVar(&timeoutMS, "timeout-ms", 10000, "End-to-end q timeout (1-30000 ms)")
	command.Flags().StringVar(&page, "page", "", "Opaque q-v1 continuation cursor")
	command.Flags().BoolVar(&fresh, "fresh", false, "Return previously delivered source text again")
	command.Flags().BoolVar(&dedupe, "dedupe", false, "Opt in to session-scoped result deduplication")
	return command
}
