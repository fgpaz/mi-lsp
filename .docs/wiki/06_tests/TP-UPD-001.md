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
  - path: internal/daemon/update_owner_cache_test.go
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

### Regresión acotada del dueño de actualización

- **Caso**: TC-UPD-CACHE-001; **RF**: RF-UPD-001 — Criterio 2; Alcance parcial: selección y caché de dueño; **disposición**: manual_evidence_pending.
- **Evidencia prevista**: `internal/daemon/update_owner_cache_test.go`: `TestUpdateOwnerCacheConcurrencyAndInvalidation` comprueba concurrencia e invalidación de caché; `TestUpdateOwnerCacheRejectsDeadCachedPID` no reutiliza sin reprobar un PID cacheado considerado muerto; `TestUpdateOwnerLockCancellation` comprueba cancelación del lock. Corresponden solo a la faceta de selección del dueño y caché del cambio en `internal/daemon/update.go`; no prueban throttle de metadata por 24 horas, reinicio ni aplicación por dos instalaciones. No reemplazan la prueba de lock entre procesos del caso 2 ni acreditan una ejecución nueva.

1. Criterio 1: checksum incorrecto, downgrade, piso, semver, origen/redirect y extracción limitada. Disposición: verified_automated. Suite ab3e6cb y ejecución nativa WSL ARM64; las descargas públicas 0.16.0 Windows ARM64 y Linux x64 verificaron el SHA antes de promocionar. No convierte compilación macOS en prueba nativa.
2. Criterio 2: lock real entre dos procesos, throttle persistido y dos instalaciones con metadata común. Disposición: verified_automated. Suite final Windows y WSL ARM64; 30 repeticiones del snapshot lock/check pasan con stat fresco por handle, sin suprimir mtime/hash/size/mode. El plazo 24 h se verifica con reloj de prueba, no con una espera real de 24 h.
3. Criterio 3: pin/opt-out explícitos y user-bin no pin. Disposición: verified_automated. Pruebas de política/estado pasan en la suite final; las instalaciones personales no introdujeron pin ni cambiaron políticas globales.
4. Criterio 4: rename de ejecutable propio en uso, rollback/readback, atomicidad, backup alterado/ajeno y limpieza acotada. Disposición: verified_automated_and_public. Rollback/readback fallido y TOCTOU cubiertos por tests; el E2E público Windows preservó al proceso viejo con RPC parcial mientras cambió el binario. No se afirma haber inducido rollback contra un contexto compartido real.
5. Criterio 5: diez campos/fecha histórica/failed-readable exit0, sin cambios de hash+mtime+size ni llamadas de red en check. Disposición: verified_automated_and_public. Ambos E2E públicos conservaron el snapshot durante --check. auto=false en esas lecturas corresponde al opt-out del proceso de sondeo; el trigger anterior habilitó la ruta automática real.
6. Criterio 6: fábrica real del hijo oculto/desconectado y exclusiones. Disposición: verified_factory_and_native_detach. Flags Windows de la fábrica y exclusiones pasan; TestDetachedIndexJobSurvivesParentExit ejecutó un hijo real después de salir su padre. Actualización pública ejecutada en segundo plano y sin admin; no se hizo una medición visual separada de HWND/flash.
7. Criterio 7: requests incompletos, respuesta/HTTP, jobs, sesiones/contextos, gate Accept, watcher y shutdown/state antes de spawn. Disposición: verified_automated_and_public. Suite final acredita barreras y Serve join tras RED124/125. En Windows/VPS públicos el viejo contestó completamente antes de salir; el nuevo adoptó hash, versión, source y opciones. Helpers locales no sustituyen estas pruebas.
8. Criterio 8: Windows y VPS Linux reales desde bootstrap identificado hacia destino público. Disposición: verified_public. Release v0.16.0/source a01835b y siete assets/readbacks verificadas; ambos runtimes descargaron la release GitHub real y se reciclaron sin cortar la RPC. Bootstrap 0.15.6+a15bootstrap fue compilado de fuente nueva identificada; originales 0.15.6/0.15.4 carecen de updater y requieren primera adopción manual.

## FINAL_VERIFY observada (base de código ab3e6cb)

- Suite Windows completa: A15-suite-comparison.json, exactamente cinco fallos baseline reproducidos y cero nuevos; **no green global**. El primer fallo adicional de snapshot se conservó en A15-suite-first-comparison.json y no se aceptó como baseline. El fix exclusivo de fixture usa metadata fresca por handle después de leer hashes; 30 repeticiones de lock/check pasan sin quitar comprobaciones de mode, size, mtime o hash.
- Windows real: TestDetachedIndexJobSurvivesParentExit pasó tras salida del padre; la suite cubre los diez campos, límites, checksum, rollback, caché/política, fábrica real y barreras de admisión/joins. No es evidencia nativa macOS.
- Linux ARM64 real WSL: A15-native-linux-results.json, paquetes selfupdate/daemon/service ejecutados y PASS. El intento anterior falló antes de ejecutar tests por quoting del driver y quedó preservado. **No es VPS Lenovo**.
- Controller Windows real local: fixture a15-owned-t0pjcmti; PID67412 respondió el RPC completo después de promoción en disco manteniendo una trama parcial abierta. Check conservó hash/mtime/size del estado; luego salió el viejo y PID63312 adoptó 0.16.0 conservando opciones. Promoción/recibo fueron controlados por fixture local: **NO acredita descarga automática pública**. Bootstrap 0.15.6+a15bootstrap es fuente nueva ab3e6cb, no binario antiguo publicado. El primer intento se abortó por CRLF del helper, sin PASS ficticio ni corte de procesos ajenos.
- VPS Lenovo Linux x64 REAL, fixture /tmp/a15-owned-pdg4_5y9/source24fdfba (sólo docs sobre Goab3): PID367153 conservó trama parcial durante promoción controlada y respondió OK antes de salir; PID377904 adoptó16/source/hash/opciones. A15-vps-local-final-readback.json confirma salida del nuevo y stateausente tras stop sólo propio; los siete PIDs preexistentes conservaron su generación. El comando finish no produjo recibo por carrera de transporte; no se ocultó y el cierre se acreditó por readback independiente. **No es descarga pública automática**; el bootstrap es fuente nueva identificada, no release original15.4/15.6.
- Corpus reales readonly botsd/Desktop/Traditore: A15-*-readonly-proof.json acredita hash/mtime/size intactos y perfil sin escrituras; resultados conservan ADVISORY/links reales, no PASS inventado. SQLite original nunca abierto, sólo copias privadas con WAL.

