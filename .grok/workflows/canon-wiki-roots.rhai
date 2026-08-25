let meta = #{
    name: "canon-wiki-roots",
    description: "Add [[canon]] roots, nav wiki-root, honor governance paths outside .docs/wiki, and workspace link federation",
    when_to_use: "When implementing external wiki/canon resolution for mi-lsp",
    phases: [
        #{ title: "A-canon-config", detail: "[[canon]] types, validation, round-trip, tests" },
        #{ title: "B-wiki-root", detail: "mi-lsp nav wiki-root command and handler" },
        #{ title: "C-governance", detail: "honor source_doc and projection.output outside .docs/wiki" },
        #{ title: "D-federation", detail: "workspace link plus docs-only indexing of canon roots" },
        #{ title: "docs", detail: "changelog, wiki RF/CT, skill surface" },
        #{ title: "verify", detail: "adversarial acceptance criteria" },
    ],
};

let packet_schema = #{
    "type": "object",
    "properties": #{
        "changed_paths": #{ "type": "array", "items": #{ "type": "string" } },
        "summary": #{ "type": "string" },
        "blocked": #{ "type": "boolean" },
        "blocker": #{ "type": "string" },
    },
};

let verdict_schema = #{
    "type": "object",
    "properties": #{
        "pass": #{ "type": "boolean" },
        "reason": #{ "type": "string" },
        "evidence": #{ "type": "string" },
        "failures": #{ "type": "array", "items": #{ "type": "string" } },
    },
};

fn common_rules() {
    let s = "";
    s += "You are a leaf implementer in C:/repos/mios/mi-lsp (Go module github.com/fgpaz/mi-lsp).\n";
    s += "READ files before editing. Use grep/read_file. Match existing style (gofmt, no new deps).\n";
    s += "Do not bump model.ProtocolVersion (keep mi-lsp-v1.1). Do not add fields to model.Envelope.\n";
    s += "Do not commit, push, tag, or edit AGENTS.md/CLAUDE.md.\n";
    s += "No absolute machine/user paths in any versioned file.\n";
    s += "A project.toml without [[canon]] MUST behave exactly as today.\n";
    s += "Fail-closed: explicit actionable errors, never silent resolution.\n";
    s += "WRITE tests for your surface. You MAY run gofmt -w and go test on packages you touched to keep them compiling. Do not declare the goal complete.\n";
    s += "This lane is NOT a completion boundary. Report changed_paths + summary only.\n";
    s += "Do not use symlinks/junctions as a feature. Do not copy canon trees.\n";
    s
}

