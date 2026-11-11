#!/usr/bin/env python3
"""Consistency check and index generator for docs/enterprise.

Standard library only. Run from anywhere:

    python3 docs/enterprise/tools/check_plan.py            # check only
    python3 docs/enterprise/tools/check_plan.py --write    # check and regenerate
                                                           # the generated blocks

Checks (00-conventions.md section 7): task identifiers unique, dependencies
exist, no cycles, cut C1 closure, phase order, required task fields, rule /
use-case / screen identifiers defined once and every reference defined,
relative links resolve. With --write it regenerates backlog/INDEX.md, the rule
index, the screen indexes, the roadmap lists and the check result block.

--against FILE compares task metadata with a pipe-separated skeleton
(id|title|phase|cut|type|size|prio|deps); --files limits that comparison and
the task-field check to the named backlog files.
"""
from __future__ import annotations

import argparse
import collections
import datetime
import functools
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
WEIGHT = {"XS": 0.5, "S": 1.0, "M": 2.0, "L": 4.0}
FIELDS = [
    "Meta", "Objective", "Rules and use cases", "Depends on", "Read first",
    "Scope", "Areas", "Steps", "Deliverable", "Acceptance", "Negative cases",
    "Validation", "Risks and assumptions", "Done when",
]
TASK_RE = re.compile(r"^### (EMP-[A-Z]+-\d{3}[a-z]?) — (.+?)\s*$")
FIELD_RE = re.compile(r"^- \*\*([^*:]+):\*\*\s*(.*)$")
EPIC_RE = re.compile(r"^# (EP\d{2}) — (.+?)\s*$")
ID_PATTERNS = {
    "BR": re.compile(r"(?<![A-Za-z0-9-])BR-[A-Z]+-\d{3}\b"),
    "UC": re.compile(r"(?<![A-Za-z0-9-])UC-[A-Z]+-\d{2}\b"),
    "SCR": re.compile(r"(?<![A-Za-z0-9-])SCR-[A-Z]+-\d{2}\b"),
    "IND": re.compile(r"(?<![A-Za-z0-9-])IND-\d{3}\b"),
    "FT": re.compile(r"(?<![A-Za-z0-9-])FT-\d{3}\b"),
    "VAL": re.compile(r"(?<![A-Za-z0-9-])VAL-\d{2}\b"),
    "EDR": re.compile(r"(?<![A-Za-z0-9-])EDR-\d{3}\b"),
    "CX": re.compile(r"(?<![A-Za-z0-9-])CX-\d{2}\b"),
    "AS": re.compile(r"(?<![A-Za-z0-9-])AS-\d{2}\b"),
    "OQ": re.compile(r"(?<![A-Za-z0-9-])OQ-\d{2}\b"),
    "LV": re.compile(r"(?<![A-Za-z0-9-])LV-\d{2}\b"),
    "JRN": re.compile(r"(?<![A-Za-z0-9-])JRN-\d{2}\b"),
    "REQ": re.compile(r"(?<![A-Za-z0-9-])REQ-\d{2}\.\d{1,2}\b"),
    "EMP": re.compile(r"(?<![A-Za-z0-9-])EMP-[A-Z]+-\d{3}[a-z]?\b"),
}
DEF_PATTERNS = {
    "BR": re.compile(r"^- \*\*(BR-[A-Z]+-\d{3}) — ([^*]+?)\.?\*\*"),
    "UC": re.compile(r"^\*\*(UC-[A-Z]+-\d{2}) — ([^*]+?)\.?\*\*"),
    "SCR": re.compile(r"^\| (SCR-[A-Z]+-\d{2}) \|"),
    "IND": re.compile(r"^\| (IND-\d{3}) \|"),
    "FT": re.compile(r"^\| (FT-\d{3}) \|"),
    "VAL": re.compile(r"^#{2,4} (VAL-\d{2})\b"),
    "EDR": re.compile(r"^\| (EDR-\d{3}) \|"),
    "CX": re.compile(r"^\| (CX-\d{2}) \|"),
    "AS": re.compile(r"^\| (AS-\d{2}) \|"),
    "OQ": re.compile(r"^\| (OQ-\d{2}) \|"),
    "LV": re.compile(r"^\| (LV-\d{2}) \|"),
    "JRN": re.compile(r"^#{2,4} (JRN-\d{2})\b"),
    "REQ": re.compile(r"^\| (REQ-\d{2}\.\d{1,2}) \|"),
}
# 16-decisions.md restates the conflict table of 01-diagnosis.md by design.
DEF_HOME = {"CX": "16-decisions.md"}
LINK_RE = re.compile(r"(?<!\!)\[[^\]]*\]\(([^)\s]+)\)")
SKIP_REFS_IN = {"00-conventions.md"}  # holds format examples, not references


