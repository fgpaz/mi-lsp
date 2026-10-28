---
wiki_contract: WIKI-CONTRACT-v1
id: FL-HOST-001
kind: flow
status: draft
title: Ejecución y cancelación por host Windows
---

## Precondiciones

El consumidor selecciona un host de la release oficial y arquitectura correctas con tag público, SHA, fuente y readback verificados; sin esa evidencia permanece BLOCKED. La admisión del consumidor se decide por separado. Crea un archivo de control regular, vacío, privado y estable, en directorio propio; el sibling `.result` no existe. El caller mantiene exclusividad y seguridad del directorio sin cambios ACL por el host.

```mermaid
sequenceDiagram
    participant C as Consumidor
    participant H as mi-lsp-host GUI
    participant J as Job privado
    participant P as Root y descendientes
    C->>H: argv absoluto, control privado y 3 streams
    H->>H: crea result exclusivo
    H->>J: crear y configurar KILL_ON_JOB_CLOSE
    H->>P: CreateProcess suspendido y CREATE_NO_WINDOW
    H->>J: asignar root por handle
    H->>P: reanudar tras asignación
    C->>P: stdin sin framing
    P-->>C: stdout y stderr sin framing
    alt INT o TERM, timeout o error
        C->>H: token en control (cuando corresponde)
        H->>J: terminar y drenar por handle
    else salida normal
        P-->>H: root exit DWORD
        H->>J: esperar activeProcesses=0
    end
    H->>H: cerrar handles y escribir header + JSON, cerrar result
    H-->>C: status de transporte
    C->>C: esperar cierre host y streams; validar registros y propagar resultado tipado
```

Un prefijo de cancelación incompleto queda pendiente. Si job ya está vacío, la observación de vaciado linealiza la finalización; no se transforma root exit en cancelación por un token tardío. Timeout y errores son infraestructura y nunca éxito implícito. Si el host termina sin resultado íntegro o con status no cero, el consumidor falla cerrado. Véase el framing y los límites de [[TECH-WINDOWS-HOST]], requisitos [[RF-HOST-001]] y oráculos [[TP-HOST-001]].
