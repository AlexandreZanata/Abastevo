#!/usr/bin/env python3
"""Offline, counts-only independent census of private ANP source bytes (MEX-17)."""
import argparse
import collections
import csv
import datetime
import hashlib
import io
import json
from pathlib import Path
import re
import xml.etree.ElementTree as ET
import zipfile

NS = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
ENFORCEMENT_HEADERS = ['UF', 'Município', 'Bairro', 'ENDEREÇO', 'CNPJ/CPF',
                       'Agente Econômico', 'Segmento Fiscalizado', 'DATA DO DF',
                       'Número do Documento', 'Procedimento de Fiscalização', 'Resultado']
PMQC_HEADERS = ['DataColeta', 'IdNumeric', 'GrupoProduto', 'Produto', 'RazaoSocialPosto',
                'CnpjPosto', 'Distribuidora', 'Endereço', 'Complemento', 'Bairro',
                'Município', 'Latitude', 'Longitude', 'Uf', 'RegiaoPolitica',
                'Ensaio', 'Resultado', 'UnidadeEnsaio', 'Conforme']


def checked_zip(raw):
    z = zipfile.ZipFile(io.BytesIO(raw))
    if len(z.infolist()) > 32 or sum(i.file_size for i in z.infolist()) > 200_000_000:
        raise ValueError('archive budget')
    if len(set(z.namelist())) != len(z.namelist()):
        raise ValueError('duplicate archive member')
    return z


def classify_segment(label, non_retail):
    # Exact source label only; source category never proves a station binding.
    if label == 'Revenda de Combustíveis':
        return 'AUTOMOTIVE_RETAIL_SOURCE_LABEL'
    return 'NON_AUTOMOTIVE_RETAIL' if label in non_retail else 'UNKNOWN'


def subject_kind(text):
    value = re.sub(r'[./\-\s]', '', text)
    return 'company-shaped' if re.fullmatch(r'[0-9]{14}', value) else 'personal-shaped' if re.fullmatch(r'[0-9]{11}', value) else 'other'


def enforcement(raw):
    """Complete worksheet census, independent of Rust or runtime parsers; no row output."""
    with checked_zip(raw) as z:
        workbook = ET.fromstring(z.read('xl/workbook.xml'))
        props = workbook.find('s:workbookPr', NS)
        date1904 = props is not None and props.get('date1904', '0') in ('1', 'true')
        strings = []
        if 'xl/sharedStrings.xml' in z.namelist():
            strings = [''.join(si.itertext()) for si in ET.fromstring(z.read('xl/sharedStrings.xml')).findall('s:si', NS)]
        def value(c):
            if c.get('t') == 'inlineStr':
                return ''.join(c.find('s:is', NS).itertext())
            v = c.find('s:v', NS)
            text = '' if v is None else (v.text or '')
            return strings[int(text)] if c.get('t') == 's' else text
        def position(ref):
            n = 0
            for ch in re.match('[A-Z]+', ref)[0]:
                n = n * 26 + ord(ch) - 64
            return n - 1
        headers = None
        total = 0
        subjects, segments = collections.Counter(), collections.Counter()
        docs, cases, exact = set(), set(), set()
        results, beyond64, max_result, nonnumeric_dates = set(), 0, 0, 0
        with z.open('xl/worksheets/sheet1.xml') as sheet:
            for _, row in ET.iterparse(sheet, events=('end',)):
                if row.tag != '{'+NS['s']+'}row':
                    continue
                cells = [''] * 11
                for c in row.findall('s:c', NS):
                    pos = position(c.get('r'))
                    if pos >= 11:
                        raise ValueError('worksheet width')
                    cells[pos] = value(c)
                if headers is None:
                    headers = cells
                    if headers != ENFORCEMENT_HEADERS:
                        raise ValueError('enforcement header mismatch')
                else:
                    total += 1
                    subjects[subject_kind(cells[4])] += 1
                    segments[cells[6]] += 1
                    docs.add(cells[8])
                    cases.add(tuple(cells[i] for i in [4, 7, 8, 9]))
                    # Keep only hashes and counts, never names/subjects/addresses/results.
                    exact.add(hashlib.sha256(json.dumps(cells, ensure_ascii=False).encode()).digest())
                    results.add(hashlib.sha256(cells[10].encode()).digest())
                    max_result = max(max_result, len(cells[10]))
                    beyond64 += len(cells[10]) > 64
                    nonnumeric_dates += not bool(re.fullmatch(r'[0-9]+(?:\.[0-9]+)?', cells[7]))
                row.clear()
        return {'headers': headers, 'rows': total, 'subject_shapes': dict(subjects),
                'segments': dict(sorted(segments.items())), 'distinct_documents': len(docs),
                'distinct_subject_date_document_procedure': len(cases), 'exact_duplicate_rows': total-len(exact),
                'result_vocabulary_size': len(results), 'result_max_chars': max_result,
                'results_over_64_chars': beyond64, 'date1904': date1904,
                'nonnumeric_dates': nonnumeric_dates, 'date_precision': 'EXCEL_SERIAL_WITHOUT_TIMEZONE',
                'expanded_bytes': sum(i.file_size for i in z.infolist()), 'members': len(z.infolist()),
                'runtime_admitted': False}


