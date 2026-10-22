#!/usr/bin/env python3
"""EE-03 local contract oracle; not an EE-06 bridge or source admission tool.

No network, source parsing, DB, row emission or publication. A successful result
certifies only the prepared envelope. Future preparers must independently census
all raw source units and match these decisions before atomically publishing.
"""
import argparse
from collections import Counter
from datetime import datetime
from decimal import Decimal
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import struct

ROOT = Path(__file__).resolve().parents[1]
SCHEMA = ROOT / 'contracts/openapi/ev-preparation-planned.json'
REGISTRY = ROOT / 'contracts/ev/preparation-profiles-v1.json'
FILES = ('entities.ndjson', 'decisions.ndjson', 'aliases.ndjson')
KINDS = ('PLACE', 'EVSE', 'CONNECTOR', 'CONNECTION_GROUP', 'VEHICLE_REFERENCE')
PARENT = {'PLACE': None, 'EVSE': 'PLACE', 'CONNECTOR': 'EVSE',
          'CONNECTION_GROUP': 'PLACE', 'VEHICLE_REFERENCE': None}
MAX_BYTES = 100 << 20
MAX_RECORD = 1 << 20
MAX_ROWS = 500_000


class ContractError(ValueError):
    """Messages contain reason codes only, never untrusted private data."""


def require(ok, reason):
    if not ok:
        raise ContractError(reason)


def canonical(value):
    def bounded(v, depth=0):
        require(depth <= 32, 'json-depth')
        if isinstance(v, dict):
            for k, x in v.items():
                require(isinstance(k, str) and k.isascii(), 'json-key')
                bounded(k, depth + 1)
                bounded(x, depth + 1)
        elif isinstance(v, list):
            for x in v:
                bounded(x, depth + 1)
        elif isinstance(v, str):
            require(len(v.encode('utf-8')) <= 64 << 10, 'json-string')
        elif type(v) is int:
            require(abs(v) <= 9007199254740991, 'json-integer')
        else:
            require(v is None or type(v) is bool, 'json-exact-type')
    try:
        bounded(value)
        raw = json.dumps(value, ensure_ascii=False, sort_keys=True,
                         separators=(',', ':'), allow_nan=False).encode('utf-8')
        require(len(raw) <= MAX_RECORD, 'json-record-size')
        return raw
    except (UnicodeError, RecursionError) as exc:
        raise ContractError('json-encoding-or-depth') from exc


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def source_key(namespace, kind, external_id):
    require(re.fullmatch('[a-z0-9-]{1,64}', namespace) is not None
            and kind in KINDS and isinstance(external_id, str)
            and 1 <= len(external_id.encode('utf-8')) <= 256
            and not any(ord(c) < 32 or 127 <= ord(c) <= 159 for c in external_id),
            'source-identity')
    result = bytearray()
    for field in ('ev-source-key-v1', namespace, kind, external_id):
        data = field.encode('utf-8')
        result += struct.pack('>I', len(data)) + data
    return sha(result)


def parse(raw):
    def pairs(items):
        d = {}
        for k, v in items:
            require(k not in d, 'json-duplicate-key')
            d[k] = v
        return d
    def no_float(_):
        raise ContractError('json-float-or-constant')
    try:
        value = json.loads(raw.decode('utf-8'), object_pairs_hook=pairs,
                           parse_float=no_float, parse_constant=no_float)
        canonical(value)
        return value
    except ContractError:
        raise
    except (UnicodeError, ValueError, RecursionError) as exc:
        raise ContractError('json-invalid-or-truncated') from exc


def read_file(path, cap):
    # Refuse non-regular files and symlinks, including replacement during open.
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as stream:
            info = os.fstat(stream.fileno())
            require(stat.S_ISREG(info.st_mode) and info.st_size <= cap, 'file-boundary')
            raw = stream.read(cap + 1)
            require(len(raw) <= cap, 'file-size')
            return raw
    except OSError as exc:
        raise ContractError('file-unavailable') from exc


def shape(schema, value, schemas):
    """The deliberately small OpenAPI3 subset used by this frozen fragment.

    Go's independent kin-openapi test validates the same schemas/fixtures. This
    is a local protocol oracle, not a general-purpose OpenAPI validator.
    """
    if '$ref' in schema:
        return shape(schemas[schema['$ref'].split('/')[-1]], value, schemas)
    if value is None and schema.get('nullable'):
        return
    if 'oneOf' in schema:
        matches = 0
        for alternative in schema['oneOf']:
            try:
                shape(alternative, value, schemas)
                matches += 1
            except ContractError:
                pass
        require(matches == 1, 'shape-one-of')
        return
    t = schema['type']
    require({'object': type(value) is dict, 'array': type(value) is list,
             'string': type(value) is str, 'integer': type(value) is int}[t], 'shape-type')
    if 'enum' in schema:
        require(value in schema['enum'], 'shape-enum')
    if t == 'object':
        require(set(value) == set(schema['required']), 'shape-object-fields')
        for k, v in value.items():
            shape(schema['properties'][k], v, schemas)
    elif t == 'array':
        require(len(value) <= schema['maxItems'], 'shape-array-limit')
        for v in value:
            shape(schema['items'], v, schemas)
    elif t == 'string':
        require(schema.get('minLength', 0) <= len(value) <= schema.get('maxLength', MAX_RECORD),
                'shape-string-length')
        if 'pattern' in schema:
            require(re.fullmatch(schema['pattern'], value) is not None, 'shape-string-pattern')
    elif t == 'integer':
        require(schema['minimum'] <= value <= schema['maximum'], 'shape-integer-range')


