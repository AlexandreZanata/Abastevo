#!/usr/bin/env python3
"""Bounded, retry-free staging rollout proof; Python stdlib only."""
import argparse
from concurrent.futures import ThreadPoolExecutor, wait
from datetime import datetime, timezone
import hashlib
import http.client
import json
import math
import os
from pathlib import Path
import stat
import subprocess
import threading
import time
from urllib.parse import urlsplit

USER_AGENT = "abastevo-rollout-probe/1.0"
ROUTES = [("ready", "/health/ready"), ("stations", "/v1/stations?limit=1"),
          ("nearby", "/v1/stations/nearby?lat=-15.8&lon=-47.9&radius_m=3000&limit=1"),
          ("landing", "/")]

def stamp():
    return datetime.now(timezone.utc).isoformat()

def origin(raw):
    u = urlsplit(raw)
    if u.username or u.password or u.fragment or u.scheme not in ("http", "https"):
        raise ValueError("invalid probe URL")
    if not ((u.scheme == "https" and u.hostname == "abastevo.com.br" and u.port in (None, 443))
            or u.hostname in ("127.0.0.1", "localhost", "::1")):
        raise ValueError("probe is restricted to the owned official host or loopback fixtures")
    return u

def request(label, raw, timeout, upload=None):
    started = time.monotonic()
    row = {"kind":"request", "route":label, "started_at":stamp()}
    conn = None
    try:
        u = origin(raw)
        cls = http.client.HTTPSConnection if u.scheme == "https" else http.client.HTTPConnection
        conn = cls(u.hostname, u.port, timeout=timeout)
        path = u.path or "/"
        if u.query:
            path += "?" + u.query
        headers = {"User-Agent":USER_AGENT}
        if upload:
            headers.update(upload.get("headers", {}))
            headers["User-Agent"] = USER_AGENT
            headers["Content-Length"] = str(upload["bytes"])
            conn.putrequest("PUT", path)
            for key, value in headers.items():
                conn.putheader(key, value)
            conn.endheaders()
            sent = 0
            while sent < upload["bytes"]:
                size = min(8192, upload["bytes"] - sent)
                conn.send(b"x" * size)  # Synthetic bytes only, never a production photo.
                sent += size
                time.sleep(size / upload["bytes_per_second"])
        else:
            conn.request("GET", path, headers=headers)
        response = conn.getresponse()
        row["status"] = response.status
        total = 0
        while True:
            chunk = response.read(65536)
            if not chunk:
                break
            total += len(chunk)
            if total > 2 * 1024 * 1024:
                raise ValueError("response size bound exceeded")
        row["ok"] = 200 <= response.status < 300
    except Exception as error:
        row.update(ok=False, error=type(error).__name__)
    finally:
        if conn:
            conn.close()
    row.update(latency_ms=round((time.monotonic()-started)*1000, 3), finished_at=stamp())
    return row

