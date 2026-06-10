# Task L10: Docs/scripts/release + provenance gate

## Shared Context
**Goal:** Gate de provenance en el release script; warnings de seguridad en install; nota de migración del pre-push guard; dedup CLAUDE/AGENTS; manifest mínimo de AE docs.
**Stack:** PowerShell, Markdown.
**Architecture:** Worktree `C:/wt/v050-l10-docs-scripts-release`, branch `v050/l10-docs-scripts-release`. Único dueño de scripts release/install/ae y de CLAUDE.md/AGENTS.md/PATHS.md/AE docs/CHANGELOG. Disjunto del código Go.

## Locked Decisions
- `ae-release-binaries.ps1`: abortar si el árbol de build está dirty (vcs.modified); verificar que el binario resultante reporte `vcs_modified=false` y que el daemon propague la versión real (no "dev") tras spawn. (Implementa la idea nueva 4 / AUD-02 lado release.)
- `install.ps1`/`install-agent.ps1`: warning si `GITHUB_TOKEN` está en env (SEC-08); no imprimir el valor.
- `pre-push-guard.ps1`: nota de migración para `mi_lsp_preflight` (deprecation window opcional como warning); no relajar el gate, solo documentar.
- CLAUDE.md/AGENTS.md: extraer la sección compartida (gateway AE, workflow, governance) a un fragmento referenciado o a PATHS.md; cada archivo conserva lo específico (TOK-07).
- AE docs: crear `AE-MANIFEST.toml` (fase + evidencia requerida + hash por doc) para lazy-load; los 9 docs completos solo se leen en manifest_repair o gobernanza bloqueada (TOK-08).
- SEC-07: documentar en `.docs/wiki/ae/` o 01_alcance el riesgo de MSBuild sobre repos no confiables.

## Task Metadata
```yaml
id: L10
depends_on: [T2]
agent_type: ps-worker
goal_id: G6
github_issues: []
expected_outcome: "release script rechaza dirty; install advierte sobre token; docs deduplicadas; AE manifest creado."
files:
  - modify: scripts/release/ae-release-binaries.ps1
  - modify: scripts/install/install.ps1
  - modify: scripts/install/install-agent.ps1
  - modify: scripts/ae/pre-push-guard.ps1
  - modify: CLAUDE.md
  - modify: AGENTS.md
  - modify: PATHS.md
  - create: .docs/wiki/ae/AE-MANIFEST.toml
  - modify: CHANGELOG.md
complexity: medium
done_when:
  - "ae-release-binaries.ps1 aborts (non-zero) when git tree is dirty (dry-run test)"
  - "CLAUDE.md and AGENTS.md no longer duplicate the shared AE gateway block verbatim"
  - "AE-MANIFEST.toml exists with one entry per required AE doc"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L10-verdict.yaml"
stop_if:
  - "deduplicating CLAUDE.md/AGENTS.md would drop a rule with no shared home — keep it until a home exists"
```

## Reference
`scripts/release/ae-release-binaries.ps1`, `.docs/wiki/ae/*` (9 docs). Auditoría TOK-07/08, SEC-07/08, AUD-02.

## Prompt
Editá SOLO tu set (sin código Go). Cambios:
1. Provenance gate en `ae-release-binaries.ps1`: al inicio, si `git status --porcelain` no está vacío o el build embebería `vcs.modified=true`, abortar con mensaje claro; al final, correr el binario nuevo y verificar `vcs_modified=false` y, tras spawnear daemon, `version != dev`.
2. SEC-08: en install scripts, si `$env:GITHUB_TOKEN` está seteado, imprimir warning (sin el valor).
3. pre-push-guard: agregar comentario/nota de migración para session contracts viejos sin `mi_lsp_preflight` (no relajar).
4. TOK-07: identificá el bloque AE compartido idéntico en CLAUDE.md y AGENTS.md; movelo a PATHS.md (o a un fragmento) y dejá una referencia en ambos. Conservá lo específico de cada archivo.
5. TOK-08: creá `.docs/wiki/ae/AE-MANIFEST.toml` con una entrada por doc AE requerido (nombre, fase, evidencia requerida, sha256).
6. SEC-07: documentá el riesgo MSBuild/repos no confiables en `.docs/wiki/ae/` (sección de seguridad) o en CLAUDE.md.
7. CHANGELOG: agregá la sección `## v0.5.0 (unreleased)` con los grupos de cambios.

## Execution Procedure
1. `cd C:/wt/v050-l10-docs-scripts-release`; `git merge --no-edit main`.
2. Aplicá los cambios.
3. Dry-run del gate: ensuciá un archivo temporal y confirmá que el script abortaría (sin publicar nada).
4. Commit. `L10-verdict.yaml`.

## Skeleton
```powershell
# ae-release-binaries.ps1 (top)
if ((git status --porcelain) -ne $null) { Write-Error "release aborted: working tree is dirty (would embed +dirty)"; exit 1 }
# (bottom) after build:
$v = & $built --version
if ($v -match '\+dirty') { Write-Error "release aborted: binary reports +dirty"; exit 1 }
```

## Verify
`pwsh -c "& scripts/release/ae-release-binaries.ps1 -DryRun"` aborta en dirty (exit != 0)

## Commit
`chore(release): provenance gate, install token warning, AE manifest, docs dedup (AUD-02 SEC-07/08 TOK-07/08)`