def read(path: pathlib.Path) -> list[str]:
    return path.read_text(encoding="utf-8").split("\n")


def md_files() -> list[pathlib.Path]:
    return sorted(p for p in ROOT.rglob("*.md"))


def rel(path: pathlib.Path) -> str:
    return path.relative_to(ROOT).as_posix()


def strip_code(lines: list[str]) -> list[str]:
    """Blank out fenced code blocks so examples are not parsed as content."""
    out, fenced = [], False
    for line in lines:
        if line.lstrip().startswith("```"):
            fenced = not fenced
            out.append("")
            continue
        out.append("" if fenced else line)
    return out


def parse_tasks(errors: list[str]):
    tasks, epics = {}, []
    for path in sorted((ROOT / "backlog").glob("ep*.md")):
        lines = read(path)
        epic = None
        for line in lines:
            m = EPIC_RE.match(line)
            if m:
                epic = (m.group(1), m.group(2), path.name)
                break
        if epic is None:
            errors.append(f"{rel(path)}: missing '# EPnn — Title' heading")
            epic = (path.name[:4].upper(), path.stem, path.name)
        epics.append(epic)
        current = None
        for line in lines:
            m = TASK_RE.match(line)
            if m:
                tid = m.group(1)
                if tid in tasks:
                    errors.append(f"{rel(path)}: duplicate task {tid}")
                current = {"id": tid, "title": m.group(2), "file": path.name,
                           "epic": epic[0], "fields": {}}
                tasks[tid] = current
                continue
            if line.startswith("## ") or line.startswith("# "):
                current = None
                continue
            if current is not None:
                f = FIELD_RE.match(line)
                if f:
                    current["fields"][f.group(1).strip()] = f.group(2).strip()
    for t in tasks.values():
        meta = t["fields"].get("Meta", "")
        parts = dict(
            (kv.split(" ", 1) + [""])[:2] for kv in (s.strip() for s in meta.split("·")) if kv
        )
        t["module"] = parts.get("module", "")
        t["phase"] = parts.get("phase", "")
        t["cut"] = parts.get("cut", "")
        t["prio"] = parts.get("priority", "")
        t["type"] = parts.get("type", "")
        t["size"] = parts.get("size", "")
        t["state"] = parts.get("state", "")
        dep = t["fields"].get("Depends on", "")
        t["deps"] = ID_PATTERNS["EMP"].findall(dep)
        where = f"backlog/{t['file']}: {t['id']}"
        for name in FIELDS:
            if not t["fields"].get(name):
                errors.append(f"{where}: missing field '{name}'")
        if not re.fullmatch(r"F[0-8]", t["phase"]):
            errors.append(f"{where}: bad phase '{t['phase']}'")
        if t["cut"] not in ("C1", "-"):
            errors.append(f"{where}: bad cut '{t['cut']}'")
        if t["size"] not in WEIGHT:
            errors.append(f"{where}: bad size '{t['size']}'")
        if not re.fullmatch(r"P[0-3]", t["prio"]):
            errors.append(f"{where}: bad priority '{t['prio']}'")
        if t["module"] and t["module"] != t["id"].split("-")[1]:
            errors.append(f"{where}: module '{t['module']}' differs from identifier")
        rules = t["fields"].get("Rules and use cases", "")
        if rules and not (ID_PATTERNS["BR"].search(rules) or ID_PATTERNS["UC"].search(rules)
                          or ID_PATTERNS["EDR"].search(rules) or ID_PATTERNS["CX"].search(rules)
                          or ID_PATTERNS["OQ"].search(rules) or ID_PATTERNS["LV"].search(rules)
                          or rules.lower().startswith("n/a")):
            errors.append(f"{where}: 'Rules and use cases' cites no identifier and is not n/a")
    return tasks, epics


