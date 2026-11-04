#!/usr/bin/env python3
"""Reconcile one UF from local ANP, IBGE and optional Directory snapshots."""
import argparse
import csv
import hashlib
import json
import re
import unicodedata
from collections import Counter
from pathlib import Path


def normalized(text):
    decomposed = unicodedata.normalize("NFD", text.strip().upper())
    return " ".join("".join(c for c in decomposed if not unicodedata.combining(c)).split())


def by_id(rows, key):
    result = {}
    for row in rows:
        value = row[key].strip().upper()
        if not re.fullmatch(r"[0-9A-Z]{12}[0-9]{2}", value) or value in result:
            raise ValueError("missing, malformed or duplicate establishment identifier")
        result[value] = row
    if not result:
        raise ValueError("empty UF inventory")
    return result


def in_ring(lon, lat, ring):
    return sum(
        (y1 > lat) != (y2 > lat) and lon < (x2 - x1) * (lat - y1) / (y2 - y1) + x1
        for (x1, y1), (x2, y2) in zip(ring, ring[1:])
    ) % 2 == 1


def contains(geometry, lon, lat):
    parts = geometry["coordinates"]
    if geometry["type"] == "Polygon":
        parts = [parts]
    elif geometry["type"] != "MultiPolygon":
        raise ValueError("unsupported IBGE geometry")
    return any(in_ring(lon, lat, p[0]) and not any(in_ring(lon, lat, h) for h in p[1:]) for p in parts)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def json_file(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")


def audit(args):
    checkout = Path(__file__).resolve().parents[1]
    if args.output.resolve().is_relative_to(checkout):
        raise ValueError("keep the private audit outputs outside the checkout")
    if not 1 <= len(args.api) <= 50:
        raise ValueError("ANP API page count is out of bounds")
    paths = {key: getattr(args, key) for key in ("registry", "aliases", "boundaries")}
    hashes = {key: digest(path.read_bytes()) for key, path in paths.items()}
    raw = args.registry.read_bytes()
    try:
        registry_text = raw.decode("utf-8-sig")
    except UnicodeDecodeError:
        registry_text = raw.decode("latin-1")
    reader = csv.DictReader(registry_text.splitlines(keepends=True), delimiter=";")
    registry = by_id((r for r in reader if r["UF"].strip() == args.uf), "CNPJ")
    rows, page_hashes = [], []
    total = size = None
    for number, path in enumerate(args.api, 1):
        payload = path.read_bytes()
        api = json.loads(payload)
        page = api["searchPageFilter"]
        if number == 1:
            total, size = page["totalRegistro"], page["tamanhoPagina"]
        if (api["status"] != 200 or api["succeeded"] is not True or total <= 0 or not 1 <= size <= 5000
                or page["numeroPagina"] != number or page["totalPagina"] != len(args.api)
                or page["totalRegistro"] != total or page["tamanhoPagina"] != size
                or len(api["data"]) != min(size, max(0, total - (number - 1) * size))
                or any(r["uf"] != args.uf for r in api["data"])):
            raise ValueError("incomplete, out-of-order or inconsistent official ANP pages")
        rows.extend(api["data"])
        page_hashes.append(digest(payload))
    if len(rows) != total or len(args.api) != (total + size - 1) // size:
        raise ValueError("ANP API traversal did not account for the declared total")
    hashes["api"] = page_hashes[0] if len(page_hashes) == 1 else digest("\n".join(page_hashes).encode())
    api_rows = by_id(rows, "cnpj")
    if registry.keys() != api_rows.keys():
        raise ValueError("ANP CSV and API editions disagree on UF membership")

    towns = {}
    for entry in json.loads(args.aliases.read_bytes())["entries"]:
        if entry["uf"] == args.uf:
            for alias in entry["aliases"]:
                name = normalized(alias)
                if name in towns and towns[name] != entry["ibge"]:
                    raise ValueError("ambiguous municipality alias")
                towns[name] = entry["ibge"]
    boundaries = {f["properties"]["codarea"]: f["geometry"]
                  for f in json.loads(args.boundaries.read_bytes())["features"]}
    current = {}
    if args.directory:
        for cnpj, municipality, status, quality, point in csv.reader(
                args.directory.read_text().splitlines(), delimiter="\t"):
            if cnpj in current or point not in {"0", "1"}:
                raise ValueError("invalid or duplicated Directory snapshot row")
            current[cnpj] = (municipality, status, quality, point)

    places = []
    municipalities = {code: Counter() for code in sorted(set(towns.values()))}
    outcomes = Counter()
    for cnpj, row in sorted(registry.items()):
        source = api_rows[cnpj]
        code = towns.get(normalized(row["MUNICIPIO"]))
        if not code or code != towns.get(normalized(source["municipio"])) or code not in boundaries:
            raise ValueError("unresolved or conflicting official municipality")
        counts = municipalities[code]
        counts["official"] += 1
        previous = current.get(cnpj)
        if previous and previous[0] != code:
            raise ValueError("Directory municipality conflicts with the source")
        if not previous:
            counts["missing_directory"] += 1
        elif previous[1] == "active" and previous[3] == "1":
            counts["existing_pin"] += 1

        lat, lon = source.get("latitude"), source.get("longitude")
        if lat in (None, "") and lon in (None, ""):
            outcomes["no_official_coordinate"] += 1
        elif (source.get("srid") != "4674" or source.get("src", "").upper() != "SIRGAS2000"
              or lat in (None, "") or lon in (None, "")):
            outcomes["unsupported_coordinate"] += 1
        elif not contains(boundaries[code], float(lon), float(lat)):
            outcomes["outside_municipality"] += 1
        else:
            outcomes["inside_municipality"] += 1
            if previous is None or (previous[1] == "active" and previous[3] == "0"):
                counts["new_pin_candidate"] += 1
                places.append({
                    "schema_version": "station-location-tier-v1", "cnpj": cnpj, "tier": "L3",
                    "quality": "anp-reported", "latitude_deg": lat, "longitude_deg": lon,
                    "provider": "anp-revendedores-api",
                    "source_reference": f"anp-api|sha256={hashes['api']}|ibge={code}",
                })

    args.output.mkdir(mode=0o700)
    with (args.output / "registry.csv").open("w", encoding="utf-8", newline="") as out:
        writer = csv.DictWriter(out, fieldnames=reader.fieldnames, delimiter=";")
        writer.writeheader()
        writer.writerows(registry.values())
    summary = {
        "uf": args.uf, "input_sha256": hashes, "api_pages_sha256": page_hashes,
        "official_establishments": len(registry),
        "coordinate_outcomes": outcomes, "municipalities": municipalities,
        "directory_checked": args.directory is not None,
        "extra_directory_identifiers": len(current.keys() - registry.keys()),
    }
    json_file(args.output / "coverage.json", summary)
    if args.prepare_anp_pins and places:
        raw = "".join(json.dumps(p, sort_keys=True, separators=(",", ":")) + "\n" for p in places).encode()
        (args.output / "locations.ndjson").write_bytes(raw)
        json_file(args.output / "locations-manifest.json", {
            "format": "abastevo.station-location-tiers-v1", "schema_version": "station-location-tier-v1",
            "generation": f"{args.uf.lower()}-anp-{hashes['api'][:16]}",
            "owner_decisions": {"D1_anp_reported": True, "D2_address_point": False},
            "files": {"locations.ndjson": {"bytes": len(raw), "sha256": digest(raw)}},
        })
    print(f"{args.uf}: {len(registry)} official establishments, {len(places)} new ANP pin candidates")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--uf", required=True, type=str.upper)
    for flag in ("registry", "aliases", "boundaries", "output"):
        parser.add_argument(f"--{flag}", required=True, type=Path)
    parser.add_argument("--api", required=True, action="append", type=Path, help="ordered ANP API page; repeat per page")
    parser.add_argument("--directory", type=Path, help="CNPJ, IBGE code, status, quality and has-point TSV")
    parser.add_argument("--prepare-anp-pins", action="store_true", help="explicitly select the existing D1 tier")
    audit(parser.parse_args())
