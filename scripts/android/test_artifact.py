import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile
import artifact
import publish

VERSION={'schema':1,'channel':'staging','application_id':'com.anpfuel.app','version_name':'3.1.1','version_code':5,'commit':'a'*40,'tag':'android-v3.1.1'}
CERT=json.loads(Path('release/android/signing-certificate.json').read_text())

class ArtifactRules(unittest.TestCase):
 def apk(self,root,missing=None):
  apk=root/'app.apk'
  with zipfile.ZipFile(apk,'w') as z:
   if missing!='engine':z.writestr('lib/arm64-v8a/liborganicmaps.so',b'test-only-fixture')
   z.writestr('lib/arm64-v8a/libother.so',b'fixture')
   for name,source in artifact.LICENSE_FILES.items():
    if missing!=name:z.writestr('assets/licenses/'+name,Path(source).read_bytes())
   if missing=='private':z.writestr('assets/private.p12',b'fixture')
  return apk
 def output(self,debug=False,cert=None,package='com.anpfuel.app'):
  def run(cmd,**kwargs):
   if 'badging' in cmd:return f"package: name='{package}' versionCode='5' versionName='3.1.1'\n"+('application-debuggable\n' if debug else '')
   return 'Signer #1 certificate SHA-256 digest: '+(cert or CERT['certificate_sha256'])+'\n'
  return run
 def test_only_correct_non_debug_signed_native_licensed_apk(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d)
   for missing in [None,'engine','Abastevo-LICENSE.txt','private']:
    apk=self.apk(root,missing)
    with patch('subprocess.check_output',side_effect=self.output()):
     if missing:
      with self.assertRaises((ValueError,KeyError)):artifact.verify_apk(apk,VERSION,CERT,'aapt','apksigner')
     else:self.assertEqual(artifact.verify_apk(apk,VERSION,CERT,'aapt','apksigner'),['arm64-v8a'])
   apk=self.apk(root)
   for opts in [{'debug':True},{'cert':'0'*64},{'package':'com.anpfuel.app.debug'}]:
    with patch('subprocess.check_output',side_effect=self.output(**opts)):
     with self.assertRaises(ValueError):artifact.verify_apk(apk,VERSION,CERT,'aapt','apksigner')
   with open(apk,'wb') as stream:stream.truncate(artifact.MAX_APK_BYTES+1)
   with self.assertRaises(ValueError):artifact.verify_apk(apk,VERSION,CERT,'aapt','apksigner')
 def test_bundle_checksums_and_immutable_asset_conflicts(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);apk=self.apk(root);out=root/'public'
   with patch('subprocess.check_output',side_effect=self.output()):artifact.bundle(apk,VERSION,CERT,out,'aapt','apksigner')
   self.assertEqual(publish.verify_local(out)['commit'],VERSION['commit'])
   target=out/'Abastevo-3.1.1-android.apk'
   remote={'size':target.stat().st_size,'digest':'sha256:'+artifact.digest(target)}
   self.assertTrue(publish.same_asset(target,remote))
   self.assertFalse(publish.same_asset(target,{**remote,'digest':'sha256:'+'0'*64}))
   (out/'unexpected').write_text('no')
   with self.assertRaises(ValueError):publish.verify_local(out)
   (out/'unexpected').unlink();target.write_bytes(b'corrupt')
   with self.assertRaises(ValueError):publish.verify_local(out)

