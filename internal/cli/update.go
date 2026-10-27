package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/daemon"
	"github.com/fgpaz/mi-lsp/internal/selfupdate"
	"github.com/spf13/cobra"
)

type updateHookKey struct{}
type updateHooks struct {
	version string
	check   func(context.Context) (selfupdate.Status, error)
	start   func() error
}

func setUpdateCheckForTest(root *cobra.Command, check func(context.Context) (selfupdate.Status, error)) {
	root.Context().Value(updateHookKey{}).(*updateHooks).check = check
}
func setUpdateAutomaticStartForTest(root *cobra.Command, version string, start func() error) {
	hooks := root.Context().Value(updateHookKey{}).(*updateHooks)
	hooks.version, hooks.start = version, start
}

func shouldStartAutomaticUpdate(args []string, version string, child bool) bool {
	if child {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(version))
	if v == "" || v == "dev" || v == "devel" || v == "unknown" || strings.HasPrefix(v, "(devel)") {
		return false
	}
	for _, arg := range args {
		switch arg {
		case "update", "doctor", "probe", "repair", "help", "--help", "-h":
			return false
		}
	}
	return true
}

func startAutomaticUpdate(cmd *cobra.Command, hooks *updateHooks) {
	var names []string
	for c := cmd; c != nil && c.Parent() != nil; c = c.Parent() {
		names = append(names, c.Name())
	}
	if !shouldStartAutomaticUpdate(names, hooks.version, os.Getenv("MI_LSP_UPDATE_CHILD") == "1") {
		return
	}
	if hooks.start != nil {
		_ = hooks.start()
		return
	}
	config, err := selfupdate.DefaultConfig(hooks.version)
	if err != nil {
		return
	}
	_ = selfupdate.CleanupOld(config.ExecutablePath)
	available := daemon.CompatibleUpdateOwner(cmd.Context(), config.ExecutablePath)
	_, _ = selfupdate.MaybeStartAutomatic(config, available)
}

func newUpdateCommand(state *rootState, hooks *updateHooks) *cobra.Command {
	var check bool
	cmd := &cobra.Command{Use: "update", Short: "Check or update this installation", Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&check, "check", false, "Read the last update status without changing it")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var status selfupdate.Status
		var err error
		if check && hooks.check != nil {
			status, err = hooks.check(cmd.Context())
		} else {
			config, configErr := selfupdate.DefaultConfig(hooks.version)
			if configErr != nil {
				return errors.New("update configuration unavailable")
			}
			updater := selfupdate.New(config)
			if check {
				status, err = updater.Check(cmd.Context())
			} else {
				status, err = updater.Apply(cmd.Context(), os.Getenv("MI_LSP_UPDATE_CHILD") == "1")
			}
		}
		if err != nil {
			return errors.New("update status unavailable")
		}
		if state.format == "json" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "installed: %s\nlatest: %s\nresult: %s\nchannel: %s\nauto: %t\n", status.Installed, status.Latest, status.LastResult, status.Channel, status.Auto)
		return err
	}
	return cmd
}
