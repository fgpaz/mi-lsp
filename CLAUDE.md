# mi-lsp Claude Policy

## Mandatory Workflow

For every task in this repository:

1. Run `$ps-contexto`.
2. Validate governance before planning or execution:
   - `mi-lsp workspace status <alias> --format toon`
   - `mi-lsp nav governance --workspace <alias> --format toon`
3. If governance is blocked, only diagnosis and repair work are allowed.
4. Run `$brainstorming` once after context is loaded.
5. Close critical context gaps before execution.
6. Work as an orchestrator by default.
7. Prefer `dispatching-parallel-agents` when the work can be split safely.
8. Run `$ps-trazabilidad` before finishing.

Escalation rules:

- Spec-driven development is mandatory in ALL tasks.
- `.docs/wiki/00_gobierno_documental.md` is the human governance authority.
- `.docs/wiki/_mi-lsp/read-model.toml` is the versioned executable projection of `00`.
- Do not push directly to `main`; create a branch, open a pull request, and merge through the PR flow unless the user explicitly asks to bypass that repository rule.
- Invalid, ambiguous, incomplete, or stale governance puts the repo in `blocked mode`.
- In `blocked mode`, use `mi-lsp nav governance`, `$ps-asistente-wiki`, and `crear-gobierno-documental`; normal work must stop.
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

- `.docs/raw/plans/2026-04-12-governance-profile-hardening.md`

## Governance Source of Truth

- Human authority: `.docs/wiki/00_gobierno_documental.md`
- Executable projection: `.docs/wiki/_mi-lsp/read-model.toml`
- Primary diagnostic surface: `mi-lsp nav governance --workspace <alias> --format toon`
- If `governance_blocked=true`, do not continue with normal docs-first work.
- After repairing governance or auto-syncing the projection, rerun `mi-lsp index --workspace <alias>` before resuming `nav ask` or `nav pack`.

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
- Active RFs: `RF-WKS-001`, `RF-WKS-002`, `RF-WKS-003`, `RF-WKS-004`, `RF-WKS-005`, `RF-IDX-001`, `RF-IDX-002`, `RF-IDX-003`, `RF-QRY-001`, `RF-QRY-002`, `RF-QRY-003`, `RF-QRY-004`, `RF-QRY-005`, `RF-QRY-006`, `RF-QRY-007`, `RF-QRY-008`, `RF-QRY-009`, `RF-QRY-010`, `RF-QRY-011`, `RF-QRY-012`, `RF-QRY-013`, `RF-CS-001`, `RF-DAE-001`, `RF-DAE-002`, `RF-DAE-003`, `RF-DAE-004`
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
  - `mi-lsp <command> --workspace <alias> --format toon`
- Recommended ladder:
  1. `mi-lsp workspace status <alias> --format toon` or `mi-lsp init . --name <alias>`
  2. `mi-lsp nav governance --workspace <alias> --format toon`
  3. `mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format toon`
  4. `mi-lsp nav workspace-map --workspace <alias> --format toon`
  5. `mi-lsp nav search "<pattern>" --include-content --workspace <alias> --format toon` or `mi-lsp nav multi-read ...`
  6. `mi-lsp nav related|context|refs ... --workspace <alias> --format toon`
  7. `mi-lsp nav service <path> --workspace <alias> --format toon`
- If `workspace status` reports `governance_blocked=true`, stop normal execution and repair governance before any `nav ask`, `nav pack`, planning, or implementation.
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
- Output formats: `--format toon` (recommended, ~20-40% savings on arrays), `--format yaml` (readable), `--format compact` (backward compat/jq)
- `hint` field: envelopes may include a `hint` string when items=0 or daemon is unavailable — act on it before retrying

## Search Shortcuts

```powershell
rg -n "FL-|TECH-|DB-|CT-|daemon|worker|tsserver|Roslyn|contract|schema" .docs/wiki docs README.md internal worker-dotnet
rg -n "RF-|TP-|WorkspaceRegistration|DaemonState|RuntimeSnapshot|AccessEvent" .docs/wiki
rg --files .docs/wiki
```

## Workflow Catalog

### A) Standard Task Flow
1. `ps-contexto` — load project context
2. governance gate — `workspace status` + `nav governance`
3. `brainstorming` — challenge and lock design decisions
4. orchestrate and execute
5. documentation synchronization when needed
6. `ps-trazabilidad` — closure

### B) Large / Risky / Multi-Step Task Flow
1. `ps-contexto` — load project context
2. governance gate — `workspace status` + `nav governance`
3. `brainstorming` — design and harden
4. `writing-plans` — generate wave-dispatchable plan when the work benefits from formal waves
5. wave execution and docs sync
6. `ps-trazabilidad` — final closure
7. `ps-auditar-trazabilidad` — read-only audit before marking done

### C) Policy-Change Flow
1. `ps-contexto`
2. governance gate
3. `brainstorming`
4. `ps-crear-agentsclaudemd`
5. update both policy files
6. `ps-trazabilidad`

### D) Governance-Repair Flow
1. `ps-contexto`
2. `mi-lsp workspace status <alias> --format toon`
3. `mi-lsp nav governance --workspace <alias> --format toon`
4. `ps-asistente-wiki`
5. `crear-gobierno-documental`
6. `mi-lsp index --workspace <alias>`
7. verify `governance_blocked=false`

## Skill Invocation Semantics

| Skill | When | Mandatory |
|-------|------|-----------|
| `ps-contexto` | At the start of every task | Yes |
| `mi-lsp` | Governance diagnostics, docs-first navigation, code exploration | Yes |
| `brainstorming` | After context and governance gate, before non-trivial execution | Yes |
| `ps-asistente-wiki` | Governance/documentation diagnosis and next-step routing | Yes when governance or wiki work is involved |
| `crear-gobierno-documental` | Create, repair, or refactor `.docs/wiki/00_gobierno_documental.md` and its projection | Yes when governance is missing, invalid, or stale |
| `writing-plans` | Large, risky, or multi-step work | Yes when a formal wave plan is needed |
| `ps-crear-agentsclaudemd` | Editing `AGENTS.md` or `CLAUDE.md` | Yes |
| `ps-trazabilidad` | Before closing any task | Yes |
| `ps-auditar-trazabilidad` | Large, risky, multi-module, or cross-layer changes | Yes |

## Keep These Rules Tight

- Do not skip `$ps-contexto`.
- Do not skip the governance gate at the start of every task.
- Do not skip the mandatory single `$brainstorming` pass.
- Do not finish without `$ps-trazabilidad`.
- Do not continue normal work when `governance_blocked=true`.
- Do not treat `00_gobierno_documental.md` and `read-model.toml` as co-authorities; `00` always wins.
- Do not leave cross-layer documentation drift behind.
- Keep `AGENTS.md` and `CLAUDE.md` synchronized.
- If updating any skill under `C:\Users\fgpaz\.agents\skills`, also update the mirrored copy under `C:\repos\buho\assets\skills` in the same task.
