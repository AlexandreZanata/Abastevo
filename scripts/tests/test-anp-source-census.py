#!/usr/bin/env python3
"""Meaningful boundary checks for the offline counts-only source oracle."""
import importlib.util
import io
import json
import collections
from pathlib import Path
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('census', Path(__file__).parents[1]/'audit-anp-source-census.py')
census = importlib.util.module_from_spec(spec)
spec.loader.exec_module(census)

class CensusBoundaries(unittest.TestCase):
    def test_unknown_segment_never_defaults_to_retail(self):
        known = {'Revenda de GLP', 'TRR'}
        for label in ('', 'Agente Não Regulado', 'Future segment', 'Revenda de combustíveis'):
            self.assertEqual(census.classify_segment(label, known), 'UNKNOWN')
        self.assertEqual(census.classify_segment('Revenda de Combustíveis', known), 'AUTOMOTIVE_RETAIL_SOURCE_LABEL')
        self.assertEqual(census.classify_segment('Revenda de GLP', known), 'NON_AUTOMOTIVE_RETAIL')

    def test_shape_is_not_checksum_or_identity_proof(self):
        self.assertEqual(census.subject_kind('11.111.111/1111-11'), 'company-shaped')
        self.assertEqual(census.subject_kind('000.000.000-00'), 'personal-shaped')
        self.assertEqual(census.subject_kind('unknown'), 'other')

    def test_pmqc_whole_member_not_first_row_and_duplicates_are_explicit(self):
        row = ['2026-09-01', 'synthetic', '', 'UNKNOWN_PRODUCT'] + ['']*14 + ['Unknown status']
        raw = ';'.join(census.PMQC_HEADERS)+'\n'+';'.join(row)+'\n'+';'.join(row)+'\n'
        f = io.BytesIO()
        with zipfile.ZipFile(f, 'w') as z:
            z.writestr('synthetic.csv', raw)
        result = census.pmqc(f.getvalue())
        self.assertEqual(result['assay_rows'], 2)
        self.assertEqual(result['exact_duplicate_rows'], 1)
        self.assertEqual(result['native_sample_product_groups'], 1)
        self.assertEqual(result['unknown_conformity'], 2)
        self.assertEqual(result['blank_results'], 2)
        self.assertNotIn('rows', result)  # Individual records never leave the oracle.

    def test_frozen_inventory_reconciles_full_resources_and_keeps_gaps(self):
        inventory = json.loads((Path(__file__).parents[2]/'docs/backend/evidence/mobile-expansion/MEX-17-source-inventory.json').read_text())
        self.assertFalse(inventory['source_activation'])
        for item in inventory['covered']['sanctions']:
            c = item['counts']
            self.assertEqual(c['input'], c['accepted']+c['duplicates']+c['quarantined'])
            self.assertEqual(item['segment_classification'], 'UNKNOWN')
            self.assertEqual(item['whole_source_oracle'], 'PASS')
        self.assertEqual(inventory['covered']['enforcement']['rows'], 244247)
        self.assertFalse(inventory['covered']['enforcement']['runtime_admitted'])
        self.assertEqual(inventory['coverage_gaps']['pmqc_missing_month'], '2026-02')
        self.assertGreater(len(inventory['coverage_gaps']['pmqc_listed_not_censused']), 0)
        policy = inventory['segment_policy']
        counts = collections.Counter()
        for label, count in inventory['covered']['enforcement']['segments'].items():
            counts[census.classify_segment(label, set(policy['non_automotive_retail_exact_labels']))] += count
        self.assertEqual(counts['AUTOMOTIVE_RETAIL_SOURCE_LABEL'], 190282)
        self.assertEqual(counts['UNKNOWN'], 4062)
        self.assertEqual(sum(counts.values()), 244247)

    def test_ambiguous_archive_and_header_refuse(self):
        for members in [ [('a.csv', 'wrong\n')], [('a.csv',''),('b.csv','')] ]:
            f=io.BytesIO()
            with zipfile.ZipFile(f,'w') as z:
                for name,body in members:z.writestr(name,body)
            with self.assertRaises(ValueError):census.pmqc(f.getvalue())

if __name__ == '__main__':unittest.main()
