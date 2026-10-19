package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fgpaz/mi-lsp/internal/userconfig"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{Use: "config", Short: "Inspect user configuration"}
	configCmd.AddCommand(&cobra.Command{
		Use:   "setup",
		Short: "Create a local configuration file without changing existing settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := userconfig.Path()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				if os.IsExist(err) {
					fmt.Fprintf(cmd.OutOrStdout(), "config: already exists; unchanged (%s)\n", path)
					return nil
				}
				return fmt.Errorf("create config file: %w", err)
			}
			if _, err := fmt.Fprintln(f, "# Optional mi-lsp settings. Add an embeddings provider only when needed."); err != nil {
				f.Close()
				os.Remove(path)
				return fmt.Errorf("write config file: %w", err)
			}
			if err := f.Close(); err != nil {
				os.Remove(path)
				return fmt.Errorf("close config file: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config: created (%s)\n", path)
			return nil
		},
	})
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
				fmt.Fprintln(cmd.OutOrStdout(), "missing: provider selection, provider type, base_urls, model (optional; set these only to enable embeddings)")
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
