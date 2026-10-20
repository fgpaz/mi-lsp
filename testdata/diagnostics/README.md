# Fixtures de diagnósticos

Fuentes intencionalmente erróneas para el smoke real de [[TP-HARNESS-DIAG-001]]. No son paquetes del producto y no se compilan durante BUILDING. Copiar cada subcarpeta a un workspace de QA aislado, consultar su archivo con el backend correspondiente, corregirlo en la copia y volver a consultar para detectar buffers stale. No reemplazar un backend ausente con grep, build o tests.

`schema.json` describe el resultado estructurado `diagnostics/v1`. El test Rust prueba diagnósticos nativos de sintaxis, no cobertura de rustc; push sin barrera de finalización conserva `partial`.
