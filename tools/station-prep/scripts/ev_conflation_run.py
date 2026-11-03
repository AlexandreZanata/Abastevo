#!/usr/bin/env python3
"""Measured private conflation of owner-approved normalized v2 source artifacts."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
from ev_national_run import PROFILE_FILE, RunError, require_local_environment, run_process
from ev_national_sources import SourceError, new_directory, outside_checkout, write_json

FILES={'catalog.ndjson','pair-review.ndjson','conflicts.ndjson','hierarchy-review.ndjson',
       'equipment-assignments.ndjson','uf-census.json','owner-review-sample.json'}
def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ['binary','normalized','manifest-sha256','boundaries','output']:parser.add_argument('--'+name,required=True)
    args=parser.parse_args();require_local_environment();os.umask(0o077)
    profile=json.loads(PROFILE_FILE.read_bytes())
    binary=Path(args.binary).resolve(strict=True)
    normalized=outside_checkout(args.normalized).resolve(strict=True)
    boundary=outside_checkout(args.boundaries).resolve(strict=True)
    if hashlib.sha256((normalized/'manifest.json').read_bytes()).hexdigest()!=args.manifest_sha256:
        raise RunError('input manifest differs')
    root=new_directory(args.output);generation=root/'generation'
    measurements=run_process([str(binary),'--normalized',str(normalized),'--manifest-sha256',args.manifest_sha256,
                             '--boundaries',str(boundary),'--output',str(generation)],
                            root,[boundary,*normalized.rglob('*')],profile['resources'])
    manifest_raw=(generation/'manifest.json').read_bytes();manifest=json.loads(manifest_raw)
    expected={'id':'ev-conflation-v1','sha256':hashlib.sha256((PROFILE_FILE.parent/'conflation-profile-v1.json').read_bytes()).hexdigest(),'exposure':'PRIVATE_PREPARATION'}
    if manifest.get('format')!='abastevo.ev-conflated-v1' or manifest.get('conflation_profile')!=expected or manifest.get('input_manifest_sha256')!=args.manifest_sha256 or set(manifest.get('files',{}))!=FILES:
        raise RunError('conflation manifest/profile/file set differs')
    for name,expected in manifest['files'].items():
        raw=(generation/name).read_bytes()
        if len(raw)!=expected['bytes'] or hashlib.sha256(raw).hexdigest()!=expected['sha256']:
            raise RunError('conflated artifact hash differs')
    receipt={'status':'LOCAL_CONFLATED_OWNER_REVIEW_AND_IMPORT_PENDING','manifest_sha256':hashlib.sha256(manifest_raw).hexdigest(),
             'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'input_manifest_sha256':args.manifest_sha256,
             'profile':manifest['conflation_profile'],'stats':manifest['stats'],'measurements':measurements,'limits':profile['resources']}
    write_json(root/'execution-receipt.json',receipt);print(json.dumps(receipt,separators=(',',':')))
if __name__=='__main__':
    try:main()
    except (RunError,SourceError,OSError,ValueError,KeyError):
        print('private conflation failed; inspect local private logs',file=sys.stderr);raise SystemExit(2)
