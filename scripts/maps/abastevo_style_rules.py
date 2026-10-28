#!/usr/bin/env python3
"""Abastevo map-style rule transform (colors live in maps/style/colors/).

Policy (reviewed, minimal):
- POI icons and POI captions render only at zoom 16+. Sub-16 ladder rungs are
  dropped when a same-feature sibling already covers zoom 16+; otherwise the
  rung is raised to exactly 16. Open ranges keep their upper bound.
- Rules without icon-image/text bodies, exact `text: none` suppressors and
  geographic names (continents, countries, places, water, parks) are untouched.
- Minor-road borders become visible (`casing-opacity: 0.1` -> `1`); the gray
  itself comes from the per-mode colors files.

Usage: abastevo_style_rules.py <file.mapcss> [--basemap-label]
Rewrites the whole Icons.mapcss, or only the landuse-caption section of
Basemap_label.mapcss.
"""

import re
import sys

ZOOM_RE = re.compile(r"\|(z)(\d+)(?:(-)(\d+)?)?")

MIN_ZOOM = 16


def split_selectors(rule_head: str) -> list[str]:
    parts, depth, current = [], 0, ""
    for ch in rule_head:
        if ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
        if ch == "," and depth == 0:
            if current.strip():
                parts.append(current.strip())
            current = ""
        else:
            current += ch
    if current.strip():
        parts.append(current.strip())
    return parts


def zoom_cover(token: re.Match) -> tuple[int, int, bool]:
    """Return (low, high, is_open) of a |z... token; open means up to maxzoom."""
    low = int(token.group(2))
    if token.group(3) is None:
        return low, low, False
    if token.group(4) is None:
        return low, 20, True
    return low, int(token.group(4)), False


def normalize_selector(sel: str) -> str:
    """Feature identity of a selector: zoom token and whitespace removed."""
    return ZOOM_RE.sub("", sel).replace(" ", "")


def parse_rules(lines: list[str], lo: int, hi: int) -> list[dict]:
    """Split lines[lo:hi] into rule dicts without rewriting anything."""
    rules: list[dict] = []
    pending: list[str] = []
    in_comment = False
    i = lo
    while i < hi:
        line = lines[i]
        stripped = line.strip()
        if in_comment:
            pending.append(line)
            if "*/" in stripped:
                in_comment = False
                rules.append({"verbatim": True, "raw": pending})
                pending = []
            i += 1
            continue
        if stripped.startswith("/*") and "*/" not in stripped:
            if pending:
                rules.append({"verbatim": True, "raw": pending})
                pending = []
            pending.append(line)
            in_comment = True
            i += 1
            continue
        if (not stripped or stripped.startswith("/*") or stripped.startswith("*")
                or stripped.startswith("@")):
            if pending:
                rules.append({"verbatim": True, "raw": pending})
                pending = []
            rules.append({"verbatim": True, "raw": [line]})
            i += 1
            continue
        brace_at = line.find("{")
        if brace_at < 0:
            pending.append(line)
            i += 1
            continue
        head_part = line[:brace_at]
        selectors: list[str] = []
        for chunk in split_selectors(( "".join(pending) + head_part ).strip()):
            selectors.append(chunk)
        raw_head = list(pending)
        if head_part.strip():
            raw_head.append(head_part + "\n")
        pending = []
        rest = line[brace_at + 1 :]
        if "}" in rest:
            body_text = rest[: rest.rindex("}")]
            rules.append(
                {
                    "verbatim": False,
                    "selectors": selectors,
                    "body": [body_text],
                    "closing": "}\n",
                    "raw": raw_head + [line] if not head_part.strip() else [line],
                }
            )
            i += 1
            continue
        body_lines: list[str] = []
        if rest.strip():
            body_lines.append(rest)
        raw_body = [line]
        i += 1
        while i < hi and "}" not in lines[i]:
            body_lines.append(lines[i])
            raw_body.append(lines[i])
            i += 1
        closing = lines[i] if i < hi else ""
        if closing:
            raw_body.append(closing)
        rules.append(
            {
                "verbatim": False,
                "selectors": selectors,
                "body": body_lines,
                "closing": closing,
                "raw": raw_head + raw_body,
            }
        )
        i += 1
    if pending:
        rules.append({"verbatim": True, "raw": pending})
    return rules


def has_suppressor(body: str) -> bool:
    return "text:none" in body.replace(" ", "")


def is_poi_rule(body: str) -> str:
    if has_suppressor(body):
        return ""
    if "icon-image:" in body:
        return "icon"
    if re.search(r"(?<![-:])text\s*:", body) is not None:
        return "text"
    return ""
def sibling_covers_16(rules: list[dict], sel: str, kind: str) -> bool:
    """Whether another POI rule for the same feature+layer covers zoom 16+."""
    ident = normalize_selector(sel)
    layer = sel.rsplit("::", 1)[1] if "::" in sel else ""
    for rule in rules:
        if rule.get("verbatim") or "selectors" not in rule:
            continue
        body = "".join(rule["body"])
        if not is_poi_rule(body):
            continue
        for other in rule["selectors"]:
            if normalize_selector(other) != ident:
                continue
            other_layer = other.rsplit("::", 1)[1] if "::" in other else ""
            if other_layer != layer:
                continue
            m = ZOOM_RE.search(other)
            if not m:
                return True
            _, high, _ = zoom_cover(m)
            if high >= MIN_ZOOM:
                return True
    return False