class Watch:
    def __init__(self, executable, emit):
        self.emit = emit
        self.rows = []
        self.failed = False
        self.initial = threading.Event()
        self.process = subprocess.Popen([executable, "-n", "abastevo-temp", "get", "deployment",
            "abastevo-temp-api", "--watch", "--output-watch-events", "-o", "json"],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
        self.thread = threading.Thread(target=self.read, daemon=True)
        self.thread.start()

    def read(self):
        buffer = ""
        try:
            for line in self.process.stdout:
                buffer += line
                if len(buffer) > 2*1024*1024:
                    raise ValueError("watch bound")
                try:
                    event, end = json.JSONDecoder().raw_decode(buffer.lstrip())
                except json.JSONDecodeError:
                    continue
                buffer = buffer.lstrip()[end:].lstrip()
                if event.get("type") in ("ERROR", "DELETED"):
                    raise ValueError("watch error")
                d = event.get("object", event)
                if d["metadata"]["namespace"] != "abastevo-temp" or d["metadata"]["name"] != "abastevo-temp-api":
                    raise ValueError("watch scope")
                s = d["status"]
                desired = d["spec"].get("replicas", 1)
                images = sorted((c["name"], c["image"]) for c in d["spec"]["template"]["spec"]["containers"])
                row = {"kind":"availability", "at":stamp(), "generation":d["metadata"]["generation"],
                       "observed":s.get("observedGeneration", 0), "desired":desired,
                       "available":s.get("availableReplicas", 0), "updated":s.get("updatedReplicas", 0),
                       "unavailable":max(s.get("unavailableReplicas", 0), desired-s.get("availableReplicas", 0)),
                       "image_identity":hashlib.sha256(json.dumps(images).encode()).hexdigest()}
                self.rows.append(row)
                self.emit(row)
                self.initial.set()
        except Exception:
            self.failed = True
        finally:
            self.process.stdout.close()

    def stop(self):
        if self.process.poll() is not None:
            self.failed = True
        self.process.terminate()
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait(timeout=5)
        self.thread.join(timeout=5)
        if self.thread.is_alive():
            self.failed = True

def descriptor(path, base, duration, start):
    p = Path(path)
    if stat.S_IMODE(p.stat().st_mode) & 0o077:
        raise ValueError("upload descriptor must be mode 600")
    data = json.loads(p.read_text())
    u = origin(data["url"])
    b = origin(base)
    if (u.scheme, u.netloc) != (b.scheme, b.netloc) or not u.path.startswith("/abastevo-dev-photos/q/"):
        raise ValueError("upload must use the same origin and synthetic quarantine namespace")
    data.setdefault("bytes", 262144)
    data.setdefault("bytes_per_second", 65536)
    if not (1 <= data["bytes"] <= 3*1024*1024 and data["bytes_per_second"] > 0
            and 0 <= start and start + data["bytes"]/data["bytes_per_second"] <= duration):
        raise ValueError("slow upload must fit inside the campaign")
    if any(k.lower() in ("host", "authorization", "content-length", "transfer-encoding") for k in data.get("headers", {})):
        raise ValueError("upload descriptor may contain only signed content headers")
    return data

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="https://abastevo.com.br")
    parser.add_argument("--duration", type=float, default=600)
    parser.add_argument("--rate", type=float, default=20)
    parser.add_argument("--workers", type=int, default=128)
    parser.add_argument("--timeout", type=float, default=5)
    parser.add_argument("--output", required=True)
    parser.add_argument("--kubectl", default="kubectl")
    parser.add_argument("--slow-upload-file")
    parser.add_argument("--slow-start", type=float, default=30)
    parser.add_argument("--http-only", action="store_true", help="incomplete diagnostic; cannot pass acceptance")
    args = parser.parse_args()
    base = origin(args.base_url)
    if base.path not in ("", "/") or base.query or not (0 < args.duration <= 3600 and 0 < args.rate <= 100
            and 1 <= args.workers <= 512 and 0 < args.timeout <= 30):
        parser.error("invalid campaign bounds")
    if not args.http_only and not args.slow_upload_file:
        parser.error("full acceptance requires a private slow-upload descriptor")
    upload = descriptor(args.slow_upload_file, args.base_url, args.duration, args.slow_start) if args.slow_upload_file else None
    lock = threading.Lock()
    rows = []
    watch = None
    with open(args.output, "x", encoding="utf-8") as output:
        os.chmod(args.output, 0o600)
        def emit(row):
            with lock:
                output.write(json.dumps(row)+"\n")
                output.flush()
        try:
            if not args.http_only:
                watch = Watch(args.kubectl, emit)
                if not watch.initial.wait(timeout=10):
                    raise ValueError("no initial Kubernetes observation")
            start = time.monotonic()
            sent_upload = False
            pending = set()
            with ThreadPoolExecutor(max_workers=args.workers) as pool:
                for index in range(math.ceil(args.duration*args.rate)):
                    delay = start + index/args.rate - time.monotonic()
                    if delay > 0:
                        time.sleep(delay)
                    elif -delay > max(.25, 2/args.rate):
                        row = {"kind":"request", "route":"scheduler", "ok":False,"error":"ScheduleLag", "latency_ms":0}
                        rows.append(row); emit(row); break
                    done = {f for f in pending if f.done()}
                    for future in done:
                        row = future.result(); rows.append(row); emit(row)
                    pending -= done
                    if any(not r["ok"] for r in rows) or (watch and (watch.failed or any(r["unavailable"] for r in watch.rows))):
                        break
                    if upload and not sent_upload and time.monotonic()-start >= args.slow_start:
                        pending.add(pool.submit(request, "slow-upload", upload["url"], args.timeout, upload))
                        sent_upload = True
                    if len(pending) >= args.workers:
                        row = {"kind":"request", "route":"scheduler", "ok":False,"error":"Saturation", "latency_ms":0}
                        rows.append(row); emit(row); break
                    label, path = ROUTES[index % len(ROUTES)]
                    pending.add(pool.submit(request, label, args.base_url.rstrip("/")+path, args.timeout))
                for future in wait(pending).done:
                    row = future.result(); rows.append(row); emit(row)
        except Exception as error:
            row = {"kind":"request", "route":"observer", "ok":False,"error":type(error).__name__,"latency_ms":0}
            rows.append(row); emit(row)
        finally:
            if watch:
                watch.stop()
        samples = watch.rows if watch else []
        completed = bool(samples and samples[-1]["observed"] >= samples[-1]["generation"]
                         and samples[-1]["updated"] >= samples[-1]["desired"])
        rollout = bool(samples and len({s["image_identity"] for s in samples}) > 1 and completed)
        failures = sum(not row["ok"] for row in rows)
        observer_ok = bool(watch and not watch.failed and samples and all(s["unavailable"] == 0 and s["desired"] == 1 for s in samples))
        slow_ok = any(r["route"] == "slow-upload" and r["ok"] for r in rows)
        latency = sorted(r["latency_ms"] for r in rows)
        passed = bool(rows and failures == 0 and observer_ok and slow_ok and rollout)
        summary = {"kind":"summary", "at":stamp(), "requests":len(rows), "failed_requests":failures,
                   "configured_rate":args.rate, "configured_duration_s":args.duration,
                   "p99_ms":latency[math.ceil(len(latency)*.99)-1] if latency else None,
                   "availability_samples":len(samples), "availability_ok":observer_ok,
                   "image_rollout_observed":rollout, "slow_upload_ok":slow_ok,
                   "acceptance_passed":passed, "http_only":args.http_only}
        emit(summary)
    print(json.dumps(summary))
    return 0 if passed else (1 if failures or (watch and not observer_ok) else 2)

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, OSError, KeyError, TypeError):
        raise SystemExit("probe configuration failed; private details withheld")
