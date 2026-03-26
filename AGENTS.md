# mi-lsp Agent Policy

## Orchestration Mode (MANDATORY - Always Active)

For every task in this repository:

1. Run `$ps-contexto` first.
2. After context load, run `$brainstorming` exactly once before planning or execution.
3. Close critical context gaps before acting.
4. Work in orchestrator mode by default.
5. Prefer `dispatching-parallel-agents` when work is safely partitionable.
6. Run `$ps-trazabilidad` before closing the task.

Additional strict rules:

- Run `$ps-auditar-trazabilidad` for large, risky, cross-layer, or multi-module changes.
- If editing `AGENTS.md` or `CLAUDE.md`, use `$ps-crear-agentsclaudemd`.
- If updating any skill under `C:\Users\fgpaz\.agents\skills`, also update the mirrored copy under `C:\repos\buho\assets\skills` in the same task.
- If creating or refactoring technical wiki docs under `07/08/09`, use `$crear-capa-tecnica-wiki`.
- If changing scope, architecture, or flows, use `crear-alcance`, `crear-arquitectura`, and `crear-flujo` in that order when applicable.

## Canonical Source of Truth (Project Paths)

Functional source of truth:

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

Technical source of truth:

- `.docs/wiki/07_baseline_tecnica.md`
- `.docs/wiki/08_modelo_fisico_datos.md`
- `.docs/wiki/09_contratos_tecnicos.md`
- `.docs/wiki/07_tech/`
- `.docs/wiki/08_db/`
- `.docs/wiki/09_contratos/`

Implementation plan reference:

- `docs/plans/2026-03-16-mi-lsp-v1.md`

## Layering Rule

- `00-06` are the functional truth layers.
- `07+` are the technical truth layers.
- Root `07/08/09` docs stay short, human-canonical, and decision-oriented.
- `TECH-*`, `DB-*`, and `CT-*` hold high-entropy implementation detail.
- Do not move ownership-defining decisions into detail docs only.

## Context Map

- Product: `mi-lsp`, a non-MCP semantic CLI for large non-monorepo `.NET/C# + TypeScript` codebases.
- Current architecture baseline: Go CLI + optional global daemon + repo-local SQLite + .NET Roslyn worker.
- Current hardening direction: one daemon per OS user, shared across Codex/Claude/subagents; runtime pool keyed by `(workspace_root, backend_type)`; local governance UI; optional `tsserver` semantic backend; dependency hardening for the .NET worker.
- Active flow set:
  - `FL-BOOT-01`
  - `FL-IDX-01`
  - `FL-QRY-01`
  - `FL-CS-01`
  - `FL-DAE-01`
- Active RF set:
  - `RF-WKS-001`
  - `RF-WKS-002`
  - `RF-WKS-003`
  - `RF-IDX-001`
  - `RF-IDX-002`
  - `RF-QRY-001`
  - `RF-QRY-002`
  - `RF-QRY-003`
  - `RF-QRY-004`
  - `RF-QRY-005`
  - `RF-QRY-006`
  - `RF-QRY-007`
  - `RF-QRY-008`
  - `RF-QRY-009`
  - `RF-QRY-010`
  - `RF-CS-001`
  - `RF-DAE-001`
  - `RF-DAE-002`
  - `RF-DAE-003`
  - `RF-DAE-004`
- Canonical operational entities:
  - `WorkspaceRegistration`
  - `ProjectConfig`
  - `SymbolRecord`
  - `FileRecord`
  - `WorkspaceMeta`
  - `DaemonState`
  - `DaemonRun`
  - `RuntimeSnapshot`
  - `AccessEvent`
  - `QueryEnvelope`
- Repo-local operational state:
  - `.mi-lsp/project.toml`
  - `.mi-lsp/index.db`
- Global local-machine state:
  - `~/.mi-lsp/registry.toml`
  - `~/.mi-lsp/daemon/state.json`
  - `~/.mi-lsp/daemon/daemon.db`

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

## Wiki Navigation

- Scope: `.docs/wiki/01_alcance_funcional.md`
- Architecture: `.docs/wiki/02_arquitectura.md`
- Flow index: `.docs/wiki/03_FL.md`
- Flow docs: `.docs/wiki/03_FL/`
- RF index: `.docs/wiki/04_RF.md`
- RF docs: `.docs/wiki/04_RF/`
- Data model: `.docs/wiki/05_modelo_datos.md`
- Test matrix: `.docs/wiki/06_matriz_pruebas_RF.md`
- Test plans: `.docs/wiki/06_pruebas/`
- Technical baseline: `.docs/wiki/07_baseline_tecnica.md`
- Physical data model: `.docs/wiki/08_modelo_fisico_datos.md`
- Technical contracts: `.docs/wiki/09_contratos_tecnicos.md`
- Technical detail docs: `.docs/wiki/07_tech/`
- Physical detail docs: `.docs/wiki/08_db/`
- Contract detail docs: `.docs/wiki/09_contratos/`

## Documentation Sync Rule

When the change affects runtime, supervision, governance, bootstrapping, optional backends, or dependency posture:

- review/update `.docs/wiki/07_baseline_tecnica.md`
- review/update related `TECH-*` docs

When the change affects repo-local persistence, daemon state, telemetry, migrations, retention, or schema shape:

- review/update `.docs/wiki/08_modelo_fisico_datos.md`
- review/update related `DB-*` docs

