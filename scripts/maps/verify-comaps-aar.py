#!/usr/bin/env python3
"""Verify a Gradle release AAR and publish an unpacked, measured candidate."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import xml.etree.ElementTree as ET
import zipfile


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify(args):
    output = args.output.absolute()
    if output.exists():
        raise ValueError("output exists; choose a new directory")
    if not output.parent.is_dir():
        raise ValueError("output parent directory does not exist")
    stage = Path(tempfile.mkdtemp(prefix=".comaps-release-", dir=output.parent))
    try:
        with zipfile.ZipFile(args.aar) as archive:
            for name in archive.namelist():
                if not (stage / name).resolve().is_relative_to(stage):
                    raise ValueError("unsafe AAR entry")
            archive.extractall(stage)
        manifest = ET.parse(stage / "AndroidManifest.xml").getroot()
        uses_sdk = manifest.find("uses-sdk")
        android_ns = "{http://schemas.android.com/apk/res/android}"
        if uses_sdk is None or uses_sdk.get(android_ns + "minSdkVersion") != "26":
            raise ValueError("release AAR must declare minSdk 26")
        native = sorted((stage / "jni").glob("*/*.so"))
        if not native or any(path.parent.name != args.abi for path in native):
            raise ValueError("unexpected or missing native ABI")
        if not any(path.name == "liborganicmaps.so" for path in native):
            raise ValueError("native core missing")
        if any("qt" in path.name.lower() for path in native):
            raise ValueError("Qt runtime must not ship")
        javap = Path(os.environ["JAVA_HOME"]) / "bin/javap"
        constants = subprocess.check_output(
            [str(javap), "-constants", "-classpath", str(stage / "classes.jar"),
             "app.organicmaps.sdk.BuildConfig"], text=True,
        )
        if "boolean DEBUG = false;" not in constants:
            raise ValueError("debug or unknown Java classes; hybrid AAR refused")
        readelf = args.ndk / "toolchains/llvm/prebuilt/linux-x86_64/bin/llvm-readelf"
        libraries = []
        for path in native:
            headers = subprocess.check_output([str(readelf), "-lW", str(path)], text=True)
            loads = [int(line.split()[-1], 16) for line in headers.splitlines()
                     if line.lstrip().startswith("LOAD ")]
            if not loads or (args.abi != "armeabi-v7a" and min(loads) < 16384):
                raise ValueError("64-bit LOAD segments must align to at least 16 KB")
            sections = subprocess.check_output([str(readelf), "-SW", str(path)], text=True)
            if re.search(r"\.(debug_info|symtab)\s", sections):
                raise ValueError("unstripped native release library")
            libraries.append({"path": str(path.relative_to(stage)), "bytes": path.stat().st_size,
                              "sha256": digest(path), "load_alignments": loads})
        shutil.copy2(args.aar, stage / "sdk-release.aar")
        report = {"upstream_sha": "4587b8ad4d12c9f59982709fdaf03cc925948827",
                  "ndk": "28.2.13676358", "cmake": "3.22.1", "min_sdk": 26,
                  "abi": args.abi, "java_debug": False, "aar_bytes": args.aar.stat().st_size,
                  "aar_sha256": digest(args.aar), "native": libraries,
                  "engine_gate": "PARTIAL", "licenses": "AUDIT_PENDING",
                  "runtime_16kb_acceptance": "PENDING"}
        (stage / "build.json").write_text(json.dumps(report, indent=2) + "\n")
        stage.rename(output)
        print(json.dumps(report, indent=2))
    finally:
        if stage.exists():
            shutil.rmtree(stage)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--aar", required=True, type=Path)
    parser.add_argument("--abi", required=True, choices=("arm64-v8a", "armeabi-v7a", "x86_64"))
    parser.add_argument("--ndk", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    options = parser.parse_args()
    try:
        verify(options)
    except (ValueError, OSError, ET.ParseError, zipfile.BadZipFile, subprocess.CalledProcessError) as error:
        parser.exit(1, f"Invalid CoMaps release candidate: {error}\n")
