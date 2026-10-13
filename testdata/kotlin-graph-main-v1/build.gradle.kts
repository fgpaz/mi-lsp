plugins { kotlin("jvm") version "2.1.0" }

// Solo ejecución futura offline, con inputs exactos ya cacheados.
// dependency-src se compila a este JAR únicamente en sandbox FINAL_VERIFY.
dependencies { compileOnly(files("external-fixture.jar")) }
kotlin { jvmToolchain(11) }
