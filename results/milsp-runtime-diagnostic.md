# milsp runtime diagnóstico (read-only)

**Estado:** diagnóstico parcial; no se editaron archivos ni se ejecutaron tests/build.

## Hallazgos

- `get_errors/v0` declara `checked | partial | not_checked` y `compilation_state: not_checked`; contractualmente un diagnóstico no demuestra compilación: `internal/cli/provider_manifest.go:195-207`.
- `getErrors` mantiene `not_checked` cuando no hay archivos comprobados; marca `partial` cuando hay truncamiento, `NextHint`, `diagnostics.Partial` o archivos faltantes. Propaga `Partial`, `Degraded`, reason, warnings y omissions; `errors_observed` solo se incluye si hubo archivos comprobados. No convierte incertidumbre en resultado limpio: `internal/service/native_verbs.go:320-382`.
- `find_symbol` devolvió evidencia lexical aproximada, `semantic_status=not_started`, y degradación/incompletitud; no es prueba de identidad semántica. `nav related getErrors` informó `active_generation_missing` y grafo desconocido.
- Workspace alias no resolvió para un comando directo; la ruta física absoluta sí funcionó. No se intentó registrar/indexar el workspace.
- El diagnóstico entregado ya fijaba Windows `find`/`rename` positivos como PASS y Tantalo como registrado; no re-registrar Tantalo ni repetir ese trabajo.
- `botsd buscar` agotó su timeout (5000 ms) al leer NATS; no impidió continuar la inspección independiente.

## Paths inspeccionados

- `internal/cli/provider_manifest.go:195-207`
- `internal/service/app.go:125-135`
- `internal/service/native_verbs.go:320-382`
- `internal/cli/native_verbs.go:62-89`

**Paths tocados:** `results/milsp-runtime-diagnostic.md` (solo esta nota; ningún código fuente).

**Siguiente acción:** si se autoriza un follow-up de tests, inspeccionar los tests directos de `getErrors` y validar el envelope parcial. Mantener fuera de alcance las ediciones de mi-mcp y no rebuild/indexar/registrar Tantalo sin autorización.

REPORTE-FINAL: results/milsp-runtime-diagnostic.md
LISTO