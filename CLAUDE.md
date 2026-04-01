# mi-lsp Claude Policy

## Mandatory Workflow

For every task in this repository:

1. Run `$ps-contexto`.
2. Run `$brainstorming` once after context is loaded.
3. Close critical context gaps before execution.
4. Work as an orchestrator by default.
5. Prefer `dispatching-parallel-agents` when the work can be split safely.
6. Run `$ps-trazabilidad` before finishing.

Escalation rules:

- Run `$ps-auditar-trazabilidad` for large, risky, multi-module, or cross-layer changes.
- Use `$ps-crear-agentsclaudemd` when editing `AGENTS.md` or `CLAUDE.md`.
- Use `$crear-capa-tecnica-wiki` when creating or restructuring docs under `07/08/09`.

## Canonical Project Paths

Functional docs:

- `.docs/wiki/00_gobierno_documental.md`
- `.docs/wiki/01_alcance_funcional.md`
- `.docs/wiki/02_arquitectura.md`
- `.docs/wiki/03_FL.md`
- `.docs/wiki/03_FL/`
- `.docs/wiki/04_RF.md`
- `.docs/wiki/04_RF/`
- `.docs/wiki/05_modelo_datos.md`
- `.docs/wiki/06_matriz_pruebas_RF.md`
- `.docs/wiki/06_pruebas/`

Technical docs:

- `.docs/wiki/07_baseline_tecnica.md`
- `.docs/wiki/08_modelo_fisico_datos.md`
- `.docs/wiki/09_contratos_tecnicos.md`
- `.docs/wiki/07_tech/`
- `.docs/wiki/08_db/`
- `.docs/wiki/09_contratos/`

Plan reference:

- `docs/plans/2026-03-16-mi-lsp-v1.md`

## Layer Boundary

- `00-06` = functional source of truth.
- `07+` = technical source of truth.
- Root `07/08/09` docs are short and canonical.
- `TECH-*`, `DB-*`, and `CT-*` contain delegated technical detail.

## Context Map

- Repository purpose: local semantic CLI for large `.NET/C# + TypeScript` non-monorepo workspaces.
- Runtime shape: Go CLI, optional global daemon, repo-local SQLite, Roslyn worker, optional TS semantic backend.
- Hardening direction: one daemon per OS user, shared across agents, runtime pool by `(workspace_root, backend_type)`, local governance UI, explicit worker install, strict dependency remediation.
- Active flows: `FL-BOOT-01`, `FL-IDX-01`, `FL-QRY-01`, `FL-CS-01`, `FL-DAE-01`
- Active RFs: `RF-WKS-001`, `RF-WKS-002`, `RF-WKS-003`, `RF-IDX-001`, `RF-IDX-002`, `RF-QRY-001`, `RF-QRY-002`, `RF-QRY-003`, `RF-QRY-004`, `RF-QRY-005`, `RF-QRY-006`, `RF-QRY-007`, `RF-QRY-008`, `RF-QRY-009`, `RF-QRY-010`, `RF-CS-001`, `RF-DAE-001`, `RF-DAE-002`, `RF-DAE-003`, `RF-DAE-004`
- Canonical entities: `WorkspaceRegistration`, `ProjectConfig`, `SymbolRecord`, `FileRecord`, `WorkspaceMeta`, `DaemonState`, `DaemonRun`, `RuntimeSnapshot`, `AccessEvent`, `QueryEnvelope`
- Local machine state:
  - `~/.mi-lsp/registry.toml`
  - `~/.mi-lsp/daemon/state.json`
  - `~/.mi-lsp/daemon/daemon.db`
- Repo-local state:
  - `.mi-lsp/project.toml`
  - `.mi-lsp/index.db`

## Placeholder Mapping

- `<ALCANCE_DOC>` -> `.docs/wiki/01_alcance_funcional.md`
- `<ARQUITECTURA_DOC>` -> `.docs/wiki/02_arquitectura.md`
- `<FL_INDEX_DOC>` -> `.docs/wiki/03_FL.md`
- `<FL_DOCS_DIR>` -> `.docs/wiki/03_FL/`
- `<RF_INDEX_DOC>` -> `.docs/wiki/04_RF.md`
- `<RF_DOCS_DIR>` -> `.docs/wiki/04_RF/`
- `<MODELO_DATOS_DOC>` -> `.docs/wiki/05_modelo_datos.md`
- `<TP_INDEX_DOC>` -> `.docs/wiki/06_matriz_pruebas_RF.md`
- `<TP_DOCS_DIR>` -> `.docs/wiki/06_pruebas/`
- `<BASELINE_TECNICA_DOC>` -> `.docs/wiki/07_baseline_tecnica.md`
- `<MODELO_FISICO_DOC>` -> `.docs/wiki/08_modelo_fisico_datos.md`
- `<CONTRATOS_TECNICOS_DOC>` -> `.docs/wiki/09_contratos_tecnicos.md`
- `<TECH_DOCS_DIR>` -> `.docs/wiki/07_tech/`
- `<DB_DOCS_DIR>` -> `.docs/wiki/08_db/`
- `<CONTRATOS_DOCS_DIR>` -> `.docs/wiki/09_contratos/`

