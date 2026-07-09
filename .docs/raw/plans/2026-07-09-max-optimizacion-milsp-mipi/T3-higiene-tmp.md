# Task T3: Higiene automática de `.tmp` + check de output vacío (BUG-4/BUG-5)

## Shared Context
**Goal:** Cortar la acumulación de `.tmp` de mi-pi (273 MB / 5.827 dirs medidos el 2026-07-09) con TTL de 7 días y detectar outputs vacíos de smoke.
**Stack:** Node ESM en `C:/repos/mios/mi-pi/scripts/`, npm scripts en `package.json`.
**Architecture:** Los smokes y workers escriben bajo `.tmp/<session>/` sin garbage collection (BUG-4 del forensic audit); hay `.err/.out` de 0 bytes que ocultan fallos (BUG-5).

## Locked Decisions
- Nuevo `scripts/pi-tmp-clean.mjs`: recorre SOLO `C:/repos/mios/mi-pi/.tmp/` primer nivel; borra recursivamente dirs cuyo mtime (del propio dir) sea más viejo que `--days N` (default 7). `--dry-run` es el DEFAULT (lista qué borraría y MB); `--apply` borra.
- Exclusiones: nunca borrar `.tmp/pi-workers/` con mtime < 1 día ni ningún path fuera de `.tmp`.
- Nuevo npm script `"mi-pi:tmp-clean": "node scripts/pi-tmp-clean.mjs"` en `package.json`.
- BUG-5: agregar a `pi-tmp-clean.mjs` un modo `--report-empty` que liste archivos `.err`/`.out` de 0 bytes con menos de 7 días (señal de fallo silencioso), sin borrarlos.
- Ejecutar UNA limpieza real en esta tarea: `node scripts/pi-tmp-clean.mjs --days 7 --apply` y registrar MB liberados.
- No tocar `.docs/`, no tocar `.pi/`.

## Task Metadata
```yaml
id: T3
depends_on: []
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: ".tmp deja de crecer sin límite; limpieza inicial libera ~200+ MB"
files:
  - create: C:/repos/mios/mi-pi/scripts/pi-tmp-clean.mjs
  - modify: C:/repos/mios/mi-pi/package.json
complexity: low
done_when:
  - "node --check scripts/pi-tmp-clean.mjs exit 0"
  - "--dry-run lista candidatos con tamaños; --apply reduce el conteo de dirs"
evidence_expected:
  - "Conteo de dirs y MB antes/después de --apply"
stop_if:
  - "el script resolvería paths fuera de .tmp (bug de path traversal: parar)"
```

## Reference
Medición 2026-07-09: `du .tmp` = 273 MB, 5.827 dirs; top: `mi-pi-auto-workflows/` 128 MB.

## Prompt
Creá `scripts/pi-tmp-clean.mjs` según Locked Decisions. Usá `path.resolve` y verificá con `resolved.startsWith(tmpRoot)` antes de cualquier `rm`. Salida: una línea por candidato `DELETE <dir> <MB> <age-days>` y resumen final `total: N dirs, M MB`. Después de verificar `--dry-run`, corré `--days 7 --apply`, y registrá antes/después con `Get-ChildItem .tmp | Measure-Object` o `find .tmp -maxdepth 1 -type d | wc -l` + tamaño.

## Execution Procedure
1. Creá el script y agregá el npm script en `package.json` (leé el bloque scripts existente primero, no lo reordenes).
2. `node --check scripts/pi-tmp-clean.mjs`.
3. Corré `node scripts/pi-tmp-clean.mjs` (dry-run) y capturá el resumen.
4. Corré `node scripts/pi-tmp-clean.mjs --days 7 --apply` y capturá antes/después.
5. Corré `node scripts/pi-tmp-clean.mjs --report-empty` y capturá el listado.
6. NO hagas `git commit`. Reportá todo.

## Skeleton
```javascript
const TMP_ROOT = resolve("C:/repos/mios/mi-pi/.tmp");
const days = argVal("--days", 7);
for (const entry of readdirSync(TMP_ROOT, { withFileTypes: true })) { /* mtime check, guard, rm */ }
```

## Verify
`cd C:/repos/mios/mi-pi && node --check scripts/pi-tmp-clean.mjs && node scripts/pi-tmp-clean.mjs` → exit 0 con resumen

## Commit
`chore(tmp): add pi-tmp-clean TTL garbage collection + empty-output report (BUG-4/BUG-5)`
