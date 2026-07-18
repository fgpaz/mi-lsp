"""Revision-pinned, dependency-free graph fixture extractor."""
from pathlib import Path
import re
GRAPHIFY_REVISION = "victory-graphify-v1"
DEFAULT_EXTENSIONS = {".cs":"csharp", ".go":"go", ".ts":"typescript", ".tsx":"typescript", ".py":"python", ".md":"wiki"}
def _names(text, kind):
    patterns={"csharp":[r"\b(?:class|interface|record|struct|enum)\s+(\w+)",r"\b(?:public|private|internal|protected)\s+(?:static\s+)?[\w<>,\[\]?]+\s+(\w+)\s*\("],"go":[r"\btype\s+(\w+)",r"\bfunc\s+(?:\([^)]*\)\s+)?(\w+)\s*\("],"typescript":[r"\b(?:export\s+)?(?:class|interface|type|enum)\s+(\w+)",r"\b(?:export\s+)?function\s+(\w+)",r"^\s*(\w+)\s*\([^)]*\)\s*[:{]"],"python":[r"^\s*class\s+(\w+)",r"^\s*def\s+(\w+)\s*\("],"wiki":[r"^#{1,6}\s+(.+?)\s*$"],"extension":[r"^\s*symbol:\s*(\S.*?)\s*$"]}
    out=[]
    for pattern in patterns.get(kind,[]): out.extend(re.findall(pattern,text,re.M))
    return sorted(set(out),key=lambda x:(x.casefold(),x))
def extract(paths, extension_map=None):
    kinds=dict(DEFAULT_EXTENSIONS); kinds.update(extension_map or {}); symbols=[]
    for path in sorted((Path(p) for p in paths),key=lambda p:p.as_posix()):
        kind=kinds.get(path.suffix.lower())
        if not kind: continue
        text=path.read_text(encoding="utf-8")
        for name in _names(text,kind): symbols.append({"id":f"{path.as_posix()}#{name}","name":name,"path":path.as_posix(),"kind":kind})
    symbols.sort(key=lambda x:(x["name"].casefold(),x["name"],x["path"]))
    by_name={}
    for symbol in symbols: by_name.setdefault(symbol["name"],[]).append(symbol)
    edges=[]; ambiguous=[]
    for source in sorted(symbols,key=lambda x:x["id"]):
        text=Path(source["path"]).read_text(encoding="utf-8")
        for target in sorted(by_name,key=lambda x:(x.casefold(),x)):
            if target==source["name"] or not re.search(r"\b"+re.escape(target)+r"\b",text): continue
            candidates=by_name[target]
            if len(candidates)>1: ambiguous.append({"from":source["id"],"name":target,"candidates":[x["id"] for x in candidates]})
            else: edges.append({"from":source["id"],"to":candidates[0]["id"]})
    return {"graphify_revision":GRAPHIFY_REVISION,"symbols":symbols,"edges":sorted(edges,key=lambda x:(x["from"],x["to"])),"diagnostics":{"ambiguous":sorted(ambiguous,key=lambda x:(x["from"],x["name"])),"unresolved":[]}}
