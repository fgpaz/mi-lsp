# Task T3: Telemetria Enrichment -- seq Column

## Shared Context
**Goal:** Agregar columna `seq` a access_events para registrar el orden de operaciones dentro de una sesion, habilitando session replay para context-pack adaptativo.
**Stack:** Go, SQLite, internal/daemon
**Architecture:** El daemon registra cada operacion en `access_events` via `recordAccess()`. `client_name` y `session_id` ya existen en el schema pero hay que verificar que se populen correctamente.

## Task Metadata
```yaml
id: T3
depends_on: [T0]
agent_type: ps-worker
files:
  - modify: internal/daemon/state_store.go:224-272  # ALTER TABLE for seq column
  - modify: internal/daemon/state_store.go:378-413  # RecordAccess insert to include seq
  - modify: internal/daemon/server.go:213-246        # compute seq before recording
  - read: internal/model/access_event.go             # AccessEvent struct
  - modify: internal/model/access_event.go           # add Seq field
complexity: medium
done_when: "go build ./... exits 0 AND access_events rows show incrementing seq per session_id"
```

## Reference
`internal/daemon/state_store.go:224-248` -- existing access_events DDL with client_name and session_id.
`internal/daemon/server.go:213-246` -- recordAccess function.

## Prompt
This task adds a `seq` (sequence number) column to `access_events` that auto-increments per `session_id`, enabling session replay for future context-pack prefetch.

**Step 1: Model** (`internal/model/access_event.go`)

Add a `Seq int` field to the `AccessEvent` struct. Place it after `SessionID`.

**Step 2: Schema migration** (`internal/daemon/state_store.go`)

In the migration section (around lines 255-272), add:
```go
_, _ = s.db.Exec(`ALTER TABLE access_events ADD COLUMN seq INTEGER DEFAULT 0`)
```

This follows the existing pattern of silent ALTER TABLE migrations.

**Step 3: Compute seq** (`internal/daemon/server.go`)

In `recordAccess` (around line 213), before creating the `AccessEvent`, compute the sequence number:

```go
// Compute seq: count existing events for this session_id + 1
var seq int
if request.Context.SessionID != "" {
    row := s.stateStore.DB().QueryRow(
        `SELECT COALESCE(MAX(seq), 0) + 1 FROM access_events WHERE session_id = ?`,
        request.Context.SessionID,
    )
    _ = row.Scan(&seq)
}
event.Seq = seq
```

**Important:** If `session_id` is empty (CLI invocation without session tracking), `seq` stays 0. This is the expected behavior for non-agent callers.

**Step 4: Insert** (`internal/daemon/state_store.go`)

In `RecordAccess` and `RecordAccessDirect` functions, add `seq` to the INSERT statement. It should be a straightforward column addition following the existing pattern -- add `event.Seq` to the VALUES list and `seq` to the column list.

**Step 5: Verify session_id population**

Based on T0 findings, if `session_id` is NOT being populated by the CLI:
- Open `internal/cli/root.go` and find where the `CommandContext` is built.
- Generate a session_id as: `fmt.Sprintf("cli-%d", os.Getpid())`. This gives a unique ID per CLI process, which is the natural session boundary (one `mi-lsp` invocation = one session, multiple batch ops share the same PID).
- Set `client_name` to `"mi-lsp-cli"` if not already set.

Do NOT change how `nav batch` works -- batch operations should share the same session_id (same process).
Do NOT change the daemon socket protocol.
Do NOT add a separate `seq` tracking mechanism -- use the simple MAX+1 query approach.

## Skeleton
```go
// internal/model/access_event.go (addition)
type AccessEvent struct {
    // ... existing fields ...
    SessionID  string    // already exists
    Seq        int       // NEW: sequence within session (0 if no session)
    // ... rest of fields ...
}
```

## Verify
```bash
go build ./... && go test ./internal/daemon/ -run TestStateStore -v
```
Then manually: `mi-lsp nav find "X" && mi-lsp nav search "Y"` with same session, query `SELECT session_id, seq, operation FROM access_events ORDER BY id DESC LIMIT 10` from daemon.db.

## Commit
`feat(telemetry): add seq column to access_events for session replay support`
