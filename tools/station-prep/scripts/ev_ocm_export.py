#!/usr/bin/env python3
"""Private, bounded Brazil-only acquisition of the owner-approved dated OCM export.

GitHub connector Contents output is capped at 1,000 files. The local transfer
therefore follows Git trees without recursive/truncated listings and keeps raw
contact-bearing payload outside the model transcript, Git and product artifacts.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import urllib.parse
import urllib.request
from ev_national_sources import SourceError, new_directory, transport, utc_now, write_json

COMMIT = '8e3bedca48ca94807d96b2f5e7ee02cffe63ae54'
API = 'https://api.github.com/repos/openchargemap/ocm-export/git/'
RAW = f'https://raw.githubusercontent.com/openchargemap/ocm-export/{COMMIT}/data/'
LICENSES = {1:'Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0)',40:'CC-0',57:'CC0'}
MAX_FILES = 5000
MAX_FILE = 1 << 20
MAX_TOTAL = 128 << 20


def git_hash(kind, raw):
    return hashlib.sha1(f'{kind} {len(raw)}\0'.encode()+raw).hexdigest()


def tree_hash(entries):
    raw = b''
    for entry in sorted(entries,key=lambda x: x['path']+('/' if x['type']=='tree' else '')):
        path=entry['path']
        if '/' in path or '\0' in path or path in ('.','..'):
            raise SourceError('unsafe Git tree entry')
        if not re.fullmatch('[0-9a-f]{40}',entry['sha']):
            raise SourceError('invalid Git identity')
        raw += entry['mode'].lstrip('0').encode()+b' '+path.encode()+b'\0'+bytes.fromhex(entry['sha'])
    return git_hash('tree',raw)


def request(url):
    if not (re.fullmatch(re.escape(API)+r'(commits|trees)/[0-9a-f]{40}',url)
            or re.fullmatch(re.escape(RAW)+r'(referencedata\.json|BR/OCM-[0-9]+\.json)',url)):
        raise SourceError('unapproved pinned export URL')
    return urllib.request.Request(url,headers={'User-Agent':'Abastevo-EVC01-local/1','Accept':'application/vnd.github+json'})


def read(url, maximum, fetch):
    with fetch(request(url)) as response:
        raw=response.read(maximum+1)
        declared=response.headers.get('Content-Length')
        if len(raw)>maximum or (declared is not None and int(declared)!=len(raw)):
            raise SourceError('export response byte budget/truncation refused')
        return raw


def tree(sha, fetch):
    result=json.loads(read(API+'trees/'+sha,8<<20,fetch))
    if result.get('truncated') is not False or result.get('sha')!=sha:
        raise SourceError('truncated or changed Git tree')
    entries=result['tree']
    if not isinstance(entries,list) or len(entries)>MAX_FILES or tree_hash(entries)!=sha:
        raise SourceError('Git tree hash/budget mismatch')
    if len({x['path'] for x in entries})!=len(entries):
        raise SourceError('duplicate Git tree path')
    return entries


def child(entries, name, kind):
    matches=[x for x in entries if x['path']==name and x['type']==kind]
    if len(matches)!=1:
        raise SourceError('required pinned tree path missing')
    return matches[0]


def collect(output, fetch=transport):
    root=new_directory(output)
    try:
        commit=json.loads(read(API+'commits/'+COMMIT,1<<20,fetch))
        if commit.get('sha')!=COMMIT:
            raise SourceError('changed export commit')
        root_tree=tree(commit['tree']['sha'],fetch)
        data=tree(child(root_tree,'data','tree')['sha'],fetch)
        br_id=child(data,'BR','tree')['sha']
        entries=tree(br_id,fetch)
        if not entries or any(x['type']!='blob' or x['mode']!='100644' or not re.fullmatch(r'OCM-[0-9]+\.json',x['path']) or not 0<x['size']<=MAX_FILE for x in entries):
            raise SourceError('Brazil file inventory refused')
        if sum(x['size'] for x in entries)>MAX_TOTAL:
            raise SourceError('Brazil aggregate byte budget exceeded')
        reference=read(RAW+'referencedata.json',8<<20,fetch)
        if git_hash('blob',reference)!=child(data,'referencedata.json','blob')['sha']:
            raise SourceError('reference Git blob mismatch')
        providers={row['ID']:row.get('License') for row in json.loads(reference)['DataProviders']}
        if any(providers.get(i)!=text for i,text in LICENSES.items()):
            raise SourceError('OCM provider license changed')
        (root/'pois').mkdir(mode=0o700)
        (root/'referencedata.json').write_bytes(reference)
        (root/'referencedata.json').chmod(0o600)
        def download(entry):
            raw=read(RAW+'BR/'+entry['path'],MAX_FILE,fetch)
            if len(raw)!=entry['size'] or git_hash('blob',raw)!=entry['sha']:
                raise SourceError('POI Git blob mismatch')
            poi=json.loads(raw)
            if poi.get('ID')!=int(entry['path'][4:-5]) or poi.get('AddressInfo',{}).get('CountryID')!=36:
                raise SourceError('foreign or changed OCM POI identity')
            provider=poi.get('DataProviderID')
            if provider not in LICENSES:
                raise SourceError('unadmitted provider in export')
            path=root/'pois'/entry['path']
            with open(path,'xb',opener=lambda p,f:os.open(p,f,0o600)) as stream:
                stream.write(raw)
            return {'file':entry['path'],'git_blob_sha':entry['sha'],'bytes':len(raw),
                    'sha256':hashlib.sha256(raw).hexdigest(),'provider_id':provider,
                    'connection_groups':len(poi.get('Connections') or [])}
        with ThreadPoolExecutor(max_workers=4) as pool:
            inventory=list(pool.map(download,sorted(entries,key=lambda x:x['path'])))
        write_json(root/'source-inventory.json',inventory)
        pin={'profile':'ev-ocm-export-acquisition-v1','source':'OCM','country':'BR',
             'commit':COMMIT,'source_date':commit['committer']['date'],'country_tree_sha':br_id,
             'acquired_at':utc_now(),'rows':len(inventory),'bytes':sum(x['bytes'] for x in inventory),
             'connection_groups':sum(x['connection_groups'] for x in inventory),
             'provider_counts':{str(i):sum(x['provider_id']==i for x in inventory) for i in LICENSES},
             'reference_sha256':hashlib.sha256(reference).hexdigest(),
             'inventory_sha256':hashlib.sha256((root/'source-inventory.json').read_bytes()).hexdigest(),
             'provider_license_sha256':{str(i):hashlib.sha256(v.encode()).hexdigest() for i,v in LICENSES.items()},
             'freshness':'DATED_EXPORT_NOT_LIVE_STATUS','complete_country_tree':True}
        write_json(root/'source-pin.json',pin)
        return pin
    except Exception:
        shutil.rmtree(root)
        raise


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',required=True)
    args=parser.parse_args()
    modes=[os.environ.get(k,'').lower() for k in ['ANPFUEL_ENV','ANPFUEL_RUN_ENV','DEPLOY_ENV']]
    if any(x in ('prod','production','stage','staging') for x in modes) or os.environ.get('KUBERNETES_SERVICE_HOST'):
        raise SourceError('export acquisition is local-only')
    os.umask(0o077)
    print(json.dumps(collect(args.output),separators=(',',':')))

if __name__=='__main__':
    try:
        main()
    except (SourceError,OSError,ValueError,KeyError,TypeError):
        raise SystemExit('pinned export acquisition failed; private source details withheld') from None
