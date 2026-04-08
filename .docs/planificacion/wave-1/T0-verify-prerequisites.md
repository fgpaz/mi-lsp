# Task T0: Verify Prerequisites (FTS5 + Session Tracking)

## Shared Context
**Goal:** Confirmar que FTS5 esta disponible en el build de SQLite y trazar como fluye session_id desde el CLI al daemon.
**Stack:** Go, mattn/go-sqlite3, Cobra CLI
**Architecture:** CLI envia requests al daemon via Unix socket. Cada request incluye `Context{SessionID, Workspace, ...}`.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: ps-worker
files:
  - read: go.mod                              # verify mattn/go-sqlite3 version and build tags
  - read: internal/store/schema.go            # existing schema, check for FTS5 usage
  - read: internal/cli/root.go                # trace session_id generation
  - read: internal/daemon/server.go:213-246   # trace session_id consumption in recordAccess
  - read: internal/model/request.go           # CommandContext struct definition
complexity: low
done_when: "Report confirming: (1) FTS5 available or not, (2) session_id flow from CLI to daemon, (3) client_name population status"
```

## Reference
`internal/daemon/state_store.go:224-248` -- access_events DDL already has `client_name TEXT` and `session_id TEXT` columns.

## Prompt
Investigate two things and report findings:

**1. FTS5 availability:**
- Open `go.mod` and check the `mattn/go-sqlite3` dependency version.
- Search for any build tags related to FTS5 in the project (grep for `fts5`, `ENABLE_FTS5`, `sqlite_fts5`).
- If mattn/go-sqlite3 is used with CGo, FTS5 is available by default since v1.14. If `modernc.org/sqlite` (pure Go) is used, check if FTS5 is included.
- Write a small test or check existing tests that confirm FTS5 works: `SELECT * FROM pragma_compile_options WHERE compile_options LIKE '%FTS5%'`.

**2. Session tracking flow:**
- Open `internal/cli/root.go` and find where `CommandContext` or equivalent is built before sending to daemon.
- Trace: does the CLI generate a `session_id`? If so, how? (UUID per process? per invocation?)
- Trace: does the CLI set `client_name`? What value does it use?
- Open `internal/model/request.go` to see the `Context` struct fields.
- Open `internal/daemon/server.go:213-246` to see `recordAccess` -- confirm `request.Context.SessionID` is read.
- Determine: are `client_name` and `session_id` being populated by the CLI today, or are they always empty?

**Output format:**
Write a brief report (max 50 lines) with:
- FTS5 status: available / not available / needs build tag
- session_id: populated / empty / partially populated -- with the code path
- client_name: populated / empty -- with the code path
- Blockers for Wave 1 tasks T2 (FTS5) and T3 (telemetria seq)

## Skeleton
```
# T0 Verification Report

## FTS5
- mattn/go-sqlite3 version: X.Y.Z
- FTS5 available: yes/no
- Evidence: [pragma output or build tag]

## Session Tracking
- session_id populated: yes/no
- Source: [file:line where generated]
- client_name populated: yes/no
- Source: [file:line where set]

## Blockers
- [none / list blockers]
```

## Verify
Report file written with clear yes/no answers for each prerequisite.

## Commit
`chore(wave-1): T0 prerequisite verification report`
