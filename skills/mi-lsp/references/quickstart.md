# mi-lsp Quickstart

Use this when `SKILL.md` is not enough and you need a slightly longer decision guide without loading the full recipe set.

## If `mi-lsp` is missing

If the skill is installed but the command is missing, install the CLI before doing anything else:

1. Download the matching release bundle from `https://github.com/fgpaz/mi-lsp/releases`.
2. Extract it into a stable directory and keep `workers/<rid>/` next to `mi-lsp(.exe)`.
3. Add that directory to the current session `PATH`.
4. Verify the binary before trying to initialize any workspace:

```powershell
mi-lsp info
mi-lsp worker status --format compact
```

If the bundle was moved after extraction, run `mi-lsp worker install`.

## First-use bootstrap

```powershell
mi-lsp workspace list
mi-lsp
mi-lsp init . --name <alias>
mi-lsp workspace status <alias>
```

If the repo already exists in the registry, reuse that alias instead of creating a new one.
If `mi-lsp workspace list` fails because the command is missing, return to the install steps above instead of falling back immediately.

## Preferred command order

1. `mi-lsp` or `workspace status` when you need the first onboarding/discovery pass
2. `nav route` when you need the cheapest canonical orientation (no index needed, governance-first)
3. `nav ask` when you need richer orientation, ownership, or docs-first evidence synthesis
4. `nav search --include-content` when you need literal implementation evidence
5. `nav workspace-map --axi` when you need structure across repos or services
6. `nav related` when you need one symbol's neighborhood in one call
7. `nav service` when you need evidence-first understanding of a backend area
8. `nav intent` when you know what the code does but not the symbol name
9. `nav multi-read` or `nav batch` when you already know the targets

## Choose the right command

| Need | Prefer |
|---|---|
| Cheapest canonical orientation (no index needed) | `nav route "how is this workspace organized?"` |
| Understand the repo with full evidence | `nav ask "how is this workspace organized?"` |
| Find the right repo/entrypoint in a parent folder | `nav workspace-map --axi` |
| Understand one symbol fully | `nav related MySymbol` |
| Find code by purpose | `nav intent "password reset frontend"` |
| Read code around a known line | `nav context path/to/file.cs 42` |
| Search text and see the matching code | `nav search "pattern" --include-content` |
| Read several files/ranges together | `nav multi-read ...` |
| Do mixed search + read + context in one shot | `nav batch` |

## Search syntax reminder

- `nav search` takes exactly one positional `pattern` argument.
- Quote the full pattern when it contains spaces: `mi-lsp nav search "forgot password" --workspace <alias> --format compact`.
- If the pattern is regex-like, keep it quoted and add `--regex`.
- Avoid `mi-lsp nav search forgot password ...`; those bare words are parsed as extra arguments.

## Routing reminder

- Direct and daemon-insensitive: `find`, `search`, `intent`, `symbols`, `outline`, `overview`, `multi-read`
- Potentially daemon-backed: `refs`, `context`, `deps`, `related`, `service`, `workspace-map`, `diff-context`, `batch`, `ask`

If a cheap read is slow, suspect stale binary, stale index, or wrong PATH before suspecting daemon health.
In container workspaces, prefer `--repo` for direct `find`, `search`, or `intent` before reaching for semantic selectors.
