# Exact wiki navigation - trazabilidad, auditoria y pre-push

```yaml
harness_protocol: SDD-HARNESS-v1
id: "2026-04-28-exact-wiki-navigation-trazabilidad-auditoria"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-016]]'
  - '[[TECH-WIKI-AWARE-SEARCH]]'
  - '[[CT-NAV-WIKI]]'
  - '[[TP-QRY]]'
exports:
  - '2026-04-28-exact-wiki-navigation-trazabilidad-auditoria'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-016.md
  - .docs/wiki/07_tech/TECH-WIKI-AWARE-SEARCH.md
  - .docs/wiki/09_contratos/CT-NAV-WIKI.md
  - .docs/wiki/06_pruebas/TP-QRY.md
  - .docs/planificacion/2026-04-28-exact-wiki-navigation-trazabilidad-auditoria.md
agent_may_edit:
  - .docs/planificacion/2026-04-28-exact-wiki-navigation-trazabilidad-auditoria.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp workspace status mi-lsp --format toon
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
  - mi-lsp nav wiki trace RF-QRY-016 --workspace mi-lsp --format toon
  - go test ./...
  - git diff --check
stop_if:
  - governance_blocked=true
  - governance_sync!=in_sync
  - governance_index_sync!=current
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
  - RF-QRY-016 trace coverage < 1
  - exact lookup status missing from search, trace, route, or pack
evidence:
  - .docs/planificacion/2026-04-28-exact-wiki-navigation-trazabilidad-auditoria.md
```

Date: 2026-04-28
Branch: `feature/sdd-harness-compiler-v1`
Scope: `RF-QRY-016` exact wiki navigation, Harness compiler, Wiki Source validator, exact identity envelope, docs sync, and installed binary refresh.

## Verdict

Approved with waiver for push-readiness review, but no push was executed.

Waiver reason: no GitHub issue/card is active in this repo context and `.pj-crear-tarjeta.conf` is absent. The user explicitly approved closing follow-ups with a waiver and without staging or committing.

`ps-pre-push` status: completed with the repository-local guard created in `infra/git/Invoke-PrePushGuard.ps1`.
Follow-up shared-skill update: `ps-pre-push` skill source and mirror were updated to document the missing-guard rule, then verified as byte-identical.

## Scope Ownership

Owned closure scope:

- `backend`
- `canon-docs`
- `evidence-docs`

Task-owned implementation surfaces:

- `internal/model/types.go`
- `internal/service/wiki_lookup_status.go`
- `internal/service/wiki_search.go`
- `internal/service/trace.go`
- `internal/service/route.go`
- `internal/service/pack.go`
- `internal/service/pack_test.go`
- `internal/service/wiki_search_test.go`
- `internal/service/trace_test.go`
- validator/parser/indexing files already present in the active wave

Task-owned documentation surfaces:

- `.docs/wiki/04_RF/RF-QRY-016.md`
- `.docs/wiki/07_tech/TECH-WIKI-AWARE-SEARCH.md`
- `.docs/wiki/09_contratos/CT-NAV-WIKI.md`
- `.docs/wiki/06_pruebas/TP-QRY.md`
- `.docs/planificacion/2026-04-28-exact-wiki-navigation-trazabilidad-auditoria.md`

Protected and not staged:

- pre-existing dirty wave files outside this closure scope
- `.docs/raw/*`
- `artifacts/release-regression/`

## Governance Evidence

Commands run:

```powershell
mi-lsp workspace status mi-lsp --format toon
mi-lsp nav governance --workspace mi-lsp --format toon
```

Observed:

- `governance_blocked=false`
- `governance_sync=in_sync`
- `governance_index_sync=current`
- `docs_index_ready=true`
- `doc_count=81`
- profile: `spec_backend`
- projection: `.docs/wiki/_mi-lsp/read-model.toml`

## Traceability Chain

Governance-first backend chain reviewed:

