#!/usr/bin/env python3
"""Trace generated landing PNGs into real SVG contours; no runtime dependencies.

Uses the same preinstalled Pillow/NumPy/VTracer/Inkscape asset toolchain as
trace-fuel-icon.py. Raw generated PNGs are immutable inputs. Each icon is
flattened to one category ink plus white details before tracing both masks.
"""
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import xml.etree.ElementTree as ET

import cv2
import numpy as np
from PIL import Image, ImageFilter
import vtracer

ROOT = Path(__file__).resolve().parents[2]
INPUT = ROOT / 'docs/assets/landing/hero-20261011/icons'
OUTPUT = ROOT / 'landing/static/assets/art'
COLORS = {'consult': '#1478DD', 'check': '#209437', 'contribute': '#F57C00',
          'driver': '#1478DD', 'ride-driver': '#209437', 'fleet': '#F57C00'}
SVG = 'http://www.w3.org/2000/svg'


def trace(name, color):
    source = INPUT / 'sources' / f'{name}.png'
    rgba = np.array(Image.open(source).convert('RGBA'))
    visible = rgba[:, :, 3] >= 128
    count, labels, stats, _ = cv2.connectedComponentsWithStats(visible.astype(np.uint8), 8)
    retained = np.array([False] + [stats[i, cv2.CC_STAT_AREA] >= 64 for i in range(1, count)])
    visible = retained[labels]
    rgba[:, :, 3] = np.where(visible, rgba[:, :, 3], 0)
    ys, xs = np.where(visible)
    if not len(xs):
        raise ValueError(f'Empty generated icon: {name}')
    rgba = rgba[ys.min():ys.max()+1, xs.min():xs.max()+1]
    rgb = rgba[:, :, :3].astype(np.int16)
    visible = rgba[:, :, 3] >= 128
    # Preserve solid neutral interior details, discard neutral antialias halos.
    # The source generator can put pale fringes on the transparent outer edge.
    # Keep meaningful white components, then normalize with the established
    # home-icon 2.4px edge filter; all other visible pixels become category ink.
    white = visible & (rgb.min(2) >= 200) & (rgb.max(2) - rgb.min(2) <= 35) & (rgba[:, :, 3] >= 240)
    count, labels, stats, _ = cv2.connectedComponentsWithStats(white.astype(np.uint8), 8)
    retained = np.array([False] + [stats[i, cv2.CC_STAT_AREA] >= 64 for i in range(1, count)])
    white = retained[labels]
    # These filled pictograms use white interior details and exterior transparency.
    # Repair enclosed alpha dropouts from generation; intentional open gaps
    # between vehicle tires/map folds remain connected to the exterior.
    padded = np.pad(visible.astype(np.uint8), 1)
    exterior = padded.copy()
    cv2.floodFill(exterior, None, (0, 0), 1)
    visible |= (exterior[1:-1, 1:-1] == 0)
    visible = cv2.GaussianBlur(visible.astype(np.uint8)*255, (0, 0), 2.4) >= 128
    white = visible & (cv2.GaussianBlur(white.astype(np.uint8)*255, (0, 0), 2.4) >= 128)
    colored = visible & ~white
    side = round(max(rgba.shape[:2]) / .85)
    left, top = (side-rgba.shape[1])//2, (side-rgba.shape[0])//2
    normalized = np.zeros((side, side, 4), dtype=np.uint8)
    ink = np.array([int(color[i:i+2], 16) for i in (1, 3, 5)])
    block = normalized[top:top+rgba.shape[0], left:left+rgba.shape[1]]
    block[colored, :3] = ink
    block[white, :3] = 255
    block[visible, 3] = 255
    Image.fromarray(normalized).save(INPUT / 'normalized' / f'{name}.png', optimize=True)
    svg = ET.Element('svg', xmlns=SVG, width='96', height='96', viewBox=f'0 0 {side*2} {side*2}')
    with tempfile.TemporaryDirectory(prefix='abastevo-card-trace-') as tmp:
        tmp = Path(tmp)
        for mask, fill in [(colored, color), (white, '#FFFFFF')]:
            if not mask.any():
                continue
            canvas = np.full((side, side), 255, dtype=np.uint8)
            canvas[top:top+mask.shape[0], left:left+mask.shape[1]][mask] = 0
            sampled = Image.fromarray(canvas).resize((side*2, side*2), Image.Resampling.BICUBIC).filter(ImageFilter.GaussianBlur(1.2))
            Image.fromarray(np.where(np.array(sampled) < 128, 0, 255).astype(np.uint8)).save(tmp / 'mask.png')
            vtracer.convert_image_to_svg_py(str(tmp/'mask.png'), str(tmp/'trace.svg'),
                colormode='binary', mode='spline', filter_speckle=4,
                corner_threshold=180, length_threshold=4, max_iterations=10,
                splice_threshold=45, path_precision=4)
            for contour in ET.parse(tmp/'trace.svg').getroot().findall(f'{{{SVG}}}path'):
                if contour.get('fill', '').upper() != '#000000':
                    raise ValueError('Unexpected trace background')
                attrs = {'d': contour.attrib['d'], 'fill': fill}
                if contour.get('transform'):
                    attrs['transform'] = contour.get('transform')
                ET.SubElement(svg, 'path', attrs)
        content = ET.tostring(svg, encoding='utf-8', xml_declaration=True)
        if len(content) > 1024*1024:
            raise ValueError(f'SVG budget exceeded: {name} ({len(content)} bytes)')
        compressed = len(gzip.compress(content))
        if compressed > 80*1024:
            raise ValueError(f'SVG gzip budget exceeded: {name} ({compressed} bytes)')
        digest = hashlib.sha256(content).hexdigest()
        filename = f'{name}.{digest[:8]}.svg'
        vector = OUTPUT / filename
        vector.write_bytes(content)
        # Compare the rendered vector at original normalization resolution.
        subprocess.run(['inkscape', str(vector), f'--export-width={side}',
            f'--export-height={side}', '--export-filename='+str(tmp/'render.png')],
            check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        actual = np.asarray(Image.open(tmp/'render.png').convert('RGBA'))
        a, b = normalized[:, :, 3]>=128, actual[:, :, 3]>=128
        silhouette = float((a & b).sum() / (a | b).sum())
        if silhouette < .99:
            raise ValueError(f'Below 99% silhouette fidelity: {name}: {silhouette}')
        rendered_ink = b & (((actual[:,:,:3].astype(float)-ink)**2).sum(2) < ((actual[:,:,:3].astype(float)-255)**2).sum(2))
        scores = {}
        for label, reference, result in [('ink', a & np.any(normalized[:,:,:3]!=255,axis=2), rendered_ink), ('white', a & np.all(normalized[:,:,:3]==255,axis=2), b & ~rendered_ink)]:
            union = (reference | result).sum()
            score = float((reference & result).sum()/union) if union else 1.0
            if score < .99:
                raise ValueError(f'Below 99% {label} fidelity: {name}: {score}')
            scores[label+'_iou'] = round(score, 6)
    return {'file': filename, 'sha256': digest, 'bytes': len(content), 'gzipBytes': compressed, 'ink': color,
            'sourceSha256': hashlib.sha256(source.read_bytes()).hexdigest(),
            'paths': len(svg), 'silhouetteIou': round(silhouette, 6), **scores}


if __name__ == '__main__':
    outputs = [trace(name, color) for name, color in COLORS.items()]
    (INPUT / 'provenance.json').write_text(json.dumps({'method': 'Flat ink/white normalization, enclosed alpha-dropout repair, established 2.4px edge filter, 2x trace sampling, real VTracer spline contours, Inkscape render comparison; no embedded bitmap', 'outputs': outputs}, indent=2)+'\n')
    manifest_path = OUTPUT / 'provenance.json'
    manifest = json.loads(manifest_path.read_text())
    manifest['outputs'] = [x for x in manifest['outputs'] if not x['file'].endswith('.svg')] + outputs
    manifest_path.write_text(json.dumps(manifest, indent=2)+'\n')
    page = ROOT / 'landing/static/index.html'
    html = page.read_text()
    for name, item in zip(COLORS, outputs):
        import re
        html = re.sub(r'/assets/art/'+re.escape(name)+r'(?:\.[a-f0-9]{8})?\.svg', '/assets/art/'+item['file'], html)
    page.write_text(html)
    print(json.dumps(outputs, indent=2))
