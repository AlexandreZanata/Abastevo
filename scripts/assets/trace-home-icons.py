#!/usr/bin/env python3
"""Trace generated Home pictograms with the existing local VTracer tool.

Build-time asset tool only; Pillow, NumPy, OpenCV and VTracer are not app dependencies.
Raw generated sources stay unchanged. Transparent single-ink PNGs and filled
SVG masters and Android PNG exports are reproducible from those sources.
"""
import argparse
from pathlib import Path
import tempfile
import xml.etree.ElementTree as ET

import cv2
import numpy as np
from PIL import Image
import vtracer

COLORS = {
    "savings": "#209437", "community": "#1478DD", "trust": "#F57C00",
    "stations": "#1478DD", "history": "#209437", "location": "#1478DD",
}
SVG = "http://www.w3.org/2000/svg"


def trace(root: Path, name: str, color: str) -> None:
    assets = root / "docs/assets/home-icons"
    source = assets / "sources" / f"ic_home_{name}.png"
    pixels = np.asarray(Image.open(source).convert("RGBA"))
    rgb = pixels[:, :, :3].astype(np.int16)
    # Binarize colored ink to full opacity; generated shading/soft alpha
    # must not become extra colors or unintended holes in the single-ink SVG.
    # Exclude white details so they remain negative-space cutouts.
    mask = (pixels[:, :, 3] >= 128) & ((rgb.max(2) - rgb.min(2)) >= 45)
    ys, xs = np.where(mask)
    if len(xs) == 0:
        raise ValueError(f"No colored silhouette: {source}")
    crop = Image.fromarray((mask * 255).astype(np.uint8)).crop(
        (xs.min(), ys.min(), xs.max() + 1, ys.max() + 1)
    )
    # Keep every source pixel; do not downsample before tracing. A tiny
    # subpixel edge filter removes raster alias noise without moving the form.
    side = round(max(crop.width, crop.height) / 0.875)
    alpha = Image.new("L", (side, side))
    alpha.paste(crop, ((side - crop.width) // 2, (side - crop.height) // 2))
    edge = cv2.GaussianBlur(np.asarray(alpha), (0, 0), sigmaX=2.4)
    alpha = Image.fromarray(edge)
    normalized = Image.new("RGBA", (side, side), color)
    normalized.putalpha(alpha)
    transparent = assets / "transparent"
    transparent.mkdir(exist_ok=True)
    normalized.save(transparent / f"ic_home_{name}.png", optimize=True)
    with tempfile.TemporaryDirectory(prefix="abastevo-trace-") as temporary:
        bitmap = Path(temporary) / "mask.png"
        traced = Path(temporary) / "trace.svg"
        # Trace at double sampling for subpixel curves; original PNG pixels
        # stay at their native resolution. No downsampling of the source.
        trace_side = side * 2
        trace_alpha = alpha.resize((trace_side, trace_side), Image.Resampling.BICUBIC)
        Image.fromarray(np.where(np.asarray(trace_alpha) >= 128, 0, 255).astype(np.uint8)).save(bitmap)
        vtracer.convert_image_to_svg_py(
            str(bitmap), str(traced), colormode="binary", mode="spline",
            filter_speckle=12, corner_threshold=20 if name == "community" else 60 if name == "history" else 90,
            length_threshold=14 if name == "community" else 8 if name == "history" else 4,
            max_iterations=10, splice_threshold=60 if name == "history" else 20, path_precision=8,
        )
        paths = ET.parse(traced).getroot().findall(f"{{{SVG}}}path")
    svg = ET.Element("svg", xmlns=SVG, width="64", height="64", viewBox=f"0 0 {trace_side} {trace_side}")
    import re
    for path in paths:
        attributes = {"fill": color, "d": path.attrib["d"]}
        transform = path.get("transform")
        if transform:
            match = re.fullmatch(r"translate\(([-\d.]+),\s*([-\d.]+)\)", transform)
            if not match:
                raise ValueError(f"Unexpected VTracer transform: {transform}")
            attributes["transform"] = transform
        ET.SubElement(svg, "path", attributes)
    ET.indent(svg)
    ET.ElementTree(svg).write(assets / f"ic_home_{name}.svg", encoding="utf-8", xml_declaration=True)
    # Match the fuel pipeline: complex high-resolution SVG masters stay in
    # docs; Android uses sharp local PNGs to avoid aapt string-size limits.
    import subprocess
    subprocess.run([
        "inkscape", str(assets / f"ic_home_{name}.svg"),
        "--export-width=256", "--export-height=256",
        "--export-filename=" + str(root / "app/src/main/res/drawable-nodpi" / f"ic_home_{name}.png"),
    ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    print(f"{name}: {len(paths)} paths, single ink {color}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    args = parser.parse_args()
    for icon, ink in COLORS.items():
        trace(args.root, icon, ink)
