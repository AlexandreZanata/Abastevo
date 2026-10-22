#!/usr/bin/env python3
"""Offline full-source privacy audit; output contains counts and hashes only."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import xml.etree.ElementTree as ET

SPEC = importlib.util.spec_from_file_location('anp_census', Path(__file__).with_name('audit-anp-source-census.py'))
CENSUS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CENSUS)
KEYS = set('schema_version source row_locator record_hash tuple_hash subject_cnpj subject_kind uf municipality segment document_date document_number procedure result exact_repeat'.split())
PERSON = re.compile(r'(?<![0-9])[0-9]{3}[.\s]?[0-9]{3}[.\s]?[0-9]{3}[-\s]?[0-9]{2}(?![0-9])')
PIN = '952743f3fa2b83094ad2011c5a4a1bffc14222eabde1bb4ee3bf4816e2f7db40'


def check_record(row, personal, names, addresses):
    if set(row) != KEYS:
        raise ValueError('artifact field allowlist mismatch')
    subject = row['subject_cnpj']
    if row['subject_kind'] == 'company-shaped':
        if not isinstance(subject, str) or not re.fullmatch('[0-9]{14}', subject):
            raise ValueError('company identifier shape mismatch')
    elif subject is not None:
        raise ValueError('noncompany subject carries an identifier')
    for value in row.values():
        if isinstance(value, str):
            for candidate in PERSON.findall(value):
                if re.sub(r'[.\-\s]', '', candidate) in personal:
                    raise ValueError('personal identifier in artifact')
    for key in ('procedure', 'result'):
        if row[key].strip() in names or row[key].strip() in addresses:
            raise ValueError('source name/address copied into public wording')


def excluded_values(raw):
    personal, names, addresses = set(), set(), set()
    ns = CENSUS.NS
    with CENSUS.checked_zip(raw) as z:
        shared = [''.join(si.itertext()) for si in ET.fromstring(z.read('xl/sharedStrings.xml')).findall('s:si', ns)]
        with z.open('xl/worksheets/sheet1.xml') as sheet:
            for _, row in ET.iterparse(sheet, events=('end',)):
                if row.tag != '{' + ns['s'] + '}row':
                    continue
                if row.get('r') == '1':
                    row.clear()
                    continue
                for cell in row.findall('s:c', ns):
                    column = re.match('[A-Z]+', cell.get('r'))[0]
                    if column not in ('D', 'E', 'F'):
                        continue
                    v = cell.find('s:v', ns)
                    text = '' if v is None else (v.text or '')
                    text = shared[int(text)] if cell.get('t') == 's' else text
                    if column == 'E' and CENSUS.subject_kind(text) == 'personal-shaped':
                        personal.add(re.sub(r'[./\-\s]', '', text))
                    elif column == 'F' and len(text.strip()) > 3:
                        names.add(text.strip())
                    elif column == 'D' and len(text.strip()) > 3:
                        addresses.add(text.strip())
                row.clear()
    return personal, names, addresses


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--workbook', type=Path, required=True)
    parser.add_argument('--preparer', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    with args.workbook.open('rb') as source:
        raw = source.read(16188088)
    if len(raw) != 16188087 or hashlib.sha256(raw).hexdigest() != PIN:
        raise ValueError('pinned source identity mismatch')
    personal, names, addresses = excluded_values(raw)
    del raw
    count = 0
    with tempfile.TemporaryDirectory(prefix='enforcement-privacy-') as root:
        out = Path(root) / 'prepared'
        subprocess.run([str(args.preparer), str(args.workbook), str(out)], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=120)
        with (out / 'enforcement-source-records.jsonl').open() as stream:
            while line := stream.readline((1 << 20) + 1):
                if len(line) > 1 << 20:
                    raise ValueError('artifact line budget')
                check_record(json.loads(line), personal, names, addresses)
                count += 1
        if count != 244247:
            raise ValueError('artifact census mismatch')
    report = dict(schema='enforcement-privacy-acceptance-v1', sha256=PIN,
                  records_checked=count, personal_identifiers_checked=len(personal),
                  source_names_checked=len(names), source_addresses_checked=len(addresses),
                  personal_identifier_matches=0, name_address_wording_matches=0,
                  excluded_columns_emitted=0, temporary_artifacts_removed=True)
    fd = os.open(args.out, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as dest:
        json.dump(report, dest, indent=2, sort_keys=True)
        dest.write('\n')
    print('Whole-source privacy audit PASS (aggregate output only)')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, ET.ParseError, subprocess.SubprocessError):
        raise SystemExit('Whole-source privacy audit REFUSED (details suppressed)') from None
