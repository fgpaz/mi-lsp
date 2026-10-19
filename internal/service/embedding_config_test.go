package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestResolveWikiEmbeddingConfigUsesUserProviderAndEnvironmentPrecedence(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("MI_LSP_EMBEDDINGS_BASE_URL", "http://env.example/v1")
	t.Setenv("MI_LSP_EMBEDDINGS_MODEL", "env-model")
	path := filepath.Join(configDir, "mi-lsp", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "provider = \"gateway\"\n[providers.gateway]\ntype = \"openai-compatible\"\nbase_urls = [\"http://primary/v1\", \"http://fallback/v1\"]\nmodel = \"configured-model\"\ndimensions = 768\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, active, err := resolveWikiEmbeddingConfig(nil)
	if err != nil || !active {
		t.Fatalf("resolve: active=%v err=%v", active, err)
	}
	if cfg.Model != "env-model" || len(cfg.BaseURLs) != 1 || cfg.BaseURLs[0] != "http://env.example/v1" {
		t.Fatalf("environment did not override user config: %+v", cfg)
	}
	if cfg.Dim != 768 {
		t.Fatalf("dimension lost: %+v", cfg)
	}
}

func TestResolveWikiEmbeddingConfigFallsBackToProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, active, err := resolveWikiEmbeddingConfig(&model.EmbeddingsBlock{BaseURL: "http://project/v1", Model: "project-model", Dim: 768})
	if err != nil || !active {
		t.Fatalf("resolve: active=%v err=%v", active, err)
	}
	if cfg.BaseURL != "http://project/v1" || cfg.Model != "project-model" {
		t.Fatalf("project fallback lost: %+v", cfg)
	}
}
