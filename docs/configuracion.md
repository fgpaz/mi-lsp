# User configuration

mi-lsp reads optional user settings from `$XDG_CONFIG_HOME/mi-lsp/config.toml`, or `~/.config/mi-lsp/config.toml` when `XDG_CONFIG_HOME` is unset. This file is local to the user and must not be committed.

```toml
provider = "personal"

[providers.personal]
type = "openai-compatible"
base_urls = ["https://embeddings.example/v1", "https://backup.example/v1"]
model = "embedding-model"
dimensions = 768
api_key_env = "MI_LSP_EMBEDDINGS_API_KEY"
# Alternatively, set api_key_command = "my-secret-cli resolve EMBEDDINGS_KEY".

[[complementos]]
name = "skills-catalog"
command = "mi-skill-catalog serve"
# Or use socket = "/run/user/1000/mi-skill-catalog.sock".
```

Only `openai-compatible` providers are supported. `base_urls` are tried in order after connection failures and server errors; client errors do not trigger fallback. Credentials are resolved locally from the named environment variable or command and are never printed by `mi-lsp config doctor`. Do not put credential values in this file.

Embedding settings resolve with explicit command options first when available, then `MI_LSP_EMBEDDINGS_BASE_URL`, `MI_LSP_EMBEDDINGS_MODEL`, and `MI_LSP_EMBEDDINGS_API_KEY` (with legacy `OPENAI_API_KEY` support), then this file, then legacy project-level `[embeddings]` settings. The user provider applies to semantic wiki indexing and recall. Indexing sends small batches sequentially with low gateway priority; the client treats the priority header as advisory. If embeddings are unavailable, semantic enrichment is skipped and lexical behavior remains available.

Run `mi-lsp setup` or `mi-lsp config setup` to create a private, minimal config file if one does not exist; it never overwrites an existing file and does not invent provider endpoints or credentials. Add provider settings only when embeddings are wanted. Run `mi-lsp config doctor` to validate the file and report missing provider fields; without a provider, keyword-only behavior remains available. Doctor does not invoke `api_key_command`. Complementos are declarations only; mi-lsp does not execute them as part of configuration validation.

## Privacy

Never commit local config files, API-key values, credential command output, personal hostnames/IP addresses, machine names, usernames, or absolute home-directory paths. Prefer generic hostnames and paths in examples and fixtures. Before publishing, inspect generated artifacts, release reports, and closure packets for local identifying data.
