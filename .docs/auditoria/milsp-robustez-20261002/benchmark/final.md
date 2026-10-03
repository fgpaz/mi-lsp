# Benchmark mi-lsp vs ripgrep: final

- Binario: `mi-lsp version (devel) protocol=mi-lsp-v1.1`
- Casos: 44, corridas por latencia: 3 (mediana), timeout por comando: 30s
- Precisión y recall se promedian sobre los casos con resultado puntuable; un error de mi-lsp cuenta recall 0. Para `intent` se reporta hit@5 en lugar de precisión/recall.
- Bytes: promedio por caso (mi-lsp `--format compact` y salida por defecto; rg stdout). Latencia: media de las medianas por caso, en ms.

| Tipo | Lenguaje | N | Precisión | Recall | hit@5 mi-lsp | hit@5 rg | Bytes mi-lsp (compact) | Bytes mi-lsp (default) | Bytes rg | ms mi-lsp | ms rg | Falsos vacíos | Errores |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| definition | csharp | 4 | 100% | 100% | - | - | 308 | 122 | 189 | 17 | 7 | 0 | 0 |
| definition | go | 8 | 100% | 100% | - | - | 467 | 194 | 118 | 22 | 7 | 0 | 0 |
| definition | typescript | 4 | 100% | 100% | - | - | 334 | 118 | 107 | 70 | 9 | 0 | 0 |
| intent | csharp | 2 | - | - | 100% | 50% | 2738 | 2550 | 3318 | 48 | 7 | 0 | 0 |
| intent | go | 4 | - | - | 50% | 75% | 1755 | 1574 | 6756 | 29 | 7 | 0 | 0 |
| intent | typescript | 2 | - | - | 100% | 0% | 1954 | 1792 | 25478 | 337 | 9 | 0 | 0 |
| literal | csharp | 1 | 100% | 17% | - | - | 2294 | 586 | 4210 | 33 | 6 | 0 | 0 |
| literal | go | 2 | 100% | 59% | - | - | 1762 | 552 | 1998 | 31 | 5 | 0 | 0 |
| literal | typescript | 1 | 100% | 36% | - | - | 2111 | 671 | 2117 | 92 | 7 | 0 | 0 |
| refs | csharp | 4 | 100% | 100% | - | - | 5734 | 2932 | 3644 | 677 | 6 | 0 | 0 |
| refs | go | 8 | 100% | 84% | - | - | 2239 | 2132 | 1743 | 1330 | 6 | 0 | 0 |
| refs | typescript | 4 | 100% | 100% | - | - | 5827 | 2547 | 3447 | 8137 | 7 | 0 | 0 |

**Total:** 44 casos, 0 falsos vacíos (0%), 0 errores (0%).

## Definition: posición esperada

- mi-lsp devuelve la posición esperada en 16/16 casos; el oráculo de rg incluye la posición esperada en 16/16.

