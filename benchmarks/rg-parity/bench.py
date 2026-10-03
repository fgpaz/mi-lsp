#!/usr/bin/env python3
"""Benchmark de paridad mi-lsp vs ripgrep (definition, refs, intent, literal).

Solo stdlib. Ver README.md. Las consultas son de solo lectura: nunca ejecuta
`mi-lsp index`.
"""
import argparse
import json
import os
import re
import statistics
import subprocess
import sys
import time
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
TIMEOUT_S = 30
RUNS = 3
TOP_K = 5
MAX_STORED_ITEMS = 200
FILE_KEYS = ("file", "file_path", "path", "doc_path")
STOPWORDS = {
    "where", "what", "when", "which", "does", "this", "that", "with", "from", "into",
    "how", "the", "and", "for", "are", "was", "is", "it", "its", "by", "in", "of", "to",
    "a", "an", "or", "on", "when", "than", "then", "there", "their", "them", "other",
}


def run(cmd, cwd, env):
    """Ejecuta un comando; devuelve dict con rc, stdout (bytes), ms, timeout."""
    start = time.perf_counter()
    try:
        proc = subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, stdin=subprocess.DEVNULL, timeout=TIMEOUT_S)
        rc, out, timed_out = proc.returncode, proc.stdout, False
    except subprocess.TimeoutExpired as exc:
        rc, out, timed_out = None, exc.stdout or b"", True
    ms = (time.perf_counter() - start) * 1000.0
    return {"rc": rc, "stdout": out, "ms": ms, "timeout": timed_out}


def median_run(cmd, cwd, env, runs=RUNS):
    """Corre `runs` veces; devuelve la primera salida con la latencia mediana."""
    results = []
    for _ in range(runs):
        results.append(run(cmd, cwd, env))
        if results[-1]["timeout"]:
            break
    first = results[0]
    first["ms"] = statistics.median(r["ms"] for r in results)
    first["any_timeout"] = any(r["timeout"] for r in results)
    return first


def norm_file(value, root):
    if not isinstance(value, str) or not value:
        return None
    value = value.replace("\\", "/")
    root_prefix = str(root).replace("\\", "/").rstrip("/") + "/"
    if root and value.startswith(root_prefix):
        value = value[len(root_prefix):]
    while value.startswith("./"):
        value = value[2:]
    return value


def item_line(item):
    for key in ("line", "start_line", "line_start"):
        if isinstance(item.get(key), int) and item[key] > 0:
            return item[key]
    rng = item.get("range")
    if isinstance(rng, dict):
        start = rng.get("start")
        if isinstance(start, dict) and isinstance(start.get("line"), int):
            return start["line"] + 1
    return None


def parse_milsp(stdout, root):
    """Devuelve (ok, error_code, items[(file, line)], truncated, backend)."""
    text = stdout.decode("utf-8", "replace")
    data = None
    try:
        data = json.loads(text)
    except ValueError:
        idx = text.find("{")
        if idx >= 0:
            try:
                data = json.loads(text[idx:])
            except ValueError:
                data = None
    if not isinstance(data, dict):
        return False, "invalid_json", [], False, ""
    items = []
    for item in data.get("items") or []:
        if not isinstance(item, dict):
            continue
        file = None
        for key in FILE_KEYS:
            file = norm_file(item.get(key), root)
            if file:
                break
        if file:
            items.append((file, item_line(item)))
    ok = bool(data.get("ok", False))
    error_code = None
    if not ok:
        err = data.get("error")
        error_code = (err.get("code") if isinstance(err, dict) else None) or "ok_false"
    return ok, error_code, items, bool(data.get("truncated")), str(data.get("backend") or "")


def parse_rg_lines(stdout):
    pairs = []
    for line in stdout.decode("utf-8", "replace").splitlines():
        m = re.match(r"^(.*?):(\d+):", line)
        if m:
            pairs.append((norm_file(m.group(1), ""), int(m.group(2))))
    return pairs


