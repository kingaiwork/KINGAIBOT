#!/usr/bin/env python3
"""Generate a standards-compliant, reproducible CycloneDX SBOM for KINGAIBOT.

The BOM structure is produced by the official CycloneDX Go module generator,
pinned to an explicit version. This script only normalizes release metadata that
must be deterministic across repeated builds of the same source revision.
"""

import datetime as dt
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

CYCLONEDX_GOMOD = "github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.10.0"
CYCLONEDX_SPEC = "1.6"

root = Path(__file__).resolve().parents[1]
out = Path(sys.argv[1]) if len(sys.argv) > 1 else root / "dist" / "sbom.cdx.json"
version = os.environ.get("VERSION", "1.2.1").lstrip("v")
source_epoch = int(os.environ.get("SOURCE_DATE_EPOCH", "0") or "0")

if source_epoch <= 0:
    try:
        source_epoch = int(
            subprocess.check_output(
                ["git", "log", "-1", "--format=%ct"],
                cwd=root,
                text=True,
                stderr=subprocess.DEVNULL,
            ).strip()
        )
    except Exception:
        source_epoch = 315532800

timestamp = (
    dt.datetime.fromtimestamp(source_epoch, tz=dt.timezone.utc)
    .replace(microsecond=0)
    .isoformat()
    .replace("+00:00", "Z")
)

with tempfile.TemporaryDirectory(prefix="kingaibot-sbom-") as tmpdir:
    raw_bom = Path(tmpdir) / "bom.cdx.json"
    cmd = [
        "go",
        "run",
        CYCLONEDX_GOMOD,
        "mod",
        "-json",
        "-noserial",
        "-output-version",
        CYCLONEDX_SPEC,
        "-type",
        "application",
        "-output",
        str(raw_bom),
        str(root),
    ]
    subprocess.run(cmd, cwd=root, check=True, timeout=300)
    bom = json.loads(raw_bom.read_text(encoding="utf-8"))

if bom.get("bomFormat") != "CycloneDX":
    raise SystemExit("official generator did not return a CycloneDX BOM")
if bom.get("specVersion") != CYCLONEDX_SPEC:
    raise SystemExit(
        f"unexpected CycloneDX version: {bom.get('specVersion')!r}; expected {CYCLONEDX_SPEC}"
    )

metadata = bom.setdefault("metadata", {})
metadata["timestamp"] = timestamp
component = metadata.setdefault("component", {})
component["type"] = "application"
component["name"] = "KINGAIBOT"
component["version"] = version

# Keep generator-created bom-ref/PURL/dependency relationships intact. Only
# deterministic presentation ordering is normalized below.
components = bom.get("components")
if isinstance(components, list):
    components.sort(
        key=lambda item: (
            str(item.get("bom-ref", "")),
            str(item.get("name", "")),
            str(item.get("version", "")),
        )
    )
dependencies = bom.get("dependencies")
if isinstance(dependencies, list):
    dependencies.sort(key=lambda item: str(item.get("ref", "")))
    for item in dependencies:
        depends_on = item.get("dependsOn")
        if isinstance(depends_on, list):
            depends_on.sort()

out.parent.mkdir(parents=True, exist_ok=True)
out.write_text(
    json.dumps(bom, ensure_ascii=False, sort_keys=True, indent=2) + "\n",
    encoding="utf-8",
)
