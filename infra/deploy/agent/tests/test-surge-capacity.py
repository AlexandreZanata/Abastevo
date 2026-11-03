#!/usr/bin/env python3
"""Capacity refusal matrix over the actual reviewed API pod."""
import copy
import json
from pathlib import Path
import runpy
import unittest

ROOT = Path(__file__).resolve().parents[1]
check = runpy.run_path(str(ROOT / "surge-capacity.py"))["check"]

class Capacity(unittest.TestCase):
    def setUp(self):
        self.deployment = json.loads((ROOT.parents[1] / "k8s/staging/api-deployment.yaml").read_text())
        self.quota = {"status": {
            "hard": {"limits.cpu":"6","limits.memory":"8Gi","requests.cpu":"2","requests.memory":"4Gi"},
            "used": {"limits.cpu":"3500m","limits.memory":"3840Mi","requests.cpu":"400m","requests.memory":"1088Mi"}}}

    def test_current_namespace_can_fit_independent_api_pod(self):
        self.assertEqual(len(check(self.deployment, self.quota)), 4)

    def test_every_resource_dimension_refuses_independently(self):
        for key in self.quota["status"]["hard"]:
            with self.subTest(resource=key):
                quota = copy.deepcopy(self.quota)
                quota["status"]["used"][key] = quota["status"]["hard"][key]
                with self.assertRaises(ValueError):
                    check(self.deployment, quota)

    def test_missing_unknown_and_shared_writer_fail_closed(self):
        pod = self.deployment["spec"]["template"]["spec"]
        pod["containers"].append({"name":"private-storage"})
        with self.assertRaises(ValueError): check(self.deployment, self.quota)
        pod["containers"].pop()
        self.quota["status"]["used"]["limits.memory"] = "unknown"
        with self.assertRaises(ValueError): check(self.deployment, self.quota)
        del self.quota["status"]["used"]["limits.memory"]
        with self.assertRaises(KeyError): check(self.deployment, self.quota)

if __name__ == "__main__":
    unittest.main()
