"""Tests for the app tutorial screenshot contract and emulator helper."""
from __future__ import annotations

import contextlib
import hashlib
import io
import json
import struct
import subprocess
import sys
import tempfile
import unittest
import zlib
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import emulator  # noqa: E402
import tutorial_contract as tc  # noqa: E402

SCREEN = "app/src/main/kotlin/com/anpfuel/app/ui/demo/DemoScreen.kt"
OTHER = "app/src/main/kotlin/com/anpfuel/app/ui/demo/Other.kt"
IMAGE = "demo/01-route-button.png"


def png(width: int, height: int, colour_type: int = 2, fill: bytes = b"\x10") -> bytes:
    channels = {2: 3, 6: 4}[colour_type]
    raw = b"".join(b"\x00" + fill * (width * channels) for _ in range(height))

    def chunk(kind: bytes, data: bytes) -> bytes:
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

    header = struct.pack(">IIBBBBB", width, height, 8, colour_type, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def strings_xml(values: dict[str, str]) -> str:
    body = "".join(f'    <string name="{k}">{v}</string>\n' for k, v in values.items())
    return f"<resources>\n{body}</resources>\n"


class Fixture:
    """A tiny repository with one captured step and a landing page using it."""

    def __init__(self, tmp: Path):
        self.root = tmp
        self.write(SCREEN, "Text(stringResource(R.string.demo_route))\nIcon(painterResource(R.drawable.ic_demo))\n")
        self.write(OTHER, "Text(stringResource(R.string.demo_other))\n")
        self.write("app/src/main/res/values/strings.xml",
                   strings_xml({"demo_route": "Route and go", "demo_other": "Other"}))
        self.write("app/src/main/res/values-pt-rBR/strings.xml",
                   strings_xml({"demo_route": "Rota e ir", "demo_other": "Outro"}))
        self.write("app/src/main/res/drawable/ic_demo.xml", "<vector/>\n")
        self.write("landing/content/guias/demo.md",
                   "# Guia\n\nToque em **Rota e ir** para traçar a rota.\n\n"
                   "![Rota](/assets/app/demo-01-route-button-720w.webp)\n")
        self.write_bytes(f"docs/assets/app-tutorial/{IMAGE}", png(1080, 1920))
        self.git("init", "-q")
        self.git("config", "user.email", "tests@example.invalid")
        self.git("config", "user.name", "tests")
        self.commit("app")
        self.capture_commit = self.git("rev-parse", "HEAD").strip()
        step = {
            "file": "01-route-button.png", "caption": "Tap Rota e ir.", "strings": ["demo_route"],
            "extraSources": [], "sources": [SCREEN], "context": {"city": "sorriso-mt"},
            "sha256": hashlib.sha256(self.read_bytes(f"docs/assets/app-tutorial/{IMAGE}")).hexdigest(),
        }
        self.manifest = {
            "schemaVersion": 1,
            "capture": {"appSourceCommit": self.capture_commit},
            "features": [{"id": "demo", "title": "Demo", "summary": "Demo feature.",
                          "landingTargets": ["/como-usar/demo/"], "steps": [step]}],
            "textOnlySteps": [],
        }
        self.save_manifest()
        self.write("docs/assets/app-tutorial/README.md",
                   "# Tutorial\n\n" + tc.render_steps(self.manifest) + "\n")
        self.commit("tutorial")

    # helpers
    def path(self, rel: str) -> Path:
        return self.root / rel

    def write(self, rel: str, text: str) -> None:
        self.path(rel).parent.mkdir(parents=True, exist_ok=True)
        self.path(rel).write_text(text, encoding="utf-8")

    def write_bytes(self, rel: str, data: bytes) -> None:
        self.path(rel).parent.mkdir(parents=True, exist_ok=True)
        self.path(rel).write_bytes(data)

    def read_bytes(self, rel: str) -> bytes:
        return self.path(rel).read_bytes()

    def git(self, *args: str) -> str:
        return subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True, text=True).stdout

    def commit(self, message: str) -> None:
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)

    def save_manifest(self) -> None:
        self.write(tc.MANIFEST_REL, json.dumps(self.manifest, indent=2) + "\n")

    def load(self) -> dict:
        return tc.load_manifest(self.root)

    def cli(self, *args: str) -> tuple[int, str]:
        out = io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
            code = tc.main(["--root", str(self.root), *args])
        return code, out.getvalue()


class ContractTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.fx = Fixture(Path(self._tmp.name))

    def tearDown(self):
        self._tmp.cleanup()

    def errors(self) -> list[str]:
        return tc.validate(self.fx.root, self.fx.load())

    # validate -------------------------------------------------------------

    def test_valid_fixture_passes(self):
        self.assertEqual(self.errors(), [])
        code, out = self.fx.cli("validate")
        self.assertEqual(code, 0, out)

    def test_unlisted_png_fails(self):
        self.fx.write_bytes("docs/assets/app-tutorial/demo/02-extra.png", png(1080, 1920))
        self.assertTrue(any("not listed in manifest.json" in e for e in self.errors()))

    def test_recaptured_image_without_refresh_fails(self):
        self.fx.write_bytes(f"docs/assets/app-tutorial/{IMAGE}", png(1080, 1920, fill=b"\x20"))
        self.assertTrue(any("sha256 mismatch" in e for e in self.errors()))

    def test_wrong_size_and_alpha_fail(self):
        self.fx.write_bytes(f"docs/assets/app-tutorial/{IMAGE}", png(1080, 2400, colour_type=6))
        errors = self.errors()
        self.assertTrue(any("1080x2400" in e for e in errors), errors)
        self.assertTrue(any("opaque RGB" in e for e in errors), errors)

    def test_numbering_gap_fails(self):
        manifest = self.fx.load()
        manifest["features"][0]["steps"][0]["file"] = "02-route-button.png"
        errors = tc.validate(self.fx.root, manifest)
        self.assertTrue(any("without gaps" in e for e in errors), errors)

    def test_unknown_string_and_stale_sources_fail(self):
        manifest = self.fx.load()
        manifest["features"][0]["steps"][0]["strings"] = ["missing_key"]
        errors = tc.validate(self.fx.root, manifest)
        self.assertTrue(any("unknown string resource missing_key" in e for e in errors), errors)
        self.assertTrue(any("sources out of sync" in e for e in errors), errors)

    def test_readme_must_match_manifest(self):
        manifest = self.fx.load()
        manifest["features"][0]["steps"][0]["caption"] = "New caption."
        self.fx.manifest = manifest
        self.fx.save_manifest()
        self.assertTrue(any("README.md generated steps are out of date" in e for e in self.errors()))
        code, _ = self.fx.cli("render-readme", "--write")
        self.assertEqual(code, 0)
        self.assertEqual(self.errors(), [])

    def test_sync_sources_rewrites_manifest(self):
        manifest = self.fx.load()
        manifest["features"][0]["steps"][0]["sources"] = []
        self.fx.manifest = manifest
        self.fx.save_manifest()
        code, _ = self.fx.cli("sync-sources")
        self.assertEqual(code, 1)
        code, _ = self.fx.cli("sync-sources", "--write")
        self.assertEqual(code, 0)
        self.assertEqual(self.fx.load()["features"][0]["steps"][0]["sources"], [SCREEN])

    # stale ----------------------------------------------------------------

    def stale(self, **kwargs) -> dict:
        return tc.stale(self.fx.root, self.fx.load(), kwargs.get("base"), kwargs.get("head"))

    def test_no_app_change_is_fresh(self):
        self.assertEqual(self.stale()["affected"], [])

    def test_unrelated_source_change_is_fresh(self):
        self.fx.write(OTHER, "Text(stringResource(R.string.demo_other))\n// tweak\n")
        self.fx.commit("other")
        self.assertEqual(self.stale(head="HEAD")["affected"], [])

    def test_screen_source_change_is_stale(self):
        self.fx.write(SCREEN, self.fx.path(SCREEN).read_text() + "Spacer()\n")
        self.assertEqual(self.stale()["affected"][0]["reasons"], [f"source changed: {SCREEN}"])

    def test_label_change_points_at_landing_image_and_text(self):
        self.fx.write("app/src/main/res/values-pt-rBR/strings.xml",
                      strings_xml({"demo_route": "Traçar rota", "demo_other": "Outro"}))
        self.fx.commit("rename label")
        affected = self.stale(head="HEAD")["affected"]
        self.assertEqual(len(affected), 1)
        item = affected[0]
        self.assertEqual(item["image"], IMAGE)
        self.assertIn("string demo_route changed (values-pt-rBR)", item["reasons"])
        self.assertEqual(item["landingImageUses"], ["landing/content/guias/demo.md"])
        self.assertEqual(item["landingQuotes"][0]["line"], 3)
        self.assertEqual(item["landingQuotes"][0]["after"], "Traçar rota")

    def test_drawable_change_is_stale(self):
        self.fx.write("app/src/main/res/drawable/ic_demo.xml", "<vector android:width='24dp'/>\n")
        reasons = self.stale()["affected"][0]["reasons"]
        self.assertEqual(reasons, [f"drawable used by {SCREEN} changed"])

    def test_strict_cli_fails_and_report_mode_does_not(self):
        self.fx.write(SCREEN, self.fx.path(SCREEN).read_text() + "Spacer()\n")
        code, out = self.fx.cli("stale", "--strict")
        self.assertEqual(code, 1, out)
        self.assertIn("RECAPTURE: 1 step(s) are stale", out)
        code, _ = self.fx.cli("stale", "--report-only")
        self.assertEqual(code, 0)

    def test_refresh_rebaselines_the_step(self):
        self.fx.write(SCREEN, self.fx.path(SCREEN).read_text() + "Spacer()\n")
        self.fx.write_bytes(f"docs/assets/app-tutorial/{IMAGE}", png(1080, 1920, fill=b"\x30"))
        self.fx.commit("ui change and recapture")
        code, out = self.fx.cli("refresh", IMAGE)
        self.assertEqual(code, 0, out)
        manifest = self.fx.load()
        step = manifest["features"][0]["steps"][0]
        self.assertEqual(step["appSourceCommit"], self.fx.git("rev-parse", "HEAD").strip())
        self.assertEqual(tc.stale(self.fx.root, manifest, None, "HEAD")["affected"], [])
        self.assertEqual(tc.validate(self.fx.root, manifest), [])

    def test_missing_history_is_reported(self):
        manifest = self.fx.load()
        manifest["capture"]["appSourceCommit"] = "f" * 40
        self.fx.manifest = manifest
        self.fx.save_manifest()
        code, out = self.fx.cli("stale")
        self.assertEqual(code, 2)
        self.assertIn("not in this clone", out)
        code, out = self.fx.cli("stale", "--report-only")
        self.assertEqual(code, 0)
        self.assertIn("SKIP", out)