## Documentation Sync Triggers

- Runtime, daemon, governance, bootstrap, TS backend, or dependency changes:
  - sync `.docs/wiki/07_baseline_tecnica.md`
  - sync affected `07_tech/TECH-*.md`

- Persistence, schema, migration, retention, or telemetry changes:
  - sync `.docs/wiki/08_modelo_fisico_datos.md`
  - sync affected `08_db/DB-*.md`

- Commands, flags, envelopes, handshake, admin API, or worker protocol changes:
  - sync `.docs/wiki/09_contratos_tecnicos.md`
  - sync affected `09_contratos/CT-*.md`

- Functional behavior or flow changes:
  - also review `.docs/wiki/01_alcance_funcional.md`
  - also review `.docs/wiki/02_arquitectura.md`
  - also review `.docs/wiki/03_FL.md` and `.docs/wiki/03_FL/`

## `$mi-lsp` Usage Policy

- For repo orientation, docs-first questions, symbol lookup, service audits, semantic refs/context, and batched reads, prefer `$mi-lsp` before raw `rg` or broad file inspection.
- Run `mi-lsp` through the host shell tool:
  - Codex: `functions.shell_command`
  - Claude Code: shell/Bash tool
  - Do not treat `mi-lsp` as an MCP server or wait for a dedicated built-in tool.
- Default invocation shape:
  - `mi-lsp <command> --workspace <alias> --format compact`
- Recommended ladder:
  1. `mi-lsp workspace status <alias> --format compact` or `mi-lsp init . --name <alias>`
  2. `mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format compact`
  3. `mi-lsp nav workspace-map --workspace <alias> --format compact`
  4. `mi-lsp nav search "<pattern>" --include-content --workspace <alias> --format compact` or `mi-lsp nav multi-read ...`
  5. `mi-lsp nav related|context|refs ... --workspace <alias> --format compact`
  6. `mi-lsp nav service <path> --workspace <alias> --format compact`
- Query routing expectations:
  - cheap reads stay direct: `nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`
  - semantic/compound queries may use daemon warm state: `nav.ask`, `nav.related`, `nav.context`, `nav.refs`, `nav.deps`, `nav.service`, `nav.workspace-map`, `nav.diff-context`, `nav.batch`
  - if a container workspace returns `backend=router`, rerun with `--repo`, `--entrypoint`, `--solution`, or `--project`
- Fall back to plain `rg` only when `mi-lsp` is unavailable or the request is outside the CLI surface.

## Agent Acceleration Commands (v1.3)

Docs-first orientation:

- `mi-lsp init . --name <alias>` — detect, register, and index the current workspace
- `mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format compact` — start from the wiki/read-model before code hunting

Compound commands to reduce agent round-trips from 7+ to 1-2:

- `nav multi-read file1:1-120 file2:260-440` — batch-read N file ranges in one call
- `nav search "pattern" --include-content` — search with inline code content (hybrid symbol/lines mode)
- `nav batch < ops.json` — N heterogeneous operations in one process spawn (parallel by default)
- `nav related <symbol>` — symbol neighborhood: definition + callers + implementors + tests
- `nav workspace-map` — high-level map of services, endpoints, events, and dependencies
- `nav diff-context [ref]` — semantic context of all changed symbols in a git diff, with impact analysis
- `nav search "X" --all-workspaces` / `nav find X --all-workspaces` — cross-workspace parallel search
- Auto-start daemon: semantic queries (refs/context/deps/related) auto-start daemon if not running
- Auto-index: `workspace add` automatically indexes after registration (use `--no-index` to skip)
- `--compress` flag: aggressive token compression (strips parent, scope, implements from output)
- Incremental indexing: `mi-lsp index` auto-detects git changes, only re-indexes modified files
- Output formats: `--format compact` (default, ~35% savings), `--format toon` (~40%, tight budgets), `--format yaml` (~25%, readable)
- `hint` field: envelopes may include a `hint` string when items=0 or daemon is unavailable — act on it before retrying

## Search Shortcuts

```powershell
rg -n "FL-|TECH-|DB-|CT-|daemon|worker|tsserver|Roslyn|contract|schema" .docs/wiki docs README.md internal worker-dotnet
rg -n "RF-|TP-|WorkspaceRegistration|DaemonState|RuntimeSnapshot|AccessEvent" .docs/wiki
rg --files .docs/wiki
```

## Keep These Rules Tight

- Do not skip `$ps-contexto`.
- Do not skip the mandatory single `$brainstorming` pass.
- Do not finish without `$ps-trazabilidad`.
- Do not leave cross-layer documentation drift behind.
- Keep `AGENTS.md` and `CLAUDE.md` synchronized.
- If updating any skill under `C:\Users\fgpaz\.agents\skills`, also update the mirrored copy under `C:\repos\buho\assets\skills` in the same task.
