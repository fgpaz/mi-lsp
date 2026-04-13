# 6. Matriz de pruebas RF

## Cobertura vigente

| RF | FL origen | TP | Casos positivos minimos | Casos negativos minimos | Estado |
|---|---|---|---|---|---|
| RF-WKS-001 | FL-BOOT-01 | TP-WKS | TC-WKS-001, TC-WKS-002, TC-WKS-004, TC-WKS-005 | TC-WKS-003 | ready |
| RF-WKS-002 | FL-BOOT-01 | TP-WKS | TC-WKS-006, TC-WKS-007 | TC-WKS-008 | ready |
| RF-WKS-003 | FL-BOOT-01 | TP-WKS | TC-WKS-009 | TC-WKS-010 | ready |
| RF-WKS-004 | FL-BOOT-01 | TP-WKS | TC-WKS-011, TC-WKS-012 | TC-WKS-013 | ready |
| RF-WKS-005 | FL-BOOT-01 | TP-WKS | TC-WKS-014 | TC-WKS-015 | ready |
| RF-IDX-001 | FL-IDX-01 | TP-IDX | TC-IDX-001, TC-IDX-002, TC-IDX-004, TC-IDX-005 | TC-IDX-003, TC-IDX-006 | ready |
| RF-IDX-002 | FL-IDX-01 | TP-IDX | TC-IDX-007, TC-IDX-008 | TC-IDX-009 | ready |
| RF-IDX-003 | FL-IDX-01 | TP-IDX | TC-IDX-010 | TC-IDX-011 | ready |
| RF-QRY-001 | FL-QRY-01 | TP-QRY | TC-QRY-001, TC-QRY-002, TC-QRY-042, TC-QRY-045 | TC-QRY-003 | ready |
| RF-QRY-002 | FL-QRY-01 | TP-QRY | TC-QRY-004, TC-QRY-005, TC-QRY-007, TC-QRY-008, TC-QRY-012, TC-QRY-013, TC-QRY-035, TC-QRY-036, TC-QRY-037 | TC-QRY-006, TC-QRY-038 | ready |
| RF-QRY-003 | FL-QRY-01 | TP-QRY | TC-QRY-009, TC-QRY-010 | TC-QRY-011 | ready |
| RF-QRY-004 | FL-QRY-01 | TP-QRY | TC-QRY-014, TC-QRY-015 | TC-QRY-016 | ready |
| RF-QRY-005 | FL-QRY-01 | TP-QRY | TC-QRY-017, TC-QRY-018 | TC-QRY-019 | ready |
| RF-QRY-006 | FL-QRY-01 | TP-QRY | TC-QRY-020, TC-QRY-021 | TC-QRY-022 | ready |
| RF-QRY-007 | FL-QRY-01 | TP-QRY | TC-QRY-023, TC-QRY-024, TC-QRY-047 | TC-QRY-025 | ready |
| RF-QRY-008 | FL-QRY-01 | TP-QRY | TC-QRY-026, TC-QRY-027 | TC-QRY-028 | ready |
| RF-QRY-009 | FL-QRY-01 | TP-QRY | TC-QRY-029, TC-QRY-030 | TC-QRY-031 | ready |
| RF-QRY-010 | FL-QRY-01 | TP-QRY | TC-QRY-032, TC-QRY-033, TC-QRY-039, TC-QRY-043, TC-QRY-046 | TC-QRY-034 | ready |
| RF-QRY-011 | FL-QRY-01 | TP-QRY | TC-QRY-040, TC-QRY-044, TC-QRY-045 | TC-QRY-041 | ready |
| RF-QRY-012 | FL-QRY-01 | TP-QRY | TC-QRY-048, TC-QRY-049, TC-QRY-058, TC-QRY-059 | TC-QRY-050 | implemented |
| RF-QRY-013 | FL-QRY-01 | TP-QRY | TC-QRY-051 | TC-QRY-052 | ready |
| RF-QRY-014 | FL-QRY-01 | TP-QRY | TC-QRY-054, TC-QRY-055, TC-QRY-056 | TC-QRY-053 | implemented |
| RF-QRY-015 | FL-QRY-01 | TP-QRY | TC-QRY-057 | - | implemented |
| RF-CS-001 | FL-CS-01 | TP-CS | TC-CS-001, TC-CS-002, TC-CS-004, TC-CS-005 | TC-CS-003 | ready |
| RF-DAE-001 | FL-DAE-01 | TP-DAE | TC-DAE-001, TC-DAE-002 | TC-DAE-003 | ready |
| RF-DAE-002 | FL-DAE-01 | TP-DAE | TC-DAE-004, TC-DAE-005, TC-DAE-007, TC-DAE-008, TC-DAE-009 | TC-DAE-006 | ready |
| RF-DAE-003 | FL-DAE-01 | TP-DAE | TC-DAE-010, TC-DAE-011 | TC-DAE-012 | ready |
| RF-DAE-004 | FL-DAE-01 | TP-DAE | TC-DAE-013, TC-DAE-014 | TC-DAE-015 | ready |

## Regla de mantenimiento

- Ningun RF se considera cerrado si no tiene al menos un caso positivo y uno negativo trazado a un `TP-*`.
- Los smoke tests manuales del repo deben seguir esta matriz aunque todavia no exista automatizacion completa.