def transform_selector(sel: str, kind: str, rules: list[dict]) -> str | None:
    """Return the rewritten selector, or None to drop it."""
    m = ZOOM_RE.search(sel)
    if not m:
        return sel
    low, high, is_open = zoom_cover(m)
    if low >= MIN_ZOOM:
        return sel
    if high >= MIN_ZOOM:
        # Partial overlap: narrow the lower bound, keep the upper bound
        # (open ranges stay open so zoom 20 keeps working).
        upper = "-" if is_open else "-{}".format(high)
        return sel[: m.start()] + "|z{}{}".format(MIN_ZOOM, upper) + sel[m.end() :]
    # Entirely below 16: drop when a sibling covers 16+, else raise to 16.
    if sibling_covers_16(rules, sel, kind):
        return None
    return sel[: m.start()] + "|z{}".format(MIN_ZOOM) + sel[m.end() :]


def transform_rule(selectors: list[str], body: str, rules: list[dict], only_kind: str = "") -> list[str] | None:
    """Return rewritten selectors, [] to drop the rule, or None to keep as-is."""
    kind = is_poi_rule(body)
    if not kind:
        return None
    if only_kind and kind != only_kind:
        return None
    out: list[str] = []
    for sel in selectors:
        new_sel = transform_selector(sel, kind, rules)
        if new_sel is not None:
            out.append(new_sel)
    if not out:
        return []
    if out == selectors:
        return None
    return out


def find_section(lines: list[str], start_marker: str, end_marker: str) -> tuple[int, int]:
    lo = next(i for i, line in enumerate(lines) if start_marker in line)
    hi = next(i for i, line in enumerate(lines) if end_marker in line)
    if hi <= lo:
        raise SystemExit("section markers out of order in mapcss")
    return lo + 1, hi + 1  # 1-based inclusive lo, exclusive hi


def process(
    path: str,
    line_from: int = 1,
    line_to: int | None = None,
    only_kind: str = "",
) -> tuple[str, int, int]:
    with open(path) as f:
        lines = f.readlines()
    total = len(lines)
    lo = max(1, line_from) - 1
    hi = total if line_to is None else min(total, line_to)
    changed_rules = 0
    dropped_rules = 0
    rules = parse_rules(lines, lo, hi)
    out: list[str] = []
    out.extend(lines[:lo])
    for rule in rules:
        if rule["verbatim"]:
            out.extend(rule["raw"])
            continue
        selectors: list[str] = rule["selectors"]
        body = "".join(rule["body"])
        new_selectors = transform_rule(selectors, body, rules, only_kind)
        if new_selectors is None:
            out.extend(rule["raw"])
        elif not new_selectors:
            dropped_rules += 1
        else:
            changed_rules += 1
            out.append(",\n".join(new_selectors) + "\n{\n")
            out.extend(rule["body"])
            out.append(rule["closing"])
    out.extend(lines[hi:])
    with open(path, "w") as f:
        f.writelines(out)
    return path, changed_rules, dropped_rules


def bump_opacity(path: str, old: str, new: str, count_expected: int) -> None:
    with open(path) as f:
        content = f.read()
    found = content.count(old)
    if found == 0 and content.count(new) == count_expected:
        return  # Already applied (fixpoint re-run): nothing to do.
    if found != count_expected:
        raise SystemExit(
            "opacity anchor {!r} found {}x in {}, expected {}x".format(old, found, path, count_expected)
        )
    with open(path, "w") as f:
        f.write(content.replace(old, new))


def main(argv: list[str]) -> None:
    if len(argv) < 2:
        raise SystemExit("usage: abastevo_style_rules.py <file.mapcss> [--basemap-label]")
    path = argv[1]
    only_section = "--basemap-label" in argv
    if only_section:
        # Pass 1: POI outline icons everywhere (forest/park); pass 2: the
        # landuse-caption section (industrial -> commercial). Geographic names
        # (places, water, nature) are never touched.
        _, changed_icons, dropped_icons = process(path, only_kind="icon")
        with open(path) as f:
            section_lines = f.readlines()
        lo, hi = find_section(section_lines, "/* 7. LANDUSE */", "/* 8.BUILDINGS */")
        _, changed, dropped = process(path, lo, hi)
        changed += changed_icons
        dropped += dropped_icons
    else:
        _, changed, dropped = process(path)
    # Minor-road borders become visible; the gray tone is per-mode in colors.
    if path.endswith("default/include/Roads.mapcss"):
        bump_opacity(path, "casing-color: @minor_road_border; casing-opacity: 0.1;", "casing-color: @minor_road_border; casing-opacity: 1;", 1)
        bump_opacity(path, "color: @minor_road_border; opacity: 0.1;", "color: @minor_road_border; opacity: 1;", 1)
    print("{}: {} rules raised to z16, {} sub-z16 rules dropped".format(path, changed, dropped))


if __name__ == "__main__":
    main(sys.argv)
