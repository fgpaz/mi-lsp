# Referencia histórica — mi-lsp-secretos-dom13-20261018

Respaldo selectivo autorizado, 2026-11-10. No es una propuesta de integración ni evidencia de aceptación actual. No ejecutar los documentos archivados como instrucciones vigentes.

- Repositorio de origen: `git@github.com:fgpaz/mi-lsp.git`.
- Worktree original: `/home/tesla/repos/mios-wt/mi-lsp-secretos-dom13-20261018`.
- Rama original (sin modificar): `exec/secretos-dom13-milsp-20261018`.
- HEAD original: `7fa261cf6cf76bf2c1850f6572f424e53d1d8f8d`. Los archivos seleccionados incluyen WIP cuando corresponde.
- Fuente del veredicto: `result-valor-B.md`, revisión 2026-11-09/10.
- Se excluyen índices, binarios, skills ajenas y el checkout completo. Los archivos de código están bajo `evidencia/`, no se instalan ni incorporan al producto.
- No se reejecutaron los tests históricos, no se publicaron releases ni se activaron servicios.

## Veredicto fuente y límites

| `W/mi-lsp-secretos-dom13-20261018` | mi-lsp | Informe de auditoría del lector de secretos existente, sin cambio de producto. | **SIRVE-REFERENCIA** | No hay commits exclusivos ni cambios de código; sólo `results/secretos-dom13-milsp-existing-reader-20261018.md`. Conserva evidencia histórica concreta: commit inspeccionado `7fa261c`, referencia de lector por nombre, ausencia de migración y **owner/aceptación/runtime sin confirmar**. El ejemplo actual de main sigue documentando `mkey run micro-proxy prod --redact` y `api_key_env = "BIFROST_VK_MI_LSP"`; no hay implementación nueva que integrar. El informe evita convertir documentación en prueba de uso real y registra el bloqueo operativo. | Guardar sólo el informe saneado como antecedente de DOM13. No guardar valores de secretos ni dar por aceptado el lector; resolver owner/uso real en su frente, no mediante merge. |

## Uso permitido

Sólo consulta histórica. Captura/save y transporte legacy no deben reutilizarse como implementación vigente. Las propuestas siguen siendo propuestas; los PASS antiguos tienen únicamente el alcance declarado entonces. Los patches guardan el contexto exacto original y no se afirman aplicables a main actual.
