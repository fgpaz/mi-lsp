---
wiki_contract: WIKI-CONTRACT-v1
id: TP-WKS-PROBE
kind: test
status: active
title: TP-WKS-PROBE - Plan de pruebas del Batch B
links:
  verifies:
  - RF-WKS-007
---

## Alcance

RF-WKS-007. Se conserva el inventario de escenarios, límites, disposiciones y evidencia histórica, sin reejecución.

## Casos

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 1
  - RF-WKS-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: La raíz mostrada conserva una forma estable y legible para el usuario. La identidad física se calcula de forma absoluta, limpia y, cuando el path existe, con evaluación de symlink/junction. La comparación solo ignora casing en plataformas con semántica de filesystem insensible al casing. Dos paths físicamente distintos no se agrupan por una normalización que solo baje a minúsculas.
La respuesta para agentes identifica en su primera línea el workspace seleccionado. Esto hace visible el alias o selector efectivo sin modificar la identidad física ni la resolución fail-closed.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 2
  - RF-WKS-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: Un selector explícito inválido o stale nunca puede convertirse silenciosamente en el workspace del `caller_cwd`. La resolución omitida conserva la precedencia contextual existente y expone `source` y warnings suficientes para auditoría.
Los aliases heredados que apunten a la misma raíz física siguen siendo válidos y se conservan; la presentación compacta no los migra, elimina ni redirige. Un alias explícito desconocido o stale sigue fallando sin sustituirse por el workspace del cwd.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 3
  - RF-WKS-007 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: Cuando el `caller_cwd` omitido está dentro de un linked worktree, la resolución compara `git dir` y `git common dir` como rutas absolutas, canónicas y normalizadas. Si el root principal está registrado, devuelve ese alias y no crea ni indexa un alias para el linked worktree. Un **path explícito** a un linked worktree no registrado en operaciones read-only conserva la raíz física del target (Git, frontier y generaciones del store existente de ese root) sin auto-registro ni fallback al main. Las operaciones mutantes con path explícito siguen resolviendo al main registrado cuando corresponda. Si el main no está registrado, devuelve `explicit_incomplete` con `reason_code=invalid_workspace` y no inicia indexación. `MI_LSP_AUTOREGISTER=force` opta explícitamente por registrar el linked worktree como raíz física independiente.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 4
  - RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 4. Disposición: manual_evidence_pending. Evidencia esperada: Las operaciones que requieren workspace (`nav.*`, `index.*`, `info`, `workspace.status`) ejecutan un único camino compartido por CLI y daemon (`App.Execute`, `internal/service/auto_register.go` y `internal/workspace/autoregister.go`), de modo que sirve igual a la CLI y a `mi-mcp`, que la invoca por llamada. [RETIRADO 2026-10-15: `mi-lsp mcp`, el plugin milsp y el hijo persistente de mi-mcp.]

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 5
  - RF-WKS-007 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: La especificación normativa del auto-registro, incluida la respuesta cuando el repo aún no tiene commits, el alcance del opt-out, linked worktrees, force y GC del registry, está agrupada en el bloque TOON `RF-WKS-007-B08`. Los casos de verificación se definen normativamente en `TP-WKS` (`TC-WKS-048..060`, `TC-WKS-064..066`). El enlace al flag CLI se apoya en evidencia de código; `TC-WKS-049` no declara cobertura E2E de su propagación.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 6
  - RF-WKS-007 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: La configuración que debe viajar con el repositorio permanece repo-local. El estado operativo se resuelve a una ruta local de la máquina y no se incorpora a la configuración portable. El estado legacy se conserva durante esta ola; cualquier compatibilidad o migración se informa explícitamente y no mueve ni borra archivos automáticamente. Un worktree y su parent, aun cuando compartan `git_common_dir`, conservan roots canónicos y operational paths distintos.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 7
  - RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: `mi-lsp probe` puede ejecutarse sin daemon y sin DB. Si existe una DB, solo puede abrirla en modo SQLite `ro` real; si no existe, debe reportar evidencia ausente sin crear directorios ni archivos. El probe no inicializa schema, no aplica migraciones, no escribe registry, no crea snapshots, no envía telemetría y devuelve `side_effects:false` en salida estructurada.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 8
  - RF-WKS-007 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 8. Disposición: manual_evidence_pending. Evidencia esperada: Los estados son diagnósticos, no instrucciones implícitas de reparación. La salida incluye evidencia mínima, migration status, resolución del workspace y provenance reutilizable de `mi-lsp version` cuando corresponda.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

