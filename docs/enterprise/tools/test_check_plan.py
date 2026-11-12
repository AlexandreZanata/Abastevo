import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location(
    "check_plan", Path(__file__).with_name("check_plan.py")
)
check_plan = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check_plan)


class ScreenIndexTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.root_patch = patch.object(check_plan, "ROOT", self.root)
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)
        (self.root / "modules").mkdir()
        (self.root / "modules/example.md").write_text(
            "| SCR-EXAMPLE-01 | APP, PANEL | Read an example | Member |\n"
        )
        for name, surfaces in (
            ("12-ux-app.md", ("APP",)),
            ("13-ux-web.md", ("PANEL", "PORTAL", "ADMIN")),
        ):
            content = "\n".join(
                f"<!-- SCREEN-INDEX:{surface}:BEGIN -->\n"
                f"<!-- SCREEN-INDEX:{surface}:END -->"
                for surface in surfaces
            )
            (self.root / name).write_text(content + "\n")

    def test_generation_is_repeatable_and_preserves_canonical_source(self):
        errors = []
        _, _, screens = check_plan.collect_ids(errors)
        self.assertEqual(errors, [])
        check_plan.write_screen_indexes(screens)
        first = {
            name: (self.root / name).read_text()
            for name in ("12-ux-app.md", "13-ux-web.md")
        }

        errors = []
        definitions, _, screens = check_plan.collect_ids(errors)
        self.assertEqual(errors, [])
        self.assertEqual(
            definitions["SCR"]["SCR-EXAMPLE-01"][0], "modules/example.md"
        )
        self.assertEqual(screens["SCR-EXAMPLE-01"][2], "modules/example.md")
        check_plan.write_screen_indexes(screens)
        for name, content in first.items():
            self.assertEqual((self.root / name).read_text(), content)

    def test_generated_references_still_require_definitions(self):
        (self.root / "12-ux-app.md").write_text(
            "<!-- SCREEN-INDEX:APP:BEGIN -->\n"
            "| SCR-EXAMPLE-02 | Missing source | [source](modules/example.md) |\n"
            "<!-- SCREEN-INDEX:APP:END -->\n"
        )
        errors = []
        definitions, references, _ = check_plan.collect_ids(errors)
        check_plan.check_refs(definitions, references, {}, errors)
        self.assertTrue(any("SCR-EXAMPLE-02" in error for error in errors))

    def test_duplicate_canonical_definitions_are_still_rejected(self):
        (self.root / "modules/duplicate.md").write_text(
            "| SCR-EXAMPLE-01 | APP | Duplicate | Member |\n"
        )
        errors = []
        check_plan.collect_ids(errors)
        self.assertTrue(any("SCR-EXAMPLE-01 defined in" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
