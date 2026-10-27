package userconfig

import (
	"fmt"
	"github.com/fgpaz/mi-lsp/internal/processutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Provider     string              `toml:"provider"`
	Providers    map[string]Provider `toml:"providers"`
	Complementos []Complemento       `toml:"complementos"`
}

type Provider struct {
	Type          string   `toml:"type"`
	BaseURLs      []string `toml:"base_urls"`
	Model         string   `toml:"model"`
	Dimensions    int      `toml:"dimensions"`
	APIKeyEnv     string   `toml:"api_key_env"`
	APIKeyCommand string   `toml:"api_key_command"`
}

type Complemento struct {
	Name    string `toml:"name"`
	Command string `toml:"command"`
	Socket  string `toml:"socket"`
}

func Path() (string, error) {
	if d := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); d != "" {
		return filepath.Join(d, "mi-lsp", "config.toml"), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".config", "mi-lsp", "config.toml"), nil
}

func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	var c Config
	if _, err := toml.DecodeFile(p, &c); err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return Config{}, fmt.Errorf("read user config: %w", err)
	}
	return c, nil
}

func (p Provider) Validate() error {
	if p.Type != "openai-compatible" {
		return fmt.Errorf("provider type must be openai-compatible")
	}
	if len(p.BaseURLs) == 0 {
		return fmt.Errorf("provider needs at least one base_urls entry")
	}
	for _, u := range p.BaseURLs {
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			return fmt.Errorf("base_urls entries must use http or https")
		}
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("provider model is required")
	}
	if p.Dimensions < 0 {
		return fmt.Errorf("dimensions cannot be negative")
	}
	if p.APIKeyEnv != "" && p.APIKeyCommand != "" {
		return fmt.Errorf("choose api_key_env or api_key_command, not both")
	}
	return nil
}

// APIKey resolves a key only in the local process. The value is never included in errors.
func (p Provider) APIKey() (string, error) {
	if p.APIKeyEnv != "" {
		v := strings.TrimSpace(os.Getenv(p.APIKeyEnv))
		if v == "" {
			return "", fmt.Errorf("configured API key environment variable is empty")
		}
		return v, nil
	}
	if p.APIKeyCommand != "" {
		parts := strings.Fields(p.APIKeyCommand)
		if len(parts) == 0 {
			return "", fmt.Errorf("api_key_command is empty")
		}
		out, err := processutil.Command(parts[0], parts[1:]...).Output()
		if err != nil {
			return "", fmt.Errorf("api_key_command failed")
		}
		v := strings.TrimSpace(string(out))
		if v == "" {
			return "", fmt.Errorf("api_key_command returned an empty key")
		}
		return v, nil
	}
	return "", nil
}

func (c Config) ActiveProvider() (Provider, bool) {
	name := strings.TrimSpace(c.Provider)
	if name == "" && len(c.Providers) == 1 {
		for n := range c.Providers {
			name = n
		}
	}
	p, ok := c.Providers[name]
	return p, ok
}

func (c Config) Validate() error {
	if c.Provider != "" {
		if _, ok := c.Providers[c.Provider]; !ok {
			return fmt.Errorf("selected provider %q is not registered", c.Provider)
		}
	}
	for name, p := range c.Providers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("provider name is empty")
		}
		if err := p.Validate(); err != nil {
			return fmt.Errorf("provider %q: %w", name, err)
		}
	}
	for _, x := range c.Complementos {
		if strings.TrimSpace(x.Name) == "" || (strings.TrimSpace(x.Command) == "" && strings.TrimSpace(x.Socket) == "") {
			return fmt.Errorf("each complemento needs a name and command or socket")
		}
	}
	return nil
}
