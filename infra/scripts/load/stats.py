#!/usr/bin/env python3
"""Latency statistics for the load harness (P08-T07, extended RST-12).

Legacy mode (unchanged): ``stats.py LAT CODES`` reads seconds-per-request
plus status codes and prints count/p50/p95/p99/max and the 5xx rate.

RST-12 sample mode: ``stats.py --samples FILE [--budget-p95-ms N
[--budget-err-rate R]]`` reads the raw-sample schema v1 and prints one
machine-readable summary line, exiting nonzero on budget breach (a slow
or erroring fixture can never report healthy) or on malformed input.

Raw-sample schema v1 (CSV, monotonic milliseconds, header optional):
``scheduled_ms,started_ms,completed_ms,status,bytes``. Empty
``started_ms`` means never started (drop); empty ``completed_ms`` with a
``started_ms`` means cancelled. ``status`` is a 3-digit code, ``timeout``
(right-censored at the deadline), ``drop``, ``wrong`` (semantic
mismatch), ``error`` (transport failure) or ``cancel``. Timeouts are
failures, never omitted samples.

Exit codes: 0 pass, 1 budget breached, 2 input/IO error.
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


# Fixed histogram buckets in ms; merge is elementwise addition, so split
# runs combine into exactly the whole-run histogram.
HISTOGRAM_BUCKETS = (1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000)


def histogram_of(values):
    counts = [0] * (len(HISTOGRAM_BUCKETS) + 1)
    for value in values:
        placed = False
        for index, bound in enumerate(HISTOGRAM_BUCKETS):
            if value < bound:
                counts[index] += 1
                placed = True
                break
        if not placed:
            counts[-1] += 1
    return counts


def merge_histograms(first, second):
    if len(first) != len(second):
        raise ValueError("histogram bucket mismatch")
    return [a + b for a, b in zip(first, second)]


def _number(raw, what, lineno):
    try:
        return float(raw)
    except ValueError:
        raise ValueError("line %d: bad %s %r" % (lineno, what, raw))


def parse_samples(path):
    """Read schema-v1 samples; truncated lines fail loudly with a number."""
    try:
        with open(path) as fh:
            return parse_samples_file(fh, path)
    except IsADirectoryError:
        raise ValueError("%s: not a sample file" % path)
    except OSError as err:
        raise ValueError("%s: cannot read samples: %s" % (path, err))


def _is_success(status):
    return len(status) == 3 and status.startswith("2")


def summarize(samples):
    offered = len(samples)
    started = sum(1 for s in samples if s["started"] is not None)
    completed = sum(1 for s in samples if s["completed"] is not None)
    dropped = sum(1 for s in samples if s["status"] == "drop")
    cancelled = sum(1 for s in samples if s["status"] == "cancel")
    timeouts = sum(1 for s in samples if s["status"] == "timeout")
    wrong = sum(1 for s in samples if s["status"] == "wrong")
    successful = sum(
        1 for s in samples if s["completed"] is not None and _is_success(s["status"]))
    error_codes = completed - successful - timeouts - wrong
    errors = timeouts + wrong + error_codes
    latencies = [s["completed"] - s["started"] for s in samples
                 if s["completed"] is not None and s["started"] is not None
                 and _is_success(s["status"])]
    sched = [s["started"] - s["scheduled"] for s in samples if s["started"] is not None]
    latency = percentiles(latencies) if latencies else None
    histogram = histogram_of(latencies)
    reconcile = (offered == completed + dropped + cancelled
                 and completed == successful + timeouts + wrong + error_codes
                 and error_codes >= 0)
    return {
        "offered": offered,
        "started": started,
        "completed": completed,
        "successful": successful,
        "dropped": dropped,
        "cancelled": cancelled,
        "timeouts": timeouts,
        "wrong": wrong,
        "error_codes": error_codes,
        "errors": errors,
        "err_rate": errors / max(1, offered),
        "latency": latency,
        "histogram": histogram,
        "sched_p50": percentiles(sched)["p50"] if sched else 0.0,
        "sched_max": max(sched) if sched else 0.0,
        "reconcile": reconcile,
    }


def compare_overhead(base, trial):
    """Paired on/off-telemetry report: deltas plus a watch flag (|p95| > 10%).

    A reporting aid for paired trials, not an acceptance gate: noisy
    comparisons stay INCONCLUSIVE and are rerun, never averaged green.
    """
    deltas = {}
    verdict = "ok"
    for key in ("p50", "p95"):
        before = (base["latency"] or {}).get(key)
        after = (trial["latency"] or {}).get(key)
        if before is None or after is None:
            deltas[key] = None
            continue
        deltas[key] = after - before
        if before > 0 and abs(after - before) / before > 0.10:
            verdict = "watch"
    return {"d_p50_ms": deltas.get("p50"), "d_p95_ms": deltas.get("p95"), "verdict": verdict}


def summary_line(summary):
    latency = summary["latency"] or {"count": 0, "p50": 0.0, "p95": 0.0, "p99": 0.0, "max": 0.0}
    return (
        "offered=%d started=%d completed=%d successful=%d dropped=%d cancelled=%d "
        "timeouts=%d wrong=%d errors=%d err_rate=%.4f "
        "p50=%.2f p95=%.2f p99=%.2f max=%.2f sched_p50=%.2f sched_max=%.2f reconcile=%s" % (
            summary["offered"], summary["started"], summary["completed"],
            summary["successful"], summary["dropped"], summary["cancelled"],
            summary["timeouts"], summary["wrong"], summary["errors"], summary["err_rate"],
            latency["p50"], latency["p95"], latency["p99"], latency["max"],
            summary["sched_p50"], summary["sched_max"],
            "ok" if summary["reconcile"] else "MISMATCH"))


def _self_test():
    samples = [float(i) / 1000.0 for i in range(1, 1001)]
    stats = percentiles(samples)
    assert stats["count"] == 1000, stats
    assert abs(stats["p50"] - 0.500) < 1e-9, stats
    assert abs(stats["p95"] - 0.950) < 1e-9, stats
    assert abs(stats["p99"] - 0.990) < 1e-9, stats
    assert abs(stats["max"] - 1.000) < 1e-9, stats

    def row(sched, start, done, status, size=100):
        fmt = lambda v: "" if v is None else str(v)
        return "%s,%s,%s,%s,%s" % (fmt(sched), fmt(start), fmt(done), status, size)

    # Slow fixture: p95 breach is reported, never healthy.
    slow = [row(i, i, i + 5, "200") for i in range(90)]
    slow += [row(i, i, i + 500, "200") for i in range(90, 100)]
    slow_summary = summarize(parse_samples_lines(slow))
    assert slow_summary["latency"]["p95"] == 500.0, slow_summary
    assert slow_summary["reconcile"], slow_summary

    # Error/drop/timeout fixture: every class counted, denominators explicit.
    mixed = [row(i, i, i + 5, "200") for i in range(80)]
    mixed += [row(i, i, i + 5, "500") for i in range(80, 85)]
    mixed += [row(i, i, i + 1000, "timeout") for i in range(85, 90)]
    mixed += [row(i, i, i + 5, "wrong") for i in range(90, 95)]
    mixed += [row(i, None, None, "drop") for i in range(95, 98)]
    mixed += [row(i, i, None, "cancel") for i in range(98, 100)]
    mixed_summary = summarize(parse_samples_lines(mixed))
    assert mixed_summary["offered"] == 100, mixed_summary
    assert mixed_summary["successful"] == 80, mixed_summary
    assert mixed_summary["errors"] == 5 + 5 + 5, mixed_summary
    assert abs(mixed_summary["err_rate"] - 0.15) < 1e-9, mixed_summary
    assert mixed_summary["dropped"] == 3 and mixed_summary["cancelled"] == 2
    assert mixed_summary["reconcile"], mixed_summary

    # Histogram merge: split halves combine into exactly the whole run.
    half = len(slow) // 2
    whole = histogram_of([5.0] * 90 + [500.0] * 10)
    assert merge_histograms(histogram_of([5.0] * 45 + [500.0] * 5),
                            histogram_of([5.0] * 45 + [500.0] * 5)) == whole
    assert slow_summary["histogram"] == whole, slow_summary["histogram"]
    try:
        merge_histograms([1], [1, 2])
    except ValueError:
        pass
    else:
        raise AssertionError("bucket mismatch must fail")

    # Truncated input fails loudly instead of skewing the report.
    try:
        parse_samples_lines(["1,1,6,200,100", "2,2"])
    except ValueError as err:
        assert "line 2" in str(err), err
    else:
        raise AssertionError("truncated sample must fail")
    try:
        parse_samples_lines([])
    except ValueError:
        pass
    else:
        raise AssertionError("empty input must fail")

    # Overhead aid: identical trials are flat, degraded trials raise watch.
    flat = compare_overhead(slow_summary, slow_summary)
    assert flat["verdict"] == "ok" and flat["d_p95_ms"] == 0.0, flat
    degraded = dict(slow_summary)
    degraded["latency"] = dict(slow_summary["latency"], p95=600.0)
    watch = compare_overhead(slow_summary, degraded)
    assert watch["verdict"] == "watch" and watch["d_p95_ms"] == 100.0, watch

    print("stats self-test ok")
    return 0


def parse_samples_lines(lines):
    import io
    return parse_samples_file(io.StringIO("\n".join(lines) + "\n"), "<self-test>")


def parse_samples_file(fh, path):
    lines = fh.read().splitlines()
    samples = []
    for lineno, line in enumerate(lines, 1):
        text = line.strip()
        if not text:
            continue
        if lineno == 1 and text.startswith("scheduled_ms"):
            continue
        fields = text.split(",")
        if len(fields) != 5:
            raise ValueError("line %d: truncated sample (%d fields)" % (lineno, len(fields)))
        scheduled_raw, started_raw, completed_raw, status, bytes_raw = fields
        scheduled = _number(scheduled_raw, "scheduled_ms", lineno)
        started = _number(started_raw, "started_ms", lineno) if started_raw else None
        completed = _number(completed_raw, "completed_ms", lineno) if completed_raw else None
        status = status.strip()
        if not status:
            raise ValueError("line %d: empty status" % lineno)
        samples.append({
            "scheduled": scheduled,
            "started": started,
            "completed": completed,
            "status": status,
            "bytes": _number(bytes_raw, "bytes", lineno),
        })
    if not samples:
        raise ValueError("%s: no samples" % path)
    return samples


def main(argv):
    if "--self-test" in argv:
        return _self_test()
    if "--samples" in argv:
        idx = argv.index("--samples")
        try:
            path = argv[idx + 1]
        except IndexError:
            print("REFUSED: --samples needs a file", file=sys.stderr)
            return 2
        budget_p95 = None
        budget_err = None
        if "--budget-p95-ms" in argv:
            budget_p95 = float(argv[argv.index("--budget-p95-ms") + 1])
        if "--budget-err-rate" in argv:
            budget_err = float(argv[argv.index("--budget-err-rate") + 1])
        try:
            summary = summarize(parse_samples(path))
        except ValueError as err:
            print("REFUSED: %s" % err, file=sys.stderr)
            return 2
        print(summary_line(summary))
        if not summary["reconcile"]:
            print("FAIL: samples do not reconcile", file=sys.stderr)
            return 1
        if budget_p95 is not None and (summary["latency"] or {"p95": float("inf")})["p95"] > budget_p95:
            print("FAIL: p95 budget breached", file=sys.stderr)
            return 1
        if budget_err is not None and summary["err_rate"] > budget_err:
            print("FAIL: error budget breached", file=sys.stderr)
            return 1
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
