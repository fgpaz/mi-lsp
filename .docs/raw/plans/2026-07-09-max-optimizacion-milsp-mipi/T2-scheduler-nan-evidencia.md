# Task T2: Scheduler global NaN (3 slots máquina-wide) + evidencia por spawn

## Shared Context
**Goal:** Que TODOS los workers pi de la máquina (hasta 50 encolados, multi-terminal) respeten `max_concurrent_text_requests=3` de NaN sin timeouts de cola invisible, y que cada spawn deje evidencia yaml.
**Stack:** Node ESM en `C:/repos/mios/mi-pi/scripts/` (`pi-worker-launch.mjs`, `pi-fanout.mjs`) + `extensions/mi-pi-orchestrator/delegate.ts` (importa `launchPiWorker`).
**Architecture:** Hoy `pi-fanout` hace `Promise.all` de N lanes, cada lane crea sesión pi completa ANTES de tener slot de API; `.pi/nan-models.json` declara `rate_limits.max_concurrent_text_requests: 3`. Timeout default 240s por intento (pi-worker-launch.mjs:82). Drift preexistente en pi-worker-launch.mjs (strict model find-miss) que se CONSERVA.

## Locked Decisions
- Nuevo módulo `scripts/nan-slots.mjs` exportando `acquireNanSlot({timeoutMs})` → `{slotId, release()}`.
- Slots dir: `os.homedir() + "/.pi/nan-scheduler/slots/"`. Cantidad de slots: `MI_PI_NAN_MAX_CONCURRENT` env, default **3**.
- Adquisición: intentar crear `slot-<i>.lock` (i de 0 a N-1) con `fs.openSync(path,"wx")` escribiendo `{pid, acquiredAt, laneId}`. Reclaim de stale: si el lock existe con mtime más viejo que `MI_PI_NAN_SLOT_STALE_MS` (default 360000 = timeout 240s + margen), borrarlo y reintentar.
- Espera: polling con backoff 500ms→2s + jitter, hasta `timeoutMs` (default 30 min — las lanes encoladas son baratas). Al expirar → error `nan_slot_queue_timeout` (verdict FAIL con esa razón, NO un timeout genérico).
- **Sesión-dentro-del-slot (punto exacto):** en `attemptWorker()` (pi-worker-launch.mjs:91-126), el `await acquireNanSlot(...)` se inserta INMEDIATAMENTE ANTES de la línea que llama `createAgentSession` (~línea 94, tras el setup de SettingsManager/ResourceLoader que puede quedar fuera del slot); desde ahí hasta `extractAssistantText` inclusive va dentro de `try { ... } finally { slot?.release(); }`. Así 50 lanes encoladas ≈ 0 RAM de sesiones.
- El slot se toma UNA vez por intento de modelo (si escala de modelo, re-adquiere: liberar entre intentos).
- Bypass explícito: `MI_PI_NAN_SCHEDULER=0` desactiva el scheduler (para debug). Modelos NO-nan (sin prefijo `nan/`) no toman slot.
- **Evidencia por spawn** (cierra Gap-1..3 del forensic audit 2026-07-09): la escribe `launchPiWorker()` (el nivel que ve la chain completa y todos los attempts), NO `attemptWorker()`. Cuando `--evidence-root <dir>` está presente (y si no, default `.tmp/pi-workers/<id>-<timestamp>/`), escribir al terminar los 4 archivos con EXACTAMENTE esta estructura (YAML plano por template strings, sin dependencias):
  - `model-selection.yaml`: `schema: model-selection/v1`, `lane_id`, `requested_chain: [..]`, `selected_model` (el activo real reportado), `chain_source: caller_locked|default`.
  - `fallback-chain.yaml`: `schema: fallback-chain/v1`, `lane_id`, `attempts:` lista de `{model, outcome: DONE|FAIL|TIMEOUT|QUEUE_TIMEOUT, reason, duration_ms}`.
  - `verdict.yaml`: `schema: worker-verdict/v1`, `lane_id`, `verdict: DONE|FAIL|BLOCKED`, `model`, `milsp_tools: [..]`, `duration_ms`, `slot_wait_ms`, `summary` (1 línea).
  - `command-status.yaml`: `schema: command-status/v1`, `lane_id`, `command` (argv sin secretos), `timeout_ms`, `exit: completed|timeout|error`.
