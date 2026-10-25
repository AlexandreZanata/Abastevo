#!/usr/bin/env python3
"""Evaluate one bounded static-portal capacity campaign.

Reads the frozen workload, driver manifest, stats summary, Docker resource
samples and browser probe output. It never contacts the server/API. The result
is a local drill, not a CDN/VPS capacity acceptance.
"""
import argparse
import json
import math
import os
import sys

SCHEMA = "abastevo-portal-capacity-v1"


def parse_pairs(line):
    pairs = {}
    for token in line.split():
        if "=" not in token:
            continue
        key, value = token.split("=", 1)
        pairs[key] = value
    return pairs


def parse_size(text):
    text = text.strip().replace(",", "")
    units = {
        "B": 1,
        "kB": 1000, "KB": 1000,
        "KiB": 1024,
        "MB": 1000 ** 2, "MiB": 1024 ** 2,
        "GB": 1000 ** 3, "GiB": 1024 ** 3,
        "TB": 1000 ** 4, "TiB": 1024 ** 4,
    }
    for suffix in sorted(units, key=len, reverse=True):
        if text.endswith(suffix):
            number = text[: -len(suffix)].strip()
            return int(float(number) * units[suffix])
    return int(float(text))


def read_text(path, default=""):
    try:
        with open(path, encoding="utf-8") as handle:
            return handle.read()
    except OSError:
        return default


def load_stats(path):
    line = read_text(path).strip().splitlines()
    line = line[-1] if line else ""
    pairs = parse_pairs(line)
    if not pairs:
        raise ValueError("stats summary missing")
    numeric = {
        "offered": int, "started": int, "completed": int, "successful": int,
        "dropped": int, "cancelled": int, "timeouts": int, "wrong": int,
        "errors": int, "p50": float, "p95": float, "p99": float, "max": float,
        "err_rate": float, "window_s": float, "achieved_rps": float,
        "bytes_total": int, "bytes_per_s": float,
    }
    out = {}
    for key, kind in numeric.items():
        if key not in pairs:
            raise ValueError("stats missing %s" % key)
        out[key] = kind(float(pairs[key]) if kind is int else pairs[key])
    out["reconcile"] = pairs.get("reconcile") == "ok"
    return out


def load_driver(path):
    for line in read_text(path).splitlines():
        if line.startswith("wall_start="):
            return parse_pairs(line)
    raise ValueError("driver manifest missing")


def load_resources(path):
    cpu_max = 0.0
    mem_max = 0
    net_rx = net_tx = block_read = block_write = 0
    samples = 0
    for line in read_text(path).splitlines():
        fields = line.strip().split("|")
        if len(fields) != 4:
            continue
        try:
            cpu = float(fields[0].strip().rstrip("%"))
        except ValueError:
            continue
        mem_used = fields[1].split("/")[0].strip()
        net = fields[2].split("/")
        block = fields[3].split("/")
        cpu_max = max(cpu_max, cpu)
        mem_max = max(mem_max, parse_size(mem_used))
        net_rx = parse_size(net[0].strip())
        net_tx = parse_size(net[1].strip()) if len(net) > 1 else 0
        block_read = parse_size(block[0].strip())
        block_write = parse_size(block[1].strip()) if len(block) > 1 else 0
        samples += 1
    if samples == 0:
        raise ValueError("docker resource samples missing")
    return {
        "samples": samples,
        "cpu_max_pct": cpu_max,
        "mem_max_mib": mem_max / (1024 ** 2),
        "net_rx_bytes": net_rx,
        "net_tx_bytes": net_tx,
        "block_read_bytes": block_read,
        "block_write_bytes": block_write,
    }


def load_browser(path):
    try:
        with open(path, encoding="utf-8") as handle:
            data = json.load(handle)
    except OSError as err:
        raise ValueError("browser output missing: %s" % err)
    if data.get("schema_version") != "abastevo-portal-browser-v1":
        raise ValueError("browser schema mismatch")
    if not data.get("summary"):
        raise ValueError("browser summary missing")
    return data


def load_workload(path):
    try:
        with open(path, encoding="utf-8") as handle:
            return json.load(handle)
    except OSError:
        return {}


