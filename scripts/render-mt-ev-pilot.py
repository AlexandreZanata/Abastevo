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
MANIFESTS = ROOT / "data/src/main/resources/charging"


def render(boundaries, sorriso=False):
    edition = "sorriso-supplement" if sorriso else "mt-top10"
    resource = "sorriso-supplement-v1.json" if sorriso else "mt-top10-pilot-v1.json"
    profile = "charging-mt-sorriso-supplement-v1" if sorriso else "charging-mt-top10-pilot-v1"
    source = boundaries.read_bytes()
    catalog = json.loads((MANIFESTS / resource).read_bytes())
    if catalog["profile"] != profile:
        raise ValueError("unexpected manifest profile")
    if hashlib.sha256(source).hexdigest() != catalog["ibge_boundaries_sha256"]:
        raise ValueError("IBGE boundary pin mismatch")
    municipalities = {m["code"] for m in catalog["municipalities"]}
    expected_count = 1 if sorriso else 10
    if (len(municipalities) != expected_count or len(catalog["municipalities"]) != expected_count
            or (sorriso and municipalities != {"5107925"})):
        raise ValueError("unexpected municipalities for the pilot")
    geometry = {f["properties"]["codarea"]: f["geometry"] for f in json.loads(source)["features"]}
    contains = runpy.run_path(str(ROOT / "scripts/audit-station-uf.py"))["contains"]
    seen = set()
    for row in catalog["places"]:
        ref = row["external_id"]
        expected = uuid.uuid5(uuid.NAMESPACE_URL, f"https://abastevo.com.br/charging/{edition}/{ref}")
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
    parser.add_argument("--sorriso", action="store_true", help="render the Sorriso supplement")
    args = parser.parse_args()
    print(render(args.boundaries, args.sorriso), end="")
