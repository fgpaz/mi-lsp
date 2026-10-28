---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-WINDOWS-HOST
kind: technical
status: draft
title: Host Windows sin ventana
---

Diseño pendiente de integración y verificación: `mi-lsp-host.exe` implementa `execsafe-host/v1`; no es una capacidad publicada ni una afirmación de conformidad nativa. El consumidor usa rutas absolutas y `--control-file ABS -- ABS_EXE [args...]`, sin shell ni búsqueda en PATH. El control es un archivo privado existente y vacío; el resultado hermano se crea exclusivamente. Los tres canales estándar transportan bytes sin framing; control y resultado no se heredan.

El resultado contiene cabecera ASCII `execsafe-host/v1 KIND DWORD\n` y JSON compacto terminado en LF. El proceso host devuelve cero sólo cuando el resultado se escribió y cerró correctamente: no significa que el payload haya devuelto cero. El consumidor rechaza host no cero o resultado incompleto y conserva el DWORD y su tipo, incluso 125, 130 y 143. INT y TERM por control admiten prefijos parciales pendientes.

La política prevista configura KILL_ON_JOB_CLOSE antes de crear el hijo suspendido y sin consola, lo asigna al job antes de reanudarlo y espera el vaciado del árbol entero, no sólo el fin del root. Cancelación y timeout terminan y drenan el job por handle. No se mata por PID ni se hereda el job. Esto es incompatible con la conservación de daemons de la política NormalDaemon del candidato B; compartir wire no implica compartir esa semántica. No garantiza ausencia de GUI creada deliberadamente por un payload malicioso.

Los dos assets Windows previstos pertenecen a la misma release oficial de mi-lsp; tag, SHA, provenance y nombres definitivos siguen pendientes. No hay fallback a CUI ni repo nuevo. La medición nativa de ventanas y el readback de release son gates pendientes, no evidencia ya obtenida.
