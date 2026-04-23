# Traceability and Audit Closure - nav wiki

Date: 2026-04-23
Scope: `mi-lsp nav wiki` plus compatibility guidance for `nav ask|route|pack --repo`.
Skills applied: `ps-trazabilidad`, `ps-auditar-trazabilidad`.

## Final Verdict

Approved with follow-ups.

No blocking traceability, governance, implementation, test, installed-runtime, or shared-skill mirror drift was found for `RF-QRY-016`.

Required before any push to `main`:

- Run `ps-pre-push`.
- Stage only task-owned files; the repo already has unrelated dirty work.
- Use this file as closure evidence: `.docs/planificacion/2026-04-23-nav-wiki-trazabilidad-auditoria.md`.

Non-blocking residuals:

- Build logs still emit existing NuGet `NU1903` warnings for `System.Security.Cryptography.Xml` 9.0.0.
- Built binaries report `vcs.modified=true` because they were built from a dirty worktree.

## Governance Evidence

Commands:

```powershell
C:\Users\fgpaz\bin\mi-lsp.exe workspace status mi-lsp --format toon
C:\Users\fgpaz\bin\mi-lsp.exe nav governance --workspace mi-lsp --format toon
```

Observed:

- `governance_blocked=false`
- `governance_sync=in_sync`
- `governance_index_sync=current`
- `docs_index_ready=true`
- `doc_count=78`
- `read_model=.docs/wiki/_mi-lsp/read-model.toml`
- profile: `spec_backend`

## Traceability Chain

Governance-first backend chain reviewed:

- `00`: `.docs/wiki/00_gobierno_documental.md`
- `FL`: `.docs/wiki/03_FL/FL-QRY-01.md`
- `RF`: `.docs/wiki/04_RF/RF-QRY-016.md`
- `TECH`: `.docs/wiki/07_tech/TECH-WIKI-AWARE-SEARCH.md`
- `CT`: `.docs/wiki/09_contratos_tecnicos.md`, `.docs/wiki/09_contratos/CT-NAV-WIKI.md`
- `TP`: `.docs/wiki/06_pruebas/TP-QRY.md`

Trace command:

```powershell
C:\Users\fgpaz\bin\mi-lsp.exe nav wiki trace RF-QRY-016 --workspace mi-lsp --format toon
```

Observed:

- `status=implemented`
- `coverage=1`
- `drift[0]`
- implementation markers verified:
  - `internal/cli/axi_mode.go`
  - `internal/cli/nav.go`
  - `internal/cli/root.go`
  - `internal/model/types.go`
  - `internal/service/app.go`
  - `internal/service/ask.go`
  - `internal/service/pack.go`
  - `internal/service/route.go`
  - `internal/service/wiki_compat.go`
  - `internal/service/wiki_search.go`
- test markers verified:
  - `internal/cli/nav_test.go`
  - `internal/cli/root_test.go`
  - `internal/service/wiki_search_test.go`

## Test Evidence

Commands:

```powershell
go test ./...
git diff --check
```

Observed:

- `go test ./...`: pass
- `git diff --check`: pass, with LF-to-CRLF warnings only

`TP-QRY` now maps `RF-QRY-016` to:

- `TC-QRY-073`: layer-filtered `nav wiki search` plus `next_queries`
- `TC-QRY-074`: empty docgraph diagnostic
- `TC-QRY-075`: governance-blocked diagnostic
- `TC-QRY-076`: `nav ask|route|pack --repo docs` compatibility warning/hint

## Runtime And Distribution Evidence

Installed runtime was checked from `C:\Users\fgpaz` with absolute paths to avoid repo-root shadowing.

Commands:

```powershell
C:\Users\fgpaz\bin\mi-lsp.exe worker status --format toon
C:\Users\fgpaz\bin\mi-lsp.exe nav wiki search "RF IDX" --workspace mi-lsp --format toon
go version -m C:\Users\fgpaz\bin\mi-lsp.exe
go version -m C:\repos\buho\assets\skills\mi-lsp\bin\mi-lsp-win-x64.exe
go version -m dist\linux-arm64\mi-lsp
```

Observed:

- installed CLI path: `C:\Users\fgpaz\bin\mi-lsp.exe`
- selected worker: `C:\Users\fgpaz\.mi-lsp\workers\win-arm64\MiLsp.Worker.exe`
- selected worker compatible: `true`
- protocol version: `mi-lsp-v1.1`
- global binary: `GOOS=windows`, `GOARCH=arm64`
- Buho mirror binary: `GOOS=windows`, `GOARCH=amd64`
- Linux artifact: `GOOS=linux`, `GOARCH=arm64`
- installed `nav wiki search "RF IDX"` returned `backend=wiki.search`

## Shared Skill Mirror Evidence

Shared skill comparisons returned no differences for:

- repo `skills/mi-lsp/SKILL.md` vs global `C:\Users\fgpaz\.agents\skills\mi-lsp\SKILL.md`
- repo `skills/mi-lsp/SKILL.md` vs Buho mirror `C:\repos\buho\assets\skills\mi-lsp\SKILL.md`
- global `ps-contexto` vs Buho mirror `ps-contexto`
- global `ps-asistente-wiki` vs Buho mirror `ps-asistente-wiki`

Expected Buho mirror dirty paths from this task:

- `skills/mi-lsp/SKILL.md`
- `skills/mi-lsp/bin/mi-lsp-win-x64.exe`
- `skills/mi-lsp/references/compound-commands.md`
- `skills/mi-lsp/references/quickstart.md`
- `skills/ps-asistente-wiki/SKILL.md`
- `skills/ps-contexto/SKILL.md`

## Board And Raw Evidence

- `.pj-crear-tarjeta.conf`: absent
- touched GitHub cards/issues: none identified
- board sync: not needed
- added/modified `.docs/raw`: none observed for this closure
- `.docs/auditoria/*` is ignored by `.gitignore`, so the push-ready evidence copy is this `.docs/planificacion/...` file

## Findings

Critical: none.
High: none.
Medium: none.
Low: build warning `NU1903` remains outside the `nav wiki` closure scope.

Blocking drift: none.