def evaluate(results, budgets):
    failures = []
    load = load_stats(os.path.join(results, "stats.txt"))
    driver = load_driver(os.path.join(results, "driver.log"))
    resources = load_resources(os.path.join(results, "docker-stats.txt"))
    browser = load_browser(os.path.join(results, "browser.json"))
    workload = load_workload(os.path.join(results, "workload.json"))

    def fail(message):
        failures.append(message)

    if not load["reconcile"]:
        fail("sample counters do not reconcile")
    if load["dropped"] or driver.get("generator_limited") == "yes":
        fail("load generator dropped scheduled requests")
    if load["cancelled"]:
        fail("requests were cancelled at teardown")
    if load["timeouts"]:
        fail("request timeouts observed")
    if load["wrong"]:
        fail("semantic mismatch observed")
    if load["err_rate"] > budgets["err"]:
        fail("error rate %.4f > %.4f" % (load["err_rate"], budgets["err"]))
    if load["p95"] > budgets["p95"]:
        fail("p95 %.2f ms > %.2f ms" % (load["p95"], budgets["p95"]))
    if load["p99"] > budgets["p99"]:
        fail("p99 %.2f ms > %.2f ms" % (load["p99"], budgets["p99"]))
    if load["achieved_rps"] < budgets["min_rps"]:
        fail("achieved %.2f rps < %.2f rps" % (load["achieved_rps"], budgets["min_rps"]))
    if resources["cpu_max_pct"] > budgets["cpu"]:
        fail("CPU %.2f%% > %.2f%%" % (resources["cpu_max_pct"], budgets["cpu"]))
    if resources["mem_max_mib"] > budgets["mem"]:
        fail("memory %.2f MiB > %.2f MiB" % (resources["mem_max_mib"], budgets["mem"]))

    expected = len(browser.get("pages", [])) * len(browser.get("viewports", []))
    if len(browser["summary"]) != expected:
        fail("browser summary rows %d != expected %d" % (len(browser["summary"]), expected))
    for row in browser["summary"]:
        name = "%s/%s" % (row.get("viewport"), row.get("page"))
        if row.get("fcp_unavailable") or row.get("fcp_median_ms") is None:
            fail("%s FCP unavailable" % name)
        elif row["fcp_median_ms"] > budgets["fcp"]:
            fail("%s FCP %.2f ms > %.2f ms" % (name, row["fcp_median_ms"], budgets["fcp"]))
        if row.get("lcp_unavailable") or row.get("lcp_median_ms") is None:
            fail("%s LCP unavailable" % name)
        elif row["lcp_median_ms"] > budgets["lcp"]:
            fail("%s LCP %.2f ms > %.2f ms" % (name, row["lcp_median_ms"], budgets["lcp"]))
        if row.get("cls_median") is None or row["cls_median"] > budgets["cls"]:
            fail("%s CLS %.4f > %.4f" % (name, row.get("cls_median") or 0, budgets["cls"]))
        if row.get("errors"):
            fail("%s browser errors=%d" % (name, row["errors"]))
        if row.get("external_resources"):
            fail("%s external resources=%d" % (name, len(row["external_resources"])))
        if row.get("api_resources"):
            fail("%s API resources=%d" % (name, len(row["api_resources"])))

    summary = {
        "schema_version": SCHEMA,
        "passed": not failures,
        "failures": failures,
        "budgets": budgets,
        "load": load,
        "driver": driver,
        "resources": resources,
        "browser": browser["summary"],
        "artifact": workload.get("artifact", {}),
        "hardware": workload.get("hardware", {}),
        "workload": workload.get("workload", {}),
        "scope": "isolated local container drill; not CDN/VPS/production capacity",
    }
    return summary


