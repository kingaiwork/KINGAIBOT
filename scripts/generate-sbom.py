#!/usr/bin/env python3
import datetime as dt
import json
import os
import subprocess
import sys
from pathlib import Path
from urllib.parse import quote

root = Path(__file__).resolve().parents[1]
out = Path(sys.argv[1]) if len(sys.argv) > 1 else root / "dist" / "sbom.cdx.json"
version = os.environ.get("VERSION", "1.1.0").lstrip("v")
source_epoch = int(os.environ.get("SOURCE_DATE_EPOCH", "0") or "0")
if source_epoch <= 0:
    try: source_epoch = int(subprocess.check_output(["git", "log", "-1", "--format=%ct"], cwd=root, text=True, stderr=subprocess.DEVNULL).strip())
    except Exception: source_epoch = 315532800
if source_epoch > 0:
    timestamp = dt.datetime.fromtimestamp(source_epoch, tz=dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
else: timestamp = "1970-01-01T00:00:00Z"
def go_modules():
    proc = subprocess.run(["go", "list", "-m", "-json", "all"], cwd=root, text=True, capture_output=True, check=True)
    dec = json.JSONDecoder(); text = proc.stdout; pos = 0; mods = []
    while pos < len(text):
        while pos < len(text) and text[pos].isspace(): pos += 1
        if pos >= len(text): break
        obj, pos = dec.raw_decode(text, pos); mods.append(obj)
    return mods
mods = go_modules(); components = []
for m in mods:
    if m.get("Main"): continue
    path = m.get("Path"); ver = m.get("Version") or "unknown"; repl = m.get("Replace")
    if repl: path = repl.get("Path", path); ver = repl.get("Version") or ver
    if not path: continue
    purl = f"pkg:golang/{quote(path, safe='/')}@{quote(ver, safe='.+~-')}"
    components.append({"type":"library","name":path,"version":ver,"purl":purl,"bom-ref":purl})
components.sort(key=lambda x: (x["name"], x["version"]))
bom = {"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"metadata":{"timestamp":timestamp,"tools":{"components":[{"type":"application","name":"Go toolchain","version":subprocess.check_output(["go","version"],text=True).strip()}]},"component":{"type":"application","name":"KINGAIBOT","version":version,"bom-ref":f"pkg:generic/king-agent-os@{quote(version, safe='.+~-')}"}},"components":components}
out.parent.mkdir(parents=True, exist_ok=True)
out.write_text(json.dumps(bom, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