- `00`: `.docs/wiki/00_gobierno_documental.md`
- `FL`: `.docs/wiki/03_FL/FL-QRY-01.md`
- `RF`: `.docs/wiki/04_RF/RF-QRY-016.md`
- `TECH`: `.docs/wiki/07_tech/TECH-WIKI-AWARE-SEARCH.md`
- `CT`: `.docs/wiki/09_contratos/CT-NAV-WIKI.md`, `.docs/wiki/09_contratos_tecnicos.md`
- `TP`: `.docs/wiki/06_pruebas/TP-QRY.md`, `.docs/wiki/06_matriz_pruebas_RF.md`

Trace command:

```powershell
mi-lsp nav wiki trace RF-QRY-016 --workspace mi-lsp --format toon
```

Observed:

- `coverage=1`
- `status=implemented`
- `lookup_status.match_kind=canonical_indexed_id`
- `lookup_status.path=.docs/wiki/04_RF/RF-QRY-016.md`
- 15 explicit implementation links verified
- 10 test links verified
- `drift=[]`

## Harness And Wiki Source Readiness

Commands run:

```powershell
mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
mi-lsp nav wiki validate-source --workspace mi-lsp --format toon
```

Observed:

- `harness_readiness=ready`
- `harness_verdict=PASS`
- `harness_contracts_reviewed=81`
- `harness_links_reviewed=303`
- `wiki_source_readiness=not_declared`
- `wiki_source_verdict=PASS`
- `navigation_readiness=ready`
- `index_freshness=current`
- `governance_sync=in_sync`

Interpretation: no migrated `SDD-WIKI-SOURCE-v1` documents are declared in this repo snapshot, so `not_declared/PASS` is acceptable for this closure. Declared-but-invalid source artifacts would be blocking.

## Exact Lookup Evidence

Commands run:

```powershell
go run ./cmd/mi-lsp nav wiki search "RF-QRY-016" --workspace mi-lsp --format toon --full
go run ./cmd/mi-lsp nav wiki trace RF-QRY-016 --workspace mi-lsp --format toon
go run ./cmd/mi-lsp nav wiki route "RF-QRY-016" --workspace mi-lsp --format toon --full
go run ./cmd/mi-lsp nav wiki pack "RF-QRY-016" --workspace mi-lsp --format toon --full
```

Observed:

- `search.lookup_status.doc_id=RF-QRY-016`
- `search.lookup_status.match_kind=canonical_indexed_id`
- `search.lookup_status.total_matches=72`
- `trace.lookup_status.doc_id=RF-QRY-016`
- `trace.lookup_status.match_kind=canonical_indexed_id`
- `route.lookup_status.doc_id=RF-QRY-016`
- `route.lookup_status.match_kind=canonical_indexed_id`
- `pack.primary_doc=.docs/wiki/04_RF/RF-QRY-016.md`
- `pack.lookup_status.doc_id=RF-QRY-016`
- `pack.lookup_status.match_kind=canonical_indexed_id`

Audit finding remediated during closure: `pack.lookup_status` originally described the first pack document instead of the `primary_doc`. The fix makes `packLookupStatus` prefer `result.PrimaryDoc`, and `TestNavPackLookupStatusUsesPrimaryDocForExactRF` locks the behavior.

## Test And Build Evidence

Commands run:

```powershell
go test ./internal/service
go test ./internal/wikisource ./internal/service ./internal/docgraph ./internal/store ./internal/cli ./internal/output
go test ./...
go build ./cmd/mi-lsp
git diff --check
```

Observed:

- all Go test commands passed
- build passed
- `git diff --check` exited 0
- `git diff --check` emitted LF/CRLF warnings only

## Installed Runtime Evidence

Commands run:

```powershell
Copy-Item -LiteralPath .\mi-lsp.exe -Destination C:\Users\fgpaz\bin\mi-lsp.exe -Force
go version -m C:\Users\fgpaz\bin\mi-lsp.exe
C:\Users\fgpaz\bin\mi-lsp.exe nav wiki pack "RF-QRY-016" --workspace mi-lsp --format toon --full
C:\Users\fgpaz\bin\mi-lsp.exe nav wiki validate-source --workspace mi-lsp --format toon
C:\Users\fgpaz\bin\mi-lsp.exe daemon start
C:\Users\fgpaz\bin\mi-lsp.exe worker status --format toon
```

Observed:

- installed binary revision: `9a3c9bd87f207fbd725bcbcadb84959f139053d5`
- installed binary reports `vcs.modified=true`, expected because the build was produced from the active dirty worktree
- installed `pack RF-QRY-016` shows `lookup_status.doc_id=RF-QRY-016`
- installed `pack RF-QRY-016` shows `match_kind=canonical_indexed_id`
- installed `validate-source` returns `wiki_source_verdict=PASS`
- daemon restarted successfully
- worker status selected the bundled compatible `win-arm64` worker with protocol `mi-lsp-v1.1`

## Manual ps-pre-push Equivalent

Checks completed:

- `git fetch origin main` succeeded.
- `git merge-base --is-ancestor origin/main HEAD` succeeded.
- `git log --oneline origin/main..HEAD -n 20` returned no pending commits.
- `git log --oneline HEAD..origin/main -n 20` returned no missing commits.
- The branch has no configured upstream; fast-forward safety was checked explicitly against `origin/main`.
- `infra/git/Invoke-PrePushGuard.ps1` executed successfully with `verdict=Approved with waiver`.
- Guard command:

```powershell
.\infra\git\Invoke-PrePushGuard.ps1 `
  -WaiverReason "No board card: exact wiki navigation and ps-pre-push shared-skill update approved in chat; no .pj-crear-tarjeta.conf in repo" `
  -ExpectedScope backend,canon-docs,evidence-docs,git-tooling,shared-skill `
  -SharedSkillName ps-pre-push `
  -TraceabilityEvidence .docs/planificacion/2026-04-28-exact-wiki-navigation-trazabilidad-auditoria.md `
  -Json
```

- Branch is not `main`: `feature/sdd-harness-compiler-v1`
- No push executed
- No staging executed
- Governance current and valid
- Traceability evidence exists in this durable `.docs/planificacion` file
- Board/card: waived because no active board config was found
- Shared skills: `ps-pre-push` touched and mirror-checked
- Shared skill source: `C:\Users\fgpaz\.agents\skills\ps-pre-push\SKILL.md`
- Shared skill mirror: `C:\repos\buho\assets\skills\ps-pre-push\SKILL.md`
- Shared skill mirror check: `in_sync=true`
- Shared skill SHA256: `24CA05581CD9597C35343C18F85C2161AC60873FE7DF2146DCD8F6AD3AD12B3E`
- Shared skill SDD Harness validation: PASS
- Shared skill SDD Wiki Source validation: PASS
- `.docs/raw`: no current added/modified raw paths in `git status -- .docs/raw`
- Dangerous untracked artifacts: `artifacts/release-regression/` remains untracked and protected; it was not staged or modified by this closure
- Dangerous untracked artifacts under `src/`: none found
- Guard warnings: board/card verification waived; one unknown-surface warning remains due non-staged/untracked support paths in the broad dirty worktree.

## Remaining Risks

Non-blocking for local closure:

- Large dirty worktree remains from the active wave.
- LF/CRLF warnings remain when Git inspects changed files.
- Installed binary was built from a dirty worktree and therefore reports `vcs.modified=true`.

Blocking for an actual push unless explicitly accepted by the human operator:

- Stage only task-owned paths.
- Decide whether to include or exclude pre-existing dirty wave files.
- Treat absent `infra/git/Invoke-PrePushGuard.ps1` as either an accepted repo limitation or a separate governance/tooling task.

## Final Closure

`ps-trazabilidad`: PASS.

`ps-auditar-trazabilidad`: Approved after remediating `pack.lookup_status` drift.

`ps-pre-push`: Approved with waiver by repository-local guard.
