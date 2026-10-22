#!/usr/bin/env python3
"""Independent EE-03 census oracle and semantic failures after hash resealing."""
import copy
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('ev_prepared', ROOT / 'scripts/verify-ev-prepared-contract.py')
v = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(v)
FIXTURES = ROOT / 'contracts/testdata/ev/prepared-v1'


class PreparedContractTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.batch = Path(self.tmp.name) / 'batch'
        shutil.copytree(FIXTURES / 'nested', self.batch)
        self.manifest = json.loads((self.batch / 'manifest.json').read_bytes())

    def rows(self, file):
        return [json.loads(x) for x in (self.batch / file).read_bytes().splitlines()]

    def write(self, file, rows):
        order = {'entities.ndjson': 'key', 'decisions.ndjson': 'unit', 'aliases.ndjson': 'alias_key'}[file]
        raw = b''.join(v.canonical(row) + b'\n' for row in sorted(rows, key=lambda r: r[order]))
        self.raw(file, raw, len(rows))

    def raw(self, file, raw, records=None):
        (self.batch / file).write_bytes(raw)
        d = self.manifest['artifacts'][file]
        d.update(sha256=v.sha(raw), bytes=len(raw))
        if records is not None:
            d['records'] = records

    def seal(self):
        m = copy.deepcopy(self.manifest)
        m.pop('content_sha256', None)
        self.manifest['content_sha256'] = v.sha(v.canonical(m))
        (self.batch / 'manifest.json').write_bytes(v.canonical(self.manifest) + b'\n')

    def refused(self, reason):
        self.seal()
        with self.assertRaisesRegex(v.ContractError, reason):
            v.verify(self.batch)

    def test_hand_counted_hierarchy(self):
        result = v.verify(self.batch)
        expected = json.loads((FIXTURES / 'expected-census.json').read_bytes())
        # Explicit numbers are the independent reviewed oracle, not derived by
        # the producer/checker from its own totals. Quantity3 never becomes3 EVSE.
        self.assertEqual(expected['counts'], {
            'PLACE': dict(input=3, accepted=1, duplicates=1, quarantined=1),
            'EVSE': dict(input=3, accepted=1, duplicates=1, quarantined=1),
            'CONNECTOR': dict(input=5, accepted=2, duplicates=2, quarantined=1),
            'CONNECTION_GROUP': dict(input=2, accepted=1, duplicates=1, quarantined=0),
            'VEHICLE_REFERENCE': dict(input=1, accepted=1, duplicates=0, quarantined=0)})
        self.assertEqual(result['counts'], expected['counts'])
        self.assertEqual(result['input_roots'], 4)
        self.assertEqual(len(self.rows('decisions.ndjson')), 14)
        entities = self.rows('entities.ndjson')
        self.assertEqual(sum(e['kind'] == 'CONNECTOR' for e in entities), 2)
        self.assertEqual(next(e['payload']['reported_quantity'] for e in entities if e['kind'] == 'CONNECTION_GROUP'), 3)
        unknown = next(e for e in entities if e['external_id'] == 'c2')
        self.assertEqual(unknown['payload'], dict(connector_code='UNKNOWN', raw_connector_code='unknown-42', current='UNKNOWN', power_w=None))
        self.assertEqual(result['aliases'], 1)
        self.assertFalse(result['source_admission_enabled'])
        self.assertFalse(result['input_bytes_verified'])
        self.assertEqual(v.verify(self.batch), result)

    def test_empty_stream_hash_and_zero_census(self):
        result = v.verify(FIXTURES / 'empty')
        self.assertEqual(result['input_roots'], 0)
        for f in v.FILES:
            self.assertEqual((FIXTURES / 'empty' / f).read_bytes(), b'')
        self.assertEqual(v.sha(b''), 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')

    def test_byte_and_identity_goldens(self):
        golden = json.loads((FIXTURES / 'byte-goldens.json').read_bytes())
        for x in golden['canonical']:
            self.assertEqual(v.canonical(x['value']), x['expected_utf8'].encode())
        for x in golden['identities']:
            self.assertEqual(v.source_key(x['namespace'], x['kind'], x['external_id']), x['sha256'])
        self.assertNotEqual(v.source_key('ab', 'PLACE', 'c'), v.source_key('a', 'PLACE', 'bc'))
        for bad in ('', '\x85', 'x' * 257):
            with self.assertRaises(v.ContractError):
                v.source_key('ocm', 'PLACE', bad)

    def test_strict_json_negatives(self):
        for bad in (b'{"a":1,"a":2}', b'{"a":1.0}', b'{"a":NaN}', b'{"a":1e3}',
                    b'{"a":9007199254740992}', b'\xef\xbb\xbf{}', b'\xff',
                    b'{"a":"\\ud800"}', b'{"a":', b'[' * 40 + b'0' + b']' * 40,
                    b'{"a":' + b'9' * 5000 + b'}'):
            with self.subTest(raw=bad), self.assertRaises(v.ContractError):
                v.parse(bad)
        for val in (1.5, -9007199254740992, {'á': 'x'}, 'x' * (65537)):
            with self.subTest(value_type=type(val)), self.assertRaises(v.ContractError):
                v.canonical(val)

    def test_raw_tamper_returns_no_stdout_or_rows(self):
        p = self.batch / 'aliases.ndjson'
        p.write_bytes(p.read_bytes().replace(b'REVIEWED', b'XEVIEWED'))
        result = subprocess.run([sys.executable, str(ROOT / 'scripts/verify-ev-prepared-contract.py'), str(self.batch)], capture_output=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, b'')
        self.assertIn(b'artifact-hash-or-length', result.stderr)
        self.assertNotIn(b'XEVIEWED', result.stderr)

    def test_truncation_even_with_rehashed_descriptor(self):
        f = 'decisions.ndjson'
        self.raw(f, (self.batch / f).read_bytes()[:-4])
        self.refused('artifact-final-lf')

    def test_noncanonical_and_bad_framing(self):
        f = 'aliases.ndjson'
        original = (self.batch / f).read_bytes()
        for raw, reason in ((b' ' + original, 'artifact-canonical'),
                            (original[:-1], 'artifact-final-lf'),
                            (original + b'\n', 'artifact-count'),
                            (b'\xef\xbb\xbf' + original, 'json-invalid')):
            with self.subTest(reason=reason):
                self.raw(f, raw)
                self.refused(reason)

    def test_descriptor_bytes_count_and_manifest_identity(self):
        for field, reason in (('bytes', 'artifact-hash-or-length'), ('records', 'artifact-count')):
            self.manifest['artifacts']['aliases.ndjson'][field] += 1
            self.refused(reason)
            self.manifest['artifacts']['aliases.ndjson'][field] -= 1
        self.seal()
        m = copy.deepcopy(self.manifest)
        m['source_revision'] = 'changed'
        (self.batch / 'manifest.json').write_bytes(v.canonical(m) + b'\n')
        with self.assertRaisesRegex(v.ContractError, 'manifest-content-hash'):
            v.verify(self.batch)

    def test_bad_census_after_rehash(self):
        self.manifest['counts']['CONNECTOR']['duplicates'] += 1
        self.refused('stream-census')

    def test_missing_root_and_nested_unit(self):
        for unit, reason in (('synthetic.json#3', 'entity-decision-bijection'),
                             ('synthetic.json#0/evses/0', 'nested-parent')):
            original = self.rows('decisions.ndjson')
            self.write('decisions.ndjson', [d for d in original if d['unit'] != unit])
            self.refused(reason)
            self.write('decisions.ndjson', original)

    def test_orphan_and_accepted_child_of_quarantine(self):
        original = self.rows('decisions.ndjson')
        for parent, reason in (('synthetic.json#99', 'nested-parent'), ('synthetic.json#2', 'nested-parent')):
            rows = copy.deepcopy(original)
            next(d for d in rows if d['unit'] == 'synthetic.json#0/evses/0')['parent_unit'] = parent
            self.write('decisions.ndjson', rows)
            self.refused(reason)
        rows = copy.deepcopy(original)
        child = next(d for d in rows if d['unit'] == 'synthetic.json#2/evses/0')
        child['outcome'] = 'ACCEPTED'
        self.write('decisions.ndjson', rows)
        self.refused('quarantined-parent')

    def test_duplicate_conflict_and_chain(self):
        original = self.rows('decisions.ndjson')
        for field, value, reason in (('assertion_sha256', '0' * 64, 'decision-assertion'),
                                      ('duplicate_of', 'synthetic.json#1', 'duplicate-target'),
                                      ('key', '0' * 64, 'stable-key')):
            rows = copy.deepcopy(original)
            next(d for d in rows if d['unit'] == 'synthetic.json#1')[field] = value
            self.write('decisions.ndjson', rows)
            self.refused(reason)

    def test_payload_and_provenance_tamper_rehashed(self):
        original = self.rows('entities.ndjson')
        for mutation, reason in ((lambda e: e['payload'].update(contact_email='NEVER_COPY'), 'shape-one-of'),
                                  (lambda e: e['payload'].update(latitude_decimal='91'), 'coordinate-range'),
                                  (lambda e: e['payload'].update(public_access='UNKNOWN'), 'coordinate-privacy'),
                                  (lambda e: e['payload'].update(longitude_decimal=None), 'coordinate-pair'),
                                  (lambda e: e['field_provenance'].clear(), 'field-provenance'),
                                  (lambda e: e['source_clocks'][0].update(value='2026-02-30T00:00:00Z'), 'source-clock-invalid'),
                                  (lambda e: e.update(rights_sha256='0' * 64), 'entity-rights')):
            rows = copy.deepcopy(original)
            entity = next(e for e in rows if e['kind'] == 'PLACE')
            mutation(entity)
            entity['assertion_sha256'] = v.assertion_hash(entity)
            decisions = self.rows('decisions.ndjson')
            for d in decisions:
                if d['key'] == entity['key']:
                    d['assertion_sha256'] = entity['assertion_sha256']
            self.write('entities.ndjson', rows)
            self.write('decisions.ndjson', decisions)
            self.refused(reason)

    def test_alias_self_unknown_target_identity_and_cycle(self):
        original = self.rows('aliases.ndjson')[0]
        for target in (original['alias_key'], '0' * 64):
            row = dict(original, target_key=target)
            self.write('aliases.ndjson', [row])
            self.refused('alias-target-or-identity')
        row = dict(original, external_id='changed')
        self.write('aliases.ndjson', [row])
        self.refused('alias-target-or-identity')
        other = dict(original, alias_key=v.source_key('ee-test', 'PLACE', 'p-other'), external_id='p-other', target_key=original['alias_key'])
        row = dict(original, target_key=other['alias_key'])
        self.write('aliases.ndjson', [row, other])
        self.refused('alias-target-or-identity')

    def test_changed_trusted_license_profile_and_reference(self):
        registry = json.loads(v.REGISTRY.read_bytes())
        registry['profiles'][2]['rights']['state'] = 'LICENSE_CHANGED'
        target = Path(self.tmp.name) / 'registry.json'
        target.write_bytes(v.canonical(registry))
        # Rehashing the manifest cannot bless a source's changed trusted policy.
        self.seal()
        with self.assertRaisesRegex(v.ContractError, 'profile-drift'):
            v.verify(self.batch, target)
        for field, value, reason in (('rights_sha256', '0' * 64, 'rights-drift'),
                                     ('reference_sha256', '0' * 64, 'source-profile-binding'),
                                     ('profile_id', 'unknown', 'trusted-profile'),
                                     ('format', 'future-v2', 'shape-enum')):
            old = self.manifest[field]
            self.manifest[field] = value
            self.refused(reason)
            self.manifest[field] = old

    def test_real_source_gates_and_profile_pins(self):
        registry = json.loads(v.REGISTRY.read_bytes())
        evidence = json.loads((ROOT / 'docs/backend/evidence/ev-support/EE-01-samples.json').read_bytes())
        ocm, pbe, synthetic = registry['profiles']
        self.assertEqual(ocm['source_revision'], evidence['ocm_commit'])
        self.assertEqual(ocm['reference_sha256'], evidence['reference_sha256'])
        self.assertEqual(ocm['rights']['provider_license_sha256'], evidence['samples'][0]['provider_license_sha256'])
        self.assertEqual(ocm['kinds'], ['PLACE', 'CONNECTION_GROUP'])
        self.assertEqual(pbe['source_revision'], evidence['pbe']['sha256'])
        self.assertEqual(pbe['state'], 'RIGHTS_AND_LAYOUT_PENDING')
        self.assertTrue(all(p['source_admission_enabled'] is False for p in registry['profiles']))
        self.assertEqual(synthetic['state'], 'CONTRACT_TEST_ONLY')
        # Even an internally rehashed manifest cannot push actual rows through
        # a rights/layout-held profile. No synthetic adapter admission bypass.
        self.manifest.update(profile_id=pbe['id'], profile_sha256=v.sha(v.canonical(pbe)),
                             rights_sha256=v.sha(v.canonical(pbe['rights'])),
                             source_revision=pbe['source_revision'], reference_sha256=None,
                             format='vehicle-energy-reference-v1')
        self.refused('held-source')

    def test_path_files_order_and_limits(self):
        (self.batch / 'unexpected').write_bytes(b'NEVER_COPY')
        with self.assertRaisesRegex(v.ContractError, 'batch-files'):
            v.verify(self.batch)
        (self.batch / 'unexpected').unlink()
        alias = self.batch / 'aliases.ndjson'
        alias.unlink()
        alias.symlink_to(FIXTURES / 'nested/aliases.ndjson')
        with self.assertRaisesRegex(v.ContractError, 'file-unavailable'):
            v.verify(self.batch)
        alias.unlink()
        shutil.copyfile(FIXTURES / 'nested/aliases.ndjson', alias)
        rows = self.rows('entities.ndjson')
        self.raw('entities.ndjson', b''.join(v.canonical(r) + b'\n' for r in reversed(rows)))
        self.refused('artifact-order-or-duplicate')
        self.write('entities.ndjson', rows)
        self.manifest['inputs'][0]['file'] = '../private'
        self.refused('shape-string-pattern')
        self.manifest['inputs'][0]['file'] = 'synthetic.json'
        self.manifest['inputs'][0]['bytes'] = v.MAX_BYTES + 1
        self.refused('shape-integer-range')
        with self.assertRaisesRegex(v.ContractError, 'file-boundary'):
            v.read_file(self.batch / 'manifest.json', 1)


if __name__ == '__main__':
    unittest.main()
