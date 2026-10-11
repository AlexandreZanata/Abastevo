#!/usr/bin/env python3
"""Build original, texture-free glTF 2.0 fuel props. SPDX-License-Identifier: MIT.

Geometry/GLB/HTML generation uses Python's standard library. --preview uses
the workstation's existing Pillow/NumPy for a documentation-only contact sheet.
Run from the repository root; generated files are intentionally not app assets.
"""
import argparse
import base64
import hashlib
import json
import math
from pathlib import Path
import struct

OUT = Path('docs/assets/games/fuel-v1')
COLORS = ['#FD7805', '#D59F05', '#C2185B', '#209437', '#0372F3', '#5D24CF', '#FFBF00', '#FD6503']
SPECS = [
    ('gasoline_regular', 'GASOLINE_REGULAR', 'Gasolina comum', 'pump'),
    ('gasoline_additive', 'GASOLINE_PREMIUM', 'Gasolina aditivada', 'pump'),
    ('gasoline_premium', 'GASOLINE_PREMIUM_GRADE', 'Gasolina premium', 'pump'),
    ('ethanol', 'ETHANOL', 'Etanol', 'drop'),
    ('diesel_s500', 'DIESEL_S500', 'Diesel S500', 'pump'),
    ('diesel_s10', 'DIESEL_S10', 'Diesel S10', 'pump'),
    ('cng', 'CNG', 'GNV', 'pump'),
    ('lpg_p13', 'LPG_P13', 'GLP P13', 'cylinder'),
]


def sub(a, b):
    return tuple(x-y for x, y in zip(a, b))


def cross(a, b):
    return (a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2], a[0]*b[1]-a[1]*b[0])


def unit(v):
    size = math.sqrt(sum(x*x for x in v))
    return tuple(x/size for x in v) if size > 1e-10 else None


class Mesh:
    def __init__(self):
        self.faces = []

    def tri(self, a, b, c, material):
        normal = unit(cross(sub(b, a), sub(c, a)))
        if normal is not None:
            self.faces.append((a, b, c, normal, material))

    def polygon(self, points, z0, z1, material):
        # Ear clipping supports the concave crown/leaf without overlapping fans.
        area = sum(a[0]*b[1]-b[0]*a[1] for a, b in zip(points, points[1:]+points[:1]))
        points = points if area > 0 else list(reversed(points))
        remaining = list(range(len(points)))
        def side(a, b, p):
            return (b[0]-a[0])*(p[1]-a[1])-(b[1]-a[1])*(p[0]-a[0])
        while len(remaining) > 2:
            for j, idx in enumerate(remaining):
                before, after = remaining[j-1], remaining[(j+1) % len(remaining)]
                a, b, c = points[before], points[idx], points[after]
                if side(a, b, c) <= 1e-10:
                    continue
                if any(all(side(u, v, points[k]) >= -1e-10 for u, v in [(a, b), (b, c), (c, a)])
                       for k in remaining if k not in (before, idx, after)):
                    continue
                self.tri((*a, z1), (*b, z1), (*c, z1), material)
                self.tri((*c, z0), (*b, z0), (*a, z0), material)
                remaining.pop(j)
                break
            else:
                raise ValueError('Non-simple or degenerate polygon')
        for a, b in zip(points, points[1:]+points[:1]):
            self.tri((*a, z0), (*b, z0), (*b, z1), material)
            self.tri((*a, z0), (*b, z1), (*a, z1), material)

    def box(self, x, y, z, width, height, depth, material, radius=.06):
        radius = min(radius, width/2-.001, height/2-.001)
        bevel = min(.025, depth/4, radius/2)
        def ring(inset, zz):
            pts = []
            r = radius-inset
            for cx, cy, start in [(width/2-radius, height/2-radius, 0),
                                   (-width/2+radius, height/2-radius, 90),
                                   (-width/2+radius, -height/2+radius, 180),
                                   (width/2-radius, -height/2+radius, 270)]:
                for step in range(5):
                    angle = math.radians(start+step*90/4)
                    pts.append((x+cx+r*math.cos(angle), y+cy+r*math.sin(angle), zz))
            return pts
        rings = [ring(bevel, z-depth/2), ring(0, z-depth/2+bevel),
                 ring(0, z+depth/2-bevel), ring(bevel, z+depth/2)]
        for bottom, top in zip(rings, rings[1:]):
            for i in range(len(bottom)):
                j = (i+1) % len(bottom)
                self.tri(bottom[i], bottom[j], top[j], material)
                self.tri(bottom[i], top[j], top[i], material)
        for i in range(1, len(rings[0])-1):
            self.tri(rings[0][0], rings[0][i+1], rings[0][i], material)
            self.tri(rings[-1][0], rings[-1][i], rings[-1][i+1], material)

    def lathe(self, profile, material, x=0, z=0, sides=20):
        rings = [[(x+r*math.cos(i*math.tau/sides), y, z+r*math.sin(i*math.tau/sides))
                  for i in range(sides)] for y, r in profile]
        for lower, upper in zip(rings, rings[1:]):
            for i in range(sides):
                j = (i+1) % sides
                self.tri(lower[i], upper[j], lower[j], material)
                self.tri(lower[i], upper[i], upper[j], material)

    def tube(self, path, radius, material, sides=8):
        rings = []
        for i, p in enumerate(path):
            direction = unit(sub(path[min(i+1, len(path)-1)], path[max(i-1, 0)]))
            across = unit(cross(direction, (0, 0, 1)))
            other = cross(direction, across)
            rings.append([tuple(p[k]+radius*(across[k]*math.cos(j*math.tau/sides)
                          +other[k]*math.sin(j*math.tau/sides)) for k in range(3)) for j in range(sides)])
        for lower, upper in zip(rings, rings[1:]):
            for i in range(sides):
                j = (i+1) % sides
                self.tri(lower[i], lower[j], upper[j], material)
                self.tri(lower[i], upper[j], upper[i], material)
        for i in range(1, sides-1):
            self.tri(rings[0][0], rings[0][i+1], rings[0][i], material)
            self.tri(rings[-1][0], rings[-1][i], rings[-1][i+1], material)