def self_test():
    import tempfile
    root = tempfile.mkdtemp(prefix="portal-report-selftest-")
    with open(os.path.join(root, "stats.txt"), "w", encoding="utf-8") as handle:
        handle.write(
            "offered=990 started=990 completed=990 successful=990 dropped=0 "
            "cancelled=0 timeouts=0 wrong=0 errors=0 err_rate=0.0000 "
            "p50=2.0 p95=5.0 p99=8.0 max=10.0 sched_p50=0.1 sched_max=1.0 "
            "window_s=10.000 achieved_rps=99.00 bytes_total=1000 bytes_per_s=100.00 "
            "reconcile=ok\n")
    with open(os.path.join(root, "driver.log"), "w", encoding="utf-8") as handle:
        handle.write("wall_start=now mode=open clients=32 rate=100 generator_limited=no "
                     "scheduled=990 max_inflight=4 dropped=0\n")
    with open(os.path.join(root, "docker-stats.txt"), "w", encoding="utf-8") as handle:
        handle.write("10.00%|100MiB / 1GiB|1MB / 2MB|3MB / 4MB\n")
    with open(os.path.join(root, "browser.json"), "w", encoding="utf-8") as handle:
        json.dump({
            "schema_version": "abastevo-portal-browser-v1",
            "pages": ["/consultar/"],
            "viewports": ["desktop"],
            "summary": [{
                "page": "/consultar/", "viewport": "desktop", "fcp_median_ms": 100,
                "lcp_median_ms": 200, "cls_median": 0.01, "errors": 0,
                "external_resources": [], "api_resources": [],
            }],
        }, handle)
    with open(os.path.join(root, "workload.json"), "w", encoding="utf-8") as handle:
        json.dump({"artifact": {"content_sha256": "sha256:fixture"}, "hardware": {}, "workload": {}}, handle)
    budgets = {"p95": 50, "p99": 100, "err": 0.01, "cpu": 80, "mem": 256,
               "fcp": 1500, "lcp": 2500, "cls": 0.1, "min_rps": 90}
    summary = evaluate(root, budgets)
    assert summary["passed"], summary
    with open(os.path.join(root, "stats.txt"), "w", encoding="utf-8") as handle:
        handle.write(
            "offered=990 started=990 completed=990 successful=990 dropped=0 "
            "cancelled=0 timeouts=0 wrong=0 errors=0 err_rate=0.0000 "
            "p50=2.0 p95=999.0 p99=999.0 max=999.0 sched_p50=0.1 sched_max=1.0 "
            "window_s=10.000 achieved_rps=99.00 bytes_total=1000 bytes_per_s=100.00 "
            "reconcile=ok\n")
    summary = evaluate(root, budgets)
    assert not summary["passed"] and any("p95" in item for item in summary["failures"]), summary
    print("portal report self-test ok")
    return 0


def main(argv):
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--results")
    parser.add_argument("--p95", type=float, default=50.0)
    parser.add_argument("--p99", type=float, default=100.0)
    parser.add_argument("--err", type=float, default=0.01)
    parser.add_argument("--cpu", type=float, default=80.0)
    parser.add_argument("--mem", type=float, default=256.0)
    parser.add_argument("--fcp", type=float, default=1500.0)
    parser.add_argument("--lcp", type=float, default=2500.0)
    parser.add_argument("--cls", type=float, default=0.1)
    parser.add_argument("--min-rps", type=float, default=90.0)
    args = parser.parse_args(argv)
    if args.self_test:
        return self_test()
    if not args.results:
        parser.error("--results is required")
    budgets = {
        "p95": args.p95, "p99": args.p99, "err": args.err,
        "cpu": args.cpu, "mem": args.mem, "fcp": args.fcp,
        "lcp": args.lcp, "cls": args.cls, "min_rps": args.min_rps,
    }
    try:
        summary = evaluate(args.results, budgets)
    except ValueError as err:
        print("REFUSED: %s" % err, file=sys.stderr)
        return 2
    with open(os.path.join(args.results, "summary.json"), "w", encoding="utf-8") as handle:
        json.dump(summary, handle, indent=2)
        handle.write("\n")
    load = summary["load"]
    resources = summary["resources"]
    print(
        "load offered=%d successful=%d achieved=%.2f rps p95=%.2f ms p99=%.2f ms "
        "err=%.4f bytes=%d (%.2f B/s) drops=%d; caddy cpu_max=%.2f%% mem_max=%.2f MiB "
        "net=%d/%d B block=%d/%d B; browser rows=%d" % (
            load["offered"], load["successful"], load["achieved_rps"],
            load["p95"], load["p99"], load["err_rate"], load["bytes_total"],
            load["bytes_per_s"], load["dropped"], resources["cpu_max_pct"],
            resources["mem_max_mib"], resources["net_rx_bytes"],
            resources["net_tx_bytes"], resources["block_read_bytes"],
            resources["block_write_bytes"], len(summary["browser"])))
    if not summary["passed"]:
        for failure in summary["failures"]:
            print("FAIL: %s" % failure, file=sys.stderr)
        return 1
    print("PASS: bounded static-portal campaign within predeclared budgets")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