def rg_base(globs):
    cmd = ["rg", "--no-heading", "--with-filename"]
    for glob in globs:
        cmd += ["-g", glob]
    return cmd


def build_rg(case, repo):
    qtype, query = case["type"], case["query"]
    base = rg_base(repo["globs"])
    if qtype == "definition":
        regex = repo["def_regex"].replace("{sym}", re.escape(query))
        return base + ["-n", "-e", regex, "."]
    if qtype == "refs":
        return base + ["-n", "-w", "-F", "-e", query, "."]
    if qtype == "literal":
        return base + ["-n", "-F", "-e", query, "."]
    terms = [w for w in re.findall(r"[A-Za-z0-9_]+", query.lower()) if len(w) >= 4 and w not in STOPWORDS]
    cmd = base + ["-i", "-c"]
    for term in terms:
        cmd += ["-e", term]
    return cmd + ["."]


def build_milsp(binary, case, alias, fmt):
    qtype, query = case["type"], case["query"]
    sub = {"definition": ["nav", "find", query, "--exact"], "refs": ["nav", "refs", query],
           "literal": ["nav", "search", query], "intent": ["nav", "intent", query]}[qtype]
    cmd = [binary] + sub + ["--workspace", alias]
    # BENCH_MILSP_EXTRA_ARGS (p. ej. "--no-daemon") mide el binario candidato
    # sin depender del daemon global, que puede correr otra versión.
    cmd += os.environ.get("BENCH_MILSP_EXTRA_ARGS", "").split()
    if fmt:
        cmd += ["--format", fmt]
    return cmd


def rg_intent_top(stdout):
    """`rg -c` -> archivos ordenados por cantidad de coincidencias."""
    counts = []
    for line in stdout.decode("utf-8", "replace").splitlines():
        file, sep, num = line.rpartition(":")
        if sep and num.isdigit():
            counts.append((norm_file(file, ""), int(num)))
    counts.sort(key=lambda fc: (-fc[1], fc[0]))
    return [file for file, _ in counts]


def safe_cmd(cmd, binary):
    return ["mi-lsp" if part == binary else part for part in cmd]


def round_or_none(value, digits=4):
    return None if value is None else round(value, digits)


