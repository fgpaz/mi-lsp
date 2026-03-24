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

## 3. Verify bundled worker layout

Bundled releases expect this layout after extraction:

```text
mi-lsp(.exe)
workers/<rid>/
```

If you move only the binary and leave `workers/<rid>/` behind, semantic C#
operations may fail until you refresh the global worker install:

```powershell
mi-lsp worker install
```

## 4. Distinguish routing ambiguity from runtime failure

If a command returns `backend=router`, that usually means the workspace is a
multi-repo container and the command needs more context.

Common fixes:

```powershell
mi-lsp workspace status my-workspace --format compact
mi-lsp nav refs IOrderRepository --workspace my-workspace --repo MyApp.Api --format compact
mi-lsp nav context src/MyApp.Api/Program.cs 42 --workspace my-workspace --entrypoint myapp-api --format compact
```

## 5. Check optional backend dependencies

- `backend=roslyn` requires a compatible bundled or installed worker
- `backend=tsserver` requires `tsserver` to be available if you request semantic TS/JS behavior
- `backend=pyright-langserver` requires `pyright-langserver` for semantic Python behavior

If an optional backend is unavailable, `mi-lsp` should degrade with an
actionable warning instead of silently failing.

## 6. Capture useful diagnostics

When reporting a bug, include:

- Exact command
- Full stderr/stdout
- OS and architecture
- Output of `mi-lsp info`
- Output of `mi-lsp worker status --format compact`
- Whether the problem happens with the bundled release, a source build, or both

For daemon-related issues, include:

```powershell
mi-lsp daemon status
mi-lsp admin status
mi-lsp admin export --recent --format compact --limit 50
```
