#!/usr/bin/env python3
"""Latency statistics for the load harness (P08-T07).

Reads a CSV of seconds-per-request (one value per line) and prints
count/p50/p95/p99/max plus the 5xx rate when given a second file with
status codes. `--self-test` runs the deterministic unit check.
"""
import math
import sys


def percentiles(values):
    ordered = sorted(values)
    n = len(ordered)
    if n == 0:
        raise ValueError("no samples")
    def pick(p):
        idx = min(n - 1, math.ceil(p * n) - 1)
        return ordered[max(0, idx)]
    return {
        "count": n,
        "p50": pick(0.50),
        "p95": pick(0.95),
        "p99": pick(0.99),
        "max": ordered[-1],
    }


def main(argv):
    if "--self-test" in argv:
        samples = [float(i) / 1000.0 for i in range(1, 1001)]
        stats = percentiles(samples)
        assert stats["count"] == 1000, stats
        assert abs(stats["p50"] - 0.500) < 1e-9, stats
        assert abs(stats["p95"] - 0.950) < 1e-9, stats
        assert abs(stats["p99"] - 0.990) < 1e-9, stats
        assert abs(stats["max"] - 1.000) < 1e-9, stats
        print("stats self-test ok")
        return 0
    path = argv[1]
    with open(path) as fh:
        values = [float(line.strip()) for line in fh if line.strip()]
    stats = percentiles(values)
    print("count=%d p50=%.4f p95=%.4f p99=%.4f max=%.4f" % (
        stats["count"], stats["p50"], stats["p95"], stats["p99"], stats["max"]))
    if len(argv) > 2:
        with open(argv[2]) as fh:
            codes = [line.strip() for line in fh if line.strip()]
        bad = sum(1 for c in codes if c.startswith("5"))
        print("requests=%d unexpected_5xx=%d rate=%.4f" % (
            len(codes), bad, bad / max(1, len(codes))))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
