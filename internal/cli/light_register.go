package cli

import (
	"errors"
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
	if reservation == "" && !owner {
		return ""
	} // No eligible Git root, not uncertain cleanup.
	finish := func(workErr error) string {
		return finishLightRegistrationResult(workErr, func(err error) error { return workspace.FinishLightRegistration(reservation, err) })
	}
	failure, readErr := workspace.LightRegistrationFailureResult(reservation)
	if readErr != nil {
		if owner {
			return finish(fmt.Errorf("prior registration failure unavailable: %w", readErr))
		}
		return fmt.Sprintf("auto_register_pending: prior failure state unavailable; no ownership assumed: %v", readErr)
	}
	if failure != "" {
		if owner {
			return finishPriorLightRegistrationFailure(failure, func(err error) error { return workspace.FinishLightRegistration(reservation, err) })
		}
		return "auto_register_failed: " + failure
	}
	if !owner {
		return "auto_register_pending: reservation busy or indeterminate; no worker started by this query"
	}
	needed, err := workspace.NeedsLightRegistration(selector, cwd)
	if err != nil {
		return finish(err)
	}
	if !needed {
		return finish(nil)
	}
	binary, err := os.Executable()
	if err != nil {
		return finish(err)
	}
	cmd := processutil.Command(binary, "internal-light-register", selector, cwd, reservation)
	if err := cmd.Start(); err != nil {
		return finish(err)
	}
	return releaseLightRegistrationWorker(cmd.Process.Release)
}

func finishPriorLightRegistrationFailure(failure string, finish func(error) error) string {
	// Already consumed old failure: clear only our fresh reservation, never
	// republish the old error and create a permanent failure-replay cycle.
	if warning := finishLightRegistrationResult(nil, finish); warning != "" {
		return warning + "; prior failure: " + failure
	}
	return "auto_register_failed: " + failure
}

// A cleanup error cannot be turned into an empty warning even when no registry
// work was needed. Error does not prove retention OR release; report uncertainty.
func finishLightRegistrationResult(workErr error, finish func(error) error) string {
	if cleanupErr := finish(workErr); cleanupErr != nil {
		return fmt.Sprintf("auto_register_pending: reservation cleanup unconfirmed: %v", errors.Join(workErr, cleanupErr))
	}
	if workErr != nil {
		return fmt.Sprintf("auto_register_failed: %v", workErr)
	}
	return ""
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
