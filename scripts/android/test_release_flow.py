import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('release_flow',Path(__file__).with_name('release_flow.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f)
A='a'*40;B='b'*40
CONFIG={'epoch':A,'base_version':'3.1.0','base_code':4}

class ReleaseRules(unittest.TestCase):
 def test_first_delivery_and_same_head_replay(self):
  self.assertEqual(f.version_for(CONFIG,[A,B]),{'name':'3.1.1','code':5})
  self.assertEqual(f.version_for(CONFIG,[A,B,'c'*40]),{'name':'3.1.2','code':6})
  self.assertEqual(f.version_for(CONFIG,[A,B]),f.version_for(CONFIG,[A,B]))
 def test_epoch_graph_and_overflow_fail(self):
  for graph in [[],[B],[A]]:
   with self.assertRaises(ValueError):f.version_for(CONFIG,graph)
  with self.assertRaises(ValueError):f.version_for({**CONFIG,'base_code':2100000000},[A,B])
 def test_merge_counts_one_delivery(self):
  with tempfile.TemporaryDirectory() as d:
   def git(*args):return subprocess.check_output(['git','-C',d,*args],text=True).strip()
   git('init','-q','-b','main');git('config','user.name','Test');git('config','user.email','test@example.invalid')
   git('commit','--allow-empty','-qm','base');epoch=git('rev-parse','HEAD');git('checkout','-qb','construction')
   git('commit','--allow-empty','-qm','one');git('commit','--allow-empty','-qm','two');git('checkout','-q','main');git('merge','--no-ff','-qm','delivery','construction')
   graph=git('rev-list','--first-parent','--reverse','HEAD').splitlines()
   self.assertEqual(f.version_for({**CONFIG,'epoch':epoch},graph),{'name':'3.1.1','code':5})
 def event(self):return {'repository':{'full_name':f.REPOSITORY},'workflow_run':{'head_sha':B,'head_branch':'main','event':'push','head_repository':{'full_name':f.REPOSITORY},'name':'CI','conclusion':'success'}}
 def test_only_trusted_current_main_event(self):
  e=self.event();self.assertEqual(f.trusted_target(e,'workflow_run',f.REPOSITORY,'refs/heads/main',B),B)
  for key,value in [('head_sha',A),('head_branch','dev'),('event','pull_request'),('head_repository',{'full_name':'fork/repo'}),('name','other'),('conclusion','failure')]:
   bad=self.event();bad['workflow_run'][key]=value
   with self.assertRaises(ValueError):f.trusted_target(bad,'workflow_run',f.REPOSITORY,'refs/heads/main',B)
  with self.assertRaises(ValueError):f.trusted_target(e,'pull_request',f.REPOSITORY,'refs/heads/main',B)
 def test_dispatch_keeps_same_current_head_rule(self):
  e={'repository':{'full_name':f.REPOSITORY},'inputs':{'sha':B}}
  self.assertEqual(f.trusted_target(e,'workflow_dispatch',f.REPOSITORY,'refs/heads/main',B),B)
  for ref,head in [('refs/heads/dev',B),('refs/heads/main',A)]:
   with self.assertRaises(ValueError):f.trusted_target(e,'workflow_dispatch',f.REPOSITORY,ref,head)
 def runs(self):return [dict(id=i+1,run_attempt=1,path=p,event='push',head_branch='main',head_sha=B,head_repository={'full_name':f.REPOSITORY},status='completed',conclusion='success') for i,p in enumerate(sorted(f.WORKFLOWS))]
 def test_every_latest_exact_main_signal_is_required(self):
  runs=self.runs();self.assertTrue(f.gate_ready(runs,B,f.REPOSITORY))
  self.assertFalse(f.gate_ready(runs[:-1],B,f.REPOSITORY))
  for outcome in ['failure','cancelled','skipped','neutral',None]:
   bad=[{**r} for r in runs];bad[0]['conclusion']=outcome;self.assertFalse(f.gate_ready(bad,B,f.REPOSITORY))
  for key,value in [('head_sha',A),('head_branch','dev'),('event','pull_request'),('head_repository',{'full_name':'fork/repo'}),('status','in_progress')]:
   bad=[{**r} for r in runs];bad[0][key]=value;self.assertFalse(f.gate_ready(bad,B,f.REPOSITORY))
  self.assertFalse(f.gate_ready(runs+[{**runs[0],'id':999,'conclusion':'failure'}],B,f.REPOSITORY))
  self.assertFalse(f.gate_ready(runs+[{**runs[0],'run_attempt':2,'status':'in_progress'}],B,f.REPOSITORY))

if __name__=='__main__':unittest.main()

class FinalGateRecheck(unittest.TestCase):
 def test_stale_or_newly_failed_checks_refuse_signing_and_publication(self):
  from unittest.mock import patch
  for current,ready in [(A,True),(B,False)]:
   with patch.object(f,'gh',return_value={'object':{'sha':current}}),patch.object(f,'workflow_runs',return_value=[]),patch.object(f,'gate_ready',return_value=ready):
    with self.assertRaises(ValueError):f.assert_ready(B)
  with patch.object(f,'gh',return_value={'object':{'sha':B}}),patch.object(f,'workflow_runs',return_value=[]),patch.object(f,'gate_ready',return_value=True):f.assert_ready(B)
 def test_workflow_pagination_is_complete(self):
  from unittest.mock import patch
  pages=[{'workflow_runs':[{'id':i} for i in range(100)]},{'workflow_runs':[{'id':100}]}]
  with patch.object(f,'gh',side_effect=pages) as call:
   self.assertEqual(len(f.workflow_runs(B)),101);self.assertEqual(call.call_count,2)
