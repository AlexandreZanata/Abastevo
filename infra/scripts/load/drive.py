#!/usr/bin/env python3
"""Qualified load driver for the RST-12 campaign (stdlib only).

Closed loop (``--mode closed --clients N``) keeps N clients issuing back to
back; open arrival rate (``--mode open --rate RPS --clients MAX``) schedules
at a fixed cadence and records a drop whenever all MAX slots are busy
(missed scheduling is a counter, never silent load reduction).

Every request emits one schema-v1 sample row (see stats.py):
``scheduled_ms,started_ms,completed_ms,status,bytes`` in monotonic
milliseconds from run start. Layers stay separate by construction:
client scheduling (scheduled->started) vs API total (started->completed).
DB statements and EXPLAIN plans belong to diagnostic runs, never to timed
requests. Wall start is UTC ISO; intervals are monotonic.

Timeouts are right-censored at ``--deadline-ms`` and counted as failures.
``--expect-substring`` marks semantic mismatches as ``wrong``. Requests
still in flight after abandon record ``cancel``. Unwritable output fails
with REFUSED instead of losing samples silently.

Exit codes: 0 ok (budgets, if given, hold AND samples reconcile),
1 budget breached or mismatch, 2 usage/IO error.
"""
import socket
import sys
import threading
import time
import urllib.request
import urllib.error
from datetime import datetime, timezone


def _ms(now_ns, origin_ns):
    return (now_ns - origin_ns) / 1e6


class Recorder:
    def __init__(self, origin_ns):
        self.origin_ns = origin_ns
        self.lock = threading.Lock()
        self.rows = []
        self.inflight = 0
        self.max_inflight = 0
        self.max_lag = 0.0

    def scheduled(self):
        return time.monotonic_ns()

    def start(self, scheduled_ns):
        with self.lock:
            self.inflight += 1
            self.max_inflight = max(self.max_inflight, self.inflight)
        started_ns = time.monotonic_ns()
        lag = _ms(started_ns, scheduled_ns)
        with self.lock:
            self.max_lag = max(self.max_lag, lag)
        return started_ns

    def finish(self, scheduled_ns, started_ns, completed_ns, status, size):
        with self.lock:
            self.inflight -= 1
            self.rows.append((scheduled_ns, started_ns, completed_ns, status, size))

    def write_csv(self, path):
        lines = ["scheduled_ms,started_ms,completed_ms,status,bytes"]
        for scheduled_ns, started_ns, completed_ns, status, size in self.rows:
            fmt = lambda ns: "" if ns is None else "%.3f" % _ms(ns, self.origin_ns)
            lines.append("%s,%s,%s,%s,%d" % (
                fmt(scheduled_ns), fmt(started_ns), fmt(completed_ns), status, size))
        try:
            with open(path, "w") as fh:
                fh.write("\n".join(lines) + "\n")
        except OSError as err:
            raise ValueError("cannot write samples: %s" % err)


def run_request(recorder, url, deadline_s, expect, abandoned):
    """One attempt: semantic check first, then status class, else censored."""
    scheduled_ns = recorder.scheduled()
    started_ns = recorder.start(scheduled_ns)
    status, size, completed_ns = "cancel", 0, None
    try:
        try:
            code, body = fetch_body(url, deadline_s)
            if expect is not None and expect.encode() not in body:
                status, size = "wrong", len(body)
            else:
                status, size = code, len(body)
            completed_ns = time.monotonic_ns()
        except (socket.timeout, TimeoutError):
            status, size = "timeout", 0
            completed_ns = started_ns + int(deadline_s * 1e9)
        except Exception:
            # Refused/reset/unroutable: transport error, never a silent row.
            status, size = "error", 0
            completed_ns = time.monotonic_ns()
    finally:
        if abandoned.is_set() and completed_ns is not None:
            # Outcome arrived after harness teardown: too late to count.
            status, size, completed_ns = "cancel", 0, None
        recorder.finish(scheduled_ns, started_ns, completed_ns, status, size)


def fetch_body(url, deadline_s):
    try:
        with urllib.request.urlopen(url, timeout=deadline_s) as response:
            return str(response.status), response.read()
    except urllib.error.HTTPError as err:
        try:
            return str(err.code), err.read()
        except Exception:
            return str(err.code), b""


recorder = None


