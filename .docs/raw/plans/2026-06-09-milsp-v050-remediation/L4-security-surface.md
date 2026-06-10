# Task L4: Security surface (admin + pipe + protocol)

## Shared Context
**Goal:** Frame size acotado; admin con token + Host/Origin; named pipe con SDDL; CSP/X-Frame en admin UI.
**Stack:** Go, go-winio.
**Architecture:** Worktree `C:/wt/v050-l4-security-surface`, branch `v050/l4-security-surface`. Único dueño de `admin.go`, `admin_ui.go`, `server_windows.go`, `server_unix.go`, `worker/protocol.go`.

## Locked Decisions
- `MaxFrameSize = 256 << 20` (256MB); `ReadFrame` valida el header antes de alocar y devuelve error si excede.
- Admin: token pre-compartido generado al iniciar, persistido en `state.json` (campo `admin_token`), requerido en endpoints mutantes (warm, etc.) vía header `X-Mi-Lsp-Token`. Validar también `Host`/`Origin` == loopback.
- Named pipe: pasar SDDL restrictivo (owner + SYSTEM) a `winio.ListenPipe` si la versión vendorizada lo soporta; si no, documentar y dejar follow-up.
- Admin UI: headers `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'` y `X-Frame-Options: DENY`.

## Task Metadata
```yaml
id: L4
depends_on: [T2]
agent_type: general-purpose
goal_id: G2
github_issues: []
expected_outcome: "frames >256MB rechazados; endpoints mutantes requieren token+host; pipe endurecido; UI con CSP."
files:
  - modify: internal/worker/protocol.go
  - modify: internal/daemon/admin.go
  - modify: internal/daemon/admin_ui.go
  - modify: internal/daemon/server_windows.go
  - modify: internal/daemon/server_unix.go
complexity: medium
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/worker/... ./internal/daemon/... passes"
  - "a unit test asserts ReadFrame rejects oversized length and warm without token is 401/403"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L4-verdict.yaml"
stop_if:
  - "go-winio version does not expose SDDL param — record SEC-03 as documented follow-up, do not fake it"
  - "admin token change breaks the existing admin UI fetch handshake without updating admin_ui.go"
```

## Reference
`discovery.yaml` SEC-01(23-34)/SEC-02(55-190)/SEC-03(28)/SEC-10(82-86). El cliente admin existente para no romper el handshake.

## Prompt
Editá SOLO tu set. Cambios:
1. SEC-01: en `protocol.go` definí `const MaxFrameSize = 256 << 20`; en `ReadFrame`, si la longitud del header > MaxFrameSize, retorná error sin alocar.
2. SEC-02/admin-warm: generá `admin_token` al iniciar el server, persistilo en state, y exigí `X-Mi-Lsp-Token` + `Host`/`Origin` loopback en endpoints mutantes (warm). Actualizá `admin_ui.go` para mandar el token en sus fetch.
3. SEC-03: pasá SDDL restrictivo a `ListenPipe` en `server_windows.go` si el API lo permite; si no, dejá comentario `// SEC-03 follow-up` y registralo en el verdict.
4. SEC-10: agregá headers CSP + X-Frame-Options en `handleIndex`/admin_ui.
5. Agregá tests: `ReadFrame` con length gigante → error; warm sin token → 401/403.

## Execution Procedure
1. `cd C:/wt/v050-l4-security-surface`; `git merge --no-edit main`.
2. Aplicá los cambios + tests.
3. `go build ./... && go vet ./... && go test ./internal/worker/... ./internal/daemon/...`.
4. Commit. `L4-verdict.yaml` (incluí si SEC-03 quedó como follow-up).

## Skeleton
```go
const MaxFrameSize = 256 << 20
func ReadFrame(r io.Reader) ([]byte, error) {
    var n uint32
    if err := binary.Read(r, binary.BigEndian, &n); err != nil { return nil, err }
    if int64(n) > MaxFrameSize { return nil, fmt.Errorf("frame too large: %d > %d", n, MaxFrameSize) }
    buf := make([]byte, n); _, err := io.ReadFull(r, buf); return buf, err
}
```

## Verify
`go test ./internal/worker/... ./internal/daemon/...` → PASS

## Commit
`feat(security): frame size cap, admin token+host check, pipe SDDL, admin CSP (SEC-01/02/03/10)`