#### TP-WKS-PROBE / RF-WKS-007 / Criterio 9
  - RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: Las pruebas cubren snapshots antes/después de parent, worktree, home y fixtures temporales, alias inexistente y stale, resolución contextual sin selector, casing por plataforma, symlink/junction cuando el host lo permite, DB inexistente y apertura read-only sin WAL/SHM. Toda prueba que observe una mutación del filesystem o registry hace fallar el batch. La única oracle de formato para todos los Go tocados es la orden dinámica y no mutante declarada en `verify`: descubre el conjunto completo desde Git, no usa listas manuales y nunca escribe archivos.

Se contrasta con los escenarios y oráculos conservados en este plan. Esta migración no ejecuta ni sustituye la evidencia histórica.

### TP-WKS-PROBE - Plan de pruebas del Batch B
  - RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

wiki_source_table_exception: true

### [TP-WKS-PROBE-B01] Oracle y presupuesto

- **Marcador estable**: TP-WKS-PROBE-B01
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
- **plan_id**: TP-WKS-PROBE
- **acceptance_oracle**: all-targeted-go-tests-pass-and-filesystem-snapshot-equal
- **mutation_policy**: tests_may_mutate_temp_dirs_only
- **production_paths**: forbidden

### [TP-WKS-PROBE-B02] Identidad y resolución

- **Marcador estable**: TP-WKS-PROBE-B02
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
- **normative**: identity_and_resolution
- **Caso**: TC-WKS-PROBE-001; **Tipo**: negativo; **Cobertura**: alias explícito inexistente; **Resultado esperado**: error `WKS_SELECTOR_NOT_FOUND`, sin fallback a caller_cwd
  - RF-WKS-007 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-002; **Tipo**: negativo; **Cobertura**: alias explícito stale; **Resultado esperado**: error `WKS_SELECTOR_STALE`, sin fallback
  - RF-WKS-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-003; **Tipo**: positivo; **Cobertura**: selector omitido dentro de root registrado; **Resultado esperado**: `resolution_source=caller_cwd` y warning/provenance auditable
  - RF-WKS-007 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-004; **Tipo**: positivo; **Cobertura**: selector path explícito relativo; **Resultado esperado**: resuelve relativo al caller CWD capturado
  - RF-WKS-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-005; **Tipo**: plataforma; **Cobertura**: casing; **Resultado esperado**: Windows/macOS agrupan casing equivalente; Unix conserva roots distintos
  - RF-WKS-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-006; **Tipo**: plataforma; **Cobertura**: symlink/junction; **Resultado esperado**: identidad canónica converge solo cuando el host permite evaluar el enlace
  - RF-WKS-007 — Criterio 1. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-007; **Tipo**: negativo; **Cobertura**: root físico inexistente; **Resultado esperado**: estado stale/unknown explícito, nunca alias alternativo
  - RF-WKS-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-018; **Tipo**: negativo; **Cobertura**: worktree Git anidado bajo parent registrado; **Resultado esperado**: usa `git_top_level`, nunca selecciona parent léxico
  - RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-019; **Tipo**: negativo; **Cobertura**: root Git no registrado; **Resultado esperado**: resolución sintética/path del Git root, sin leer estado del parent
  - RF-WKS-007 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-020; **Tipo**: positivo; **Cobertura**: parent y worktree registrados; **Resultado esperado**: gana el root canónico exacto del worktree
  - RF-WKS-007 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-021; **Tipo**: plataforma; **Cobertura**: `.git` archivo de worktree; **Resultado esperado**: se detecta como root Git válido
  - RF-WKS-007 — Criterio 3. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-022; **Tipo**: positivo; **Cobertura**: `git_common_dir` compartido; **Resultado esperado**: roots y operational paths permanecen distintos
  - RF-WKS-007 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### [TP-WKS-PROBE-B03] Estado híbrido

