---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - FL-WIKI-01
allowed_paths:
  - .docs/wiki/01_alcance_funcional.md
  - .docs/wiki/02_arquitectura.md
forbidden_paths:
  - .docs/wiki/03_FL/**
  - .docs/wiki/04_RF/**
  - .docs/wiki/06_pruebas/**
  - .docs/wiki/07_tech/**
  - .docs/wiki/09_contratos/**
  - internal/**
verify:
  - rg -n "FL-WIKI-01" .docs/wiki/01_alcance_funcional.md -> match
  - rg -n "fan-out|federa(ci|tion)" .docs/wiki/02_arquitectura.md -> match
stop_if:
  - .docs/wiki/01_alcance_funcional.md tiene drift severo con respecto a alcances actuales (governance issue)
secret_scan: clean
---

# Task T6: Sincronizar 01_alcance y 02_arquitectura con el alcance del fan-out wiki

## Shared Context
**Goal:** Reflejar en los documentos de alcance funcional y arquitectura que la federación wiki cross-workspace ahora es parte del scope de mi-lsp, sin tocar otros docs.
**Stack:** Markdown SDD.
**Architecture:** 01 y 02 son docs canónicos del repo. 01 lista capacidades; 02 describe arquitectura macro.

## Locked Decisions
- Cambios mínimos y aditivos. NO reescribir secciones, NO reorganizar.
- 01 gana un bullet/sección que enuncia "navegación global de wikis cross-workspace" con link a FL-WIKI-01.
- 02 gana una mención corta del patrón fan-out (intra-máquina) y la nota explícita de que cross-máquina vive fuera de mi-lsp (en clientes externos como Hermes).

## Task Metadata
```yaml
id: T6
depends_on: [T0, T1, T4]
agent_type: ps-docs
goal_id: G1
github_issues: []
expected_outcome: "01_alcance_funcional.md menciona la capacidad nueva; 02_arquitectura.md describe brevemente el fan-out y el límite mi-lsp ↔ cliente externo."
files:
  - modify: .docs/wiki/01_alcance_funcional.md
  - modify: .docs/wiki/02_arquitectura.md
  - read: .docs/wiki/03_FL/FL-WIKI-01.md
  - read: .docs/wiki/07_tech/TECH-WIKI-FANOUT.md
complexity: low
done_when:
  - "01_alcance_funcional.md contiene 'FL-WIKI-01' o 'federación wiki' en al menos una línea nueva"
  - "02_arquitectura.md menciona 'fan-out' o 'federación' y deja explícito que cross-máquina vive fuera de mi-lsp"
  - "no se reorganizan otras secciones ni se cambia harness/wiki-source contracts"
evidence_expected:
  - "Diff completo de ambos archivos"
stop_if:
  - "FL-WIKI-01 o TECH-WIKI-FANOUT no existen (T1/T4 incompletos)"
```

## Reference
- `.docs/wiki/03_FL/FL-WIKI-01.md` (creado en T1).
- `.docs/wiki/07_tech/TECH-WIKI-FANOUT.md` (creado en T4).

## Prompt

Sos el ejecutor de T6 (ps-docs). Cambios pequeños y aditivos en dos archivos.

1. Abrir `.docs/wiki/01_alcance_funcional.md`. Identificar la sección donde se listan capacidades (consultas, indexado, governance, etc.). Agregar una entrada nueva sobre "Federación wiki cross-workspace" — uno o dos enunciados, con link `[[FL-WIKI-01]]`. NO reordenar las demás.
2. Abrir `.docs/wiki/02_arquitectura.md`. Identificar la sección donde se describe el modelo de consultas o el daemon. Agregar UNA mención corta (1-2 párrafos breves o un bullet) describiendo:
   - El fan-out intra-máquina con `--all-workspaces` (semaphore=4).
   - El límite explícito: mi-lsp NO maneja cross-máquina; clientes externos (ej: Hermes) orquestan SSH/Tailscale.
   - Link a `[[TECH-WIKI-FANOUT]]`.
3. Commit: `docs(wiki): sync 01_alcance and 02_arquitectura with wiki federation scope`.
4. Reportar diff.

## Execution Procedure
1. Leer FL-WIKI-01.md y TECH-WIKI-FANOUT.md.
2. Editar 01_alcance_funcional.md (cambio aditivo).
3. Editar 02_arquitectura.md (cambio aditivo).
4. Commit.
5. Reportar.

## Skeleton

(Edit aditivo, no rewrite)

01_alcance_funcional.md (agregar entrada nueva):
```markdown
- **Federación wiki cross-workspace**: navegación global con `--all-workspaces` en los cinco subcomandos `nav wiki *` (search, inventory, route, trace, pack). Ver [[FL-WIKI-01]].
```

02_arquitectura.md (agregar nota):
```markdown
### Fan-out wiki

Las consultas wiki se federan dentro de una máquina vía iterator del registry con semaphore=4. La federación cross-máquina vive **fuera** de mi-lsp: clientes externos (por ejemplo Hermes) orquestan SSH/Tailscale y mergean envelopes per-máquina. Ver [[TECH-WIKI-FANOUT]].
```

## Verify
`rg -n "FL-WIKI-01" .docs/wiki/01_alcance_funcional.md` -> match AND `rg -n "fan-out|federación" .docs/wiki/02_arquitectura.md` -> match

## Commit
`docs(wiki): sync 01_alcance and 02_arquitectura with wiki federation scope`
