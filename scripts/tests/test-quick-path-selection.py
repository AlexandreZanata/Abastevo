#!/usr/bin/env python3
"""Execute the real quick gate classifier without running the aggregate gate."""
import os
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
                     'maps/engine/src/main/MapEngine.kt',
                     'release/android/channel.json', 'release/android/signing-certificate.pem',
                     'business-web/src/main.ts']:
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

    def test_company_selection_executes_all_checks_and_tracks_landing_inputs(self):
        block = SOURCE.split('    BUSINESS_WEB_CHANGED=0', 1)[1].split('    # Bounded Rust selection', 1)[0]
        for changed, expected in [
            ('business-web/src/model.ts', True),
            ('landing/static/assets/styles.css', True),
            ('backend/cmd/api/main.go', False),
            ('docs/enterprise/README.md', False),
        ]:
            result = subprocess.run(
                ['bash', '-c',
                 'set -euo pipefail\nCHANGED_ALL=$1\nBUSINESS_WEB_CHANGED=0\n'
                 'npm() { printf "%s\\n" "$*"; }\npython3() { :; }\ndocker() { :; }\n' + block,
                 'company-selection-fixture', changed], capture_output=True, text=True)
            self.assertEqual(0, result.returncode, result.stderr)
            for command in ['run typecheck', 'run build', 'run check', 'business-web test']:
                self.assertEqual(expected, command in result.stdout)

    def test_company_selection_runs_actual_caddy_policy_where_docker_exists(self):
        block = SOURCE.split('    BUSINESS_WEB_CHANGED=0', 1)[1].split('    # Bounded Rust selection', 1)[0]

        def run(changed, docker, ci):
            script = ('set -euo pipefail\nCHANGED_ALL=$1\nBUSINESS_WEB_CHANGED=0\nnpm() { :; }\n'
                      'python3() { printf "python3 %s\\n" "$*"; }\n'
                      + ('docker() { :; }\n' if docker else 'docker() { return 1; }\n') + block)
            env = {k: v for k, v in os.environ.items() if k != 'CI'}
            if ci:
                env['CI'] = 'true'
            return subprocess.run(['bash', '-c', script, 'caddy-selection-fixture', changed],
                                  capture_output=True, text=True, env=env)

        for changed, expected in [
            ('infra/caddy/staging-web.Caddyfile', True),
            ('infra/k8s/staging/web-caddy-configmap.yaml', True),
            ('business-web/static/assets/business.css', True),
            ('infra/caddy/Caddyfile', False),
            ('backend/cmd/api/main.go', False),
        ]:
            with self.subTest(changed=changed):
                result = run(changed, True, True)
                self.assertEqual(0, result.returncode, result.stderr)
                self.assertEqual(expected, 'python3 business-web/tests/caddy.py' in result.stdout)
        refused = run('business-web/src/app.ts', False, True)
        self.assertNotEqual(0, refused.returncode)
        self.assertIn('needs Docker in CI', refused.stderr)
        local = run('business-web/src/app.ts', False, False)
        self.assertEqual(0, local.returncode, local.stderr)
        self.assertNotIn('caddy.py', local.stdout)
        self.assertIn('not run locally', local.stderr)


if __name__ == '__main__':
    unittest.main(verbosity=2)
