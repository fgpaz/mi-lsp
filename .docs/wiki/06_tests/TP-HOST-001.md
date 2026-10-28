---
wiki_contract: WIKI-CONTRACT-v1
id: TP-HOST-001
kind: test
status: draft
title: Plan de aceptación del host Windows
links:
  verifies: [RF-HOST-001]
---

## Alcance y evidencia

Verifica [[RF-HOST-001]] y [[TECH-WINDOWS-HOST]]. Las fuentes de pruebas son bindings, no evidencia de ejecución ni de conformidad. La aceptación requiere Windows ARM64 nativo, artefactos verificables y evidencia sanitizada de cada oráculo; conservar fallos y limitaciones, no convertir ausencia de evidencia en PASS. La admisión de un consumidor es una decisión separada.

## Driver de aceptación y PE

`scripts/tests/host-acceptance.py` recibe **cuatro** paths absolutos existentes; la firma real incluye ambos assets para el precheck, además del host nativo y Python:

```text
py scripts/tests/host-acceptance.py ABS_HOST ABS_PYTHON ABS_X64_HOST ABS_ARM64_HOST
```

El driver comprueba cabeceras DOS y PE, PE32+, machine x64 `0x8664` y arm64 `0xaa64`, y subsystem GUI 2 antes de lanzar fixtures. Comprueba además que el host ejecutado tenga una de esas arquitecturas y subsystem GUI. La prueba no demuestra por sí sola que ese host sea el asset público correcto ni que la CLI sea CUI: correlacionar SHA y procedencia y comprobar el PE de CLI por separado. El parser `scripts/tests/host-result.py` se carga y ejercita para transporte no cero, truncamiento, cabeceras/JSON inválidos, inconsistencia, registros extra y root 125 válido; no es un consumidor Node integrado.

Durante la observación el driver ejecuta `scripts/tests/host-wire.py` y `scripts/tests/host-job.py` con `ABS_HOST ABS_PYTHON`. El primero cubre bytes crudos, argv vacío/Unicode/comillas/barras y root DWORD 0, 1, 125, 130, 143 y `0xFEDCBA98`. El segundo ejercita el ciclo de vida y control; examinar sus casos efectivos y resultados, incluyendo wait-all, cancelación, hard-close y procesos propios ajenos al job. La terminación usa handles propios, no PID. Una falla o caso no cubierto no es PASS.

## Observador global conservador

`scripts/tests/host-observer.py` se importa desde el driver, no se invoca con PID. Instala hook global CREATE..SHOW y compara snapshots de ventanas visibles. Requiere una ventana **ya visible** para calibrar enumeración y crea una ventana STATIC **oculta** para calibrar entrega de CREATE; no crea ventana visible ni genera un SHOW positivo sintético. Esa calibración oculta no demuestra positivamente la entrega de SHOW. El hook y snapshots permanecen activos durante fixtures y una barrera final de drenaje.

Todo evento SHOW global o ventana visible nueva impide PASS, incluso si pertenece a otro proceso; el resultado es conservador y no atribuye causalidad por genealogía. Sin ventana visible preexistente, CREATE oculto observado o cierre correcto, el resultado es inconcluso/fallido. Registrar la limitación de calibración SHOW y la cobertura real; flags o subsystem aislados no demuestran ausencia de flashes. No abrir ventana visible de calibración.

## Costuras Go y consumidor

`go test ./internal/winhost` en Windows incluye `internal/winhost/host_windows_test.go` para framing y fallas write/short/close/layouts y `internal/winhost/assignment_windows_test.go`: el seam `assignOwnedProcess` simula fallo de asignación y comprueba que el root suspendido no ejecute el marcador, el resultado tipado de infraestructura y cleanup. No usar esa prueba como medición visual. El consumidor debe probar por separado espera de `close` y streams, validación de ambos registros, tipo y DWORD, sin truncamiento; las fixtures del productor no demuestran integración ni admisión de Node.

## Provenance y release

Comprobar fuente VCS limpia y revisión, recursos PE y versión de archivo/producto, `go version -m`, y `--version --format json` en arquitectura ejecutable. Para aceptación de distribución, verificar tag público, source, nueve assets (seis CLI/worker, dos host, checksum), inclusión y SHA-256 de ambos host y readback de los assets descargados. Una release parcial requiere inspección, no reintento ciego. Registrar plataforma y evidencias; cada gate sin evidencia queda BLOCKED o NOT_MEASURED según corresponda.
