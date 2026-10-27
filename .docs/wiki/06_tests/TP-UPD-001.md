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
  - path: internal/daemon/update_foundation_test.go
    role: test
  - path: internal/daemon/update_controller_test.go
    role: test
  - path: internal/daemon/update_listener_test.go
    role: test
  - path: internal/daemon/update_hardening_test.go
    role: test
  - path: internal/daemon/update_hardening_lifecycle_test.go
    role: test
  - path: internal/daemon/update_watcher_test.go
    role: test
  - path: internal/daemon/update_admin_last_test.go
    role: test
  - path: internal/daemon/update_serve_join_test.go
    role: test
  - path: internal/indexer/update_jobs_test.go
    role: test
  - path: internal/service/update_session_test.go
    role: test
  - path: internal/service/update_detached_test.go
    role: test
  - path: internal/selfupdate/deferred_test.go
    role: test
  - path: internal/selfupdate/distribution_test.go
    role: test
  - path: internal/selfupdate/download_budget_test.go
    role: test
---

## Alcance

RF-UPD-001 criterios 1–8, sin convertir fuentes de pruebas en afirmación de ejecución.

## Casos

1. Criterio 1: checksum incorrecto, downgrade, piso, semver, origen/redirect y extracción limitada. Disposición: manual_evidence_pending. Pruebas escritas antes de producción; primer RED de APIs ausentes no demuestra ejecución de cada caso.
2. Criterio 2: lock real entre dos procesos, throttle persistido y dos instalaciones con metadata común. Disposición: manual_evidence_pending. RED conductual observado en Windows y Linux arm64 para segunda instalación ausente/stale y programación omitida; A15-review-behavioral-red.log y A15-review-linux-behavioral-red.log, source9c543c0; correcciones integradas, green pendiente.
3. Criterio 3: pin/opt-out explícitos y user-bin no pin. Disposición: manual_evidence_pending. Pruebas fuente, sin green acreditado.
4. Criterio 4: rename de ejecutable propio en uso, rollback/readback, atomicidad, backup alterado/ajeno, limpieza siguiente arranque. Disposición: manual_evidence_pending. RED conductual observado para falsa restauración y archivo inválido en Windows/Linux, child cleanup en Windows y limpieza POSIX con helper vivo en Linux. TOCTOU antes de backup tuvo RED de compilación; seam y fix ya integrados, ejecución final pendiente.
5. Criterio 5: diez campos/fecha histórica/failed-readable exit0, sin cambios de hash+mtime+size ni llamadas de red. Disposición: manual_evidence_pending. openStateJSON tuvo RED de compilación; implementación acotada integrada, ejecución final pendiente.
6. Criterio 6: fábrica real del hijo oculto/desconectado y exclusiones. Disposición: manual_evidence_pending. El primer RED CLI desconocía update; no acredita aún Windows HWND ni ausencia de flash.
7. Criterio 7: requests incompletos y escritura de respuesta, HTTP, background, sesiones/contextos y gate de Accept, shutdown/state antes de spawn. Disposición: manual_evidence_pending. RED124 conductual demuestra retornos prematuros de Stop y cierre HTTP irreversible antes de commit. Fixes integrados, sin green todavía; la unión de Serve con el spawn comprometido sigue en BUILD. Helpers aislados no acreditan E2E.
8. Criterio 8: Windows y VPS Linux reales desde bootstrap identificado hacia destino público. Disposición: manual_evidence_pending. Originales 0.15.6/0.15.4 no contienen updater; sólo preflight VPS readonly completado, ningún E2E aún. Bootstrap real nuevo-source 0.15.6+a15bootstrap no se presenta como binario público original.

## FINAL_VERIFY observada (base de código ab3e6cb)

- Suite Windows completa: A15-suite-comparison.json, exactamente cinco fallos baseline reproducidos y cero nuevos; **no green global**. El primer fallo adicional de snapshot se conservó en A15-suite-first-comparison.json y no se aceptó como baseline. El fix exclusivo de fixture usa metadata fresca por handle después de leer hashes; 30 repeticiones de lock/check pasan sin quitar comprobaciones de mode, size, mtime o hash.
- Windows real: TestDetachedIndexJobSurvivesParentExit pasó tras salida del padre; la suite cubre los diez campos, límites, checksum, rollback, caché/política, fábrica real y barreras de admisión/joins. No es evidencia nativa macOS.
- Linux ARM64 real WSL: A15-native-linux-results.json, paquetes selfupdate/daemon/service ejecutados y PASS. El intento anterior falló antes de ejecutar tests por quoting del driver y quedó preservado. **No es VPS Lenovo**.
- Controller Windows real local: fixture a15-owned-t0pjcmti; PID67412 respondió el RPC completo después de promoción en disco manteniendo una trama parcial abierta. Check conservó hash/mtime/size del estado; luego salió el viejo y PID63312 adoptó 0.16.0 conservando opciones. Promoción/recibo fueron controlados por fixture local: **NO acredita descarga automática pública**. Bootstrap 0.15.6+a15bootstrap es fuente nueva ab3e6cb, no binario antiguo publicado. El primer intento se abortó por CRLF del helper, sin PASS ficticio ni corte de procesos ajenos.
- VPS Lenovo Linux x64 REAL, fixture /tmp/a15-owned-pdg4_5y9/source24fdfba (sólo docs sobre Goab3): PID367153 conservó trama parcial durante promoción controlada y respondió OK antes de salir; PID377904 adoptó16/source/hash/opciones. A15-vps-local-final-readback.json confirma salida del nuevo y stateausente tras stop sólo propio; los siete PIDs preexistentes conservaron su generación. El comando finish no produjo recibo por carrera de transporte; no se ocultó y el cierre se acreditó por readback independiente. **No es descarga pública automática**; el bootstrap es fuente nueva identificada, no release original15.4/15.6.
- Corpus reales readonly botsd/Desktop/Traditore: A15-*-readonly-proof.json acredita hash/mtime/size intactos y perfil sin escrituras; resultados conservan ADVISORY/links reales, no PASS inventado. SQLite original nunca abierto, sólo copias privadas con WAL.

Criterios1–7 conservan los oráculos anteriores y ahora estas evidencias automatizadas; la demostración de fuente pública y readbacks finales del criterio8 sigue **manual_evidence_pending**, autorizada como comprobación posterior a publicación. Esta sección no adelanta ese cierre ni reemplaza el audit independiente.

## Evidencia histórica de RED (no repetir)

Bitácora externa del encargo A15 y A-evidence/A15-review-red-results.json: fuente9c543c0, compile RED por dos seams ausentes. Overlay sólo omitió esas dos funciones de tests para observar conductas existentes; producción sin modificación. Compilación linux/arm64 y ejecución real WSL registraron los rojos, no son ejecución VPS ni macOS. Rojos iniciales117/118 precedieron producción. FINAL_VERIFY y certificación/release siguen pendientes.

RED123 conductual: archivo fixture de escala oficial rechazado y deadline de asset igual a metadata (30 s). Se inspeccionó el ZIP público real de 0.15.6: 708896784 bytes expandidos, 288114357 comprimidos, 1973 entradas. Fix de límites y plazo integrado después del rojo; no se afirma descarga pública exitosa aún.

## Restricciones ADR

No hay green global inferido, no se silencian los cinco fallos baseline ni se fabrica un sexto. No se toca daemon compartido con sesiones ajenas; el test real usa perfil/endpoint y procesos propios.