def check_graph(tasks, errors):
    for t in tasks.values():
        for d in t["deps"]:
            if d not in tasks:
                errors.append(f"{t['id']}: depends on undefined {d}")
    state = {}

    def visit(n, stack):
        if state.get(n) == 1:
            errors.append("dependency cycle: " + " -> ".join(stack + [n]))
            return
        if state.get(n) == 2 or n not in tasks:
            return
        state[n] = 1
        for d in tasks[n]["deps"]:
            visit(d, stack + [n])
        state[n] = 2

    for n in sorted(tasks):
        visit(n, [])
    for t in tasks.values():
        for d in t["deps"]:
            if d not in tasks:
                continue
            dep = tasks[d]
            if t["cut"] == "C1" and dep["cut"] != "C1":
                errors.append(f"{t['id']} (C1) depends on {d}, which is not C1")
            if dep["phase"] > t["phase"] and dep["cut"] != "C1":
                errors.append(f"{t['id']} ({t['phase']}) depends on later-phase {d} ({dep['phase']})")


def collect_ids(errors):
    defs = collections.defaultdict(dict)   # kind -> id -> (file, title)
    refs = collections.defaultdict(set)    # (kind, id) -> files
    screens = {}                           # id -> (surface, objective, file)
    for path in md_files():
        name = rel(path)
        lines = strip_code(read(path))
        generated_screen_index = False
        for line in lines:
            if re.fullmatch(r"<!-- SCREEN-INDEX:[A-Z]+:BEGIN -->", line):
                generated_screen_index = True
                continue
            if re.fullmatch(r"<!-- SCREEN-INDEX:[A-Z]+:END -->", line):
                generated_screen_index = False
                continue
            if name not in SKIP_REFS_IN and name != "backlog/INDEX.md":
                for kind, pat in ID_PATTERNS.items():
                    for ident in pat.findall(line):
                        refs[(kind, ident)].add(name)
            if generated_screen_index:
                continue
            for kind, pat in DEF_PATTERNS.items():
                m = pat.match(line)
                if not m:
                    continue
                if kind in DEF_HOME and name != DEF_HOME[kind]:
                    continue
                ident = m.group(1)
                title = m.group(2).strip() if m.lastindex and m.lastindex >= 2 else ""
                if ident in defs[kind] and defs[kind][ident][0] != name:
                    errors.append(f"{ident} defined in {defs[kind][ident][0]} and {name}")
                elif ident in defs[kind] and kind in ("BR", "UC"):
                    errors.append(f"{ident} defined twice in {name}")
                defs[kind].setdefault(ident, (name, title))
                if kind == "SCR":
                    cells = [c.strip() for c in line.strip().strip("|").split("|")]
                    if len(cells) >= 3:
                        screens[ident] = (cells[1], cells[2], name)
    return defs, refs, screens


def check_refs(defs, refs, tasks, errors):
    for (kind, ident), files in sorted(refs.items()):
        if kind == "EMP":
            if ident not in tasks:
                errors.append(f"{ident} referenced in {sorted(files)[0]} but no such task")
            continue
        if ident not in defs[kind]:
            errors.append(f"{ident} referenced in {', '.join(sorted(files)[:3])} but not defined")


