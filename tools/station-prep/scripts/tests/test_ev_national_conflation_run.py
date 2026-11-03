"""Synthetic parser exercises profile/file binding and private wrapper output."""
from contextlib import redirect_stdout
import hashlib
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import ev_conflation_run as runner
class ConflationWrapperTests(unittest.TestCase):
    def execute(self,root,fault=None):
        normalized=root/'normalized';normalized.mkdir();raw=b'[P25-TEST] synthetic input manifest';(normalized/'manifest.json').write_bytes(raw);input_hash=hashlib.sha256(raw).hexdigest()
        boundary=root/'boundary';boundary.write_bytes(b'[P25-TEST] synthetic boundaries')
        expected={'id':'ev-conflation-v1','sha256':hashlib.sha256((runner.PROFILE_FILE.parent/'conflation-profile-v1.json').read_bytes()).hexdigest(),'exposure':'PRIVATE_PREPARATION'}
        manifest={'format':'abastevo.ev-conflated-v1','conflation_profile':expected,'input_manifest_sha256':input_hash,'stats':{'confirmed_pairs':0},'files':{name:{'bytes':0,'sha256':hashlib.sha256(b'').hexdigest()} for name in runner.FILES}}
        if fault=='profile':manifest['conflation_profile']['sha256']='0'*64
        if fault=='missing':manifest['files'].pop('catalog.ndjson')
        if fault=='file':manifest['files']['catalog.ndjson']['sha256']='0'*64
        if fault=='input':manifest['input_manifest_sha256']='0'*64
        seed=root/'seed';seed.write_text(json.dumps(manifest))
        script=root/'synthetic-parser';script.write_text('#!'+sys.executable+'\nimport sys,json\nfrom pathlib import Path\nm=json.loads(Path('+repr(str(seed))+').read_bytes());out=Path(sys.argv[sys.argv.index("--output")+1]);out.mkdir()\n[(out/n).write_bytes(b"") for n in '+repr(sorted(runner.FILES))+']\n(out/"manifest.json").write_text(json.dumps(m))\nprint("[P25-TEST] synthetic-private-output")\n');script.chmod(0o700)
        args=['wrapper','--binary',str(script),'--normalized',str(normalized),'--manifest-sha256',input_hash,'--boundaries',str(boundary),'--output',str(root/'out')]
        capture=io.StringIO()
        with patch.object(sys,'argv',args),redirect_stdout(capture):runner.main()
        return capture.getvalue()
    def test_private_hash_bound_receipt(self):
        with tempfile.TemporaryDirectory() as name:
            root=Path(name);out=self.execute(root);self.assertEqual('LOCAL_CONFLATED_OWNER_REVIEW_AND_IMPORT_PENDING',json.loads(out)['status']);self.assertNotIn('synthetic-private-output',out);self.assertEqual(0o600,(root/'out/execution-receipt.json').stat().st_mode&0o777)
    def test_wrong_profiles_input_and_file_descriptors_never_record_success(self):
        for fault in ['profile','missing','file','input']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as name:
                root=Path(name)
                with self.assertRaises(runner.RunError):self.execute(root,fault)
                self.assertFalse((root/'out/execution-receipt.json').exists())
