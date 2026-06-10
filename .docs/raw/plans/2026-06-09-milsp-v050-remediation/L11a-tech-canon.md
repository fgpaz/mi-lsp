# Task L11a: Sync de canon técnico 07/08/09

## Shared Context
**Goal:** Sincronizar la documentación técnica con el nuevo comportamiento (async-index, admin auth, telemetría, profile agent, doctor).
**Stack:** Markdown wiki, `mi-lsp nav wiki`.
**Architecture:** Corre tras integración+binario (Wave 4). Único dueño de `07_baseline_tecnica.md`, `07_tech/`, `08_modelo_fisico_datos.md`, `08_db/`, `09_contratos_tecnicos.md`, `09_contratos/`.

## Locked Decisions
- Disparadores de sync (CLAUDE.md): runtime/daemon/governance/bootstrap → 07 + TECH-*; persistencia/schema/retención/telemetría → 08 + DB-*; comandos/flags/envelopes/admin API/worker protocol → 09 + CT-*.
- CT-CLI-DAEMON-ADMIN debe reflejar: admin token + Host/Origin, frame size cap, `--profile agent`, `mi-lsp doctor`, recent_accesses default 5, ProtocolVersion requerido.
- 08/DB: nuevos pragmas, índice de telemetría con retención+VACUUM, decision_hash.
- 07/TECH: indexado async-first, caché FTS/rank, timeout per-call al worker, memory reclaim, watcher cap.
- Tras editar, reindexar: `mi-lsp index --workspace axi-smoke` y confirmar `nav governance` `in_sync`.

## Task Metadata
```yaml
id: L11a
depends_on: [B1]
agent_type: ps-worker
goal_id: G6
github_issues: []
expected_outcome: "07/08/09 + owners CT/TECH/DB describen el comportamiento v0.5.0; nav governance in_sync."
files:
  - modify: .docs/wiki/07_baseline_tecnica.md
  - modify: .docs/wiki/09_contratos_tecnicos.md
  - modify: .docs/wiki/08_modelo_fisico_datos.md
complexity: medium
done_when:
  - "mi-lsp nav governance --workspace axi-smoke reports in_sync after reindex"
  - "CT-CLI-DAEMON-ADMIN mentions admin token, --profile agent, mi-lsp doctor"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L11a-verdict.yaml"
stop_if:
  - "docs would describe a behavior the integrated code does not actually have — verify against A1/B1 first"
```

## Reference
CLAUDE.md sección "Documentation Sync Triggers". Owners existentes en `07_tech/`, `08_db/`, `09_contratos/`.

## Prompt
Usá `mi-lsp nav wiki search` para ubicar los owners afectados. Sincronizá 07/08/09 y los CT/TECH/DB relevantes con el comportamiento realmente integrado (verificá contra `A1-integration.yaml` y `release-provenance.yaml`, no contra el plan). Foco en CT-CLI-DAEMON-ADMIN (admin auth, frame cap, profile, doctor, protocol, recent_accesses), 08 (pragmas, retención telemetría, decision_hash), 07 (async-index, cachés, timeout worker, memory, watcher). Reindexá y confirmá `in_sync`. No toques `00_gobierno_documental.md` ni `read-model.toml`.

## Execution Procedure
1. `cd C:/repos/mios/mi-lsp` (canon vive en el repo principal, ya integrado tras F3; en Wave 4 trabajá sobre `v050/integration`).
2. `mi-lsp nav wiki search "<tema>" --workspace axi-smoke --format toon` por cada owner.
3. Editá los docs.
4. `mi-lsp index --workspace axi-smoke`; `mi-lsp nav governance --workspace axi-smoke --format toon` → `in_sync`.
5. Commit. `L11a-verdict.yaml`.

## Verify
`mi-lsp nav governance --workspace axi-smoke` → `sync: in_sync`

## Commit
`docs(tech): sync 07/08/09 + CT/TECH/DB to v0.5.0 behavior`