## Comprobación pública posterior a publicación — 2026-10-28

- Release inmutable **v0.16.0**, source **a01835b69b7298ee7c70c0afa183ed8df11adee4**: A15-release-verification.json acredita latest, tag anotado/peeled, siete SHA/tamaños y texto checksums descargado. A15-ae-release-certificate.json contiene seis procedencias limpias; ambos PE son 0.16.0.0. A15-vulnerability-audit.json no lista paquetes NuGet vulnerables.
- Windows público real: A15-windows-public-final-readback.json, fixture a15-owned-nci9vy3m. ViejoPID65872 respondió la RPC parcial completada después de la promoción automática; nuevoPID37136 adoptó0.16/source/hash/opciones. Check puro conservó estado; ambos procesos propios salieron y state quedó ausente tras stop sólo propio.
- VPS Linux x64 público real: A15-vps-public-final-readback.json, fixture /tmp/a15-owned-zr5we8ef. ViejoPID762271 contestó íntegro antes de salir; nuevoPID793628 adoptó0.16/source/hash/opciones. Ambos salieron y state quedó ausente; siete procesos preexistentes conservaron generación. Sin recibos sintéticos, servidor mock ni corte de sesiones ajenas.
- Instalación personal manual segura: A15-windows-installed-proof.json, A15-wsl-dotlocal-proof.json, A15-wsl-bin-proof.json y A15-vps-installed-proof.json. Windows y dos bins WSL pasaron de0.15.6 a0.16; user-bin VPS pasó de0.15.4 a0.16. CLI/hash/source y protocolo self-contained nativo verificados en Windows ARM64, WSL ARM64 y VPS x64. En Linux se usaron paquetes públicos comprobados y un HOME de fixture antes de promoción real; no se invocó el instalador que detiene daemons.
- Agregado: A-evidence/A15-delivery-results.json. Instalación en disco **no** acredita adopción de un daemon legacy compartido: sus contextos continúan hasta un drain natural seguro. macOS permanece compilado/certificado en metadata, no ejecutado nativamente. La suite conserva cinco baseline y cero nuevos, no green global.
- Estas evidencias completan la comprobación posterior autorizada del criterio8. El tag conserva el source de release; esta ficha registra el readback posterior sin mover ni rehacer la release y sin repetir Go C2 por documentación. La conformidad AE final consume el packet actualizado, separada de la cobertura.

## Evidencia histórica de RED (no repetir)

Bitácora externa del encargo A15 y A-evidence/A15-review-red-results.json: fuente9c543c0, compile RED por dos seams ausentes. Overlay sólo omitió esas dos funciones de tests para observar conductas existentes; producción sin modificación. Compilación linux/arm64 y ejecución real WSL registraron los rojos, no son ejecución VPS ni macOS. Rojos iniciales117/118 precedieron producción. Ese registro describe el instante del RED; FINAL_VERIFY y certificación/release posteriores constan arriba.

RED123 conductual: archivo fixture de escala oficial rechazado y deadline de asset igual a metadata (30 s). Se inspeccionó el ZIP público real de 0.15.6: 708896784 bytes expandidos, 288114357 comprimidos, 1973 entradas. Fix de límites y plazo integrado después del rojo; la descarga pública posterior de0.16 se acredita arriba, no se atribuye a este RED.

## Restricciones ADR

No hay green global inferido, no se silencian los cinco fallos baseline ni se fabrica un sexto. No se toca daemon compartido con sesiones ajenas; el test real usa perfil/endpoint y procesos propios.

```yaml
harness_protocol: SDD-HARNESS-v1
id: TP-UPD-001
kind: test-plan
audience: llm-first
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-UPD-001.md
  - .docs/wiki/04_flows/FL-UPD-001.md
exports:
  - TP-UPD-001
agent_must_read:
  - .docs/wiki/06_tests/TP-UPD-001.md
agent_may_edit:
  - .docs/wiki/06_tests/TP-UPD-001.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-UPD-001.md
  - internal/daemon/
verify:
  - .docs/wiki/06_tests/TP-UPD-001.md#Casos
stop_if:
  - harness_verdict=BLOCKED
  - unrelated_daemon_or_session_is_touched
  - baseline_failures_are_hidden_or_relabelled
evidence:
  - .docs/wiki/06_tests/TP-UPD-001.md
  - .docs/wiki/05_requirements/RF-UPD-001.md
```