def check_links(errors):
    for path in md_files():
        for line in strip_code(read(path)):
            for target in LINK_RE.findall(line):
                if re.match(r"^[a-z][a-z0-9+.-]*:", target) or target.startswith("#"):
                    continue
                file_part = target.split("#", 1)[0]
                if not file_part:
                    continue
                dest = (path.parent / file_part).resolve()
                if not dest.exists():
                    errors.append(f"{rel(path)}: broken link {target}")


def against(tasks, skeleton: pathlib.Path, only: set[str] | None, errors):
    expected, epic_file = {}, {}
    current_file = None
    for line in skeleton.read_text(encoding="utf-8").split("\n"):
        if not line or line.startswith("# "):
            continue
        if line.startswith("## "):
            current_file = line[3:].split("|")[2]
            continue
        p = line.split("|")
        expected[p[0]] = dict(title=p[1], phase=p[2], cut=p[3], type=p[4], size=p[5],
                              prio=p[6], deps=[d for d in p[7].split(",") if d])
        epic_file[p[0]] = current_file
    for tid, exp in expected.items():
        if only is not None and epic_file[tid] not in only:
            continue
        t = tasks.get(tid)
        if t is None:
            errors.append(f"{epic_file[tid]}: task {tid} missing")
            continue
        if t["file"] != epic_file[tid]:
            errors.append(f"{tid}: found in {t['file']}, expected {epic_file[tid]}")
        for key in ("title", "phase", "cut", "type", "size", "prio"):
            if t[key] != exp[key]:
                errors.append(f"{tid}: {key} is '{t[key]}', skeleton says '{exp[key]}'")
        if sorted(t["deps"]) != sorted(exp["deps"]):
            errors.append(f"{tid}: deps {sorted(t['deps'])} differ from skeleton {sorted(exp['deps'])}")
    for tid, t in tasks.items():
        if tid not in expected and (only is None or t["file"] in only):
            errors.append(f"{t['file']}: task {tid} is not in the skeleton")


# ---------------------------------------------------------------- generation

def replace_block(path: pathlib.Path, marker: str, body: list[str]) -> bool:
    if not path.exists():
        return False
    text = path.read_text(encoding="utf-8")
    begin, end = f"<!-- {marker}:BEGIN -->", f"<!-- {marker}:END -->"
    if begin not in text or end not in text:
        return False
    head, rest = text.split(begin, 1)
    _, tail = rest.split(end, 1)
    new = head + begin + "\n" + "\n".join(body) + ("\n" if body else "") + end + tail
    if new != text:
        path.write_text(new, encoding="utf-8")
    return True


def fmt(n: float) -> str:
    return f"{n:g}"