fn locked_design() {
    let s = "";
    s += "LOCKED DESIGN (do not reopen):\n";
    s += "1) [[canon]] lives in .mi-lsp/project.toml as repeated [[canon]] tables. Struct WorkspaceCanon {ID, Root, Role, Mode} with toml tags id/root/role/mode. ProjectFile.Canons []WorkspaceCanon `toml:\"canon\"`.\n";
    s += "2) Optional [canon_policy] CanonPolicyBlock { EscapeMax *int `toml:\"escape_max,omitempty\"` }. Nil/omitted => 1. Explicit 0 => no parent escape. Pointer required so omitted vs 0 differ.\n";
    s += "3) Resolve Root relative to the WORKSPACE ROOT (directory that contains .mi-lsp/project.toml), NEVER relative to .mi-lsp/ and NEVER to cwd. Reason: advertised example is root=\"../wiki-repo/Ingenieria\" with default escape_max=1. If resolved from .mi-lsp/ that example would need ../../ and would be illegal.\n";
    s += "4) Absolute roots are ERRORS not warnings: filepath.IsAbs, leading /, Windows volume X: and UNC \\\\ . Also reject ~/\n";
    s += "5) Escape count = number of '..' components AFTER filepath.Clean+ToSlash of the DECLARED root. Reject if count > escapeMax. Message must say the max and the declared path.\n";
    s += "6) Role required when [[canon]] present: producto | ecosistema | gobierno_local. Mode omitted => read-only. Any other mode => error. Only read-only is implemented.\n";
    s += "7) id required, unique (case-insensitive). Empty id => error.\n";
    s += "8) Symlink/junction: at resolution, Lstat every component of the resolved absolute path. If ModeSymlink (Go 1.23+ junctions too) => error canon_root_symlink. Missing path => error canon_root_missing with the declared relative path, not the absolute.\n";
    s += "9) Validation of declared string (abs/escape/role/mode/id) is separate from existence/symlink. nav find must still work if [[canon]] is invalid; only wiki-root, docs-only canon walk, and governance-from-canon fail closed.\n";
    s += "10) SaveProjectFile / mergeProjectFile / workspace.init MUST preserve Canons and CanonPolicy. Init currently Detect+Save and would wipe them: LoadProjectFile first and copy Canons/CanonPolicy onto the detected project before Save.\n";
    s += "11) BurntSushi TOML ignores unknown keys. Without the struct field [[canon]] is a silent no-op. Tests must round-trip Save+Load.\n";
    s += "12) wiki-root CLI MUST be hyphenated: mi-lsp nav wiki-root (Use: \"wiki-root\"). Operation id: nav.wiki-root. Also add nav wiki root as an alias forwarding the same operation.\n";
    s += "13) Items type model.WikiRootResolution { WikiRoot, Role, Workspace, GovernanceDoc, ResolvedFrom, ID } json tags wiki_root, role, workspace, governance_doc, resolved_from, id. wiki_root and governance_doc are slash-normalized DECLARED relative paths (portable), never absolute.\n";
    s += "14) resolved_from values: canon.<id> | repo.root | default | registry.link . repo.root only if you actually resolve from [[repo]]; do not fake it. New command items only; do not change existing envelopes.\n";
    s += "15) No [[canon]] => one item: wiki_root=.docs/wiki, governance_doc=.docs/wiki/00_gobierno_documental.md, resolved_from=default, role empty or omitted, workspace=alias. Must not error.\n";
    s += "16) Multiple [[canon]] without --role => return ALL as items (stable sort by id). --role filters; zero matches => error with the available roles/ids. Do not silently fall back.\n";
    s += "17) governance_doc for a canon = <declared-root>/00_gobierno_documental.md unless a readable profile source_doc under that root says otherwise. Keep relative.\n";
    s += "18) Read-only enforcement: any write whose abs target is inside a resolved canon root fails with canon_root_read_only. Index and nav are reads. Projection auto_sync must NOT write into a foreign canon root; write inside the code workspace instead (declared projection.output if in-workspace, else .docs/wiki/_mi-lsp/read-model.toml).\n";
    s += "19) Protocol stays mi-lsp-v1.1. Add nav.wiki-root to App.Execute, operationRequiresWorkspaceResolution, shouldUseDaemon=false (same list as nav.governance / nav.wiki.map). Do not add to wikiCodeEnrichedOperations.\n";
    s += "20) compactItems: use a typed slice so toon/json/yaml keep full json tags. Do not use []map[string]any with name/kind/file/line keys.\n";
    s
}

