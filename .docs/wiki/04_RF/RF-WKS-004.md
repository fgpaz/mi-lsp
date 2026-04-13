# RF-WKS-004 - Exponer AXI selectivo por superficie para onboarding y discovery del CLI

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-WKS-004 |
| Titulo | Exponer AXI selectivo por superficie para onboarding y discovery del CLI |
| Actores | Desarrollador, Skill, Agente, CLI/Core |
| Prioridad | media |
| Severidad | media |
| FL origen | FL-BOOT-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| CLI publica disponible | tecnica | obligatorio |
| Resolucion centralizada del modo efectivo por superficie | funcional | obligatorio |
| Si se usa `--full`, el comando debe quedar en modo AXI efectivo | funcional | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI resuelve el modo efectivo combinando defaults por superficie, `--axi`, `--classic` y `MI_LSP_AXI=1`.
2. `mi-lsp` sin subcomando entra en AXI por default y devuelve un home content-first salvo `--classic`.
3. El home intenta resolver el workspace por `--workspace`, `cwd` o ultimo workspace registrado.
4. `init`, `workspace status`, `nav search` y `nav intent` pertenecen a la superficie AXI-default y arrancan en preview-first si no hubo `--classic`.
5. `nav ask` entra en AXI por default solo para preguntas cortas de onboarding/orientacion; las preguntas con señales de implementacion quedan clasicas salvo `--axi`.
6. `nav workspace-map` y el resto de la CLI quedan en modo clasico por default; pueden entrar en AXI solo por `--axi` o `MI_LSP_AXI=1`.
7. Si el usuario define `--format`, `--max-items`, `--max-chars` o `--token-budget`, esos overrides ganan sobre defaults AXI.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `WKS_AXI_UNSUPPORTED_SURFACE` | expansion fuera de alcance | `--full` en un comando no cubierto | ignorar o degradar sin romper el comando base |
| `WKS_AXI_WORKSPACE_UNRESOLVED` | sin contexto de workspace | root AXI no puede resolver workspace actual/default | devolver home con sugerencias de bootstrap, sin side effects |

## 5. Special Cases and Variants

- `--axi=false` debe poder anular `MI_LSP_AXI=1`.
- `--classic` debe prevalecer sobre defaults por superficie y sobre `MI_LSP_AXI=1`.
- `--axi` y `--classic` juntos deben fallar con error claro.
- La version actual no usa hooks ni contexto ambiente fuera del proceso CLI.
- El home AXI puede mostrar readiness barata (daemon/worker) pero no debe mutar runtime solo para renderizar el overview.

## 6. Data Model Impact

- `QueryOptions`
- `QueryEnvelope`
