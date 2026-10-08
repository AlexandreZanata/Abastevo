#!/usr/bin/env python3
"""Score private device output against anonymous exact-value annotations.

Never copy tokens, labels, coordinates or image bytes to the report. A successful
invocation means the evaluation ran, not that the model met a release threshold.
"""
import argparse
import hashlib
import json
import statistics
from pathlib import Path


def score(manifest, records, images=None, require_device_hash=False):
    samples = manifest["samples"]
    expected = {s["file"]: s for s in samples}
    actual = {r["file"]: r for r in records}
    if len(expected) != len(samples) or len(actual) != len(records):
        raise ValueError("duplicate sample or result")
    if expected.keys() != actual.keys():
        raise ValueError("sample/result membership mismatch")
    outcomes = []
    for name, sample in expected.items():
        record = actual[name]
        if require_device_hash and "input_sha256" not in record:
            raise ValueError("device input identity missing")
        if images is not None:
            if Path(name).name != name:
                raise ValueError("unsafe sample path")
            digest = hashlib.sha256((images / name).read_bytes()).hexdigest()
            if digest != sample["sha256"]:
                raise ValueError("image identity mismatch")
        if record.get("input_sha256") is not None and record["input_sha256"] != sample["sha256"]:
            raise ValueError("device input identity mismatch")
        want = {(r["fuel"], r["milli_brl"]) for r in sample["expected_rows"]}
        visible = {amount for _, amount in want} | set(sample.get("manual_prices", []))
        got = {(r["fuel"], r["milli_brl"]) for r in record.get("rows", [])}
        if len(got) != len(record.get("rows", [])):
            raise ValueError("duplicate suggestion")
        outcomes.append({
            "file": name, "tags": sample["tags"], "expected": len(want),
            "exact": len(want & got), "missed": len(want - got), "wrong": len(got - want),
            "failed": int(record["status"] != "recognized"),
            "condition_missed": int(sample.get("conditional_required", False) and not record.get("conditional", False)),
            "manual_expected": len(set(sample.get("manual_prices", []))),
            "manual_recovered": len(set(sample.get("manual_prices", [])) & set(record["orphans"])) if "orphans" in record else None,
            "unassigned_exact": len(set(record["orphans"]) & visible) if "orphans" in record else None,
            "unassigned_wrong": len(set(record["orphans"]) - visible) if "orphans" in record else None,
            "elapsed_ms": record.get("recognition_ms", record["elapsed_ms"]),
        })

    def aggregate(items):
        keys = ("expected", "exact", "missed", "wrong", "failed", "condition_missed", "manual_expected")
        out = {k: sum(r[k] for r in items) for k in keys}
        measured = [r["manual_recovered"] for r in items if r["manual_recovered"] is not None]
        out["manual_measured_images"] = len(measured)
        out["manual_recovered"] = sum(measured) if measured else None
        for key in ["unassigned_exact", "unassigned_wrong"]:
            known = [r[key] for r in items if r[key] is not None]
            out[key] = sum(known) if known else None
        out["images"] = len(items)
        out["precision"] = out["exact"] / (out["exact"] + out["wrong"]) if out["exact"] + out["wrong"] else None
        out["recall"] = out["exact"] / out["expected"] if out["expected"] else None
        return out

    times = sorted(r["elapsed_ms"] for r in outcomes)
    return {
        "schema_version": 1, "summary": aggregate(outcomes),
        "latency_ms": {"p50": statistics.median(times), "p95": times[max(0, (95 * len(times) + 99) // 100 - 1)]},
        "by_tag": {tag: aggregate([r for r in outcomes if tag in r["tags"]]) for tag in sorted({tag for r in outcomes for tag in r["tags"]})},
        "samples": outcomes,
        "scope": "Targeted regression corpus. Tags overlap. Exact fuel AND milli-BRL required. Wrong values count as both wrong and missed. No population/release acceptance inferred.",
    }


def check_budget(report, minimum_exact=0, maximum_wrong=None):
    summary = report["summary"]
    if summary["exact"] < minimum_exact or (maximum_wrong is not None and summary["wrong"] > maximum_wrong):
        raise ValueError("exact-value budget failed")
    if summary["failed"] or summary["condition_missed"]:
        raise ValueError("recognition or required condition failed")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("results", type=Path)
    parser.add_argument("--images", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--require-device-hash", action="store_true")
    parser.add_argument("--minimum-exact", type=int, default=0)
    parser.add_argument("--maximum-wrong", type=int)
    args = parser.parse_args()
    try:
        report = score(json.loads(args.manifest.read_text()), json.loads(args.results.read_text()), args.images, args.require_device_hash)
    except (ValueError, KeyError, OSError, TypeError):
        parser.exit(1, "Evaluation refused: invalid input, membership or image identity.\n")
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"summary": report["summary"], "latency_ms": report["latency_ms"]}))
    try:
        check_budget(report, args.minimum_exact, args.maximum_wrong)
    except ValueError:
        parser.exit(1, "Evaluation budget failed; inspect the sanitized report.\n")


if __name__ == "__main__":
    main()
