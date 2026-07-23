"""Canonical JSON and comparable measurement units."""
import json,re,hashlib
from pathlib import Path
VOLATILE_KEYS={"started_at","finished_at","pid","hostname","duration_ns","rss_bytes"}
def canonicalize(value):
    if isinstance(value,dict): return {k:canonicalize(v) for k,v in sorted(value.items()) if k not in VOLATILE_KEYS}
    if isinstance(value,list): return [canonicalize(v) for v in value]
    if isinstance(value,str): return value.replace("\\","/").replace("\r\n","\n").replace("\r","\n")
    return value
def canonical_json(value): return json.dumps(canonicalize(value),ensure_ascii=False,sort_keys=True,separators=(",",":"))
def content_hash(path): return hashlib.sha256(Path(path).read_bytes()).hexdigest()
def token_count(value):
    text=value if isinstance(value,str) else canonical_json(value)
    return len(re.findall(r"\w+|[^\w\s]",text,re.UNICODE))
def comparable_units(value):
    text=value if isinstance(value,str) else canonical_json(value)
    return {"bytes":len(text.encode("utf-8")),"characters":len(text),"tokens":token_count(text),"unit_version":"victory-units-v1"}
