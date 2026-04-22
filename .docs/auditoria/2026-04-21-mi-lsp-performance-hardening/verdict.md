# mi-lsp performance hardening verdict

Fecha: 2026-04-21

## Scope

- Daemon lifecycle, watcher containment, status/process telemetry, request backpressure and hot-path bounded reads.
- CLI/contract/docs updates for `daemon start`, `daemon status`, `/api/status`, `daemon perf-smoke`, `nav.workspace-map`, `nav.context`, LSP `didOpen`, log tail and trace verification.
- Shared `mi-lsp` skill and mirror sync.

## Commits

- `8834231 fix: harden daemon resource usage`
- `a1776fc chore: sync mi-lsp performance hardening skill` in `C:\repos\buho\assets`

## Governance

- `mi-lsp workspace status mi-lsp --format toon`: `governance_blocked=false`, `governance_sync=in_sync`, `governance_index_sync=current`.
- `mi-lsp nav governance --workspace mi-lsp --format toon`: `blocked=false`, `sync=in_sync`.

## Traceability

Focused RF trace results:

- `RF-DAE-002`: implemented, coverage `1`.
- `RF-DAE-004`: implemented, coverage `1`.
- `RF-QRY-002`: implemented, coverage `1`.
- `RF-QRY-007`: implemented, coverage `1`.

Reviewed chain:

- `00 -> FL-DAE-01 -> RF-DAE-002/RF-DAE-004 -> 07/08/09 -> TP-DAE`
- `00 -> FL-QRY-01 -> RF-QRY-002/RF-QRY-007 -> 07/09 -> TP-QRY`
- Owner docs: `07_baseline_tecnica.md`, `07_tech/TECH-DAEMON-GOBERNANZA.md`, `08_modelo_fisico_datos.md`, `08_db/DB-STATE-Y-TELEMETRIA.md`, `09_contratos_tecnicos.md`, `09_contratos/CT-CLI-DAEMON-ADMIN.md`.

## Runtime Evidence

- Clean worktree test from commit `8834231`: `go test -count=1 ./...` PASS.
- Installed daemon smoke: `mi-lsp daemon perf-smoke --callers 16 --watch-mode lazy --max-working-set-mb 250 --max-private-mb 300 --max-handles 5000` PASS.
- Smoke sample: 16 callers, 0 failures, working set about 29 MB, private bytes about 30 MB, handles about 235.
- `daemon status` exposes `daemon_process` and `watchers`.
- One live `mi-lsp.exe daemon serve` after restart.

## Skill Sync

- Source skill: `C:\Users\fgpaz\.agents\skills\mi-lsp`.
- Mirror: `C:\repos\buho\assets\skills\mi-lsp`.
- Markdown source/mirror comparison: no differences.
- Mirror binaries rebuilt for `win-x64` and `linux-x64`; global Windows binary rebuilt for current workstation architecture.

## Board and Push Guard

- No `.pj-crear-tarjeta.conf` was found in `C:\repos\mios\mi-lsp`; no board/card sync was applicable for this repo.
- `ps-pre-push` command/script was not present in this repo; the local skill exists at `C:\Users\fgpaz\.agents\skills\ps-pre-push`.
- If pushing to `main`, use this evidence path with explicit waiver for missing board card unless an issue/card is assigned first.

## Verdict

Approved with follow-ups:

- Push requires final `ps-pre-push` guard with issue/card or waiver.
- Unrelated dirty work remains intentionally unstaged and uncommitted.