let a_prompt = common_rules();
a_prompt += locked_design();
a_prompt += "PHASE A ONLY. Do not implement nav wiki-root, governance decoupling, workspace link, or docs-only walking yet. Leave function signatures that later phases can call.\n";
a_prompt += "CREATE internal/workspace/canon.go and internal/workspace/canon_test.go.\n";
a_prompt += "Public API (exact names):\n";
a_prompt += "  var ErrCanonRootAbsolute, ErrCanonRootEscape, ErrCanonRootSymlink, ErrCanonRootMissing, ErrCanonRoleInvalid, ErrCanonModeInvalid, ErrCanonIDDuplicate, ErrCanonIDMissing, ErrCanonRootReadOnly error\n";
a_prompt += "  func CanonEscapeMax(project model.ProjectFile) int\n";
a_prompt += "  func ValidateCanonDeclarations(project model.ProjectFile) error  // declared-string checks only\n";
a_prompt += "  type ResolvedCanon struct { ID, Role, Mode, DeclaredRoot, AbsRoot string }\n";
a_prompt += "  func ResolveCanons(workspaceRoot string, project model.ProjectFile) ([]ResolvedCanon, error) // validate + abs + exist + symlink\n";
a_prompt += "  func ResolveCanonsByRole(workspaceRoot string, project model.ProjectFile, role string) ([]ResolvedCanon, error)\n";
a_prompt += "  func PathIsReadOnlyCanon(workspaceRoot string, project model.ProjectFile, absTarget string) (bool, error)\n";
a_prompt += "  Errors must include the declared relative path and a repair hint (e.g. use ../sibling not C:\\\\...; raise [canon_policy].escape_max to allow ../../).\n";
a_prompt += "EDIT internal/model/types.go ProjectFile to add Canons and CanonPolicy. Put WorkspaceCanon and CanonPolicyBlock next to WorkspaceRepo (~796).\n";
a_prompt += "EDIT internal/workspace/topology.go mergeProjectFile so Canons/CanonPolicy from existing are kept (they already would if you do not overwrite; do not copy empty detected over existing).\n";
a_prompt += "EDIT internal/service/workspace_ops.go workspaceInit (~50-65): before SaveProjectFile, LoadProjectFile(absPath) and if Canons/CanonPolicy present, copy them onto project.\n";
a_prompt += "TESTS in canon_test.go (table-driven, t.TempDir, no machine paths in assertions of declared fields):\n";
a_prompt += "  relative ../wiki-repo/Ingenieria from workspace root resolves; declared root preserved as slash path\n";
a_prompt += "  absolute Unix, Windows C:\\\\, UNC rejected\n";
a_prompt += "  ../../ escape rejected at default max=1; accepted when escape_max=2\n";
a_prompt += "  symlink/junction component rejected (skip test if os.Symlink/junction create fails, same pattern as identity_test.go)\n";
a_prompt += "  empty Canons: ValidateCanonDeclarations nil; ResolveCanons empty slice nil error\n";
a_prompt += "  duplicate ids rejected; bad role rejected; mode=write rejected; omitted mode treated as read-only\n";
a_prompt += "  LoadProjectFile+SaveProjectFile round trip preserves [[canon]]\n";
a_prompt += "Do not change walker.go, nav.go, governance.go in this phase except workspace_ops init preserve.\n";
a_prompt += "gofmt -w touched files. go test ./internal/workspace -count=1 -timeout 60s\n";
a_prompt += "Return {changed_paths, summary, blocked, blocker}.\n";

phase("A-canon-config");
log("Phase A: [[canon]] config and validation");
let a = agent(a_prompt, #{
    label: "A-canon-config",
    capability_mode: "all",
    agent_type: "claudex-writer",
    output_schema: packet_schema,
});
if a == () || !a.success {
    await_user("verification", "Phase A writer failed. Inspect /workflows, repair, then resume.");
}
if a.output != () && a.output.blocked == true {
    await_user("verification", "Phase A reported blocked. Resume after unblocking.");
}

let b_prompt = common_rules();
b_prompt += locked_design();
b_prompt += "PHASE B. Phase A already added WorkspaceCanon, ValidateCanonDeclarations, ResolveCanons in internal/workspace/canon.go. READ those files and use them. Do not reimplement validation.\n";
b_prompt += "Implement mi-lsp nav wiki-root.\n";
b_prompt += "CLI: internal/cli/nav.go newNavCommand AddCommand a wiki-root command (Use: \"wiki-root\") next to workspace-map/governance. Flag --role string. Payload {role}. Operation \"nav.wiki-root\". preferDaemon true (shouldUseDaemon will return false once listed).\n";
b_prompt += "Also in newNavWikiCommand add Use:\"root\" alias that executes the same operation so `nav wiki root` works.\n";
b_prompt += "CLI test: TestNavWikiRootCommandExists in nav_test.go using command.Find([]string{\"wiki-root\"}) and Find([]string{\"wiki\",\"root\"}). Also assert --role flag exists. Use executeOperationHook to assert operation id nav.wiki-root and preferDaemon true (daemon bypass is shouldUseDaemon, hook still sees the prefer flag).\n";
b_prompt += "Service: new internal/service/wiki_root.go method (a *App) wikiRoot. Wire in app.go Execute switch, operationRequiresWorkspaceResolution long nav case, recommendedWorkspaceCommand if needed.\n";
b_prompt += "CLI shouldUseDaemon in internal/cli/root.go: add \"nav.wiki-root\" to the false list with nav.governance.\n";
b_prompt += "Handler: resolve workspace, LoadProjectFile(root), if Canons empty => default item. Else ResolveCanons, filter by role payload. Items []model.WikiRootResolution. Backend \"wiki-root\". Ok true. Workspace alias. Do not set extra envelope keys.\n";
b_prompt += "If Validate/Resolve returns error, return that error (CLI wraps envelope). Messages actionable, include declared path.\n";
b_prompt += "Default governance_doc: if .docs/wiki/00_gobierno_documental.md exists OR no canons, use that relative path. For a canon, use declaredRoot + \"/00_gobierno_documental.md\" (slash).\n";
b_prompt += "Add WikiRootResolution to internal/model/types.go near WikiInventoryItem.\n";
b_prompt += "Service tests in wiki_root_test.go using t.TempDir sibling layout: parent/code + parent/wiki-repo/Ingenieria/00_gobierno_documental.md, write code/.mi-lsp/project.toml with [[canon]] root=\"../wiki-repo/Ingenieria\" id/role/mode, RegisterWorkspace, App.Execute nav.wiki-root, assert items wiki_root, resolved_from=canon.<id>, no absolute paths in items.\n";
b_prompt += "Second test: no [[canon]] => resolved_from=default, wiki_root=.docs/wiki, ok.\n";
b_prompt += "Third: --role missing match => error.\n";
b_prompt += "Do not change safeGovernanceSourceDoc yet (phase C). Do not add workspace link (phase D).\n";
b_prompt += "gofmt -w. go test ./internal/cli ./internal/service ./internal/workspace ./internal/model -count=1 -timeout 120s\n";
b_prompt += "Return {changed_paths, summary, blocked, blocker}.\n";

