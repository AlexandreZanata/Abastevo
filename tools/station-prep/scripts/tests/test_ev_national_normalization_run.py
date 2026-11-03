"""Synthetic parser subprocess; verifies the wrapper, not national parsing."""
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
import ev_normalization_run as runner

class NormalizationRunTests(unittest.TestCase):
    def execute(self,root,fault=None):
        pin_raw=(runner.PROFILE_FILE.parent/'national-source-pins-20261103-v1.json').read_bytes()
        profile={'profile_id':json.loads(pin_raw)['profile_id'],'sha256':hashlib.sha256(pin_raw).hexdigest(),'exposure':'PRIVATE_PREPARATION'}
        manifest={'format':'abastevo.ev-source-sites-v1','source_profile':profile,'summary':{'accepted_source_sites':0},'files':{name:{'bytes':0,'sha256':hashlib.sha256(b'').hexdigest()} for name in ['sites.ndjson','decisions.ndjson','issues.ndjson','uf-census.json']}}
        manifest['normalization_profile_sha256']=hashlib.sha256((runner.PROFILE_FILE.parent/'normalization-profile-v1.json').read_bytes()).hexdigest()
        if fault=='normalization':manifest['normalization_profile_sha256']='0'*64
        if fault=='profile':manifest['source_profile']['sha256']='0'*64
        if fault=='format':manifest['format']='wrong-private-format'
        if fault=='file':manifest['files']['sites.ndjson']['sha256']='0'*64
        if fault=='missing':manifest['files'].pop('issues.ndjson')
        if fault=='path':manifest['files']={'../unsafe':manifest['files']['sites.ndjson']}
        seed=root/'seed.json';seed.write_text(json.dumps(manifest))
        binary=root/'synthetic-parser';binary.write_text('#!'+sys.executable+'\nfrom pathlib import Path\nimport sys\nout=Path(sys.argv[sys.argv.index("--output")+1]);out.mkdir()\n[(out/name).write_bytes(b"") for name in ["sites.ndjson","decisions.ndjson","issues.ndjson","uf-census.json"]]\n(out/"manifest.json").write_bytes(Path('+repr(str(seed))+').read_bytes())\nprint("[P25-TEST] synthetic-private-parser-output")\n');binary.chmod(0o700)
        osm=root/'synthetic.pbf';osm.write_bytes(b'[P25-TEST] synthetic input')
        boundary=root/'synthetic-boundary';boundary.write_bytes(b'[P25-TEST] synthetic boundary')
        ocm=root/'ocm';ocm.mkdir()
        argv=['runner','--binary',str(binary),'--osm',str(osm),'--ocm',str(ocm),'--boundaries',str(boundary),'--output',str(root/'output')]
        capture=io.StringIO()
        with patch.object(sys,'argv',argv),redirect_stdout(capture):runner.main()
        return capture.getvalue()

    def test_hash_bound_receipt_and_private_child_output(self):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);out=self.execute(root)
            self.assertEqual('LOCAL_NORMALIZED_NOT_CONFLATED_OR_IMPORTED',json.loads(out)['status'])
            self.assertNotIn('synthetic-private-parser-output',out)
            self.assertEqual(0o600,(root/'output/execution-receipt.json').stat().st_mode&0o777)
            self.assertEqual(0o700,(root/'output').stat().st_mode&0o777)

    def test_bad_profile_format_file_and_path_never_record_success(self):
        for fault in ['profile','normalization','format','file','path','missing']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as raw:
                root=Path(raw)
                with self.assertRaises(runner.RunError):self.execute(root,fault)
                self.assertFalse((root/'output/execution-receipt.json').exists())
