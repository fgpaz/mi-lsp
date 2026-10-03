# Benchmark mi-lsp vs ripgrep: baseline

- Binario: `mi-lsp version (devel) protocol=mi-lsp-v1.1`
- Casos: 44, corridas por latencia: 3 (mediana), timeout por comando: 30s
- Precisión y recall se promedian sobre los casos con resultado puntuable; un error de mi-lsp cuenta recall 0. Para `intent` se reporta hit@5 en lugar de precisión/recall.
- Bytes: promedio por caso (mi-lsp `--format compact` y salida por defecto; rg stdout). Latencia: media de las medianas por caso, en ms.

| Tipo | Lenguaje | N | Precisión | Recall | hit@5 mi-lsp | hit@5 rg | Bytes mi-lsp (compact) | Bytes mi-lsp (default) | Bytes rg | ms mi-lsp | ms rg | Falsos vacíos | Errores |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| definition | csharp | 4 | - | 0% | - | - | 625 | 289 | 189 | 13 | 7 | 4 | 4 |
| definition | go | 8 | - | 0% | - | - | 606 | 278 | 118 | 12 | 6 | 8 | 8 |
| definition | typescript | 4 | - | 0% | - | - | 704 | 321 | 107 | 65 | 8 | 4 | 4 |
| intent | csharp | 2 | - | - | 0% | 50% | 1272 | 1072 | 3318 | 27 | 7 | 0 | 0 |
| intent | go | 4 | - | - | 0% | 75% | 846 | 648 | 6756 | 22 | 7 | 0 | 0 |
| intent | typescript | 2 | - | - | 0% | 0% | 617 | 417 | 25478 | 73 | 9 | 0 | 0 |
| literal | csharp | 1 | 100% | 17% | - | - | 1927 | 771 | 4210 | 31 | 5 | 0 | 0 |
| literal | go | 2 | 100% | 59% | - | - | 1637 | 516 | 1998 | 29 | 5 | 0 | 0 |
| literal | typescript | 1 | 100% | 36% | - | - | 1984 | 538 | 2117 | 81 | 7 | 0 | 0 |
| refs | csharp | 4 | - | 0% | - | - | 371 | 171 | 3644 | 211 | 6 | 4 | 4 |
| refs | go | 8 | 83% | 50% | - | - | 999 | 898 | 1743 | 566 | 6 | 4 | 0 |
| refs | typescript | 4 | 78% | 100% | - | - | 6857 | 6684 | 3447 | 125 | 7 | 0 | 0 |

**Total:** 44 casos, 24 falsos vacíos (55%), 20 errores (45%).

## Códigos de error de mi-lsp

- `index_not_ready`: 12
- `workspace_db_open_failed`: 4
- `nav_generic`: 4

## Definition: posición esperada

- mi-lsp devuelve la posición esperada en 0/16 casos; el oráculo de rg incluye la posición esperada en 16/16.

