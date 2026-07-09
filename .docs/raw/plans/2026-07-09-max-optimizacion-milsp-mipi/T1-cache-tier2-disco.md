# Task T1: Cache tier-2 en disco compartido para seed/context del provider nativo

## Shared Context
**Goal:** Que N procesos pi concurrentes sobre el mismo workspace hagan 1 solo `mi-lsp seed` + `context compile` por TTL, compartiendo el resultado por disco.
**Stack:** TypeScript ESM, extensión pi en `C:/repos/mios/mi-pi/extensions/mi-pi-native-milsp/index.ts`.
**Architecture:** Hoy `contextCache`/`contextInflight` son `Map` en memoria del proceso (index.ts:76-77), TTL 5 min (línea 12, override `MI_PI_NATIVE_MILSP_TTL_MS`), singleflight solo intra-proceso (líneas 279-286). El seed corre `mi-lsp seed` (línea 311, 120s) y `context compile` (línea 323, 120s).

## Locked Decisions
- El cache persistente vive en `os.homedir() + "/.mi-lsp/cache/native-milsp/"` (crear con `mkdir recursive`).
- Key de archivo: `sha1(normalizedWorkspace)` (normalización existente de línea 348) + `.json`.
- Invalidación: el record persistido guarda `indexDbMtimeMs` = mtime de `<workspace>/.mi-lsp/index.db` al momento del seed. Un record es válido si (a) no expiró el TTL y (b) el mtime actual del index.db es igual al guardado. Si index.db no existe, se comporta como hoy (sin tier-2, solo memoria).
- TTL default sube de 5 a **30 minutos** (la invalidación por generación lo hace seguro). `MI_PI_NATIVE_MILSP_TTL_MS` sigue mandando si está seteado.
- Singleflight cross-proceso: lockfile `<cacheDir>/<sha1>.lock` creado con `fs.openSync(path, "wx")` escribiendo `{pid, startedAt}`. Si el lock existe y su mtime tiene más de 150.000 ms, se considera stale: borrarlo y reintentar una vez. Mientras el lock esté vivo, el proceso que NO lo tiene hace polling cada 2s (máx 150s) esperando que aparezca el JSON fresco; si expira el polling, sigue el flujo actual (seed propio) para no deadlockear.
- El `Map` en memoria se mantiene como tier-1 (lookup más barato); el disco es tier-2. Orden de lookup: memoria → disco → seed.
- Escritura atómica: escribir a `<file>.tmp` y `fs.renameSync` al nombre final.
- No tocar el guard de paths, la clasificación de prompts ni el followUp gate (ya arreglados el 2026-07-09).
- No agregar dependencias npm nuevas (usar `node:crypto`, `node:fs`, `node:os`, `node:path`).

## Task Metadata
```yaml
id: T1
depends_on: []
agent_type: general-purpose
goal_id: G1
github_issues: []
expected_outcome: "Segundo proceso pi sobre el mismo workspace reusa seed/context desde disco sin lanzar mi-lsp"
files:
  - modify: C:/repos/mios/mi-pi/extensions/mi-pi-native-milsp/index.ts
complexity: medium
done_when:
  - "node --check extensions/mi-pi-native-milsp/index.ts exit 0"
  - "npm run mi-pi:check exit 0"
  - "npm run mi-pi:native-milsp-smoke exit 0 con verdict PASS"
evidence_expected:
  - "Salida de los 3 comandos + descripción del flujo tier-1/tier-2 implementado"
stop_if:
  - "index.ts difiere estructuralmente de las líneas citadas (leerlo primero; si los anclajes no existen, reportar y no improvisar)"
  - "el smoke falla tras el cambio"
```

## Reference
`C:/repos/mios/mi-pi/extensions/mi-pi-native-milsp/index.ts:76-77` (Maps actuales), `:279-357` (singleflight + ensureNativeContext + freshContextForWorkspace + normalización), `:12` (TTL const).

## Prompt
Abrí y leé COMPLETO `extensions/mi-pi-native-milsp/index.ts` antes de editar. Implementá un tier-2 de disco para el record de contexto nativo siguiendo exactamente las Locked Decisions. Pasos concretos: (1) agregá helpers `cacheDir()`, `cacheFileFor(wsKey)`, `readDiskRecord(wsKey)` (parse JSON, validar TTL + `indexDbMtimeMs`), `writeDiskRecord(wsKey, record)` (tmp+rename), `acquireSeedLock(wsKey)`/`releaseSeedLock` y `waitForDiskRecord(wsKey, maxMs)`. (2) En `freshContextForWorkspace` (o donde se hace el lookup del Map), si hay miss en memoria, consultá disco; si hay hit válido, hidratá el Map y devolvelo. (3) En el camino de seed (`ensureNativeContextSingleflight`), antes de lanzar seed intentá `acquireSeedLock`; si no lo conseguís, `waitForDiskRecord`; si lo conseguís, corré el seed/compile actual, persistí a disco en el mismo punto donde hoy se puebla el Map, y liberá el lock en un `finally`. (4) Subí `DEFAULT_TTL` a 30 min. (5) Capturá el mtime de `.mi-lsp/index.db` ANTES de lanzar el seed y guardalo en el record. No reformatees el resto del archivo. Errores de disco (EACCES, JSON corrupto) se tragan con degradación al comportamiento actual — nunca deben romper un prompt.

## Execution Procedure
1. Leé `extensions/mi-pi-native-milsp/index.ts` completo y ubicá los anclajes citados.
2. Aplicá los cambios de las Locked Decisions con Edit (no reescribas el archivo entero).
3. Corré `node --check extensions/mi-pi-native-milsp/index.ts`.
4. Corré `npm run mi-pi:check` y `npm run mi-pi:native-milsp-smoke` desde `C:/repos/mios/mi-pi`.
5. Si algo falla, arreglá tu cambio; si el fallo es preexistente, deteneté y reportalo.
6. NO hagas `git commit`. Reportá diff aplicado + salidas de verify.

## Skeleton
```typescript
const NATIVE_CACHE_DIR = join(homedir(), ".mi-lsp", "cache", "native-milsp");
interface DiskRecord { record: NativeMiLspContextRecord; expiresAt: number; indexDbMtimeMs: number | null; }
function readDiskRecord(wsKey: string, wsPath: string): NativeMiLspContextRecord | null { /* TTL + mtime check */ }
function writeDiskRecord(wsKey: string, wsPath: string, record: NativeMiLspContextRecord, ttlMs: number): void { /* tmp + rename */ }
```

## Verify
`cd C:/repos/mios/mi-pi && node --check extensions/mi-pi-native-milsp/index.ts && npm run mi-pi:check && npm run mi-pi:native-milsp-smoke` → todo exit 0, smoke PASS

## Commit
`perf(native-milsp): shared disk tier-2 cache + cross-process singleflight for seed/context`