DROP = [(0, .36), (-.10, .23), (-.20, .06), (-.22, -.08), (-.18, -.20),
        (-.10, -.27), (0, -.29), (.10, -.27), (.18, -.20), (.22, -.08), (.20, .06), (.10, .23)]


def build(kind, name):
    m = Mesh()
    if kind == 'pump':
        m.box(-.16, .09, 0, 1.24, .18, .66, 0)
        m.box(-.16, .94, 0, 1.04, 1.58, .55, 0, .12)
        m.box(-.16, 1.33, .29, .80, .49, .045, 1, .07)
        m.box(-.16, 1.33, .32, .66, .35, .025, 2, .04)
        # Blank neutral display bars, never a fabricated fuel price.
        for dx in [-.21, -.07, .07, .21]:
            m.box(-.16+dx, 1.33, .34, .085, .035, .012, 1, .01)
        path = [(.36, .83, 0), (.50, .83, 0), (.59, .75, 0), (.59, .31, 0)]
        path += [(.77+.18*math.cos(math.pi+i*math.pi/8), .31+.18*math.sin(math.pi+i*math.pi/8), 0) for i in range(1, 9)]
        path += [(.95, .78, 0), (.95, 1.12, 0)]
        m.tube(path, .045, 2)
        m.box(.91, 1.20, .01, .18, .38, .18, 0, .04)
        m.box(.91, 1.20, .11, .075, .15, .025, 1, .02)
        m.tube([(.91, 1.38, .01), (.77, 1.51, .01), (.72, 1.55, .01)], .047, 2)
        if name == 'cng':
            m.box(-.16, .67, .31, .72, .30, .08, 1, .14)
            for dx in [-.21, .21]:
                m.box(-.16+dx, .67, .36, .035, .29, .025, 0, .01)
            m.box(-.16, .88, .31, .16, .05, .08, 1, .015)
        else:
            m.polygon([(x-.16, y+.65) for x, y in DROP], .28, .32, 1)
            if name == 'gasoline_premium':
                crown = [(-.14, .59), (.14, .59), (.17, .76), (.075, .69), (0, .82), (-.075, .69), (-.17, .76)]
                m.polygon([(x-.16, y) for x, y in crown], .32, .34, 0)
            if name == 'gasoline_additive':
                plus = [(-.025, -.10), (.025, -.10), (.025, -.025), (.085, -.025),
                        (.085, .025), (.025, .025), (.025, .10), (-.025, .10),
                        (-.025, .025), (-.085, .025), (-.085, -.025), (-.025, -.025)]
                m.polygon([(x-.16, y+.65) for x, y in plus], .32, .35, 0)
            if name.startswith('diesel'):
                for i in range(1 if name == 'diesel_s10' else 3):
                    m.box(-.16+(i-(0 if name == 'diesel_s10' else 1))*.09, .65, .34, .035, .15, .025, 0, .01)
    elif kind == 'drop':
        m.lathe([(0, 0), (.025, .17), (.10, .32), (.24, .43), (.43, .49),
                 (.64, .47), (.86, .38), (1.10, .25), (1.34, .11), (1.55, 0)], 0, sides=24)
        leaf = [(-.19, .37), (-.22, .55), (-.17, .75), (-.02, .88), (.22, 1.02),
                (.23, .78), (.14, .57), (-.04, .43)]
        m.polygon(leaf, .51, .54, 1)
        m.tube([(-.18, .35, .555), (-.10, .54, .555), (.02, .71, .555), (.16, .91, .555)], .015, 0, sides=6)
    else:
        m.lathe([(0, 0), (0, .36), (.09, .36), (.10, .31), (.18, .39), (.30, .47),
                 (.91, .47), (1.05, .39), (1.13, .23), (1.15, .12), (1.15, 0)], 0, sides=24)
        m.box(0, 1.19, 0, .10, .16, .10, 2, .02)
        m.box(0, 1.28, 0, .27, .055, .13, 1, .02)
        m.tube([(-.25, 1.10, 0), (-.25, 1.42, 0), (.25, 1.42, 0), (.25, 1.10, 0)], .055, 0)
        m.box(0, .63, .48, .46, .33, .035, 1, .06)
        m.polygon([(x*.30, y*.30+.63) for x, y in DROP], .50, .52, 0)
    return m


