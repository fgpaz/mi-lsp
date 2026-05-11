---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - FL-WIKI-01
allowed_paths:
  - .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example
  - .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/T15-hermes-hosts.md
forbidden_paths:
  - .docs/wiki/**
  - internal/**
  - worker-dotnet/**
verify:
  - test -f .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example
  - "ConvertFrom-Yaml on hosts.yaml.example -> parses without error"
stop_if:
  - "PowerShell-Yaml module not installable on host (uncommon — escalate)"
secret_scan: clean
---

# Task T15: Crear ~/.hermes/hosts.yaml schema + ejemplo

## Shared Context
**Goal:** Materializar el schema y un ejemplo del archivo de configuración de hosts del lado Hermes, sin tocar `~/.hermes/` real (vive en companion folder).
**Stack:** YAML.
**Architecture:** El archivo vive en `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example` como referencia. El humano luego copiará el contenido a `~/.hermes/hosts.yaml` en cada máquina.

## Locked Decisions
- Path: `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example` (companion folder; NO `~/.hermes/`).
- Dos hosts: `local`, `tesla-desktop`.
- Defaults: `timeout_seconds: 5`, `semaphore_hosts: 2`.
- Documentar cómo override `mi_lsp_bin` por host.

## Task Metadata
```yaml
id: T15
depends_on: [T0]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "hosts.yaml.example existe en companion folder con schema completo y comentarios explicativos."
files:
  - create: .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example
complexity: low
done_when:
  - "hosts.yaml.example existe"
  - "el YAML parsea sin error (validar con ConvertFrom-Yaml o yq)"
  - "incluye al menos hosts: local + tesla-desktop + defaults timeout/semaphore"
evidence_expected:
  - "Output de ConvertFrom-Yaml -Input (Get-Content ... -Raw) o yq eval . hosts.yaml.example"
stop_if:
  - "ConvertFrom-Yaml o yq no disponibles — validar manualmente y reportar"
```

## Reference
- Decisiones del plan principal sección "Hosts del lado Hermes".
- Spec del wrapper que viene en T16.

## Prompt

Sos el ejecutor de T15 (ps-worker). Crear UN archivo YAML.

1. Crear `.docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example` con el contenido del Skeleton.
2. Validar parseo:
   ```powershell
   $content = Get-Content .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example -Raw
   Install-Module -Name powershell-yaml -Scope CurrentUser -Force -SkipPublisherCheck -ErrorAction SilentlyContinue
   Import-Module powershell-yaml -ErrorAction SilentlyContinue
   $parsed = ConvertFrom-Yaml $content
   $parsed | ConvertTo-Json -Depth 5
   ```
   (Si `powershell-yaml` no está disponible, usar `yq eval . hosts.yaml.example` o validar manualmente leyendo).
3. Confirmar que el output contiene los dos hosts y los defaults.
4. Commit: `feat(hermes): add hosts.yaml schema example for cross-machine wiki nav`.
5. Reportar diff y output del parseo.

## Execution Procedure
1. Escribir el YAML.
2. Validar parseo.
3. Commit.
4. Reportar.

## Skeleton

```yaml
# ~/.hermes/hosts.yaml.example
#
# Inventario de hosts que Hermes consulta cuando recibe el comando
# global de navegación wiki. Cada host se invoca via CLI puro:
#  - transport: local  -> mi-lsp directo en este host
#  - transport: tailscale-ssh -> tailscale ssh <ssh_target> -- mi-lsp ...
#
# mi-lsp permanece CLI puro per-máquina; el merge cross-host vive en Hermes.

hosts:
  - name: local
    transport: local
    # mi_lsp_bin: mi-lsp        # opcional; default = "mi-lsp" en PATH

  - name: tesla-desktop
    transport: tailscale-ssh
    ssh_target: tesla-desktop   # nombre Tailscale del peer
    # mi_lsp_bin: mi-lsp        # opcional; override si en la otra máquina vive en otro path

defaults:
  timeout_seconds: 5           # tiempo máximo por host antes de marcarlo como failed
  semaphore_hosts: 2           # paralelismo entre hosts (con 2 hosts, default 2)

# Notas:
# - Hermes no aborta si un host está apagado o tarda más que timeout_seconds.
#   El host queda en stats.hosts_failed[] y el resto sigue.
# - Para agregar máquinas, agregar entradas bajo `hosts:` sin tocar defaults.
# - mi-lsp NO conoce este archivo — vive enteramente del lado de Hermes.
```

## Verify
`Get-Content .docs/raw/plans/2026-05-11-hermes-wiki-global-nav/hosts.yaml.example | ConvertFrom-Yaml` -> dict con keys `hosts` y `defaults`

## Commit
`feat(hermes): add hosts.yaml schema example for cross-machine wiki nav`
