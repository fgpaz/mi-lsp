# Referencia histórica — mi-lsp-overlay-full-20261030

Respaldo selectivo autorizado, 2026-11-10. No es una propuesta de integración ni evidencia de aceptación actual. No ejecutar los documentos archivados como instrucciones vigentes.

- Repositorio de origen: `git@github.com:fgpaz/mi-lsp.git`.
- Worktree original: `/home/tesla/repos/mios-wt/mi-lsp-overlay-full-20261030`.
- Rama original (sin modificar): `feat/native-overlay-full-20261030`.
- HEAD original: `efb88b3041f6d1100d272e5e3e84fb8f8eb95db1`. Los archivos seleccionados incluyen WIP cuando corresponde.
- Fuente del veredicto: `result-valor-B.md`, revisión 2026-11-09/10.
- Se excluyen índices, binarios, skills ajenas y el checkout completo. Los archivos de código están bajo `evidencia/`, no se instalan ni incorporan al producto.
- No se reejecutaron los tests históricos, no se publicaron releases ni se activaron servicios.

## Veredicto fuente y límites

| `W/mi-lsp-overlay-full-20261030` | mi-lsp | Prototipo amplio de grafo base+delta, worker C# capturado y endurecimiento de identidad. | **SIRVE-REFERENCIA** | **El overlay no es candidato de integración:** `cd15311` retiró los snapshots de grafo por worktree y `TECH-GRAPH-NATIVE.worktree-graph-query-overlay` declara `retired_for_linked_worktrees`; hoy delta físico devuelve `worktree_graph_unavailable` y deriva a refs en vivo. C# actual sigue usando `MSBuildWorkspace`, no este host capturado. **Sí hay material independiente no presente en main:** `legacyOwnerMatches`, `legacyHealthCache` y `probeLegacyOwner` ligan el health legacy a PID/runID/estado/digest y no convierten la mera conexión bajo contención en health autenticado. Main aún usa mtime/tamaño y acepta dial bajo contención (`internal/daemon/health_cache.go`). También conserva el diseño explícito de procedencia Git para GC (`verifiedWorktreeIdentity`, RF-WKS-007). Es referencia concreta de endurecimiento, no evidencia de fallo explotable ni solución validada. | Guardar respaldo selectivo de los hunks de health/owner y de procedencia/GC con su documentación. No integrar el paquete de overlay/Roslyn ni afirmar que pasó pruebas actuales. Para cambiar main, abrir una corrección acotada con reproducción y tests de identidad/contención. |

## Uso permitido

Sólo consulta histórica. Captura/save y transporte legacy no deben reutilizarse como implementación vigente. Las propuestas siguen siendo propuestas; los PASS antiguos tienen únicamente el alcance declarado entonces. Los patches guardan el contexto exacto original y no se afirman aplicables a main actual.
