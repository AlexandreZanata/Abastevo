#!/usr/bin/env python3
"""MAP-24 local offline proof server: signed manifests + synthetic bytes.

Loopback-only. Signs with an ephemeral in-memory Ed25519 key (never persisted);
exposes the raw public key for the pinned-key verifier. Endpoints:
  /proof/pubkey            raw public key, base64url no padding
  /proof/requests          JSON request log + current generation
  /proof/advance           move to the next generation (same package bytes)
  /proof/corrupt?id=<pkg>  toggle corrupted transfer bytes for one package
  /proof/truncate?id=<pkg>&bytes=<n>  send only n bytes of the next transfers of
                           that package, then drop the connection (interrupted
                           update); /proof/truncate?clear=1 removes all truncation
  Package transfers honour a single-range "Range: bytes=<from>-" request (206).
  /v1/map/regions/<r>/manifest
  /v1/map/packages/<id>
"""
import argparse
import base64
import copy
import hashlib
import http.server
import json
import threading

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

PACKAGES = {
    "east": 262144,
    "world": 131072,
    "world-coasts": 65536,
}
ROLES = {"east": "REGION", "world": "OVERVIEW", "world-coasts": "OVERVIEW"}


def b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode().rstrip("=")


def pattern(seed: int, size: int) -> bytes:
    return bytes(((i * 31 + seed) % 251 for i in range(size)))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=18766)
    args = parser.parse_args()

    private_key = Ed25519PrivateKey.generate()
    public_raw = private_key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)
    good = {pid: pattern(i + 1, size) for i, (pid, size) in enumerate(PACKAGES.items())}
    corrupt = set()
    truncate = {}
    state = {"generation": "gen-24-0", "requests": [], "ranges": [], "sequence": 0}
    lock = threading.Lock()

    manifest_template = {
        "format": "abastevo.native-map-manifest-v1",
        "version": 1,
        "generation": "",
        "engine": {
            "name": "comaps",
            "upstream_sha": "4587b8ad4d12c9f59982709fdaf03cc925948827",
            "generator_sha": "4587b8ad4d12c9f59982709fdaf03cc925948827",
            "format_version": "261004",
        },
        "region": {
            "id": "test-rect",
            "name": "Rect",
            "bbox": {"min_lon_deg": -1.0, "min_lat_deg": -1.0, "max_lon_deg": 1.0, "max_lat_deg": 1.0},
            "overview": "world",
        },
        "packages": [],
        "dependencies": ["world", "world-coasts"],
        "sources": [{
            "id": "synthetic",
            "license": "CC-BY-4.0",
            "attribution": "MAP-24 offline proof",
            "sha256": "00" * 32,
        }],
        "rights": {
            "license": "CC-BY-4.0",
            "attribution": "MAP-24 offline proof",
            "commercial": True,
            "offline": True,
            "redistribution": True,
        },
        "generated_at": "2026-10-26T12:00:00Z",
    }
    for pid, size in PACKAGES.items():
        manifest_template["packages"].append({
            "id": pid,
            "role": ROLES[pid],
            "path": f"261004/{pid}.mwm",
            "bytes": size,
            "sha256": hashlib.sha256(good[pid]).hexdigest(),
        })

    def envelope(generation: str) -> bytes:
        current = copy.deepcopy(manifest_template)
        current["generation"] = generation
        raw = json.dumps(current, separators=(",", ":"), ensure_ascii=False).encode()
        return json.dumps({
            "format": "abastevo.map-distribution-v1",
            "version": 1,
            "key_id": "map24-proof",
            "algorithm": "Ed25519",
            "manifest": b64(raw),
            "manifest_sha256": hashlib.sha256(raw).hexdigest(),
            "signature": b64(private_key.sign(raw)),
        }).encode()

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def end_headers(self):
            # Every response ends the connection explicitly. Without this an
            # HTTP/1.0 reply let OkHttp pool a socket the server then closed, and
            # the next pooled request failed with "unexpected end of stream".
            self.send_header("Connection", "close")
            super().end_headers()

        def do_GET(self):
            with lock:
                state["requests"].append(self.path)
                if self.path == "/proof/advance":
                    state["sequence"] += 1
                    state["generation"] = f"gen-24-{state['sequence']}"
                generation = state["generation"]
            if self.path == "/proof/pubkey":
                body = b64(public_raw).encode()
            elif self.path == "/proof/requests":
                with lock:
                    body = json.dumps(state).encode()
            elif self.path == "/proof/advance":
                body = b"ok"
            elif self.path.startswith("/proof/corrupt"):
                query = self.path.split("?", 1)[1] if "?" in self.path else ""
                params = dict(p.split("=") for p in query.split("&") if "=" in p)
                with lock:
                    if "clear" in params:
                        corrupt.clear()
                    elif "id" in params:
                        corrupt.add(params["id"])
                body = b"ok"
            elif self.path.startswith("/proof/truncate"):
                query = self.path.split("?", 1)[1] if "?" in self.path else ""
                params = dict(p.split("=") for p in query.split("&") if "=" in p)
                with lock:
                    if "clear" in params:
                        truncate.clear()
                    elif "id" in params and "bytes" in params:
                        truncate[params["id"]] = int(params["bytes"])
                body = b"ok"
            elif self.path.endswith("/manifest"):
                body = envelope(generation)
            elif "/v1/map/packages/" in self.path or "/dados/mapa-dist/" in self.path:
                pid = self.path.rsplit("/", 1)[-1].split("?")[0]
                if pid not in good:
                    self.send_error(404)
                    return
                payload = bytearray(good[pid])
                with lock:
                    bad = pid in corrupt
                if bad:
                    payload[0] ^= 0xFF
                with lock:
                    cut = truncate.get(pid)
                start = 0
                range_header = self.headers.get("Range", "")
                if range_header.startswith("bytes=") and range_header.endswith("-"):
                    start = int(range_header[len("bytes="):-1])
                with lock:
                    state["ranges"].append({"id": pid, "from": start})
                body_bytes = bytes(payload)[start:]
                if start:
                    self.send_response(206)
                    self.send_header("Content-Range", f"bytes {start}-{len(payload) - 1}/{len(payload)}")
                else:
                    self.send_response(200)
                self.send_header("Content-Length", str(len(body_bytes)))
                self.end_headers()
                try:
                    self.wfile.write(body_bytes if cut is None else body_bytes[:cut])
                    self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    pass
                return
            else:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    with http.server.ThreadingHTTPServer(("127.0.0.1", args.port), Handler) as server:
        server.serve_forever()


if __name__ == "__main__":
    main()