def write_index(tasks, epics):
    rows = list(tasks.values())
    o = ["# Backlog index", "",
         "Status: **PLANNED**, 2026-11-10. Generated from the task blocks in `backlog/ep*.md` by "
         "[tools/check_plan.py](../tools/check_plan.py) ([00-conventions](../00-conventions.md) §7). "
         "Do not edit by hand: change the task block and regenerate.", "",
         "Columns: phase (`F0`–`F8`), cut (`C1` = required for the first operational cut), type and "
         "size as defined in [00-conventions](../00-conventions.md) §5. Sizes are relative hypotheses, "
         "not dates.", "", "## Summary", ""]
    total = sum(WEIGHT.get(r["size"], 0) for r in rows)
    c1 = sum(WEIGHT.get(r["size"], 0) for r in rows if r["cut"] == "C1")
    o.append(f"- Tasks: **{len(rows)}** in {len(epics)} epics. Cut C1: "
             f"**{sum(1 for r in rows if r['cut'] == 'C1')}** tasks.")
    o.append(f"- Relative effort (XS 0.5, S 1, M 2, L 4 agent cycles): **{fmt(total)}** in total, "
             f"**{fmt(c1)}** for cut C1. Hypothesis only.")
    o += ["", "| Phase | Tasks | Relative effort |", "|---|---|---|"]
    for ph in sorted({r["phase"] for r in rows}):
        rs = [r for r in rows if r["phase"] == ph]
        o.append(f"| {ph} | {len(rs)} | {fmt(sum(WEIGHT.get(r['size'], 0) for r in rs))} |")
    o += ["", "| Epic | Title | File | Tasks |", "|---|---|---|---|"]
    for code, title, fname in epics:
        n = sum(1 for r in rows if r["file"] == fname)
        o.append(f"| {code} | {title} | [{fname}]({fname}) | {n} |")
    o.append("")
    for code, title, fname in epics:
        o += [f"## {code} — {title}", "", f"Detail: [{fname}]({fname}).", "",
              "| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |",
              "|---|---|---|---|---|---|---|---|"]
        for r in rows:
            if r["file"] != fname:
                continue
            deps = ", ".join(r["deps"]) if r["deps"] else "none"
            o.append(f"| {r['id']} | {r['title']} | {r['phase']} | {r['cut']} | {r['type']} | "
                     f"{r['size']} | {r['prio']} | {deps} |")
        o.append("")
    (ROOT / "backlog" / "INDEX.md").write_text("\n".join(o), encoding="utf-8")


def levels(tasks, subset):
    level = {}

    def lv(n):
        if n in level:
            return level[n]
        ds = [d for d in tasks[n]["deps"] if d in subset]
        level[n] = 1 + max((lv(d) for d in ds), default=0)
        return level[n]

    for n in subset:
        lv(n)
    return level


def critical_path(tasks, target):
    @functools.lru_cache(None)
    def lp(n):
        best = (0.0, ())
        for d in tasks[n]["deps"]:
            if d in tasks:
                c = lp(d)
                if c[0] > best[0]:
                    best = c
        return (best[0] + WEIGHT.get(tasks[n]["size"], 0), best[1] + (n,))

    return lp(target) if target in tasks else (0.0, ())


def write_roadmap(tasks):
    path = ROOT / "14-roadmap.md"
    c1 = {k for k, t in tasks.items() if t["cut"] == "C1"}
    body = ["| Home phase | Tasks |", "|---|---|"]
    for ph in sorted({tasks[k]["phase"] for k in c1}):
        ids = [k for k in tasks if k in c1 and tasks[k]["phase"] == ph]
        body.append(f"| {ph} | {', '.join(ids)} |")
    body.append("")
    body.append(f"Total: {len(c1)} tasks, relative effort "
                f"{fmt(sum(WEIGHT.get(tasks[k]['size'], 0) for k in c1))}.")
    replace_block(path, "C1-TASKS", body)
    effort, chain = critical_path(tasks, "EMP-REL-002")
    body = [" → ".join(chain), "", f"Relative effort along this chain: {fmt(effort)} agent cycles "
            "(hypothesis). Every other C1 task has slack against it."] if chain else []
    replace_block(path, "C1-CRITICAL", body)
    lv = levels(tasks, c1)
    body = ["| Level | Tasks that can start once the previous level is done |", "|---|---|"]
    for n in sorted(set(lv.values())):
        body.append(f"| {n} | {', '.join(k for k in tasks if lv.get(k) == n)} |")
    replace_block(path, "C1-WAVES", body)
    free = [k for k, t in tasks.items() if not t["deps"]]
    after = [k for k, t in tasks.items() if t["deps"] == ["EMP-GOV-001"]]
    body = [f"- No dependency: {', '.join(free)}.",
            f"- After EMP-GOV-001 only: {', '.join(after)}."]
    replace_block(path, "UNBLOCKED", body)
    body = ["| Phase | Tasks | Relative effort | Of which cut C1 |", "|---|---|---|---|"]
    for ph in sorted({t["phase"] for t in tasks.values()}):
        rs = [t for t in tasks.values() if t["phase"] == ph]
        body.append(f"| {ph} | {len(rs)} | {fmt(sum(WEIGHT.get(t['size'], 0) for t in rs))} | "
                    f"{fmt(sum(WEIGHT.get(t['size'], 0) for t in rs if t['cut'] == 'C1'))} |")
    total = sum(WEIGHT.get(t["size"], 0) for t in tasks.values())
    body.append(f"| **All** | **{len(tasks)}** | **{fmt(total)}** | "
                f"**{fmt(sum(WEIGHT.get(tasks[k]['size'], 0) for k in c1))}** |")
    replace_block(path, "EFFORT", body)


