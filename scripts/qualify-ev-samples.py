#!/usr/bin/env python3
"""Local EE-01 sample inspector, not a production source preparer or admission."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import resource
import subprocess
import tempfile


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate JSON key")
            result[key] = value
        return result
    def reject(_):
        raise ValueError("non-finite JSON number")
    return json.loads(raw, object_pairs_hook=pairs, parse_constant=reject)


def bounded(path, maximum):
    with path.open('rb') as source:
        raw = source.read(maximum + 1)
    if len(raw) > maximum:
        raise ValueError("sample byte budget exceeded")
    return raw


def qualify_ocm(raw, reference):
    obj = strict_json(raw)
    brazil = [x['ID'] for x in reference['Countries'] if x.get('ISOCode') == 'BR']
    if len(brazil) != 1 or obj.get('AddressInfo', {}).get('CountryID') != brazil[0]:
        raise ValueError("sample country not qualified Brazil")
    provider = [x for x in reference['DataProviders'] if x['ID'] == obj.get('DataProviderID')]
    if len(provider) != 1 or not provider[0].get('License'):
        raise ValueError("missing provider or license")
    connections = obj.get('Connections')
    if not isinstance(connections, list) or len(connections) > 1000:
        raise ValueError("invalid sample connection array")
    lookup = {x['ID'] for x in reference['ConnectionTypes']}
    if any(x.get('ConnectionTypeID') not in lookup for x in connections):
        raise ValueError("unknown connection reference")
    # Do not copy address/coordinates/comments/contact/contributor values.
    return {'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw),
            'top_level_fields': sorted(obj), 'country_code': 'BR',
            'provider_id': provider[0]['ID'],
            'provider_license_sha256': hashlib.sha256(provider[0]['License'].encode()).hexdigest(),
            'provider_reported_license': 'CC-BY-4.0' if 'CC BY 4.0' in provider[0]['License'] else 'UNRECOGNIZED',
            'provider_open_data_flag': provider[0].get('IsOpenDataLicensed'),
            'connection_rows': len(connections),
            'connection_fields': sorted({k for c in connections for k in c}),
            'clock_fields_present': [k for k in ['DateLastVerified', 'DateLastStatusUpdate', 'DateLastConfirmed', 'DateCreated'] if obj.get(k)],
            'source_clock_values': {k: v for k in ['DateLastVerified', 'DateLastStatusUpdate', 'DateLastConfirmed', 'DateCreated']
                                    if isinstance((v := obj.get(k)), str) and re.fullmatch(r'[0-9TtZz:+.\-]{10,64}', v)},
            'qualification': 'SAMPLE_VERIFIED', 'admission_enabled': False}


def inspect_process(command):
    def limits():
        resource.setrlimit(resource.RLIMIT_AS, (512 * 1024 * 1024,) * 2)
        resource.setrlimit(resource.RLIMIT_FSIZE, (8 * 1024 * 1024,) * 2)
        resource.setrlimit(resource.RLIMIT_CPU, (15, 15))
    with tempfile.TemporaryFile() as output:
        subprocess.run(command, stdout=output, stderr=subprocess.DEVNULL,
                       check=True, timeout=30, preexec_fn=limits)
        output.seek(0)
        raw = output.read(8 * 1024 * 1024 + 1)
        if len(raw) > 8 * 1024 * 1024:
            raise ValueError("inspection output budget exceeded")
        return raw


def run():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--folder', type=Path, required=True)
    args = parser.parse_args()
    root = args.folder
    reference_raw = bounded(root / 'referencedata.json', 8 * 1024 * 1024)
    reference = strict_json(reference_raw)
    files = sorted(root.glob('OCM-*.json'))
    if not 1 <= len(files) <= 10:
        raise ValueError("qualification requires 1..10 samples, not a national load")
    report = {'state': 'QUALIFICATION_ONLY_NOT_SOURCE_ADMISSION',
              'reference_sha256': hashlib.sha256(reference_raw).hexdigest(),
              'samples': [{'file': p.name, **qualify_ocm(bounded(p, 512 * 1024), reference)} for p in files]}
    pdf = bounded(root / 'pbe-2026.pdf', 8 * 1024 * 1024)
    if not pdf.startswith(b'%PDF-'):
        raise ValueError("PBE resource is not PDF")
    # Existing Poppler only. A bounded profile inspection, no table parser/census.
    text = inspect_process(['pdftotext', '-layout', str(root / 'pbe-2026.pdf'), '-'])
    info = inspect_process(['pdfinfo', str(root / 'pbe-2026.pdf')]).decode('utf-8', errors='replace')
    pages = re.search(r'^Pages:\s*(\d+)', info, re.M)
    if not pages:
        raise ValueError("missing page count")
    report['pbe'] = {'sha256': hashlib.sha256(pdf).hexdigest(), 'bytes': len(pdf),
                     'pages': int(pages[1]), 'extracted_text_sha256': hashlib.sha256(text).hexdigest(),
                     'profile': 'PDF_LAYOUT_NOT_CSV', 'qualified_as': 'LAB_REFERENCE_NOT_CHARGING_CATALOG',
                     'qualification': 'SAMPLE_VERIFIED', 'rights': 'RIGHTS_PENDING',
                     'full_vehicle_census': 'NOT_PERFORMED', 'admission_enabled': False}
    print(json.dumps(report, indent=2, ensure_ascii=False))


if __name__ == '__main__':
    run()
