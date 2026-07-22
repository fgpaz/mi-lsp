---
linear_parent: not_applicable_user_authorized
linear_child: not_applicable_user_authorized
anchors: [FL-QRY-01, FL-WIKI-01, RF-QRY-016, RF-GPH-011, CT-GRAPH-CLI, CT-NAV-WIKI, TP-GPH, TP-WIKI]
allowed_paths: [C:/Users/fgpaz/.agents/skills/**, C:/repos/buho/assets/skills/**, .docs/wiki/**, README.md, CLAUDE.md, AGENTS.md, .docs/ae/repo-policy.yaml, scripts/compare-skill-mirrors.ps1]
forbidden_paths: [C:/Users/fgpaz/.claude/skills/**, .docs/wiki/00_gobierno_documental.md, .docs/wiki/_mi-lsp/read-model.toml]
verify: [scripts/compare-skill-mirrors.ps1 -Mode Compare -AsJson, mi-lsp nav wiki validate-harness, mi-lsp nav wiki validate-source]
stop_if: [skill mirror mismatch, policy exposes opt-out for supported intents, canon contradicts runtime]
secret_scan: {required: true, evidence: names-only-no-values}
---
# Task T5: Skills, policy y canon Harness-first

## Shared Context
**Goal:** G5, adopción automática en todos los Harness.
**Stack:** Markdown/YAML skills + wiki contracts + mirror Buho.
**Architecture:** una regla uniforme de intención llama mi-lsp primero y sólo cae a herramientas generalistas con estado terminal explícito.

## Locked Decisions
- Fuente compartida: `C:/Users/fgpaz/.agents/skills`; mirror byte-identical en Buho.
- Sin opt-out para intenciones cubiertas; preview siempre enseña expansión.

## Task Metadata
```yaml
id: T5
depends_on: [T3, T4]
agent_type: ps-worker
goal_id: G5
github_issues: []
expected_outcome: "planning, exploration, review, QA, debugging y traceability usan mi-lsp automáticamente."
files:
  - modify: C:/Users/fgpaz/.agents/skills/mi-lsp/**
  - modify: C:/Users/fgpaz/.agents/skills/ps-contexto/**
  - modify: C:/Users/fgpaz/.agents/skills/writing-plans/**
  - modify: C:/Users/fgpaz/.agents/skills/brainstorming/**
  - modify: C:/Users/fgpaz/.agents/skills/ae-work/**
  - modify: C:/Users/fgpaz/.agents/skills/ps-trazabilidad/**
  - modify: C:/Users/fgpaz/.agents/skills/ps-auditar-trazabilidad/**
  - modify: C:/repos/buho/assets/skills/**
  - modify: .docs/wiki/**
complexity: high
done_when:
  - "selected skill source/mirror trees are byte-identical"
  - "wiki validators pass"
evidence_expected:
  - "inventory of changed skills and hashes"
  - "contract/TP sync list"
stop_if:
  - "a referenced command is absent from real binary"
  - "a mirror cannot be updated in same run"
```

## Reference
Skill `mi-lsp` hot path y `references/compound-commands.md`; mirrors homónimos bajo Buho.

## Prompt
Actualiza mi-lsp, ps-contexto, writing-plans, brainstorming, ae-work, exploration/review/QA/debugging y traceability para enrutar por intención. Define terminales de fallback: unsupported_operation, unavailable_binary, invalid_workspace o explicit_incomplete; timeout silencioso no habilita fallback sin diagnóstico. Añade recipes explain-change, affected, callers/callees, path, explain, graph freshness/rank y expansions. Sincroniza canon CT/TECH/TP/README y copia cada skill modificada al mirror exacto.

## Execution Procedure
1. Confirma comandos reales con `go run ./cmd/mi-lsp nav --help` y tests, no con suposiciones.
2. Actualiza la skill `mi-lsp` y reference compound commands.
3. Actualiza skills consumidoras con matriz intención→comando→fallback.
4. Copia archivos exactos a Buho y compara hashes.
5. Sincroniza CT-GRAPH-CLI, CT-NAV-WIKI, TECH-GRAPH-NATIVE, TP-GPH, TP-WIKI, índices y README.
6. Reindexa docs y ejecuta validators.

## Skeleton
```yaml
intent_routing:
  supported: automatic_mi_lsp_first_no_opt_out
  fallback_only_on: [unsupported_operation, unavailable_binary, invalid_workspace, explicit_incomplete]
  preview_requires: [available_information, expansion_command, expansion_reason]
```

## Test
Validar comandos mencionados, mirror byte parity, Harness/Wiki Source y ausencia de rutas primarias `rg`/Grep/Glob en journeys soportados.

## Verify
`pwsh -File scripts/compare-skill-mirrors.ps1 -Mode Compare -AsJson` + wiki validators -> PASS

## Commit
`docs(harness): make mi-lsp intent routing mandatory - Gabriel Paz -`
