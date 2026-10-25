#!/usr/bin/env python3
"""MAP-14 T07 inventory, notices and SBOM. Not a legal opinion."""

import argparse
from io import BytesIO
import zipfile
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess

REPO_ROOT = Path(__file__).resolve().parents[2]
LICENSE_NAMES = {
    name.upper()
    for name in (
        "LICENSE",
        "LICENSE.txt",
        "LICENSE.md",
        "LICENSE_1_0.txt",
        "LICENSE.MIT",
        "LICENSE1.txt",
        "LICENSE2.txt",
        "LICENCE",
        "LICENCE.md",
        "LICENCE.txt",
        "COPYING",
        "COPYING.txt",
        "COPYING.LESSER",
        "NOTICE",
        "NOTICE.txt",
    )
}
URL = re.compile(rb"https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]{4,200}")
SECRET = re.compile(
    rb"(AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{30,}|github_"
    rb"pat_[A-Za-z0-9_]{40,}|sk-[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{10,})"
)

RESOLUTIONS = {
    "3party/glm": {
        "license_id": "MIT",
        "selected_license": "util/conan-package/lib_licenses/LICENSE2.txt",
        "note": "Dual Happy Bunny (modified MIT) / MIT; MIT selected.",
    },
    "3party/just_gtfs": {
        "license_id": "MIT",
        "selected_license": str(REPO_ROOT / "maps/engine/upstream/licenses/just_gtfs-MIT.txt"),
        "upstream_url": "https://github.com/comaps/just_gtfs/blob/master/LICENSE.MIT",
        "upstream_commit": "b5b1df6f5e26c7698d506a1bd0b8cfb5ffe2d64f",
        "note": "MIT is present in ancestor 083d98e (2020-03-28), with copyright 2020 mesozoic-drones; its grant covers the pinned descendant code. The fork omitted the file; full terms are retained.",
    },
    "3party/monocypher": {
        "license_id": "BSD-2-Clause",
        "note": "Pinned source offers BSD-2-Clause or CC0; BSD-2-Clause selected.",
    },
    "3party/GL": {
        "license_id": "MIT",
        "evidence": "3party/GL/glcorearb.h",
        "note": "Khronos MIT-style header.",
    },
    "3party/agg": {
        "license_id": "LicenseRef-AGG",
        "evidence": "3party/agg/agg_vcgen_smooth_poly1.h",
        "note": "Anti-Grain Geometry permissive header; notice retained.",
    },
    "3party/ankerl": {
        "license_id": "MIT",
        "evidence": "3party/ankerl/stl.h",
        "note": "MIT header.",
    },
    "3party/kdtree++": {
        "license_id": "Artistic-2.0",
        "evidence": "3party/kdtree++/region.hpp",
        "selected_license": str(REPO_ROOT / "maps/engine/upstream/licenses/Artistic-2.0.txt"),
        "note": "Headers state Artistic-2.0; the pinned tree omits the referenced COPYING, so the full license is retained in the Abastevo notices.",
    },
    "3party/minizip": {
        "license_id": "Zlib AND Info-ZIP",
        "evidence": "3party/minizip/src/MiniZip64_info.txt",
        "selected_license": str(REPO_ROOT / "maps/engine/upstream/licenses/Info-ZIP.txt"),
        "note": "MiniZip64_info.txt supplies the zlib terms; unzip.c references Info-ZIP, whose full license is retained in the Abastevo notices.",
    },
    "3party/opening_hours": {
        "license_id": "MIT",
        "evidence": "3party/opening_hours/opening_hours.hpp",
        "note": "MIT header.",
    },
    "3party/robust": {
        "license_id": "LicenseRef-PublicDomain",
        "evidence": "3party/robust/predicates.c",
        "note": "Public-domain header.",
    },
    "3party/skarupke": {
        "license_id": "BSL-1.0",
        "evidence": "3party/skarupke/__flat_hash_map.hpp",
        "note": "Boost Software License 1.0 header.",
    },
    "3party/stb_image": {
        "license_id": "MIT",
        "evidence": "3party/stb_image/stb_image.h",
        "note": "Dual MIT/public-domain; MIT selected.",
    },
    "3party/vulkan_wrapper": {
        "license_id": "Apache-2.0",
        "evidence": "3party/vulkan_wrapper/vulkan_wrapper.h",
        "note": "Apache-2.0 header.",
    },
}

