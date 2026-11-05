#!/usr/bin/env python3
"""Structural staging safety and render regressions; stdlib only."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import re
import os

ROOT = Path(__file__).resolve().parent

def obj(name):
    return json.loads((ROOT / name).read_text())

class StagingManifests(unittest.TestCase):
    def test_registry_render_requires_offline_source_root(self):
        path=ROOT/"registry-sync-cronjob.yaml"
        keys=set(re.findall(r"\$\{([A-Z0-9_]+)\}",path.read_text()))
        with tempfile.TemporaryDirectory() as directory:
            env=Path(directory)/"render.env"
            env.write_text("".join(k+"=/owned/fixture\n" for k in keys if k!="REGISTRY_RFB_SOURCE_PATH"))
            result=subprocess.run(["python3",str(ROOT/"render.py"),str(path)],env={**os.environ,"STAGING_RENDER_ENV":str(env)},text=True,capture_output=True)
            self.assertNotEqual(result.returncode,0)
            self.assertIn("REGISTRY_RFB_SOURCE_PATH",result.stderr)
            self.assertEqual(result.stdout,"")

    def test_storage_is_independent_single_writer(self):
        api = obj("api-deployment.yaml")["spec"]["template"]["spec"]
        self.assertEqual({c["name"] for c in api["containers"]}, {"api", "worker"})
        self.assertNotIn("dev-private-media", {v["name"] for v in api["volumes"]})
        storage = obj("storage-deployment.yaml")
        self.assertEqual(storage["spec"]["replicas"], 1)
        self.assertEqual(storage["spec"]["strategy"], {"type": "Recreate"})
        self.assertEqual(storage["metadata"]["namespace"], "abastevo-temp")
        service = obj("storage-service.yaml")
        self.assertEqual(service["spec"]["type"], "ClusterIP")
        self.assertEqual(service["spec"]["selector"], storage["spec"]["selector"]["matchLabels"])
        self.assertEqual(service["spec"]["ports"][0]["port"], 9000)
        self.assertNotIn("nodePort", service["spec"]["ports"][0])
        env = {e["name"]:e.get("value") for e in api["containers"][0]["env"]}
        self.assertEqual(env["ANPFUEL_DEV_STORAGE_SERVICE"], "true")

    def test_rollout_keeps_serving_replica(self):
        spec=obj("api-deployment.yaml")["spec"]
        self.assertEqual(spec["strategy"]["rollingUpdate"],{"maxSurge":1,"maxUnavailable":0})
        self.assertEqual(spec["minReadySeconds"],10)
        self.assertGreaterEqual(spec["revisionHistoryLimit"],3)
        pod=spec["template"]["spec"]
        self.assertGreater(pod["terminationGracePeriodSeconds"],30+10+5)
        api=pod["containers"][0]
        self.assertEqual(api["startupProbe"]["httpGet"]["path"],"/health/live")
        self.assertEqual(api["readinessProbe"]["periodSeconds"],3)
        self.assertEqual(api["readinessProbe"]["failureThreshold"],2)
        for container in pod["containers"]:
            self.assertEqual(container["imagePullPolicy"],"IfNotPresent")

    def test_render_keeps_shell_variables(self):
        paths=[ROOT/n for n in ("api-deployment.yaml","storage-deployment.yaml","storage-configmap.yaml","storage-service.yaml")]
        keys=set()
        for path in paths:
            keys.update(re.findall(r"\$\{([A-Z0-9_]+)\}", path.read_text()))
        with tempfile.TemporaryDirectory() as directory:
            env=Path(directory)/"render.env"
            env.write_text("".join(k+"=fixture-value\n" for k in keys))
            output=subprocess.check_output(["python3",str(ROOT/"render.py"),*map(str,paths)],env={**os.environ,"STAGING_RENDER_ENV":str(env)},text=True)
            documents=[json.loads(x) for x in output.split("\n---\n")]
            self.assertEqual(len(documents),4)
            script=documents[2]["data"]["start.sh"]
            self.assertIn('$MINIO_ROOT_USER',script)
            self.assertNotIn("${PRIVATE_MEDIA_PATH}",output)

if __name__ == "__main__":
    unittest.main()
