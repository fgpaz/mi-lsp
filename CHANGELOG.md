# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `nav recall` command: semantic search over markdown knowledge wikis using pluggable OpenAI-compatible embeddings
- `[embeddings]` config block in `.mi-lsp/project.toml`: provider, base_url, model, dim, api_key_env, profile, batch_size, timeout_ms
- Reference embeddings backend: tesla bge-m3 (1024-dim multilingual)
- `wiki_chunk_embeddings` repo-local SQLite BLOB table with incremental re-embedding by content hash
- `knowledge-wiki` profile: auto-detected when no `00_gobierno_documental.md` exists, bypasses spec-driven governance gate
- API key injection via `mkey run` and `MI_LSP_EMBEDDINGS_API_KEY` environment variable (never committed)
- Offline ⇒ lexical fallback: when embeddings service is unavailable, `nav recall` degrades to keyword search

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