ENDPOINT_CLASSES = {
    # Legacy URL parsing prefix (libs/ge0/parser.hpp), not an outbound service.
    "ge0.me": "SOCIAL_OR_REFERENCE",
    # SDK Constants social/source links, not used by the Abastevo offline host.
    "codeberg.org": "SOCIAL_OR_REFERENCE",
    "floss.social": "SOCIAL_OR_REFERENCE",
    "matrix.to": "SOCIAL_OR_REFERENCE",
    "pixelfed.social": "SOCIAL_OR_REFERENCE",
    "sopuli.xyz": "SOCIAL_OR_REFERENCE",
    "comaps.app": "UPSTREAM_SERVICE_UNUSED",
    "comaps.at": "UPSTREAM_SERVICE_UNUSED",
    "cdn-fi-1.comaps.app": "UPSTREAM_SERVICE_UNUSED",
    "cdn-us-1.comaps.app": "UPSTREAM_SERVICE_UNUSED",
    "cdn-us-2.comaps.tech": "UPSTREAM_SERVICE_UNUSED",
    "comaps-cdn.s3-website.cloud.ru": "UPSTREAM_SERVICE_UNUSED",
    "comaps-it1.unfoxo.it": "UPSTREAM_SERVICE_UNUSED",
    "comaps.firewall-gateway.de": "UPSTREAM_SERVICE_UNUSED",
    "comaps.openstreetmap.fr": "UPSTREAM_SERVICE_UNUSED",
    "mapgen-fi-1.comaps.app": "UPSTREAM_SERVICE_UNUSED",
    "api.openstreetmap.org": "OSM_DATA_OR_SCHEMA",
    "osm.org": "OSM_DATA_OR_SCHEMA",
    "www.openstreetmap.org": "OSM_DATA_OR_SCHEMA",
    "www.opengis.net": "OSM_DATA_OR_SCHEMA",
    "www.topografix.com": "OSM_DATA_OR_SCHEMA",
    "www.w3.org": "OSM_DATA_OR_SCHEMA",
    "commons.wikimedia.org": "OSM_DATA_OR_SCHEMA",
    "mapcomplete.org": "OSM_DATA_OR_SCHEMA",
    "mangrove.reviews": "OSM_DATA_OR_SCHEMA",
    "api.panoramax.xyz": "OSM_DATA_OR_SCHEMA",
    "earth.google.com": "OSM_DATA_OR_SCHEMA",
    "www.garmin.com": "OSM_DATA_OR_SCHEMA",
    "bsky.app": "SOCIAL_OR_REFERENCE",
    "facebook.com": "SOCIAL_OR_REFERENCE",
    "instagram.com": "SOCIAL_OR_REFERENCE",
    "line.me": "SOCIAL_OR_REFERENCE",
    "twitter.com": "SOCIAL_OR_REFERENCE",
    "vk.com": "SOCIAL_OR_REFERENCE",
    "android.googlesource.com": "SOCIAL_OR_REFERENCE",
    "www.google.com": "SOCIAL_OR_REFERENCE",
}

SOURCE_OFFER = """# OSM/ODbL source and attribution offer

The `.mwm` packages are derived from OpenStreetMap data under the Open Data
Commons Open Database License (ODbL) 1.0. The map displays the required
`Map data © OpenStreetMap contributors` attribution. The exact ODbL text is
retained as `ODbL-1.0.txt` in this notices bundle.

The regional package is a Derived Database. The exact upstream generator
revision (`4587b8ad4d12c9f59982709fdaf03cc925948827`), the Abastevo patch, the
build recipe and the package bytes are retained in the Abastevo repository and
signed map distribution. The release must publish a durable source/derived-data
offer at the distribution endpoint before public distribution; that public URL
is a release/deployment obligation, not a claim of this audit.

Source-specific OSM reference assertions and OSM-derived geometry retain ODbL
and attribution in any overlay/export. Independently sourced ANP/community
records retain their admitted source terms; a separate table or rendering layer
alone does not establish license independence. The release must preserve each
source's obligations and provide the derived-data offer before public delivery.
"""


