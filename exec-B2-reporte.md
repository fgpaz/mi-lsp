# Exec B2 — mi-lsp autocura

## Resultado

Commits publicados directamente en `origin/main`:

- Bloque 1, sin Python: `bf97f9636418339b39b69545da4035bd8b5e0fdb` (`fix(install): validate worker manifest without Python`).
- Bloque 2, resolver wiki: `37c5c642f139bd813d59b01e88849dc2b7d10049`; estados tipados: `2d6b79134f510c47fb85a252024ea964207f56c6`; compatibilidad contrato anidado: `11ee1577953a4724a2de889ee67078f89d671595`.
- Bloque 3, provider/readback/parser: `8af5d366feaabf78f839f32db3366472959fc277`.
- SHA final comprobado con `git ls-remote origin refs/heads/main`: **`11ee1577953a4724a2de889ee67078f89d671595`**.

El código quedó alineado con `origin/main` al verificarlo. La nota local `exec-B2-reporte.md` queda sin commit; la copia durable solicitada está en la ruta de caché indicada al final.

## Evidencia

### 1. Instalación sin Python

Se quitó el parser Python muerto situado tras un `return`; la validación quedó en `scripts/install/validate-worker-manifest.js` (Node). Corrí:

```text
PATH='/c/Program Files/nodejs:/usr/bin:/bin' TMPDIR=/tmp sh scripts/tests/install-worker-manifest.sh
PASS: valid worker manifest accepted
PASS: malformed worker manifest rejected
PATH='/c/Program Files/nodejs:/usr/bin:/bin' bash -n scripts/install/install.sh
python: absent
python3: absent
```

La primera ejecución sin filtrar PATH falló porque `TMPDIR` heredado apuntaba a un directorio inexistente; corregí el entorno a `/tmp` y pasó.

### 2. Resolver y estados

Se añadió `mi-lsp nav wiki resolve --root <dir> --json`, contrato `sandinas-wiki-resolution/v1`, exit 0 resuelto / 2 pending, lectura JSON embebido y YAML, `wiki` anidado bajo `project` y `wiki_root` legacy. El test `TestResolveWikiSupportsJSONAndLegacyWikiRoot` pasa. `workspace status` expone estados independientes `runtime`, `workspace`, `index`, `wiki`, `policy`, con `state` y `action`; `TestWorkspaceTypedStatesStayIndependent` pasa. `q` intenta autorregistro e indexación automática al primer uso si no se pidió `--no-auto-register`; se conserva el camino directo si no se puede usar el daemon.

Comandos reales sobre las rutas indicadas por el brief:

```text
/tmp/mi-lsp-b2 nav wiki resolve --root /c/repos/sandinas/Marketplace --json
=> state=pending, pending=true, notes=[governance_file_not_found], exit 2
/tmp/mi-lsp-b2 nav wiki resolve --root /c/repos/sandinas/Lumi --json
=> state=pending, pending=true, notes=[governance_file_not_found], exit 2
/tmp/mi-lsp-b2 nav wiki resolve --root /c/repos/sandinas/sandinas-proxy --json
=> state=pending, pending=true, notes=[governance_file_not_found], exit 2
/tmp/mi-lsp-b2 nav wiki resolve --root . --json
=> state=resolved, pending=false, canon_root=<worktree>/.docs/wiki, exit 0
```

Los tres checkouts Sandinas existentes no contienen los archivos `.docs/wiki/00_governance.md` que el brief cita; por eso no presento esos tres `pending` como resolución de sus contratos reales. `testdata/wiki-resolver/topologies.json` queda como fixture de cuatro formas, pero no se ejecutó una matriz automatizada completa sobre sus cuatro entradas.

### 3. Defectos

- **provider v2/argv:** antes, `go test ./internal/cli -run TestProviderManifestCommandPrintsContractAndRealQFlags` fallaba: snapshot/contrato no documentaba invocación por operación `q` y el test asumía 9 operaciones cuando la CLI publicaba 13. Se agregó el argv/mapping v2 para `q`, se alineó el test con las 13 operaciones reales y regeneré `integrations/mi-mcp/provider-manifest.json`. El test dirigido pasa.
- **readback budget:** el caso pequeño de `CapRendered` podía producir marcador + continuación mayores que `max_chars` cuando ambos ya excedían el límite. Ahora cae a prefijo acotado en ese caso; `TestCapRenderedNeverExceedsTinyReadbackBudget` pasa.
- **ae_canon:** el extractor anterior dependía de YAML delimitado; se añadió extracción balanceada de objeto JSON embebido para que la proyección no pierda el metadata `ae_canon`. `TestWikiContractProjectionReadsJSONGovernanceBlock` pasa y verifica `kernel_v2` en la proyección.

### Suites

`go build ./...` pasó en las verificaciones previas a los pushes. `go test ./...` no está verde en el host. Los fallos siguientes se reprodujeron también en worktree limpio de `5e90791` (sin mis cambios), con los mismos resultados:

- `internal/docgraph`: `TestBuildWikiMatrixParserWarningVersusGapR2`, `TestBuildWikiMatrixFixture`.
- `internal/service`: `TestDiagnosticsGitUnionAndMergeBase`, `TestFindFilesGitIgnoreRankingBudgetAndCache`.
- `internal/workspace`: `TestRegisteredMainWorktreePrefersLastWorkspaceThenLexicalAlias`; confirmado preexistente en base limpia, salida `registeredMainWorktree() = ({Name: Root: ...}, false)`.

En una corrida adicional apareció el fallo no determinista `internal/service/TestFindRefsSemanticTimeoutFallsBackToTextAndCoolsDown`; no apareció en la corrida final antes del reporte.

## Paths modificados por esta ejecución

- `exec-B2-reporte.md` (nota durable del ejecutor)
- `scripts/install/install.sh`
- `scripts/install/validate-worker-manifest.js`
- `scripts/tests/install-worker-manifest.sh`
- `testdata/wiki-resolver/topologies.json`
- `internal/cli/nav.go`
- `internal/cli/wiki_resolve.go`
- `internal/cli/wiki_resolve_test.go`
- `internal/cli/provider_manifest.go`
- `internal/cli/provider_manifest_test.go`
- `integrations/mi-mcp/provider-manifest.json`
- `internal/output/truncator.go`
- `internal/output/truncator_test.go`
- `internal/docgraph/wiki_contract_projection.go`
- `internal/docgraph/wiki_contract_projection_test.go`
- `internal/service/primitives_q.go`
- `internal/service/workspace_ops.go`
- `internal/service/workspace_typed_states_test.go`

BLOQUEADO: no pude demostrar la resolución de las tres wikis reales ni el primer `q` con indexación completa porque faltan los governance files citados en esos checkouts; la suite completa además conserva fallos preexistentes documentados arriba. El commit ya está en main; para cerrar esa verificación hace falta ejecutar contra checkouts que sí contengan esos contratos y repetir el primer uso en un repo sin índice.

REPORTE-FINAL: C:/Users/fgpaz/.cache/sandinas-roadmap-20261019/exec-B2-reporte.md
TRABADO
