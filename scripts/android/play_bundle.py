#!/usr/bin/env python3
"""Verify a signed Android App Bundle before Google Play upload."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from artifact import LICENSE_FILES

MAX_AAB_BYTES = 500 * 1024 * 1024
PRIVATE_MATERIAL_RE = re.compile(r'(?i)(\.p12$|\.jks$|signing\.properties$|credentials\.json$)')


def verify_aab_signature(aab: Path) -> None:
    """Require a valid jarsigner signature on the AAB."""
    result = subprocess.run(
        ['jarsigner', '-verify', '-verbose', '-certs', str(aab)],
        capture_output=True,
        text=True,
        check=False,
    )
    output = (result.stdout or '') + '\n' + (result.stderr or '')
    if result.returncode != 0 or 'jar verified.' not in output or 'jar is unsigned' in output.lower():
        raise ValueError('AAB signature verification failed')


def verify_aab(aab: Path, version: dict) -> list[str]:
    """Validate Play Store bundle identity, signature, native engine, licenses and hygiene."""
    if not aab.is_file() or not 0 < aab.stat().st_size <= MAX_AAB_BYTES:
        raise ValueError('release AAB missing or exceeds 500 MiB Play Store limit')
    verify_aab_signature(aab)
    with zipfile.ZipFile(aab) as archive:
        names = archive.namelist()
        if 'base/manifest/AndroidManifest.xml' not in names:
            raise ValueError('AAB base manifest missing')
        manifest_bytes = archive.read('base/manifest/AndroidManifest.xml')
        app_id = version['application_id']
        if app_id.encode('utf-8') not in manifest_bytes:
            raise ValueError(f'AAB manifest missing application_id {app_id}')
        if f'{app_id}.fileprovider'.encode('utf-8') not in manifest_bytes:
            raise ValueError(f'AAB manifest authority mismatch for {app_id}')
        if version['version_name'].encode('utf-8') not in manifest_bytes:
            raise ValueError('AAB manifest versionName mismatch')
        abis = {
            name.split('/')[2]
            for name in names
            if re.fullmatch(r'base/lib/[^/]+/[^/]+\.so', name)
        }
        if not abis or any(f'base/lib/{abi}/liborganicmaps.so' not in names for abi in abis):
            raise ValueError('AAB native map engine missing')
        for name, source in LICENSE_FILES.items():
            entry = f'base/assets/licenses/{name}'
            if entry not in names or archive.read(entry) != Path(source).read_bytes():
                raise ValueError(f'AAB license bundle mismatch: {name}')
        if any(PRIVATE_MATERIAL_RE.search(name) for name in names):
            raise ValueError('private signing material found inside AAB')
    return sorted(abis)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument('--aab', type=Path, required=True)
    parser.add_argument('--version', type=Path, required=True)
    args = parser.parse_args()
    version = json.loads(args.version.read_text(encoding='utf-8'))
    abis = verify_aab(args.aab, version)
    print(
        f"Verified signed Play AAB {version['application_id']} "
        f"{version['version_name']} (code {version['version_code']}, abis={','.join(abis)})"
    )


if __name__ == '__main__':
    main()