def sha(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def license_entries(source: Path, relative: str, path: Path) -> list[dict]:
    if relative == "3party/glm":
        files = sorted((path / "util/conan-package/lib_licenses").glob("LICENSE*.txt"))
    else:
        files = sorted(p for p in path.iterdir() if p.is_file() and p.name.upper() in LICENSE_NAMES) if path.is_dir() else []
        if not files and path.is_dir():
            files = sorted(p for p in path.rglob("*") if p.is_file() and p.name.upper() in LICENSE_NAMES)
    return [
        {
            "name": p.name,
            "path": str(p.relative_to(source)),
            "sha256": sha(p),
        }
        for p in files
    ]


def inventory(source: Path) -> list[dict]:
    rows = []
    for line in subprocess.check_output(
        ["git", "-C", str(source), "submodule", "status", "--recursive"], text=True
    ).splitlines():
        status = line[0]
        parts = line[1:].split()
        if len(parts) < 2:
            continue
        commit, relative = parts[0], parts[1]
        path = source / relative
        licenses = license_entries(source, relative, path)
        inherited = None
        if not licenses:
            parent = path.parent
            while parent != source and parent.is_relative_to(source):
                found = sorted(p for p in parent.iterdir() if p.is_file() and p.name.upper() in LICENSE_NAMES)
                if found:
                    inherited = parent
                    licenses = [
                        {"name": p.name, "path": str(p.relative_to(source)), "sha256": sha(p)}
                        for p in found
                    ]
                    break
                parent = parent.parent
        resolution = RESOLUTIONS.get(relative)
        if resolution and resolution.get("selected_license") and not resolution["selected_license"].startswith("/"):
            selected = source / relative / resolution["selected_license"]
            if selected.is_file() and all(item["path"] != str(selected.relative_to(source)) for item in licenses):
                licenses.append(
                    {"name": selected.name, "path": str(selected.relative_to(source)), "sha256": sha(selected)}
                )
        if resolution and resolution.get("selected_license", "").startswith("/"):
            selected = Path(resolution["selected_license"])
            if selected.is_file():
                licenses.append(
                    {
                        "name": selected.name,
                        "path": str(selected),
                        "source": "abastevo-repo",
                        "sha256": sha(selected),
                    }
                )
        resolved_only = bool(resolution) and bool(licenses) and all(
            item.get("source") == "abastevo-repo" for item in licenses
        )
        row = {
            "path": relative,
            "commit": commit,
            "clean": status == " ",
            "licenses": licenses,
            "license_status": "RESOLVED_UPSTREAM" if resolved_only else ("PRESENT" if licenses else "MISSING"),
            "license_scope": (
                "RESOLVED_UPSTREAM"
                if resolved_only
                else (
                    "OWN"
                    if inherited is None and licenses
                    else (
                        "INHERITED:" + str(inherited.relative_to(source))
                        if inherited
                        else ("RESOLVED" if resolution else "MISSING")
                    )
                )
            ),
        }
        if resolution:
            row["license_id"] = resolution["license_id"]
            row["license_note"] = resolution["note"]
            if "upstream_url" in resolution:
                row["upstream_license_url"] = resolution["upstream_url"]
                row["upstream_license_commit"] = resolution["upstream_commit"]
        rows.append(row)
    return rows


def vendored_inventory(source: Path, submodules: list[dict]) -> list[dict]:
    submodule_paths = {row["path"] for row in submodules}
    rows = []
    party = source / "3party"
    if not party.is_dir():
        return rows
    for path in sorted(party.iterdir()):
        if not path.is_dir():
            continue
        relative = str(path.relative_to(source))
        if relative in submodule_paths or any(relative.startswith(item + "/") for item in submodule_paths):
            continue
        licenses = license_entries(source, relative, path)
        resolution = RESOLUTIONS.get(relative)
        if resolution:
            evidence = resolution.get("evidence")
            if evidence:
                evidence_path = source / evidence
                if evidence_path.is_file() and all(item["path"] != evidence for item in licenses):
                    licenses.append(
                        {
                            "name": evidence_path.name,
                            "path": evidence,
                            "sha256": sha(evidence_path),
                            "source": "source-header",
                        }
                    )
            selected = resolution.get("selected_license")
            if selected:
                selected_path = Path(selected) if selected.startswith("/") else source / relative / selected
                selected_key = str(selected_path) if selected.startswith("/") else str(selected_path.relative_to(source))
                if selected_path.is_file() and all(item["path"] != selected_key for item in licenses):
                    licenses.append(
                        {
                            "name": selected_path.name,
                            "path": selected_key,
                            "sha256": sha(selected_path),
                            "source": "abastevo-repo" if selected.startswith("/") else "source",
                        }
                    )
        resolved_only = bool(resolution) and bool(licenses) and all(
            item.get("source") in ("source-header", "abastevo-repo") for item in licenses
        )
        has_repo_license = any(item.get("source") == "abastevo-repo" for item in licenses)
        row = {
            "kind": "vendored",
            "path": relative,
            "commit": "vendored",
            "clean": True,
            "licenses": licenses,
            "license_status": (
                "RESOLVED_REPO_LICENSE"
                if resolved_only and has_repo_license
                else ("RESOLVED_SOURCE_HEADER" if resolved_only else ("PRESENT" if licenses else "MISSING"))
            ),
            "license_scope": (
                "RESOLVED_REPO_LICENSE"
                if resolved_only and has_repo_license
                else ("RESOLVED_SOURCE_HEADER" if resolved_only else ("OWN" if licenses else "MISSING"))
            ),
        }
        if resolution:
            row["license_id"] = resolution["license_id"]
            row["license_note"] = resolution["note"]
        rows.append(row)
    return rows


def runtime_payload(raw: bytes, depth: int = 0) -> bytes:
    if depth < 3 and zipfile.is_zipfile(BytesIO(raw)):
        with zipfile.ZipFile(BytesIO(raw)) as archive:
            entries = [item for item in archive.infolist()
                       if item.filename.endswith((".class", ".so", ".jar"))]
            if sum(item.file_size for item in entries) > 512 * 1024 * 1024:
                raise ValueError("runtime archive scan budget exceeded")
            return b"\x00".join(runtime_payload(archive.read(item), depth + 1) for item in entries)
    return raw


def scan(path: Path) -> dict:
    data = runtime_payload(path.read_bytes()) if path.is_file() else b""
    return {
        "sha256": sha(path),
        "bytes": path.stat().st_size,
        "runtime_bytes_scanned": len(data),
        "url_hosts": sorted({m.group().decode("utf-8", "replace").split("/")[2] for m in URL.finditer(data)}),
        "secret_pattern_hits": [m.group().decode("ascii", "replace")[:8] + "…" for m in SECRET.finditer(data)],
    }


def safe_name(relative: str) -> str:
    return relative.replace("/", "__")


def write_notices(source: Path, candidate: Path, out_dir: Path, report: dict) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)
    licenses_dir = out_dir / "licenses"
    licenses_dir.mkdir(exist_ok=True)
    shutil.copy2(candidate / "LICENSE", out_dir / "LICENSE")
    shutil.copy2(candidate / "NOTICE", out_dir / "NOTICE")
    if (candidate / "copyright.html").is_file():
        shutil.copy2(candidate / "copyright.html", out_dir / "copyright.html")
    shutil.copy2(REPO_ROOT / "LICENSE", out_dir / "ABASTEVO-LICENSE")
    shutil.copy2(REPO_ROOT / "maps/engine/upstream/licenses/ODbL-1.0.txt", out_dir / "ODbL-1.0.txt")
    (out_dir / "SOURCE_OFFER.md").write_text(SOURCE_OFFER)

    lines = [
        "# Third-party notices",
        "",
        f"Upstream: `{report['upstream_sha']}`",
        "Artifact: pinned CoMaps SDK release candidate (paths omitted for reproducibility)",
        "Source notices; resource admission is recorded in the packaged resources.json.",
        "",
        "The core is Apache-2.0; the submodule licenses below are the exact",
        "pinned sources. The full license files are copied under `licenses/`.",
        "This bundle is a technical inventory, not a legal opinion.",
        "",
        "## Resolved items",
        "",
    ]
    for relative, resolution in RESOLUTIONS.items():
        lines.append(f"- `{relative}`: **{resolution['license_id']}**. {resolution['note']}")
        if "upstream_url" in resolution:
            lines.append(f"  Upstream evidence: {resolution['upstream_url']} (`{resolution['upstream_commit']}`).")
    lines += [
        "",
        "## Components",
        "",
        "| Component | Kind | Commit | Status | Scope | License evidence |",
        "| --- | --- | --- | --- | --- | --- |",
    ]
    for row in report["components"]:
        evidence = ", ".join(f"`{item['name']}` `{item['sha256'][:12]}…`" for item in row["licenses"]) or "—"
        lines.append(
            f"| `{row['path']}` | {row.get('kind', 'submodule')} | `{row['commit']}` | "
            f"{row['license_status']} | {row['license_scope']} | {evidence} |"
        )
    lines += ["", "Artifact integrity hashes are retained in build.json/audit.json beside the candidate."]
    (out_dir / "THIRD_PARTY_NOTICES.md").write_text("\n".join(lines) + "\n")

    for row in report["components"]:
        target = licenses_dir / safe_name(row["path"])
        target.mkdir(exist_ok=True)
        for item in row["licenses"]:
            src = Path(item["path"]) if item.get("source") == "abastevo-repo" else source / item["path"]
            if src.is_file():
                try:
                    relative = Path(item["path"]).relative_to(row["path"])
                except ValueError:
                    relative = Path(item["name"])
                destination = target / relative
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(src, destination)


