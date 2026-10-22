#!/usr/bin/env python3
"""Synthetic negative checks; no real source rows or identifiers."""
import importlib.util
from pathlib import Path
import unittest
spec = importlib.util.spec_from_file_location('audit', Path(__file__).parents[1] / 'audit-enforcement-privacy.py')
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

class PrivacyAuditTests(unittest.TestCase):
    def row(self):
        row = dict.fromkeys(audit.KEYS, 'synthetic')
        row.update(subject_cnpj=None, subject_kind='personal-shaped', exact_repeat=False)
        return row
    def test_minimized_record(self):
        audit.check_record(self.row(), set(), set(), set())
    def test_forbidden_columns(self):
        row = self.row(); row['agent_name'] = 'discarded synthetic name'
        with self.assertRaises(ValueError): audit.check_record(row, set(), set(), set())
    def test_personal_identifier_in_wording(self):
        row = self.row(); row['result'] = 'synthetic 123.456.789-01'
        with self.assertRaises(ValueError): audit.check_record(row, {'12345678901'}, set(), set())
    def test_subject_identifier_and_source_wording(self):
        for key, value in [('subject_cnpj','12345678901'),('procedure','discarded synthetic name'),('result','discarded synthetic address')]:
            row = self.row(); row[key] = value
            with self.assertRaises(ValueError): audit.check_record(row, set(), {'discarded synthetic name'}, {'discarded synthetic address'})

if __name__ == '__main__': unittest.main()
