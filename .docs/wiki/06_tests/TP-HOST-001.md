---
wiki_contract: WIKI-CONTRACT-v1
id: TP-HOST-001
kind: test
status: draft
title: Plan de aceptación del host Windows
links:
  verifies: [RF-HOST-001]
---

## Estado y alcance

Plan de [[RF-HOST-001]] y [[TECH-WINDOWS-HOST]]; sólo bindings y escenarios, sin evidencia de ejecución ni PASS. Parent ejecuta una FINAL_VERIFY fresca después de integrar runtime, release y fixtures, con Windows ARM64 nativo y los límites operativos autorizados. Los comandos siguientes son futuros, no ejecutados en BUILDING. Las rutas y disponibilidad de fixtures se confirman tras integración.

## Contrato y transporte

- En Windows, `go test ./internal/winhost` cubre los casos Go integrados; revisar qué costuras se ejercitan realmente y conservar errores propios. No atribuir al test Go medición visual.
- `py scripts/tests/host-wire.py ABS_HOST_EXE ABS_PYTHON_EXE` usa paths absolutos existentes y comprueba bytes binarios, argv vacío/Unicode/comillas/barras, header y JSON, status host y root DWORD 0, 1, 125, 130, 143 y `0xFEDCBA98`. Comprobar además rechazo de resultado ausente, truncado, malformado o inconsistente, errores de escritura/cierre y colisión con el 125 genuino; si falta costura real, declarar brecha y no simularla.
- `py scripts/tests/host-job.py ABS_HOST_EXE ABS_PYTHON_EXE` debe contrastarse contra los casos efectivamente implementados: INT/TERM completos y prefijos, control inválido, timeout, result exclusivo, asignación fallida antes de resume, hijo vivo tras root, wait-all, cancelación, hard-close y supervivencia de dummy propio ajeno. Terminar únicamente por handles propios; observación por PID no autoriza matanza por PID. Falta de caso o fallo es evidencia explícita, no PASS.
- Consumidor Node: verificar `close` y drenaje de streams antes de parsear; rechazar status host no cero y framing inválido; propagar tipo y DWORD sin colisión ni truncamiento. Un test de Node pendiente no equivale a consumidor integrado.

## Ventanas y lifecycle

Inspeccionar PE de host x64 y arm64: subsystem GUI=2; CLI sigue CUI. En ARM64 nativo, observar procesos propios durante carga con WinEvent y EnumWindows, correlacionando PID y generación sólo para observación. `py scripts/tests/host-observer.py ROOT_PID DURATION_SECONDS` es una fuente de observación, no por sí sola un oráculo completo: snapshots pueden perder procesos efímeros. Calibrar lectura contra ventana visible ya existente o fixture oculto; nunca abrir ventana visible de calibración. Sin calibración válida, cobertura temporal y asociación del árbol demostradas, ausencia de eventos no demuestra ausencia de flashes. No abrir ventanas visibles de prueba.

## Release y evidencia

Después de autorización del parent: construir desde fuente limpia, generar recursos PE para CLI y host, revisar versión de archivo/producto, `go version -m` y revisión exacta para ambas arquitecturas; comprobar `--version --format json` en arquitectura ejecutable. Inspeccionar nueve assets, SHA-256 del checksum oficial y coincidencia de cada host con archivo descargado; incluir dos host sin alterar seis workers. Una release parcial exige inspección, no reintento ciego. Antes de publicación no declarar tags, hashes ni smoke público existentes. Registrar resultados sanitizados, plataforma, comandos, fuente y limitaciones; cualquier evidencia no medida queda NOT_MEASURED/BLOCKED según el gate.
