#!/usr/bin/env python3
"""Owner-only measured normalization of already-approved local source files."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
from ev_national_run import PROFILE_FILE, RunError, require_local_environment, run_process
from ev_national_sources import SourceError, new_directory, outside_checkout, write_json

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ['binary','osm','ocm','boundaries','output']:parser.add_argument('--'+name,required=True)
    args=parser.parse_args()
    require_local_environment()
    os.umask(0o077)
    profile=json.loads(PROFILE_FILE.read_bytes())
    binary=Path(args.binary).resolve(strict=True)
    osm=outside_checkout(args.osm).resolve(strict=True)
    ocm=outside_checkout(args.ocm).resolve(strict=True)
    boundaries=outside_checkout(args.boundaries).resolve(strict=True)
    root=new_directory(args.output)
    index=osm.with_suffix('.pif')
    extras=[osm,index,index.with_suffix('.pif.tmp'),boundaries,*ocm.rglob('*')]
    generation=root/'generation'
    measurements=run_process([str(binary),'--osm',str(osm),'--ocm',str(ocm),
                              '--boundaries',str(boundaries),'--output',str(generation)],
                             root,extras,profile['resources'])
    manifest_raw=(generation/'manifest.json').read_bytes()
    manifest=json.loads(manifest_raw)
    pins_raw=(PROFILE_FILE.parent/'national-source-pins-20261103-v1.json').read_bytes()
    expected_profile={'profile_id':json.loads(pins_raw)['profile_id'],'sha256':hashlib.sha256(pins_raw).hexdigest(),'exposure':'PRIVATE_PREPARATION'}
    normalization_hash=hashlib.sha256((PROFILE_FILE.parent/'normalization-profile-v1.json').read_bytes()).hexdigest()
    if manifest.get('normalization_profile_sha256')!=normalization_hash:
        raise RunError('normalization profile differs')
    if manifest.get('format')!='abastevo.ev-source-sites-v1' or manifest.get('source_profile')!=expected_profile:
        raise RunError('binary and source profile differ')
    if set(manifest.get('files',{}))!={'sites.ndjson','decisions.ndjson','issues.ndjson','uf-census.json'}:
        raise RunError('incomplete normalized generation')
    for name,expected in manifest['files'].items():
        if Path(name).name!=name:raise RunError('unsafe manifest filename')
        raw=(generation/name).read_bytes()
        if len(raw)!=expected['bytes'] or hashlib.sha256(raw).hexdigest()!=expected['sha256']:
            raise RunError('normalized artifact hash mismatch')
    receipt={'status':'LOCAL_NORMALIZED_NOT_CONFLATED_OR_IMPORTED','manifest_sha256':hashlib.sha256(manifest_raw).hexdigest(),
             'source_profile':manifest['source_profile'],'summary':manifest['summary'],
             'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),
             'measurements':measurements,'limits':profile['resources']}
    write_json(root/'execution-receipt.json',receipt)
    print(json.dumps(receipt,separators=(',',':')))

if __name__=='__main__':
    try:main()
    except (RunError,SourceError,OSError,ValueError,KeyError):
        print('local normalization failed; inspect private working logs locally',file=sys.stderr)
        raise SystemExit(2)