phase("B-wiki-root");
log("Phase B: nav wiki-root");
let b = agent(b_prompt, #{
    label: "B-wiki-root",
    capability_mode: "all",
    agent_type: "claudex-writer",
    output_schema: packet_schema,
});
if b == () || !b.success {
    await_user("verification", "Phase B writer failed. Inspect /workflows, repair, then resume.");
}
if b.output != () && b.output.blocked == true {
    await_user("verification", "Phase B reported blocked. Resume after unblocking.");
}

let c_prompt = common_rules();
c_prompt += locked_design();
c_prompt += "PHASE C. Use workspace.ResolveCanons / PathIsReadOnlyCanon from phase A. READ internal/docgraph/governance.go and governance_test.go before editing.\n";
c_prompt += "GOAL: a repo can have a single 00_gobierno_documental.md outside .docs/wiki/ and nav governance must pass without a duplicate human doc under .docs/wiki/. Honor governance.source_doc and governance.projection.output as declared.\n";
c_prompt += "TODAY (must change): safeGovernanceSourceDoc requires prefix .docs/wiki/ (governance.go:154-156). InspectGovernance hardcodes ProjectionDoc to .docs/wiki/_mi-lsp/read-model.toml (174). validateAndResolveGovernance requires projection.output EQUAL that constant (1070-1071). Writes always ProfilePath(root) (274). buildDocsReadProfileFromGovernance hardcodes SourceDoc wiki path (1216). ProfilePath/LoadProfile only look at .docs/wiki/_mi-lsp/read-model.toml (docgraph.go:132-216).\n";
c_prompt += "NEW safeGovernanceSourceDoc(root, sourceDoc, canons []ResolvedCanon) (string, bool):\n";
c_prompt += "  reject empty, '.', absolute.\n";
c_prompt += "  join+clean against workspace root.\n";
c_prompt += "  if Rel stays inside workspace => accept (ANY relative path, not just .docs/wiki/). Return slash rel.\n";
c_prompt += "  if Rel escapes: accept ONLY when abs candidate is inside some canon.AbsRoot (slash-prefix dir match). Return slash Rel from workspace (will start with ../). Else reject.\n";
c_prompt += "  Keep TestInspectGovernanceBlocksSourceDocOutsideWorkspace for random ../../outside.md NOT in a canon.\n";
c_prompt += "  ADD TestSafeGovernanceSourceDocAllowsInWorkspaceNonWiki for Ingenieria/00_gobierno_documental.md.\n";
c_prompt += "  ADD TestSafeGovernanceSourceDocAllowsDeclaredCanonRoot for ../wiki-repo/Ingenieria/00_....md with matching [[canon]].\n";
c_prompt += "projection.output: new safeProjectionOutput(root, output, canons) (string, bool). Relative, in-workspace only. Must NOT resolve inside a read-only canon AbsRoot (foreign). Empty => .docs/wiki/_mi-lsp/read-model.toml (compat). Replace the equality check in validateAndResolveGovernance with this (still require format=toml, auto_sync, versioned as today).\n";
c_prompt += "InspectGovernance: ProjectionDoc = safe declared output, not hardcoded. Write rendered read-model to that in-workspace path. If declared output would be inside a canon root, do not write there; set issue/warning canon_root_read_only and write to the default in-workspace path OR block auto_sync — fail closed with an actionable issue, do not write foreign trees.\n";
c_prompt += "LoadProfile/ProfilePath: add DiscoverProfilePath(root) string. Order: (1) .docs/wiki/_mi-lsp/read-model.toml if exists (compat). (2) for each ResolveCanons role=producto then others, if <AbsRoot>/_mi-lsp/read-model.toml exists, READ it (never write). (3) if in-workspace Ingenieria/_mi-lsp/read-model.toml exists, use it. Else default path (may be missing => DefaultProfile).\n";
c_prompt += "resolveGovernanceDoc: default still .docs/wiki/00_gobierno_documental.md if that file exists. Else well-known in-workspace candidates: Ingenieria/00_gobierno_documental.md, wiki/00_gobierno_documental.md, docs/00_gobierno_documental.md. If several besides missing default, fail closed (INVALID + issue listing them). If one, use it. Then if [[canon]] and still missing, <canon.DeclaredRoot>/00_gobierno_documental.md. Then honor profile source_doc via safeGovernanceSourceDoc.\n";
c_prompt += "buildDocsReadProfileFromGovernance SourceDoc must be the resolved human doc relative path, not always .docs/wiki/00_....\n";
c_prompt += "Update invalid-source issue text: no longer \"must be under .docs/wiki/\"; say it must be relative, in-workspace or inside a declared [[canon]] root, and not absolute.\n";
c_prompt += "Do not rewrite route.go default globs in this phase except if a test fails solely because of the prefix gate. Keep DefaultProfile families on .docs/wiki for compat.\n";
c_prompt += "CT-GOVERNANCE-AE-KERNEL-V2.md governed_boundary currently says source_doc must remain below .docs/wiki/ — DO NOT edit wiki in this phase (docs phase will). Code must implement the new rule; leave the wiki file for the docs agent.\n";
c_prompt += "gofmt -w. go test ./internal/docgraph ./internal/service ./internal/workspace -count=1 -timeout 180s\n";
c_prompt += "Return {changed_paths, summary, blocked, blocker}.\n";