- **Marcador estable**: TP-WKS-PROBE-B03
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
- **normative**: hybrid_state
- **Caso**: TC-WKS-PROBE-008; **Tipo**: positivo; **Cobertura**: config repo-local; **Resultado esperado**: `portable_config=repo-local`
  - RF-WKS-007 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-009; **Tipo**: positivo; **Cobertura**: estado operativo local; **Resultado esperado**: path resuelto por máquina, sin mover legacy
  - RF-WKS-007 — Criterio 6. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-010; **Tipo**: negativo; **Cobertura**: legacy existente; **Resultado esperado**: `migration_status=legacy_present`, bytes y mtime sin cambios
  - RF-WKS-007 — Criterio 2. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### [TP-WKS-PROBE-B04] Probe no mutante

- **Marcador estable**: TP-WKS-PROBE-B04
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
- **normative**: non_mutating_probe
- **Caso**: TC-WKS-PROBE-011; **Tipo**: positivo; **Cobertura**: sin daemon y sin DB; **Resultado esperado**: status `absent` o `partial`, exit controlado, sin archivos nuevos
  - RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-012; **Tipo**: positivo; **Cobertura**: DB existente; **Resultado esperado**: abre `mode=ro`, `db_mode=ro`, no crea WAL/SHM
  - RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-013; **Tipo**: negativo; **Cobertura**: DB ausente; **Resultado esperado**: no crea `.mi-lsp`, `index.db`, WAL ni SHM
  - RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-014; **Tipo**: negativo; **Cobertura**: home sin registry; **Resultado esperado**: no crea `~/.mi-lsp` ni escribe registry
  - RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-015; **Tipo**: positivo; **Cobertura**: snapshot antes/después; **Resultado esperado**: lista, bytes y mtimes iguales fuera de fixtures temporales
  - RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-016; **Tipo**: positivo; **Cobertura**: salida estructurada; **Resultado esperado**: `side_effects:false`, envelope estable y provenance mínima
  - RF-WKS-007 — Criterio 7. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-017; **Tipo**: negativo; **Cobertura**: telemetría/snapshot/migración; **Resultado esperado**: ningún registro ni artefacto generado
  - RF-WKS-007 — Criterio 5. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-023; **Tipo**: negativo; **Cobertura**: worktree Git y parent con fixtures distintos; **Resultado esperado**: no lee config/DB del parent y snapshots antes/después iguales
  - RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.
- **Caso**: TC-WKS-PROBE-024; **Tipo**: negativo; **Cobertura**: Git top-level no disponible después de detectar contexto Git; **Resultado esperado**: error tipado o raíz sintética, nunca fallback léxico al parent
  - RF-WKS-007 — Criterio 9. Disposición: manual_evidence_pending. Evidencia esperada: el resultado, snapshot y oráculo definidos en este caso; conservar cualquier omisión o estado pendiente. No acredita una ejecución de esta migración.

### [TP-WKS-PROBE-B05] Comandos de verificación

- **Marcador estable**: TP-WKS-PROBE-B05
- **Comprobación prevista**:
  - go run ./cmd/mi-lsp nav wiki validate-source --workspace . --format toon --no-daemon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
- **commands**:
  - **format**: dynamic_touched_go_gofmt_from_git_diff
  - **tests**: go test ./internal/workspace ./internal/store ./internal/service ./internal/cli
  - **snapshot**: before_after_parent_worktree_home_and_temp_fixture
  - **forbidden_paths**:
    - src
    - cmd
    - worker-dotnet
    - deployment
    - real_user_home

La única oracle de formato para todos los Go tocados es la orden dinámica y no mutante declarada en `verify`. El conjunto se obtiene desde Git para incluir cambios staged y unstaged; no se aceptan listas manuales ni invocaciones mutantes del formateador, porque la verificación no debe mutar archivos. Los casos que requieren symlink/junction se marcan skip únicamente cuando el sistema operativo o permisos no los permiten; el motivo queda en el resultado de test.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-WKS-PROBE
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-WKS-007.md
exports:
  - TP-WKS-PROBE
agent_must_read:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-WKS-007.md
  - internal/workspace/
verify:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - unavailable_platform_case_is_reported_as_pass
  - test_oracle_or_acceptance_would_change
evidence:
  - .docs/wiki/06_tests/TP-WKS-PROBE.md
  - .docs/wiki/05_requirements/RF-WKS-007.md
```
