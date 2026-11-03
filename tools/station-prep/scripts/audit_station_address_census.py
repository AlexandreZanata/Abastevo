#!/usr/bin/env python3
"""Independently recount paired v4 station address preparations, files only."""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
from pathlib import Path
import re
import sys

MAX_MANIFEST = 1 << 20
MAX_STREAM = 1 << 30
MAX_LINE = 256 << 10
MAX_ROWS = 200_000
HASH = re.compile(r"[0-9a-f]{64}")
UF = set("AC AL AM AP BA CE DF ES GO MA MG MS MT PA PB PE PI PR RJ RN RO RR RS SC SE SP TO".split())
BASE = {"anp", "municipality", "unknown"}
AFTER = BASE | {"rfb", "osm"}
POLICY = Path(__file__).resolve().parents[3] / "contracts/station-prep/address-source-policy-v1.json"


class AuditError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise AuditError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def decode(raw):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "duplicate JSON key")
            result[key] = value
        return result
    def reject_constant(_):
        raise AuditError("invalid JSON number")
    return json.loads(raw, object_pairs_hook=unique, parse_constant=reject_constant)


def bounded_file(path, maximum):
    require(not path.is_symlink() and path.is_file() and path.stat().st_size <= maximum,
            "regular bounded artifact required")
    return path.open("rb")


def read_generation(root, expected, baseline):
    require(HASH.fullmatch(expected), "expected manifest hash required")
    with bounded_file(root / "manifest.json", MAX_MANIFEST) as f:
        raw = f.read(MAX_MANIFEST + 1)
    require(len(raw) <= MAX_MANIFEST and sha(raw) == expected, "manifest hash mismatch")
    m = decode(raw)
    require(m["format_version"] == "station-batch-v1" and
            m["parser_version"] == "station-prep-v0.3.1+addresses-v1" and
            m["address_policy_sha256"] == sha(POLICY.read_bytes()), "unexpected v4 policy")
    require(not baseline or (m["address_inputs"] == [] and m["address_skips"] == []),
            "baseline has supplemental address inputs")
    files = [v for v in m["outputs"] if v["path"] == "assertions.jsonl"]
    require(len(files) == 1, "assertion stream descriptor missing or duplicated")
    output = files[0]
    require(type(output["bytes"]) is int and type(output["rows"]) is int and
            0 <= output["bytes"] <= MAX_STREAM and 0 <= output["rows"] <= MAX_ROWS,
            "assertion stream budget exceeded")
    current = {}
    identities = {}
    historical = set()
    digest = hashlib.sha256()
    size = rows = 0
    with bounded_file(root / "assertions.jsonl", MAX_STREAM) as f:
        while raw := f.readline(MAX_LINE + 1):
            require(len(raw) <= MAX_LINE and raw.endswith(b"\n"), "assertion line budget or EOF")
            size += len(raw)
            rows += 1
            require(size <= output["bytes"] and rows <= output["rows"], "assertion stream bounds differ")
            digest.update(raw)
            v = decode(raw)
            require(v["schema_version"] == "station-assertion-v4" and v["source"] == "registry-13col" and
                    v["latitude"] is None and v["longitude"] is None and v["location_quality"] == "unknown",
                    "unexpected assertion shape or point")
            cnpj, checksum = v["source_key"], v["checksum"]
            require(re.fullmatch(r"[0-9A-Z]{12}[0-9]{2}", cnpj) and v["uf"] in UF and HASH.fullmatch(checksum),
                    "invalid station assertion identity/geography")
            row_sha = sha(raw)
            key = (cnpj, checksum)
            require(key not in identities or identities[key] == row_sha, "conflicting repeated assertion")
            identities[key] = row_sha
            if v["supersedes"] is not None:
                require(HASH.fullmatch(v["supersedes"]), "invalid historical link")
                historical.add((cnpj, v["supersedes"]))
                continue
            facts = v.pop("address_facts")
            source = "unknown" if facts["source"] is None else facts["source"]
            require(source in (BASE if baseline else AFTER), "unexpected address category")
            v.pop("checksum")
            v.pop("supersedes")
            original = sha(json.dumps(v, sort_keys=True, ensure_ascii=False, separators=(",", ":")).encode())
            row = (v["uf"], source, original, row_sha, checksum)
            require(cnpj not in current or current[cnpj] == row, "conflicting current assertion")
            current[cnpj] = row
    require((rows, size, digest.hexdigest()) == (output["rows"], output["bytes"], output["sha256"]),
            "assertion stream fingerprint mismatch")
    require(all(cnpj in current and current[cnpj][-1] == target for cnpj, target in historical),
            "historical link does not target the current assertion of the same station")
    accounting = m["counts"]["registry-13col"]
    require(accounting["accepted"] == len(identities) and accounting["duplicates"] == rows - len(identities)
            and accounting["input"] == rows + accounting["quarantined"], "assertion accounting mismatch")
    return m, current


def transitions(before, after):
    by_uf = defaultdict(lambda: {"before": Counter(), "after": Counter()})
    national = {"before": Counter(), "after": Counter()}
    require(before.keys() == after.keys(), "current station universe changed")
    for identity, (uf, old_source, old_original, _, _) in before.items():
        other_uf, new_source, new_original, _, _ = after[identity]
        require((uf, old_original) == (other_uf, new_original), "original station facts changed")
        require(old_source != "anp" or new_source == "anp", "official address precedence changed")
        by_uf[uf]["before"][old_source] += 1
        by_uf[uf]["after"][new_source] += 1
        national["before"][old_source] += 1
        national["after"][new_source] += 1
    return dict(by_uf), national


def audit(baseline, baseline_sha, enriched, enriched_sha):
    a, old = read_generation(baseline, baseline_sha, True)
    b, new = read_generation(enriched, enriched_sha, False)
    for field in ("inputs", "counts", "municipality_reference", "name_inputs", "completeness", "policy_version"):
        require(a.get(field) == b.get(field), "registry, locality, name or policy inputs changed")
    for m, end in ((a, old), (b, new)):
        by_uf, national = transitions(old, end)
        c = m["address_coverage"]
        require((c["station_count"], c["by_uf"], c["national"]) == (len(old), by_uf, national),
                "prepared address census disagrees with recount")
    return {
        "format": "abastevo.station-address-census-audit-v1",
        "baseline_manifest_sha256": baseline_sha,
        "enriched_manifest_sha256": enriched_sha,
        "station_count": len(old), "by_uf": by_uf, "national": national,
        "source_policy_sha256": b["address_policy_sha256"],
        "units": "current distinct station CNPJs in the same pinned registry input",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True, type=Path)
    parser.add_argument("--baseline-sha256", required=True)
    parser.add_argument("--enriched", required=True, type=Path)
    parser.add_argument("--enriched-sha256", required=True)
    args = parser.parse_args()
    try:
        result = audit(args.baseline, args.baseline_sha256, args.enriched, args.enriched_sha256)
    except (OSError, ValueError, TypeError, KeyError) as exc:
        # Input parsing exceptions may carry source data; expose only our labels.
        reason = str(exc) if isinstance(exc, AuditError) else "invalid local artifact"
        print(f"address census audit refused: {reason}", file=sys.stderr)
        return 2
    print(json.dumps(result, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
