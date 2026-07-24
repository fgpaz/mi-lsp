from pathlib import Path
import re

path = Path(r"C:/wt/milsp-ae-v2-only/internal/docgraph/governance.go")
text = path.read_text(encoding="utf-8")

start = text.find("func inspectAECanonFromProjection")
end = text.find("func rejectedLegacyAECanon")
if start < 0 or end < 0:
    raise SystemExit(f"markers not found start={start} end={end}")

replacement = '''func inspectAECanonFromProjection(root string, reason string) model.AECanonStatus {
	profile, source, _ := LoadProfile(root)
	if source == "project" {
		if strings.TrimSpace(profile.Governance.AECanon.Mode) != "" {
			status := inspectConfiguredAECanon(root, profile.Governance.AECanon, profile.Governance.Hierarchy, "read_model")
			status.Status = "projection_only"
			// A stale projection may describe AE while the human governance source is being repaired.
			// Report the projected contract without making that secondary source block repair.
			status.Blocking = false
			status.Reason = reason + "_read_model_projection_only"
			return status
		}
		roots := declaredAECanonRoots(profile.Governance.Hierarchy)
		if len(roots) > 0 {
			status := rejectedLegacyAECanon("read_model", roots)
			status.Status = "projection_only"
			// A stale projection may describe the removed repo-local AE canon. Report it
			// for repair, but do not let the secondary source block governance repair.
			status.Blocking = false
			status.Reason = reason + "_legacy_read_model_projection_only"
			return status
		}
	}
	return model.AECanonStatus{
		Status:   "not_applicable",
		Source:   "none",
		Blocking: false,
		Reason:   reason + "_not_declared",
	}
}

func inspectConfiguredAECanon(root string, config model.GovernanceAECanon, hierarchy []model.GovernanceHierarchyItem, source string) model.AECanonStatus {
	mode := strings.ToLower(strings.TrimSpace(config.Mode))
	switch mode {
	case "":
		if roots := declaredAECanonRoots(hierarchy); len(roots) > 0 {
			return rejectedLegacyAECanon(source, roots)
		}
		return model.AECanonStatus{
			Status:   "not_applicable",
			Source:   "none",
			Blocking: false,
			Reason:   "ae_canon_not_declared",
		}
	case "kernel_v2":
		return inspectKernelV2AECanon(root, config)
	default:
		return model.AECanonStatus{
			Status:         "mismatch",
			Source:         source,
			Blocking:       true,
			Reason:         "ae_canon_mode_invalid",
			MissingModules: []string{"ae_canon.mode must be kernel_v2"},
		}
	}
}

'''

text = text[:start] + replacement + text[end:]


def drop_func(src: str, name: str) -> str:
    m = re.search(rf"^func {name}\b", src, flags=re.M)
    if not m:
        return src
    n = re.search(r"^func ", src[m.end() :], flags=re.M)
    if not n:
        return src[: m.start()]
    end_i = m.end() + n.start()
    return src[: m.start()] + src[end_i:]


for name in [
    "inspectAECanonRoots",
    "inspectAECanonRoot",
    "aeCanonRedirectRoot",
    "containsAEPathMention",
]:
    text = drop_func(text, name)

# Drop requiredAECanonModules if unused.
if "requiredAECanonModules" in text and text.count("requiredAECanonModules") == 1:
    text = re.sub(
        r"var requiredAECanonModules = \[\]string\{[\s\S]*?\}\n\n",
        "",
        text,
        count=1,
    )

# Drop directoryExists if unused after removing fallback.
if "directoryExists(" not in text.replace("func directoryExists", ""):
    text = drop_func(text, "directoryExists")

path.write_text(text, encoding="utf-8", newline="\n")
print("patched", path)
print("requiredAECanonModules count", text.count("requiredAECanonModules"))
print("has inspectAECanonRoots", "func inspectAECanonRoots" in text)
print("has inspectAECanonFromHierarchy", "func inspectAECanonFromHierarchy" in text)
print("has directoryExists", "func directoryExists" in text)
