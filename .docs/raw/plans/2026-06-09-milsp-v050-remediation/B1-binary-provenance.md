# Task B1: Binary build + provenance gate + reinstall

## Shared Context
**Goal:** Construir el binario limpio desde la rama integrada, pasar el gate de provenance nuevo, verificar propagación de versión al daemon, y reinstalar en ~/bin (resuelve AUD-02).
**Stack:** Go, PowerShell, Windows ARM64.
**Architecture:** Secuencial post-integración. Usa el `ae-release-binaries.ps1` ya endurecido por L10.

## Locked Decisions
- Build desde `v050/integration` con árbol limpio (commit todo antes). El gate debe ver `vcs_modified=false`.
- Tras build, spawnear daemon con el binario nuevo y confirmar `daemon status` → `version` == versión de build (no "dev").
- Reinstalar en `C:/Users/fgpaz/bin/mi-lsp.exe` y verificar `mi-lsp version` (vcs_modified=false) y `mi-lsp doctor` exit 0.
- Plataforma objetivo del usuario: win-arm64 (memoria: usar binarios arm64).

## Task Metadata
```yaml
id: B1
depends_on: [A1]
agent_type: ps-worker
goal_id: G5
github_issues: []
expected_outcome: "binario limpio instalado; daemon reporta versión real; doctor verde."
files:
  - read: scripts/release/ae-release-binaries.ps1
complexity: medium
done_when:
  - "mi-lsp version reports vcs_modified=false"
  - "daemon status reports version != dev"
  - "mi-lsp doctor exits 0"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/release-provenance.yaml"
stop_if:
  - "provenance gate still reports +dirty after a clean commit — investigate build flags, do not bypass the gate"
  - "daemon still reports version=dev — L2 version propagation regressed, escalate"
```

## Reference
`scripts/release/ae-release-binaries.ps1` (post-L10). Hallazgo AUD-02 (binario +dirty, daemon dev).

## Prompt
Desde `C:/wt/v050-integration` con árbol limpio, corré el flujo de release de binarios (build win-arm64) usando el script endurecido. Confirmá que el gate de provenance pasa (no +dirty). Detené el daemon viejo, spawneá con el binario nuevo, y confirmá `daemon status` → version real. Reinstalá en `~/bin` y verificá `mi-lsp version` y `mi-lsp doctor`. Registrá todo en `release-provenance.yaml` (sha256 del binario, version string, vcs_modified, daemon version, doctor exit).

## Execution Procedure
1. `cd C:/wt/v050-integration`; asegurá `git status` limpio.
2. `pwsh scripts/release/ae-release-binaries.ps1` (target win-arm64).
3. Stop daemon viejo: `mi-lsp daemon stop` (o el comando real).
4. Reinstalá binario en `~/bin`; `mi-lsp version`; spawneá daemon; `mi-lsp daemon status`.
5. `mi-lsp doctor`.
6. `release-provenance.yaml`.

## Skeleton
```yaml
# release-provenance.yaml
binary_sha256: "..."
version_string: "v0.5.0 ..."
vcs_modified: false
daemon_version: "v0.5.0"
doctor_exit: 0
```

## Verify
`mi-lsp version` → `vcs_modified=false`; `mi-lsp doctor` → exit 0

## Commit
(sin cambios de código; evidencia bajo auditoria. Si el script genera artefactos versionables, commitearlos por separado)
