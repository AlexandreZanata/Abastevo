#!/usr/bin/env python3
"""Register new Swift sources of iosApp/ in AnpFuel.xcodeproj without Xcode.

The project file is maintained by hand on Linux hosts, where XcodeGen and
Xcode are unavailable. For each source folder this adds a file reference,
a build file, the group child and the matching target "Sources" phase entry
for every *.swift file the project does not reference yet. Object IDs are
derived from the relative path, so two hosts registering the same file
produce identical entries.

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


def append_to_list(text: str, owner_id: str, entry: str) -> str:
    """Append entry to the children/files list of the object owner_id."""
    start = text.index(f"\t\t{owner_id} /*")
    open_list = text.index("= (\n", text.index("{isa = ", start)) + len("= (\n")
    close_list = text.index("\t\t);", open_list)
    return text[:close_list] + f"\t\t\t{entry},\n" + text[close_list:]


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
            text = append_to_list(text, group, file_ref)
            text = append_to_list(text, phase, build)
            added.append(relative)
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