def drive(urls, mode, clients, rate, duration_s, deadline_s, out, expect, telemetry):
    # pylint: disable=global-statement
    global recorder
    origin_ns = time.monotonic_ns()
    recorder = Recorder(origin_ns)
    wall_start = datetime.now(timezone.utc).isoformat()
    abandoned = threading.Event()
    stop = origin_ns + int(duration_s * 1e9)
    threads = []

    def spawn(url):
        thread = threading.Thread(
            target=run_request, args=(recorder, url, deadline_s, expect, abandoned), daemon=True)
        thread.start()
        threads.append(thread)

    if mode == "closed":
        def worker(slot):
            index = 0
            while time.monotonic_ns() < stop:
                spawn(urls[(slot + index) % len(urls)])
                index += 1
                # Bound thread fan-out: wait for our own request before next.
                threads[-1].join()
        workers = [threading.Thread(target=worker, args=(s,), daemon=True) for s in range(clients)]
        for worker_thread in workers:
            worker_thread.start()
        for worker_thread in workers:
            remaining = max(0, (stop - time.monotonic_ns()) / 1e9 + deadline_s + 2)
            worker_thread.join(timeout=remaining)
    else:
        interval_ns = int(1e9 / rate)
        nxt = origin_ns
        while time.monotonic_ns() < stop:
            now = time.monotonic_ns()
            if now < nxt:
                time.sleep(min(0.005, (nxt - now) / 1e9))
                continue
            with recorder.lock:
                busy = recorder.inflight
            if busy >= clients:
                scheduled_ns = recorder.scheduled()
                recorder.finish(scheduled_ns, None, None, "drop", 0)
            else:
                spawn(urls[0] if len(urls) == 1 else urls[int(nxt) % len(urls)])
            nxt += interval_ns
            if nxt < now - 5 * interval_ns:
                nxt = now  # avoid unbounded catch-up burst after stalls

    abandoned.set()
    grace = deadline_s + 2.0
    deadline_wall = time.monotonic() + grace
    for thread in threads:
        remaining = deadline_wall - time.monotonic()
        if remaining <= 0:
            break
        thread.join(timeout=remaining)

    recorder.write_csv(out)
    scheduled = len(recorder.rows)
    dropped = sum(1 for r in recorder.rows if r[3] == "drop")
    manifest = (
        "wall_start=%s mode=%s clients=%d rate=%s duration_s=%.1f deadline_ms=%d "
        "telemetry=%s scheduled=%d max_inflight=%d max_sched_lag_ms=%.2f "
        "dropped=%d generator_limited=%s out=%s" % (
            wall_start, mode, clients, rate if mode == "open" else "-",
            duration_s, int(deadline_s * 1000), telemetry,
            scheduled, recorder.max_inflight, recorder.max_lag,
            dropped, "yes" if dropped else "no", out))
    print(manifest)
    return recorder


def parse_args(argv):
    args = {"mode": "closed", "telemetry": "off"}
    pos = 0
    while pos < len(argv):
        flag = argv[pos]
        if flag == "--self-test":
            return {"self_test": True}
        if flag in ("--url", "--urls-file", "--mode", "--clients", "--rate",
                    "--duration", "--deadline-ms", "--out", "--expect-substring",
                    "--telemetry"):
            if pos + 1 >= len(argv):
                raise ValueError("%s needs a value" % flag)
            args[flag[2:]] = argv[pos + 1]
            pos += 2
        else:
            raise ValueError("unknown flag %s" % flag)
    for key in ("mode", "clients", "duration", "deadline-ms", "out"):
        if key not in args:
            raise ValueError("missing --%s" % key)
    if args["mode"] not in ("closed", "open"):
        raise ValueError("mode must be closed|open")
    if args["mode"] == "open" and "rate" not in args:
        raise ValueError("open mode needs --rate")
    return args


def main(argv):
    try:
        args = parse_args(argv)
    except ValueError as err:
        print("REFUSED: %s" % err, file=sys.stderr)
        return 2
    if "self_test" in args:
        return self_test()
    if "url" in args:
        urls = [args["url"]]
    else:
        try:
            with open(args["urls-file"]) as fh:
                urls = [line.strip() for line in fh if line.strip()]
        except OSError as err:
            print("REFUSED: cannot read urls: %s" % err, file=sys.stderr)
            return 2
        if not urls:
            print("REFUSED: no urls", file=sys.stderr)
            return 2
    try:
        drive(urls,
              mode=args["mode"],
              clients=int(args["clients"]),
              rate=float(args.get("rate", 0)),
              duration_s=float(args["duration"]),
              deadline_s=float(args["deadline-ms"]) / 1000.0,
              out=args["out"],
              expect=args.get("expect-substring"),
              telemetry=args.get("telemetry", "off"))
    except ValueError as err:
        print("REFUSED: %s" % err, file=sys.stderr)
        return 2
    return 0


