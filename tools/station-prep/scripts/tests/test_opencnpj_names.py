import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("opencnpj_names", Path(__file__).parents[1] / "export_opencnpj_names.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class OpenCNPJNamesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.snapshot = self.root / "snapshot.ocnpj"
        self.snapshot.write_bytes(b"synthetic snapshot")
        self.registry = self.root / "registry.csv"
        self.registry.write_text("CNPJ;RAZAOSOCIAL\n00123456000100;Synthetic Legal\n00123456000290;Synthetic Branch\n")
        self.rows = [{"cnpj": "00123456000100", "nome_fantasia": "Synthetic Trade", "situacao_cadastral": "02",
                      "correio_eletronico": "PRIVATE", "socios": [{"nome": "PRIVATE"}]},
                     {"cnpj": "00123456000290", "nome_fantasia": "Old Branch", "situacao_cadastral": "08"}]

    def run_source(self, command):
        if command[1] == "info":
            return b'{"competence":"2026-10","source_snapshot":"synthetic"}'
        self.assertIn("cnpj=00123456000100,00123456000290", command)
        return b"\n".join(json.dumps(row).encode() for row in self.rows)

    def export(self, **kwargs):
        return module.export_names("opencnpj", self.snapshot, kwargs.get("sha", module.digest(self.snapshot)),
                                   self.registry, self.root / "output", run=self.run_source)

    def test_exact_branches_minimized_and_hash_bound(self):
        result = self.export()
        self.assertEqual((result["matched"], result["active"]), (2, 1))
        output = self.root / "output"
        data = (output / "rfb-names.jsonl").read_text()
        self.assertNotIn("PRIVATE", data)
        self.assertNotIn("socios", data)
        rows = [json.loads(line) for line in data.splitlines()]
        self.assertEqual(rows[0]["cnpj"], "00123456000100")
        self.assertFalse(rows[1]["active"])
        self.assertEqual(set(rows[0]), {"schema_version", "cnpj", "trade_name", "active", "observed_at"})
        pin = json.loads((output / "manifest.json").read_text())["inputs"][0]
        self.assertEqual(pin["sha256"], module.digest(output / pin["file"]))
        self.assertEqual((output.stat().st_mode & 0o777), 0o700)

    def test_wrong_snapshot_never_calls_export(self):
        with self.assertRaisesRegex(ValueError, "checksum"):
            self.export(sha="0" * 64)
        self.assertFalse((self.root / "output").exists())

    def test_missing_establishment_is_not_fabricated(self):
        self.rows.pop()
        self.assertEqual(self.export()["not_found"], 1)

    def test_duplicate_or_wrong_branch_fails_without_partial_output(self):
        for replacement in [dict(self.rows[0]), dict(self.rows[1], cnpj="00123456000370")]:
            self.rows[1] = replacement
            with self.assertRaisesRegex(ValueError, "CNPJ"):
                self.export()
            self.assertFalse((self.root / "output").exists())

    def test_unknown_status_and_nontext_name_are_refused(self):
        for key, value in [("situacao_cadastral", ""), ("nome_fantasia", {"unsafe": True})]:
            prior = self.rows[0][key]
            self.rows[0][key] = value
            with self.assertRaises(ValueError):
                self.export()
            self.rows[0][key] = prior

    def test_invalid_snapshot_metadata_is_refused(self):
        for metadata in ([], {"competence": None}, {"competence": "0000-01"}):
            with self.assertRaises(ValueError):
                module.export_names("opencnpj", self.snapshot, module.digest(self.snapshot),
                                    self.registry, self.root / "output",
                                    run=lambda command: json.dumps(metadata).encode())
            self.assertFalse((self.root / "output").exists())


if __name__ == "__main__":
    unittest.main()