- `pi-fanout.mjs`: eliminar el cap implícito por `Promise.all` sin control — puede seguir con `Promise.all` porque ahora el scheduler serializa; agregar al progreso por lane el estado `queued(slot)` vs `running`.
- `delegate.ts` no se toca salvo que importe algo renombrado; NO renombrar exports existentes de `pi-worker-launch.mjs`.
- Conservar el drift preexistente (strict find-miss) tal como está en el working tree.
- Nunca imprimir `NAN_API_KEY` ni su contenido.

## Task Metadata
```yaml
id: T2
depends_on: []
agent_type: general-purpose
goal_id: G1
github_issues: []
expected_outcome: "6 lanes con 3 slots completan 6/6 con espera visible y evidencia yaml por spawn"
files:
  - create: C:/repos/mios/mi-pi/scripts/nan-slots.mjs
  - modify: C:/repos/mios/mi-pi/scripts/pi-worker-launch.mjs
  - modify: C:/repos/mios/mi-pi/scripts/pi-fanout.mjs
  - read: C:/repos/mios/mi-pi/extensions/mi-pi-orchestrator/delegate.ts
complexity: high
done_when:
  - "node --check en los 3 scripts exit 0"
  - "smoke de contención: script temporal que adquiere 3 slots y verifica que el 4to espera y adquiere al liberar (sin API real)"
  - "pi-worker-launch --dry-run sigue funcionando idéntico"
evidence_expected:
  - "Salida del smoke de contención + listado de los 4 yaml generados en un --dry-run con --evidence-root"
stop_if:
  - "delegate.ts rompe por cambio de firma (revertir firma, adaptar internamente)"
  - "el working tree de pi-worker-launch.mjs no contiene el drift strict find-miss descrito (reportar)"
```

## Reference
`scripts/pi-worker-launch.mjs:79-164` (chain, timeout, loop de escalación, attemptWorker), `scripts/pi-fanout.mjs` (Promise.all de lanes), `.pi/nan-models.json` → `rate_limits`.

## Prompt
Leé completos `scripts/pi-worker-launch.mjs` y `scripts/pi-fanout.mjs` antes de editar. Implementá `scripts/nan-slots.mjs` y la integración exactamente según Locked Decisions. En `attemptWorker`, envolvé la creación de sesión + prompt así: `const slot = isNanModel(model) && schedulerEnabled() ? await acquireNanSlot({...}) : null; try { ...sesión+prompt actual... } finally { slot?.release(); }`. Medí `slot_wait_ms` (tiempo entre pedir y adquirir) y propagalo al verdict JSON impreso y a `verdict.yaml`. La escritura de evidencia va en una función `writeSpawnEvidence(dir, data)` llamada en el `finally` del launch completo (éxito, FAIL o timeout — SIEMPRE quedan los 4 yaml, regla de micro-lanes del canon). Para el smoke de contención escribí un script temporal en `.tmp/` que importe `nan-slots.mjs`, tome 3 slots, verifique que el 4to `acquireNanSlot({timeoutMs:3000})` falla con `nan_slot_queue_timeout`, libere uno y verifique que ahora adquiere; corrélo con `node` y borralo después.

## Execution Procedure
1. Leé los 3 archivos citados y `.pi/nan-models.json`.
2. Creá `scripts/nan-slots.mjs`.
3. Integrá en `pi-worker-launch.mjs` (slot + evidencia) y `pi-fanout.mjs` (estado queued/running en stderr).
4. `node --check` sobre los 3 archivos.
5. Corré el smoke de contención descrito. Corré `node scripts/pi-worker-launch.mjs --id t2smoke --workspace C:/repos/mios/mi-pi --brief "dry" --dry-run --evidence-root .tmp/t2-evidence` y verificá que los yaml existen.
6. NO hagas `git commit`. Reportá diffs + salidas.

## Skeleton
```javascript
// scripts/nan-slots.mjs
export async function acquireNanSlot({ timeoutMs = 1_800_000, laneId = "" } = {}) {
  const max = Number(process.env.MI_PI_NAN_MAX_CONCURRENT ?? 3);
  // loop: try slot-0..slot-(max-1) with openSync(path, "wx"); stale reclaim; backoff+jitter
  return { slotId, waitMs, release() { /* unlink best-effort */ } };
}
```

## Verify
`cd C:/repos/mios/mi-pi && node --check scripts/nan-slots.mjs && node --check scripts/pi-worker-launch.mjs && node --check scripts/pi-fanout.mjs` → exit 0; smoke de contención OK; dry-run genera 4 yaml

## Commit
`feat(workers): machine-wide NaN slot scheduler + per-spawn evidence pipeline (closes forensic Gap-1..3)`