class TagConcurrency(unittest.TestCase):
 def test_existing_tag_is_never_updated(self):
  from subprocess import CompletedProcess
  with patch('publish.subprocess.run',return_value=CompletedProcess([],0,'{}','')) as run, patch('publish.verify_tag') as verify:
   publish.ensure_tag('android-v3.1.1','a'*40)
   self.assertEqual(run.call_count,1);verify.assert_called_once_with('android-v3.1.1','a'*40)
 def test_tag_creation_race_rechecks_exact_identity(self):
  from subprocess import CompletedProcess
  results=[CompletedProcess([],1,'','HTTP 404'),CompletedProcess([],1,'','HTTP 422')]
  with patch('publish.subprocess.run',side_effect=results) as run, patch('publish.verify_tag') as verify:
   publish.ensure_tag('android-v3.1.1','a'*40)
   self.assertEqual(run.call_count,2);verify.assert_called_once()
   self.assertEqual(json.loads(run.call_args.kwargs['input']),{'ref':'refs/tags/android-v3.1.1','sha':'a'*40})
 def test_permission_failure_does_not_create_tag(self):
  from subprocess import CompletedProcess
  with patch('publish.subprocess.run',return_value=CompletedProcess([],1,'','HTTP 403')) as run:
   with self.assertRaises(ValueError):publish.ensure_tag('android-v3.1.1','a'*40)
   self.assertEqual(run.call_count,1)

if __name__=='__main__':unittest.main()

class PublicationRecovery(unittest.TestCase):
 def exercise(self,mode):
  import copy
  import sys
  from subprocess import CompletedProcess
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);out=root/'public';fixture=ArtifactRules();apk=fixture.apk(root)
   with patch('subprocess.check_output',side_effect=fixture.output()):artifact.bundle(apk,VERSION,CERT,out,'aapt','apksigner')
   assets=[{'name':p.name,'size':p.stat().st_size,'digest':'sha256:'+artifact.digest(p)} for p in out.iterdir()]
   state={'release':None};calls=[]
   if mode in ['existing','partial','conflict']:
    state['release']={'draft':mode!='existing','prerelease':True,'assets':copy.deepcopy(assets if mode=='existing' else assets[:1]),'html_url':'https://example.invalid/release'}
   if mode=='conflict':state['release']['assets'][0]['digest']='sha256:'+'0'*64
   def run(cmd,**kwargs):
    calls.append(cmd)
    if cmd[1:3]==['release','create']:state['release']={'draft':True,'prerelease':True,'assets':[],'html_url':'https://example.invalid/release'}
    elif cmd[1:3]==['release','upload']:
     p=Path(cmd[4]);state['release']['assets'].append({'name':p.name,'size':p.stat().st_size,'digest':'sha256:'+artifact.digest(p)})
    elif cmd[1:3]==['release','edit']:
     self.assertEqual(len(state['release']['assets']),len(assets),'incomplete draft exposed')
     state['release']['draft']=False
    else:raise AssertionError('unexpected external operation')
    return CompletedProcess(cmd,0,'','')
   with patch.object(sys,'argv',['publish.py',str(out)]), patch('publish.gh',return_value={'object':{'sha':'b'*40 if mode=='stale' else VERSION['commit']}}), patch('publish.assert_ready',side_effect=ValueError('checks changed') if mode=='recheck-failed' else None), patch('publish.ensure_tag'), patch('publish.verify_tag'), patch('publish.release_or_none',side_effect=lambda _:state['release']), patch('publish.subprocess.run',side_effect=run):
    if mode in ['conflict','stale','recheck-failed']:
     with self.assertRaises(ValueError):publish.main()
     self.assertEqual(calls,[])
    else:
     publish.main();self.assertFalse(state['release']['draft'])
     if mode=='existing':self.assertEqual(calls,[])
     else:self.assertEqual(calls[-1][1:3],['release','edit'])
 def test_new_publication_is_exposed_after_hash_readback(self):self.exercise('new')
 def test_missing_draft_uploads_resume_without_clobber(self):self.exercise('partial')
 def test_same_published_bundle_is_idempotent(self):self.exercise('existing')
 def test_conflicting_asset_is_not_mutated(self):self.exercise('conflict')
 def test_stale_main_cannot_publish(self):self.exercise('stale')
 def test_failed_final_recheck_cannot_create_tag_or_release(self):self.exercise('recheck-failed')
