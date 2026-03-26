# TP-CS

## Cobertura objetivo

- RF-CS-001

## Casos

| Caso | Tipo | RF | Descripcion |
|---|---|---|---|
| TC-CS-001 | positivo | RF-CS-001 | resuelve consulta C# con `backend=roslyn` |
| TC-CS-002 | positivo | RF-CS-001 | reutiliza runtime caliente o worker ya disponible |
| TC-CS-003 | negativo | RF-CS-001 | falla con error accionable cuando el worker no esta disponible |
| TC-CS-004 | positivo | RF-CS-001 | `nav context` devuelve `slice_text` y metadatos semanticos en la misma respuesta |
| TC-CS-005 | positivo | RF-CS-001 | si Roslyn no puede enriquecer pero el archivo existe, el core devuelve el slice con warning accionable |
| TC-CS-006 | positivo | RF-CS-001 | si el primer candidato Roslyn falla por bootstrap, el core reintenta una sola vez con el siguiente sin reprobe en cascada |
