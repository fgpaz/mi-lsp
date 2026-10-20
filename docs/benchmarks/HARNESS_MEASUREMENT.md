# Medición de herramientas en Pi: MiniMax y LongCat

Esta campaña compara una misma conversación pequeña de tres tareas en dos momentos: binario publicado anterior e implementación integrada. No es una suite de implementación ni demuestra causalidad. El baseline está autorizado como medición; el after pertenece a la FINAL_VERIFY del objetivo padre.

## Qué se mide

El piloto deriva del corpus Go de `benchmarks/victory-lab/v1`: localizar la implementación de `Greet`, editar el prefijo del saludo y descubrir/reparar un caller con tipo incorrecto mediante diagnostics antes de cualquier test. El oráculo es determinista y acotado: path final, expresión/firma esperadas, diagnóstico observado y caller reparado. Los tres microobjetivos comparten conversación; no se asignan tokens imaginarios por tarea.

Los procesos son Pi reales con herramientas read/edit/write y un ejecutor de comandos estrictamente acotado. Cada uno usa un fixture separado, sin acceso de escritura al producto. El runner se llama con MiniMax y LongCat free explícitos; no hay fallback de modelo ni compra de créditos. Un 429 de LongCat es un fallo de ese intento de LongCat, aunque MiniMax complete su tarea.

## Reproducir sin alterar el experimento

El contrato detallado y fijado antes del baseline está en [`measurement-contract.md`](../../measurement-contract.md). Los scripts viven en [`benchmarks/harness-measurement/`](../../benchmarks/harness-measurement/). El prompt, runner, guard, fixture, herramientas, límites, modelo y versión de Pi deben conservarse en el par. El backend mi-lsp es la única variable intencional. La revisión inmutable del modelo detrás del alias es unknown y limita toda interpretación.

```sh
# Baseline: un solo intento por modelo, en paralelo, no volver a correr para mejorar.
python3 benchmarks/harness-measurement/run.py baseline minimax --binary results/harness-measurement/bin/mi-lsp-baseline
python3 benchmarks/harness-measurement/run.py baseline longcat --binary results/harness-measurement/bin/mi-lsp-baseline

# Después, SOLO en la FINAL_VERIFY del padre con producto integrado:
python3 benchmarks/harness-measurement/run.py after minimax --binary "$INTEGRATED_MI_LSP" --integrated-sha "$INTEGRATED_SHA"
python3 benchmarks/harness-measurement/run.py after longcat --binary "$INTEGRATED_MI_LSP" --integrated-sha "$INTEGRATED_SHA"

# Render offline, sin invocar modelos ni producto:
python3 benchmarks/harness-measurement/report.py
```

El runner rechaza carpetas de intentos existentes. No borrarlas para seleccionar el mejor resultado. Si se descubre un defecto de infraestructura, se conserva el intento como inválido y se publica una nueva revisión de contrato para una campaña futura, sin atribuir mejoras a la presente. El after reinicia únicamente el fixture temporal de su propio modelo; conserva los datos baseline y usa el mismo cwd para su par.

## Evidencia y límites

Datos en `results/harness-measurement/{baseline,after}/{minimax,longcat}/`: invocación con hashes, eventos nativos seleccionados y sanitizados, journal de peticiones/status, métricas crudas y archivos finales. Tabla legible en [`results/measurement-baseline.md`](../../results/measurement-baseline.md).

- Tokens: únicamente contadores finales que reporta Pi; missing/no usage = unknown con motivo. No dividir bytes por cuatro ni estimar por tamaño.
- Peticiones: hook pre-provider y respuestas observadas; reintentos HTTP internos no observables no se inventan.
- Archivos leídos: paths de herramientas read; inventario transitivo de gopls/mi-lsp es unknown.
- Errores de herramientas y proveedor se conservan, separados del exit code y del score.
- Ratios after/baseline se muestran por modelo. Unknown, fallo, par distinto o denominador cero se explicitan; nunca se convierten en ahorro.
- Un solo intento por modelo/fase; sin inferencia estadística, control completo de carga, revisión inmutable del modelo ni garantía de estabilidad del free.
- Este flujo mide CLI dentro de Pi, no provider MCP, Claude Code, otra máquina o compatibilidad multiplataforma. Esas superficies requieren la evidencia independiente del padre.

## Seguridad

La extensión opt-in de medición no se instala como hook. Solo permite lectura Go en src/ y edición de dos archivos del fixture; no expone shell libre. Los cuatro comandos posibles se ejecutan por argv en bwrap, sin red, con filesystem del host readonly y cachés propias. La resolución de credenciales pertenece al harness; jamás se vuelcan auth, headers, secretos o cuerpos de error HTTP. No hay tests antes del diagnóstico ni se da al modelo una herramienta de test.

La copia de binario en `results/harness-measurement/bin/` es un artefacto local de reproducibilidad, no un archivo a publicar en git. Para una campaña en otra máquina se requiere el artefacto oficial de mi-lsp 0.14.5 con el digest publicado en el contrato; no recompilar el baseline y asumir equivalencia binaria.
