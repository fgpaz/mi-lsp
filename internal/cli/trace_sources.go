package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Both existing trace spellings dispatch nav.trace; no new command or q stage.
type traceSourceOptions struct {
	enabled    bool
	historical bool
	docPath    string
	bindingID  string
}

func (o *traceSourceOptions) flags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&o.enabled, "binding-sources", false, "Opt in to exact canonical source declarations (not scored trace lookup)")
	cmd.Flags().StringVar(&o.docPath, "doc", "", "Exact repository-relative document path for --binding-sources")
	cmd.Flags().StringVar(&o.bindingID, "binding-id", "", "Exact source binding ID; combined with document selectors using AND")
	cmd.Flags().BoolVar(&o.historical, "historical", false, "Include deprecated/retired source declarations; requires --binding-sources")
}

func (o *traceSourceOptions) apply(payload map[string]any) error {
	if !o.enabled {
		if o.historical || o.docPath != "" || o.bindingID != "" {
			return fmt.Errorf("--doc, --binding-id and --historical require --binding-sources")
		}
		return nil
	}
	for _, key := range []string{"all", "all_workspaces", "summary"} {
		if enabled, _ := payload[key].(bool); enabled {
			return fmt.Errorf("--binding-sources cannot combine with --all, --all-workspaces or --summary")
		}
	}
	id, _ := payload["rf"].(string)
	if strings.TrimSpace(id) == "" && strings.TrimSpace(o.docPath) == "" && strings.TrimSpace(o.bindingID) == "" {
		return fmt.Errorf("--binding-sources requires DOC-ID, --doc or --binding-id")
	}
	payload["binding_sources"] = true
	if o.docPath != "" {
		payload["doc"] = o.docPath
	}
	if o.bindingID != "" {
		payload["binding_id"] = o.bindingID
	}
	if o.historical {
		payload["historical"] = true
	}
	return nil
}
