#!/usr/bin/env python3
"""Render the Google Play store cards, feature graphic and icon.

Reads cards.json, fills card.html / feature.html with real emulator
screenshots from docs/store/screenshots/raw, renders each page with a local
headless Chrome/Chromium at the exact pixel size, and writes opaque RGB PNGs
(Play rejects alpha in screenshots and the feature graphic) to
docs/store/listing-assets.

Tooling only (not an app or backend dependency): Python 3 with Pillow and a
Chrome/Chromium binary. Fonts come from the app's own bundled Manrope files,
so the cards use the same typography as the app.

Usage:
    python3 docs/store/cards/build_cards.py [--chrome /path/to/chrome] [--only phone]
"""

from __future__ import annotations

import argparse
import html
import json
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from string import Template

from PIL import Image

CARDS_DIR = Path(__file__).resolve().parent
STORE_DIR = CARDS_DIR.parent
REPO = STORE_DIR.parent.parent
RAW_DIR = STORE_DIR / "screenshots" / "raw"
OUT_DIR = STORE_DIR / "listing-assets"
FONT_DIR = REPO / "app" / "src" / "main" / "res" / "font"
ICON_SOURCE = REPO / "docs" / "assets" / "brand" / "abastevo-app-icon-1024.png"
BASE_WIDTH = 1080  # card.html is designed at 1080 px wide and scaled from there

CHROME_CANDIDATES = ("google-chrome", "google-chrome-stable", "chromium", "chromium-browser")


def find_chrome(explicit: str | None) -> str:
    if explicit:
        return explicit
    for name in CHROME_CANDIDATES:
        path = shutil.which(name)
        if path:
            return path
    sys.exit("Chrome/Chromium not found; pass --chrome")


def headline_html(text: str) -> str:
    """Escape the caption and turn {words} into highlighted <em> spans."""
    escaped = html.escape(text, quote=False)
    return re.sub(r"\{([^{}]+)\}", r"<em>\1</em>", escaped)


def render(chrome: str, page: Path, out: Path, size: tuple[int, int], work: Path) -> None:
    width, height = size
    shot = work / (out.stem + ".raw.png")
    cmd = [
        chrome,
        "--headless=new",
        "--disable-gpu",
        "--hide-scrollbars",
        "--force-device-scale-factor=1",
        "--default-background-color=00000000",
        f"--user-data-dir={work / 'profile'}",
        f"--window-size={width},{height}",
        f"--screenshot={shot}",
        "--virtual-time-budget=4000",
        page.as_uri(),
    ]
    subprocess.run(cmd, check=True, capture_output=True, timeout=120)
    with Image.open(shot) as img:
        if img.size != (width, height):
            raise SystemExit(f"{out}: rendered {img.size}, expected {(width, height)}")
        rgb = Image.new("RGB", img.size, (5, 11, 34))
        rgb.paste(img.convert("RGBA"), mask=img.convert("RGBA").split()[3])
    out.parent.mkdir(parents=True, exist_ok=True)
    rgb.save(out, optimize=True)
    print(f"wrote {out.relative_to(REPO)} {width}x{height}")


def common_vars() -> dict[str, str]:
    return {
        "font_manrope_extrabold": (FONT_DIR / "manrope_extrabold.ttf").as_uri(),
        "font_manrope_semibold": (FONT_DIR / "manrope_semibold.ttf").as_uri(),
        "icon": ICON_SOURCE.as_uri(),
    }


def build_sets(chrome: str, manifest: dict, only: str | None, work: Path) -> None:
    template = Template((CARDS_DIR / "card.html").read_text(encoding="utf-8"))
    for card_set in manifest["sets"]:
        if only and card_set["name"] != only:
            continue
        width, height = card_set["size"]
        for card in card_set["cards"]:
            shot = RAW_DIR / card["shot"]
            if not shot.is_file():
                raise SystemExit(f"missing screenshot {shot}")
            page = work / f"{card_set['name']}-{card['out']}.html"
            page.write_text(
                template.substitute(
                    common_vars(),
                    width=width,
                    height=height,
                    scale=f"{width / BASE_WIDTH:.6f}",
                    headline=headline_html(card["headline"]),
                    shot=shot.as_uri(),
                ),
                encoding="utf-8",
            )
            render(chrome, page, OUT_DIR / card_set["name"] / card["out"], (width, height), work)


def build_feature(chrome: str, manifest: dict, work: Path) -> None:
    spec = manifest["feature_graphic"]
    width, height = spec["size"]
    page = work / "feature.html"
    template = Template((CARDS_DIR / "feature.html").read_text(encoding="utf-8"))
    page.write_text(
        template.substitute(
            common_vars(),
            width=width,
            height=height,
            title=html.escape(spec["title"]),
            tagline=headline_html(spec["tagline"]),
        ),
        encoding="utf-8",
    )
    render(chrome, page, OUT_DIR / spec["out"], (width, height), work)


def build_icon(manifest: dict) -> None:
    spec = manifest["icon"]
    size = spec["size"]
    with Image.open(REPO / spec["source"]) as img:
        # Play wants a full-bleed 512x512 32-bit PNG; it applies its own mask and shadow.
        icon = img.convert("RGBA").resize((size, size), Image.LANCZOS)
    out = OUT_DIR / spec["out"]
    out.parent.mkdir(parents=True, exist_ok=True)
    icon.save(out, optimize=True)
    print(f"wrote {out.relative_to(REPO)} {size}x{size}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--chrome", help="Chrome/Chromium binary")
    parser.add_argument("--only", help="render a single set (phone, tablet-7in, tablet-10in)")
    args = parser.parse_args()
    chrome = find_chrome(args.chrome)
    manifest = json.loads((CARDS_DIR / "cards.json").read_text(encoding="utf-8"))
    with tempfile.TemporaryDirectory(prefix="abastevo-cards-") as tmp:
        work = Path(tmp)
        build_sets(chrome, manifest, args.only, work)
        if not args.only:
            build_feature(chrome, manifest, work)
            build_icon(manifest)


if __name__ == "__main__":
    main()
