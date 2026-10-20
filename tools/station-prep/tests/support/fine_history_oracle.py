#!/usr/bin/env python3
"""Independent whole-file historical ANP oracle. Print aggregates only.

All source/prepared files remain private local inputs. This verifies codecs,
calendar, exact money, CNPJ, minimization, every locator, four artifact hashes
and the complete logical manifest; it does not call the Rust parser.
"""
import csv
import datetime
import hashlib
import io
import json
import pathlib
import re
import sys

from fine_source_oracle import company, money

PROFILES = {
    "2016-2019": ("multas-aplicadas-2016a2019.csv", "utf-8-sig", 5, "anp-sicom-utf8bom-group-2016-2019-v1", "15ccbf54367881946104dc6baf65c246b6189638cb306ab06662570451989f25"),
    "2020": ("multas-aplicadas-2020.csv", "utf-8-sig", 7, "anp-sicom-utf8bom-2020-v1", "aef7fce874b29db234ab7779019c0bc9fcf13d1bdb2de7d59bbf09f51e32654f"),
    "2021": ("multas-2021-junho22.xlsx", "utf-8-sig", 7, "anp-sicom-utf8bom-csv-named-xlsx-2021-v1", "85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d"),
    "2022": ("multas-aplicadas-2022.csv", "utf-8-sig", 7, "anp-sicom-utf8bom-2022-v1", "5ee3df1d9188f273fbf3fbeb7e95f2cea0bc2171fdda23bc4af3cce9478251b5"),
    "2023": ("multas-aplicadas-2023.csv", "cp850", 6, "anp-sicom-cp850-2023-v1", "3865025eb04a82ee5d937d703f063d617843eb7a9f5cbf4c18f989caa33839bb"),
    "2024": ("multas-aplicadas-2024.csv", "utf-8-sig", 7, "anp-sicom-utf8bom-2024-v1", "0723561b182ac0cc3d031a77050049844e3b29ec098472383b0229051cb67dcd"),
}


