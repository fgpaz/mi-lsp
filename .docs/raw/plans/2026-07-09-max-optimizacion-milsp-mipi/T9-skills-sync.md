# Task T9: Sync de skills — ae-worker-pi (+ mirror buho) con las reglas nuevas

## Shared Context
**Goal:** Que la skill canónica enseñe el scheduler global de máquina y el cache tier-2, y que el mirror quede idéntico.
**Stack:** Markdown. Skill: `C:/Users/fgpaz/.agents/skills/ae-worker-pi/SKILL.md`; mirror: `C:/repos/buho/assets/skills/ae-worker-pi/SKILL.md`.
**Architecture:** Regla de repo: si se toca una skill bajo `~/.agents/skills`, el mirror en buho se actualiza en la misma tarea. La skill hoy dice "Maximo 3 lanes paralelas por round" (límite por terminal).

## Locked Decisions
- Agregar sección nueva `## Scheduler global NaN (2026-07-09)` después de `## Native mi-lsp quiet/cache gate (2026-07-09)` con exactamente estos puntos:
  - Los 3 slots de `max_concurrent_text_requests` son DE LA MÁQUINA, no de la terminal; `scripts/nan-slots.mjs` los arbitra vía `~/.pi/nan-scheduler/slots/` (env `MI_PI_NAN_MAX_CONCURRENT`, default 3; bypass `MI_PI_NAN_SCHEDULER=0`).
  - Las lanes encoladas son baratas (la sesión pi se crea recién al adquirir slot): está permitido encolar muchas lanes (hasta ~50) en `pi-fanout`; el throughput real sigue siendo ~3-5 lanes/min.
  - `nan_slot_queue_timeout` es una clase de fallo distinta de `worker_timeout`: no escalar de modelo por cola llena.
  - `slot_wait_ms` aparece en el verdict; espera larga = señal de sobredemanda, no de worker lento.
- Actualizar la sección `## Micro-lanes y cierre obligatorio (2026-07-06)`: reemplazar la regla "Maximo 3 lanes paralelas por round ... rounds secuenciales" por: el scheduler global arbitra los slots — se pueden ENCOLAR más lanes por round, manteniendo micro-packets de una sola pregunta; la evidencia terminal (verdict/model-selection/fallback-chain/command-status) ahora la escribe el launcher automáticamente con `--evidence-root`.
- Agregar 2 bullets a `## Native mi-lsp quiet/cache gate (2026-07-09)`: cache tier-2 en disco `~/.mi-lsp/cache/native-milsp/` compartido entre procesos, keyed por generación del index.db, TTL default 30 min; singleflight cross-proceso por lockfile.
- Después de editar: copiar el archivo COMPLETO al mirror y verificar `sha256` idéntico.
- NO tocar otras secciones ni reformatear.

## Task Metadata
```yaml
id: T9
depends_on: [T1, T2]
agent_type: ps-worker
goal_id: G3
github_issues: []
expected_outcome: "Cualquier harness que cargue ae-worker-pi opera con scheduler global y cache compartido"
files:
  - modify: C:/Users/fgpaz/.agents/skills/ae-worker-pi/SKILL.md
  - modify: C:/repos/buho/assets/skills/ae-worker-pi/SKILL.md
complexity: low
done_when:
  - "sha256 idéntico entre skill y mirror"
  - "las 3 ediciones presentes (sección nueva, micro-lanes actualizado, 2 bullets cache)"
evidence_expected:
  - "sha256 de ambos archivos + diff de las secciones tocadas"
stop_if:
  - "el mirror difiere HOY de la skill canónica antes de editar (reportar diff y parar)"
```

## Reference
`C:/Users/fgpaz/.agents/skills/ae-worker-pi/SKILL.md` líneas 70-143 (secciones a tocar). Antes de editar, verificar sha256 actual de ambos == `4784ef0701bb6c9325482a94ca6e37ae41e873291ab5d7e986da9d49a0d4825c`.

## Prompt
Ediciones quirúrgicas de prosa según Locked Decisions, en español con la misma voz del documento. Verificá primero que skill y mirror son idénticos (sha256 citado); si no, STOP. Aplicá las 3 ediciones en la skill canónica, copiá al mirror, verificá hashes.

## Execution Procedure
1. `Get-FileHash` de ambos archivos; comparar con el sha esperado.
2. Editar la skill canónica (3 ediciones).
3. `Copy-Item` al mirror; `Get-FileHash` de ambos → idénticos.
4. NO commitees. Reportá hashes + secciones.

## Skeleton
```markdown
## Scheduler global NaN (2026-07-09)
Los 3 slots de `max_concurrent_text_requests` son de la MÁQUINA, no de la terminal...
```

## Verify
`Get-FileHash` skill == mirror

## Commit
`docs(skill): ae-worker-pi machine-wide NaN scheduler + tier-2 cache rules (+ buho mirror)`
