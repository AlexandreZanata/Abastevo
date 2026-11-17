import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))  # publish imports its sibling helpers by name
spec = importlib.util.spec_from_file_location('publish', HERE / 'publish.py')
p = importlib.util.module_from_spec(spec); spec.loader.exec_module(p)
TAG = 'android-v3.1.18'
SHA = 'e' * 40


class FakeGitHub:
    """The GitHub semantics this flow depends on: a draft release appears in
    the release list, but the tag endpoint resolves only published releases."""

    def __init__(self, releases=()):
        self.releases = [json.loads(json.dumps(r)) for r in releases]
        self.created = 0
        self.mutations = []

    def release(self, tag):
        found = [r for r in self.releases if r['tag_name'] == tag]
        assert len(found) == 1, found
        return found[0]

    def run(self, args, input=None, text=None, capture_output=None, check=None):
        ok = lambda out='': SimpleNamespace(returncode=0, stdout=out, stderr='')
        if args[:2] == ['gh', 'api'] and args[2] == f'repos/{p.REPOSITORY}/releases/tags/{TAG}':
            found = [r for r in self.releases if r['tag_name'] == TAG and not r['draft']]
            if found:
                return ok(json.dumps(found[0]))
            return SimpleNamespace(returncode=1, stdout='', stderr='gh: Not Found (HTTP 404)')
        if args[:3] == ['gh', 'api', '--paginate'] and args[3].startswith(f'repos/{p.REPOSITORY}/releases?'):
            return ok(''.join(json.dumps(r) + '\n' for r in self.releases))
        if args[:3] == ['gh', 'release', 'create']:
            self.created += 1
            self.releases.append({'tag_name': args[3], 'draft': True, 'prerelease': True, 'assets': [],
                                  'html_url': 'https://github.com/x/releases/tag/untagged-1'})
            return ok()
        if args[:3] == ['gh', 'release', 'upload']:
            path = Path(args[4])
            self.mutations.append(('upload', path.name))
            self.release(args[3])['assets'].append(
                {'name': path.name, 'size': path.stat().st_size, 'digest': 'sha256:' + p.digest(path)})
            return ok()
        if args[:3] == ['gh', 'release', 'edit']:
            self.mutations.append(('publish', args[3]))
            release = self.release(args[3])
            release['draft'], release['html_url'] = False, 'https://github.com/x/releases/tag/' + args[3]
            return ok()
        raise AssertionError(f'unexpected command {args}')


def bundle(directory):
    files = {'abastevo-staging.apk': b'synthetic apk', 'NOTICE.txt': b'synthetic notice'}
    for name, raw in files.items():
        (directory / name).write_bytes(raw)
    manifest = {'schema': 1, 'channel': 'staging', 'application_id': 'com.anpfuel.app', 'tag': TAG, 'commit': SHA,
                'version_name': '3.1.18', 'version_code': 22,
                'files': {n: {'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)} for n, raw in files.items()}}
    (directory / 'release.json').write_text(json.dumps(manifest))
    (directory / 'SHA256SUMS').write_text(''.join(
        f'{p.digest(path)}  {path.name}\n' for path in sorted(directory.iterdir()) if path.name != 'SHA256SUMS'))
    return sorted(path.name for path in directory.iterdir())


class Publication(unittest.TestCase):
    def publish(self, github):
        with tempfile.TemporaryDirectory() as temp:
            names = bundle(Path(temp))
            out = io.StringIO()
            with mock.patch.object(p.subprocess, 'run', github.run), \
                 mock.patch.object(p, 'gh', lambda path: {'object': {'sha': SHA}}), \
                 mock.patch.object(p, 'assert_ready', lambda sha: None), \
                 mock.patch.object(p, 'ensure_tag', lambda tag, sha: None), \
                 mock.patch.object(p, 'verify_tag', lambda tag, sha: None), \
                 mock.patch.object(sys, 'argv', ['publish.py', temp]), \
                 contextlib.redirect_stdout(out):
                p.main()
            return names, out.getvalue()

    def test_first_publication_completes_the_draft_it_created(self):
        github = FakeGitHub()
        names, out = self.publish(github)
        release = github.release(TAG)
        self.assertEqual(github.created, 1)
        self.assertFalse(release['draft'])
        self.assertTrue(release['prerelease'])
        self.assertEqual(sorted(a['name'] for a in release['assets']), names)
        self.assertEqual(out.strip(), 'https://github.com/x/releases/tag/' + TAG)

    def test_rerun_resumes_an_interrupted_draft_instead_of_adding_another(self):
        github = FakeGitHub([{'tag_name': TAG, 'draft': True, 'prerelease': True, 'assets': [],
                              'html_url': 'https://github.com/x/releases/tag/untagged-0'}])
        names, _ = self.publish(github)
        self.assertEqual(github.created, 0)
        self.assertEqual(len(github.releases), 1)
        self.assertFalse(github.release(TAG)['draft'])
        self.assertEqual(sorted(a['name'] for a in github.release(TAG)['assets']), names)

    def test_several_releases_claiming_the_tag_refuse_without_mutation(self):
        draft = {'tag_name': TAG, 'draft': True, 'prerelease': True, 'assets': [], 'html_url': 'u'}
        github = FakeGitHub([draft, draft])
        with self.assertRaisesRegex(ValueError, 'several releases'):
            self.publish(github)
        self.assertEqual((github.created, github.mutations), (0, []))


if __name__ == '__main__':
    unittest.main()
