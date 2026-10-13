#!/usr/bin/env python3
"""Reproduce counter deltas from the bounded, aggregate-only observation."""
import gzip
import json
from datetime import datetime, timezone
from pathlib import Path


def delta(a, b):
    ticks = sum(b["host_cpu_ticks"]) - sum(a["host_cpu_ticks"])
    stolen = b["host_cpu_ticks"][7] - a["host_cpu_ticks"][7]
    seconds = b["monotonic"] - a["monotonic"]
    if ticks <= 0 or stolen < 0 or stolen > ticks or seconds <= 0:
        raise ValueError("counter reset/invalid observation; do not qualify")
    return {"steal_percent": 100 * stolen / ticks, "seconds": seconds,
            "until_utc": datetime.fromtimestamp(b["wall"], timezone.utc).isoformat()}


def summarize(rows):
    if len(rows) < 2:
        raise ValueError("insufficient samples")
    intervals = [delta(a, b) for a, b in zip(rows, rows[1:])]
    windows = []
    start = 0
    for end in range(1, len(rows)):
        while start + 1 < end and rows[end]["monotonic"] - rows[start + 1]["monotonic"] >= 5:
            start += 1
        if rows[end]["monotonic"] - rows[start]["monotonic"] >= 5:
            windows.append(delta(rows[start], rows[end]))
    before, after = rows[0]["postgres_cpu"], rows[-1]["postgres_cpu"]
    return {"samples": len(rows), "overall": delta(rows[0], rows[-1]),
            "peak_sample": max(intervals, key=lambda r: r["steal_percent"]),
            "peak_window_at_least_5s": max(windows, key=lambda r: r["steal_percent"]) if windows else None,
            "postgres_cpu_seconds": (after["usage_usec"] - before["usage_usec"]) / 1e6,
            "postgres_throttled_seconds": (after["throttled_usec"] - before["throttled_usec"]) / 1e6,
            "postgres_throttled_periods": after["nr_throttled"] - before["nr_throttled"],
            "capacity_qualified": False, "causal_attribution": "not proven"}


if __name__ == "__main__":
    path = Path(__file__).with_name("cpu-observation.jsonl.gz")
    with gzip.open(path, "rt") as source:
        rows = [json.loads(line) for line in source]
    print(json.dumps(summarize(rows), indent=2))
