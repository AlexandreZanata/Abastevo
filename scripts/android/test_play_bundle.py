import json
from pathlib import Path
from subprocess import CompletedProcess
import tempfile
import unittest
from unittest.mock import patch
import zipfile
import play_bundle

VERSION = {
    'schema': 1,
    'channel': 'play-production',
    'application_id': 'com.abastevo',
    'version_name': '3.1.43',
    'version_code': 47,
    'commit': 'a' * 40,
    'tag': 'android-v3.1.43',
}


class PlayBundleRules(unittest.TestCase):
    def build_aab(self, root: Path, *, app_id: str = 'com.abastevo', missing: str | None = None) -> Path:
        aab = root / 'app-release.aab'
        with zipfile.ZipFile(aab, 'w') as archive:
            if missing != 'manifest':
                manifest = (
                    b'\x00' + app_id.encode('utf-8')
                    + b'\x00' + f'{app_id}.fileprovider'.encode('utf-8')
                    + b'\x00' + VERSION['version_name'].encode('utf-8')
                )
                archive.writestr('base/manifest/AndroidManifest.xml', manifest)
            if missing != 'engine':
                archive.writestr('base/lib/arm64-v8a/liborganicmaps.so', b'test-only-fixture')
            archive.writestr('base/lib/arm64-v8a/libother.so', b'fixture')
            for name, source in play_bundle.LICENSE_FILES.items():
                if missing != name:
                    archive.writestr(f'base/assets/licenses/{name}', Path(source).read_bytes())
            if missing == 'private':
                archive.writestr('base/assets/upload.jks', b'fixture')
        return aab

    def test_valid_signed_com_abastevo_aab_passes(self):
        with tempfile.TemporaryDirectory() as tmp:
            aab = self.build_aab(Path(tmp))
            ok = CompletedProcess([], 0, 'jar verified.\nSigner #1\n', '')
            with patch('subprocess.run', return_value=ok):
                self.assertEqual(play_bundle.verify_aab(aab, VERSION), ['arm64-v8a'])

    def test_unsigned_or_tampered_aab_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            aab = self.build_aab(Path(tmp))
            for result in (
                CompletedProcess([], 0, 'jar is unsigned.\n', ''),
                CompletedProcess([], 1, '', 'jarsigner error'),
            ):
                with patch('subprocess.run', return_value=result):
                    with self.assertRaises(ValueError):
                        play_bundle.verify_aab(aab, VERSION)

    def test_wrong_app_id_missing_parts_or_private_key_fail(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            ok = CompletedProcess([], 0, 'jar verified.\n', '')
            with patch('subprocess.run', return_value=ok):
                wrong_id = self.build_aab(root, app_id='com.anpfuel.app')
                with self.assertRaises(ValueError):
                    play_bundle.verify_aab(wrong_id, VERSION)
                for defect in ('manifest', 'engine', 'Abastevo-LICENSE.txt', 'private'):
                    bad = self.build_aab(root, missing=defect)
                    with self.assertRaises(ValueError):
                        play_bundle.verify_aab(bad, VERSION)


if __name__ == '__main__':
    unittest.main()
