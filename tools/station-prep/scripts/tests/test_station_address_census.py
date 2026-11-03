import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from audit_station_address_census import AuditError, POLICY, audit, sha


class StationAddressCensusTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.baseline = Path(directory.name) / "baseline"
        self.enriched = Path(directory.name) / "enriched"
        self.baseline.mkdir()
        self.enriched.mkdir()

    def generation(self, root, source, *, wrong_target=False, changed_name=False, wrong_count=False):
        current_checksum = sha(b"[P25-TEST] current " + source.encode())
        row = {
            "schema_version": "station-assertion-v4", "source": "registry-13col",
            "source_key": "91000000000066", "uf": "SP", "latitude": None,
            "longitude": None, "location_quality": "unknown", "supersedes": None,
            "checksum": current_checksum, "legal_name": "[P25-TEST] Legal LTDA",
            "address_facts": {"source": source, "components": {}, "provenance": None},
        }
        if changed_name:
            row["legal_name"] = "[P25-TEST] Different LTDA"
        history = dict(row, checksum=sha(b"[P25-TEST] historical"),
                       supersedes=sha(b"[P25-TEST] missing") if wrong_target else current_checksum)
        assertions = b"".join(json.dumps(v).encode() + b"\n" for v in (row, history, row))
        (root / "assertions.jsonl").write_bytes(assertions)
        transition = {"before": {"municipality": 1}, "after": {source: 1}}
        manifest = {
            "format_version": "station-batch-v1", "parser_version": "station-prep-v0.3.1+addresses-v1",
            "policy_version": "station-policy-v1", "address_policy_sha256": sha(POLICY.read_bytes()),
            "inputs": [{"key": "registry-13col", "sha256": sha(b"[P25-TEST] source")}],
            "counts": {"registry-13col": {"input": 3, "accepted": 2, "duplicates": 1, "quarantined": 0}},
            "address_inputs": [] if source == "municipality" else [{"source": "rfb"}],
            "address_skips": [],
            "address_coverage": {"station_count": 2 if wrong_count else 1,
                                 "national": transition, "by_uf": {"SP": transition}},
            "outputs": [{"path": "assertions.jsonl", "bytes": len(assertions),
                         "rows": 3, "sha256": sha(assertions)}],
        }
        raw = json.dumps(manifest).encode()
        (root / "manifest.json").write_bytes(raw)
        return sha(raw)

    def test_unique_current_station_and_bound_recount(self):
        b = self.generation(self.baseline, "municipality")
        a = self.generation(self.enriched, "rfb")
        result = audit(self.baseline, b, self.enriched, a)
        self.assertEqual(result["station_count"], 1)
        self.assertEqual(result["by_uf"]["SP"], {"before": {"municipality": 1}, "after": {"rfb": 1}})

    def test_rejects_hiding_history_behind_nonexistent_current_identity(self):
        b = self.generation(self.baseline, "municipality")
        a = self.generation(self.enriched, "rfb", wrong_target=True)
        with self.assertRaisesRegex(AuditError, "historical"):
            audit(self.baseline, b, self.enriched, a)

    def test_rejects_changed_facts_or_inconsistent_coverage(self):
        b = self.generation(self.baseline, "municipality")
        for args, reason in (({"changed_name": True}, "original station facts"),
                             ({"wrong_count": True}, "disagrees with recount")):
            with self.subTest(args=args):
                a = self.generation(self.enriched, "rfb", **args)
                with self.assertRaisesRegex(AuditError, reason):
                    audit(self.baseline, b, self.enriched, a)

    def test_rejects_manifest_and_stream_tampering(self):
        b = self.generation(self.baseline, "municipality")
        a = self.generation(self.enriched, "rfb")
        with self.assertRaisesRegex(AuditError, "manifest hash"):
            audit(self.baseline, b, self.enriched, "0" * 64)
        path = self.enriched / "assertions.jsonl"
        path.write_bytes(path.read_bytes().replace(b"Legal LTDA", b"Other LTDA"))
        with self.assertRaisesRegex(AuditError, "stream fingerprint"):
            audit(self.baseline, b, self.enriched, a)


if __name__ == "__main__":
    unittest.main()
