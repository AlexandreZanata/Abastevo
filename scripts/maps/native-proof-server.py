#!/usr/bin/env python3
"""Loopback-only signed download proof using the pinned MAP-14 input receipt.

Requires the operator's existing cryptography package (Apache-2.0 OR BSD-3-Clause).
No private key is persisted; this is a test harness, never a distribution service.
"""
import argparse
import base64
import copy
import hashlib
import http.server
import json
import threading
import time
from pathlib import Path

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

FILES = {
    "brazil-parana-east": ("brazil-parana-east.mwm", "Brazil_Parana_East.mwm"),
    "world": ("world.mwm", "World.mwm"),
    "world-coasts": ("world-coasts.mwm", "WorldCoasts.mwm"),
}


def b64(value):
    return base64.urlsafe_b64encode(value).decode().rstrip("=")


def proof_handler(inputs, public_key_output):
    receipt_path = Path(__file__).resolve().parents[2] / "docs/backend/evidence/maps/map14-final/runtime-map-inputs.json"
    receipt = json.loads(receipt_path.read_text())
    pinned = {item["name"]: item for item in receipt["maps"]}
    manifest = json.loads((inputs / "manifest.json").read_text())
    if str(receipt["native_data_directory_version"]) != manifest["engine"]["format_version"]:
        raise ValueError("native data version differs from the admitted receipt")
    if {item["id"] for item in manifest["packages"]} != set(FILES):
        raise ValueError("proof requires exactly the three admitted packages")
    for package in manifest["packages"]:
        local_name, native_name = FILES[package["id"]]
        data = (inputs / local_name).read_bytes()
        expected = pinned[native_name]
        if len(data) != expected["bytes"] or hashlib.sha256(data).hexdigest() != expected["sha256"]:
            raise ValueError("proof input differs from the admitted receipt: " + native_name)
        package.update(path=native_name, bytes=expected["bytes"], sha256=expected["sha256"])

    private_key = Ed25519PrivateKey.generate()
    public_key_output.write_text(b64(private_key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)))
    run = str(time.time_ns())
    state = {"generation": "native-proof-" + run + "-0", "requests": [], "sequence": 0}
    lock = threading.Lock()

    def envelope(generation):
        current = copy.deepcopy(manifest)
        current["generation"] = generation
        raw = json.dumps(current, separators=(",", ":"), ensure_ascii=False).encode()
        return json.dumps({
            "format": "abastevo.map-distribution-v1", "version": 1,
            "key_id": "local-native-proof", "algorithm": "Ed25519",
            "manifest": b64(raw), "manifest_sha256": hashlib.sha256(raw).hexdigest(),
            "signature": b64(private_key.sign(raw)),
        }).encode()

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def end_headers(self):
            # Close every response: an HTTP/1.0 reply without this let OkHttp pool a
            # socket the server then dropped, failing the next request intermittently.
            self.send_header("Connection", "close")
            super().end_headers()

        def do_GET(self):
            with lock:
                state["requests"].append(self.path)
                if self.path == "/proof/advance":
                    state["sequence"] += 1
                    state["generation"] = "native-proof-" + run + "-" + str(state["sequence"])
                generation = state["generation"]
                status = json.dumps(state).encode()
            if self.path == "/proof/advance":
                body = b"ok"
            elif self.path == "/proof/requests":
                body = status
            elif self.path in ("/v1/map/regions/brazil-parana-east/manifest",
                               "/dados/mapa-dist/regions/brazil-parana-east/manifest"):
                body = envelope(generation)
            elif self.path.startswith("/v1/map/packages/") or self.path.startswith("/dados/mapa-dist/"):
                package_id = self.path.rsplit("/", 1)[-1]
                if package_id not in FILES:
                    self.send_error(404)
                    return
                path = inputs / FILES[package_id][0]
                self.send_response(200)
                self.send_header("Content-Length", str(path.stat().st_size))
                self.end_headers()
                try:
                    with path.open("rb") as stream:
                        while chunk := stream.read(262144):
                            self.wfile.write(chunk)
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

    return Handler


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inputs", type=Path, required=True)
    parser.add_argument("--public-key-output", type=Path, required=True)
    parser.add_argument("--port", type=int, default=18765)
    args = parser.parse_args()
    handler = proof_handler(args.inputs, args.public_key_output)
    with http.server.ThreadingHTTPServer(("127.0.0.1", args.port), handler) as server:
        server.serve_forever()


if __name__ == "__main__":
    main()
