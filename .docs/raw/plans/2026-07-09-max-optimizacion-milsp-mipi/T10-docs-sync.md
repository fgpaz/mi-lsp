# Task T10: Sync documental — mi-lsp 07/09 y mi-pi 08/10

## Shared Context
**Goal:** Que las wikis reflejen el runtime real post-olas: daemon-first + result cache + límites nuevos + registry gc (mi-lsp) y cache tier-2 + scheduler NaN + evidencia por spawn (mi-pi).
**Stack:** Markdown SDD (`doc_id`/`block_id`/bloques `toon`; sin tablas normativas).
**Architecture:** Triggers de CLAUDE.md (mi-lsp): cambios de runtime/daemon → `07_baseline_tecnica.md` + `07_tech/TECH-*`; cambios de comandos/flags → `09_contratos_tecnicos.md` + `09_contratos/CT-*`. En mi-pi los equivalentes son `08_baseline_tecnica.md`/`08_tech/TECH-*` y `10_contratos_tecnicos.md`/`10_contratos/CT-*`.

## Locked Decisions
- mi-lsp: actualizar el TECH-* de daemon/runtime (buscar con `mi-lsp nav wiki search "daemon" --workspace mi-lsp`) con: routing daemon-first de nav.ask/search/pack (sin auto-start), result cache (256/10min/generación, env `MI_LSP_DAEMON_RESULT_CACHE`), defaults nuevos (inflight 48, pool 6, techo 1024 MB, WAL checkpoint ~30 min). Actualizar el CT-* de comandos con `registry gc` (dry-run default, `--apply`, backup).
- mi-pi: actualizar TECH baseline (TECH-MIPI-BASELINE-08) con cache tier-2 (`~/.mi-lsp/cache/native-milsp/`, TTL 30 min, invalidación por mtime de index.db, singleflight lockfile) y scheduler NaN (`~/.pi/nan-scheduler/`, envs). Actualizar/crear CT de evidencia de workers: los 4 yaml por spawn como contrato del launcher (cierra Gap-1..3 del forensic audit — citarlo).
- Índices raíz (07/09 en mi-lsp, 08/10 en mi-pi): tocar SOLO si el doc delegado nuevo/renombrado lo exige; preferir editar docs delegados existentes.
- Cada bloque normativo nuevo con `block_id` y bloque `toon`; NADA de tablas normativas.
- Tras editar cada repo: `mi-lsp index --workspace <alias>` y verificar `nav governance` sigue `blocked: false`.

## Task Metadata
```yaml
id: T10
depends_on: [T1, T2, T4, T5, T6, T7]
agent_type: ps-docs
goal_id: G3
github_issues: []
expected_outcome: "Wiki y runtime sin drift: un agente que lea 07/09 (mi-lsp) o 08/10 (mi-pi) opera con las reglas reales"
files:
  - modify: C:/repos/mios/mi-lsp/.docs/wiki/07_tech/ (doc de daemon)
  - modify: C:/repos/mios/mi-lsp/.docs/wiki/09_contratos/ (doc de comandos)
  - modify: C:/repos/mios/mi-pi/.docs/wiki/08_tech/TECH-MIPI-BASELINE-08.md
  - modify: C:/repos/mios/mi-pi/.docs/wiki/10_contratos/ (CT de workers/evidencia)
complexity: medium
done_when:
  - "mi-lsp nav governance --workspace mi-lsp --format toon → blocked:false tras reindex"
  - "mi-lsp nav governance --workspace mi-pi --format toon → blocked:false tras reindex"
evidence_expected:
  - "Lista de docs tocados con block_ids nuevos + governance verde en ambos repos"
stop_if:
  - "no existe doc delegado razonable y crear uno requiere crear-capa-tecnica-wiki (reportar en vez de crear ad hoc)"
```

## Reference
Localizar docs objetivo con `mi-lsp nav wiki search "daemon|worker|contrato" --workspace <alias> --format toon`. Forensic audit citable: `C:/repos/mios/mi-pi/.docs/auditoria/2026-07-09-forensic-audit-v020-known-errors.md`.

## Prompt
Protocolo de despacho del plan aplica: usá lanes pi read-only o `mi-lsp nav` para localizar los docs exactos y sus block_ids vecinos antes de editar. Escribí bloques `toon` compactos con los valores exactos de Locked Decisions (no prosa duplicando el toon). Mantené español, voz existente, y los Harness Contracts de cada doc (actualizá `verify`/`evidence` si el doc los declara).

## Execution Procedure
1. Localizá los 4 docs objetivo.
2. Editá con block_ids nuevos y toon.
3. `mi-lsp index --workspace mi-lsp` y `--workspace mi-pi`; `nav governance` en ambos.
4. NO commitees. Reportá docs tocados + governance.

## Skeleton
```markdown
### Result cache del daemon {#block_id: TECH-MILSP-DAEMON-RESULT-CACHE-01}
```toon
result_cache: { max_entries: 256, ttl_min: 10, key: [workspace, index_generation, op, args_hash], disable_env: MI_LSP_DAEMON_RESULT_CACHE=0 }
```
```

## Verify
`mi-lsp nav governance --workspace mi-lsp --format toon && mi-lsp nav governance --workspace mi-pi --format toon` → blocked:false en ambos

## Commit
`docs(wiki): sync 07/09 (mi-lsp) + 08/10 (mi-pi) with scheduler, caches, limits, registry gc`
