#!/usr/bin/env python3
"""Reuse only a fully verified immutable publication for a same-head retry."""
import argparse
import json
import os
from pathlib import Path
import subprocess
from artifact import LICENSE_FILES, verify_apk
from publish import release_or_none, verify_tag, verify_local, same_asset
from release_flow import REPOSITORY


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--version',type=Path,required=True)
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--build-tools',type=Path,required=True)
    args=parser.parse_args()
    version=json.loads(args.version.read_text())
    release=release_or_none(version['tag'])
    exists=release is not None and not release['draft']
    if exists:
        verify_tag(version['tag'],version['commit'])
        apk=f"Abastevo-{version['version_name']}-android.apk"
        expected=set(LICENSE_FILES)|{apk,'release.json','SHA256SUMS','signing-certificate.pem'}
        assets={a['name']:a for a in release['assets']}
        if not release['prerelease'] or len(assets)!=len(release['assets']) or set(assets)!=expected:
            raise ValueError('existing publication is incomplete or incompatible')
        args.output.mkdir(parents=True,exist_ok=False)
        for name in sorted(expected):
            subprocess.run(['gh','release','download',version['tag'],'--repo',REPOSITORY,'--pattern',name,'--dir',str(args.output)],check=True)
        manifest=verify_local(args.output)
        if any(manifest.get(k)!=v for k,v in version.items()) or any(not same_asset(args.output/name,asset) for name,asset in assets.items()):
            raise ValueError('existing release identity or digest mismatch')
        certificate=json.loads(Path('release/android/signing-certificate.json').read_text())
        if manifest['certificate_sha256']!=certificate['certificate_sha256']:
            raise ValueError('existing certificate mismatch')
        verify_apk(args.output/apk,version,certificate,args.build_tools/'aapt',args.build_tools/'apksigner')
    with open(os.environ['GITHUB_OUTPUT'],'a',encoding='utf-8') as output:
        output.write(f'existing={str(exists).lower()}\n')
    print('Reused verified same-head Android release' if exists else 'A new signed build is required')


if __name__=='__main__':main()
