#!/usr/bin/env python3
"""Immediate privacy/profile negatives for the local EE-01 inspector."""
import importlib.util
import json
from pathlib import Path
import unittest
import tempfile

path = Path(__file__).resolve().parents[1] / 'qualify-ev-samples.py'
spec = importlib.util.spec_from_file_location('ev_qualification', path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class QualificationTest(unittest.TestCase):
    def setUp(self):
        self.reference = {'Countries': [{'ID': 32, 'ISOCode': 'BR'}],
                          'DataProviders': [{'ID': 1, 'License': 'Synthetic test license'}],
                          'ConnectionTypes': [{'ID': 25}]}
        self.value = {'DataProviderID': 1, 'AddressInfo': {'CountryID': 32, 'ContactEmail': 'NEVER_COPY'},
                      'UserComments': [{'UserName': 'NEVER_COPY'}], 'Connections': [{'ConnectionTypeID': 25}]}

    def test_privacy_and_no_admission(self):
        result = module.qualify_ocm(json.dumps(self.value).encode(), self.reference)
        self.assertNotIn('NEVER_COPY', json.dumps(result))
        self.assertEqual(result['country_code'], 'BR')
        self.assertFalse(result['admission_enabled'])

    def test_wrong_country(self):
        self.value['AddressInfo']['CountryID'] = 999
        with self.assertRaisesRegex(ValueError, 'country'):
            module.qualify_ocm(json.dumps(self.value).encode(), self.reference)

    def test_missing_license(self):
        self.reference['DataProviders'][0]['License'] = ''
        with self.assertRaisesRegex(ValueError, 'license'):
            module.qualify_ocm(json.dumps(self.value).encode(), self.reference)

    def test_unknown_connection(self):
        self.value['Connections'][0]['ConnectionTypeID'] = 999
        with self.assertRaisesRegex(ValueError, 'connection'):
            module.qualify_ocm(json.dumps(self.value).encode(), self.reference)

    def test_duplicate_and_nonfinite(self):
        for raw in [b'{"a":1,"a":2}', b'{"a":NaN}']:
            with self.assertRaises(ValueError):
                module.strict_json(raw)

    def test_byte_budget(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'synthetic.json'
            path.write_bytes(b'12345')
            with self.assertRaisesRegex(ValueError, 'byte budget'):
                module.bounded(path, 4)

    def test_invalid_pdf_has_no_raw_error_body(self):
        import subprocess
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'synthetic.pdf'
            path.write_bytes(b'NEVER_COPY invalid pdf')
            with self.assertRaises(subprocess.CalledProcessError) as raised:
                module.inspect_process(['pdftotext', str(path), '-'])
            self.assertIsNone(raised.exception.stderr)


if __name__ == '__main__':
    unittest.main()