def canonical(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()


def day(value):
    assert re.fullmatch(r"[0-9]{2}/[0-9]{2}/[0-9]{4}", value)
    return datetime.datetime.strptime(value, "%d/%m/%Y").date().isoformat()


def currency(value, unknown=False):
    text = value.strip()
    if text.startswith("R$"):
        text = text[2:].strip()
        if text == "-" and unknown:
            return None
    return money(text)


def verify(source, artifacts, period):
    name, encoding, header_record, contract, pinned = PROFILES[period]
    raw = source.read_bytes()
    assert source.name == name and hashlib.sha256(raw).hexdigest() == pinned
    rows = list(csv.reader(io.StringIO(raw.decode(encoding)), delimiter=";"))
    header = rows[header_record - 1]
    assert header[0] == "Status Processo" and header[4] == "CNPJ/CPF"
    transit = "Data Transito Julgado" in header
    if period == "2016-2019":
        clock = {"precision": "DAY", "value": day(rows[0][4].removeprefix("Data: "))}
    else:
        match = re.fullmatch(r"RELAÇÃO DE MULTAS APLICADAS EM " + period + r"  \(Relatório extraído em ([^/]+)/([0-9]{4})\)", rows[2][0])
        assert match
        months = "janeiro fevereiro março abril maio junho julho agosto setembro outubro novembro dezembro".split()
        clock = {"precision": "MONTH", "value": f"{match[2]}-{months.index(match[1]) + 1:02}"}
    accepted, duplicates, quarantine, notes = [], [], [], []
    seen = set()
    for number, row in enumerate(rows[header_record:], header_record + 1):
        assert len(row) == len(header)
        locator = f"{name}:{number}"
        metadata = period in ("2020", "2021") and not any(row[1:]) and (not row[0] or row[0].startswith(("(*) ", "(**) ")))
        cnpj = company(row[4])
        if metadata or cnpj is None:
            quarantine.append({"schema_version": "station-quarantine-v1", "source": "sanctions", "row_locator": locator, "reason": "source_metadata" if metadata else "invalid_cnpj", "detail": "historical fine row classified; raw fields redacted", "source_key": None})
            if metadata:
                notes.append({"schema_version": "anp-fine-source-note-v1", "kind": "PUBLIC_PAYMENT_NOTE" if row[0] else "BLANK_SEPARATOR", "text": row[0], "evidence_ref": locator})
            continue
        due_index = 7 if transit else 6
        value = {"schema_version": "anp-fine-history-observation-v1", "source": "sanctions", "observation_key": "", "source_key": cnpj,
                 "proceeding_ref": row[2], "reference_label": header[3], "reference_ref": row[3], "department": row[1], "status": row[0],
                 "source_transit_on": day(row[6]) if transit and row[6] else None, "due_on": day(row[due_index]),
                 "amount_text": row[due_index + 1], "amount_minor": currency(row[due_index + 1]),
                 "paid_text": row[due_index + 2], "paid_minor": currency(row[due_index + 2], period == "2023"),
                 "unlabelled_field": row[9] if period == "2024" else "", "source_period": period, "clock": clock, "input_contract": contract, "evidence_ref": ""}
        value["observation_key"] = "observation:" + hashlib.sha256(canonical(value)).hexdigest()
        value["evidence_ref"] = locator
        if value["observation_key"] in seen:
            duplicates.append(value)
        else:
            seen.add(value["observation_key"])
            accepted.append(value)
    accepted.sort(key=lambda row: row["observation_key"])
    counts = {"input": len(rows) - header_record, "accepted": len(accepted), "duplicates": len(duplicates), "quarantined": len(quarantine)}
    manifest = json.loads((artifacts / "manifest.json").read_bytes())
    assert manifest["schema_version"] == "fine-history-prepared-v1" and manifest["input_contract"] == contract
    assert manifest["source_file"] == name and manifest["input_bytes"] == len(raw) and manifest["input_sha256"] == pinned
    assert manifest["counts"] == counts and manifest["header_record"] == header_record
    assert manifest["source_period"] == period and manifest["source_clock"] == clock and manifest["metadata_records"] == len(notes)
    outputs = {item["file"]: item for item in manifest["outputs"]}
    assert set(outputs) == {"history.jsonl", "duplicates.jsonl", "quarantine.jsonl", "source_notes.jsonl"}
    census = set()
    for name, expected in [("history.jsonl", accepted), ("duplicates.jsonl", duplicates), ("quarantine.jsonl", quarantine), ("source_notes.jsonl", notes)]:
        data = (artifacts / name).read_bytes()
        assert len(data) == outputs[name]["bytes"] and hashlib.sha256(data).hexdigest() == outputs[name]["sha256"]
        actual = [json.loads(line) for line in data.splitlines()]
        # Do not print private observations on disagreement.
        assert actual == expected, "whole-file minimized historical observation mismatch"
        if name != "source_notes.jsonl":
            for row in actual:
                locator = row.get("evidence_ref", row.get("row_locator"))
                assert locator not in census
                census.add(locator)
    assert len(census) == counts["input"]
    logical = [contract, period, clock, header_record, accepted, duplicates, quarantine, notes,
               counts["input"], counts["accepted"], counts["duplicates"], counts["quarantined"]]
    assert hashlib.sha256(canonical(logical)).hexdigest() == manifest["manifest_hash"]
    print(json.dumps({"period": period, "source_sha256": pinned, "counts": counts, "metadata_records": len(notes),
                      "unknown_paid_observations": sum(row["paid_minor"] is None for row in accepted), "oracle": "PASS"}, sort_keys=True))


if __name__ == "__main__":
    source_path, prepared_path, source_period = sys.argv[1:]
    verify(pathlib.Path(source_path), pathlib.Path(prepared_path), source_period)
