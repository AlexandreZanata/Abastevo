"""Behavior of the staging release gate in release-staging.yml.

The gate script is taken from the workflow text and run with a fake `gh` and
`oras`, so its rules are exercised without GitHub. Standard library only; the
gate's own `jq` is the one tool expected on PATH.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/release-staging.yml'
SHA = 'b' * 40
PATHS = ('.github/workflows/quick.yml', '.github/workflows/ci.yml', '.github/workflows/backend.yml')
PASSED = dict.fromkeys(PATHS, 'success')
SKIP_CANCELLED = ("    if: ${{ github.event_name != 'workflow_run' ||"
                  " github.event.workflow_run.conclusion != 'cancelled' }}\n")
FAKES = {
    'gh': '#!/usr/bin/env bash\ncat "$FAKE_RUNS"\n',
    'oras': ('#!/usr/bin/env bash\n'
             'if [[ "$1" == "login" ]]; then cat >/dev/null; exit 0; fi\n'
             'if [[ "$1 $2" == "manifest fetch" ]]; then [[ "$FAKE_PUBLISHED" == "true" ]]; exit; fi\n'
             'exit 2\n'),
}


def step_script(name):
    step = WORKFLOW.read_text().split(f'      - name: {name}\n', 1)[1]
    body = step.split('        run: |\n', 1)[1]
    lines = []
    for line in body.splitlines():
        if line.strip() and not line.startswith(' ' * 10):
            break
        lines.append(line[10:])
    return '\n'.join(lines) + '\n'


def job_head(name):
    return WORKFLOW.read_text().split(f'\n  {name}:\n', 1)[1].split('\n    steps:\n', 1)[0] + '\n'


def evaluate(conclusions, published=False):
    """Runs the gate for a workflow_run event; returns (exit code, step output)."""
    runs = [{'path': path, 'event': 'push', 'head_branch': 'main',
             'created_at': '2026-01-01T00:00:00Z', 'conclusion': conclusion}
            for path, conclusion in conclusions.items()]
    with tempfile.TemporaryDirectory() as raw:
        tmp = Path(raw)
        (tmp / 'runs.json').write_text(json.dumps({'workflow_runs': runs}))
        for tool, body in FAKES.items():
            (tmp / tool).write_text(body)
            (tmp / tool).chmod(0o755)
        (tmp / 'gate.sh').write_text(step_script('Evaluate release gate'))
        output = tmp / 'output'
        output.touch()
        env = {
            'PATH': f"{tmp}{os.pathsep}{os.environ['PATH']}",
            'EVENT_NAME': 'workflow_run', 'GH_TOKEN': 'fake', 'REPOSITORY': 'owner/repo',
            'SHA': SHA, 'OWNER': 'Owner', 'ACTOR': 'actor', 'GITHUB_OUTPUT': str(output),
            'FAKE_RUNS': str(tmp / 'runs.json'), 'FAKE_PUBLISHED': 'true' if published else 'false',
        }
        done = subprocess.run(['bash', str(tmp / 'gate.sh')], env=env, capture_output=True,
                              text=True, check=False)
        return done.returncode, output.read_text()


class ReleaseGateTests(unittest.TestCase):
    def test_gate_opens_only_when_all_three_main_workflows_passed(self):
        self.assertEqual(evaluate(PASSED), (0, 'ready=true\n'))

    def test_unfinished_or_absent_workflow_leaves_the_gate_closed(self):
        unfinished = {**PASSED, PATHS[2]: None}
        absent = {path: 'success' for path in PATHS[:2]}
        for conclusions in (unfinished, absent):
            with self.subTest(conclusions=conclusions):
                self.assertEqual(evaluate(conclusions), (0, 'ready=false\n'))

    def test_cancelled_workflow_leaves_the_gate_closed_without_failing(self):
        # A later push to main cancels the runs of the commit it replaces. That
        # commit has nothing to release and nothing failed, so the run must not
        # end red.
        for path in PATHS:
            with self.subTest(cancelled=path):
                self.assertEqual(evaluate({**PASSED, path: 'cancelled'}), (0, 'ready=false\n'))

    def test_failed_workflow_fails_the_gate(self):
        for outcome in ('failure', 'timed_out'):
            with self.subTest(outcome=outcome):
                self.assertEqual(evaluate({**PASSED, PATHS[1]: outcome}), (1, ''))

    def test_published_commit_is_not_released_twice(self):
        self.assertEqual(evaluate(PASSED, published=True), (0, 'ready=false\n'))

    def test_cancelled_trigger_skips_the_gate_and_every_publishing_job(self):
        # The cancelled run itself still fires workflow_run. Skipping the gate
        # job leaves every job that needs it skipped as well.
        self.assertIn(SKIP_CANCELLED, job_head('gate'))
        for job in ('landing', 'images', 'manifest'):
            with self.subTest(job=job):
                head = job_head(job)
                self.assertRegex(head, r'(?m)^    needs: (gate|\[gate, .*\])$')
                self.assertIn("    if: needs.gate.outputs.ready == 'true'\n", head)


if __name__ == '__main__':
    unittest.main()
