# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.3]

### Fixed

- **`daemon status` telemetry verbosity escape**: the full `recent_accesses` view (20 events with path fields) is now reachable via `--verbose` (or `--full`). The prior trigger checked a non-existent `--format telemetry` value (dead code since v0.5.0) and `--full` is AXI-gated, so there was no working way to get the full view. Found during live verification of v0.5.2 TOK-03.

## [0.5.2]

### Fixed

- **PERF-04**: `PRAGMA optimize` now actually runs after a bulk index publish (`ReplaceWorkspaceIndex`/`Docs`/`Catalog`). The helper was added in v0.5.0 but never called (dead code); SQLite query-planner statistics were never refreshed post-publish. Caught by adversarial verification.
- **TOK-03**: `daemon status` `recent_accesses` strips the long, repeated absolute-path fields (`workspace_root`, `runtime_key`, `entrypoint_id`) from the compact view to cut token cost (`--verbose` for full detail; see v0.5.3 for the escape wiring fix). Previously claimed "handled by the output layer" but unimplemented. Caught by adversarial verification.

## [0.5.1]

### Changed

- Auto-index for `workspace.add`/`init` is now **hybrid smart-sync** (FD1): synchronous within a short window (`MI_LSP_INDEX_SYNC_TIMEOUT`, default 20s) so small/incremental repos preserve the init-then-query contract, degrading to a background job (returning `job_id`) when a very large first index exceeds the window. `--background` forces immediate async; `--wait` forces full sync. Reconciles AUD-01 with the async-first intent of D6.

### Fixed

- Reinstated doc/FTS caching (PERF-02/03) with **generation-keyed invalidation** (`internal/service/doc_cache.go`): caches `ListDocRecords` and FTS scores per `(workspaceRoot, active_docs_generation_id)`, structurally invalidated on reindex. Replaces the v0.5.0-removed caches that served stale cross-workspace/post-reindex data.

### Notes

- SEC-11 (Authenticode code-signing) deferred: artifacts remain integrity-verified via SHA256 checksums. See `AE-RELEASE-DISTRIBUTION.md` (`code_signing_posture`).
- SEC-03 (named pipe SDDL) was already shipped in v0.5.0 (`server_windows.go`).

## [0.5.0]

### Added

- `AE-MANIFEST.toml`: lazy-load index of required AE docs with phase metadata and evidence tracking (TOK-08)
- Provenance gate in release script (`ae-release-binaries.ps1`): abort if working tree is dirty, verify built binary does not report `vcs.modified=true` (AUD-02)
- Security warning in install scripts when `GITHUB_TOKEN` environment variable is set (SEC-08)
- Migration note in `pre-push-guard.ps1` for session contracts without `mi_lsp_preflight` block
- Security documentation for MSBuild trust model (SEC-07) in CLAUDE.md

### Changed

- `CLAUDE.md` and `AGENTS.md` now reference `PATHS.md` for shared AE Programa Gateway and Subagent Orchestration sections (TOK-07)
- Moved authoritative AE gateway documentation to `PATHS.md` to eliminate duplication across harness policy files

### Docs

- `PATHS.md`: expanded with AE Programa Gateway and Subagent Orchestration Protocol (shared foundation for all harnesses)

## [Unreleased]

### Fixed

- `[embeddings]` now activates when `base_url` and `model` are present even if `enabled` is omitted; `enabled = false` remains the explicit kill switch.
- `mi-lsp index`/`index.run` now attempt wiki embedding backfill after docs indexing and no-change incremental runs, so missing `wiki_chunk_embeddings` rows can be populated without forcing unrelated source changes.

## [0.4.0] - 2026-05-31

### Added

- `nav recall` command: semantic search over markdown knowledge wikis using pluggable OpenAI-compatible embeddings
- `[embeddings]` config block in `.mi-lsp/project.toml`: provider, base_url, model, dim, api_key_env, profile, batch_size, timeout_ms
- Reference embeddings backend: tesla bge-m3 (1024-dim multilingual)
- `wiki_chunk_embeddings` repo-local SQLite BLOB table with incremental re-embedding by content hash
- `knowledge-wiki` profile: auto-detected when no `00_gobierno_documental.md` exists, bypasses spec-driven governance gate
- API key injection via `mkey run` and `MI_LSP_EMBEDDINGS_API_KEY` environment variable (never committed)
- Offline ⇒ lexical fallback: when embeddings service is unavailable, `nav recall` degrades to keyword search
- `nav evidence inventory <query>` command: preview-first operational evidence inventory for agents, with canonical wiki anchors first, manifest/verdict guidance, file/size estimates, loading profiles, and metadata-only handling for raw prompts, logs, turns, and screenshots

### Fixed

- AE release binary refresh now autodetects the active WSL user and home directory instead of assuming `/home/fgpaz`, so local WSL installs work on machines whose distro user differs from the Windows username.

## [0.2.0] - 2026-03-31

### Added

- `--format toon` output format: Token-Oriented Object Notation via `toon-format/toon-go`; ~20-40% fewer tokens vs JSON on large item arrays
- `--format yaml` output format: standard YAML via `gopkg.in/yaml.v3`; human-readable alternative to compact JSON
- `hint` field (omitempty) in all envelopes: diagnostic string present when `items=[]` (pattern not found, timeout, regex-like pattern) or when daemon is unavailable
- `nav search` now returns actionable hint when results are empty: explains cause (no matches, pattern looks regex-like, search timeout)
- Daemon fallback now visible: when daemon is unavailable and direct fallback is used, envelope includes `hint: "daemon_unavailable; served from local text index"`

### Fixed

- `nav multi-read` crashed with Windows OS error 123 (`ERROR_INVALID_NAME`) when path args contained embedded newlines (`\n`/`\r`) — now trims whitespace and returns descriptive error `"invalid path: contains newline in ..."`

### Docs

- `skills/mi-lsp/SKILL.md`: full format documentation with real TOON/YAML/compact examples, parsing rules, and when-to-switch guidance
- `skills/mi-lsp/references/compound-commands.md`: format selection guide with hint interpretation table
- `AGENTS.md`, `CLAUDE.md`: output formats table and hint field usage
- Technical contracts synced: `CT-CLI-DAEMON-ADMIN`, `09_contratos_tecnicos`, `RF-QRY-001`, `RF-QRY-002`, `RF-QRY-004`
- Companion skills updated with mi-lsp format guidance: `ps-contexto`, `ps-trazabilidad`, `ps-auditar-trazabilidad`, `ps-asistente-wiki`, `crear-capa-tecnica-wiki`, `crear-requerimiento`, `ps-gap-terminator`

## [0.1.0] - 2026-03-24

### Added

- Initial public release of `mi-lsp`
- Local semantic CLI for `.NET/C#`, TypeScript, and Python-aware workflows
- Optional shared daemon with governance UI and local telemetry
- Roslyn-backed C# semantic queries with bundled worker distribution by RID
- Repo-local indexing, service exploration, batch navigation, diff context, and cross-workspace search
- Public governance docs, issue templates, CI workflow, and release automation
