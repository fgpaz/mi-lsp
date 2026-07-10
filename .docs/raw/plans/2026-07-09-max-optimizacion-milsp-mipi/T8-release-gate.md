# Task T8: Build/test/release gate de mi-lsp (ARM64)

## Shared Context
**Goal:** Cerrar el gate binario de la Ola 2: build + tests verdes, binario ARM64 refrescado en PATH y evidencia AE-RELEASE-DISTRIBUTION, o waiver explícito.
**Stack:** Go, Windows ARM64 (regla de máquina: SIEMPRE binarios arm64), script `scripts/release/ae-release-binaries.ps1`.
**Architecture:** CLAUDE.md exige no cerrar trabajo binary-affecting sin evidencia de release o waiver registrado. El daemon global viejo puede responder "unknown operation" a ops nuevas — hay que reiniciarlo tras instalar.

## Locked Decisions
- Correr en `C:/repos/mios/mi-lsp`: `go build ./...`, `go test ./...` (completo, no solo internal), y `go vet ./...`.
- Ejecutar `scripts/release/ae-release-binaries.ps1` (leerlo primero; si requiere parámetros, usar los defaults documentados en él). Si el script falla por algo ajeno a esta ola (p. ej. firma/publicación externa), registrar WAIVER con la salida exacta y seguir: el binario local se instala igual con `go build -o` al path que el script use para install local.
- Target de compilación: el default del script (la máquina es ARM64; verificar con `go env GOARCH` que produce arm64).
- Tras instalar: `mi-lsp daemon stop` + arrancar de nuevo + `mi-lsp workspace status mi-lsp --format toon` para confirmar que el binario nuevo responde y gobernanza sigue verde.
- Capturar versión/hash del binario final (`sha256`) para el closure packet.

## Task Metadata
```yaml
id: T8
depends_on: [T4, T5, T6, T7]
agent_type: ps-worker
goal_id: G2
github_issues: []
expected_outcome: "Binario mi-lsp con las optimizaciones instalado y daemon reiniciado, con evidencia de release o waiver"
files:
  - read: C:/repos/mios/mi-lsp/scripts/release/ae-release-binaries.ps1
complexity: low
done_when:
  - "go build ./... && go test ./... && go vet ./... exit 0"
  - "mi-lsp workspace status mi-lsp --format toon responde con el binario nuevo y governance_blocked=false"
evidence_expected:
  - "Salidas de build/test/vet, salida del release script (o waiver), sha256 del binario, status post-restart"
stop_if:
  - "tests fallan (volver a Wave 2, no instalar)"
  - "el binario instalado no responde workspace status"
```

## Reference
`scripts/release/ae-release-binaries.ps1` (leer completo antes de ejecutar). Memoria de máquina: usuario en Windows 11 ARM64 — binarios arm64 siempre.

## Prompt
Tarea mecánica de gate: no edites código de producto. Secuencia exacta de Execution Procedure; ante cualquier fallo de test, STOP y reportá el output completo (no arregles vos). El restart del daemon es obligatorio (memoria conocida: daemon viejo responde "unknown operation" a ops nuevas).

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp && go build ./... && go vet ./... && go test ./...`.
2. Leé `scripts/release/ae-release-binaries.ps1`; ejecutalo con defaults (PowerShell).
3. Si falla por causa externa: registrá waiver con salida exacta; instalá local con el mecanismo que el script use internamente.
4. `mi-lsp daemon stop`; verificá con `mi-lsp workspace status mi-lsp --format toon` (auto-start o `daemon start`).
5. `Get-FileHash <binario> -Algorithm SHA256`.
6. NO commitees. Reportá todo.

## Skeleton
```powershell
go build ./... ; go vet ./... ; go test ./...
pwsh -File scripts/release/ae-release-binaries.ps1
mi-lsp daemon stop; mi-lsp workspace status mi-lsp --format toon
```

## Verify
`mi-lsp workspace status mi-lsp --format toon` → `governance_blocked: false` con binario nuevo

## Commit
`chore(release): refresh arm64 binaries post daemon optimizations`
