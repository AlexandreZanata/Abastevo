#!/usr/bin/env python3
"""Synthetic exact-head/pagination/check-state contract; no GitHub mutations."""
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("merge_checks", Path(__file__).parents[1] / "validate-merge-checks.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
HEAD = "a" * 40


def check(name="Quick verification", status="completed", conclusion="success", head=HEAD):
    return dict(name=name, status=status, conclusion=conclusion, head_sha=head)


def page(*rows, total=None):
    value = {"check_runs": list(rows)}
    if total is not None:
        value["total_count"] = total
    return json.dumps(value, indent=2)


class MergeCheckTest(unittest.TestCase):
    def test_all_enabled_failures_and_pending_states_refuse(self):
        for status, result in [("completed", x) for x in ("failure", "timed_out", "cancelled", "action_required", "stale", "startup_failure", None)] + [(x, None) for x in ("queued", "in_progress", "waiting", "unknown")]:
            with self.subTest(status=status, result=result), self.assertRaises(module.CheckError):
                module.validate(page(check(), check("integration", status, result)), HEAD, "Quick verification")

    def test_required_skipped_and_neutral_refuse(self):
        for result in ("skipped", "neutral"):
            with self.subTest(result=result), self.assertRaises(module.CheckError):
                module.validate(page(check(conclusion=result)), HEAD, "Quick verification")

    def test_nonrequired_path_skips_and_neutral_do_not_claim_success(self):
        self.assertEqual(3, module.validate(page(check(), check("other", conclusion="skipped"), check("annotation", conclusion="neutral")), HEAD, "Quick verification"))

    def test_later_pages_cannot_hide_a_failure(self):
        raw = page(check(), total=2) + "\n" + page(check("integration", conclusion="failure"), total=2)
        with self.assertRaises(module.CheckError):
            module.validate(raw, HEAD, "Quick verification")

    def test_complete_paginated_success(self):
        raw = page(check(), total=2) + "\n" + page(check("integration"), total=2)
        self.assertEqual(2, module.validate(raw, HEAD, " Quick verification, integration "))

    def test_missing_truncated_corrupt_or_foreign_head_refuse(self):
        for raw in ("", "bad", "[]", "{}", page(), page(check(), total=2), page(check(head="b" * 40)), page(check()) + "garbage", page({"name": "Quick verification"})):
            with self.subTest(raw=raw), self.assertRaises(module.CheckError):
                module.validate(raw, HEAD, "Quick verification")

    def test_names_are_literal_and_empty_required_list_refuses(self):
        self.assertEqual(1, module.validate(page(check("Quick verification [mobile]")), HEAD, "Quick verification [mobile]"))
        for required in (" , ", "fast"):
            with self.subTest(required=required), self.assertRaises(module.CheckError):
                module.validate(page(check()), HEAD, required)


if __name__ == "__main__":
    unittest.main()
