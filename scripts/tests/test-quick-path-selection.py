#!/usr/bin/env python3
"""Execute the real quick gate classifier without running the aggregate gate."""
from pathlib import Path
import re
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[2]
SOURCE = (ROOT / 'scripts/quick-verify.sh').read_text()
CLASSIFIER = re.search(r'case "\$f" in\n.*?\n        esac', SOURCE, re.S).group()


def classify(path):
    return subprocess.run(
        ['bash', '-c', 'set -euo pipefail\nf=$1\nDOCS_ONLY=1\n' + CLASSIFIER +
         '\nprintf "%s\\n" "$DOCS_ONLY"\n', 'quick-path-fixture', path],
        capture_output=True, text=True,
    )


class QuickPathSelection(unittest.TestCase):
    def test_docker_build_filter_runs_full_gate(self):
        result = classify('.dockerignore')
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual('0', result.stdout.strip())

    def test_existing_code_and_document_selection_is_preserved(self):
        for path in ['backend/internal/sample.go', 'infra/docker/Dockerfile',
                     'tools/station-prep/src/lib.rs', 'app/src/main/Sample.kt',
                     'maps/engine/src/main/MapEngine.kt']:
            with self.subTest(path=path):
                result = classify(path)
                self.assertEqual((0, '0'), (result.returncode, result.stdout.strip()))
        for path in ['docs/planning/PROGRESS.md', 'README.md']:
            with self.subTest(path=path):
                result = classify(path)
                self.assertEqual((0, '1'), (result.returncode, result.stdout.strip()))

    def test_unknown_path_still_refuses(self):
        result = classify('unclassified-new-area/implementation.txt')
        self.assertNotEqual(0, result.returncode)
        self.assertIn('unclassified changed path', result.stderr + result.stdout)


if __name__ == '__main__':
    unittest.main(verbosity=2)
