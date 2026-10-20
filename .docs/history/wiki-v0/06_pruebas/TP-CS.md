# TP-CS

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TP-CS"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TP-CS]]'
exports:
  - 'TP-CS'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/06_pruebas/TP-CS.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-CS.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_pruebas/TP-CS.md
```

## Cobertura objetivo

- RF-CS-001

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-CS-001 | positivo | RF-CS-001 | resuelve consulta C# con `backend=roslyn` |
| TC-CS-002 | positivo | RF-CS-001 | reutiliza runtime caliente o worker ya disponible |
| TC-CS-003 | negativo | RF-CS-001 | falla con error accionable cuando el worker no esta disponible |
| TC-CS-004 | positivo | RF-CS-001 | `nav context` devuelve `slice_text` y metadatos semanticos en la misma respuesta |
| TC-CS-005 | positivo | RF-CS-001 | si Roslyn no puede enriquecer pero el archivo existe, el core devuelve el slice con warning accionable |
| TC-CS-006 | positivo | RF-CS-001 | si el primer candidato Roslyn falla por bootstrap, el core reintenta una sola vez con el siguiente sin reprobe en cascada |
