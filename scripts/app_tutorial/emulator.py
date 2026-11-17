#!/usr/bin/env python3
"""Put an Android emulator in the frozen app-tutorial capture conditions.

The conditions come from docs/assets/app-tutorial/manifest.json ("capture").
Only emulator serials are accepted: tutorial captures never use a physical
phone. Commands:

  setup         locale, timezone, demo-mode status bar, 1080x1920@420, Wi-Fi
                only, on-screen keyboard off, screen kept awake
  gps CITY      synthetic location fix (sao-paulo-sp or sorriso-mt)
  shot PATH     screenshot saved as opaque RGB PNG (fails on a black capture,
                which means a FLAG_SECURE screen)
  reset         undo setup (demo mode, display size, keyboard)

Use --dry-run to print the adb commands. `shot` needs Pillow (tooling only).
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / "docs/assets/app-tutorial/manifest.json"
APP_ID = "com.anpfuel.app.debug"
KEYBOARD_PACKAGE = "com.google.android.inputmethod.latin"
DEMO = "am broadcast -a com.android.systemui.demo -e command"


class EmulatorError(RuntimeError):
    pass


def capture_conditions(manifest_path: Path = MANIFEST) -> dict:
    return json.loads(manifest_path.read_text(encoding="utf-8"))["capture"]


def check_serial(serial: str | None) -> str:
    if not serial:
        raise EmulatorError("set --serial or ANDROID_SERIAL to the tutorial emulator (emulator-NNNN)")
    if not serial.startswith("emulator-"):
        raise EmulatorError(f"{serial} is not an emulator; tutorial captures never use a physical device")
    return serial


def setup_commands(capture: dict) -> list[list[str]]:
    size = capture["display"]["wmSize"]
    density = str(capture["display"]["density"])
    locale = capture["locale"]
    shell = [
        "settings put global sysui_demo_allowed 1",
        f"{DEMO} enter",
        f"{DEMO} clock -e hhmm 1000",
        f"{DEMO} battery -e level 100 -e plugged false",
        f"{DEMO} network -e wifi show -e level 4 -e fully true",
        f"{DEMO} network -e mobile hide",
        f"{DEMO} notifications -e visible false",
        f"{DEMO} status -e volume hide -e bluetooth hide -e location hide -e alarm hide -e sync hide -e mute hide",
        f"wm size {size}",
        f"wm density {density}",
        f"cmd locale set-app-locales {APP_ID} --locales {locale['app']}",
        "cmd uimode night no",
        "svc power stayon true",
        "settings put system screen_off_timeout 1800000",
        "cmd wifi connect-network AndroidWifi open",
        "svc data disable",
        "settings put secure stylus_handwriting_enabled 0",
        "settings put secure show_ime_with_hard_keyboard 0",
        f"pm disable-user --user 0 {KEYBOARD_PACKAGE}",
    ]
    commands = [["shell", line] for line in shell]
    # System locale and timezone need root (available on google_apis images).
    commands.append(["root"])
    commands.append(["wait-for-device"])
    commands.append(["shell", f"setprop persist.sys.locale {locale['system']}; "
                              f"setprop persist.sys.timezone {locale['timezone']}"])
    return commands


def reset_commands() -> list[list[str]]:
    return [["shell", line] for line in (
        f"{DEMO} exit",
        "wm size reset",
        "wm density reset",
        f"pm enable {KEYBOARD_PACKAGE}",
        "settings put secure show_ime_with_hard_keyboard 1",
        "svc data enable",
    )]


def gps_command(capture: dict, city: str) -> list[str]:
    try:
        lat, lon = capture["syntheticLocation"][city]
    except KeyError as exc:
        raise EmulatorError(f"unknown city {city}; use one of {sorted(capture['syntheticLocation'])}") from exc
    return ["emu", "geo", "fix", str(lon), str(lat)]


def run(serial: str, args: list[str], dry_run: bool, binary: bool = False):
    cmd = ["adb", "-s", serial, *args]
    if dry_run:
        print(" ".join(cmd))
        return b"" if binary else ""
    result = subprocess.run(cmd, capture_output=True, check=False)
    if result.returncode != 0:
        raise EmulatorError(f"{' '.join(cmd)} failed: {result.stderr.decode(errors='replace').strip()}")
    return result.stdout if binary else result.stdout.decode(errors="replace")


def save_shot(png_bytes: bytes, out: Path) -> tuple[int, int]:
    from io import BytesIO

    from PIL import Image  # tooling-only dependency, as in docs/store/cards

    image = Image.open(BytesIO(png_bytes)).convert("RGB")
    width, height = image.size
    content = image.crop((0, 80, width, height - 80)).convert("L").getextrema()
    if content[1] < 16:
        raise EmulatorError("capture is black: the screen is FLAG_SECURE and must not be captured")
    out.parent.mkdir(parents=True, exist_ok=True)
    image.save(out, "PNG", optimize=True)
    return image.size


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--serial", default=os.environ.get("ANDROID_SERIAL"))
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--manifest", type=Path, default=MANIFEST)
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("setup")
    sub.add_parser("reset")
    p_gps = sub.add_parser("gps")
    p_gps.add_argument("city")
    p_shot = sub.add_parser("shot")
    p_shot.add_argument("path", type=Path)
    args = parser.parse_args(argv)
    try:
        serial = check_serial(args.serial)
        capture = capture_conditions(args.manifest)
        if args.command == "setup":
            for command in setup_commands(capture):
                run(serial, command, args.dry_run)
            print("setup done; restart the app so the locale applies")
        elif args.command == "reset":
            for command in reset_commands():
                run(serial, command, args.dry_run)
        elif args.command == "gps":
            run(serial, gps_command(capture, args.city), args.dry_run)
        elif args.command == "shot":
            data = run(serial, ["exec-out", "screencap", "-p"], args.dry_run, binary=True)
            if not args.dry_run:
                width, height = save_shot(data, args.path)
                print(f"{args.path} {width}x{height}")
        return 0
    except EmulatorError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
