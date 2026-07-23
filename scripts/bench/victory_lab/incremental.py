"""Deterministic incremental freshness measurement for Victory Lab."""
from __future__ import annotations
import hashlib, json, shutil, tempfile
from pathlib import Path
from typing import Iterable
try:
    from .graphify import extract
except ImportError:
    from graphify import extract

def _file_state(root: Path) -> dict[str, str]:
    return {p.relative_to(root).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((x for x in root.rglob("*") if x.is_file()), key=lambda x: x.as_posix())}

def _copy_inputs(root: Path, paths: Iterable[Path], destination: Path) -> list[Path]:
    copied = []
    for source in sorted(paths, key=lambda p: p.as_posix()):
        target = destination / source.relative_to(root); target.parent.mkdir(parents=True, exist_ok=True); shutil.copyfile(source, target); copied.append(target)
    return copied

def _stable_item(item: dict) -> dict:
    value = dict(item)
    for key in ("id", "from", "to", "path"):
        if key in value:
            text = value[key].replace("\\", "/")
            for marker in ("/fixture/", "/clean/"):
                if marker in text:
                    text = text.split(marker, 1)[1]
            value[key] = text
    return value

def _graph_records(graph: dict) -> set[str]:
    records = {"symbol:" + json.dumps(_stable_item(item), sort_keys=True) for item in graph["symbols"]}
    records.update("edge:" + json.dumps(_stable_item(item), sort_keys=True) for item in graph["edges"])
    return records

def _append_fixture_marker(path: Path) -> None:
    marker = "\n## Changed Fixture Marker\n" if path.suffix.lower() == ".md" else "\n# Changed Fixture Marker\n"
    path.write_text(path.read_text(encoding="utf-8") + marker, encoding="utf-8")

def measure_stale_rate(root: Path, paths: Iterable[Path], extension_map: dict | None = None) -> dict:
    """Compare a mutated graph with a separate clean full rebuild."""
    source_paths = list(paths)
    with tempfile.TemporaryDirectory(prefix="victory-stale-") as temporary:
        fixture, clean = Path(temporary) / "fixture", Path(temporary) / "clean"
        fixture.mkdir(); copied = _copy_inputs(root, source_paths, fixture)
        deleted_seed = fixture / "_victory_deleted.py"; deleted_seed.write_text("class DeletedFixture:\n    pass\n", encoding="utf-8")
        renamed_seed = fixture / "_victory_renamed.py"; renamed_seed.write_text("class RenamedFixture:\n    pass\n", encoding="utf-8")
        initial_state = _file_state(fixture)
        initial_graph = extract([p for p in fixture.rglob("*") if p.is_file()], extension_map)
        (fixture / "_victory_created.py").write_text("class CreatedFixture:\n    pass\n", encoding="utf-8")
        _append_fixture_marker(copied[0] if copied else deleted_seed); deleted_seed.unlink(); renamed_seed.rename(fixture / "_victory_renamed_after.py")
        mutated_state = _file_state(fixture); mutated_paths = [p for p in fixture.rglob("*") if p.is_file()]
        incremental_graph = extract(mutated_paths, extension_map)
        clean.mkdir(); clean_graph = extract(_copy_inputs(fixture, mutated_paths, clean), extension_map)
        incremental_records, clean_records = _graph_records(incremental_graph), _graph_records(clean_graph)
        stale_records = sorted(incremental_records ^ clean_records); comparable_records = max(1, len(clean_records))
        changed_inputs = sorted(r for r in set(initial_state) | set(mutated_state) if initial_state.get(r) != mutated_state.get(r))
        return {"method": "full-initial/mutated-incremental/clean-equivalent-v1", "operations": ["create", "change", "delete", "rename"], "initial_files": len(initial_state), "mutated_files": len(mutated_state), "changed_inputs": changed_inputs, "changed_input_count": len(changed_inputs), "initial_graph_records": len(_graph_records(initial_graph)), "changed_graph_records": len(_graph_records(initial_graph) ^ clean_records), "stale_records": stale_records, "stale_record_count": len(stale_records), "comparable_records": comparable_records, "stale_rate": len(stale_records) / comparable_records, "states_differ": initial_state != mutated_state, "clean_equivalent": incremental_records == clean_records}
