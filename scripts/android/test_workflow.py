from pathlib import Path
import re
import unittest

class WorkflowBoundary(unittest.TestCase):
 def test_signing_is_isolated_from_untrusted_events_and_backend_deploy(self):
  raw=Path('.github/workflows/android-release.yml').read_text()
  self.assertNotIn('pull_request',raw)
  self.assertNotIn('write-all',raw)
  self.assertNotIn('set -x',raw)
  self.assertNotIn('printenv',raw)
  self.assertIn('cancel-in-progress: false',raw)
  gate,release=raw.split('\n  release:',1)
  self.assertNotIn('secrets.',gate)
  self.assertNotIn('contents: write',gate)
  self.assertIn("if: needs.gate.outputs.ready == 'true'",release)
  self.assertIn('environment: android-staging-release',release)
  self.assertIn('ref: ${{ needs.gate.outputs.sha }}',release)
  secrets=set(re.findall(r'secrets\.([A-Z0-9_]+)',raw))
  self.assertEqual(secrets,{'ANDROID_RELEASE_KEYSTORE_BASE64','ANDROID_RELEASE_KEYSTORE_PASSWORD','ANDROID_RELEASE_KEY_ALIAS','ANDROID_RELEASE_KEY_PASSWORD'})
  for action in re.findall(r'uses: (\S+)',raw):self.assertRegex(action,r'^[a-zA-Z0-9/-]+@[a-f0-9]{40}$')
  self.assertIn("trap 'rm -f",raw)
  self.assertIn('--max-workers=1',raw)
  self.assertEqual(raw.count('if: always()'),1)
  self.assertNotIn('android-release',Path('.github/workflows/release-staging.yml').read_text())

if __name__=='__main__':unittest.main()
