package cli

import (
	"github.com/spf13/cobra"
	"strings"
)

func newPrepareCommand(state *rootState) *cobra.Command {
	c := &cobra.Command{Use: "prepare", Short: "Create and verify portable semantic preparation packets"}
	for _, action := range []string{"create", "verify", "refresh"} {
		action := action
		var output, packet, task, intent string
		var affected []string
		cmd := &cobra.Command{Use: action, Short: strings.Title(action) + " a preparation packet", RunE: func(cmd *cobra.Command, args []string) error {
			p := map[string]any{}
			if task != "" {
				p["task"] = task
			}
			if intent != "" {
				p["intent"] = intent
			}
			if output != "" {
				p["output"] = output
			}
			if packet != "" {
				p["packet_path"] = packet
			}
			if len(affected) > 0 {
				p["affected_paths"] = affected
			}
			return state.executeOperation(cmd, "prepare."+action, p, false)
		}}
		cmd.Flags().StringVar(&task, "task", "", "Task description")
		cmd.Flags().StringVar(&intent, "intent", "read_only", "Descriptive intent")
		cmd.Flags().StringVar(&output, "output", "", "Explicit packet output path")
		cmd.Flags().StringVar(&packet, "packet-path", "", "Packet to verify")
		cmd.Flags().StringSliceVar(&affected, "affected", nil, "Allowed relative path (repeatable)")
		c.AddCommand(cmd)
	}
	return c
}
