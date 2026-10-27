---
wiki_contract: WIKI-CONTRACT-v1
id: TP-UPD-001
kind: test
status: active
title: Oráculos de actualización portable y reciclado idle
links:
  verifies:
  - RF-UPD-001
  code:
  - path: internal/selfupdate/engine_test.go
    role: test
  - path: internal/selfupdate/source_test.go
    role: test
  - path: internal/selfupdate/policy_test.go
    role: test
  - path: internal/selfupdate/schedule_test.go
    role: test
  - path: internal/selfupdate/transaction_test.go
    role: test
  - path: internal/selfupdate/transaction_more_test.go
    role: test
  - path: internal/selfupdate/transaction_wrapped_test.go
    role: test
  - path: internal/selfupdate/transaction_regression_test.go
    role: test
  - path: internal/cli/update_runtime_test.go
    role: test
  - path: internal/cli/update_hooks_test.go
    role: test
  - path: internal/daemon/update_lifecycle_test.go
    role: test
---

## Alcance

RF-UPD-001 criterios 1–8, sin convertir fuentes de pruebas en afirmación de ejecución.

## Casos

1. Criterio 1: checksum incorrecto, downgrade, piso, semver, origen/redirect y extracción limitada. Disposición: manual_evidence_pending. Pruebas escritas antes de producción; primer RED de APIs ausentes no demuestra ejecución de cada caso.
2. Criterio 2: lock real entre dos procesos, throttle persistido y dos instalaciones con metadata común. Disposición: manual_evidence_pending. RED conductual observado en Windows y Linux arm64 para segunda instalación ausente/stale y programación omitida; A15-review-behavioral-red.log y A15-review-linux-behavioral-red.log, source9c543c0, corrección pendiente.
3. Criterio 3: pin/opt-out explícitos y user-bin no pin. Disposición: manual_evidence_pending. Pruebas fuente, sin green acreditado.
4. Criterio 4: rename de ejecutable propio en uso, rollback/readback, atomicidad, backup alterado/ajeno, limpieza siguiente arranque. Disposición: manual_evidence_pending. RED conductual observado para falsa restauración y archivo inválido en Windows/Linux, child cleanup en Windows y limpieza POSIX con helper vivo en Linux. TOCTOU antes de backup sigue RED de compilación por seam aún ausente.
5. Criterio 5: diez campos/fecha histórica/failed-readable exit0, sin cambios de hash+mtime+size ni llamadas de red. Disposición: manual_evidence_pending. openStateJSON registra RED de compilación, no prueba conductual de lectura limitada aún.
6. Criterio 6: fábrica real del hijo oculto/desconectado y exclusiones. Disposición: manual_evidence_pending. El primer RED CLI desconocía update; no acredita aún Windows HWND ni ausencia de flash.
7. Criterio 7: requests incompletos y escritura de respuesta, HTTP, background, sesiones/contextos y gate de Accept, shutdown/state antes de spawn. Disposición: manual_evidence_pending. Runtime y foundation están en BUILD; no acreditados por helpers aislados.
8. Criterio 8: Windows y VPS Linux reales desde bootstrap identificado hacia destino público. Disposición: manual_evidence_pending. Originales 0.15.6/0.15.4 no contienen updater; sólo preflight VPS readonly completado, ningún E2E aún. Bootstrap real nuevo-source 0.15.6+a15bootstrap no se presenta como binario público original.

## Evidencia

Bitácora externa del encargo A15 y A-evidence/A15-review-red-results.json: fuente9c543c0, compile RED por dos seams ausentes. Overlay sólo omitió esas dos funciones de tests para observar conductas existentes; producción sin modificación. Compilación linux/arm64 y ejecución real WSL registraron los rojos, no son ejecución VPS ni macOS. Rojos iniciales117/118 precedieron producción. FINAL_VERIFY y certificación/release siguen pendientes.

## Restricciones ADR

No hay green global inferido, no se silencian los cinco fallos baseline ni se fabrica un sexto. No se toca daemon compartido con sesiones ajenas; el test real usa perfil/endpoint y procesos propios.
