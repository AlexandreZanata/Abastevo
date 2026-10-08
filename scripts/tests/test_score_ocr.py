import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("score_ocr", Path(__file__).parents[1] / "score-ocr.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class OcrScoreTest(unittest.TestCase):
    def setUp(self):
        self.manifest = {"samples": [{"file": "sample.jpeg", "sha256": "expected", "tags": ["test"],
            "expected_rows": [{"fuel": "ETHANOL", "milli_brl": 4321}], "manual_prices": [7400],
            "conditional_required": True}]}
        self.record = {"file": "sample.jpeg", "status": "recognized", "elapsed_ms": 123,
            "rows": [{"fuel": "ETHANOL", "milli_brl": 4321}], "conditional": True,
            "orphans": [7400], "tokens": [{"text": "PRIVATE_SENTINEL", "box": [1, 2, 3, 4]}]}

    def test_exact_pair_and_manual_condition_are_scored_without_private_tokens(self):
        result = module.score(self.manifest, [self.record])
        self.assertEqual((1, 0, 0, 1, 0), tuple(result["summary"][k] for k in
            ["exact", "wrong", "missed", "manual_recovered", "condition_missed"]))
        self.assertNotIn("PRIVATE_SENTINEL", str(result))

    def test_one_milli_or_wrong_product_counts_as_wrong_and_missed(self):
        for row in [{"fuel": "ETHANOL", "milli_brl": 4320}, {"fuel": "DIESEL_S10", "milli_brl": 4321}]:
            self.record["rows"] = [row]
            summary = module.score(self.manifest, [self.record])["summary"]
            self.assertEqual((0, 1, 1), tuple(summary[k] for k in ["exact", "wrong", "missed"]))

    def test_missing_condition_and_no_rows_are_explicit_misses(self):
        self.record.update(rows=[], conditional=False, orphans=[])
        summary = module.score(self.manifest, [self.record])["summary"]
        self.assertEqual((1, 1, 0), tuple(summary[k] for k in ["missed", "condition_missed", "manual_recovered"]))

    def test_missing_extra_duplicate_and_wrong_device_hash_refuse(self):
        for records in [[], [self.record, self.record], [dict(self.record, file="extra.jpeg")],
                        [dict(self.record, input_sha256="wrong")]]:
            with self.assertRaises(ValueError):
                module.score(self.manifest, records)

    def test_hash_required_and_explicit_acceptance_budget_cannot_pass_missing_or_wrong_results(self):
        with self.assertRaises(ValueError):
            module.score(self.manifest, [self.record], require_device_hash=True)
        result = module.score(self.manifest, [self.record])
        module.check_budget(result, 1, 0)
        with self.assertRaises(ValueError):
            module.check_budget(result, 2, 0)
        self.record["rows"][0]["milli_brl"] = 4320
        with self.assertRaises(ValueError):
            module.check_budget(module.score(self.manifest, [self.record]), 0, 0)


if __name__ == "__main__":
    unittest.main()
