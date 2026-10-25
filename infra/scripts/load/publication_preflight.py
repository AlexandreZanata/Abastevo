#!/usr/bin/env python3
"""ESP-18 publication preflight. It verifies a named public scope and a portal
candidate locally; it never contacts DNS, CDN, VPS, API, database or ANP and
never publishes. Exit 0 is not a publication; exit 3 means the local candidate
is ready but execution approval/external ownership verification is still
required.
"""
import argparse
import datetime as dt
import ipaddress
import json
import os
import re
import sys
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path
from urllib.parse import urlparse

SCHEMA = "abastevo-portal-publication-preflight-v1"
SCOPE_SCHEMA = "abastevo-portal-publication-scope-v1"
PUBLIC_SLICES = {"official-sanctions"}
PLACEHOLDER = re.compile(r"(example|invalid|localhost|tbd|todo|placeholder|dummy|test-only|changeme|<|>)", re.I)
SHA = re.compile(r"^sha256:[0-9a-f]{64}$")


def valid_public_origin(value):
    try:
        parsed = urlparse(value)
    except ValueError:
        return False, "unparseable origin"
    if parsed.scheme != "https":
        return False, "origin must use https"
    if not parsed.hostname:
        return False, "origin host missing"
    if parsed.path not in ("", "/") or parsed.query or parsed.fragment or parsed.username or parsed.password:
        return False, "origin must be a bare https origin"
    host = parsed.hostname.lower().rstrip(".")
    if host in {"localhost", "example.com", "example.org", "example.net"} or host.endswith((".localhost", ".example", ".invalid", ".test")):
        return False, "origin is not a public host"
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        return True, ""
    if address.is_private or address.is_loopback or address.is_link_local or address.is_reserved or address.is_multicast:
        return False, "origin is a private/reserved IP"
    return True, ""


def placeholder(value):
    return isinstance(value, str) and bool(PLACEHOLDER.search(value))


def load_json(path):
    try:
        with open(path, encoding="utf-8") as handle:
            return json.load(handle)
    except OSError as err:
        raise ValueError(f"cannot read {path}: {err}")
    except json.JSONDecodeError as err:
        raise ValueError(f"invalid JSON in {path}: {err}")


def require_string(scope, key, errors):
    value = scope.get(key)
    if not isinstance(value, str) or not value.strip() or placeholder(value):
        errors.append(f"scope.{key} must be a non-placeholder string")


def validate_scope(scope):
    errors = []
    if scope.get("schema_version") != SCOPE_SCHEMA:
        errors.append("scope.schema_version invalid")
    for key in ("origin", "host", "root", "cdn", "tls", "approval_ref", "approved_by",
                "window_start", "window_end", "candidate_content_sha256",
                "rollback_content_sha256", "source_admission_ref", "host_ownership_attestation",
                "source_revision_id", "source_lease_id", "source_checksum", "source_edition"):
        require_string(scope, key, errors)
    origin = scope.get("origin", "")
    ok, why = valid_public_origin(origin)
    if not ok:
        errors.append(f"scope.origin: {why}")
    root = scope.get("root", "")
    if not isinstance(root, str) or not root.startswith("/") or ".." in root.split("/") or root == "/" or placeholder(root):
        errors.append("scope.root must be an absolute private path")
    for key in ("candidate_content_sha256", "rollback_content_sha256"):
        value = scope.get(key, "")
        if not isinstance(value, str) or not SHA.match(value):
            errors.append(f"scope.{key} must be sha256:<64 lowercase hex>")
    if scope.get("candidate_content_sha256") == scope.get("rollback_content_sha256"):
        errors.append("scope.rollback_content_sha256 must differ from the candidate")
    slices = scope.get("source_slices")
    if not isinstance(slices, list) or not slices:
        errors.append("scope.source_slices must be a non-empty list")
    else:
        if "prices" in slices:
            errors.append("RIGHTS_PENDING: prices are not publicly admitted in the current source register")
        unknown = [item for item in slices if item not in PUBLIC_SLICES]
        if unknown:
            errors.append(f"scope.source_slices not publicly admitted: {','.join(unknown)}")
    # Publication mode is inferred from the declared slices and checked
    # against the artifact manifest: an official-only scope requires an
    # official-only candidate, and vice versa.
    mode = scope.get("publication_mode")
    if mode is None:
        mode = "official-only" if slices == ["official-sanctions"] else "full"
        scope["publication_mode"] = mode
    if mode not in ("full", "official-only"):
        errors.append("scope.publication_mode must be full or official-only")
    elif mode == "official-only" and slices != ["official-sanctions"]:
        errors.append("scope.publication_mode official-only requires source_slices [official-sanctions]")
    evidence = scope.get("rights_evidence")
    if not isinstance(evidence, list) or not evidence or any(placeholder(item) for item in evidence):
        errors.append("scope.rights_evidence must be non-placeholder references")
    rows = scope.get("source_rows")
    if not isinstance(rows, int) or rows <= 0:
        errors.append("scope.source_rows must be positive")
    for key in ("window_start", "window_end"):
        value = scope.get(key)
        if not isinstance(value, str):
            continue
        try:
            dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        except ValueError:
            errors.append(f"scope.{key} must be RFC3339")
    try:
        start = dt.datetime.fromisoformat(str(scope.get("window_start", "")).replace("Z", "+00:00"))
        end = dt.datetime.fromisoformat(str(scope.get("window_end", "")).replace("Z", "+00:00"))
        if start >= end:
            errors.append("scope publication window is empty")
    except ValueError:
        pass
    return errors


