# Runtime Drift

Use this reference when source, docs, daemon, and installed binary appear out of sync.

## Fast sanity check

```powershell
where.exe mi-lsp
mi-lsp nav --help
mi-lsp daemon status
mi-lsp worker status --format compact
```

## What to suspect first

- If docs mention a command that `mi-lsp nav --help` does not show, suspect a stale installed binary first.
- If `worker status` does not expose `cli_path` and `protocol_version`, suspect a stale installed binary first.
- If a daemon-backed command behaves older than the current source tree, suspect a stale daemon and restart it.
- If `nav.find`, `nav.search`, or `nav.intent` are slow or inconsistent, suspect wrong PATH, stale `.mi-lsp/index.db`, or a stale binary before blaming daemon health.
- If a direct query in a container workspace returns `backend=router`, suspect missing scope before suspecting runtime drift and rerun with `--repo`.

## After rebuild or reinstall

```powershell
mi-lsp daemon restart
mi-lsp worker status --format compact
mi-lsp workspace status <alias> --format compact
```

## Path and install guidance

- Prefer the canonical installed binary on `PATH`
- For this repo's local release flow, treat `dist/<rid>/mi-lsp(.exe)` plus the installed copy under the chosen install dir as canonical
- Do not assume the repo-root `mi-lsp.exe` is the active binary unless `Get-Command mi-lsp` proves it
- Use `cli_path` from `mi-lsp worker status --format compact` to confirm which executable answered the probe
- Compare `protocol_version` in that output with the current source/docs when you suspect binary drift

## Semantic backend reminders

- `backend=pyright-langserver` with zero items means the Python backend answered but found no result
- `unsupported backend: pyright` means the running binary or daemon does not expose that backend
- If Roslyn bootstrap fails, the user-facing remediation should point to `mi-lsp worker install`
