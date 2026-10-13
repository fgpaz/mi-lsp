# Fixture manual Kotlin Graph main-jvm

Fuente únicamente, sin ejecución ni PASS, sin runner nuevo. Owner coord-milsp; vencimiento 2026-10-17. Seguir `TP-GPH.kotlin-compiler-export-v1` y `ADR-KOTLIN-GRAPH-ADMISSION-V1` dentro de la única FINAL_VERIFY posterior.

## Oráculo previo y límites

- `a.Box` type, `a.Box.value` property, dos `pick` method con firmas diferentes; `a.greeting` property, dos `a.echo` function, `a.String.decorate` function extension y `a.localDomain` function. `b.echo` y `b.use` function son identidades diferentes del homónimo en a. Parámetros, typealias, constructor/accessors, locales y anónimo no son nodos admitidos.
- Solo REFERENCE_TARGET cuyos dos descriptores originales están en el inventario local producen references. Constructor `Box(3)`, JavaInput/External, stdlib, locals y targets no representables producen omisiones. Ninguna calls edge, aunque haya invocaciones; calls unsupported visible fuera del batch.
- `a.echo(1)` y `a.echo(2)` dentro de use comparten arista y tienen evidencia distinta; la ocurrencia repetida se contabiliza representacionalmente. El overload String es otro target. Comentarios/literales homónimos no producen aristas.
- Raw coverage cuenta cada KtDeclaration/KtReferenceExpression visitado, no una cifra inventada de nodos. El oracle exacto de ocurrencias/rangos/compiler identities se debe fijar por el verificador con el pin antes de comparar; la autoría no ejecutó compiler ni afirma ese conteo.
- Emoji antes de referencias prueba UTF-16→byte columns; desplazar comentarios/rangos conserva NodeKey y cambia evidence/source digest. Renombrar solo parámetros conserva identidad. Receiver/tipo/firma/compiler module cambian identidad; root absoluto y config JSONSHA no entran en NodeKey.

## Preparación futura aislada

Copiar las siete fuentes a sandbox, red deshabilitada, JDK11/Gradle8.10/compiler2.1.0 cacheados. Compilar dependency-src a un JAR local allí (nunca versionar binario); incluir JavaInput en main inventory y External solo classpath. Luego, y solo entonces: `MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1 bash worker-kotlin/config/export-main-jvm.sh /sandbox/modulo`. No usar wrapper ni Java 25. El exporter debe dejar el JSON y el recibo en `.mi-lsp/` del sandbox, no en git. Sin ese recibo, o con solo stdlib/backup, no hay observación. Con recibo válido el batch sigue partial hasta los seis oráculos de RF-GPH-008; no esperar publicación positiva por esta fixture sola.

Crear segunda instancia del módulo y root relocalizado dentro del mismo sandbox para comprobar no mezcla entre módulos e identidad relocatable. Variar classpath/build/config sin cambiar declaraciones: fingerprint cambia, keys no. Casos negativos: diagnostics reales, target sin resolver, input/config/JAR drift, surrogate interior/range inválido, raw v1/stock/malformed/no opt-in, cancel/deadline/crash/restart y Windows sin Job Objects. Conservar generación previa ante rechazo. Verificar límites/razones en pipeline/direct/daemon. No editar claves C#/Go/docgraph ni migrar durante query.
