#!/usr/bin/env python3
"""Produce the existing minimized RFB name input from an offline OpenCNPJ snapshot."""
import argparse
import csv
from datetime import date
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

REFERENCE = "https://arquivos.receitafederal.gov.br/dados/cnpj/dados_abertos_cnpj/"
MAX_ROWS = 500_000
BATCH_SIZE = 100  # OpenCNPJ bounds each filter value to 2,000 bytes.


def digest(path):
    value = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            value.update(block)
    return value.hexdigest()


def invoke(command):
    result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                            timeout=60, check=False)
    if result.returncode or len(result.stdout) > 32 << 20:
        raise ValueError("OpenCNPJ subprocess failed or exceeded its output budget")
    return result.stdout


def registry_ids(path):
    if Path(path).stat().st_size > 100 << 20:
        raise ValueError("registry exceeds byte budget")
    with Path(path).open(encoding="utf-8-sig", newline="") as stream:
        reader = csv.DictReader(stream, delimiter=";")
        if "CNPJ" not in (reader.fieldnames or []):
            raise ValueError("ANP CNPJ column missing")
        ids = set()
        for count, row in enumerate(reader, 1):
            if count > MAX_ROWS:
                raise ValueError("registry exceeds row budget")
            cnpj = re.sub(r"[./\-\s]", "", row.get("CNPJ") or "").upper()
            if not re.fullmatch(r"[A-Z0-9]{12}[0-9]{2}", cnpj):
                raise ValueError("invalid full registry CNPJ")
            ids.add(cnpj)
    if not ids:
        raise ValueError("registry is empty")
    return sorted(ids)


def export_names(binary, snapshot, expected_sha256, registry, output, run=invoke):
    snapshot, output = Path(snapshot), Path(output)
    if not re.fullmatch(r"[a-f0-9]{64}", expected_sha256) or digest(snapshot) != expected_sha256:
        raise ValueError("OpenCNPJ snapshot checksum mismatch")
    if output.exists():
        raise ValueError("output already exists")
    metadata = json.loads(run([str(binary), "info", str(snapshot)]))
    if not isinstance(metadata, dict):
        raise ValueError("invalid snapshot metadata")
    competence = metadata.get("competence", "")
    if not isinstance(competence, str) or not re.fullmatch(r"[0-9]{4}-(0[1-9]|1[0-2])", competence):
        raise ValueError("invalid snapshot competence")
    observed_at = date.fromisoformat(competence + "-01").isoformat()
    ids = registry_ids(registry)
    rows, seen = [], set()
    for offset in range(0, len(ids), BATCH_SIZE):
        batch = ids[offset:offset + BATCH_SIZE]
        raw = run([str(binary), "export", str(snapshot), "--filter", "cnpj=" + ",".join(batch),
                   "--max", str(len(batch) + 1)])
        for line in raw.splitlines():
            if len(line) > 256 << 10:
                raise ValueError("OpenCNPJ row exceeds byte budget")
            row = json.loads(line)
            if not isinstance(row, dict):
                raise ValueError("invalid establishment row")
            cnpj, trade, status = row.get("cnpj"), row.get("nome_fantasia"), row.get("situacao_cadastral")
            if cnpj not in batch or cnpj in seen:
                raise ValueError("unexpected or duplicate establishment CNPJ")
            if trade is None:
                trade = ""
            if not isinstance(trade, str) or len(trade.encode("utf-8")) > 2048:
                raise ValueError("invalid trade name")
            if status not in {"01", "02", "03", "04", "08"}:
                raise ValueError("unknown establishment status")
            seen.add(cnpj)
            # Only establishment identity/name/status survive the OpenCNPJ export.
            rows.append({"schema_version": "rfb-station-names-v1", "cnpj": cnpj,
                         "trade_name": trade, "active": status == "02",
                         "observed_at": observed_at})
    rows.sort(key=lambda row: row["cnpj"])
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".opencnpj-names-", dir=output.parent) as tmp:
        directory = Path(tmp) / "published"
        directory.mkdir(mode=0o700)
        stream = directory / "rfb-names.jsonl"
        with stream.open("w", encoding="utf-8") as target:
            for row in rows:
                target.write(json.dumps(row, ensure_ascii=False, separators=(",", ":")) + "\n")
                if target.tell() > 64 << 20:
                    raise ValueError("minimized names exceed byte budget")
        edition = "opencnpj-" + competence + "-sha256:" + expected_sha256
        manifest = {"format_version": "registry-name-inputs-v1", "inputs": [{
            "source": "rfb", "profile": "rfb-station-names-v1", "file": stream.name,
            "reference": REFERENCE + competence + "/", "edition": edition,
            "sha256": digest(stream), "bytes": stream.stat().st_size, "rows": len(rows)}]}
        summary = {"snapshot_sha256": expected_sha256, "competence": competence,
                   "source_snapshot": metadata.get("source_snapshot"),
                   "registry_sha256": digest(registry), "registry_cnpjs": len(ids),
                   "matched": len(rows), "not_found": len(ids) - len(rows),
                   "active": sum(row["active"] for row in rows),
                   "active_nonblank_names": sum(row["active"] and bool(row["trade_name"].strip()) for row in rows)}
        for name, body in [("manifest.json", manifest), ("summary.json", summary)]:
            (directory / name).write_text(json.dumps(body, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        for file in directory.iterdir():
            file.chmod(0o600)
        os.rename(directory, output)
    return summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("binary", "snapshot", "snapshot-sha256", "registry", "output"):
        parser.add_argument("--" + name, required=True)
    args = parser.parse_args()
    try:
        summary = export_names(args.binary, args.snapshot, args.snapshot_sha256, args.registry, args.output)
    except (ValueError, OSError, subprocess.SubprocessError, KeyError) as error:
        # Never print source rows, child output or personal fields on failure.
        parser.exit(1, "OpenCNPJ name export refused: " + type(error).__name__ + "\n")
    print(json.dumps(summary, sort_keys=True))


if __name__ == "__main__":
    main()
