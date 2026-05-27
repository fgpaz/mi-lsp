# T7 - Integrate Main and Cleanup

After PR checks pass:

- merge through PR flow
- update local `main`
- refresh local binary
- reindex `mi-lsp`
- run workspace hygiene
- remove task worktree
- delete local and remote task branch
- confirm `main` is clean and aligned with `origin/main`

