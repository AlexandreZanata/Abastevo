#!/usr/bin/env python3
"""Draft MAP-14 T07 inventory. This is evidence, not a legal opinion."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

LICENSE_NAMES = {name.upper() for name in ("LICENSE", "LICENSE.txt", "LICENSE.md", "LICENSE_1_0.txt", "LICENCE", "LICENCE.txt", "COPYING", "COPYING.txt", "COPYING.LESSER", "NOTICE", "NOTICE.txt")}
URL = re.compile(rb"https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]{4,200}")
SECRET = re.compile(rb"(AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|sk-[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{10,})")


def sha(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def inventory(source):
    rows = []
    for line in subprocess.check_output(["git", "-C", str(source), "submodule", "status", "--recursive"], text=True).splitlines():
        status = line[0]
        parts = line[1:].split()
        if len(parts) < 2:
            continue
        commit, relative = parts[0], parts[1]
        path = source / relative
        licenses = [p for p in path.iterdir() if p.is_file() and p.name.upper() in LICENSE_NAMES] if path.is_dir() else []
        if not licenses and path.is_dir():
            licenses = [p for p in path.rglob("*") if p.is_file() and p.name.upper() in LICENSE_NAMES][:1]
        inherited = None
        if not licenses:
            parent = path.parent
            while parent != source and parent.is_relative_to(source):
                found = [p for p in parent.iterdir() if p.is_file() and p.name.upper() in LICENSE_NAMES]
                if found:
                    inherited = parent
                    licenses = found
                    break
                parent = parent.parent
        rows.append({
            "path": relative, "commit": commit, "clean": status == " ",
            "licenses": [{"name": p.name, "sha256": sha(p)} for p in licenses],
            "license_status": "PRESENT" if licenses else "MISSING",
            "license_scope": "OWN" if inherited is None and licenses else ("INHERITED:" + str(inherited.relative_to(source)) if inherited else "MISSING"),
        })
    return rows


def scan(path):
    data = path.read_bytes() if path.is_file() else b""
    return {
        "sha256": sha(path),
        "bytes": len(data),
        "url_hosts": sorted({m.group().decode("utf-8", "replace").split("/")[2] for m in URL.finditer(data)}),
        "secret_pattern_hits": [m.group().decode("ascii", "replace")[:8] + "…" for m in SECRET.finditer(data)],
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--candidate", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    source = args.source.resolve()
    candidate = args.candidate.resolve()
    build = json.loads((candidate / "build.json").read_text())
    if build.get("upstream_sha") != "4587b8ad4d12c9f59982709fdaf03cc925948827":
        raise SystemExit("wrong upstream pin")
    submodules = inventory(source)
    missing = [row["path"] for row in submodules if row["license_status"] != "PRESENT"]
    artifacts = {}
    for path in [candidate / "sdk-release.aar", candidate / "classes.jar", *sorted((candidate / "jni").glob("*/*.so"))]:
        if path.exists():
            artifacts[str(path.relative_to(candidate))] = scan(path)
    report = {
        "status": "DRAFT_NOT_LEGAL_ACCEPTANCE",
        "upstream_sha": build["upstream_sha"],
        "candidate": str(candidate),
        "candidate_build": build,
        "submodules_total": len(submodules),
        "submodules_clean": sum(1 for row in submodules if row["clean"]),
        "license_files_present": sum(1 for row in submodules if row["license_status"] == "PRESENT"),
        "license_files_missing": missing,
        "submodules": submodules,
        "artifacts": artifacts,
        "notice_files_required": ["LICENSE", "NOTICE", "data/copyright.html"],
        "attribution_required": "ODbL/OSM and every shipped third-party notice",
        "legal_review": "PENDING",
    }
    args.out.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({k: report[k] for k in ("status", "submodules_total", "submodules_clean", "license_files_present", "legal_review")}, indent=2))
    print(f"license files missing for {len(missing)} submodules; see {args.out}")


if __name__ == "__main__":
    main()
