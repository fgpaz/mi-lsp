# Task L11b: Skill mi-lsp + mirror byte-idéntico

## Shared Context
**Goal:** Actualizar la skill mi-lsp con las features nuevas y mantener el mirror byte-idéntico.
**Stack:** Markdown skill.
**Architecture:** Wave 4, en paralelo con L11a. Único dueño de `C:/Users/fgpaz/.agents/skills/mi-lsp/**` (source) y `C:/repos/buho/assets/skills/mi-lsp/**` (mirror).

## Locked Decisions
- Documentar en SKILL.md: `--profile agent` (default para harness), `mi-lsp doctor`, indexado async-first (`--wait`/`--no-index`), nuevos defaults (recent_accesses 5), admin token.
- Source y mirror deben quedar byte-idénticos (sha256 igual) en el mismo run.
- No cambiar la lógica operativa de la skill que no haya cambiado en el binario.

## Task Metadata
```yaml
id: L11b
depends_on: [B1]
agent_type: ps-worker
goal_id: G6
github_issues: []
expected_outcome: "SKILL.md mi-lsp documenta v0.5.0; source y mirror byte-idénticos."
files:
  - modify: C:/Users/fgpaz/.agents/skills/mi-lsp/SKILL.md
  - modify: C:/repos/buho/assets/skills/mi-lsp/SKILL.md
complexity: low
done_when:
  - "scripts/compare-skill-mirrors.ps1 reports byte_identical for mi-lsp"
  - "SKILL.md mentions --profile agent, mi-lsp doctor, async indexing"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L11b-verdict.yaml"
stop_if:
  - "source and mirror hashes differ after edit — re-copy until identical"
```

## Reference
`scripts/compare-skill-mirrors.ps1`. Shared Skill Update And Mirror Gate (ae-programa). CLAUDE.md: "If updating any skill under .agents/skills, also update the mirrored copy under buho/assets/skills".

## Prompt
Actualizá la SKILL.md de mi-lsp en el source con las features nuevas (perfil agent, doctor, indexado async, nuevos defaults, admin token). Copiá el archivo idéntico al mirror. Verificá con `scripts/compare-skill-mirrors.ps1` que quedan byte-idénticos (sha256). Registrá ambos hashes en el verdict.

## Execution Procedure
1. Editá `C:/Users/fgpaz/.agents/skills/mi-lsp/SKILL.md`.
2. Copiá idéntico a `C:/repos/buho/assets/skills/mi-lsp/SKILL.md`.
3. `pwsh scripts/compare-skill-mirrors.ps1` → byte_identical.
4. Commit en cada repo correspondiente. `L11b-verdict.yaml` con sha256 source+mirror.

## Verify
`pwsh scripts/compare-skill-mirrors.ps1` → `byte_identical: true` para mi-lsp

## Commit
`docs(skill): document mi-lsp v0.5.0 features (profile agent, doctor, async index) + mirror`
