# Task T7: Comando `mi-lsp registry gc`

## Shared Context
**Goal:** Purgar del registry global (`~/.mi-lsp/registry.toml`, hoy 493 workspaces) los workspaces cuya raíz ya no existe (worktrees muertos, tmp).
**Stack:** Go, repo `C:/repos/mios/mi-lsp`, paquete `internal/cli` + el paquete que ya lee/escribe registry.toml.
**Architecture:** El registry se carga en cada resolución de workspace. Existe la superficie `mi-lsp workspace ...`; hay que localizar el store del registry (buscar `registry.toml` en el código con mi-lsp/grep).

## Locked Decisions
- Comando nuevo: `mi-lsp registry gc` con `--dry-run` como DEFAULT (lista `alias  path  reason`) y `--apply` para escribir.
- Criterio de purga v1 (solo esto): la raíz del workspace **no existe en disco** (`os.Stat` falla con NotExist). NADA más (no purgar por antigüedad ni por falta de index).
- Backup antes de `--apply`: copiar `registry.toml` a `registry.toml.bak-<timestamp>` en el mismo dir.
- Salida en el formato estándar del CLI (`--format toon|yaml|compact` si el framework lo da gratis; si no, texto plano tabular).
- Respetar locking/atomicidad que ya use el store del registry para escrituras (imitar el patrón de `workspace add/remove` si existe un remove).
- Test unitario con registry temporal: 2 paths existentes + 2 inexistentes → gc lista 2; apply deja 2.

## Task Metadata
```yaml
id: T7
depends_on: []
agent_type: general-purpose
goal_id: G2
github_issues: []
expected_outcome: "El operador puede reducir 493 workspaces a los reales con un comando seguro"
files:
  - create: C:/repos/mios/mi-lsp/internal/cli/registry_gc.go
  - read: C:/repos/mios/mi-lsp/internal/cli/root.go
complexity: medium
done_when:
  - "go build ./... && go test ./internal/... exit 0"
  - "go run ./cmd/mi-lsp registry gc (dry-run) lista candidatos reales de esta máquina sin modificar el archivo"
evidence_expected:
  - "Salida del dry-run real (conteo de candidatos) + test verde"
stop_if:
  - "el store del registry no expone remove/write seguro y agregarlo excede la tarea (reportar)"
```

## Reference
Buscar con `mi-lsp nav search "registry.toml" --workspace mi-lsp --include-content --format toon` el store y el patrón de comandos en `internal/cli/`.

## Prompt
Protocolo de despacho del plan aplica: micro-lane pi read-only para localizar (a) el store del registry (load/save/remove) y (b) cómo se registran subcomandos en el CLI (patrón cobra u otro). Después implementá el comando según Locked Decisions imitando el comando existente más parecido (`workspace remove` si existe). El dry-run corre sin tocar nada; `--apply` hace backup + rewrite atómico (tmp+rename si el store no da algo mejor).

## Execution Procedure
1. Lane pi de anclajes (store + patrón de comando).
2. Implementá `registry_gc.go` + test.
3. `go build ./... && go test ./internal/...`.
4. Dry-run real: `go run ./cmd/mi-lsp registry gc` y capturá el conteo. NO corras `--apply` (eso lo decide el root en la integración).
5. NO commitees. Reportá diffs + salidas.

## Skeleton
```go
// internal/cli/registry_gc.go
func newRegistryGcCommand() *cobra.Command {
    // --apply flag; default dry-run; os.Stat(ws.Root) -> IsNotExist => candidate
}
```

## Verify
`cd C:/repos/mios/mi-lsp && go build ./... && go test ./internal/...` → exit 0; dry-run lista candidatos

## Commit
`feat(registry): registry gc command to purge dead workspace roots (dry-run default)`