def rgb(hex_color):
    return [int(hex_color[i:i+2], 16)/255 for i in (1, 3, 5)]


def export_glb(mesh, color, name, wire):
    blob = bytearray()
    doc = {'asset': {'version': '2.0', 'generator': 'abastevo fuel-v1 stdlib builder',
                     'copyright': 'MIT; see repository LICENSE'},
           'scene': 0, 'scenes': [{'nodes': [0]}],
           'nodes': [{'name': name, 'mesh': 0, 'extras': {'fuelProduct': wire, 'draft': True}}],
           'meshes': [{'name': name, 'primitives': []}], 'materials': [],
           'buffers': [], 'bufferViews': [], 'accessors': []}
    colors = [rgb(color), rgb('#F5F5EF'), rgb('#273343')]
    for i, c in enumerate(colors):
        doc['materials'].append({'name': ['category', 'ivory', 'charcoal'][i],
                                 'pbrMetallicRoughness': {'baseColorFactor': c+[1], 'metallicFactor': 0,
                                                       'roughnessFactor': .38 if i == 0 else .55}})
    def accessor(values, fmt, kind, component, target, bounds=False):
        while len(blob) % 4:
            blob.append(0)
        start = len(blob)
        flat = [v for item in values for v in item] if kind == 'VEC3' else values
        blob.extend(struct.pack('<'+fmt*len(flat), *flat))
        view = len(doc['bufferViews'])
        doc['bufferViews'].append({'buffer': 0, 'byteOffset': start, 'byteLength': len(blob)-start, 'target': target})
        item = {'bufferView': view, 'componentType': component, 'count': len(values), 'type': kind}
        if bounds:
            item.update(min=[min(v[i] for v in values) for i in range(3)], max=[max(v[i] for v in values) for i in range(3)])
        doc['accessors'].append(item)
        return len(doc['accessors'])-1
    for material in range(3):
        positions, normals, indices, vertices = [], [], [], {}
        for a, b, c, n, mat in mesh.faces:
            if mat == material:
                for point in (a, b, c):
                    key = tuple(round(value, 9) for value in (*point, *n))
                    if key not in vertices:
                        vertices[key] = len(positions)
                        positions.append(point)
                        normals.append(n)
                    indices.append(vertices[key])
        if positions:
            pos = accessor(positions, 'f', 'VEC3', 5126, 34962, True)
            normal = accessor(normals, 'f', 'VEC3', 5126, 34962)
            indices = accessor(indices, 'H', 'SCALAR', 5123, 34963)
            doc['meshes'][0]['primitives'].append({'attributes': {'POSITION': pos, 'NORMAL': normal},
                                                  'indices': indices, 'material': material, 'mode': 4})
    while len(blob) % 4:
        blob.append(0)
    doc['buffers'] = [{'byteLength': len(blob)}]
    metadata = json.dumps(doc, separators=(',', ':'), ensure_ascii=True).encode()
    metadata += b' '*((-len(metadata)) % 4)
    content = struct.pack('<III', 0x46546C67, 2, 12+8+len(metadata)+8+len(blob))
    content += struct.pack('<II', len(metadata), 0x4E4F534A)+metadata
    content += struct.pack('<II', len(blob), 0x004E4942)+blob
    path = OUT/(name+'.glb')
    path.write_bytes(content)
    points = [p for face in mesh.faces for p in face[:3]]
    return {'file': path.name, 'fuel_product': wire, 'category_ink': color, 'bytes': len(content),
            'triangles': len(mesh.faces), 'primitives': len(doc['meshes'][0]['primitives']),
            'bounds_m': {'min': [round(min(p[i] for p in points), 5) for i in range(3)],
                         'max': [round(max(p[i] for p in points), 5) for i in range(3)]},
            'sha256': hashlib.sha256(content).hexdigest()}