def sitemap_locations(path):
    try:
        root = ET.parse(path).getroot()
    except (OSError, ET.ParseError) as err:
        raise ValueError(f"sitemap unreadable: {err}")
    return [node.text.strip() for node in root.iter("{http://www.sitemaps.org/schemas/sitemap/0.9}loc") if node.text]


def verify_artifact(artifact, scope):
    failures = []
    checks = []
    manifest_path = artifact / "portal-manifest.json"
    try:
        manifest = load_json(manifest_path)
    except ValueError as err:
        return [str(err)], checks, {}
    checks.append(("manifest", "INFO", "portal manifest present"))
    mode = scope.get("publication_mode")
    if mode is None:
        mode = "official-only" if scope.get("source_slices") == ["official-sanctions"] else "full"
    manifest_official_only = bool(manifest.get("official_only", False))
    if (mode == "official-only") != manifest_official_only:
        failures.append(f"publication mode mismatch: scope={mode} artifact_official_only={manifest_official_only}")
    else:
        checks.append(("publication_mode", "PASS", mode))
    content = manifest.get("content_sha256")
    if content != scope.get("candidate_content_sha256"):
        failures.append(f"candidate content hash mismatch: artifact={content} scope={scope.get('candidate_content_sha256')}")
    else:
        checks.append(("candidate_hash", "PASS", content))
    official = manifest.get("official_records")
    if isinstance(official, dict):
        checks.append(("official_pin", "INFO", f"revision={official.get('revision_id')} lease={official.get('lease_id')} rows={official.get('rows')}"))
        for scope_key, manifest_key in (("source_revision_id", "revision_id"), ("source_lease_id", "lease_id"),
                                        ("source_checksum", "source_checksum"), ("source_edition", "edition"),
                                        ("source_rows", "rows")):
            if scope.get(scope_key) != official.get(manifest_key):
                failures.append(f"source pin mismatch: {scope_key}")
        census = official.get("association_census") or {}
        if census.get("total") != official.get("rows"):
            failures.append("official pin census is incomplete")
        checks.append(("official_census", "PASS" if census.get("total") == official.get("rows") else "FAIL",
                       f"total={census.get('total')}"))
    else:
        failures.append("official_records pin missing")
    price_pages = (artifact / "precos" / "index.html").exists() or (artifact / "postos").is_dir() or any(artifact.glob("dados/*/precos.csv"))
    if price_pages:
        failures.append("RIGHTS_PENDING: artifact contains price/station/download pages; the selected public slice is official-sanctions only")
    else:
        checks.append(("price_pages_absent", "PASS", "no RIGHTS_PENDING price pages"))
    if (artifact / "fiscalizacao" / "index.html").exists():
        if "official-sanctions" not in (scope.get("source_slices") or []):
            failures.append("official pages present but scope.source_slices lacks official-sanctions")
        else:
            checks.append(("official_slice", "PASS", "fiscalizacao pages declared"))
    else:
        failures.append("official-sanctions pages missing from candidate")
    sitemap = artifact / "sitemap.xml"
    if not sitemap.exists():
        failures.append("public indexable candidate must contain sitemap.xml")
    else:
        try:
            locations = sitemap_locations(sitemap)
        except ValueError as err:
            failures.append(str(err))
            locations = []
        origin = str(scope.get("origin", "")).rstrip("/")
        wrong = [loc for loc in locations if not (loc == origin or loc.startswith(origin + "/"))]
        if wrong:
            failures.append(f"sitemap origin mismatch: expected {origin}, found e.g. {wrong[0]}")
            checks.append(("sitemap_origin", "FAIL", wrong[0]))
        else:
            checks.append(("sitemap_origin", "PASS", origin))
        if not locations:
            failures.append("sitemap has no URLs")
    robots = artifact / "robots.txt"
    robots_text = robots.read_text(encoding="utf-8", errors="replace") if robots.exists() else ""
    origin = str(scope.get("origin", "")).rstrip("/")
    if "Allow: /" not in robots_text:
        failures.append("robots.txt does not allow the public portal")
    elif f"Sitemap: {origin}/sitemap.xml" not in robots_text:
        failures.append("robots.txt sitemap does not match the public origin")
    else:
        checks.append(("robots", "PASS", "Allow + origin sitemap"))
    home_name = "fiscalizacao" if mode == "official-only" else "consultar"
    home = artifact / home_name / "index.html"
    page = home.read_text(encoding="utf-8", errors="replace") if home.exists() else ""
    if 'name="robots" content="index,follow"' not in page:
        failures.append(f"{home_name} page is not indexable")
    else:
        checks.append(("page_indexable", "PASS", "index,follow"))
    return failures, checks, manifest


