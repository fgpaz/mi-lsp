# 6. Matriz de pruebas RF

```yaml
harness_protocol: SDD-HARNESS-v1
id: "06_matriz_pruebas_RF"
kind: "support-doc"
audience: "dual"
imports:
  - '[[00_gobierno_documental]]'
  - '.docs/wiki/06_matriz_pruebas_RF.md'
exports:
  - '06_matriz_pruebas_RF'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/06_matriz_pruebas_RF.md
agent_may_edit:
  - .docs/wiki/06_matriz_pruebas_RF.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/06_matriz_pruebas_RF.md
```

## Cobertura vigente

| RF | FL origen | TP | Casos positivos minimos | Casos negativos minimos | Estado |
|---|---|---|---|---|---|
| RF-WKS-001 | FL-BOOT-01 | TP-WKS | TC-WKS-001, TC-WKS-002, TC-WKS-004, TC-WKS-005, TC-WKS-026 | TC-WKS-003 | ready |
| RF-WKS-002 | FL-BOOT-01 | TP-WKS | TC-WKS-006, TC-WKS-007, TC-WKS-017, TC-WKS-018, TC-WKS-019, TC-WKS-026 | TC-WKS-008 | implemented |
| RF-WKS-003 | FL-BOOT-01 | TP-WKS | TC-WKS-009 | TC-WKS-010 | ready |
| RF-WKS-004 | FL-BOOT-01 | TP-WKS | TC-WKS-011, TC-WKS-012, TC-WKS-016, TC-WKS-017, TC-WKS-018, TC-WKS-021, TC-WKS-022, TC-WKS-031, TC-WKS-032, TC-WKS-034 | TC-WKS-013 | implemented |
| RF-WKS-005 | FL-BOOT-01 | TP-WKS | TC-WKS-014, TC-WKS-019, TC-WKS-020 | TC-WKS-015 | implemented |
| RF-WKS-006 | FL-BOOT-01 | TP-WKS | TC-WKS-027, TC-WKS-028 | - | implemented |
| RF-IDX-001 | FL-IDX-01 | TP-IDX | TC-IDX-001, TC-IDX-002, TC-IDX-004, TC-IDX-005, TC-IDX-012, TC-IDX-020, TC-IDX-022, TC-IDX-023, TC-IDX-025 | TC-IDX-003, TC-IDX-006, TC-IDX-021 | ready |
| RF-IDX-002 | FL-IDX-01 | TP-IDX | TC-IDX-007, TC-IDX-008, TC-IDX-013 | TC-IDX-009 | ready |
| RF-IDX-003 | FL-IDX-01 | TP-IDX | TC-IDX-010 | TC-IDX-011 | ready |
| RF-QRY-001 | FL-QRY-01 | TP-QRY | TC-QRY-001, TC-QRY-002, TC-QRY-042, TC-QRY-045, TC-QRY-106, TC-QRY-108, TC-QRY-109 | TC-QRY-003 | ready |
| RF-QRY-002 | FL-QRY-01 | TP-QRY | TC-QRY-004, TC-QRY-005, TC-QRY-007, TC-QRY-008, TC-QRY-012, TC-QRY-013, TC-QRY-035, TC-QRY-036, TC-QRY-037, TC-QRY-047A, TC-QRY-108 | TC-QRY-006, TC-QRY-038 | ready |
| RF-QRY-003 | FL-QRY-01 | TP-QRY | TC-QRY-009, TC-QRY-010 | TC-QRY-011 | ready |
| RF-QRY-004 | FL-QRY-01 | TP-QRY | TC-QRY-014, TC-QRY-015 | TC-QRY-016 | ready |
| RF-QRY-005 | FL-QRY-01 | TP-QRY | TC-QRY-017, TC-QRY-018 | TC-QRY-019 | ready |
| RF-QRY-006 | FL-QRY-01 | TP-QRY | TC-QRY-020, TC-QRY-021 | TC-QRY-022 | ready |
| RF-QRY-007 | FL-QRY-01 | TP-QRY | TC-QRY-023, TC-QRY-024, TC-QRY-047, TC-QRY-047A | TC-QRY-025 | ready |
| RF-QRY-008 | FL-QRY-01 | TP-QRY | TC-QRY-026, TC-QRY-027 | TC-QRY-028 | ready |
| RF-QRY-009 | FL-QRY-01 | TP-QRY | TC-QRY-029, TC-QRY-030 | TC-QRY-031 | ready |
| RF-QRY-010 | FL-QRY-01 | TP-QRY | TC-QRY-032, TC-QRY-033, TC-QRY-039, TC-QRY-043, TC-QRY-046, TC-QRY-107 | TC-QRY-034 | ready |
| RF-QRY-011 | FL-QRY-01 | TP-QRY | TC-QRY-040, TC-QRY-044, TC-QRY-045 | TC-QRY-041 | ready |
| RF-QRY-012 | FL-QRY-01 | TP-QRY | TC-QRY-048, TC-QRY-049, TC-QRY-058, TC-QRY-059 | TC-QRY-050 | implemented |
| RF-QRY-013 | FL-QRY-01 | TP-QRY | TC-QRY-051 | TC-QRY-052 | ready |
| RF-QRY-014 | FL-QRY-01 | TP-QRY | TC-QRY-054, TC-QRY-055, TC-QRY-056, TC-QRY-060, TC-QRY-061, TC-QRY-062 | TC-QRY-053 | implemented |
| RF-QRY-015 | FL-QRY-01 | TP-QRY | TC-QRY-057 | - | implemented |
| RF-QRY-016 | FL-QRY-01 | TP-QRY | TC-QRY-073, TC-QRY-076, TC-QRY-077, TC-QRY-078, TC-QRY-079, TC-QRY-082, TC-QRY-083, TC-QRY-084, TC-QRY-085, TC-QRY-090, TC-QRY-091, TC-QRY-095, TC-QRY-096, TC-QRY-097, TC-QRY-099, TC-QRY-100, TC-QRY-101, TC-QRY-102, TC-QRY-103 | TC-QRY-074, TC-QRY-075, TC-QRY-080, TC-QRY-081, TC-QRY-086, TC-QRY-087, TC-QRY-088, TC-QRY-089, TC-QRY-092, TC-QRY-093, TC-QRY-094, TC-QRY-098 | implemented |
| RF-QRY-017 | FL-QRY-01 | TP-QRY | TC-QRY-110, TC-QRY-111, TC-QRY-112, TC-QRY-113, TC-QRY-116 | TC-QRY-114, TC-QRY-115 | implemented |
| RF-QRY-018 | FL-QRY-01 | TP-QRY | TC-QRY-118, TC-QRY-119, TC-QRY-120, TC-QRY-126, TC-QRY-127, TC-QRY-128, TC-QRY-133, TC-QRY-134 | TC-QRY-121, TC-QRY-122, TC-QRY-123, TC-QRY-124, TC-QRY-125, TC-QRY-129, TC-QRY-130, TC-QRY-131, TC-QRY-132 | implemented |
| RF-CS-001 | FL-CS-01 | TP-CS | TC-CS-001, TC-CS-002, TC-CS-004, TC-CS-005 | TC-CS-003 | ready |
| RF-DAE-001 | FL-DAE-01 | TP-DAE | TC-DAE-001, TC-DAE-002 | TC-DAE-003 | ready |
| RF-DAE-002 | FL-DAE-01 | TP-DAE | TC-DAE-004, TC-DAE-005, TC-DAE-007, TC-DAE-008, TC-DAE-009, TC-DAE-018, TC-DAE-020, TC-DAE-026 | TC-DAE-006, TC-DAE-019 | ready |
| RF-DAE-003 | FL-DAE-01 | TP-DAE | TC-DAE-010, TC-DAE-011 | TC-DAE-012 | ready |
| RF-DAE-004 | FL-DAE-01 | TP-DAE | TC-DAE-013, TC-DAE-014, TC-DAE-017, TC-DAE-018 | TC-DAE-015 | ready |
| RF-WIKI-001 | FL-WIKI-01 | TP-WIKI | TC-WIKI-001, TC-WIKI-002 | TC-WIKI-004 | ready |
| RF-WIKI-002 | FL-WIKI-01 | TP-WIKI | TC-WIKI-005, TC-WIKI-006, TC-WIKI-007, TC-WIKI-008 | - | ready |
| RF-WIKI-003 | FL-WIKI-01 | TP-WIKI | TC-WIKI-009, TC-WIKI-011 | TC-WIKI-010, TC-WIKI-012 | ready |
| RF-WIKI-004 | FL-WIKI-01 | TP-WIKI | TC-WIKI-013, TC-WIKI-014, TC-WIKI-015, TC-WIKI-016 | - | ready |
| RF-WIKI-005 | FL-WIKI-01 | TP-WIKI | TC-WIKI-017, TC-WIKI-019, TC-WIKI-020 | TC-WIKI-018 | ready |

## Regla de mantenimiento

- Ningun RF se considera cerrado si no tiene al menos un caso positivo y uno negativo trazado a un `TP-*`.
- Los smoke tests manuales del repo deben seguir esta matriz aunque todavia no exista automatizacion completa.