def assertion_hash(entity):
    return sha(canonical({k: entity[k] for k in
                         ('kind', 'key', 'parent_key', 'payload', 'rights_sha256', 'source_clocks')}))


def verify(directory, registry_path=REGISTRY):
    directory = Path(directory)
    require(directory.is_dir() and not directory.is_symlink(), 'batch-directory')
    require({p.name for p in directory.iterdir()} == {'manifest.json', *FILES}, 'batch-files')
    schemas = parse(read_file(SCHEMA, MAX_RECORD))['components']['schemas']
    registry = parse(read_file(registry_path, MAX_RECORD))
    manifest_raw = read_file(directory / 'manifest.json', MAX_RECORD)
    m = parse(manifest_raw)
    shape(schemas['PreparedManifest'], m, schemas)
    require(manifest_raw == canonical(m) + b'\n', 'manifest-canonical')
    expected_content = dict(m)
    del expected_content['content_sha256']
    require(m['content_sha256'] == sha(canonical(expected_content)), 'manifest-content-hash')
    profiles = [p for p in registry['profiles'] if p['id'] == m['profile_id']]
    require(registry['version'] == 1 and len(profiles) == 1, 'trusted-profile')
    p = profiles[0]
    require(p['source_admission_enabled'] is False, 'admission-not-owned-here')
    require(m['profile_sha256'] == sha(canonical(p)), 'profile-drift')
    require(m['rights_sha256'] == sha(canonical(p['rights'])), 'rights-drift')
    require(m['source_revision'] == p['source_revision']
            and m['reference_sha256'] == p['reference_sha256']
            and m['format'] in p['formats'], 'source-profile-binding')
    for k in ('byte_profile', 'identity_profile', 'privacy_profile'):
        require(m[k] == p[k], 'protocol-profile-binding')
    inputs = {i['file']: i for i in m['inputs']}
    require(len(inputs) == len(m['inputs']) and len(inputs) > 0, 'input-identity')
    require(m['inputs'] == sorted(m['inputs'], key=lambda x: x['file']), 'input-order')
    require(all(n not in ('.', '..') for n in inputs), 'input-name')
    require(sum(i['bytes'] for i in inputs.values()) <= MAX_BYTES
            and sum(i['roots'] for i in inputs.values()) <= MAX_ROWS, 'input-aggregate-limit')
    streams = {}
    total_bytes = 0
    for filename, schema_name, order in zip(FILES,
            ('PreparedEntity', 'PreparedDecision', 'PreparedAlias'), ('key', 'unit', 'alias_key')):
        raw = read_file(directory / filename, MAX_BYTES)
        total_bytes += len(raw)
        require(total_bytes <= MAX_BYTES, 'batch-byte-limit')
        desc = m['artifacts'][filename]
        require(desc['bytes'] == len(raw) and desc['sha256'] == sha(raw), 'artifact-hash-or-length')
        require(not raw or raw.endswith(b'\n'), 'artifact-final-lf')
        lines = raw.split(b'\n')[:-1] if raw else []
        require(len(lines) == desc['records'] and len(lines) <= MAX_ROWS, 'artifact-count')
        rows = []
        previous = None
        for line in lines:
            require(len(line) <= MAX_RECORD, 'line-limit')
            row = parse(line)
            shape(schemas[schema_name], row, schemas)
            require(canonical(row) == line, 'artifact-canonical')
            require(previous is None or previous < row[order], 'artifact-order-or-duplicate')
            previous = row[order]
            rows.append(row)
        streams[filename] = rows
    entities = {e['key']: e for e in streams['entities.ndjson']}
    decisions = {d['unit']: d for d in streams['decisions.ndjson']}
    require(len(decisions) <= MAX_ROWS, 'decision-limit')
    if p['state'] == 'RIGHTS_AND_LAYOUT_PENDING':
        require(not decisions and not entities and not streams['aliases.ndjson']
                and all(i['roots'] == 0 for i in inputs.values()), 'held-source')
    counts = {k: dict(input=0, accepted=0, duplicates=0, quarantined=0) for k in KINDS}
    accepted_keys = set()
    roots = Counter()
    for d in decisions.values():
        kind = d['kind']
        require(kind in p['kinds'], 'profile-kind')
        require(d['source_file'] in inputs and d['root'] < inputs[d['source_file']]['roots'], 'source-unit-root')
        expected_unit = f"{d['source_file']}#{d['root']}{d['pointer']}"
        require(d['unit'] == expected_unit, 'unit-identity')
        counts[kind]['input'] += 1
        counts[kind][{'ACCEPTED': 'accepted', 'DUPLICATE': 'duplicates', 'QUARANTINED': 'quarantined'}[d['outcome']]] += 1
        parent = decisions.get(d['parent_unit'])
        if PARENT[kind] is None:
            require(d['parent_unit'] is None and d['pointer'] == '', 'root-parent')
            roots[d['source_file']] += 1
        else:
            require(parent is not None and parent['kind'] == PARENT[kind]
                    and parent['source_file'] == d['source_file'] and parent['root'] == d['root']
                    and d['pointer'].startswith(parent['pointer'] + '/'), 'nested-parent')
            if parent['outcome'] == 'QUARANTINED':
                require(d['outcome'] == 'QUARANTINED' and d['reason'] == 'PARENT_QUARANTINED', 'quarantined-parent')
        if d['outcome'] == 'QUARANTINED':
            require(all(d[k] is None for k in ('key', 'external_id', 'assertion_sha256', 'duplicate_of'))
                    and d['reason'] is not None, 'quarantine-minimization')
            require((d['reason'] == 'PARENT_QUARANTINED') == (parent is not None and parent['outcome'] == 'QUARANTINED'), 'quarantine-reason')
            continue
        require(d['reason'] is None and d['external_id'] is not None, 'decision-state')
        require(d['key'] == source_key(p['namespace'], kind, d['external_id']), 'stable-key')
        e = entities.get(d['key'])
        require(e is not None and e['kind'] == kind and e['external_id'] == d['external_id']
                and e['assertion_sha256'] == d['assertion_sha256'], 'decision-assertion')
        if parent is not None:
            require(parent['outcome'] != 'QUARANTINED' and e['parent_key'] == parent['key'], 'assertion-parent')
        if d['outcome'] == 'DUPLICATE':
            original = decisions.get(d['duplicate_of'])
            require(original is not None and original['outcome'] == 'ACCEPTED'
                    and original['key'] == d['key'] and original['assertion_sha256'] == d['assertion_sha256'], 'duplicate-target')
        else:
            require(d['duplicate_of'] is None and e['unit'] == d['unit'], 'accepted-unit')
            accepted_keys.add(d['key'])
    require(accepted_keys == set(entities), 'entity-decision-bijection')
    require(all(roots[name] == i['roots'] for name, i in inputs.items()), 'root-census')
    require(counts == m['counts'], 'stream-census')
    for e in entities.values():
        require(e['rights_sha256'] == m['rights_sha256'], 'entity-rights')
        require(e['assertion_sha256'] == assertion_hash(e), 'assertion-hash')
        if PARENT[e['kind']] is None:
            require(e['parent_key'] is None, 'entity-root')
        else:
            parent = entities.get(e['parent_key'])
            require(parent is not None and parent['kind'] == PARENT[e['kind']], 'entity-parent')
        shape(schemas['Payload' + e['kind']], e['payload'], schemas)
        if e['kind'] == 'PLACE':
            lat, lon = (e['payload'][f] for f in ('latitude_decimal', 'longitude_decimal'))
            require((lat is None) == (lon is None), 'coordinate-pair')
            if lat is not None:
                require(abs(Decimal(lat)) <= 90 and abs(Decimal(lon)) <= 180, 'coordinate-range')
                require(e['payload']['public_access'] == 'PUBLIC', 'coordinate-privacy')
        required_paths = p['field_paths'].get(e['kind'])
        require(required_paths is not None, 'field-profile-pending')
        require(e['field_provenance'] == [{'field': f, 'path': required_paths[f]} for f in sorted(required_paths)]
                and set(e['payload']) == set(required_paths), 'field-provenance')
        require([c['field'] for c in e['source_clocks']] == ['created_at', 'verified_at', 'status_at'], 'source-clocks')
        for clock in e['source_clocks']:
            if clock['value'] is not None:
                try:
                    datetime.strptime(clock['value'], '%Y-%m-%dT%H:%M:%SZ')
                except ValueError as exc:
                    raise ContractError('source-clock-invalid') from exc
    for a in streams['aliases.ndjson']:
        target = entities.get(a['target_key'])
        require(target is not None and a['kind'] == target['kind']
                and a['alias_key'] not in entities
                and a['alias_key'] == source_key(p['namespace'], a['kind'], a['external_id']), 'alias-target-or-identity')
    # No entity or source-controlled text is returned until every check succeeds.
    return {'contract': 'VALID', 'source_admission_enabled': False,
            'input_bytes_verified': False, 'profile_state': p['state'],
            'content_sha256': m['content_sha256'], 'counts': counts,
            'input_roots': sum(roots.values()), 'aliases': len(streams['aliases.ndjson'])}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    try:
        summary = verify(args.directory)
    except ContractError as exc:
        parser.exit(1, f'EV contract refused: {exc}\n')
    print(json.dumps(summary, sort_keys=True))


if __name__ == '__main__':
    main()
