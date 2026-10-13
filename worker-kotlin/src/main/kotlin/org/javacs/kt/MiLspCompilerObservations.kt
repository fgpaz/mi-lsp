package org.javacs.kt

import com.google.gson.Gson
import com.intellij.psi.util.PsiTreeUtil
import org.eclipse.lsp4j.*
import org.eclipse.lsp4j.jsonrpc.services.JsonRequest
import org.eclipse.lsp4j.jsonrpc.services.JsonDelegate
import org.eclipse.lsp4j.services.*
import org.javacs.kt.compiler.Compiler
import org.jetbrains.kotlin.descriptors.*
import org.jetbrains.kotlin.diagnostics.Severity
import org.jetbrains.kotlin.psi.*
import org.jetbrains.kotlin.renderer.DescriptorRenderer
import org.jetbrains.kotlin.resolve.BindingContext
import org.jetbrains.kotlin.resolve.DescriptorUtils
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.security.MessageDigest
import java.util.concurrent.CompletableFuture

/** Minimal compiler-only endpoint in the same launcher/JVM, without stock DB,
 * source indexing or Gradle services. No secondary subprocess or framework. */
class MiLspCompilerServer : LanguageServer, LanguageClientAware {
    private val observations = MiLspCompilerObservationService()
    @JsonDelegate fun getMiLspCompilerObservations(): MiLspCompilerObservations = observations
    override fun connect(client: LanguageClient) {}
    override fun getTextDocumentService(): TextDocumentService = object : TextDocumentService {
        override fun didOpen(params: DidOpenTextDocumentParams) {}
        override fun didChange(params: DidChangeTextDocumentParams) {}
        override fun didClose(params: DidCloseTextDocumentParams) {}
        override fun didSave(params: DidSaveTextDocumentParams) {}
    }
    override fun getWorkspaceService(): WorkspaceService = object : WorkspaceService {
        override fun didChangeConfiguration(params: DidChangeConfigurationParams) {}
        override fun didChangeWatchedFiles(params: DidChangeWatchedFilesParams) {}
    }
    override fun getNotebookDocumentService(): NotebookDocumentService? = null
    override fun initialize(params: InitializeParams): CompletableFuture<InitializeResult> {
        val options = Gson().toJsonTree(params.initializationOptions)
        require(options?.isJsonObject == true && options.asJsonObject.get("miLspCompilerOnly")?.asBoolean == true)
        observations.compilerOnly = true
        val capabilities = ServerCapabilities()
        capabilities.experimental = mapOf("miLspCompilerObservationsV2" to true)
        return CompletableFuture.completedFuture(InitializeResult(capabilities,
            ServerInfo("mi-lsp fwcd compiler fork", System.getProperty("kotlinLanguageServer.version"))))
    }
    override fun shutdown(): CompletableFuture<Any> {
        observations.compilerOnly = false
        return CompletableFuture.completedFuture(null)
    }
    override fun exit() {}
}

/** Source extension of fwcd 1.3.13; not a stock LSP method or Graph schema. */
interface MiLspCompilerObservations {
    @JsonRequest("kotlin/miLsp/compilerObservations/v2")
    fun compilerObservations(params: MiLspObservationRequest): CompletableFuture<Map<String, Any>>
}