def pmqc(raw):
    with checked_zip(raw) as z:
        names = [i for i in z.namelist() if i.lower().endswith('.csv')]
        if len(names) != 1:
            raise ValueError('ambiguous PMQC CSV')
        with z.open(names[0]) as f:
            rows = csv.reader(io.TextIOWrapper(f, encoding='utf-8-sig'), delimiter=';')
            if next(rows) != PMQC_HEADERS:
                raise ValueError('PMQC header mismatch')
            count, empty, blank, unknown = 0, 0, 0, 0
            samples, exact, products, conformity, clocks = set(), set(), collections.Counter(), collections.Counter(), collections.Counter()
            for r in rows:
                if len(r) != len(PMQC_HEADERS):
                    raise ValueError('PMQC width mismatch')
                count += 1
                samples.add((r[1], r[3]))
                exact.add(hashlib.sha256(json.dumps(r, ensure_ascii=False).encode()).digest())
                products[r[3]] += 1
                conformity[r[18]] += 1
                empty += r[16] == ''
                blank += r[16].strip() == ''
                unknown += r[18] not in ('Sim', 'Não')
                datetime.datetime.fromisoformat(r[0])
                clocks['DAY' if len(r[0]) == 10 else 'UNZONED_COLLECTION_CLOCK'] += 1
            return {'headers': PMQC_HEADERS, 'assay_rows': count, 'native_sample_product_groups': len(samples),
                    'exact_duplicate_rows': count-len(exact), 'products': dict(sorted(products.items())),
                    'conformity': dict(sorted(conformity.items())), 'empty_results': empty, 'blank_results': blank,
                    'unknown_conformity': unknown, 'collection_clock_precision': dict(clocks),
                    'member': names[0], 'expanded_bytes': sum(i.file_size for i in z.infolist())}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--enforcement', type=Path, required=True)
    parser.add_argument('--pmqc', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    out = {'schema': 'anp-independent-census-v1'}
    for kind in ('enforcement', 'pmqc'):
        path = getattr(args, kind)
        if path.stat().st_size > 25_000_000:
            raise ValueError('input budget')
        raw = path.read_bytes()
        out[kind] = {'bytes': len(raw), 'sha256': hashlib.sha256(raw).hexdigest(),
                     **globals()[kind](raw)}
    args.out.write_text(json.dumps(out, ensure_ascii=False, indent=2, sort_keys=True)+'\n')
    print('Independent source census PASS (aggregate output only)')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, IndexError, ET.ParseError, zipfile.BadZipFile, UnicodeError, OSError, StopIteration):
        raise SystemExit('Source census refused; inspect private inputs locally (no row/body output).')