def preview(meshes):
    import numpy as np
    from PIL import Image, ImageDraw, ImageFont
    scale = 2
    image = Image.new('RGB', (1600*scale, 1000*scale), '#101923')
    d = ImageDraw.Draw(image)
    font_path = '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf'
    title_font = ImageFont.truetype(font_path, 34*scale)
    font = ImageFont.truetype(font_path, 20*scale)
    small = ImageFont.truetype(font_path, 14*scale)
    d.text((50*scale, 30*scale), 'ABASTEVO / FUEL GAME ASSETS', font=title_font, fill='#F5F5EF')
    d.text((50*scale, 81*scale), '8 real GLB meshes / texture-free / original project palette / V1 draft', font=font, fill='#AAB8C6')
    angle, tilt = -.38, .23
    def rotate(p):
        x, y, z = p
        x, z = x*math.cos(angle)+z*math.sin(angle), -x*math.sin(angle)+z*math.cos(angle)
        return (x, y*math.cos(tilt)-z*math.sin(tilt), y*math.sin(tilt)+z*math.cos(tilt))
    light = unit((-.4, .8, 1))
    for index, (m, record) in enumerate(meshes):
        col, row = index % 4, index//4
        ox, oy = 45+col*390, 150+row*410
        d.rounded_rectangle((ox*scale, oy*scale, (ox+365)*scale, (oy+386)*scale), radius=16*scale, fill='#192633')
        pixels = np.full((300*scale, 365*scale, 3), (25, 38, 51), dtype=np.uint8)
        depth = np.full(pixels.shape[:2], -np.inf)
        palette = [rgb(record['category_ink']), rgb('#F5F5EF'), rgb('#273343')]
        for a, b, c, normal, material in m.faces:
            n = rotate(normal)
            if n[2] <= 0:
                continue
            points = [rotate(p) for p in (a, b, c)]
            points2d = [((160+p[0]*140)*scale, (277-p[1]*140)*scale) for p in points]
            brightness = .42+.58*max(0, sum(x*y for x, y in zip(n, light)))
            color = tuple(int(255*min(1, c*brightness)) for c in palette[material])
            # Barycentric Z buffer: centroid painter sorting hides raised badges.
            x0 = max(0, math.floor(min(p[0] for p in points2d)))
            x1 = min(pixels.shape[1]-1, math.ceil(max(p[0] for p in points2d)))
            y0 = max(0, math.floor(min(p[1] for p in points2d)))
            y1 = min(pixels.shape[0]-1, math.ceil(max(p[1] for p in points2d)))
            if x1 < x0 or y1 < y0:
                continue
            (ax, ay), (bx, by), (cx, cy) = points2d
            denominator = (by-cy)*(ax-cx)+(cx-bx)*(ay-cy)
            if abs(denominator) < 1e-8:
                continue
            yy, xx = np.mgrid[y0:y1+1, x0:x1+1]
            u = ((by-cy)*(xx+.5-cx)+(cx-bx)*(yy+.5-cy))/denominator
            v = ((cy-ay)*(xx+.5-cx)+(ax-cx)*(yy+.5-cy))/denominator
            w = 1-u-v
            zz = u*points[0][2]+v*points[1][2]+w*points[2][2]
            old = depth[y0:y1+1, x0:x1+1]
            mask = (u >= -1e-7) & (v >= -1e-7) & (w >= -1e-7) & (zz > old)
            old[mask] = zz[mask]
            pixels[y0:y1+1, x0:x1+1][mask] = color
        image.paste(Image.fromarray(pixels), (ox*scale, oy*scale))
        d.text(((ox+22)*scale, (oy+307)*scale), record['label'], font=font, fill='#F5F5EF')
        d.text(((ox+22)*scale, (oy+343)*scale), f"{record['triangles']:,} triangles / {record['bytes']/1024:.1f} KiB", font=small, fill='#AAB8C6')
    image.resize((1600, 1000), Image.Resampling.LANCZOS).save(OUT/'contact-sheet.png', optimize=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--preview', action='store_true')
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)
    records, meshes = [], []
    aliases = {'gasoline_additive': 'gasoline_premium', 'gasoline_premium': 'gasoline_premium_grade'}
    for (name, wire, label, kind), color in zip(SPECS, COLORS):
        mesh = build(kind, name)
        record = export_glb(mesh, color, name, wire)
        source = Path('docs/assets/fuel-icons/transparent')/('ic_fuel_'+aliases.get(name, name)+'.png')
        record.update(label=label, source_icon=str(source), source_icon_sha256=hashlib.sha256(source.read_bytes()).hexdigest())
        assert record['bytes'] <= 128*1024 and record['triangles'] <= 3000
        records.append(record)
        meshes.append((mesh, record))
    manifest = {'format_version': 1, 'status': 'ASSET_DRAFT_LOCAL_DONE', 'license': 'MIT',
                'generator': 'scripts/assets/build-fuel-game-assets.py', 'units': 'metres',
                'coordinates': 'right-handed, Y up, front +Z, base Y=0; artistic scale',
                'textures': 0, 'animations': 0, 'total_glb_bytes': sum(r['bytes'] for r in records), 'assets': records}
    assert manifest['total_glb_bytes'] <= 1024*1024
    (OUT/'manifest.json').write_text(json.dumps(manifest, indent=2, ensure_ascii=False)+'\n')
    # The viewer embeds exact GLB bytes for genuinely offline inspection.
    data = [{**r, 'glb': base64.b64encode((OUT/r['file']).read_bytes()).decode()} for r in records]
    template = Path('scripts/assets/fuel-game-viewer.template.html').read_text()
    (OUT/'preview.html').write_text(template.replace('__ASSETS__', json.dumps(data, ensure_ascii=False)))
    if args.preview:
        preview(meshes)
    print(f"Built {len(records)} models: {manifest['total_glb_bytes']} GLB bytes; {sum(r['triangles'] for r in records)} triangles")


if __name__ == '__main__':
    main()
