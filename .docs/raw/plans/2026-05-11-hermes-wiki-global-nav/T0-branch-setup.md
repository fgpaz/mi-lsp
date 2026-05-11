---
linear_parent: not_applicable
linear_child: not_applicable
anchors: []
allowed_paths:
  - .git/**
  - .docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md
  - .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/**
forbidden_paths:
  - "**/*.go"
  - .docs/wiki/**
verify:
  - git branch --show-current -> feature/hermes-wiki-global-nav
  - git log -1 --pretty=%s -> contains "docs(plan): add hermes-wiki-global-nav"
stop_if:
  - branch feature/hermes-wiki-global-nav already exists with divergent commits
  - working tree has uncommitted changes outside .docs/raw/plans/
secret_scan: clean
---

# Task T0: Branch setup + commit plan

## Shared Context
**Goal:** Crear branch dedicada para Wave 1 (Hermes wiki global navigation) y committear el plan + subdocs antes de cualquier mutación de código o docs SDD.
**Stack:** git (PowerShell host).
**Architecture:** Trabajo en branch `feature/hermes-wiki-global-nav`. Plan vive en `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md` con companion folder homónimo.

## Locked Decisions
- Branch name: `feature/hermes-wiki-global-nav` (no negociable).
- No push a remoto en esta tarea — solo commit local.
- Solo se stagea el plan y las subdocs; nada más.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: ps-worker
goal_id: G0
github_issues: []
expected_outcome: "Branch feature/hermes-wiki-global-nav existe y contiene un commit con el plan principal y las dieciséis subdocs."
files:
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T0-branch-setup.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T1-fl-wiki-01.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T2-rf-wiki.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T3-tp-wiki.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T4-tech-wiki-fanout.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T5-ct-nav-wiki.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T6-alcance-sync.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T7-fanout-primitive.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T8-nav-wiki-search.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T9-nav-wiki-route.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T10-nav-wiki-trace.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T11-nav-wiki-pack.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T12-nav-wiki-inventory.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T13-tests-go.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T14-regression-smoke.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T15-hermes-hosts.md
  - read: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T16-hermes-wrapper.md
complexity: low
done_when:
  - "git branch --show-current returns feature/hermes-wiki-global-nav"
  - "git log -1 --pretty=%s returns 'docs(plan): add hermes-wiki-global-nav implementation plan'"
  - "git status reports clean working tree"
evidence_expected:
  - "Output de git status, git branch --show-current, git log -1 capturado en el reporte de cierre"
stop_if:
  - "git branch -a contiene feature/hermes-wiki-global-nav con commits divergentes que no se pueden fast-forward"
  - "git status revela cambios sin commitear FUERA de .docs/raw/plans/ (NO improvisar — pedir guía al humano)"
```

## Reference
- Política CLAUDE.md: "Do not push directly to `main`; create a branch, open a pull request, and merge through the PR flow."

## Prompt

Sos el ejecutor de T0. Tu trabajo es exclusivamente git, no docs ni código. Seguí estos pasos literalmente. Si algo no coincide con el estado esperado, parás y reportás — no improvises.

1. Confirmar que estás en el repo correcto: `C:\repos\mios\mi-lsp`. Si no, parar y reportar.
2. Confirmar que `git status` reporta clean working tree EXCEPTO los archivos bajo `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md` y `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav/`. Si hay otros archivos modificados o untracked, parar y reportar — no commitearlos.
3. Confirmar que la rama actual es `main` con `git branch --show-current`. Si no, parar y reportar.
4. Crear y checkout de la branch nueva: `git checkout -b feature/hermes-wiki-global-nav`. Si la branch ya existe (`fatal: A branch named ... already exists`), parar y reportar — no la sobrescribas.
5. Stagear EXACTAMENTE el plan principal y la companion folder: `git add .docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/`.
6. Verificar con `git status` que SOLO esos archivos están stagados.
7. Crear el commit con HEREDOC para preservar formato:

```powershell
git commit -m @'
docs(plan): add hermes-wiki-global-nav implementation plan

Wave 1 plan covering five nav wiki * subcommands with --all-workspaces
(search, route, trace, pack, inventory new), FanOutWiki primitive reusing
ask.go AllWorkspaces pattern, Hermes-side ~/.hermes/hosts.yaml + PowerShell
orchestration wrapper. SDD anchors: FL-WIKI-01, RF-WIKI-001..005, TP-WIKI,
TECH-WIKI-FANOUT, CT-NAV-WIKI.

- Gabriel Paz -
'@
```

8. Confirmar con `git log -1 --pretty=fuller` que el commit existe.
9. Reportar:
   - Output completo de `git status` (post-commit)
   - Output de `git branch --show-current`
   - Output de `git log -1 --pretty=%s%n%b`

## Execution Procedure
1. cd a `C:\repos\mios\mi-lsp` (PowerShell: `Set-Location C:\repos\mios\mi-lsp`).
2. Ejecutar `git status` — verificar que sólo cambia `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav*`.
3. Ejecutar `git branch --show-current` — confirmar `main`.
4. Ejecutar `git checkout -b feature/hermes-wiki-global-nav`.
5. Ejecutar `git add .docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/`.
6. Ejecutar `git status` — confirmar staging mínimo.
7. Ejecutar el commit del paso 7 de Prompt.
8. Ejecutar `git status; git branch --show-current; git log -1 --pretty=%s%n%b`.
9. Reportar al orquestador.

## Skeleton

```powershell
# Bloque mínimo de comandos en orden
git status
git branch --show-current
git checkout -b feature/hermes-wiki-global-nav
git add .docs/raw/plans/2026-05-11-hermes-wiki-global-nav.md .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/
git status
git commit -m @'
docs(plan): add hermes-wiki-global-nav implementation plan

Wave 1 plan covering five nav wiki * subcommands with --all-workspaces ...

- Gabriel Paz -
'@
git log -1 --pretty=fuller
```

## Verify
`git branch --show-current` -> `feature/hermes-wiki-global-nav` AND `git status` -> `nothing to commit, working tree clean`

## Commit
Hecho dentro de la propia tarea (mensaje arriba). No commit posterior.