phase("C-governance");
log("Phase C: decouple governance from .docs/wiki");
let c = agent(c_prompt, #{
    label: "C-governance",
    capability_mode: "all",
    agent_type: "claudex-writer",
    output_schema: packet_schema,
});
if c == () || !c.success {
    await_user("verification", "Phase C writer failed. Inspect /workflows, repair, then resume.");
}
if c.output != () && c.output.blocked == true {
    await_user("verification", "Phase C reported blocked. Resume after unblocking.");
}

let d_prompt = common_rules();
d_prompt += locked_design();
d_prompt += "PHASE D. READ canon.go, wiki_root.go, governance.go as they exist after A-C.\n";
d_prompt += "D1) mi-lsp workspace link <alias-canon> --role producto\n";
d_prompt += "CLI in internal/cli/workspace.go: Use \"link <alias>\", required --role, operation workspace.link, payload {alias, role}. Add to AddCommand list. Tests for command existence and --role required.\n";
d_prompt += "Schema: model.WorkspaceCanonLink { Alias string `toml:\"alias\" json:\"alias\"`, Role string `toml:\"role\" json:\"role\"` }. Add to WorkspaceRegistration as CanonLinks []WorkspaceCanonLink `toml:\"canon_link,omitempty\" json:\"canon_links,omitempty\"`.\n";
d_prompt += "Register: current workspace (resolved from cwd/--workspace) stores a link to the target alias. Target alias MUST already be in the registry. Role enum same as canon roles. Duplicate (alias+role) is idempotent success. Replace same-role different-alias (one link per role). Persist via existing registry save path. Never write absolute roots into registry beyond the existing Root field of the target workspace.\n";
d_prompt += "nav.wiki-root resolution order: (1) [[canon]] in current project.toml => resolved_from=canon.<id> (2) registry CanonLinks on CURRENT registration => resolved_from=registry.link, wiki_root from the TARGET workspace (its [[canon]] else its .docs/wiki), workspace field = current alias, include id=target alias (3) default. --role applies at every step. Linking does not require --allow-cross-workspace.\n";
d_prompt += "normalizeWorkspaceRequest: if harness client would refuse cross-workspace, allow when the selected alias is a CanonLink of the cwd workspace (any role or matching). Still emit a warning that a registry canon link was used. Do not globally disable the gate.\n";
d_prompt += "nav governance on a code workspace with [[canon]] or a producto link should inspect the product canon governance (read-only) so a code repo without local 00_gobierno still works. Reuse InspectGovernance on the resolved abs canon/workspace root for reading; never auto_sync-write into that foreign root (pass autoSync=false when the target is a read-only canon OR when PathIsReadOnlyCanon).\n";
d_prompt += "D2) index --docs-only walks declared [[canon]] roots.\n";
d_prompt += "Hook: after collectDocCandidates(workspace root) inside IndexWorkspaceDocsWithSourcesWithProgressPrior (docgraph.go), also walk each ResolveCanons AbsRoot for .md files. Skip symlink dirs/files (DirEntry Type ModeSymlink => SkipDir/skip). Store DocRecord path as slash Rel(workspaceRoot, abs) which will look like ../wiki-repo/Ingenieria/foo.md. Do not follow junctions. Do not write into those trees.\n";
d_prompt += "Invalid [[canon]] during docs-only: add a warning and skip that root (do not fail the whole index of the code tree) UNLESS the operator passed a mode that requires canons — fail-closed on that root, keep local docs. Document in warning text.\n";
d_prompt += "extend isCanonicalGraphDoc OR leave graph optional; AC is that docs are in the docs index and nav wiki search can see them. Prefer including ../ prefix / paths under canon abs as indexable docs. Test with temp sibling repos, IndexWorkspaceDocsOnly, then store.ListDocRecords containing the external relative path.\n";
d_prompt += "Do not change default ignore of .git. Do not index the foreign repo as code catalog (WalkWorkspace stays on workspace root).\n";
d_prompt += "gofmt -w. go test ./internal/cli ./internal/workspace ./internal/service ./internal/docgraph ./internal/indexer -count=1 -timeout 180s\n";
d_prompt += "Return {changed_paths, summary, blocked, blocker}.\n";