def run_case(case, repos, repos_root, binary, label):
    repo = repos[case["repo"]]
    root = (Path(repos_root) / repo["root"]).resolve()
    alias = case["repo"]
    env = dict(os.environ, MI_LSP_CLIENT_NAME="bench", MI_LSP_SESSION_ID="bench-" + label)
    qtype = case["type"]
    result = {"id": case["id"], "repo": alias, "language": repo["language"], "type": qtype, "query": case["query"]}

    # --- ripgrep ---
    rg_cmd = build_rg(case, repo)
    rg = median_run(rg_cmd, root, env)
    result["rg"] = {"cmd": rg_cmd, "bytes": len(rg["stdout"]), "ms": round(rg["ms"], 1), "rc": rg["rc"]}
    if qtype == "intent":
        top = rg_intent_top(rg["stdout"])
        expected = set(case["expected_files"])
        result["rg"]["top_files"] = top[:TOP_K]
        result["rg"]["hit_at_5"] = bool(expected & set(top[:TOP_K]))
        oracle = None
    else:
        oracle = set(parse_rg_lines(rg["stdout"]))
        result["rg"]["items"] = len(oracle)
        result["oracle_size"] = len(oracle)

    # --- mi-lsp: json (puntuacion + latencia), compact y default (solo bytes) ---
    cmd_json = build_milsp(binary, case, alias, "json")
    mi = median_run(cmd_json, root, env)
    compact = run(build_milsp(binary, case, alias, "compact"), root, env)
    default = run(build_milsp(binary, case, alias, None), root, env)

    ok, error_code, items, truncated, backend = parse_milsp(mi["stdout"], root)
    if mi["any_timeout"]:
        ok, error_code = False, "timeout"
    elif not ok and error_code is None:
        error_code = "exit_%s" % mi["rc"]
    elif ok and mi["rc"] not in (0, None):
        ok, error_code = False, "exit_%s" % mi["rc"]
    result["milsp"] = {
        "cmd": safe_cmd(cmd_json, binary), "ok": ok, "error": error_code, "backend": backend,
        "truncated": truncated, "items": len(items), "ms": round(mi["ms"], 1),
        "bytes_json": len(mi["stdout"]), "bytes_compact": len(compact["stdout"]),
        "bytes_default": len(default["stdout"]),
    }
    result["milsp"]["sample_items"] = ["%s:%s" % (f, ln) if ln else f for f, ln in items[:MAX_STORED_ITEMS]]

    # --- puntuacion ---
    mi_files_ordered = []
    for file, _ in items:
        if file not in mi_files_ordered:
            mi_files_ordered.append(file)
    if qtype == "intent":
        expected = set(case["expected_files"])
        hit = ok and bool(expected & set(mi_files_ordered[:TOP_K]))
        result["milsp"]["hit_at_5"] = hit
        result["false_empty"] = (not ok) or len(items) == 0
        result["precision"] = None
        result["recall"] = None
    else:
        mi_set = set(items if all(ln for _, ln in items) else [])
        if items and not mi_set:
            # items sin linea: no se puede puntuar por file:line; se compara por archivo
            oracle_files = {f for f, _ in oracle}
            inter = len({f for f, _ in items} & oracle_files)
            result["milsp"]["line_missing"] = True
            result["precision"] = round_or_none(inter / len({f for f, _ in items}))
            result["recall"] = round_or_none(inter / len(oracle_files)) if oracle_files else None
        else:
            inter = len(mi_set & oracle)
            result["precision"] = round_or_none(inter / len(mi_set)) if mi_set else None
            result["recall"] = round_or_none(inter / len(oracle)) if oracle else None
        if not ok:
            result["recall"] = 0.0 if oracle else None
        result["false_empty"] = bool(oracle) and ((not ok) or len(items) == 0)
        if qtype == "definition":
            expected = set(case.get("expected", []))
            mi_strs = {"%s:%s" % pair for pair in items}
            result["expected_in_rg_oracle"] = expected <= {"%s:%s" % pair for pair in oracle}
            result["milsp"]["expected_hit"] = bool(expected) and expected <= mi_strs
    result["error"] = (not ok)
    return result


def mean(values):
    values = [v for v in values if v is not None]
    return statistics.fmean(values) if values else None


def fmt(value, digits=2, pct=False):
    if value is None:
        return "-"
    return ("%.0f%%" % (value * 100)) if pct else ("%.*f" % (digits, value))


def summarize(results):
    groups = defaultdict(list)
    for r in results:
        groups[(r["type"], r["language"])].append(r)
    rows = []
    for (qtype, lang), rs in sorted(groups.items()):
        errors = [r["milsp"]["error"] for r in rs if r["error"]]
        codes = defaultdict(int)
        for code in errors:
            codes[code] += 1
        rows.append({
            "type": qtype, "language": lang, "n": len(rs),
            "precision": mean([r["precision"] for r in rs]),
            "recall": mean([r["recall"] for r in rs]),
            "hit5_mi": mean([1.0 if r["milsp"].get("hit_at_5") else 0.0 for r in rs]) if qtype == "intent" else None,
            "hit5_rg": mean([1.0 if r["rg"].get("hit_at_5") else 0.0 for r in rs]) if qtype == "intent" else None,
            "bytes_compact": mean([r["milsp"]["bytes_compact"] for r in rs]),
            "bytes_default": mean([r["milsp"]["bytes_default"] for r in rs]),
            "bytes_rg": mean([r["rg"]["bytes"] for r in rs]),
            "ms_mi": mean([r["milsp"]["ms"] for r in rs]),
            "ms_rg": mean([r["rg"]["ms"] for r in rs]),
            "false_empty": sum(1 for r in rs if r["false_empty"]),
            "errors": len(errors),
            "error_codes": dict(codes),
        })
    return rows


