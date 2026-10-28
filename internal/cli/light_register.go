package cli

import (
	"fmt"
	"os"

	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/workspace"
	"github.com/spf13/cobra"
)

func lightRegisterCommand() *cobra.Command {
	return &cobra.Command{Use: "internal-light-register <selector> <cwd>", Hidden: true, Args: cobra.ExactArgs(2), RunE: func(_ *cobra.Command, args []string) error {
		_, err := workspace.RegisterGitRootLight(args[0], args[1])
		return err
	}}
}

func scheduleLightRegistration(selector, cwd string) string {
	needed, err := workspace.NeedsLightRegistration(selector, cwd)
	if err != nil {
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	if !needed {
		return ""
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	cmd := processutil.Command(binary, "internal-light-register", selector, cwd)
	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Sprintf("auto_register_failed: %v", err)
	}
	return "auto_register_pending: lightweight registration scheduled; no index started"
}