data class MiLspInput(val path: String, val sha256: String)
data class MiLspMainConfiguration(
    val schema_version: Int,
    val resolution: String,
    val module_root: String,
    val module_name: String,
    val source_set: String,
    val jdk_home: String,
    val jvm_target: String,
    val gradle_version: String,
    val compiler_version: String,
    val server_version: String,
    val sources: List<MiLspInput>,
    val classpath: List<MiLspInput>,
    val build_inputs: List<MiLspInput>
)
data class MiLspObservationRequest(val module_root: String, val configuration_path: String, val configuration_sha256: String)
data class MiLspArtifact(val path: String, val sha256: String, val coordinate: String)
data class MiLspGap(val kind: String, val detail: String)
data class MiLspAuthorityReceipt(
    val receipt_version: Int,
    val authority: String,
    val capture: String,
    val certified: Boolean?,
    val certification_blockers: List<String>?,
    val resolver: String,
    val backup_classpath: Boolean?,
    val classpath_scope: String,
    val calls: String,
    val graph_completeness: String,
    val promotion_oracles_missing: List<String>?,
    val module_root: String,
    val module_name: String,
    val source_set: String,
    val jdk_home: String,
    val jdk_release_sha256: String,
    val jvm_target: String,
    val gradle_version: String,
    val compiler_version: String,
    val server_version: String,
    val server_version_binding: String,
    val configuration_sha256: String,
    val exporter_path: String,
    val exporter_sha256: String,
    val init_script_path: String,
    val init_script_sha256: String,
    val evidence_digest: String,
    val capture_command: String,
    val sources: List<MiLspInput>?,
    val classpath: List<MiLspInput>?,
    val build_inputs: List<MiLspInput>?,
    val artifacts: List<MiLspArtifact>?,
    val unrepresentable: List<MiLspGap>?
)

