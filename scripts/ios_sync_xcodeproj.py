#!/usr/bin/env python3
"""Register iOS Swift sources and String Catalogs without Xcode.

The project file is maintained by hand on Linux hosts, where XcodeGen and
Xcode are unavailable. New files are included in the appropriate target's
Sources or Resources phase. Relative paths determine their object IDs, so the
Shell and Core catalogs may share the same filename.

Usage:
  python3 scripts/ios_sync_xcodeproj.py          # rewrite the project
  python3 scripts/ios_sync_xcodeproj.py --check  # exit 1 when a file is missing
"""

from __future__ import annotations

import hashlib
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
IOS_ROOT = ROOT / "iosApp"
PROJECT = IOS_ROOT / "AnpFuel.xcodeproj" / "project.pbxproj"
FOLDERS = ("Sources/AnpFuelCore", "Sources/AnpFuelShell", "Tests/AnpFuelCoreTests")
RESOURCE_FOLDERS = (
    ("App", "App", "AnpFuel"),
    ("Sources/AnpFuelCore", "Sources/AnpFuelCore", "AnpFuelCore"),
    ("Sources/AnpFuelCore/Resources", "Sources/AnpFuelCore", "AnpFuelCore"),
    ("Sources/AnpFuelShell", "Sources/AnpFuelShell", "AnpFuel"),
    ("Tests/AnpFuelCoreTests", "Tests/AnpFuelCoreTests", "AnpFuelCoreTests"),
)


def object_id(kind: str, relative: str) -> str:
    return hashlib.sha1(f"anpfuel-ios:{kind}:{relative}".encode()).hexdigest()[:24].upper()


def referenced_names(text: str) -> set[str]:
    return set(re.findall(r"path = ([^;\"]+\.swift);", text))


def group_id(text: str, folder_name: str) -> str:
    match = re.search(rf"\t\t([A-F0-9]{{24}}) /\* {re.escape(folder_name)} \*/ = {{isa = PBXGroup;", text)
    if not match:
        raise SystemExit(f"group {folder_name} not found in project.pbxproj")
    return match.group(1)


def sources_phase_for(text: str, folder: Path) -> str:
    """Return the Sources phase that already compiles a file of this folder."""
    for swift in sorted(folder.glob("*.swift")):
        build = re.search(rf"\t\t([A-F0-9]{{24}}) /\* {re.escape(swift.name)} in Sources \*/ =", text)
        if not build:
            continue
        for phase in re.finditer(
            r"\t\t([A-F0-9]{24}) /\* Sources \*/ = \{isa = PBXSourcesBuildPhase;[^}]*?files = \((.*?)\);",
            text,
            re.S,
        ):
            if build.group(1) in phase.group(2):
                return phase.group(1)
    raise SystemExit(f"no Sources phase compiles a file of {folder}")


def insert_before(text: str, marker: str, line: str) -> str:
    index = text.index(marker)
    return text[:index] + line + text[index:]


def list_bounds(text: str, owner_id: str, field: str) -> tuple[int, int]:
    owner = re.search(rf"(?m)^\t\t{owner_id} /\* [^\n]* \*/ = \{{", text)
    if owner is None:
        raise SystemExit(f"object {owner_id} not found in project.pbxproj")
    opening = f"{field} = (\n"
    open_list = text.index(opening, owner.end()) + len(opening)
    close_list = re.search(r"(?m)^\t+\);", text[open_list:])
    if close_list is None:
        raise SystemExit(f"{field} list not closed for {owner_id}")
    return open_list, open_list + close_list.start()


def append_to_list(text: str, owner_id: str, field: str, entry: str) -> str:
    """Insert entry once in a group, build phase or target's list."""
    start, end = list_bounds(text, owner_id, field)
    identifier = entry.split()[0]
    if re.search(rf"(?m)^\s*{identifier}(?:\s|,)", text[start:end]):
        return text
    indent = re.match(r"\t+", text[end:]).group()
    return text[:end] + f"{indent}\t{entry},\n" + text[end:]


