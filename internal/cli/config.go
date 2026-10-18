package cli

import (
	"fmt"
	"os"

	"github.com/fgpaz/mi-lsp/internal/userconfig"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{Use: "config", Short: "Inspect user configuration"}
	configCmd.AddCommand(&cobra.Command{
		Use: "doctor", Short: "Validate the user configuration without revealing credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := userconfig.Path()
			if err != nil {
				return err
			}
			cfg, err := userconfig.Load()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\n", path)
			provider, ok := cfg.ActiveProvider()
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "embeddings: not configured (keyword-only fallback)")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "embeddings: %s (%d base URL(s), model %s)\n", provider.Type, len(provider.BaseURLs), provider.Model)
				if provider.APIKeyEnv != "" {
					_, set := os.LookupEnv(provider.APIKeyEnv)
					if !set {
						fmt.Fprintf(cmd.OutOrStdout(), "credential: environment variable %s is not set\n", provider.APIKeyEnv)
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "credential: environment variable is set")
					}
				}
				if provider.APIKeyCommand != "" {
					fmt.Fprintln(cmd.OutOrStdout(), "credential: command configured (not executed by doctor)")
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "complementos: %d\n", len(cfg.Complementos))
			return nil
		},
	})
	return configCmd
}
