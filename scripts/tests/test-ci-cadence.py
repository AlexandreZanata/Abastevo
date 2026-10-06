"""Static cadence constraints, not a YAML parser or live GitHub proof.

Uses the Python standard library so the fast helper suite needs no package
installation. Validate workflow YAML independently when its shape changes.
"""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]
CONDITION = "    if: ${{ github.event_name != 'pull_request' || github.event.pull_request.draft == false }}"


class CadenceTests(unittest.TestCase):
    def test_backups_and_drafts_do_not_execute_jobs(self):
        for name, job_count in (('quick', 1), ('backend', 2), ('ci', 1)):
            workflow = (ROOT / f'.github/workflows/{name}.yml').read_text()
            with self.subTest(workflow=name):
                push = workflow.split('  push:\n', 1)[1].split('\n  pull_request:', 1)[0]
                self.assertRegex(push, r'(?m)^    branches: \[main\]$')
                self.assertNotIn('pull_request_target:', workflow)
                pull = re.search(r'(?ms)^  pull_request:\n(.*?)(?=^[^ \n#])', workflow).group(1)
                types = re.search(r'(?m)^    types: \[(.*?)\]$', pull).group(1).split(', ')
                self.assertIn('ready_for_review', types)
                self.assertIn('synchronize', types)
                self.assertIn('permissions:\n  contents: read\n', workflow)
                self.assertIn('  cancel-in-progress: true\n', workflow)
                jobs = re.findall(r'(?m)^  [a-z]+:\n(    if: [^\n]+)', workflow.split('\njobs:\n', 1)[1])
                self.assertEqual(len(jobs), job_count)
                self.assertTrue(all(condition == CONDITION for condition in jobs))

    def test_required_ready_check_has_no_path_filter(self):
        quick = (ROOT / '.github/workflows/quick.yml').read_text()
        events = quick.split('\npermissions:', 1)[0]
        self.assertNotIn('paths:', events)
        self.assertNotIn('paths-ignore:', events)
        self.assertIn('    name: Quick verification\n', quick)
        self.assertIn('        run: make quick-verify\n', quick)


if __name__ == '__main__':
    unittest.main()
