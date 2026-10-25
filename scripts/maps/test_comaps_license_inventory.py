import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

script = Path(__file__).with_name('audit-comaps-release.py')
spec = importlib.util.spec_from_file_location('audit', script)
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class LicenseInventoryTest(unittest.TestCase):
    def test_compressed_java_runtime_constants_are_scanned(self):
        with tempfile.TemporaryDirectory() as temporary:
            jar = Path(temporary) / 'classes.jar'
            with zipfile.ZipFile(jar, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
                archive.writestr('Example.class', b'constant\x00https://test.invalid/api\x00')
            self.assertIn('test.invalid', audit.scan(jar)['url_hosts'])

    def test_all_nested_licenses_are_retained_in_stable_path_order(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            component = root / '3party/combined'
            for folder, name, text in [('z', 'LICENSE', 'license Z'), ('a', 'LICENCE', 'license A')]:
                path = component / folder / name
                path.parent.mkdir(parents=True)
                path.write_text(text)
            found = audit.license_entries(root, '3party/combined', component)
            self.assertEqual(['3party/combined/a/LICENCE', '3party/combined/z/LICENSE'],
                             [item['path'] for item in found])


class AuditAdmissionTest(unittest.TestCase):
    def test_detector_still_recognizes_constructed_token_pattern(self):
        prefix = b"github_" + b"pat_"
        self.assertIsNotNone(audit.SECRET.fullmatch(prefix + b"0" * 40))
        self.assertIsNone(audit.SECRET.fullmatch(prefix + b"0" * 39))

    def clean(self):
        return {'license_files_missing': [], 'submodules_total': 193,
                'submodules_clean': 193, 'endpoint_decision': {'unclassified': []},
                'artifacts': {'jni/core.so': {'secret_pattern_hits': []}}}

    def test_unknown_endpoint_or_license_gap_or_secret_refuses_candidate(self):
        self.assertEqual([], audit.admission_failures(self.clean()))
        for reason in ('license', 'endpoint', 'secret', 'source'):
            report = self.clean()
            if reason == 'license': report['license_files_missing'] = ['3party/unknown']
            if reason == 'endpoint': report['endpoint_decision']['unclassified'] = ['unknown.invalid']
            if reason == 'secret': report['artifacts']['jni/core.so']['secret_pattern_hits'] = ['test-marker']
            if reason == 'source': report['submodules_clean'] = 192
            with self.subTest(reason=reason):
                self.assertTrue(audit.admission_failures(report))


if __name__ == '__main__':
    unittest.main()
