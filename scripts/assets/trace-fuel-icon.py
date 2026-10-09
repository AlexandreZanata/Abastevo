#!/usr/bin/env python3
"""Normalize a generated fuel PNG to one category ink and trace actual SVG paths.

Build-time dependencies only: Pillow, NumPy, VTracer and Inkscape. Preserve raw
sources. White interior details match the existing fuel set; background stays
transparent. No embedded raster is permitted in the resulting SVG.
"""
import argparse
import hashlib
import colorsys
from collections import Counter
import json
import re
import importlib.metadata
from pathlib import Path
import subprocess
import tempfile
import xml.etree.ElementTree as ET

import numpy as np
from PIL import Image
import vtracer

SVG = "http://www.w3.org/2000/svg"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--name", required=True)
    parser.add_argument("--color", required=True)
    parser.add_argument("--size", type=int, default=128)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    assets = root / "docs/assets/fuel-icons"
    if not re.fullmatch(r"[a-z][a-z0-9_]*", args.name) or args.size != 128:
        parser.error("Use a fuel resource name and the existing 128px export size")
    color = args.color.upper()
    ink = np.array(tuple(bytes.fromhex(color.removeprefix("#"))), dtype=np.uint8)
    if len(ink) != 3:
        parser.error("Color must be #RRGGBB")
    # Require a perceptually different hue as well as a distinct RGB value.
    candidate_hue = colorsys.rgb_to_hsv(*(int(c)/255 for c in ink))[0]*360
    for existing in (root / "app/src/main/res/drawable-nodpi").glob("ic_fuel_*.png"):
        if existing.stem == f"ic_fuel_{args.name}":
            continue
        a = np.asarray(Image.open(existing).convert("RGBA"))
        rgb = a[:,:,:3].astype(np.int16)
        saturated = (a[:,:,3]>200) & ((rgb.max(2)-rgb.min(2))>45)
        if not saturated.any():
            continue
        dominant = Counter(map(tuple,rgb[saturated])).most_common(1)[0][0]
        hue = colorsys.rgb_to_hsv(*(int(c)/255 for c in dominant))[0]*360
        gap = abs(candidate_hue-hue)
        if min(gap,360-gap)<20:
            raise ValueError(f"Category hue already used by {existing.stem}; choose a different ink")
    pixels = np.asarray(Image.open(args.source).convert("RGBA"))
    visible = pixels[:, :, 3] >= 128
    ys, xs = np.where(visible)
    if not len(xs):
        raise ValueError("Empty image")
    pixels = pixels[ys.min():ys.max()+1, xs.min():xs.max()+1]
    visible = pixels[:, :, 3] >= 128
    # Saturated category ink vs neutral white; generated shading never adds inks.
    rgb = pixels[:, :, :3].astype(np.int16)
    colored = visible & ((rgb.max(2) - rgb.min(2)) > 35)
    neutral = visible & ~colored
    side = max(pixels.shape[:2])
    flat = np.zeros((side, side, 4), dtype=np.uint8)
    top, left = (side-pixels.shape[0])//2, (side-pixels.shape[1])//2
    block = flat[top:top+pixels.shape[0], left:left+pixels.shape[1]]
    block[colored, :3] = ink
    block[neutral, :3] = 255
    block[visible, 3] = 255
    master = assets / "transparent" / f"ic_fuel_{args.name}.png"
    Image.fromarray(flat).save(master, optimize=True)
    svg = ET.Element("svg", xmlns=SVG, width="128", height="128", viewBox=f"0 0 {side} {side}")
    path_count = 0
    with tempfile.TemporaryDirectory(prefix="abastevo-fuel-trace-") as tmp:
        for index, (mask, fill) in enumerate(((colored, color), (neutral, "#FFFFFF"))):
            canvas = np.full((side, side), 255, dtype=np.uint8)
            canvas[top:top+mask.shape[0], left:left+mask.shape[1]][mask] = 0
            source, traced = Path(tmp)/f"mask{index}.png", Path(tmp)/f"mask{index}.svg"
            Image.fromarray(canvas).save(source)
            vtracer.convert_image_to_svg_py(str(source), str(traced), colormode="binary", mode="spline", filter_speckle=4, corner_threshold=60, length_threshold=4, max_iterations=10, splice_threshold=45, path_precision=8)
            for path in ET.parse(traced).getroot().findall(f"{{{SVG}}}path"):
                # Binary VTracer emits only foreground black contours.
                if path.get("fill", "").upper() != "#000000":
                    raise ValueError("Unexpected trace background")
                attrs = {"d": path.attrib["d"], "fill": fill}
                if path.get("transform"):
                    attrs["transform"] = path.get("transform")
                ET.SubElement(svg, "path", attrs)
                path_count += 1
    ET.indent(svg)
    vector = assets / f"ic_fuel_{args.name}.svg"
    ET.ElementTree(svg).write(vector, encoding="utf-8", xml_declaration=True)
    android = root / "app/src/main/res/drawable-nodpi" / f"ic_fuel_{args.name}.png"
    subprocess.run(["inkscape", str(vector), f"--export-width={args.size}", f"--export-height={args.size}", "--export-filename="+str(android)], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    # Native-resolution fidelity against the normalized PNG master, not against
    # generated texture intentionally removed by the one-ink contract.
    with tempfile.TemporaryDirectory(prefix="abastevo-fuel-qa-") as tmp:
        rendered = Path(tmp)/"render.png"
        subprocess.run(["inkscape", str(vector), f"--export-width={side}", f"--export-height={side}", "--export-filename="+str(rendered)], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        output = np.asarray(Image.open(rendered).convert("RGBA"))
        a, b = flat[:,:,3]>=128, output[:,:,3]>=128
        iou = float((a&b).sum()/(a|b).sum())
        if iou < 0.99:
            raise ValueError(f"Silhouette fidelity below 99%: {iou}")
        # Compare category ink and neutral interior geometry independently.
        reference_ink = a & np.any(flat[:,:,:3] != 255, axis=2)
        distances_ink = ((output[:,:,:3].astype(float)-ink)**2).sum(2)
        distances_white = ((output[:,:,:3].astype(float)-255)**2).sum(2)
        rendered_ink = b & (distances_ink < distances_white)
        mask_scores = {}
        for name, ref, actual in (("ink",reference_ink,rendered_ink),("white",a & ~reference_ink,b & ~rendered_ink)):
            score = float((ref & actual).sum()/(ref | actual).sum())
            if score < 0.99:
                raise ValueError(f"{name} geometry fidelity below 99%: {score}")
            mask_scores[name+"_iou"] = round(score,6)
        meta = {"source_sha256": hashlib.sha256(args.source.read_bytes()).hexdigest(), "category_ink":color, "neutral_detail":"#FFFFFF", "svg_paths":path_count, "master_size":[side,side], "android_size":[args.size,args.size], "silhouette_iou":round(iou,6), "embedded_raster":False, **mask_scores, "tool_versions":{package:importlib.metadata.version(package) for package in ("Pillow","numpy","vtracer")}}
        (assets / f"ic_fuel_{args.name}.provenance.json").write_text(json.dumps(meta,indent=2)+"\n")
        print(json.dumps(meta))


if __name__ == "__main__":
    main()
