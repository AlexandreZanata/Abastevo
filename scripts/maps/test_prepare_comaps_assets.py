import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('admission', Path(__file__).with_name('prepare-comaps-assets.py'))
admission = importlib.util.module_from_spec(spec)
spec.loader.exec_module(admission)


class AdmissionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.notices = self.root / 'notices'
        self.notices.mkdir()
        (self.notices / 'LICENSE').write_text('The retained license')
        self.content = {'assets/fonts/open.ttf': b'open font', 'assets/fonts/shareware.ttf': b'shareware',
                        'classes.jar': b'unchanged release classes', 'jni/arm64-v8a/liborganicmaps.so': b'unchanged core'}
        self.profile = self.root / 'profile.json'
        self.profile.write_text(json.dumps({'format': 'abastevo.map-resources-v1', 'upstream_sha': 'pinned',
            'resources': [{'path': p, 'sha256': hashlib.sha256(v).hexdigest(),
                           'admission': 'EXCLUDED' if 'shareware' in p else 'ADMITTED',
                           'license': 'NOT_ADMITTED' if 'shareware' in p else 'OFL-1.1'}
                          for p, v in self.content.items() if p.startswith('assets/')]}))

    def archive(self, changes=None):
        path = self.root / 'raw.aar'
        with zipfile.ZipFile(path, 'w') as z:
            for name, content in (self.content | (changes or {})).items():
                z.writestr(name, content)
        return path

    def test_excludes_unadmitted_font_retains_code_and_notices_deterministically(self):
        raw = self.archive()
        a, b = self.root / 'a.aar', self.root / 'b.aar'
        admission.prepare(raw, self.profile, self.notices, a)
        admission.prepare(raw, self.profile, self.notices, b)
        self.assertEqual(a.read_bytes(), b.read_bytes())
        with zipfile.ZipFile(a) as z:
            self.assertNotIn('assets/fonts/shareware.ttf', z.namelist())
            self.assertEqual(self.content['classes.jar'], z.read('classes.jar'))
            self.assertEqual(self.content['jni/arm64-v8a/liborganicmaps.so'], z.read('jni/arm64-v8a/liborganicmaps.so'))
            self.assertIn('assets/abastevo-map-licenses/notices.zip', z.namelist())

    def test_unknown_asset_and_changed_known_asset_are_refused(self):
        for change in ({'assets/fonts/new.ttf': b'unknown'}, {'assets/fonts/open.ttf': b'changed'}):
            with self.subTest(change=change), self.assertRaises(ValueError):
                admission.prepare(self.archive(change), self.profile, self.notices, self.root / 'rejected.aar')
            self.assertFalse((self.root / 'rejected.aar').exists())

    def test_incomplete_sdk_locale_retains_translation_and_explicit_default_fallback(self):
        out = self.root / 'localized.aar'
        raw = self.archive({'res/values/values.xml': b'<resources><string name="hello">Hello</string><string name="missing">Fallback</string></resources>',
                            'res/values-pt/values.xml': b'<resources><string name="hello">Ola</string></resources>'})
        admission.prepare(raw, self.profile, self.notices, out)
        with zipfile.ZipFile(out) as z:
            self.assertEqual(b'<resources><string name="hello">Ola</string></resources>', z.read('res/values-pt/values.xml'))
            fallback = z.read('res/values-pt/abastevo_fallbacks.xml')
            self.assertIn(b'name="missing"', fallback)
            self.assertIn(b'Fallback', fallback)
            self.assertNotIn(b'name="hello"', fallback)

    def test_existing_artifact_is_preserved(self):
        out = self.root / 'prior.aar'
        out.write_bytes(b'last good')
        with self.assertRaises(ValueError):
            admission.prepare(self.archive(), self.profile, self.notices, out)
        self.assertEqual(b'last good', out.read_bytes())

    def test_archive_traversal_is_refused(self):
        with self.assertRaises(ValueError):
            admission.prepare(self.archive({'../escape': b'bad'}), self.profile, self.notices, self.root / 'rejected.aar')
        self.assertFalse((self.root.parent / 'escape').exists())


if __name__ == '__main__':
    unittest.main()