def run(args):
    if args.self_test:
        return self_test()
    if args.check_origin:
        ok, why = valid_public_origin(args.check_origin)
        if not ok:
            print(f"REFUSED: {why}", file=sys.stderr)
            return 2
        print("public origin accepted")
        return 0
    if args.check_sitemap_origin:
        origin, path = args.check_sitemap_origin
        ok, why = valid_public_origin(origin)
        if not ok:
            print(f"REFUSED: {why}", file=sys.stderr)
            return 2
        try:
            locations = sitemap_locations(Path(path))
        except ValueError as err:
            print(f"REFUSED: {err}", file=sys.stderr)
            return 2
        base = origin.rstrip("/")
        bad = [loc for loc in locations if loc != base and not loc.startswith(base + "/")]
        if not locations or bad:
            print(f"REFUSED: sitemap origin mismatch {bad[:1] or 'empty'}", file=sys.stderr)
            return 1
        print(f"public sitemap origin accepted ({len(locations)} URLs)")
        return 0
    artifact = Path(args.artifact or "").resolve()
    scope_path = Path(args.scope or "")
    try:
        scope = load_json(scope_path)
    except ValueError as err:
        print(f"REFUSED: {err}", file=sys.stderr)
        return 2
    scope_errors = validate_scope(scope) if isinstance(scope, dict) else ["scope must be an object"]
    if scope_errors:
        report = {"schema_version": SCHEMA, "status": "FAILED_SCOPE", "scope_errors": scope_errors,
                  "hard_failures": scope_errors, "external_requirements": []}
        write_report(args.out, report)
        for error in scope_errors:
            print(f"FAIL: {error}", file=sys.stderr)
        return 1
    if not (artifact / "portal-manifest.json").exists():
        print("REFUSED: artifact is not a portal candidate", file=sys.stderr)
        return 2
    failures, checks, manifest = verify_artifact(artifact, scope)
    external = [
        "named host/root ownership attestation is external and not verified by this script",
        "DNS/TLS/CDN ownership and CDN purge behavior are external",
        "public publication, atomic release switch and public smoke require explicit execution approval",
        "post-publication rollback/correction monitoring and sitemap/indexability update remain operational",
    ]
    status = "FAILED_LOCAL_GATE" if failures else "READY_FOR_EXECUTION_APPROVAL"
    report = {
        "schema_version": SCHEMA,
        "status": status,
        "artifact": {
            "path": str(artifact),
            "content_sha256": (manifest or {}).get("content_sha256"),
            "input_content_sha256": (manifest or {}).get("input_content_sha256"),
            "pages": (manifest or {}).get("pages"),
            "files": len((manifest or {}).get("files") or []) + 1,
        },
        "scope": {key: scope.get(key) for key in ("origin", "host", "root", "cdn", "tls", "approval_ref",
                                                  "approved_by", "window_start", "window_end", "source_slices")},
        "checks": [{"name": name, "status": status, "detail": detail} for name, status, detail in checks],
        "hard_failures": failures,
        "external_requirements": external,
        "publication_executed": False,
    }
    write_report(args.out, report)
    for name, status, detail in checks:
        print(f"{status}: {name}: {detail}")
    for failure in failures:
        print(f"FAIL: {failure}", file=sys.stderr)
    if failures:
        return 1
    print("LOCAL PREFLIGHT PASS: publication still requires execution approval and external ownership verification")
    return 3


