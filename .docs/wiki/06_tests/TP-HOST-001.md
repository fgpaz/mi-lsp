---
wiki_contract: WIKI-CONTRACT-v1
id: TP-HOST-001
kind: test-plan
status: draft
title: Aceptación del host Windows
---

Plan pendiente; ninguna ejecución ni PASS nativo se infiere de este documento. El coordinador ejecuta tras integrar runtime, release y fixtures.

- Windows ARM64 nativo; inspeccionar ambos PE x64/arm64, subsystem 2 y metadata VCS/provenance, con assets de la misma release.
- `scripts/tests/host-wire.py` usa rutas absolutas de host y Python: bytes binarios, argumentos vacíos, Unicode, comillas y barras; códigos root 0, 1, 125, 130, 143 y DWORD completo. Es sólo una fracción de la aceptación.
- Agregar fixtures ejecutables para control INT/TERM parcial y protocolo inválido, resultado exclusivo y truncado, colisiones writer/Close con 125 si existe seam real. Ante seam ausente documentar brecha; no simular cobertura.
- Demostrar que fallo de asignación no permite ejecutar el hijo suspendido; demostrar descendiente vivo tras salida root y que el resultado espera el vaciado. Cancelación drena el job; matar el host por handle verifica kill-on-close. Un dummy propio ajeno al job permanece vivo. No cancelar por PID.
- Implementar observador WinEvent más EnumWindows con loop acotado durante carga del árbol propio, correlacionando PID con generación y handles. Calibrar por lectura de ventana ya visible o fixture oculto sin ShowWindow; nunca abrir ventana visible para calibrar. Sin calibración válida y observación real de eventos/visibilidad no declarar ausencia de flashes ni PASS. Flags y subsystem no son ese oráculo.