class EmulatorHelperTest(unittest.TestCase):
    def test_physical_devices_are_refused(self):
        with self.assertRaises(emulator.EmulatorError):
            emulator.check_serial("VKCQGAUCGYAMR4W4")
        with self.assertRaises(emulator.EmulatorError):
            emulator.check_serial(None)
        self.assertEqual(emulator.check_serial("emulator-5590"), "emulator-5590")

    def test_setup_applies_manifest_conditions(self):
        capture = emulator.capture_conditions()
        flat = [" ".join(c) for c in emulator.setup_commands(capture)]
        self.assertIn(f"shell wm size {capture['display']['wmSize']}", flat)
        self.assertIn(f"shell wm density {capture['display']['density']}", flat)
        self.assertTrue(any("clock -e hhmm 1000" in c for c in flat))
        self.assertTrue(any(f"--locales {capture['locale']['app']}" in c for c in flat))
        self.assertTrue(any(f"persist.sys.timezone {capture['locale']['timezone']}" in c for c in flat))

    def test_gps_uses_synthetic_city_fix(self):
        capture = emulator.capture_conditions()
        lat, lon = capture["syntheticLocation"]["sorriso-mt"]
        self.assertEqual(emulator.gps_command(capture, "sorriso-mt"), ["emu", "geo", "fix", str(lon), str(lat)])
        with self.assertRaises(emulator.EmulatorError):
            emulator.gps_command(capture, "nowhere")

    def test_cli_dry_run_and_refusal(self):
        out = io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
            self.assertEqual(emulator.main(["--serial", "emulator-5590", "--dry-run", "reset"]), 0)
            self.assertEqual(emulator.main(["--serial", "R58M123", "--dry-run", "setup"]), 1)
        self.assertIn("adb -s emulator-5590 shell wm size reset", out.getvalue())
        self.assertIn("never use a physical device", out.getvalue())


class RealManifestTest(unittest.TestCase):
    def test_repository_manifest_is_valid(self):
        errors = tc.validate(tc.ROOT, tc.load_manifest(tc.ROOT))
        self.assertEqual(errors, [])


if __name__ == "__main__":
    unittest.main()