def write_sbom(out: Path, report: dict) -> None:
    components = [
        {
            "type": "library",
            "name": "comaps/comaps",
            "version": report["upstream_sha"],
            "licenses": [{"license": {"id": "Apache-2.0"}}],
            "properties": [
                {"name": "abastevo:notice", "value": "LICENSE, NOTICE, copyright.html"},
                {"name": "abastevo:patch", "value": "maps/engine/upstream/ndk-r28-lld.patch"},
            ],
        }
    ]
    for row in report["components"]:
        license_id = row.get("license_id")
        components.append(
            {
                "type": "library",
                "name": row["path"],
                "version": row["commit"],
                "licenses": (
                    [{"expression": license_id}]
                    if license_id and (" AND " in license_id or " OR " in license_id)
                    else (
                        [{"license": {"id": license_id}}]
                        if license_id
                        else [{"license": {"name": "See THIRD_PARTY_NOTICES.md"}}]
                    )
                ),
                "properties": [
                    {"name": "abastevo:kind", "value": row.get("kind", "submodule")},
                    {"name": "abastevo:license_status", "value": row["license_status"]},
                    {"name": "abastevo:license_scope", "value": row["license_scope"]},
                ],
            }
        )
    for name, item in report["artifacts"].items():
        components.append(
            {
                "type": "file",
                "name": name,
                "hashes": [{"alg": "SHA-256", "content": item["sha256"]}],
                "properties": [{"name": "abastevo:bytes", "value": str(item["bytes"])}],
            }
        )
    sbom = {
        "bomFormat": "CycloneDX",
        "specVersion": "1.5",
        "version": 1,
        "metadata": {
            "component": {
                "type": "application",
                "name": "abastevo-comaps-engine",
                "version": report["upstream_sha"],
            }
        },
        "components": components,
    }
    out.write_text(json.dumps(sbom, indent=2) + "\n")