class MiLspCompilerObservationService : MiLspCompilerObservations {
    var compilerOnly = false
    private val renderer = DescriptorRenderer.FQ_NAMES_IN_TYPES
    private val identityRenderer = renderer.withOptions {
        withDefinedIn = false
        withSourceFileForTopLevel = false
        parameterNameRenderingPolicy = org.jetbrains.kotlin.renderer.ParameterNameRenderingPolicy.NONE
        parameterNamesInFunctionalTypes = false
        modifiers = emptySet()
        withoutSuperTypes = true
        includePropertyConstant = false
    }
    private fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
    private fun read(input: MiLspInput, limit: Long): ByteArray {
        val path = Paths.get(input.path)
        require(path.isAbsolute && Files.isRegularFile(path) && Files.size(path) <= limit) { "unavailable: input bound" }
        return Files.readAllBytes(path).also { require(sha(it) == input.sha256) { "unavailable: input changed" } }
    }
    private fun within(root: Path, input: MiLspInput): Path = Paths.get(input.path).toRealPath().also {
        require(it.startsWith(root) && it != root) { "unavailable: source escapes module" }
    }
    private fun identity(descriptor: DeclarationDescriptor): String? {
        val original = descriptor.original
        if (DescriptorUtils.isLocal(original) || original.name.isSpecial) return null
        if (original is ConstructorDescriptor || original is PropertyAccessorDescriptor || original is TypeAliasDescriptor) return null
        if (original !is ClassDescriptor && original !is FunctionDescriptor && original !is PropertyDescriptor) return null
        if (original is CallableMemberDescriptor && original.kind != CallableMemberDescriptor.Kind.DECLARATION) return null
        // Renderer reads resolved descriptor types/receivers/type parameters; never PSI text/ranges.
        val chain = generateSequence(original.containingDeclaration) { it.containingDeclaration }
            .takeWhile { it !is ModuleDescriptor }.toList().asReversed().joinToString("/") { identityRenderer.render(it) }
        return "descriptor-renderer-fq/v1|$chain|${identityRenderer.render(original)}"
    }
    private fun descriptorData(d: DeclarationDescriptor, module: ModuleDescriptor): Map<String, Any> = linkedMapOf(
        "identity" to (identity(d) ?: ""), "signature" to renderer.render(d.original),
        "descriptor_class" to d.original.javaClass.name, "display_name" to d.name.asString(),
        "symbol_kind" to when (d.original) {
            is ClassDescriptor -> "type"
            is PropertyDescriptor -> "property"
            is FunctionDescriptor -> if (d.containingDeclaration is ClassDescriptor) "method" else "function"
            else -> "unsupported"
        },
        "compiler_module" to DescriptorUtils.getContainingModule(d).name.asString(),
        "observation_module" to module.name.asString(),
        "dispatch_receiver" to ((d.original as? CallableDescriptor)?.dispatchReceiverParameter?.type?.let(identityRenderer::renderType) ?: ""),
        "extension_receiver" to ((d.original as? CallableDescriptor)?.extensionReceiverParameter?.type?.let(identityRenderer::renderType) ?: "")
    )
    override fun compilerObservations(params: MiLspObservationRequest): CompletableFuture<Map<String, Any>> {
        // Serialized by the same LSP request owner; no persistent snapshot or secondary process.
        return try { CompletableFuture.completedFuture(observe(params)) }
        catch (failure: Exception) { CompletableFuture<Map<String, Any>>().also { it.completeExceptionally(IllegalStateException("unavailable: compiler observation invalidated")) } }
    }
    private fun requireAuthority(root: Path, configPath: Path, configBytes: ByteArray, config: MiLspMainConfiguration): ByteArray {
        val oracles = listOf("oracle_identity_fixture", "oracle_relation_precision_recall", "oracle_determinism_30_reruns", "oracle_incremental_vs_clean", "oracle_cross_rid", "oracle_conformance_final_verify")
        val receiptPath = root.resolve(".mi-lsp/kotlin-main-v1.authority.json").toRealPath()
        require(receiptPath == configPath.parent.resolve("kotlin-main-v1.authority.json").toAbsolutePath())
        val bytes = Files.readAllBytes(receiptPath)
        require(bytes.size <= 2 shl 20)
        val receipt = Gson().fromJson(String(bytes, Charsets.UTF_8), MiLspAuthorityReceipt::class.java)
        require(receipt.receipt_version == 1 && receipt.authority == "gradle-compileClasspath-resolvedArtifacts/v1" && receipt.capture == "explicit-opt-in-offline")
        require(receipt.certified == true && receipt.certification_blockers?.isEmpty() == true && receipt.backup_classpath == false)
        require(receipt.resolver == "configurations.compileClasspath.resolvedArtifacts" && receipt.classpath_scope == "project-effective")
        require(receipt.calls == "unsupported" && receipt.graph_completeness == "partial" && receipt.promotion_oracles_missing == oracles)
        require(receipt.module_root == config.module_root && receipt.module_name == config.module_name && receipt.source_set == "main-jvm")
        require(receipt.jdk_home == config.jdk_home && receipt.jvm_target == config.jvm_target && receipt.gradle_version == "8.10")
        require(receipt.compiler_version == "2.1.0" && receipt.server_version == "1.3.13+mi-lsp.obs2" && receipt.server_version_binding == "contract_pin_not_runtime_proof")
        require(receipt.configuration_sha256 == sha(configBytes))
        require(receipt.exporter_sha256 == "97fd0a079a12cb921c21404de8415fcdbc7915168fb7d1940836fbbc3579b1b3")
        require(receipt.init_script_sha256 == "ebccdf78fdbf0d56668b924d88452e89215763b761cefdb08964facabf5ed3a5")
        require(Files.size(Paths.get(receipt.exporter_path)) <= 2L shl 20 && Files.size(Paths.get(receipt.init_script_path)) <= 2L shl 20)
        require(sha(Files.readAllBytes(Paths.get(receipt.exporter_path))) == receipt.exporter_sha256)
        require(sha(Files.readAllBytes(Paths.get(receipt.init_script_path))) == receipt.init_script_sha256)
        require(Paths.get(receipt.exporter_path).fileName.toString() == "export-main-jvm.sh" && Paths.get(receipt.init_script_path).fileName.toString() == "export-main-jvm.init.gradle.kts")
        require(receipt.capture_command == "gradle --offline --no-daemon --no-build-cache -p ${root} -I ${receipt.init_script_path} miLspExportMainJvm")
        require(receipt.sources == config.sources && receipt.classpath == config.classpath && receipt.build_inputs == config.build_inputs)
        require(receipt.unrepresentable?.isEmpty() == true)
        val release = Paths.get(receipt.jdk_home).resolve("release").toRealPath()
        require(sha(Files.readAllBytes(release)) == receipt.jdk_release_sha256)
        val artifacts = receipt.artifacts ?: emptyList()
        require(artifacts.map { it.path }.toSet() == config.classpath.map { it.path }.toSet())
        require(artifacts.any { it.coordinate == "module:org.jetbrains.kotlin:kotlin-stdlib:2.1.0" || (it.coordinate.startsWith("module:org.jetbrains.kotlin:kotlin-stdlib-jdk") && it.coordinate.endsWith(":2.1.0")) })
        require(artifacts.any { val parts = it.coordinate.split(":"); !(parts.size >= 3 && parts[0] == "module" && parts[2].startsWith("kotlin-stdlib")) })
        val lines = mutableListOf("authority=${receipt.authority}", "capture=${receipt.capture}", "resolver=${receipt.resolver}", "gradle=${receipt.gradle_version}", "compiler=${receipt.compiler_version}", "server=${receipt.server_version}", "jdk_home=${receipt.jdk_home}", "jdk_release=${receipt.jdk_release_sha256}", "jvm_target=${receipt.jvm_target}", "module_name=${receipt.module_name}", "module_root=${receipt.module_root}", "exporter_sha256=${receipt.exporter_sha256}", "init_script_sha256=${receipt.init_script_sha256}", "promotion_oracles=${oracles.joinToString(",")}")
        receipt.sources?.forEach { lines += "source\t${it.path}\t${it.sha256}" }
        receipt.classpath?.forEach { lines += "classpath\t${it.path}\t${it.sha256}" }
        artifacts.forEach { lines += "artifact\t${it.path}\t${it.sha256}\t${it.coordinate}" }
        receipt.build_inputs?.forEach { lines += "build_input\t${it.path}\t${it.sha256}" }
        require(sha(lines.joinToString("\n").toByteArray(Charsets.UTF_8)) == receipt.evidence_digest)
        return bytes
    }
    @Synchronized private fun observe(params: MiLspObservationRequest): Map<String, Any> {
        require(compilerOnly) { "unavailable: compiler-only initialize required" }
        val root = Paths.get(params.module_root).toRealPath()
        require(Paths.get(params.module_root).isAbsolute)
        val configPath = Paths.get(params.configuration_path).toRealPath()
        require(configPath == root.resolve(".mi-lsp/kotlin-main-v1.json").toAbsolutePath())
        val configBytes = read(MiLspInput(configPath.toString(), params.configuration_sha256), 2L shl 20)
        val config = Gson().fromJson(String(configBytes, Charsets.UTF_8), MiLspMainConfiguration::class.java)
        require(config.schema_version == 1 && config.resolution == "resolved" && config.source_set == "main-jvm")
        require(Paths.get(config.module_root).toRealPath() == root && config.module_name.isNotBlank())
        require(config.compiler_version == "2.1.0" && config.server_version == "1.3.13+mi-lsp.obs2")
        require(Paths.get(config.jdk_home).toRealPath() == Paths.get(System.getProperty("java.home")).toRealPath())
        require(config.gradle_version.isNotBlank() && config.jvm_target.isNotBlank())
        require(config.sources.size in 1..512 && config.classpath.size in 1..256 && config.build_inputs.size in 1..128)
        require(org.jetbrains.kotlin.config.JvmTarget.fromString(config.jvm_target) != null)
        val allInputs = config.sources + config.classpath + config.build_inputs
        require(config.classpath.sumOf { Files.size(Paths.get(it.path)) } <= (1L shl 30))
        require(allInputs.map { it.path }.distinct().size == allInputs.size)
        val main = root.resolve("src/main").toRealPath()
        val sourceBytes = config.sources.sortedBy { it.path }.associateWith { input ->
            val p = within(main, input)
            require(p.toString().endsWith(".kt") || p.toString().endsWith(".java"))
            read(input, 2L shl 20)
        }
        // Exact inventory: no hidden src/test or second module, no missing Java source.
        val diskSources = Files.walk(main).use { stream -> stream.filter { Files.isRegularFile(it) && (it.toString().endsWith(".kt") || it.toString().endsWith(".java")) }.map { it.toRealPath().toString() }.toArray().map { it.toString() }.toSet() }
        require(diskSources == config.sources.map { Paths.get(it.path).toRealPath().toString() }.toSet())
        require(config.classpath.all { it.path.endsWith(".jar") })
        config.classpath.forEach { read(it, 256L shl 20) }
        config.build_inputs.forEach { read(it, 2L shl 20) }
        val receiptBytes = requireAuthority(root, configPath, configBytes, config)
        // Caller supplies resolved Gradle inputs; compiler uses precisely these JARs, never backup resolver.
        val javaSources = config.sources.filter { it.path.endsWith(".java") }
        val javaRoots = if (javaSources.isEmpty()) emptySet<Path>() else {
            val javaRoot = root.resolve("src/main/java").toRealPath()
            require(javaSources.all { Paths.get(it.path).toRealPath().startsWith(javaRoot) }) { "unsupported_operation: nonstandard Java source root" }
            setOf(javaRoot)
        }
        val compiler = Compiler(javaSourcePath = javaRoots, classPath = config.classpath.map { Paths.get(it.path) }.toSet(),
            scriptsConfig = ScriptsConfiguration(enabled = false, buildScriptsEnabled = false),
            codegenConfig = CodegenConfiguration(enabled = false), outputDirectory = root.toFile(),
            miLspModuleName = config.module_name, miLspJdkHome = Paths.get(config.jdk_home).toFile())
        compiler.use {
            compiler.updateConfiguration(CompilerConfiguration(JVMConfiguration(config.jvm_target)))
            val inputsByFile = sourceBytes.filterKeys { it.path.endsWith(".kt") }.map { (input, bytes) ->
                require(!bytes.contains(13.toByte())) { "unsupported_operation: CRLF source v1" }
                compiler.createKtFile(String(bytes, Charsets.UTF_8), Paths.get(input.path)) to input
            }.toMap()
            val files = inputsByFile.keys.toList()
            require(files.isNotEmpty())
            val (context, module) = compiler.compileKtFiles(files, files)
            require(module.name.asString() == "<${config.module_name}>")
            val declarations = mutableListOf<Map<String, Any>>()
            val references = mutableListOf<Map<String, Any>>()
            val omissions = mutableListOf<Map<String, Any>>()
            val local = mutableMapOf<DeclarationDescriptor, Map<String, Any>>()
            var declarationEligible = 0
            var declarationUnresolved = 0
            var referenceEligible = 0
            var referenceUnresolved = 0
            for (file in files) {
                val input = inputsByFile.getValue(file)
                val owner = root.relativize(Paths.get(input.path)).toString().replace('\\', '/')
                for (psi in PsiTreeUtil.collectElementsOfType(file, KtDeclaration::class.java).sortedBy { it.textRange.startOffset }) {
                    declarationEligible++
                    val descriptor = context[BindingContext.DECLARATION_TO_DESCRIPTOR, psi]
                    if (descriptor == null || identity(descriptor) == null || (psi !is KtClassOrObject && psi !is KtNamedFunction && psi !is KtProperty)) {
                        if (descriptor == null && (psi is KtClassOrObject || psi is KtNamedFunction || psi is KtProperty)) declarationUnresolved++
                        omissions.add(mapOf("owner_path" to owner, "capability" to "declarations", "reason_code" to (if (descriptor == null && (psi is KtClassOrObject || psi is KtNamedFunction || psi is KtProperty)) "compiler_declaration_unresolved" else "descriptor_unrepresentable")))
                        continue
                    }
                    val declaration = descriptorData(descriptor, module) + mapOf("owner_path" to owner, "source_sha256" to input.sha256, "binding_slice" to "DECLARATION_TO_DESCRIPTOR", "start_offset_utf16" to psi.textRange.startOffset, "end_offset_utf16" to psi.textRange.endOffset)
                    require(local.put(descriptor.original, declaration) == null) { "unavailable: ambiguous compiler declaration" }
                    declarations.add(declaration)
                }
            }
            for (file in files) {
                val input = inputsByFile.getValue(file)
                val owner = root.relativize(Paths.get(input.path)).toString().replace('\\', '/')
                for (psi in PsiTreeUtil.collectElementsOfType(file, KtReferenceExpression::class.java).sortedBy { it.textRange.startOffset }) {
                    referenceEligible++
                    val target = context[BindingContext.REFERENCE_TARGET, psi]
                    val enclosing = generateSequence(psi.parent) { it.parent }.filterIsInstance<KtDeclaration>()
                        .mapNotNull { context[BindingContext.DECLARATION_TO_DESCRIPTOR, it] }.firstOrNull()
                    if (target == null || enclosing == null || local[enclosing.original] == null || local[target.original] == null) {
                        val reason = if (target == null && enclosing != null && local[enclosing.original] != null) "compiler_reference_unresolved" else "endpoint_unrepresentable"
                        if (reason == "compiler_reference_unresolved") referenceUnresolved++
                        omissions.add(mapOf("owner_path" to owner, "capability" to "references", "reason_code" to reason))
                        continue
                    }
                    references.add(mapOf("from_identity" to identity(enclosing)!!, "from_compiler_module" to DescriptorUtils.getContainingModule(enclosing).name.asString(),
                        "from_owner_path" to local.getValue(enclosing.original).getValue("owner_path"),
                        "target_owner_path" to local.getValue(target.original).getValue("owner_path"), "target" to descriptorData(target, module),
                        "owner_path" to owner, "source_sha256" to input.sha256, "binding_slice" to "REFERENCE_TARGET", "start_offset_utf16" to psi.textRange.startOffset, "end_offset_utf16" to psi.textRange.endOffset))
                }
            }
            val errorCount = context.diagnostics.all().count { it.severity == Severity.ERROR }
            // Input/config changes, including JARs, invalidate the entire observation before response.
            allInputs.forEach { read(it, if (it in config.classpath) 256L shl 20 else 2L shl 20) }
            require(read(MiLspInput(configPath.toString(), params.configuration_sha256), 2L shl 20).contentEquals(configBytes))
            require(read(MiLspInput(root.resolve(".mi-lsp/kotlin-main-v1.authority.json").toString(), sha(receiptBytes)), 2L shl 20).contentEquals(receiptBytes))
            require(!Thread.currentThread().isInterrupted)
            val sourceFingerprint = sha(config.sources.sortedBy { it.path }.joinToString("\n") { "${it.path}\u0000${it.sha256}" }.toByteArray(Charsets.UTF_8))
            val domainClosed = declarationEligible == declarations.size && referenceEligible == references.size && errorCount == 0
            return linkedMapOf("schema_version" to 2, "extension_version" to "mi-lsp-compiler-observations/v2",
                "server_version" to "1.3.13+mi-lsp.obs2", "compiler_version" to "2.1.0", "upstream_commit" to "f532bcc9c6578f5a35009a8b5f281f93d4d131ba",
                "module_root" to root.toString(), "module_descriptor" to module.name.asString(), "configuration" to config,
                "config_fingerprint" to params.configuration_sha256, "source_fingerprint" to sourceFingerprint,
                "resolution" to "BindingContext+ModuleDescriptor", "completeness" to "partial", "diagnostic_errors" to errorCount,
                "capabilities" to mapOf("declarations" to "experimental", "references" to "experimental", "calls" to "unsupported"),
                "coverage" to mapOf(
                    "declarations" to mapOf("eligible" to declarationEligible, "observed" to declarations.size, "unresolved" to declarationUnresolved, "omitted" to omissions.count { it["capability"] == "declarations" && it["reason_code"] != "compiler_declaration_unresolved" }),
                    "references" to mapOf("eligible" to referenceEligible, "observed" to references.size, "unresolved" to referenceUnresolved, "omitted" to omissions.count { it["capability"] == "references" && it["reason_code"] != "compiler_reference_unresolved" })), 
                "declarations" to declarations, "references" to references, "omissions" to omissions,
                "limitations" to listOf("java_declarations_not_exported", "external_endpoints_not_graph_nodes", "no_resolved_call_export", "calls_unsupported", "authority_receipt_checked", if (domainClosed) "domain_coverage_closed_promotion_oracles_missing" else "domain_coverage_open", "promotion_oracles_missing"))
        }
    }
}
