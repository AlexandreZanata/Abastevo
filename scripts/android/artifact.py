#!/usr/bin/env python3
"""Verify an Android release, then package an immutable public release bundle."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import zipfile

MAX_APK_BYTES = 80 * 1024 * 1024
LICENSE_FILES = {
    'Abastevo-LICENSE.txt': 'LICENSE',
    'MIT-original.txt': 'LICENSES/MIT-original.txt',
    'THIRD_PARTY_NOTICES.md': 'THIRD_PARTY_NOTICES.md',
    'CoMaps-LICENSE.txt': 'maps/engine/vendor/LICENSE',
    'CoMaps-NOTICE.txt': 'maps/engine/vendor/NOTICE',
    'CoMaps-THIRD_PARTY_NOTICES.md': 'docs/backend/evidence/maps/map14-t07/THIRD_PARTY_NOTICES.md',
}


def digest(path):
    with open(path, 'rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def verify_apk(apk, version, certificate, aapt, apksigner):
    if not 0 < apk.stat().st_size <= MAX_APK_BYTES:
        raise ValueError('release APK exceeds its admitted size budget')
    badging = subprocess.check_output([str(aapt), 'dump', 'badging', str(apk)], text=True)
    identity = re.search(r"package: name='([^']+)' versionCode='([0-9]+)' versionName='([^']+)'", badging)
    if not identity or identity.groups() != (version['application_id'], str(version['version_code']), version['version_name']):
        raise ValueError('APK identity/version mismatch')
    if 'application-debuggable' in badging:
        raise ValueError('debuggable APK cannot be released')
    signature = subprocess.check_output([str(apksigner), 'verify', '--verbose', '--print-certs', str(apk)], text=True)
    certs = re.findall(r'Signer #[0-9]+ certificate SHA-256 digest: ([a-fA-F0-9]+)', signature)
    if [c.lower() for c in certs] != [certificate['certificate_sha256']]:
        raise ValueError('APK signing identity mismatch')
    with zipfile.ZipFile(apk) as archive:
        names = archive.namelist()
        abis = {name.split('/')[1] for name in names if re.fullmatch(r'lib/[^/]+/[^/]+\.so', name)}
        if not abis or any(f'lib/{abi}/liborganicmaps.so' not in names for abi in abis):
            raise ValueError('APK native map engine missing')
        for name, source in LICENSE_FILES.items():
            if archive.read('assets/licenses/' + name) != Path(source).read_bytes():
                raise ValueError('APK license bundle mismatch')
        if any(re.search(r'(?i)(\.p12$|\.jks$|signing\.properties$|credentials\.json$)', name) for name in names):
            raise ValueError('private signing material found inside APK')
    return sorted(abis)


def bundle(apk, version, certificate, output, aapt, apksigner):
    abis = verify_apk(apk, version, certificate, aapt, apksigner)
    output.mkdir(parents=True, exist_ok=False)
    target = output / f"Abastevo-{version['version_name']}-android.apk"
    shutil.copyfile(apk, target)
    for name, source in LICENSE_FILES.items():
        shutil.copyfile(source, output / name)
    shutil.copyfile('release/android/signing-certificate.pem', output / 'signing-certificate.pem')
    files = {p.name: {'sha256': digest(p), 'bytes': p.stat().st_size} for p in sorted(output.iterdir())}
    manifest = {**version, 'certificate_sha256': certificate['certificate_sha256'], 'native_abis': abis,
                'origin': 'https://teste.abastevo.com.br', 'files': files}
    (output / 'release.json').write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
    (output / 'SHA256SUMS').write_text(''.join(f'{digest(p)}  {p.name}\n' for p in sorted(output.iterdir())))
    return manifest


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--apk', type=Path, required=True)
    parser.add_argument('--version', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--build-tools', type=Path, required=True)
    args = parser.parse_args()
    manifest = bundle(args.apk, json.loads(args.version.read_text()), json.loads(Path('release/android/signing-certificate.json').read_text()),
                      args.output, args.build_tools / 'aapt', args.build_tools / 'apksigner')
    print(f"Verified signed Android {manifest['version_name']} (code {manifest['version_code']})")


if __name__ == '__main__':
    main()
