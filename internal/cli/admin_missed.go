package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

func newMissedReportCommand() *cobra.Command {
	var sinceFlag string
	var outputFlag string
	command := &cobra.Command{
		Use:   "missed-report",
		Short: "Aggregate local harness tool calls mi-lsp could have answered",
		RunE: func(cmd *cobra.Command, args []string) error {
			since, err := telemetry.UsageSince(sinceFlag, time.Now())
			if err != nil {
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			roots := telemetry.DiscoverTranscriptRoots(home)
			observations := telemetry.ScanTranscripts(roots, since, 120)
			patterns := telemetry.AggregateMisses(observations)
			missed := make([]telemetry.MissedPattern, 0, 10)
			for _, pattern := range patterns {
				if pattern.Class != telemetry.Missed {
					continue
				}
				missed = append(missed, pattern)
				if len(missed) == 10 {
					break
				}
			}
			var b strings.Builder
			fmt.Fprintf(&b, "# mi-lsp missed opportunities\n\nWindow: %s. Files capped at 120 JSONL of at most 1.5MB. No prompts or file contents.\n\nRoots:\n", sinceFlag)
			if len(roots) == 0 {
				b.WriteString("- none found\n")
			}
			for _, root := range roots {
				fmt.Fprintf(&b, "- %s: %s\n", root.Harness, root.Path)
			}
			b.WriteString("\n## Top missed patterns\n\n")
			if len(missed) == 0 {
				b.WriteString("No missed patterns in the scanned window.\n")
			}
			for i, pattern := range missed {
				fmt.Fprintf(&b, "%d. %s %s shape=%s repo=%s count=%d. %s\n", i+1, pattern.Harness, pattern.Tool, pattern.PatternShape, pattern.Repo, pattern.Count, pattern.Recommendation)
			}
			if outputFlag == "" {
				_, err = fmt.Fprint(os.Stdout, b.String())
				return err
			}
			return os.WriteFile(outputFlag, []byte(b.String()), 0o600)
		},
	}
	command.Flags().StringVar(&sinceFlag, "since", "7d", "Window such as 7d or 24h")
	command.Flags().StringVar(&outputFlag, "output", "", "Write the aggregate report to this path")
	return command
}
