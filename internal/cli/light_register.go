package cli

import (
	"fmt"
	"os"

	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"github.com/spf13/cobra"
)

func lightRegisterCommand() *cobra.Command {
	return &cobra.Command{Use: "internal-light-register <selector> <cwd> <reservation>", Hidden: true, Args: cobra.ExactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		return workspace.RunLightRegistration(cmd.Context(), args[0], args[1], args[2])
	}}
}

func scheduleLightRegistration(selector, cwd string) string {
	reservation, owner, err := workspace.LightRegistrationReservation(selector, cwd)
	if err != nil {
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	if failure := workspace.LightRegistrationFailure(reservation); failure != "" {
		workspace.FinishLightRegistration(reservation, nil)
		return "auto_register_failed: " + failure
	}
	if !owner {
		return ""
	}
	needed, err := workspace.NeedsLightRegistration(selector, cwd)
	if err != nil {
		workspace.FinishLightRegistration(reservation, err)
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	if !needed {
		workspace.FinishLightRegistration(reservation, nil)
		return ""
	}
	binary, err := os.Executable()
	if err != nil {
		workspace.FinishLightRegistration(reservation, err)
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	cmd := processutil.Command(binary, "internal-light-register", selector, cwd, reservation)
	if err := cmd.Start(); err != nil {
		workspace.FinishLightRegistration(reservation, err)
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	return releaseLightRegistrationWorker(cmd.Process.Release)
}

// Start already succeeded. A handle-release error does not establish child exit
// and cannot authorize finishing its reservation. The worker alone finishes it.
// Per-call injection keeps the oracle private, without PATH/global factories.
func releaseLightRegistrationWorker(release func() error) string {
	if err := release(); err != nil {
		return fmt.Sprintf("auto_register_pending: worker started; process handle release failed; reservation retained: %v", err)
	}
	return "auto_register_pending: lightweight registration scheduled; no index started"
}
