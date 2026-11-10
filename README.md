# Referencia histórica — mi-lsp-overlay-go-delta-20261030

Respaldo selectivo autorizado, 2026-11-10. No es una propuesta de integración ni evidencia de aceptación actual. No ejecutar los documentos archivados como instrucciones vigentes.

- Repositorio de origen: `git@github.com:fgpaz/mi-lsp.git`.
- Worktree original: `/home/tesla/repos/mios-wt/mi-lsp-overlay-go-delta-20261030`.
- Rama original (sin modificar): `feat/native-overlay-go-delta-20261030`.
- HEAD original: `2190f5efe956d00f1f9cc84fde78008780fbdeeb`. Los archivos seleccionados incluyen WIP cuando corresponde.
- Fuente del veredicto: `result-valor-B.md`, revisión 2026-11-09/10.
- Se excluyen índices, binarios, skills ajenas y el checkout completo. Los archivos de código están bajo `evidencia/`, no se instalan ni incorporan al producto.
- No se reejecutaron los tests históricos, no se publicaron releases ni se activaron servicios.

## Veredicto fuente y límites

| `W/mi-lsp-overlay-go-delta-20261030` | mi-lsp | Freeze del grafo overlay Go y host capturado, más informes de seguimiento. | **SIRVE-REFERENCIA** | El único commit exclusivo, `2190f5e`, dice explícitamente **not verified**: 42 archivos, 11.164 altas. Su solución de grafo por delta quedó fuera del recorrido actual tras `cd15311`; no recuperaría sus tablas/lectores. Lo que sí vale guardar son **los informes únicos**: `results/overlay-go-delta-20261030.md` identifica el arreglo de `SnapshotFreshness`/`BoundedData` y limita el PASS histórico a suite/compilación sin fixture SQLite vivo; `results/catalog-generation-docs-only-gap-20261101.md` documenta por qué docs-only no puede crear el catálogo ausente de Tantalo y registra la falta de autorización/aceptación. La documentación nativa de código ya está mayormente en main (FL-QRY-01, RF-QRY-026 y CT-DAEMON-WORKER idénticos). | Conservar los informes, especialmente la distinción catálogo/docs y el alcance real de aquella verificación. El código congelado ya tiene respaldo en `2190f5e`; no tratarlo como candidato listo ni reabrir la arquitectura retirada. No indexar Tantalo a partir de este informe. |

## Uso permitido

Sólo consulta histórica. Captura/save y transporte legacy no deben reutilizarse como implementación vigente. Las propuestas siguen siendo propuestas; los PASS antiguos tienen únicamente el alcance declarado entonces. Los patches guardan el contexto exacto original y no se afirman aplicables a main actual.
