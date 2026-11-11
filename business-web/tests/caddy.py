#!/usr/bin/env python3
"""Exercise the actual static web Caddyfile in an isolated loopback container."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import yaml

ROOT = Path(__file__).resolve().parents[2]


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args):
        return None


def run():
    config = ROOT / "infra/caddy/staging-web.Caddyfile"
    mirror = yaml.safe_load((ROOT / "infra/k8s/staging/web-caddy-configmap.yaml").read_text())
    assert mirror["data"]["Caddyfile"] == config.read_text(), "Kubernetes Caddy mirror drift"
    # Reuse the repository's local Caddy version, not an unpinned latest pull.
    image = os.environ.get("ANPFUEL_TEST_CADDY_IMAGE", "caddy:2.10.2")
    name = "enterprise-plat008-caddy-" + str(os.getpid())
    with tempfile.TemporaryDirectory(prefix="empresas-caddy-") as directory:
        tree = Path(directory)
        shutil.copytree(ROOT / "business-web/dist", tree / "empresas")
        (tree / "index.html").write_text("Synthetic landing sentinel")
        (tree / "404.html").write_text("Synthetic public 404 sentinel")
        (tree / "admin/login").mkdir(parents=True)
        (tree / "admin/login/index.html").write_text("Synthetic admin sentinel")
        subprocess.run(["docker", "run", "--rm", "--network", "none",
                        "-v", str(config) + ":/etc/caddy/Caddyfile:ro", image,
                        "caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            subprocess.run(["docker", "run", "--rm", "-d", "--name", name, "-p", "127.0.0.1::8080",
                            "-v", str(config) + ":/etc/caddy/Caddyfile:ro",
                            "-v", str(tree) + ":/srv/landing/current:ro", image,
                            "caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"],
                           check=True, stdout=subprocess.DEVNULL)
            port = subprocess.check_output(["docker", "port", name, "8080/tcp"], text=True).strip().split(":")[-1]
            origin = "http://127.0.0.1:" + port
            client = urllib.request.build_opener(NoRedirect())

            def request(path, headers=None):
                try:
                    response = client.open(urllib.request.Request(origin + path, headers=headers or {}), timeout=5)
                except urllib.error.HTTPError as error:
                    response = error
                with response:
                    return response.status, response.headers, response.read()

            deadline = time.monotonic() + 15
            while True:
                try:
                    if request("/")[0] == 200:
                        break
                except OSError:
                    pass
                if time.monotonic() > deadline:
                    raise AssertionError("Disposable Caddy did not become ready")
                time.sleep(.1)
            code, headers, _ = request("/empresas")
            assert code == 308 and headers["Location"] == "/empresas/"
            for path in ["/empresas/", "/empresas/entrar/",
                         "/empresas/organizacoes/7b0e2a54-0000-4000-8000-000000000002/"]:
                for condition in [{}, {"If-None-Match": "*"}, {"If-Modified-Since": "Tue, 01 Jan 2030 00:00:00 GMT"}]:
                    code, headers, raw = request(path, condition)
                    assert code == 200 and b'login-form' in raw, "Known company entry failed"
                    assert headers["Cache-Control"] == "no-store" and not headers.get("ETag"), "Private document cache policy failed"
                    assert headers["Referrer-Policy"] == "no-referrer"
                    assert "noindex" in headers["X-Robots-Tag"]
                    assert "base-uri 'none'" in headers["Content-Security-Policy"]
            for path in ["/empresas/unknown/", "/empresas/assets/missing.js", "/empresas/organizacoes/foreign/"]:
                code, headers, raw = request(path)
                assert code == 404 and b'gina n' in raw and headers["Cache-Control"] == "no-store" and not headers.get("ETag"), "Unknown company path became a cacheable success"
            js = next((tree / "empresas/assets").glob("main.*.js")).name
            code, headers, _ = request("/empresas/assets/" + js)
            assert code == 200 and headers["Cache-Control"] == "public, max-age=31536000, immutable"
            assert request("/")[2] == b"Synthetic landing sentinel"
            assert request("/admin/login/")[2] == b"Synthetic admin sentinel"
            for path in ["/", "/admin/login/"]:
                _, headers, _ = request(path)
                assert headers["Referrer-Policy"] == "strict-origin-when-cross-origin"
                assert "base-uri 'self'" in headers["Content-Security-Policy"]
                assert "img-src 'self' data:" in headers["Content-Security-Policy"]
            assert request("/v1/org-access/session/read")[0] == 404, "Static tier started proxying the API"
            print("PASS: Caddy company entry, canonical deep links, private cache/conditional rules, hashed assets, real 404s, public/admin separation")
        finally:
            subprocess.run(["docker", "rm", "-f", name], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    run()
