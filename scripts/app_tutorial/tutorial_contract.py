#!/usr/bin/env python3
"""App tutorial screenshot contract.

The Android tutorial captures in docs/assets/app-tutorial/ are listed in
manifest.json. Each step names the Compose sources and string resources that
draw it. This tool keeps that contract honest:

  validate      manifest <-> PNG files <-> sources/strings <-> README (CI)
  stale         app changes since the capture commit -> affected steps, the
                landing files that use those images and the landing lines
                that quote changed pt-BR labels
  sync-sources  recompute each step's "sources" from its strings and
                extraSources (use --write to update the manifest)

Standard library only; see docs/assets/app-tutorial/CAPTURE_CONTRACT.md.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import struct
import subprocess
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MANIFEST_REL = "docs/assets/app-tutorial/manifest.json"
KOTLIN_REL = "app/src/main/kotlin"
RES_REL = "app/src/main/res"
PT_BR_VALUES = "values-pt-rBR"
LANDING_TEXT_DIRS = ("landing/content", "landing/static", "landing/src", "landing/scripts")
LANDING_TEXT_SUFFIXES = {".md", ".html", ".json", ".ts", ".mjs", ".js", ".css", ".txt", ".xml"}
STEP_NAME = re.compile(r"^(\d{2})-[a-z0-9]+(?:-[a-z0-9]+)*\.png$")
STRING_REF = re.compile(r"\bR\.string\.([A-Za-z0-9_]+)")
DRAWABLE_REF = re.compile(r"\bR\.drawable\.([A-Za-z0-9_]+)")
SHA1 = re.compile(r"^[0-9a-f]{40}$")
CAPTURE_SIZE = (1080, 1920)
MIN_QUOTE_LEN = 4


class ContractError(RuntimeError):
    pass


# ---------------------------------------------------------------- loading

def load_manifest(root: Path) -> dict:
    path = root / MANIFEST_REL
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ContractError(f"missing manifest {MANIFEST_REL}") from exc
    except json.JSONDecodeError as exc:
        raise ContractError(f"invalid JSON in {MANIFEST_REL}: {exc}") from exc


def iter_steps(manifest: dict):
    for feature in manifest.get("features", []):
        for step in feature.get("steps", []):
            yield feature, step


def png_info(data: bytes) -> tuple[int, int, int]:
    """Return (width, height, colour type) from a PNG header."""
    if len(data) < 26 or data[:8] != b"\x89PNG\r\n\x1a\n" or data[12:16] != b"IHDR":
        raise ContractError("not a PNG file")
    width, height = struct.unpack(">II", data[16:24])
    return width, height, data[25]


def string_values(xml_text: str) -> dict[str, str]:
    """Map string resource names to their text (plurals/arrays ignored)."""
    if not xml_text.strip():
        return {}
    root = ET.fromstring(xml_text)
    values = {}
    for node in root.iter("string"):
        name = node.get("name")
        if name:
            values[name] = "".join(node.itertext())
    return values


def kotlin_files(root: Path):
    base = root / KOTLIN_REL
    if base.is_dir():
        yield from sorted(base.rglob("*.kt"))


def reference_index(root: Path, pattern: re.Pattern) -> dict[str, set[str]]:
    index: dict[str, set[str]] = {}
    for path in kotlin_files(root):
        rel = path.relative_to(root).as_posix()
        for name in pattern.findall(path.read_text(encoding="utf-8", errors="replace")):
            index.setdefault(name, set()).add(rel)
    return index


def known_string_keys(root: Path) -> set[str]:
    keys: set[str] = set()
    values_dir = root / RES_REL / "values"
    for xml_path in sorted(values_dir.glob("*.xml")):
        keys.update(string_values(xml_path.read_text(encoding="utf-8")))
    return keys


def resolve_sources(step: dict, usages: dict[str, set[str]]) -> list[str]:
    sources = set(step.get("extraSources", []))
    for key in step.get("strings", []):
        sources.update(usages.get(key, set()))
    return sorted(sources)


# ---------------------------------------------------------------- README

BEGIN_MARK = "<!-- BEGIN GENERATED STEPS: tutorial_contract.py render-readme -->"
END_MARK = "<!-- END GENERATED STEPS -->"
CONTEXT_NOTES = {
    "system": "Android system screen",
    "notSubmitted": "draft, not submitted",
    "emulatorArtifact": "emulator test scene",
}


def render_steps(manifest: dict) -> str:
    """Markdown for the generated README section (features, steps, captions)."""
    text_only: dict[tuple[str, str], list[str]] = {}
    for note in manifest.get("textOnlySteps", []):
        text_only.setdefault((note["feature"], note["afterStep"]), []).append(note["text"])
    lines = [BEGIN_MARK, ""]
    for feature in manifest.get("features", []):
        fid = feature["id"]
        lines += [f"### {feature['title']}", "", feature.get("summary", ""), ""]
        targets = feature.get("landingTargets", [])
        if targets:
            lines += ["Landing pages: " + ", ".join(f"`{t}`" for t in targets), ""]
        lines += ["| Step | Image | Caption |", "|---|---|---|"]
        for step in feature.get("steps", []):
            number = step["file"][:2]
            ctx = step.get("context", {})
            tags = [CONTEXT_NOTES[key] for key in CONTEXT_NOTES if ctx.get(key)]
            if ctx.get("city"):
                tags.insert(0, ctx["city"])
            if ctx.get("account") == "signed-in":
                tags.append("signed in")
            caption = step["caption"].replace("|", "\\|")
            suffix = f" _({'; '.join(tags)})_" if tags else ""
            issue = f" **Known issue:** {ctx['knownIssue']}" if ctx.get("knownIssue") else ""
            lines.append(f"| {number} | [`{fid}/{step['file']}`]({fid}/{step['file']}) | {caption}{suffix}{issue} |")
        lines.append("")
        for step in feature.get("steps", []):
            for text in text_only.get((fid, step["file"]), []):
                lines += [f"> Not captured (after step {step['file'][:2]}): {text}", ""]
    lines.append(END_MARK)
    return "\n".join(lines)


def readme_with_steps(readme: str, manifest: dict) -> str:
    start, end = readme.find(BEGIN_MARK), readme.find(END_MARK)
    if start < 0 or end < start:
        raise ContractError("README.md is missing the generated-steps markers")
    return readme[:start] + render_steps(manifest) + readme[end + len(END_MARK):]


# ---------------------------------------------------------------- validate

def validate(root: Path, manifest: dict) -> list[str]:
    errors: list[str] = []
    asset_dir = root / Path(MANIFEST_REL).parent
    if manifest.get("schemaVersion") != 1:
        errors.append("schemaVersion must be 1")
    capture = manifest.get("capture", {})
    if not SHA1.match(str(capture.get("appSourceCommit", ""))):
        errors.append("capture.appSourceCommit must be a full 40-character commit SHA")

    usages = reference_index(root, STRING_REF)
    known_keys = known_string_keys(root)
    readme_path = asset_dir / "README.md"
    readme = readme_path.read_text(encoding="utf-8") if readme_path.exists() else ""
    if not readme:
        errors.append("missing docs/assets/app-tutorial/README.md")

    listed: set[str] = set()
    feature_ids: set[str] = set()
    for feature in manifest.get("features", []):
        fid = feature.get("id", "")
        if not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*", fid):
            errors.append(f"feature id {fid!r} is not kebab-case")
        if fid in feature_ids:
            errors.append(f"duplicate feature id {fid}")
        feature_ids.add(fid)
        if not feature.get("title"):
            errors.append(f"{fid}: missing title")
        numbers = []
        for step in feature.get("steps", []):
            rel = step.get("file", "")
            where = f"{fid}/{rel}"
            match = STEP_NAME.match(rel)
            if not match:
                errors.append(f"{where}: file name must be NN-kebab-slug.png")
            else:
                numbers.append(int(match.group(1)))
            image_rel = f"{fid}/{rel}"
            listed.add(image_rel)
            image = asset_dir / image_rel
            if not image.is_file():
                errors.append(f"{where}: image missing")
            else:
                data = image.read_bytes()
                try:
                    width, height, colour = png_info(data)
                    if (width, height) != CAPTURE_SIZE:
                        errors.append(f"{where}: {width}x{height}, expected {CAPTURE_SIZE[0]}x{CAPTURE_SIZE[1]}")
                    if colour != 2:
                        errors.append(f"{where}: PNG must be opaque RGB (colour type 2), got {colour}")
                except ContractError as exc:
                    errors.append(f"{where}: {exc}")
                digest = hashlib.sha256(data).hexdigest()
                if step.get("sha256") != digest:
                    errors.append(f"{where}: sha256 mismatch (recaptured? update the manifest)")
            if "appSourceCommit" in step and not SHA1.match(str(step["appSourceCommit"])):
                errors.append(f"{where}: appSourceCommit must be a full 40-character commit SHA")
            if not step.get("caption"):
                errors.append(f"{where}: missing caption")
            for key in step.get("strings", []):
                if key not in known_keys:
                    errors.append(f"{where}: unknown string resource {key}")
            for src in step.get("extraSources", []):
                if not (root / src).exists():
                    errors.append(f"{where}: extra source {src} does not exist")
            expected = resolve_sources(step, usages)
            if not expected:
                errors.append(f"{where}: no sources (add strings or extraSources)")
            if step.get("sources") != expected:
                errors.append(f"{where}: sources out of sync; run tutorial_contract.py sync-sources --write")
            if readme and image_rel not in readme:
                errors.append(f"{where}: not listed in README.md")
        if numbers and sorted(numbers) != list(range(1, len(numbers) + 1)):
            errors.append(f"{fid}: step numbers must run 01..{len(numbers):02d} without gaps")

    for note in manifest.get("textOnlySteps", []):
        if note.get("feature") not in feature_ids:
            errors.append(f"text-only step points at unknown feature {note.get('feature')!r}")
        for src in note.get("sources", []):
            if not (root / src).exists():
                errors.append(f"text-only step source {src} does not exist")

    on_disk = {p.relative_to(asset_dir).as_posix() for p in asset_dir.rglob("*.png")}
    for extra in sorted(on_disk - listed):
        errors.append(f"{extra}: PNG not listed in manifest.json")
    if readme:
        try:
            if readme_with_steps(readme, manifest) != readme:
                errors.append("README.md generated steps are out of date; run tutorial_contract.py render-readme --write")
        except ContractError as exc:
            errors.append(str(exc))
    return errors


# ---------------------------------------------------------------- stale

def git(root: Path, *args: str) -> str:
    result = subprocess.run(["git", *args], cwd=root, capture_output=True, text=True)
    if result.returncode != 0:
        raise ContractError(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout


def has_commit(root: Path, rev: str) -> bool:
    return subprocess.run(["git", "cat-file", "-e", f"{rev}^{{commit}}"], cwd=root,
                          capture_output=True).returncode == 0


def file_at(root: Path, rev: str | None, rel: str) -> str:
    if rev is None:
        path = root / rel
        return path.read_text(encoding="utf-8") if path.exists() else ""
    result = subprocess.run(["git", "show", f"{rev}:{rel}"], cwd=root, capture_output=True, text=True)
    return result.stdout if result.returncode == 0 else ""


def changed_paths(root: Path, base: str, head: str | None) -> list[str]:
    spec = [base] if head is None else [base, head]
    out = git(root, "diff", "--name-only", *spec, "--", "app/src/main")
    paths = set(filter(None, out.splitlines()))
    if head is None:  # include untracked files in the working tree
        untracked = git(root, "ls-files", "--others", "--exclude-standard", "--", "app/src/main")
        paths.update(filter(None, untracked.splitlines()))
    return sorted(paths)


def landing_text_files(root: Path):
    for rel_dir in LANDING_TEXT_DIRS:
        base = root / rel_dir
        if not base.is_dir():
            continue
        for path in sorted(base.rglob("*")):
            if path.is_file() and path.suffix in LANDING_TEXT_SUFFIXES and "node_modules" not in path.parts:
                yield path


def image_stems(feature_id: str, file_name: str) -> list[str]:
    stem = file_name[:-4]
    return [f"app-tutorial/{feature_id}/{stem}", f"{feature_id}-{stem}"]


def quote_needle(text: str) -> str | None:
    """Plain label text without format placeholders, or None if too short."""
    cleaned = re.sub(r"%\d*\$?[sdf]", "", text).replace("\\'", "'").replace('\\"', '"').strip()
    cleaned = cleaned.strip(".:… ")
    return cleaned if len(cleaned) >= MIN_QUOTE_LEN else None


def step_base(manifest: dict, step: dict, override: str | None) -> str:
    return override or step.get("appSourceCommit") or manifest["capture"]["appSourceCommit"]


def stale(root: Path, manifest: dict, base: str | None, head: str | None) -> dict:
    if head is not None and not has_commit(root, head):
        raise ContractError(f"head {head} is not a commit")
    bases = sorted({step_base(manifest, step, base) for _, step in iter_steps(manifest)})
    for rev in bases:
        if not has_commit(root, rev):
            raise ContractError(
                f"capture commit {rev} is not in this clone; fetch full history (git fetch --unshallow)")
    changes = {rev: diff_changes(root, rev, head) for rev in bases}
    landing_cache = {p: p.read_text(encoding="utf-8", errors="replace") for p in landing_text_files(root)}

    affected = []
    for feature, step in iter_steps(manifest):
        change = changes[step_base(manifest, step, base)]
        sources = set(step.get("sources", []))
        reasons = []
        for src in sorted(sources & change["kotlin"]):
            reasons.append(f"source changed: {src}")
        for src in sorted(sources & change["drawableUsers"]):
            reasons.append(f"drawable used by {src} changed")
        keys = sorted(set(step.get("strings", [])) & set(change["keys"]))
        for key in keys:
            locales = ", ".join(sorted(change["keys"][key]["locales"]))
            reasons.append(f"string {key} changed ({locales})")
        if not reasons:
            continue
        stems = image_stems(feature["id"], step["file"])
        image_uses = sorted({p.relative_to(root).as_posix() for p, text in landing_cache.items()
                             if any(stem in text for stem in stems)})
        quotes = []
        for key in keys:
            before = change["keys"][key]["ptBrBefore"]
            needle = quote_needle(before or "")
            if not needle:
                continue
            for path, text in landing_cache.items():
                for number, line in enumerate(text.splitlines(), start=1):
                    if needle in line:
                        quotes.append({
                            "file": path.relative_to(root).as_posix(), "line": number, "string": key,
                            "before": before, "after": change["keys"][key]["ptBrAfter"],
                        })
        affected.append({
            "image": f"{feature['id']}/{step['file']}",
            "capturedFrom": step_base(manifest, step, base),
            "reasons": reasons,
            "declaredLandingTargets": feature.get("landingTargets", []),
            "landingImageUses": image_uses,
            "landingQuotes": quotes,
        })
    return {"base": ", ".join(b[:12] for b in bases), "head": head or "WORKTREE",
            "changedPaths": len(set().union(*(c["paths"] for c in changes.values()))) if changes else 0,
            "affected": affected}


def diff_changes(root: Path, base: str, head: str | None) -> dict:
    """Kotlin files, string keys and drawable users changed between base and head."""
    paths = changed_paths(root, base, head)
    changed_kotlin = {p for p in paths if p.startswith(KOTLIN_REL + "/") and p.endswith(".kt")}
    changed_keys: dict[str, dict] = {}
    changed_drawables: set[str] = set()
    for rel in paths:
        parts = Path(rel).parts
        if len(parts) < 5 or parts[3] != "res":
            continue
        folder, name = parts[4], Path(rel).name
        if folder.startswith("values") and name.endswith(".xml"):
            before = string_values(file_at(root, base, rel))
            after = string_values(file_at(root, head, rel))
            for key in set(before) | set(after):
                if before.get(key) != after.get(key):
                    entry = changed_keys.setdefault(key, {"locales": set(), "ptBrBefore": None, "ptBrAfter": None})
                    entry["locales"].add(folder)
                    if folder == PT_BR_VALUES:
                        entry["ptBrBefore"] = before.get(key)
                        entry["ptBrAfter"] = after.get(key)
        elif folder.startswith(("drawable", "mipmap")):
            changed_drawables.add(Path(name).stem)

    drawable_users: set[str] = set()
    if changed_drawables:
        index = reference_index(root, DRAWABLE_REF)
        for name in changed_drawables:
            drawable_users.update(index.get(name, set()))

    return {"paths": set(paths), "kotlin": changed_kotlin, "keys": changed_keys, "drawableUsers": drawable_users}


def print_stale(report: dict) -> None:
    print(f"app tutorial freshness: captured from {report['base']} vs {report['head'][:12]}, "
          f"{report['changedPaths']} app path(s) changed")
    if not report["affected"]:
        print("OK: no captured tutorial step is affected")
        return
    print(f"RECAPTURE: {len(report['affected'])} step(s) are stale")
    for item in report["affected"]:
        print(f"- {item['image']}")
        for reason in item["reasons"]:
            print(f"    why: {reason}")
        for use in item["landingImageUses"]:
            print(f"    update landing image use: {use}")
        if not item["landingImageUses"] and item["declaredLandingTargets"]:
            print(f"    landing pages to review: {', '.join(item['declaredLandingTargets'])}")
        for quote in item["landingQuotes"]:
            print(f"    update landing text {quote['file']}:{quote['line']} "
                  f"({quote['string']}: {quote['before']!r} -> {quote['after']!r})")


# ---------------------------------------------------------------- CLI

def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", type=Path, default=ROOT, help="repository root (tests)")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("validate", help="check manifest, images, sources and README")
    p_stale = sub.add_parser("stale", help="list steps affected by app changes since the capture")
    p_stale.add_argument("--base", help="defaults to capture.appSourceCommit")
    p_stale.add_argument("--head", help="commit to compare; defaults to the working tree")
    p_stale.add_argument("--strict", action="store_true", help="exit 1 when any step is stale")
    p_stale.add_argument("--report-only", action="store_true",
                         help="never fail: missing history is reported and skipped (CI)")
    p_stale.add_argument("--json", action="store_true", help="print the report as JSON")
    p_sync = sub.add_parser("sync-sources", help="recompute step sources from strings/extraSources")
    p_sync.add_argument("--write", action="store_true", help="rewrite manifest.json")
    p_readme = sub.add_parser("render-readme", help="regenerate the README steps section")
    p_readme.add_argument("--write", action="store_true", help="rewrite README.md")
    p_refresh = sub.add_parser("refresh", help="after recapturing: store new sha256 and capture commit")
    p_refresh.add_argument("images", nargs="+", help="feature/NN-slug.png as listed in the manifest")
    p_refresh.add_argument("--commit", default="HEAD", help="app source commit the new capture was built from")
    args = parser.parse_args(argv)
    root = args.root.resolve()

    try:
        manifest = load_manifest(root)
        if args.command == "validate":
            errors = validate(root, manifest)
            for error in errors:
                print(f"ERROR: {error}", file=sys.stderr)
            steps = sum(1 for _ in iter_steps(manifest))
            if errors:
                return 1
            print(f"OK: app tutorial manifest valid ({steps} steps)")
            return 0
        if args.command == "sync-sources":
            usages = reference_index(root, STRING_REF)
            changed = 0
            for _, step in iter_steps(manifest):
                expected = resolve_sources(step, usages)
                if step.get("sources") != expected:
                    step["sources"] = expected
                    changed += 1
            if args.write and changed:
                (root / MANIFEST_REL).write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
                                                 encoding="utf-8")
            print(f"{changed} step(s) {'updated' if args.write else 'out of sync'}")
            return 0 if args.write or not changed else 1
        if args.command == "refresh":
            commit = git(root, "rev-parse", "--verify", f"{args.commit}^{{commit}}").strip()
            wanted = set(args.images)
            asset_dir = root / Path(MANIFEST_REL).parent
            for feature, step in iter_steps(manifest):
                image_rel = f"{feature['id']}/{step['file']}"
                if image_rel in wanted:
                    step["sha256"] = hashlib.sha256((asset_dir / image_rel).read_bytes()).hexdigest()
                    step["appSourceCommit"] = commit
                    wanted.discard(image_rel)
            if wanted:
                raise ContractError("not in manifest: " + ", ".join(sorted(wanted)))
            (root / MANIFEST_REL).write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
                                             encoding="utf-8")
            print(f"refreshed {len(args.images)} step(s) at {commit[:12]}")
            return 0
        if args.command == "render-readme":
            readme_path = root / Path(MANIFEST_REL).parent / "README.md"
            current = readme_path.read_text(encoding="utf-8")
            rendered = readme_with_steps(current, manifest)
            if args.write:
                readme_path.write_text(rendered, encoding="utf-8")
                print("README.md steps section regenerated")
                return 0
            print("README.md steps section is " + ("current" if rendered == current else "out of date"))
            return 0 if rendered == current else 1
        try:
            report = stale(root, manifest, args.base, args.head)
        except ContractError as exc:
            if args.report_only:
                print(f"SKIP: app tutorial freshness not computed: {exc}")
                return 0
            raise
        if args.json:
            print(json.dumps(report, ensure_ascii=False, indent=2))
        else:
            print_stale(report)
        return 1 if (args.strict and report["affected"]) else 0
    except ContractError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
