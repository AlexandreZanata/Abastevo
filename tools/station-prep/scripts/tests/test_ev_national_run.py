"""Synthetic subprocesses only; no national inputs, network or database."""
from pathlib import Path
import sys
import tempfile
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import ev_national_run as runner

class ResourceTests(unittest.TestCase):
    def limits(self, **changes):
        limits = dict(max_wall_seconds=5, max_cpu_seconds=3,
                      max_memory_bytes=128 << 20, max_output_file_bytes=1 << 20,
                      max_working_disk_bytes=2 << 20)
        limits.update(changes)
        return limits

    def test_success_measures_and_does_not_publish_child_output(self):
        with tempfile.TemporaryDirectory() as root:
            measured = runner.run_process([sys.executable, '-c', 'print("[P25-TEST] synthetic private output")'], Path(root), [], self.limits())
            self.assertGreater(measured['peak_rss_bytes'], 0)
            self.assertGreater(measured['wall_seconds'], 0)
            self.assertNotIn('synthetic private', str(measured))

    def test_failure_timeout_file_and_disk_caps(self):
        cases = [
            ('raise SystemExit(7)', {}),
            ('import time; time.sleep(5)', dict(max_wall_seconds=0.1)),
            ('open("large", "wb").write(b"x"*16384)', dict(max_output_file_bytes=1024)),
            ('import time; open("large", "wb").write(b"x"*16384); time.sleep(2)', dict(max_working_disk_bytes=1024)),
            ('x=bytearray(256*1024*1024)', dict(max_memory_bytes=64 << 20)),
        ]
        for code, changes in cases:
            with self.subTest(code=code), tempfile.TemporaryDirectory() as root:
                with self.assertRaises(runner.RunError):
                    runner.run_process([sys.executable, '-c', code], Path(root), [], self.limits(**changes))

    def test_disk_accounting_uses_only_owned_paths_and_refuses_symlinks(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root)
            (path/'owned').mkdir()
            (path/'owned'/'input').write_bytes(b'x'*5)
            (path/'unrelated').write_bytes(b'x'*10000)
            self.assertEqual(5, runner.working_bytes(path/'owned', []))
            (path/'owned'/'link').symlink_to(path/'unrelated')
            with self.assertRaises(runner.RunError):
                runner.working_bytes(path/'owned', [])
