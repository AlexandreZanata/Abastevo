#!/usr/bin/env python3
"""Combine two verified single-ABI CoMaps candidates into one multi-ABI candidate."""

import argparse
import hashlib
import json
import shutil
import zipfile
from pathlib import Path


def sha(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--arm64", required=True, type=Path)
    parser.add_argument("--armv7", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    arm64 = args.arm64.resolve()
    armv7 = args.armv7.resolve()
    output = args.output.resolve()
    if output.exists():
        raise SystemExit(f"output exists: {output}")
    arm64_build = json.loads((arm64 / "build.json").read_text())
    armv7_build = json.loads((armv7 / "build.json").read_text())
    if arm64_build["upstream_sha"] != armv7_build["upstream_sha"]:
        raise SystemExit("upstream SHA mismatch")
    if arm64_build["abi"] != "arm64-v8a" or armv7_build["abi"] != "armeabi-v7a":
        raise SystemExit("expected arm64-v8a and armeabi-v7a candidates")
    for candidate in (arm64, armv7):
        audit = json.loads((candidate / "audit.json").read_text())
        if audit.get("status") != "LOCAL_SOURCE_ADMISSION_PASS":
            raise SystemExit("source admission must pass before ABI merge")
    with zipfile.ZipFile(arm64 / "sdk-release.aar") as left, zipfile.ZipFile(armv7 / "sdk-release.aar") as right:
        left_assets = {name: left.read(name) for name in left.namelist() if not name.startswith("jni/")}
        right_assets = {name: right.read(name) for name in right.namelist() if not name.startswith("jni/")}
        if left_assets != right_assets:
            raise SystemExit("code/resources/notices differ between ABI candidates")
    for name in ("classes.jar", "AndroidManifest.xml"):
        if sha(arm64 / name) != sha(armv7 / name):
            raise SystemExit(f"{name} differs between ABI candidates")

    shutil.copytree(arm64, output)
    shutil.copytree(armv7 / "jni/armeabi-v7a", output / "jni/armeabi-v7a")

    with zipfile.ZipFile(arm64 / "sdk-release.aar") as src, zipfile.ZipFile(output / "sdk-release.aar", "w") as dst:
        for item in sorted(src.infolist(), key=lambda entry: entry.filename):
            info = zipfile.ZipInfo(item.filename, date_time=(2026, 10, 24, 0, 0, 0))
            info.compress_type = item.compress_type
            info.external_attr = item.external_attr
            dst.writestr(info, src.read(item.filename))
        for path in sorted((output / "jni/armeabi-v7a").glob("*.so")):
            info = zipfile.ZipInfo(f"jni/armeabi-v7a/{path.name}", date_time=(2026, 10, 24, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            dst.writestr(info, path.read_bytes())

    report = {
        "upstream_sha": arm64_build["upstream_sha"],
        "abis": ["arm64-v8a", "armeabi-v7a"],
        "arm64_aar_sha256": sha(arm64 / "sdk-release.aar"),
        "armv7_aar_sha256": sha(armv7 / "sdk-release.aar"),
        "multi_aar_sha256": sha(output / "sdk-release.aar"),
        "native": [
            {
                "abi": abi,
                "bytes": (output / "jni" / abi / "liborganicmaps.so").stat().st_size,
                "sha256": sha(output / "jni" / abi / "liborganicmaps.so"),
            }
            for abi in ("arm64-v8a", "armeabi-v7a")
        ],
        "engine_gate": "NOT_EVALUATED_BY_STATIC_BUILDER",
        "licenses": "LOCAL_SOURCE_ADMISSION_PASS" if arm64_build.get("resource_admission") == "PASS" and armv7_build.get("resource_admission") == "PASS" else "SOURCE_AUDIT_REQUIRED",
        "runtime_16kb_acceptance": "NOT_EVALUATED_BY_STATIC_BUILDER",
    }
    build = {key: value for key, value in arm64_build.items() if key not in ("abi", "native", "aar_sha256", "aar_bytes")}
    build.update({"abis": report["abis"], "aar_sha256": report["multi_aar_sha256"],
                  "aar_bytes": (output / "sdk-release.aar").stat().st_size,
                  "native": [{"path": f"jni/{item['abi']}/liborganicmaps.so", **item} for item in report["native"]],
                  "licenses": report["licenses"]})
    (output / "build.json").write_text(json.dumps(build, indent=2) + "\n")
    (output / "multi-build.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
