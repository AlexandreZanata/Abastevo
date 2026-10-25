#!/usr/bin/env python3
"""Package only the exact admitted map resources, preserving release code bytes."""

import argparse
import copy
import re
import xml.etree.ElementTree as ET
import hashlib
from io import BytesIO
import json
from pathlib import Path, PurePosixPath
import tempfile
import zipfile


LEGAL_PREFIX = 'assets/abastevo-map-licenses/'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def put(archive, name, raw):
    info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
    info.compress_type = zipfile.ZIP_DEFLATED
    info.external_attr = 0o100644 << 16
    archive.writestr(info, raw, compresslevel=9)


def validate(entries, profile):
    expected = {row['path']: row for row in profile['resources']}
    actual = {name: raw for name, raw in entries.items() if name.startswith('assets/')}
    if actual.keys() != expected.keys():
        raise ValueError('unknown or missing resource; review the exact asset profile')
    for name, row in expected.items():
        if row['admission'] not in ('ADMITTED', 'EXCLUDED'):
            raise ValueError('unknown admission decision')
        if sha(actual[name]) != row['sha256']:
            raise ValueError('resource digest mismatch: ' + name)


def complete_locale_fallbacks(entries):
    """Make SDK default-language fallback explicit without replacing translations."""
    groups = {}
    for name, raw in entries.items():
        parts = PurePosixPath(name).parts
        if len(parts) != 3 or parts[0] != 'res' or not parts[1].startswith('values') or not name.endswith('.xml'):
            continue
        values = groups.setdefault(parts[1], {})
        for node in ET.fromstring(raw):
            if node.tag in ('string', 'plurals', 'string-array') and node.get('name') and node.get('translatable') != 'false':
                values[node.get('name')] = node
    defaults = groups.get('values', {})
    generated = {}
    for folder, localized in sorted(groups.items()):
        if not re.fullmatch(r'values-(?:[a-z]{2,3}(?:-r[A-Z]{2})?|b[+][A-Za-z0-9+]+)', folder):
            continue
        language = folder.split('-')[1].split('+')[1] if folder.startswith('values-b+') else folder.split('-')[1]
        parent = groups.get('values-' + language, {})
        fallback = ET.Element('resources')
        for name, default in sorted(defaults.items()):
            if name not in localized:
                fallback.append(copy.deepcopy(parent.get(name, default)))
        if len(fallback):
            path = 'res/' + folder + '/abastevo_fallbacks.xml'
            if path in entries:
                raise ValueError('unexpected generated fallback resource')
            entries[path] = ET.tostring(fallback, encoding='utf-8', xml_declaration=True)
            generated[path] = {'entries': len(fallback), 'sha256': sha(entries[path])}
    return generated


def prepare(aar, profile_path, notices, output):
    if output.exists():
        raise ValueError('output already exists')
    profile = json.loads(profile_path.read_text())
    if profile.get('format') != 'abastevo.map-resources-v1':
        raise ValueError('unsupported resource profile')
    with zipfile.ZipFile(aar) as src:
        names = [i.filename for i in src.infolist() if not i.is_dir()]
        if len(names) != len(set(names)):
            raise ValueError('duplicate archive entries')
        for name in names:
            if PurePosixPath(name).is_absolute() or '..' in PurePosixPath(name).parts:
                raise ValueError('unsafe archive entry')
        entries = {name: src.read(name) for name in names}
    validate(entries, profile)
    removed = []
    for row in profile['resources']:
        if row['admission'] == 'EXCLUDED':
            removed.append({'path': row['path'], 'bytes': len(entries.pop(row['path'])),
                            'reason': row.get('reason', 'not admitted')})
    fallbacks = complete_locale_fallbacks(entries)
    bundle = BytesIO()
    notice_files = sorted(p for p in notices.rglob('*') if p.is_file())
    if not notice_files:
        raise ValueError('retained license notices are required')
    with zipfile.ZipFile(bundle, 'w') as dst:
        for p in notice_files:
            put(dst, str(p.relative_to(notices)), p.read_bytes())
    entries[LEGAL_PREFIX + 'notices.zip'] = bundle.getvalue()
    record = {'format': 'abastevo.admitted-resources-v1',
              'upstream_sha': profile['upstream_sha'], 'profile_sha256': sha(profile_path.read_bytes()),
              'resources': [row for row in profile['resources'] if row['admission'] == 'ADMITTED'],
              'excluded': removed, 'locale_fallbacks': fallbacks, 'notices_sha256': sha(bundle.getvalue())}
    entries[LEGAL_PREFIX + 'resources.json'] = (json.dumps(record, ensure_ascii=False, sort_keys=True, indent=2)+'\n').encode()
    with tempfile.NamedTemporaryFile(dir=output.parent, prefix='.admitted-', delete=False) as tmp:
        temporary = Path(tmp.name)
    try:
        with zipfile.ZipFile(temporary, 'w') as dst:
            for name, raw in sorted(entries.items()):
                put(dst, name, raw)
        # Refuse overwrite even if another publisher won after the initial check.
        import os
        os.link(temporary, output)
    finally:
        temporary.unlink(missing_ok=True)
    return record


def verify(candidate, profile_path):
    profile = json.loads(profile_path.read_text())
    folder = candidate / 'assets/abastevo-map-licenses'
    record = json.loads((folder / 'resources.json').read_text())
    if record['profile_sha256'] != sha(profile_path.read_bytes()):
        raise ValueError('unreviewed resource profile')
    expected = {row['path']: row for row in profile['resources'] if row['admission'] == 'ADMITTED'}
    actual = {str(p.relative_to(candidate)): p for p in (candidate/'assets').rglob('*')
              if p.is_file() and not p.is_relative_to(folder)}
    if actual.keys() != expected.keys():
        raise ValueError('unknown or missing admitted asset')
    if any(sha(p.read_bytes()) != expected[name]['sha256'] for name, p in actual.items()):
        raise ValueError('admitted resource bytes changed')
    if sha((folder/'notices.zip').read_bytes()) != record['notices_sha256']:
        raise ValueError('retained notices changed')
    for name, row in record.get('locale_fallbacks', {}).items():
        if sha((candidate/name).read_bytes()) != row['sha256']:
            raise ValueError('generated locale fallback changed')
    return record


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--aar', type=Path, required=True)
    parser.add_argument('--profile', type=Path, required=True)
    parser.add_argument('--notices', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(prepare(args.aar, args.profile, args.notices, args.output), indent=2))
    except (ValueError, OSError, KeyError, zipfile.BadZipFile) as error:
        parser.exit(1, f'Map resource admission refused: {error}\n')
