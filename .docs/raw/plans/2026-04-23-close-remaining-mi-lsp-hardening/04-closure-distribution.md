# Wave 3 - Closure / distribution

## Branch and PR lock

- Create branch from `main`: `hardening/close-remaining-doc-ranking-index-trace`
- Open exactly one PR
- Do not push directly to `main`

## Required verification before any commit

- `go test ./...`
- `git diff --check`
- `C:\Users\fgpaz\bin\mi-lsp.exe index cancel --help`
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace multi-tedi index status --format toon`
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace gastos nav trace RF-GAS-10 --format json`
- `C:\Users\fgpaz\bin\mi-lsp.exe nav wiki trace RF-QRY-016 --workspace mi-lsp --format toon`
- `C:\Users\fgpaz\bin\mi-lsp.exe nav wiki search "RF IDX" --workspace mi-lsp --format toon`
- `C:\Users\fgpaz\bin\mi-lsp.exe --workspace interbancarizacion_coelsa nav ask "Which RF, FL, CT, TECH, DB and TP docs are most relevant for a full wiki-to-code parity audit across all microservices?" --format json`
- WSL same `nav ask` proof with:
  - `~/.local/bin/mi-lsp`
  - `~/bin/mi-lsp`

## Post-merge refresh

Only from a clean merged tree:

1. rebuild `dist\\win-arm64\\mi-lsp.exe`
2. replace `C:\\Users\\fgpaz\\bin\\mi-lsp.exe`
3. restart daemon
4. refresh WSL `~/.local/bin/mi-lsp`
5. refresh WSL `~/bin/mi-lsp`

Record `~/go/bin/mi-lsp` as stale unless refreshed explicitly during closure.

## Durable closure artifact

Produce:

- `.docs/planificacion/2026-04-23-close-remaining-mi-lsp-hardening-trazabilidad-auditoria.md`

`.docs/raw/` alone is support material and is not the push-ready closure evidence.
