#!/usr/bin/env python3
"""Generate the compact Brazil map-region lookup asset from CoMaps borders.

Reads the pinned CoMaps `data/borders/Brazil_*.poly` files (one file per
leaf country id, e.g. `Brazil_Mato Grosso.poly`), simplifies every ring with
Douglas-Peucker and writes `data/src/main/assets/map_regions_brazil.txt`.
The output is deterministic for the same inputs and tolerance.

Format `abastevo.map-regions-v1` (UTF-8, LF, tab separated):

    abastevo.map-regions-v1
    # comment lines (provenance)
    R<TAB><countryId>
    O<TAB><ring>     outer ring of the preceding region
    H<TAB><ring>     hole of the preceding region

A ring is `x,y;dx,dy;...` in integer units of 1e-4 degrees (x = longitude,
y = latitude): the first vertex is absolute, the following ones are deltas
from the previous vertex. Rings are implicitly closed. Lookup uses the
even-odd rule over all rings of a region, so holes and multipolygons are
both correct.

Usage:
    python3 scripts/maps/generate-map-regions.py \
        --borders /data/dev/src/comaps-4587b8ad/data/borders \
        --out data/src/main/assets/map_regions_brazil.txt
"""

import argparse
from pathlib import Path
import re
import sys

FORMAT = 'abastevo.map-regions-v1'
SCALE = 10_000
COUNTRY_ID = re.compile(r'^Brazil_[A-Za-z0-9_ ]+$')


def parse_poly(path):
    """Return [(is_hole, [(lon, lat), ...]), ...] from an Osmosis .poly file."""
    lines = [line.strip() for line in path.read_text(encoding='utf-8').splitlines()]
    rings = []
    index = 1  # first line is the polygon name
    while index < len(lines):
        header = lines[index]
        index += 1
        if header == 'END':
            break
        if not header:
            continue
        is_hole = header.startswith('!')
        points = []
        while index < len(lines) and lines[index] != 'END':
            parts = lines[index].split()
            index += 1
            if len(parts) != 2:
                raise ValueError(f'{path.name}: malformed coordinate line')
            points.append((float(parts[0]), float(parts[1])))
        index += 1  # ring END
        if len(points) > 1 and points[0] == points[-1]:
            points.pop()
        if len(points) < 3:
            raise ValueError(f'{path.name}: ring with fewer than 3 points')
        rings.append((is_hole, points))
    if not rings or not any(not hole for hole, _ in rings):
        raise ValueError(f'{path.name}: no outer ring')
    return rings


def perpendicular(point, start, end):
    (px, py), (ax, ay), (bx, by) = point, start, end
    dx, dy = bx - ax, by - ay
    if dx == 0 and dy == 0:
        return ((px - ax) ** 2 + (py - ay) ** 2) ** 0.5
    t = max(0.0, min(1.0, ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy)))
    cx, cy = ax + t * dx, ay + t * dy
    return ((px - cx) ** 2 + (py - cy) ** 2) ** 0.5


def douglas_peucker(points, tolerance):
    """Iterative Douglas-Peucker on an open polyline; keeps both endpoints."""
    keep = [False] * len(points)
    keep[0] = keep[-1] = True
    stack = [(0, len(points) - 1)]
    while stack:
        first, last = stack.pop()
        best, best_index = 0.0, -1
        for i in range(first + 1, last):
            distance = perpendicular(points[i], points[first], points[last])
            if distance > best:
                best, best_index = distance, i
        if best_index >= 0 and best > tolerance:
            keep[best_index] = True
            stack.append((first, best_index))
            stack.append((best_index, last))
    return [p for p, k in zip(points, keep) if k]


def simplify_ring(points, tolerance):
    # Split the closed ring at vertex 0 and its farthest vertex so both halves
    # are open polylines with fixed endpoints.
    far = max(range(len(points)), key=lambda i: (points[i][0] - points[0][0]) ** 2 + (points[i][1] - points[0][1]) ** 2)
    first = douglas_peucker(points[: far + 1], tolerance)
    second = douglas_peucker(points[far:] + [points[0]], tolerance)
    ring = first[:-1] + second[:-1]
    return ring if len(ring) >= 3 else points[:3]


def encode_ring(points):
    out = []
    prev = None
    for lon, lat in points:
        x, y = round(lon * SCALE), round(lat * SCALE)
        if prev is None:
            out.append(f'{x},{y}')
        else:
            dx, dy = x - prev[0], y - prev[1]
            if dx == 0 and dy == 0:
                continue
            out.append(f'{dx},{dy}')
        prev = (x, y)
    return ';'.join(out)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--borders', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    parser.add_argument('--tolerance', type=float, default=0.01, help='degrees')
    parser.add_argument('--expected-count', type=int, default=23)
    args = parser.parse_args(argv)

    files = sorted(args.borders.glob('Brazil_*.poly'), key=lambda p: p.stem)
    if len(files) != args.expected_count:
        raise SystemExit(f'expected {args.expected_count} Brazil_*.poly files, found {len(files)}')
    lines = [
        FORMAT,
        f'# source: CoMaps data/borders/Brazil_*.poly ({len(files)} leaf country ids)',
        f'# Douglas-Peucker tolerance {args.tolerance} deg; integer 1e-4 deg, delta encoded',
        '# regenerate: scripts/maps/generate-map-regions.py',
    ]
    total = 0
    for path in files:
        country_id = path.stem
        if not COUNTRY_ID.match(country_id):
            raise SystemExit(f'unexpected country id {country_id!r}')
        lines.append(f'R\t{country_id}')
        for is_hole, points in parse_poly(path):
            ring = simplify_ring(points, args.tolerance)
            total += len(ring)
            lines.append(('H' if is_hole else 'O') + '\t' + encode_ring(ring))
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text('\n'.join(lines) + '\n', encoding='utf-8')
    print(f'{args.out}: {len(files)} regions, {total} vertices, {args.out.stat().st_size} bytes', file=sys.stderr)


if __name__ == '__main__':
    main()