def self_test():
    """Loopback qualification: every error path plus counter reconciliation."""
    import http.server
    import tempfile
    import os

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):  # noqa: N802
            if self.path == "/slow":
                time.sleep(0.30)
                body = b"ok"
            elif self.path == "/error":
                self.send_response(500)
                self.end_headers()
                self.wfile.write(b"boom")
                return
            elif self.path == "/wrong":
                body = b"something-else"
            elif self.path == "/hang":
                time.sleep(2.0)
                body = b"late"
            else:
                body = b"must-appear"
            self.send_response(200)
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    server.daemon_threads = True
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base = "http://127.0.0.1:%d" % port
    tmp = tempfile.mkdtemp()
    failures = []

    def check(name, condition, detail=""):
        print(("PASS" if condition else "FAIL") + ": " + name
              + (" (%s)" % detail if detail and not condition else ""))
        if not condition:
            failures.append(name)

    def summarize_csv(path):
        sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__))))
        import stats
        return stats.summarize(stats.parse_samples(path))

    # Fast path reconciles exactly in closed mode.
    drive([base + "/fast"], "closed", 2, 0, 0.5, 2.0, tmp + "/fast.csv", "must-appear", "off")
    fast = summarize_csv(tmp + "/fast.csv")
    check("closed fast reconciles", fast["reconcile"] and fast["errors"] == 0,
          str({k: fast[k] for k in ("offered", "errors", "reconcile")}))
    check("closed fast all successful", fast["successful"] == fast["offered"] > 0)

    # Slow endpoint under a short deadline: timeouts are failures, counted.
    drive([base + "/slow"], "closed", 2, 0, 0.6, 0.1, tmp + "/slow.csv", None, "off")
    slow = summarize_csv(tmp + "/slow.csv")
    check("slow fixture timeouts counted", slow["timeouts"] > 0 and slow["reconcile"],
          str({k: slow[k] for k in ("timeouts", "reconcile")}))
    check("slow fixture never healthy", slow["errors"] > 0)

    # Error and wrong-result paths surface distinctly.
    drive([base + "/error"], "closed", 1, 0, 0.4, 2.0, tmp + "/err.csv", None, "off")
    err = summarize_csv(tmp + "/err.csv")
    check("non-2xx counted", err["error_codes"] == err["offered"] > 0)
    drive([base + "/wrong"], "closed", 1, 0, 0.4, 2.0, tmp + "/wrong.csv", "must-appear", "off")
    wrong = summarize_csv(tmp + "/wrong.csv")
    check("semantic mismatch is wrong", wrong["wrong"] == wrong["offered"] > 0)

    # Cancellation: hung requests abandoned at teardown record cancel.
    # Open mode is required: closed workers join each request, so the
    # abandon flag can only land mid-flight while the scheduler rests.
    drive([base + "/hang"], "open", 1, 5.0, 0.2, 5.0, tmp + "/cancel.csv", None, "off")
    cancelled = summarize_csv(tmp + "/cancel.csv")
    check("abandoned requests cancel", cancelled["cancelled"] > 0 and cancelled["reconcile"],
          str({k: cancelled[k] for k in ("cancelled", "reconcile")}))

    # Open mode under overload: missed scheduling becomes drops, still reconciled.
    drive([base + "/slow"], "open", 1, 50.0, 0.8, 2.0, tmp + "/open.csv", None, "off")
    overloaded = summarize_csv(tmp + "/open.csv")
    check("open overload drops, generator-limited",
          overloaded["dropped"] > 0 and overloaded["reconcile"],
          str({k: overloaded[k] for k in ("dropped", "reconcile")}))

    # Unwritable output refuses instead of losing samples.
    try:
        drive([base + "/fast"], "closed", 1, 0, 0.2, 2.0, tmp, None, "off")
    except ValueError:
        check("unwritable out refused", True)
    else:
        check("unwritable out refused", False)

    server.shutdown()
    print("drive self-test: %d failures" % len(failures))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
