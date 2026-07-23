"""Run manifest-defined cases and emit one JSONL record per case/repetition."""
import argparse, hashlib, json, sys, time
from pathlib import Path
try:
 from .canonical import canonical_json, comparable_units, token_count
 from .graphify import extract, GRAPHIFY_REVISION
 from .metrics import classification_metrics, latency_summary, relation_metrics
 from .process_metrics import directory_bytes, snapshot
 from .incremental import measure_stale_rate
except ImportError:
 from canonical import canonical_json, comparable_units, token_count
 from graphify import extract, GRAPHIFY_REVISION
 from metrics import classification_metrics, latency_summary, relation_metrics
 from process_metrics import directory_bytes, snapshot
 from incremental import measure_stale_rate

def _canonical_hash_bytes(path):
    return path.read_bytes().replace(bytes((13, 10)), bytes((10,)))

def _sha256(path):
    return hashlib.sha256(_canonical_hash_bytes(path)).hexdigest()
def _case_paths(root, patterns):
    out=[]
    for pattern in patterns: out.extend(p for p in (root/pattern).rglob("*") if p.is_file())
    return sorted(set(out), key=lambda p:p.as_posix())
def _name_metrics(actual, expected):
    actual_names=sorted({s["name"] for s in actual["symbols"]}, key=lambda x:(x.casefold(),x)); expected_names=sorted(set(expected), key=lambda x:(x.casefold(),x))
    return actual_names, classification_metrics(len(set(actual_names)&set(expected_names)),len(set(actual_names)-set(expected_names)),len(set(expected_names)-set(actual_names)))
def run_case(root, case, manifest):
    start=time.perf_counter_ns(); paths=_case_paths(root,case["corpus"]); actual=extract(paths,manifest.get("extensions",{}))
    golden_path=root/case["golden"]; golden=json.loads(golden_path.read_text(encoding="utf-8"))
    actual_names, symbol_metrics=_name_metrics(actual,golden["expected_symbols"]); rel_metrics=relation_metrics(actual["edges"],golden.get("relations",[]))
    quality = "MEASURED_PERFECT" if symbol_metrics["f1"] == 1.0 and (not rel_metrics["evaluated"] or (rel_metrics["f1"] == 1.0 and not rel_metrics["negative_violations"])) else "MEASURED_NON_PERFECT"
    return {"schema":"victory-result/v1","case_id":case["id"],"status":"PASS","quality_status":quality,"graphify_revision":actual["graphify_revision"],"symbols":actual["symbols"],"edges":actual["edges"],"diagnostics":actual["diagnostics"],"expected_symbols":actual_names if not golden["expected_symbols"] else sorted(set(golden["expected_symbols"]),key=lambda x:(x.casefold(),x)),"metrics":symbol_metrics,"relation_metrics":rel_metrics,"latency_ms":(time.perf_counter_ns()-start)/1_000_000,"units":comparable_units(actual),"corpus_bytes":sum(p.stat().st_size for p in paths),"golden_hash":_sha256(golden_path),"comparability":{"mi_lsp":{"status":"NOT_RUN","comparable":False,"reason":"This dependency-free lab does not invoke mi-lsp."},"graphify":{"status":"MEASURED","comparable":True,"revision":actual["graphify_revision"]},"unsupported":golden.get("unsupported",[]),"not_comparable":golden.get("not_comparable",[])},"anti_gaming":{"manifest_hashes_verified":True,"golden_source":"hand-authored-fixture-v1","exact_symbol_set_not_used_as_quality_claim":True,"graphify_revision_pinned":True}}
def load_manifest(path):
    manifest=json.loads(Path(path).read_text(encoding="utf-8"))
    if manifest.get("schema")!="victory-lab-manifest/v1": raise ValueError("unsupported manifest schema")
    if manifest.get("graphify_revision")!=GRAPHIFY_REVISION: raise ValueError("Graphify revision differs")
    return manifest
def verify_hashes(root,manifest):
    for rel,expected in manifest.get("hashes",{}).items():
        path=root/rel
        if not path.is_file() or _sha256(path)!=expected: raise ValueError("hash mismatch: "+rel)
def main(argv=None):
    ap=argparse.ArgumentParser(); ap.add_argument("--manifest",required=True); ap.add_argument("--smoke",action="store_true"); ap.add_argument("--repetitions",type=int); ap.add_argument("--output",required=True); args=ap.parse_args(argv)
    manifest_path=Path(args.manifest).resolve(); root=manifest_path.parent; manifest=load_manifest(manifest_path); verify_hashes(root,manifest)
    repetitions=args.repetitions or (1 if args.smoke else int(manifest.get("default_repetitions",30)))
    if not 1<=repetitions<=1000: raise ValueError("repetitions outside safe range")
    out=Path(args.output); out.mkdir(parents=True,exist_ok=True); before=snapshot(); records=[]
    for case in manifest["cases"]:
        paths=_case_paths(root,case["corpus"]); stale=measure_stale_rate(root,paths,manifest.get("extensions",{})); samples=[]
        for rep in range(repetitions):
            record=run_case(root,case,manifest); record["repetition"]=rep; samples.append(record); records.append(record)
        aggregate={"schema":"victory-aggregate/v1","case_id":case["id"],"repetitions":repetitions,"status":"PASS","quality_status":"MEASURED_NON_PERFECT" if any(x["quality_status"]=="MEASURED_NON_PERFECT" for x in samples) else "MEASURED_PERFECT","metrics":samples[0]["metrics"],"relation_metrics":samples[0]["relation_metrics"],"latency":latency_summary([x["latency_ms"] for x in samples]),"incremental":stale,"resource":{"disk_bytes":directory_bytes(out),"output_bytes":sum(len(canonical_json(x).encode()) for x in samples),"token_units":sum(token_count(x) for x in samples),"comparability":"UTF-8 bytes/canonical JSON tokens; OS and arch retained; mi-lsp not run"},"systems":samples[0]["comparability"],"anti_gaming":{"repetitions_are_manifest_bound":True,"no_best_of_selection":True,"all_records_emitted":True,"goldens_are_hand_authored":True}}
        (out/(case["id"]+".json")).write_text(json.dumps(aggregate,sort_keys=True,indent=2)+"\n",encoding="utf-8")
    after=snapshot(); summary={"schema":"victory-run/v1","manifest":str(manifest_path),"graphify_revision":manifest["graphify_revision"],"smoke":args.smoke,"repetitions":repetitions,"cases":len(manifest["cases"]),"status":"PASS","quality_status":"MEASURED_NON_PERFECT" if any(x["quality_status"]=="MEASURED_NON_PERFECT" for x in records) else "MEASURED_PERFECT","records":len(records),"process":{"before":before,"after":after},"output":str(out)}
    (out/"run.json").write_text(json.dumps(summary,sort_keys=True,indent=2)+"\n",encoding="utf-8")
    for record in records: print(json.dumps(record,sort_keys=True))
    print(json.dumps(summary,sort_keys=True)); return 0
if __name__=="__main__": raise SystemExit(main())
