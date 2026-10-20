#!/usr/bin/env python3
"""Reconcile a private ANP CP850 CSV against prepared artifacts; print aggregates only."""
import collections
import csv
import datetime
import hashlib
import json
import pathlib
import re
import sys


def company(value):
    text = re.sub(r"[./\-\s]", "", value)
    if not re.fullmatch(r"[0-9]{14}", text) or len(set(text)) == 1:
        return None
    digits = list(map(int, text))
    for size, weights in [(12, [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]),
                          (13, [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])]:
        remainder = sum(a * b for a, b in zip(digits[:size], weights)) % 11
        if digits[size] != (0 if remainder < 2 else 11 - remainder):
            return None
    return text


def money(text):
    assert re.fullmatch(r"(?:0|[1-9][0-9]*|[1-9][0-9]{0,2}(?:\.[0-9]{3})+),[0-9]{2}", text)
    value = int(text.replace(".", "").replace(",", ""))
    assert value <= 2**63 - 1
    return value


def main():
    source, artifacts = map(pathlib.Path, sys.argv[1:])
    raw = source.read_bytes()
    manifest = json.loads((artifacts / "manifest.json").read_bytes())
    assert manifest["schema_version"] == "sanctions-prepared-v1"
    assert manifest["input_contract"] == "anp-sicom-cp850-9-v1"
    assert manifest["input_sha256"] == hashlib.sha256(raw).hexdigest()
    assert manifest["input_bytes"] == len(raw)
    assert manifest["source_file"] == source.name
    for item in manifest["outputs"]:
        data = (artifacts / item["file"]).read_bytes()
        assert len(data) == item["bytes"]
        assert hashlib.sha256(data).hexdigest() == item["sha256"]
    # Python's independent codec/calendar/check-digit implementation is the oracle.
    records = list(csv.reader(raw.decode("cp850").splitlines(), delimiter=";"))
    assert len(records[6]) == 9
    year, month = manifest["report_year"], manifest["extraction_month"]
    expected = collections.Counter()
    rejected = []
    for number, row in enumerate(records[7:], 8):
        cnpj = company(row[4])
        if cnpj is None:
            rejected.append(number)
            continue
        due = datetime.datetime.strptime(row[6], "%d/%m/%Y").date().isoformat()
        # No company name or CPF is used in the expected minimized observation.
        key = (cnpj, row[2], row[3], row[1], row[0], due, row[7], money(row[7]),
               row[8], money(row[8]), year, month)
        expected[key] += 1
    observed = collections.Counter()
    keys = set()
    for line in (artifacts / "sanctions.jsonl").read_text().splitlines():
        row = json.loads(line)
        assert set(row) == {"schema_version", "source", "observation_key", "source_key", "proceeding_ref", "duf_ref", "department", "status", "due_on", "amount_text", "amount_minor", "paid_text", "paid_minor", "report_year", "extraction_month", "evidence_ref"}
        assert row["schema_version"] == "anp-fine-observation-v1"
        assert row["source"] == "sanctions"
        assert row["observation_key"] not in keys
        keys.add(row["observation_key"])
        observed[(row["source_key"], row["proceeding_ref"], row["duf_ref"], row["department"],
                  row["status"], row["due_on"], row["amount_text"], row["amount_minor"],
                  row["paid_text"], row["paid_minor"], row["report_year"], row["extraction_month"])] += 1
    assert set(observed) == set(expected)
    assert all(v == 1 for v in observed.values())
    quarantine = [json.loads(line) for line in (artifacts / "quarantine.jsonl").read_text().splitlines()]
    assert [row["row_locator"] for row in quarantine] == [f"{source.name}:{n}" for n in rejected]
    assert all(row["source_key"] is None and row["reason"] == "invalid_cnpj" for row in quarantine)
    counts = {"input": len(records) - 7, "accepted": len(expected),
              "duplicates": sum(expected.values()) - len(expected), "quarantined": len(rejected)}
    assert counts == manifest["counts"]
    # Rejecting source changes is intentional: do not silently change pinned evidence.
    assert counts == {"input": 3395, "accepted": 3375, "duplicates": 0, "quarantined": 20}
    assert manifest["input_sha256"] == "277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96"
    print(json.dumps({"source_sha256": manifest["input_sha256"], "counts": counts,
                      "extraction_month": month, "oracle": "PASS"}, sort_keys=True))


if __name__ == "__main__":
    main()