def write_rule_index(defs):
    body = ["| Rule | Name | Defined in |", "|---|---|---|"]
    for ident in sorted(defs["BR"]):
        fname, title = defs["BR"][ident]
        target = fname if "/" not in fname else fname
        body.append(f"| {ident} | {title} | [{fname}]({target}) |")
    replace_block(ROOT / "04-business-rules.md", "RULE-INDEX", body)


def write_screen_indexes(screens):
    by_surface = collections.defaultdict(list)
    for ident, (surface, objective, fname) in sorted(screens.items()):
        for s in re.split(r"[,/]| and ", surface):
            s = s.strip().upper()
            if s:
                by_surface[s].append((ident, objective, fname))
    for surface, target in (("APP", "12-ux-app.md"), ("PANEL", "13-ux-web.md"),
                            ("PORTAL", "13-ux-web.md"), ("ADMIN", "13-ux-web.md")):
        body = ["| Screen | Objective | Specified in |", "|---|---|---|"]
        for ident, objective, fname in by_surface.get(surface, []):
            body.append(f"| {ident} | {objective} | [{fname}]({fname}) |")
        replace_block(ROOT / target, f"SCREEN-INDEX:{surface}", body)


def write_check_result(tasks, defs, errors):
    counts = ", ".join(f"{len(defs[k])} {k}" for k in ("BR", "UC", "SCR", "IND", "FT", "VAL", "EDR",
                                                       "CX", "REQ") if defs[k])
    body = [f"Last run: {datetime.date.today().isoformat()} with `python3 docs/enterprise/tools/check_plan.py --write`.",
            "",
            f"- Tasks parsed: {len(tasks)}; cut C1: {sum(1 for t in tasks.values() if t['cut'] == 'C1')}.",
            f"- Identifiers defined: {counts}.",
            f"- Problems found: {len(errors)}."]
    body += [f"  - {e}" for e in errors[:40]]
    replace_block(ROOT / "15-validation.md", "CHECK-RESULT", body)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--write", action="store_true")
    ap.add_argument("--against")
    ap.add_argument("--files", nargs="*")
    args = ap.parse_args()
    errors: list[str] = []
    tasks, epics = parse_tasks(errors)
    only = set(args.files) if args.files else None
    if only is not None:
        errors[:] = [e for e in errors if any(f in e for f in only)]
    if args.against:
        against(tasks, pathlib.Path(args.against), only, errors)
    if only is None:
        check_graph(tasks, errors)
        defs, refs, screens = collect_ids(errors)
        check_refs(defs, refs, tasks, errors)
        check_links(errors)
        if args.write:
            write_index(tasks, epics)
            write_roadmap(tasks)
            write_rule_index(defs)
            write_screen_indexes(screens)
            write_check_result(tasks, defs, errors)
        print(f"tasks {len(tasks)}; rules {len(defs['BR'])}; use cases {len(defs['UC'])}; "
              f"screens {len(defs['SCR'])}; problems {len(errors)}")
    else:
        print(f"tasks in selected files: {sum(1 for t in tasks.values() if t['file'] in only)}; "
              f"problems {len(errors)}")
    for e in errors:
        print("  -", e)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
