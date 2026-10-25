import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('provision', Path(__file__).with_name('provision-comaps-aar.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ProvisionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.aar = self.root / 'input.aar'
        self.output = self.root / 'out'

    def archive(self, extra=None):
        with zipfile.ZipFile(self.aar, 'w') as z:
            for name in ['classes.jar', 'AndroidManifest.xml', 'jni/arm64-v8a/liborganicmaps.so', 'jni/armeabi-v7a/liborganicmaps.so']:
                z.writestr(name, b'bounded test archive entry')
            if extra:
                z.writestr(extra, b'unsafe')
        return hashlib.sha256(self.aar.read_bytes()).hexdigest()

    def test_exact_input_replays_without_rewriting_cache(self):
        digest = self.archive()
        module.provision(self.aar, self.output, digest)
        stamp = (self.output / 'classes.jar').stat().st_mtime_ns
        module.provision(self.aar, self.output, digest)
        self.assertEqual(stamp, (self.output / 'classes.jar').stat().st_mtime_ns)

    def test_wrong_digest_fails_before_output(self):
        self.archive()
        with self.assertRaisesRegex(ValueError, 'digest'):
            module.provision(self.aar, self.output, '0' * 64)
        self.assertFalse(self.output.exists())

    def test_traversal_fails_without_writing(self):
        digest = self.archive('../outside')
        with self.assertRaisesRegex(ValueError, 'unsafe'):
            module.provision(self.aar, self.output, digest)
        self.assertFalse((self.root / 'outside').exists())
        self.assertFalse(self.output.exists())

    def test_modified_or_extra_cached_files_fail(self):
        digest = self.archive()
        module.provision(self.aar, self.output, digest)
        (self.output / 'classes.jar').write_bytes(b'modified')
        with self.assertRaisesRegex(ValueError, 'modified'):
            module.provision(self.aar, self.output, digest)


if __name__ == '__main__':
    unittest.main()
