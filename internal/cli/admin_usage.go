package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/telemetry"
)

func newUsageReportCommand() *cobra.Command {
	var sinceFlag string
	command := &cobra.Command{
		Use:   "usage-report",
		Short: "Aggregate local mi-lsp usage without query bodies",
		RunE: func(cmd *cobra.Command, args []string) error {
			since, err := telemetry.UsageSince(sinceFlag, time.Now())
			if err != nil {
				return err
			}
			store, err := daemon.OpenTelemetryStore()
			if err != nil {
				return fmt.Errorf("cannot open telemetry store: %w", err)
			}
			defer store.Close()
			events, err := daemon.QueryAccessEvents(store, daemon.ExportQuery{Since: since})
			if err != nil {
				return err
			}
			report := telemetry.BuildUsageReport(events)
			encoded, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(os.Stdout, string(encoded))
			return err
		},
	}
	command.Flags().StringVar(&sinceFlag, "since", "7d", "Window such as 7d or 24h")
	return command
}
