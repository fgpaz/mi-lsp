# mi-lsp Quickstart

Use this when `SKILL.md` is not enough and you need a slightly longer decision guide without loading the full recipe set.

## First-use bootstrap

```powershell
mi-lsp workspace list
mi-lsp init . --name <alias>
mi-lsp workspace status <alias> --format compact
```

If the repo already exists in the registry, reuse that alias instead of creating a new one.

## Preferred command order

1. `nav ask` when you need orientation, ownership, docs-first context, or a "where is X implemented?" answer
2. `nav workspace-map` when you need structure across repos or services
3. `nav related` when you need one symbol's neighborhood in one call
4. `nav service` when you need evidence-first understanding of a backend area
5. `nav intent` when you know what the code does but not the symbol name
6. `nav search --include-content` when you need text search plus inline code
7. `nav multi-read` or `nav batch` when you already know the targets

## Choose the right command

| Need | Prefer |
|---|---|
| Understand the repo quickly | `nav ask "how is this workspace organized?"` |
| Find the right repo/entrypoint in a parent folder | `nav workspace-map` |
| Understand one symbol fully | `nav related MySymbol` |
| Find code by purpose | `nav intent "password reset frontend"` |
| Read code around a known line | `nav context path/to/file.cs 42` |
| Search text and see the matching code | `nav search "pattern" --include-content` |
| Read several files/ranges together | `nav multi-read ...` |
| Do mixed search + read + context in one shot | `nav batch` |

## Routing reminder

- Direct and daemon-insensitive: `find`, `search`, `intent`, `symbols`, `outline`, `overview`, `multi-read`
- Potentially daemon-backed: `refs`, `context`, `deps`, `related`, `service`, `workspace-map`, `diff-context`, `batch`, `ask`

If a cheap read is slow, suspect stale binary, stale index, or wrong PATH before suspecting daemon health.
In container workspaces, prefer `--repo` for direct `find`, `search`, or `intent` before reaching for semantic selectors.