phase("D-federation");
log("Phase D: workspace link and docs-only canon walk");
let d = agent(d_prompt, #{
    label: "D-federation",
    capability_mode: "all",
    agent_type: "claudex-writer",
    output_schema: packet_schema,
});
if d == () || !d.success {
    await_user("verification", "Phase D writer failed. Inspect /workflows, repair, then resume.");
}
if d.output != () && d.output.blocked == true {
    await_user("verification", "Phase D reported blocked. Resume after unblocking.");
}

let docs_prompt = common_rules();
docs_prompt += locked_design();
docs_prompt += "PHASE DOCS. Spanish for wiki; English for CHANGELOG and skills/mi-lsp/SKILL.md. Read existing RF/CT frontmatter and copy the pattern.\n";
docs_prompt += "1) CHANGELOG.md [Unreleased] Added: [[canon]] in project.toml; mi-lsp nav wiki-root; governance source_doc/projection.output honored outside .docs/wiki when in-workspace or declared canon; workspace link --role; index --docs-only walks [[canon]]. Notes: protocol remains mi-lsp-v1.1; new command only; no [[canon]] is exact compat; absolute/escape/symlink fail closed; read-only canon roots cannot be written.\n";
docs_prompt += "2) New RF-WKS-008 .docs/wiki/04_RF/RF-WKS-008.md : declare [[canon]] external roots. New RF-WIKI-007 .docs/wiki/04_RF/RF-WIKI-007.md : nav wiki-root. New CT-NAV-WIKI-ROOT .docs/wiki/09_contratos/CT-NAV-WIKI-ROOT.md for the command envelope items. Follow RF-WIKI-006 / CT-NAV-GOVERNANCE YAML headers (Spanish body, exact command names).\n";
docs_prompt += "3) Update .docs/wiki/04_RF.md catalog table with the new RFs. Update .docs/wiki/09_contratos_tecnicos.md to mention the new CT and wiki-root. Update .docs/wiki/02_arquitectura.md topology bullet: project.toml may declare canon[] resolved vs workspace root. Update .docs/wiki/01_alcance_funcional.md one line under discovery/federation.\n";
docs_prompt += "4) Update CT-GOVERNANCE-AE-KERNEL-V2.md block path-safety governed_boundary: source_doc must be relative and either inside the workspace or inside a declared [[canon]] root; still reject absolute, extra parent escape, and symlinks. projection.output must stay inside the code workspace and never a read-only foreign canon root.\n";
docs_prompt += "5) Update CT-NAV-GOVERNANCE.md: human_doc/projection_doc follow declared paths; auto_sync will not write a read-only canon root.\n";
docs_prompt += "6) Update 05_modelo_datos.md ProjectFile / WorkspaceRegistration with Canons, CanonPolicy, CanonLinks if those entities are listed there. Keep it short.\n";
docs_prompt += "7) skills/mi-lsp/SKILL.md: add nav wiki-root (and nav wiki root alias) with --role, --format toon, resolved_from semantics so consumers stop hardcoding .docs/wiki. English, terse.\n";
docs_prompt += "8) If 06_matriz_pruebas_RF.md or TP-WIKI/TP-WKS list RFs, add a row. Do not invent Linear ids. Preserve Spanish accents.\n";
docs_prompt += "Do not change Go except if a comment in code is the only remaining .docs/wiki lie in a user-facing error you already own — prefer wiki.\n";
docs_prompt += "Return {changed_paths, summary, blocked, blocker}.\n";

