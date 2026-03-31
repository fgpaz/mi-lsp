# Troubleshooting

Use this guide when `mi-lsp` appears to fail before or during normal command execution.

## 1. Distinguish host failure from product failure

If you see a shell or runtime startup error before `mi-lsp` prints anything, treat it as a host incident first.

Examples:

```text
Failed to load the dll from [...]
Failed to create CoreCLR
Insufficient system resources
```

Interpretation:
- The shell host may have failed while starting its own runtime.
- That is not yet evidence of a `mi-lsp` product bug.
- Retry from a fresh shell before changing your `mi-lsp` install.

## 2. Verify the installed binary

Check which binary is being executed:

```powershell
where.exe mi-lsp
mi-lsp info
mi-lsp worker status --format compact
```

If the wrong binary is on `PATH`, remove it or run the expected one explicitly.
On a current build, `mi-lsp worker status --format compact` should show the same diagnostic payload whether the daemon is running or not, including `cli_path` and `protocol_version`. If `cli_path` points to an unexpected location, you are likely still hitting an older binary.

## 3. Verify bundled worker layout

Bundled releases expect this layout after extraction:

```text
mi-lsp(.exe)
workers/<rid>/
```

If you move only the binary and leave `workers/<rid>/` behind, semantic C# operations may fail until you refresh the global worker install:

```powershell
mi-lsp worker install
```

Regular queries do not run the deep worker compatibility probe first. They resolve candidates by filesystem layout in `bundle -> installed -> dev-local` order, retry once on bootstrap failure, and leave the explicit probe to `mi-lsp worker status`.

On Windows, child runtimes are launched as non-interactive processes. If you still see extra terminals, capture whether the visible window belongs to `mi-lsp.exe` itself or to a child like `MiLsp.Worker.exe`, `dotnet`, `node`, `git`, or `rg`.

## 4. Verify workspace bootstrap and docs profile

For the shortest healthy setup, run:

```powershell
mi-lsp init . --name myapp
mi-lsp workspace status myapp --format compact
```

Check these signals:
- the workspace resolves and shows the expected `kind`
- `docs_read_model` is either `builtin-default` or `.docs/wiki/_mi-lsp/read-model.toml`
- `.mi-lsp/index.db` exists after init unless you used `--no-index`

If the repo changed heavily under `.docs/wiki`, `docs/`, `.docs/`, `README*`, or the read-model file, force a clean rebuild:

```powershell
mi-lsp index --workspace myapp --clean
```

## 5. Distinguish routing ambiguity from runtime failure

If a command returns `backend=router`, that usually means the workspace is a multi-repo container and the command needs more context.

Common fixes:

```powershell
mi-lsp workspace status my-workspace --format compact
mi-lsp nav refs IOrderRepository --workspace my-workspace --repo MyApp.Api --format compact
mi-lsp nav context src/MyApp.Api/Program.cs 42 --workspace my-workspace --entrypoint myapp-api --format compact
```

## 6. Distinguish daemon-sensitive queries from direct catalog reads

Not every `nav` command should depend on daemon health.
Cheap catalog or text operations run directly against the repo-local index or text search path:

- `nav.find`
- `nav.search`
- `nav.intent`
- `nav.symbols`
- `nav.outline`
- `nav.overview`
- `nav.multi-read`

If one of those commands is slow or returns no output on a current build, suspect these first:
- wrong `mi-lsp` binary on `PATH`
- stale or missing `.mi-lsp/index.db`
- local SQLite/index corruption for that workspace

Quick checks:

```powershell
where.exe mi-lsp
mi-lsp workspace status myapp --format compact
mi-lsp worker status --format compact
mi-lsp nav find ChatShell --workspace myapp --format compact
mi-lsp daemon stop
mi-lsp nav find ChatShell --workspace myapp --format compact
```

If `nav.find`, `nav.search`, or `nav.intent` behave differently only when using an older binary, update the installed binary first. In `container` workspaces, rerun direct catalog queries with `--repo <name>` when you want to narrow the scope without invoking semantic routing.

## 7. Check optional backend dependencies

- `backend=roslyn` requires a compatible bundled or installed worker
- `backend=tsserver` requires `tsserver` to be available if you request semantic TS/JS behavior
- `backend=pyright-langserver` requires `pyright-langserver` for semantic Python behavior

If an optional backend is unavailable, `mi-lsp` should degrade with an actionable warning instead of silently failing.

## 8. Diagnose `nav ask`

If `nav ask` feels weak or generic, check the corpus first:

```powershell
mi-lsp workspace status myapp --format compact
mi-lsp nav ask "how is this workspace organized?" --workspace myapp --format compact
mi-lsp nav search RF- --workspace myapp --format compact
```

Useful interpretations:
- no docs indexed: `nav ask` falls back to textual evidence
- `read_model=default`: the project is using the built-in profile, not a repo-specific one
- weak code evidence: the docs may be missing explicit file paths or symbols in backticks

## 9. Capture useful diagnostics

When reporting a bug, include:
- Exact command
- Full stderr/stdout
- OS and architecture
- Output of `mi-lsp info`
- Output of `mi-lsp worker status --format compact`
- Output of `mi-lsp workspace status <alias> --format compact`
- Whether the problem happens with the bundled release, a source build, or both

For daemon-related issues, include:

```powershell
mi-lsp daemon status
mi-lsp admin status
mi-lsp admin export --recent --format compact --limit 50
```
