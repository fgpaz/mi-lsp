# Wave 0 - Baseline guards

## Governance evidence

### `mi-lsp workspace status mi-lsp --format toon`

```text
backend: sqlite
items[1]:
  - doc_count: 78
    docs_index_ready: true
    governance_blocked: false
    governance_profile: spec_backend
    governance_sync: in_sync
    index_ready: true
    name: mi-lsp
    root: "C:\\repos\\mios\\mi-lsp"
ok: true
workspace: mi-lsp
```

### `mi-lsp nav governance --workspace mi-lsp --format toon`

```text
backend: governance
items[1]:
  - blocked: false
    human_doc: .docs/wiki/00_gobierno_documental.md
    profile: spec_backend
    projection_doc: .docs/wiki/_mi-lsp/read-model.toml
    summary: Governance is valid for profile spec_backend and the projection is ready.
    sync: in_sync
ok: true
workspace: mi-lsp
```

## Binary resolution evidence

### `where.exe mi-lsp` from `C:\Users\fgpaz`

```text
C:\Users\fgpaz\bin\mi-lsp.exe
```

### Windows binaries: `go version -m`

```text
=== C:\Users\fgpaz\bin\mi-lsp.exe
revision=335388d8c767c882e686d04a1b71231876a68e11
GOARCH=arm64
modified=true

=== C:\repos\mios\mi-lsp\mi-lsp.exe
revision=888baa181baafcd22ee94eaa5a417127ea24bf4c
GOARCH=arm64
modified=true

=== C:\repos\mios\mi-lsp\dist\win-arm64\mi-lsp.exe
revision=335388d8c767c882e686d04a1b71231876a68e11
GOARCH=arm64
modified=true
```

### WSL binaries: `go version -m`

```text
=== /home/fgpaz/.local/bin/mi-lsp
revision=335388d8c767c882e686d04a1b71231876a68e11
GOARCH=arm64
GOOS=linux
modified=true

=== /home/fgpaz/bin/mi-lsp
revision=335388d8c767c882e686d04a1b71231876a68e11
GOARCH=arm64
GOOS=linux
modified=true

=== /home/fgpaz/go/bin/mi-lsp
revision=97433cf59020948139b6407dd97e8e19863fd64d
GOARCH=arm64
GOOS=linux
modified=true
```

`/home/fgpaz/go/bin/mi-lsp` is stale relative to the frozen repo baseline and should be treated as such unless refreshed explicitly post-merge.

## GitHub evidence

- Open PR count in `fgpaz/mi-lsp`: `0`

## Stop conditions

Stop execution if any of the following becomes true:

- dirty set changes outside the frozen 18 paths plus the approved plan/evidence artifacts
- `governance_blocked=true`
- new task-owned files appear outside:
  - `.docs/raw/plans/2026-04-23-close-remaining-mi-lsp-hardening/`
  - `.docs/planificacion/2026-04-23-close-remaining-mi-lsp-hardening-trazabilidad-auditoria.md`

## Smoke guardrails

- Windows smokes: always call `C:\Users\fgpaz\bin\mi-lsp.exe` from neutral cwd
- WSL smokes: always call explicit binary paths
- Until post-merge refresh, never use repo-root `.\mi-lsp.exe` for validation
