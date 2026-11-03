#!/usr/bin/env python3
"""Real HTTP/process tests of the rollout proof, with no VPS access."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import tempfile
import runpy
import threading
import unittest

PROBE = Path(__file__).with_name("rollout-probe.py")

class Probe(unittest.TestCase):
    def campaign(self, *, status=200, unavailable=0, change=True, http_only=False, workers=128):
        observed_agents = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_): pass
            def do_GET(self):
                observed_agents.append(self.headers.get("User-Agent"))
                self.send_response(status); self.end_headers(); self.wfile.write(b"fixture")
            def do_PUT(self):
                observed_agents.append(self.headers.get("User-Agent"))
                self.rfile.read(int(self.headers["Content-Length"]))
                self.send_response(200); self.end_headers()
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        self.addCleanup(server.server_close); self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            descriptor = root/"upload.json"
            descriptor.write_text(json.dumps({"url":f"http://127.0.0.1:{server.server_port}/abastevo-dev-photos/q/synthetic?X-Amz-Signature=DO-NOT-LOG-THIS",
                "bytes":8192,"bytes_per_second":65536}))
            descriptor.chmod(0o600)
            watcher = root/"kubectl"
            # A real subprocess emits the same stream shape as kubectl.
            watcher.write_text('''#!/usr/bin/env python3
import json, time, sys
assert sys.argv[1:5] == ['-n','abastevo-temp','get','deployment']
def event(generation,image,unavailable):
    return {"type":"MODIFIED","object":{"metadata":{"name":"abastevo-temp-api","namespace":"abastevo-temp","generation":generation},
      "spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"api","image":image}]}}},
      "status":{"observedGeneration":generation,"updatedReplicas":1,"availableReplicas":1-unavailable,"unavailableReplicas":unavailable}}}
print(json.dumps(event(1,'old',0)),flush=True)
time.sleep(.1)
print(json.dumps(event(2,IMAGE,UNAVAILABLE)),flush=True)
time.sleep(30)
'''.replace("IMAGE",repr("new" if change else "old")).replace("UNAVAILABLE",str(unavailable)))
            watcher.chmod(0o700)
            output = root/"events.jsonl"
            args=["python3",str(PROBE),"--base-url",f"http://127.0.0.1:{server.server_port}",
                "--workers",str(workers),"--duration",".5","--rate","20","--slow-start","0","--timeout","1",
                "--output",str(output),"--kubectl",str(watcher),"--slow-upload-file",str(descriptor)]
            if http_only: args.append("--http-only")
            run = subprocess.run(args,capture_output=True,text=True,timeout=15)
            events=output.read_text()
            self.assertNotIn("DO-NOT-LOG-THIS",events+run.stdout+run.stderr)
            self.assertTrue(all(a=="abastevo-rollout-probe/1.0" for a in observed_agents))
            return run.returncode,json.loads(events.splitlines()[-1])

    def test_complete_observed_rollout_passes(self):
        code,summary=self.campaign()
        self.assertEqual(code,0,summary)
        self.assertTrue(summary["acceptance_passed"])
        self.assertEqual(summary["requests"],11)

    def test_non_2xx_fails_without_retry(self):
        code,summary=self.campaign(status=503)
        self.assertEqual(code,1)
        self.assertGreater(summary["failed_requests"],0)

    def test_unavailable_replica_fails_even_with_green_http(self):
        code,summary=self.campaign(unavailable=1)
        self.assertEqual(code,1)
        self.assertFalse(summary["acceptance_passed"])

    def test_no_image_change_is_incomplete(self):
        code,summary=self.campaign(change=False)
        self.assertEqual(code,2)
        self.assertFalse(summary["acceptance_passed"])

    def test_scheduler_saturation_is_failure(self):
        code,summary=self.campaign(workers=1)
        self.assertEqual(code,1)
        self.assertFalse(summary["acceptance_passed"])

    def test_private_descriptor_permissions_refused(self):
        read=runpy.run_path(str(PROBE))["descriptor"]
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/"upload.json"
            path.write_text('{}')
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                read(str(path),"https://teste.abastevo.com.br",60,0)

    def test_http_only_never_certifies(self):
        code,summary=self.campaign(http_only=True)
        self.assertEqual(code,2)
        self.assertFalse(summary["acceptance_passed"])

if __name__ == "__main__":
    unittest.main()