When the change affects commands, flags, envelopes, protocol versioning, admin endpoints, worker framing, or compatibility:

- review/update `.docs/wiki/09_contratos_tecnicos.md`
- review/update related `CT-*` docs

When visible behavior, states, or flows change:

- also review `.docs/wiki/01_alcance_funcional.md`, `.docs/wiki/02_arquitectura.md`, and `.docs/wiki/03_FL*`

## Search Commands

Use fast discovery first:

```powershell
rg -n "FL-|TECH-|DB-|CT-|daemon|worker|tsserver|Roslyn|contract|schema" .docs/wiki docs README.md internal worker-dotnet
rg -n "RF-|TP-|WorkspaceRegistration|DaemonState|RuntimeSnapshot|AccessEvent" .docs/wiki
rg --files .docs/wiki
rg -n "07_baseline_tecnica|08_modelo_fisico_datos|09_contratos_tecnicos" .docs/wiki
```

## `$mi-lsp` Usage Policy

- For workspace orientation, docs-first repo Q&A, symbol lookup, service audits, semantic refs/context, and batched file reads, prefer `$mi-lsp` before raw `rg` or broad `Get-Content`.
- Invoke `mi-lsp` through the host shell tool:
  - Codex: `functions.shell_command`
  - Claude Code: shell/Bash tool
  - Do not model `mi-lsp` as an MCP server or wait for a dedicated `mi-lsp` tool binding.
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

## Task Flow

Standard task:

1. `$ps-contexto`
2. `$brainstorming`
3. orchestrate and execute
4. `$ps-trazabilidad`

Large or risky task:

1. `$ps-contexto`
2. `$brainstorming`
3. orchestrate, preferably with `dispatching-parallel-agents`
4. update docs if needed
5. `$ps-trazabilidad`
6. `$ps-auditar-trazabilidad`

Policy-edit task:

1. `$ps-contexto`
2. `$brainstorming`
3. `$ps-crear-agentsclaudemd`
4. sync `AGENTS.md` and `CLAUDE.md`
5. `$ps-trazabilidad`

## Agent Acceleration Commands (v1.3)

### Docs-first orientation
```powershell
mi-lsp init . --name <alias>
mi-lsp nav ask "how is this workspace organized?" --workspace <alias> --format compact
```

Use these compound commands to reduce exploration round-trips from 7+ to 1-2:

### Batch file reading (replaces sequential Read/Get-Content calls)
```powershell
mi-lsp nav multi-read file1.cs:1-120 file2.cs:260-440 file3.tsx:1-80 --workspace <alias> --format compact
```

### Search with inline content (replaces search + N reads)
```powershell
mi-lsp nav search "pattern" --include-content --workspace <alias> --format compact
mi-lsp nav search "pattern" --include-content --context-mode symbol --workspace <alias> --format compact
```

### Batch heterogeneous operations (replaces N sequential tool calls)
```powershell
echo '[
  {"id":"s1","op":"nav.search","params":{"pattern":"MapPost","include_content":true}},
  {"id":"r1","op":"nav.multi-read","params":{"items":["src/Program.cs:1-50","src/Model.cs:1-80"]}},
  {"id":"f1","op":"nav.find","params":{"pattern":"IExpenseRepository","exact":true}}
]' | mi-lsp nav batch --workspace <alias> --format compact
```

### Symbol neighborhood (replaces refs + N reads)
```powershell
mi-lsp nav related MyClassName --workspace <alias> --format compact
mi-lsp nav related IMyInterface --depth callers,implementors --workspace <alias> --format compact
```

### Workspace orientation (replaces N service calls)
```powershell
mi-lsp nav workspace-map --workspace <alias> --format compact
```

### Git-aware semantic diff (v1.3 — replaces manual diff reading)
```powershell
mi-lsp nav diff-context HEAD~1 --workspace <alias> --format compact
mi-lsp nav diff-context --include-content --workspace <alias> --format compact
```

### Cross-workspace search (v1.3 — replaces per-workspace loops)
```powershell
mi-lsp nav search "PublishAsync" --all-workspaces --format compact
mi-lsp nav find IExpenseRepository --all-workspaces --format compact
```

### Zero-friction behaviors (v1.3)
- **Auto-start daemon**: semantic queries (refs/context/deps/related) auto-start daemon. Disable: `--no-auto-daemon`.
- **Auto-index on add**: `workspace add` indexes automatically. Skip: `--no-index`.
- **Incremental indexing**: `mi-lsp index` uses git to only re-index changed files.
- **Token compression**: `--compress` strips optional fields from compact output.

### Decision guide
- Need to read multiple known files? -> `nav multi-read`
- Need to search and see the code? -> `nav search --include-content`
- Need to do search + reads + finds in one shot? -> `nav batch`
- Need to understand a symbol's full context? -> `nav related`
- Need a high-level workspace overview? -> `nav workspace-map`
- Need to understand what changed in a commit? -> `nav diff-context`
- Need to search across ALL projects? -> `nav search --all-workspaces`

## Non-Negotiables

- Do not skip `$ps-contexto`, even for documentation work.
- Do not skip the single `$brainstorming` pass after context load.
- Do not close tasks without `$ps-trazabilidad`.
- Do not treat the daemon, worker, TS backend, or governance UI as purely code concerns; keep `07/08/09` in sync.
- Keep `AGENTS.md` and `CLAUDE.md` aligned.
