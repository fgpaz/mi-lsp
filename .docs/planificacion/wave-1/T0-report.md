# T0 Verification Report

```yaml
harness_protocol: SDD-HARNESS-v1
id: "T0-report"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '.docs/planificacion/wave-1/T0-report.md'
exports:
  - 'T0-report'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/planificacion/wave-1/T0-report.md
agent_may_edit:
  - .docs/planificacion/wave-1/T0-report.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/planificacion/wave-1/T0-report.md
```

**Date:** 2026-04-07
**Status:** COMPLETE -- all prerequisites verified

## FTS5

- **Driver:** modernc.org/sqlite v1.37.1 (pure Go, no CGo)
- **FTS5 available:** YES -- included by default since ~v1.14
- **Runtime test:** `TestFTSSearchDocs_StemmerMatch` PASS, `TestFTSSearchDocs_GracefulDegradation` PASS
- **Already implemented:**
  - Virtual table `doc_records_fts` with porter+unicode61 tokenizer (`internal/store/schema.go:76-85`)
  - Content-sync triggers: INSERT/DELETE/UPDATE (`schema.go:116-142`)
  - `FTSSearchDocs()` with BM25 ranking (`internal/store/queries_docs.go:187-248`)
  - Integration in `ask.go` with graceful fallback (`internal/service/ask.go:82-124`)
- **Correction:** wave-1.md referenced `mattn/go-sqlite3` -- actual driver is `modernc.org/sqlite`

## Session Tracking

### session_id

- **Populated:** YES, always
- **Default:** `cli-<PID>` (per-process, generated at `internal/cli/root.go:40-43`)
- **Override:** `MI_LSP_SESSION_ID` env var or `--session-id` flag
- **Wire path:** CLI -> `model.QueryOptions.SessionID` -> daemon `recordAccess` -> `access_events.session_id`
- **Note:** Per-process, not per-user-session. AI agents should set `MI_LSP_SESSION_ID` for stable grouping.

### client_name

- **Populated:** YES, always
- **Default:** `manual-cli` (generated at `internal/cli/root.go:37-39`)
- **Override:** `MI_LSP_CLIENT_NAME` env var or `--client-name` flag
- **Fallback chain:** flag > env > `manual-cli` (never empty in DB)

### seq column

- **Write:** YES -- migration at `state_store.go:270`, `NextSeq()` computes `MAX(seq)+1` per session_id
- **Read-back:** NO -- `scanAccessEvent` (`state_store.go:506-526`) omits `seq` from SELECT
- **Impact:** seq is write-only; not visible in exports or `RecentAccesses`
- **Blocker for T3:** Must add `seq` to SELECT and `scanAccessEvent` before T3 is complete

## Blockers for Wave 1

| Task | Blocker | Severity |
|------|---------|----------|
| T1 (change-type) | None -- already implemented in `diff_context.go` | None |
| T2 (FTS5 ask) | None -- FTS5 fully implemented | None |
| T3 (telemetria seq) | seq read-back missing in `scanAccessEvent` + export SELECT | Medium |
| T4 (ask --all-workspaces) | None -- already implemented in `ask.go` | None |
| T5 (cursor pagination) | Not started -- `ExportQuery` has no Offset field | Scope |

## Implementation Status (bonus audit)

Most of Wave 1 is already implemented in unstaged changes (618 insertions / 40 deletions):

- **T1: DONE** (`diff_context.go`: `getGitFileChangeTypes` + `parseNameStatus`)
- **T2: DONE** (`schema.go` FTS5 DDL, `queries_docs.go` `FTSSearchDocs`, `ask.go` integration)
- **T3: PARTIAL** (seq write OK, read-back missing)
- **T4: DONE** (`ask.go` `askAllWorkspaces`, `nav.go` `--all-workspaces` flag)
- **T5: NOT STARTED** (no Offset in `ExportQuery`, no pagination in queries)
