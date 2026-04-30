# TECH-SERVICE-EXPLORATION

```yaml
harness_protocol: SDD-HARNESS-v1
id: "TECH-SERVICE-EXPLORATION"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[TECH-SERVICE-EXPLORATION]]'
exports:
  - 'TECH-SERVICE-EXPLORATION'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/07_tech/TECH-SERVICE-EXPLORATION.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-SERVICE-EXPLORATION.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-SERVICE-EXPLORATION.md
```

Volver a [07_baseline_tecnica.md](../07_baseline_tecnica.md).

## Summary

Define el perfil tecnico de `nav service`: un agregador evidence-first que combina catalogo repo-local y busqueda textual scoped a un path para resumir la superficie observable de un servicio.

## Owner and scope

- Owner logico: Core runtime / Query layer
- Scope: agregacion de evidencia, deteccion de placeholders, perfilado generico vs `.NET microservice`
- Non-goals: score fuerte de completitud, auditoria funcional completa, dependencia de Roslyn

## Runtime contract

- Input canonico: `nav service <path> --workspace <alias>`
- Fuentes permitidas en v1:
  - catalogo repo-local (`symbols`, `files`)
  - busqueda textual scoped al path
- Fuentes excluidas en v1:
  - Roslyn obligatorio
  - fanout automatico multi-repo
  - analisis de completitud basado en pesos o porcentajes

## Evidence families

- `symbols`: conteo por kind observado en catalogo
- `http_endpoints`: `MapGet|MapPost|MapPut|MapDelete|MapPatch`
- `event_consumers`: ocurrencias `IConsumer<...>`
- `event_publishers`: ocurrencias `PublishAsync<...>` o `IPublishEndpoint`
- `entities`: clases/records bajo `Domain/Entities` o `Domain/Models`
- `infrastructure`: wiring como EventBus, Redis, Npgsql, SqlServer o InMemory
- `archetype_matches`: placeholders detectados y filtrados por default

## Reliability posture

- El output debe ser accionable pero no autoritativo sobre completitud.
- Cuando el catalogo es insuficiente, el comando degrada a texto y emite warning.
- Cuando se detectan placeholders conocidos, deben quedar trazados en `archetype_matches`.
- `--include-archetype` habilita reinsertar esa evidencia filtrada, no cambia el resto del contrato.

## Related docs

- [09_contratos_tecnicos.md](../09_contratos_tecnicos.md)
- [CT-CLI-DAEMON-ADMIN.md](../09_contratos/CT-CLI-DAEMON-ADMIN.md)
- [FL-QRY-01](../03_FL/FL-QRY-01.md)