def render_md(label, rows, results, meta):
    lines = ["# Benchmark mi-lsp vs ripgrep: %s" % label, ""]
    lines.append("- Binario: `%s`" % meta["version"])
    lines.append("- Casos: %d, corridas por latencia: %d (mediana), timeout por comando: %ds" % (len(results), RUNS, TIMEOUT_S))
    lines.append("- Precisión y recall se promedian sobre los casos con resultado puntuable; un error de mi-lsp cuenta recall 0. Para `intent` se reporta hit@5 en lugar de precisión/recall.")
    lines.append("- Bytes: promedio por caso (mi-lsp `--format compact` y salida por defecto; rg stdout). Latencia: media de las medianas por caso, en ms.")
    lines.append("")
    lines.append("| Tipo | Lenguaje | N | Precisión | Recall | hit@5 mi-lsp | hit@5 rg | Bytes mi-lsp (compact) | Bytes mi-lsp (default) | Bytes rg | ms mi-lsp | ms rg | Falsos vacíos | Errores |")
    lines.append("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    for r in rows:
        lines.append("| %s | %s | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %d | %d |" % (
            r["type"], r["language"], r["n"], fmt(r["precision"], pct=True), fmt(r["recall"], pct=True),
            fmt(r["hit5_mi"], pct=True), fmt(r["hit5_rg"], pct=True), fmt(r["bytes_compact"], 0),
            fmt(r["bytes_default"], 0), fmt(r["bytes_rg"], 0), fmt(r["ms_mi"], 0), fmt(r["ms_rg"], 0),
            r["false_empty"], r["errors"]))
    total_fe = sum(r["false_empty"] for r in rows)
    total_err = sum(r["errors"] for r in rows)
    total_n = sum(r["n"] for r in rows)
    lines += ["", "**Total:** %d casos, %d falsos vacíos (%s), %d errores (%s)." % (
        total_n, total_fe, fmt(total_fe / total_n if total_n else None, pct=True),
        total_err, fmt(total_err / total_n if total_n else None, pct=True)), ""]
    codes = defaultdict(int)
    for r in rows:
        for code, count in r["error_codes"].items():
            codes[code] += count
    if codes:
        lines.append("## Códigos de error de mi-lsp")
        lines.append("")
        for code, count in sorted(codes.items(), key=lambda kv: -kv[1]):
            lines.append("- `%s`: %d" % (code, count))
        lines.append("")
    lines.append("## Definition: posición esperada")
    lines.append("")
    defs = [r for r in results if r["type"] == "definition"]
    lines.append("- mi-lsp devuelve la posición esperada en %d/%d casos; el oráculo de rg incluye la posición esperada en %d/%d." % (
        sum(1 for r in defs if r["milsp"].get("expected_hit")), len(defs),
        sum(1 for r in defs if r.get("expected_in_rg_oracle")), len(defs)))
    lines.append("")
    return "\n".join(lines)


def case_key(r):
    return (r["id"])


def render_compare(base, final):
    brows = {(r["type"], r["language"]): r for r in base["summary"]}
    frows = {(r["type"], r["language"]): r for r in final["summary"]}
    lines = ["# Comparación baseline vs final", ""]
    lines.append("- baseline: `%s`" % base["meta"]["version"])
    lines.append("- final: `%s`" % final["meta"]["version"])
    lines.append("")
    lines.append("| Tipo | Lenguaje | Precisión base→final | Recall base→final | hit@5 base→final | Falsos vacíos base→final | Errores base→final | ms mi-lsp base→final | Bytes compact base→final |")
    lines.append("|---|---|---|---|---|---|---|---|---|")
    for key in sorted(set(brows) | set(frows)):
        b, f = brows.get(key), frows.get(key)
        if not b or not f:
            continue
        lines.append("| %s | %s | %s → %s | %s → %s | %s → %s | %d → %d | %d → %d | %s → %s | %s → %s |" % (
            key[0], key[1], fmt(b["precision"], pct=True), fmt(f["precision"], pct=True),
            fmt(b["recall"], pct=True), fmt(f["recall"], pct=True),
            fmt(b["hit5_mi"], pct=True), fmt(f["hit5_mi"], pct=True),
            b["false_empty"], f["false_empty"], b["errors"], f["errors"],
            fmt(b["ms_mi"], 0), fmt(f["ms_mi"], 0), fmt(b["bytes_compact"], 0), fmt(f["bytes_compact"], 0)))
    bt = sum(r["false_empty"] for r in base["summary"])
    ft = sum(r["false_empty"] for r in final["summary"])
    be = sum(r["errors"] for r in base["summary"])
    fe = sum(r["errors"] for r in final["summary"])
    lines += ["", "**Totales:** falsos vacíos %d → %d, errores %d → %d." % (bt, ft, be, fe), ""]
    bcases = {r["id"]: r for r in base["cases"]}
    fixed, regressed = [], []
    for r in final["cases"]:
        b = bcases.get(r["id"])
        if not b:
            continue
        if b["false_empty"] and not r["false_empty"]:
            fixed.append(r["id"])
        elif not b["false_empty"] and r["false_empty"]:
            regressed.append(r["id"])
    lines.append("## Casos que dejaron de ser falso vacío (%d)" % len(fixed))
    lines += ["", ", ".join("`%s`" % c for c in fixed) or "-", ""]
    lines.append("## Regresiones: casos que pasaron a falso vacío (%d)" % len(regressed))
    lines += ["", ", ".join("`%s`" % c for c in regressed) or "-", ""]
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--milsp", required=True, help="ruta al binario mi-lsp")
    ap.add_argument("--label", required=True, help="baseline | final")
    ap.add_argument("--out", required=True, help="directorio de salida")
    ap.add_argument("--cases", default=str(HERE / "cases.json"))
    ap.add_argument("--repos-root", default=str(Path.home() / "repos" / "mios"))
    ap.add_argument("--only", default="", help="filtro por substring del id del caso")
    args = ap.parse_args()

    spec = json.loads(Path(args.cases).read_text(encoding="utf-8"))
    binary = str(Path(args.milsp).resolve())
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    version = run([binary, "--version"], str(HERE), dict(os.environ)).get("stdout", b"").decode("utf-8", "replace").strip()
    version = re.sub(r"\s+rid=\S+", "", version) or "desconocido"

    results = []
    for case in spec["cases"]:
        if args.only and args.only not in case["id"]:
            continue
        res = run_case(case, spec["repos"], args.repos_root, binary, args.label)
        results.append(res)
        print("%-48s %-10s items=%-4s err=%s fe=%s" % (
            res["id"], res["type"], res["milsp"]["items"], res["milsp"]["error"], res["false_empty"]), file=sys.stderr)

    meta = {"label": args.label, "version": version, "runs": RUNS, "timeout_s": TIMEOUT_S}
    summary = summarize(results)
    payload = {"meta": meta, "summary": summary, "cases": results}
    (out / ("%s.json" % args.label)).write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (out / ("%s.md" % args.label)).write_text(render_md(args.label, summary, results, meta) + "\n", encoding="utf-8")

    base_path, final_path = out / "baseline.json", out / "final.json"
    if base_path.exists() and final_path.exists():
        base = json.loads(base_path.read_text(encoding="utf-8"))
        final = json.loads(final_path.read_text(encoding="utf-8"))
        (out / "compare.md").write_text(render_compare(base, final) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