def write_report(path, report):
    if not path:
        return
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(report, handle, indent=2)
        handle.write("\n")
    os.chmod(path, 0o600)


def self_test():
    assert valid_public_origin("https://portal.abastevo.com.br")[0]
    for value in ("http://portal.abastevo.com.br", "https://localhost", "https://10.0.0.1",
                  "https://portal.example", "https://portal.invalid", "https://portal.abastevo.com.br/path"):
        assert not valid_public_origin(value)[0], value
    assert placeholder("TBD") and placeholder("<origin>") and not placeholder("APR-2026-001")
    valid_scope = {
        "schema_version": SCOPE_SCHEMA, "origin": "https://portal.abastevo.com.br",
        "host": "vps-public-01", "root": "/srv/abastevo/portal", "cdn": "none",
        "tls": "acme", "approval_ref": "APR-2026-001", "approved_by": "project-owner",
        "window_start": "2026-11-01T12:00:00Z", "window_end": "2026-11-01T14:00:00Z",
        "candidate_content_sha256": "sha256:" + "a" * 64,
        "rollback_content_sha256": "sha256:" + "b" * 64,
        "source_slices": ["official-sanctions"],
        "source_admission_ref": "docs/product/ENTERPRISE_SOURCE_ADMISSION.md",
        "rights_evidence": ["CC BY metadata; event-level admitted"],
        "host_ownership_attestation": "ownership-record-001",
        "source_revision_id": "00000000-0000-4000-8000-0000000000f1",
        "source_lease_id": "b" * 64, "source_checksum": "c" * 64,
        "source_edition": "2025", "source_rows": 3375,
    }
    assert not validate_scope(valid_scope), validate_scope(valid_scope)
    bad_scope = dict(valid_scope, source_slices=["official-sanctions", "prices"])
    assert any("prices" in item for item in validate_scope(bad_scope))
    bad_scope = dict(valid_scope, origin="https://localhost")
    assert any("origin" in item for item in validate_scope(bad_scope))
    with tempfile.TemporaryDirectory() as raw:
        root = Path(raw)
        good = root / "sitemap.xml"
        good.write_text('<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://portal.abastevo.com.br/consultar/</loc></url></urlset>', encoding="utf-8")
        assert sitemap_locations(good) == ["https://portal.abastevo.com.br/consultar/"]
        (root / "precos").mkdir()
        assert (root / "precos").is_dir()
    print("publication preflight self-test ok")
    return 0


def main(argv):
    parser = argparse.ArgumentParser()
    parser.add_argument("--artifact")
    parser.add_argument("--scope")
    parser.add_argument("--out")
    parser.add_argument("--check-origin")
    parser.add_argument("--check-sitemap-origin", nargs=2, metavar=("ORIGIN", "SITEMAP"))
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args(argv)
    try:
        return run(args)
    except ValueError as err:
        print(f"REFUSED: {err}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
