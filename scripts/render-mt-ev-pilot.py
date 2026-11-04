#!/usr/bin/env python3
"""Validate the reviewed regional charging manifest and render a private SQL load."""
import argparse
import hashlib
import json
import re
import runpy
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "data/src/main/resources/charging/mt-top10-pilot-v1.json"


def render(boundaries):
    source = boundaries.read_bytes()
    catalog = json.loads(MANIFEST.read_bytes())
    if hashlib.sha256(source).hexdigest() != catalog["ibge_boundaries_sha256"]:
        raise ValueError("IBGE boundary pin mismatch")
    municipalities = {m["code"] for m in catalog["municipalities"]}
    if len(municipalities) != 10 or len(catalog["municipalities"]) != 10:
        raise ValueError("expected ten distinct ranked municipalities")
    geometry = {f["properties"]["codarea"]: f["geometry"] for f in json.loads(source)["features"]}
    contains = runpy.run_path(str(ROOT / "scripts/audit-station-uf.py"))["contains"]
    seen = set()
    for row in catalog["places"]:
        ref = row["external_id"]
        expected = uuid.uuid5(uuid.NAMESPACE_URL, "https://abastevo.com.br/charging/mt-top10/" + ref)
        if (not re.fullmatch(r"[a-z0-9-]{1,80}", ref) or ref in seen or row["id"] != str(expected)
                or row["municipality_code"] not in municipalities or not row["name"].strip()
                or row["access"] not in ("PUBLIC", "UNKNOWN") or not row["sources"]
                or any(not url.startswith("https://") for url in row["sources"])
                or not contains(geometry[row["municipality_code"]], float(row["longitude"]), float(row["latitude"]))):
            raise ValueError(f"invalid or conflicting charging record: {ref}")
        seen.add(ref)
    if not 1 <= len(seen) <= 100 or not set(geometry).issuperset(municipalities):
        raise ValueError("invalid regional census")
    payload = json.dumps(catalog, ensure_ascii=False, sort_keys=True, separators=(",", ":")).replace("'", "''")
    seed = (ROOT / "backend/db/seeds/mato_grosso_charging_pilot.sql").read_text()
    return ("BEGIN;\nSET LOCAL standard_conforming_strings = on;\n"
            f"SELECT set_config('abastevo.mt_ev_manifest', '{payload}', true) IS NOT NULL;\n"
            f"{seed}\nCOMMIT;\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--boundaries", required=True, type=Path)
    print(render(parser.parse_args().boundaries), end="")
