"""Pinned synthetic Git object graph, no live HTTP or national data."""
import hashlib
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import ev_ocm_export as export

class Response(io.BytesIO):
    def __init__(self, raw):
        super().__init__(raw)
        self.headers = {'Content-Length': str(len(raw))}

class ExportTests(unittest.TestCase):
    def graph(self, license_fault=False):
        reference = {'DataProviders': [{'ID': i, 'License': text} for i, text in export.LICENSES.items()]}
        if license_fault: reference['DataProviders'][0]['License']='restricted'
        refraw = json.dumps(reference).encode()
        poi = json.dumps({'ID':90001,'DataProviderID':1,'AddressInfo':{'CountryID':36,'Title':'[P25-TEST] synthetic site'},'Connections':[{'ID':90002}]}).encode()
        entry = lambda path, raw: dict(path=path, mode='100644', type='blob', sha=export.git_hash('blob',raw), size=len(raw))
        tree = lambda entries: dict(sha=export.tree_hash(entries), tree=entries, truncated=False)
        br = tree([entry('OCM-90001.json',poi)])
        data = tree([dict(path='BR',mode='040000',type='tree',sha=br['sha']),entry('referencedata.json',refraw)])
        root = tree([dict(path='data',mode='040000',type='tree',sha=data['sha'])])
        graph = {'commit':{'sha':export.COMMIT,'tree':{'sha':root['sha']},'committer':{'date':'2026-04-22T10:59:34Z'}},root['sha']:root,data['sha']:data,br['sha']:br,'referencedata.json':refraw,'OCM-90001.json':poi}
        return graph

    def test_exact_country_graph_hashes_counts_and_private_output(self):
        graph = self.graph()
        calls = []
        def fetch(req):
            url = req.full_url
            self.assertNotIn('recursive=1',url)
            calls.append(url)
            key = 'commit' if '/commits/' in url else url.rsplit('/',1)[-1]
            value = graph[key]
            return Response(value if isinstance(value,bytes) else json.dumps(value).encode())
        with tempfile.TemporaryDirectory() as root:
            path=Path(root)/'snapshot'
            pin=export.collect(path,fetch)
            self.assertEqual(1,pin['rows'])
            self.assertEqual(1,pin['connection_groups'])
            self.assertEqual(0o700,path.stat().st_mode&0o777)
            self.assertEqual(0o600,(path/'pois/OCM-90001.json').stat().st_mode&0o777)
            self.assertEqual(export.COMMIT,pin['commit'])
            with self.assertRaises(FileExistsError): export.collect(path,fetch)
        self.assertEqual(6,len(calls))

    def test_truncated_changed_blob_foreign_country_and_rights_refused(self):
        for fault in ['truncated','blob','foreign','rights','license']:
            graph=self.graph(fault=='license')
            br=next(v for v in graph.values() if isinstance(v,dict) and isinstance(v.get('tree'),list) and v['tree'][0]['path']=='OCM-90001.json')
            if fault=='truncated': br['truncated']=True
            if fault=='blob': graph['OCM-90001.json']+=b' '
            if fault in ['foreign','rights']:
                # Content changes remain hash-bound: rebuild all containing trees.
                poi=json.loads(graph['OCM-90001.json'])
                if fault=='foreign': poi['AddressInfo']['CountryID']=1
                else: poi['DataProviderID']=999
                raw=json.dumps(poi).encode()
                graph['OCM-90001.json']=raw
                old=br['sha'];br['tree'][0].update(sha=export.git_hash('blob',raw),size=len(raw));br['sha']=export.tree_hash(br['tree']);graph[br['sha']]=br
                data=next(v for v in graph.values() if isinstance(v,dict) and isinstance(v.get('tree'),list) and v['tree'][0]['path']=='BR')
                old_data=data['sha'];data['tree'][0]['sha']=br['sha'];data['sha']=export.tree_hash(data['tree']);graph[data['sha']]=data
                root_tree=graph[graph['commit']['tree']['sha']];root_tree['tree'][0]['sha']=data['sha'];root_tree['sha']=export.tree_hash(root_tree['tree']);graph[root_tree['sha']]=root_tree;graph['commit']['tree']['sha']=root_tree['sha']
            def fetch(req):
                key='commit' if '/commits/' in req.full_url else req.full_url.rsplit('/',1)[-1]
                v=graph[key];return Response(v if isinstance(v,bytes) else json.dumps(v).encode())
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as root:
                path=Path(root)/'snapshot'
                with self.assertRaises(export.SourceError): export.collect(path,fetch)
                self.assertFalse(path.exists())

class SourcePinTests(unittest.TestCase):
    def test_new_national_profile_binds_exact_licenses_without_rewriting_old_identity(self):
        root = Path(export.__file__).resolve().parents[3]
        profile = json.loads((root/'contracts/ev/national-source-pins-20261103-v1.json').read_bytes())
        self.assertEqual('PRIVATE_PREPARATION',profile['exposure'])
        ocm = profile['sources']['OCM']
        self.assertEqual(export.COMMIT,ocm['source_revision'])
        self.assertEqual('DATED_EXPORT_NOT_LIVE_STATUS',ocm['freshness'])
        self.assertEqual(set(export.LICENSES),{p['id'] for p in ocm['providers']})
        for provider in ocm['providers']:
            self.assertEqual(export.LICENSES[provider['id']],provider['license'])
            self.assertEqual(hashlib.sha256(provider['license'].encode()).hexdigest(),provider['license_sha256'])
        old = json.loads((root/'contracts/ev/preparation-profiles-v1.json').read_bytes())['profiles'][0]
        self.assertEqual('ocm-br-20261021-v1',old['id'])
        self.assertNotEqual(old['rights']['providers'][2]['provider_license_sha256'],ocm['providers'][2]['license_sha256'])
