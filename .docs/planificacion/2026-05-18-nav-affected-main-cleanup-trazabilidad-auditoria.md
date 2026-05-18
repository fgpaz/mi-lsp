# Trazabilidad y auditoria - nav affected main cleanup

Fecha: 2026-05-18

## Resultado

Veredicto: PASS

El cambio de `nav affected` quedo integrado en `origin/main` mediante PR #14 y el repositorio local quedo sincronizado con `origin/main`.

Merge commit publicado:

```text
3ccc4cf26e581bb7f93c7b1c17e1b893d2cce7ce
```

## Gobierno

- `mi-lsp workspace status mi-lsp --format toon`: PASS
- `mi-lsp nav governance --workspace mi-lsp --format toon`: PASS
- `governance_blocked=false`
- `governance_sync=in_sync`
- `index_sync=current`

## Trazabilidad RF

`RF-QRY-017` quedo en estado `implemented`:

- `confidence=high`
- `coverage=1`
- `explicit_links_verified=5`
- `tests_verified=4`
- `drift=[]`

Comando de cierre:

```powershell
mi-lsp nav wiki trace RF-QRY-017 --workspace mi-lsp --format toon
```

## Gates ejecutados

- `go test ./...`: PASS
- `dotnet test worker-dotnet\MiLsp.Worker.sln`: PASS
- `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon`: PASS
- `mi-lsp nav wiki validate-source --workspace mi-lsp --format toon`: PASS
- `mi-lsp nav wiki trace RF-QRY-017 --workspace mi-lsp --format toon`: PASS
- `mi-lsp nav affected --from-git-diff --changed-ref HEAD~1 --workspace mi-lsp --format toon --quiet`: PASS

## Limpieza de worktrees y branches

Se removieron los worktrees deprecados:

- `C:\repos\mios\mi-lsp\.docs\temp\worktrees\release-log-audit-hardening`
- `C:\repos\mios\mi-lsp\.docs\temp\worktrees\workspace-execution-review`

Antes de removerlos, se preservaron sus cambios locales con stash:

- `cleanup/deprecated-worktree-release-log-audit-hardening-2026-05-18`
- `cleanup/deprecated-worktree-workspace-execution-review-2026-05-18`

Branches locales eliminadas por estar mergeadas en `main`:

- `feature/outcome-first-sdd-rs-release-regression`
- `feature/registry-memory-go-selfdogfood`
- `feature/release-log-audit-hardening`
- `feature/sdd-harness-compiler-v1`
- `hardening/close-remaining-doc-ranking-index-trace`

Branches remotas eliminadas por estar mergeadas en `origin/main`:

- `origin/feature/registry-memory-go-selfdogfood`
- `origin/hardening/close-remaining-doc-ranking-index-trace`

Estado final esperado:

- worktrees: solo `C:\repos\mios\mi-lsp`
- branch local: solo `main`
- branches remotas: `origin/main`
- ahead/behind: `0 0`

## Binario

El binario global se reconstruyo desde el commit publicado:

- `C:\Users\fgpaz\bin\mi-lsp.exe`
- `vcs_revision=3ccc4cf26e581bb7f93c7b1c17e1b893d2cce7ce`
- `vcs_modified=false`
- `goos=windows`
- `goarch=arm64`

Despues de versionar este cierre documental, el binario debe recompilarse una vez mas para apuntar al commit final que contiene este archivo.

## Auditoria

Hallazgos criticos: ninguno.

Hallazgos mayores: ninguno.

Hallazgos menores: ninguno.

Riesgo residual aceptado:

- Los stashes de preservacion quedan disponibles de forma intencional para recuperacion manual. No representan drift activo del repositorio.
- La salida de `dotnet test` fue breve porque la solucion quedo restaurada/up-to-date y el proceso termino con exit code 0.