def resources_phase_for(text: str, target_name: str) -> tuple[str, str]:
    target = re.search(
        rf"(?m)^\t\t([A-F0-9]{{24}}) /\* {re.escape(target_name)} \*/ = \{{\n\t\t\tisa = PBXNativeTarget;",
        text,
    )
    if target is None:
        raise SystemExit(f"target {target_name} not found in project.pbxproj")
    start, end = list_bounds(text, target.group(1), "buildPhases")
    phase_ids = set(re.findall(r"[A-F0-9]{24}", text[start:end]))
    resources = re.findall(
        r"(?m)^\t\t([A-F0-9]{24}) /\* Resources \*/ = \{isa = PBXResourcesBuildPhase;",
        text,
    )
    for phase in resources:
        if phase in phase_ids:
            return text, phase

    phase = object_id("resources", target_name)
    if phase not in resources:
        text = insert_before(
            text,
            "/* End PBXResourcesBuildPhase section */",
            f"\t\t{phase} /* Resources */ = {{isa = PBXResourcesBuildPhase; buildActionMask = 2147483647; "
            "files = (\n\t\t); runOnlyForDeploymentPostprocessing = 0; };\n",
        )
    text = append_to_list(text, target.group(1), "buildPhases", f"{phase} /* Resources */")
    return text, phase


def register_catalog(text: str, catalog: Path, group_folder: Path, phase: str) -> str:
    relative = catalog.relative_to(IOS_ROOT).as_posix()
    file_ref = object_id("file", relative)
    build = object_id("build", relative)
    name = catalog.name
    path = catalog.relative_to(group_folder).as_posix()
    group = group_id(text, group_folder.name)
    if not re.search(rf"(?m)^\t\t{file_ref} /\*", text):
        text = insert_before(
            text,
            "/* End PBXFileReference section */",
            f"\t\t{file_ref} /* {name} */ = {{isa = PBXFileReference; lastKnownFileType = text.json.xcstrings; "
            f'name = "{name}"; path = "{path}"; sourceTree = "<group>"; }};\n',
        )
    if not re.search(rf"(?m)^\t\t{build} /\*", text):
        text = insert_before(
            text,
            "/* End PBXBuildFile section */",
            f"\t\t{build} /* {name} in Resources */ = {{isa = PBXBuildFile; fileRef = {file_ref} /* {name} */; }};\n",
        )
    text = append_to_list(text, group, "children", f"{file_ref} /* {name} */")
    return append_to_list(text, phase, "files", f"{build} /* {name} in Resources */")


def sync(text: str) -> tuple[str, list[str]]:
    added: list[str] = []
    for relative_folder in FOLDERS:
        folder = IOS_ROOT / relative_folder
        names = referenced_names(text)
        missing = sorted(p for p in folder.glob("*.swift") if p.name not in names)
        if not missing:
            continue
        group = group_id(text, folder.name)
        phase = sources_phase_for(text, folder)
        for swift in missing:
            relative = f"{relative_folder}/{swift.name}"
            file_ref = object_id("file", relative)
            build = object_id("build", relative)
            text = insert_before(
                text,
                "/* End PBXFileReference section */",
                f"\t\t{file_ref} /* {swift.name} */ = {{isa = PBXFileReference; lastKnownFileType = sourcecode.swift; "
                f"path = {swift.name}; sourceTree = \"<group>\"; }};\n",
            )
            text = insert_before(
                text,
                "/* End PBXBuildFile section */",
                f"\t\t{build} /* {swift.name} in Sources */ = {{isa = PBXBuildFile; fileRef = {file_ref} /* {swift.name} */; }};\n",
            )
            text = append_to_list(text, group, "children", file_ref)
            text = append_to_list(text, phase, "files", build)
            added.append(relative)
    for relative_folder, relative_group, target in RESOURCE_FOLDERS:
        folder = IOS_ROOT / relative_folder
        for catalog in sorted(folder.glob("*.xcstrings")):
            before = text
            text, phase = resources_phase_for(text, target)
            text = register_catalog(text, catalog, IOS_ROOT / relative_group, phase)
            if text != before:
                added.append(catalog.relative_to(IOS_ROOT).as_posix())
    return text, added


def main(argv: list[str]) -> int:
    text = PROJECT.read_text(encoding="utf-8")
    updated, added = sync(text)
    if "--check" in argv:
        for relative in added:
            print(f"missing from project.pbxproj: {relative}")
        return 1 if added else 0
    if added:
        PROJECT.write_text(updated, encoding="utf-8")
    for relative in added:
        print(f"registered {relative}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
