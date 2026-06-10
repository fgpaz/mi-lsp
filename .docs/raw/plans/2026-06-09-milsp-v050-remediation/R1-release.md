# Task R1: Release v0.5.0

## Shared Context
**Goal:** Cortar el release v0.5.0 con CHANGELOG, tag, y evidencia AE-RELEASE-DISTRIBUTION.
**Stack:** git, PowerShell, GitHub.
**Architecture:** Wave 5, tras canon+skill sincronizados. Usa el binario ya verificado por B1.

## Locked Decisions
- Versión: `v0.5.0` (release único acordado en brainstorming).
- CHANGELOG con grupos: Runtime (G1), Seguridad (G2), Performance (G3), Tokens (G4), Self-check/Release (G5), Docs/Skill (G6).
- Evidencia AE-RELEASE-DISTRIBUTION: provenance (sha256, vcs_modified=false), install paths, worker status, publish/mirror, bajo `.docs/auditoria/2026-06-09-milsp-v050-remediation/`.
- El tag se crea recién después de que F1/F2/F3 (trazabilidad/auditoría/pre-push) pasen e integren a main. R1 PREPARA el release; el tag final lo coloca F3 tras la integración guarded, o R1 deja el tag listo en la rama integrada para que F3 lo empuje.

## Task Metadata
```yaml
id: R1
depends_on: [L11a, L11b]
agent_type: ps-worker
goal_id: G6
github_issues: []
expected_outcome: "CHANGELOG v0.5.0 completo + evidencia de provenance/distribución lista para el tag."
files:
  - modify: CHANGELOG.md
complexity: medium
done_when:
  - "CHANGELOG.md has a complete v0.5.0 section grouped by goal"
  - "AE-RELEASE-DISTRIBUTION evidence file exists with provenance"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/ae-release-distribution.yaml"
stop_if:
  - "release-provenance.yaml shows vcs_modified=true — do not tag a dirty release"
```

## Reference
`.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`. `release-provenance.yaml` de B1.

## Prompt
Completá el CHANGELOG v0.5.0 agrupado por goal G1-G6 (citando los hallazgos resueltos). Generá `ae-release-distribution.yaml` con: provenance (del release-provenance.yaml de B1), install paths (~/bin + dist/), worker status (build .NET o drift registrado), evidencia de mirror de skill, y plan de publicación. NO crees el tag todavía si F1/F2/F3 no pasaron; dejá el release listo. Si el flujo ya integró a main y los gates pasaron, creá el tag `v0.5.0` sobre el commit integrado.

## Execution Procedure
1. Editá CHANGELOG.md (sección v0.5.0).
2. Generá `ae-release-distribution.yaml`.
3. Verificá `release-provenance.yaml` (vcs_modified=false). Si dirty, STOP.
4. Commit del CHANGELOG. (tag lo coordina F3).

## Verify
`Test-Path .docs/auditoria/2026-06-09-milsp-v050-remediation/ae-release-distribution.yaml` → True; CHANGELOG v0.5.0 presente

## Commit
`chore(release): v0.5.0 changelog and distribution evidence`