phase("docs");
log("Phase docs: changelog, wiki, skill");
let docs = agent(docs_prompt, #{
    label: "docs-wiki-changelog",
    capability_mode: "all",
    agent_type: "ps-docs",
    output_schema: packet_schema,
});
if docs == () || !docs.success {
    await_user("verification", "Docs writer failed. Inspect /workflows, repair, then resume.");
}

let v_prompt = "";
v_prompt += "You are an adversarial verifier in C:/repos/mios/mi-lsp. Independent of the writers. READ the code and RUN tests. Do not trust writer summaries.\n";
v_prompt += "Acceptance:\n";
v_prompt += "1) Workspace with [[canon]] root=\"../wiki-repo/Ingenieria\" => nav wiki-root returns that relative path; no absolute paths in items; resolved_from=canon.<id>.\n";
v_prompt += "2) No [[canon]] => works, resolved_from=default.\n";
v_prompt += "3) Absolute or escape>max => validation error, actionable message.\n";
v_prompt += "4) index --docs-only indexes the external tree and nav can see those docs.\n";
v_prompt += "5) Unique 00_gobierno_documental.md outside .docs/wiki/ can pass nav governance without a duplicate human doc under .docs/wiki/.\n";
v_prompt += "6) Tests exist for: relative resolve, reject absolute, reject escape, symlink in path, absence of [[canon]].\n";
v_prompt += "Also: read-only canon is enforced on writes; protocol still mi-lsp-v1.1; Envelope type unchanged; project.toml without canon is behavior-preserving.\n";
v_prompt += "Commands to run (PowerShell, repo root):\n";
v_prompt += "  gofmt -l internal/workspace/canon.go internal/service/wiki_root.go internal/cli/nav.go internal/docgraph/governance.go 2>$null\n";
v_prompt += "  go test ./internal/workspace ./internal/cli ./internal/service ./internal/docgraph ./internal/indexer ./internal/model -count=1 -timeout 300s\n";
v_prompt += "If packages or files are named slightly differently, discover with grep and still run the tests.\n";
v_prompt += "Write a failing-closed verdict: pass=true only with command evidence. List concrete failures.\n";
v_prompt += "If tests fail due to missing implementation, set pass=false and quote the missing pieces. Do not implement fixes unless a one-line compile error is obvious AND you can prove tests after; prefer reporting.\n";
v_prompt += "Return {pass, reason, evidence, failures}.\n";

phase("verify");
log("Adversarial verify of acceptance criteria");
let v = agent(v_prompt, #{
    label: "verify-acceptance",
    capability_mode: "all",
    agent_type: "claudex-verifier",
    output_schema: verdict_schema,
});

let pass = false;
let reason = "verifier did not return a verdict";
let evidence = "";
let failures = [];
if v != () && v.success && v.output != () {
    if v.output.pass == true { pass = true; }
    if v.output.reason != () { reason = v.output.reason; }
    if v.output.evidence != () { evidence = v.output.evidence; }
    if v.output.failures != () { failures = v.output.failures; }
}

let report = "";
report += "# canon-wiki-roots result\n\n";
report += "pass: ";
if pass { report += "true\n"; } else { report += "false\n"; }
report += "reason: " + reason + "\n";
report += "evidence: " + evidence + "\n";
if failures.len() > 0 {
    report += "failures:\n";
    for f in failures {
        report += "- " + f + "\n";
    }
}
let report_path = write_scratch_file("canon-wiki-roots.md", report);
complete(#{
    pass: pass,
    reason: reason,
    evidence: evidence,
    report_path: report_path,
});