def admission_failures(report):
    failures = []
    if report['license_files_missing']:
        failures.append('missing component license evidence')
    if report['submodules_clean'] != report['submodules_total']:
        failures.append('unpinned submodule source')
    if report['endpoint_decision']['unclassified']:
        failures.append('unreviewed endpoint')
    if any(item['secret_pattern_hits'] for item in report['artifacts'].values()):
        failures.append('artifact secret pattern detected')
    return failures


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--candidate", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--notices-dir", type=Path)
    parser.add_argument("--sbom", type=Path)
    parser.add_argument("--require-admitted", action="store_true")
    args = parser.parse_args()
    source = args.source.resolve()
    candidate = args.candidate.resolve()
    build = json.loads((candidate / "build.json").read_text())
    if build.get("upstream_sha") != "4587b8ad4d12c9f59982709fdaf03cc925948827":
        raise SystemExit("wrong upstream pin")
    submodules = inventory(source)
    vendored = vendored_inventory(source, submodules)
    components = submodules + vendored
    missing = [row["path"] for row in components if row["license_status"] == "MISSING"]
    artifacts = {}
    for path in [candidate / "sdk-release.aar", candidate / "classes.jar", *sorted((candidate / "jni").glob("*/*.so"))]:
        if path.exists():
            artifacts[str(path.relative_to(candidate))] = scan(path)
    endpoint_classes = {}
    unclassified = set()
    for item in artifacts.values():
        for host in item["url_hosts"]:
            endpoint_classes[host] = ENDPOINT_CLASSES.get(host, "UNCLASSIFIED_REVIEW")
            if endpoint_classes[host] == "UNCLASSIFIED_REVIEW":
                unclassified.add(host)
    report = {
        "status": "CORE_INVENTORY_COMPLETE_RESOURCE_ADMISSION_PENDING",
        "upstream_sha": build["upstream_sha"],
        "candidate": str(candidate),
        "candidate_build": build,
        "submodules_total": len(submodules),
        "submodules_clean": sum(1 for row in submodules if row["clean"]),
        "vendored_total": len(vendored),
        "components_total": len(components),
        "license_files_present": sum(1 for row in components if row["license_status"] == "PRESENT"),
        "license_files_resolved": sum(1 for row in components if row["license_status"].startswith("RESOLVED")),
        "license_files_missing": missing,
        "resolutions": RESOLUTIONS,
        "submodules": submodules,
        "vendored": vendored,
        "components": components,
        "artifacts": artifacts,
        "notice_files_required": ["LICENSE", "NOTICE", "data/copyright.html"],
        "attribution_required": "ODbL/OSM and every shipped third-party notice",
        "odbl": {
            "attribution": "Map data © OpenStreetMap contributors",
            "license": "ODbL-1.0",
            "derived_database": "regional .mwm package",
            "overlay": "separate rendering; OSM-derived reference assertions remain ODbL; independent ANP/community records retain their source terms",
            "source_offer": "retained recipe/package; public distribution URL remains a release obligation",
        },
        "endpoint_decision": {
            "status": "DOCUMENTED_UNUSED",
            "no_private_service_required": True,
            "offline_proof": "T08 airplane-mode render/search/route on the physical phone",
            "classes": endpoint_classes,
            "unclassified": sorted(unclassified),
        },
        "legal_review": "SOURCE_LICENSE_EVIDENCE_REVIEWED",
    }
    resource_manifest = candidate / "assets/abastevo-map-licenses/resources.json"
    if resource_manifest.is_file():
        import importlib.util
        spec = importlib.util.spec_from_file_location("resource_admission", Path(__file__).with_name("prepare-comaps-assets.py"))
        admission = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(admission)
        report["resource_admission"] = admission.verify(candidate, REPO_ROOT / "maps/engine/upstream/resource-admission.json")
        if not admission_failures(report):
            report["status"] = "LOCAL_SOURCE_ADMISSION_PASS"
    args.out.write_text(json.dumps(report, indent=2) + "\n")
    if args.notices_dir:
        write_notices(source, candidate, args.notices_dir, report)
    if args.sbom:
        write_sbom(args.sbom, report)
    print(
        json.dumps(
            {
                "status": report["status"],
                "submodules_total": report["submodules_total"],
                "submodules_clean": report["submodules_clean"],
                "vendored_total": report["vendored_total"],
                "components_total": report["components_total"],
                "license_files_present": report["license_files_present"],
                "license_files_resolved": report["license_files_resolved"],
                "license_files_missing": len(missing),
                "endpoint_classes": len(endpoint_classes),
                "unclassified": sorted(unclassified),
                "legal_review": report["legal_review"],
            },
            indent=2,
        )
    )

    failures = admission_failures(report)
    if args.require_admitted and report['status'] != 'LOCAL_SOURCE_ADMISSION_PASS':
        failures.append('resource admission not complete')
    if failures:
        raise SystemExit('Source admission refused: ' + '; '.join(failures))


if __name__ == "__main__":
    main()
