package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/service"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func newProbeCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "probe [selector]",
		Short: "Inspect workspace identity and state without mutating it",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("probe accepts at most one selector")
			}
			if len(args) == 1 && cmd.Flags().Changed("workspace") {
				return fmt.Errorf("probe selector and --workspace cannot both be used")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			selector := state.workspace
			if len(args) == 1 {
				selector = args[0]
			}
			callerCWD, _ := os.Getwd()
			report, probeErr := service.ProbeWorkspace(cmd.Context(), model.ProbeOptions{Selector: selector, CallerCWD: callerCWD})
			if report.Provenance == nil {
				report.Provenance = map[string]any{}
			}
			version := buildRootVersionInfo(state.repoRoot)
			encodedVersion, marshalErr := json.Marshal(version)
			if marshalErr == nil {
				var versionMap map[string]any
				if json.Unmarshal(encodedVersion, &versionMap) == nil {
					report.Provenance["version"] = versionMap
				}
			}

			envelope := model.ProbeEnvelope{
				Schema:          "mi-lsp/workspace-probe/v1",
				Command:         "mi-lsp probe",
				ProtocolVersion: model.ProtocolVersion,
				OK:              probeErr == nil,
				SideEffects:     false,
				Backend:         "workspace-probe",
				Items:           []model.ProbeReport{report},
				Warnings:        report.Warnings,
			}
			if probeErr != nil {
				code, message := probeEnvelopeError(probeErr)
				envelope.Error = &model.EnvelopeError{Kind: "validation", Code: code, Message: message, Stage: "selector_validation", HintCode: code}
			}
			rendered, renderErr := renderProbeEnvelope(envelope, state.format)
			if renderErr != nil {
				return renderErr
			}
			if _, printErr := fmt.Fprintln(os.Stdout, string(rendered)); printErr != nil {
				return printErr
			}
			if probeErr != nil {
				return envelopePrintedError{err: probeErr}
			}
			return nil
		},
	}
	return command
}

func renderProbeEnvelope(envelope model.ProbeEnvelope, format string) ([]byte, error) {
	switch format {
	case "text":
		// Probe output is contractual in every supported format, including the
		// legacy text selector. Keep the envelope fields machine-readable rather
		// than dropping schema, protocol, or side-effect provenance.
		return json.MarshalIndent(envelope, "", "  ")
	case "yaml", "toon":
		return yaml.Marshal(envelope)
	case "compact":
		return json.Marshal(envelope)
	default:
		return json.MarshalIndent(envelope, "", "  ")
	}
}

func probeEnvelopeError(err error) (string, string) {
	if selectorErr, ok := workspace.AsWorkspaceSelectorError(err); ok {
		return selectorErr.Code, selectorErr.Error()
	}
	return "WKS_PROBE_FAILED", err.Error()
}
