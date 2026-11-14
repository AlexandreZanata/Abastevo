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
    config = Path(os.environ.get("ANPFUEL_TEST_CADDYFILE", ROOT / "infra/caddy/staging-web.Caddyfile"))
    mirror = yaml.safe_load((ROOT / "infra/k8s/staging/web-caddy-configmap.yaml").read_text())
    assert mirror["data"]["Caddyfile"] == config.read_text(), "Kubernetes Caddy mirror drift"
    # The reviewed digest already pinned by the staging/production edge.
    image = os.environ.get("ANPFUEL_TEST_CADDY_IMAGE",
                           "caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d")
    name = "enterprise-plat008-caddy-" + str(os.getpid())
    with tempfile.TemporaryDirectory(prefix="empresas-caddy-") as directory, \
            tempfile.TemporaryDirectory(prefix="portal-caddy-") as portal_directory:
        tree = Path(directory)
        portal = Path(portal_directory)
        shutil.copytree(ROOT / "business-web/dist", tree / "empresas")
        (tree / "index.html").write_text("Synthetic landing sentinel")
        (tree / "404.html").write_text("Synthetic public 404 sentinel")
        (tree / "sitemap.xml").write_text("Synthetic landing sitemap sentinel")
        (tree / "admin/login").mkdir(parents=True)
        (tree / "admin/login/index.html").write_text("Synthetic admin sentinel")
        (portal / "sitemap.xml").write_text("Synthetic portal sitemap sentinel")
        subprocess.run(["docker", "run", "--rm", "--network", "none",
                        "-v", str(config) + ":/etc/caddy/Caddyfile:ro", image,
                        "caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            subprocess.run(["docker", "run", "--rm", "-d", "--name", name, "-p", "127.0.0.1::8080",
                            "-v", str(config) + ":/etc/caddy/Caddyfile:ro",
                            "-v", str(tree) + ":/srv/landing/current:ro",
                            "-v", str(portal) + ":/srv/portal/current:ro", image,
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
                    assert not headers.get("Last-Modified"), "No-store company document exposes Last-Modified"
                    assert headers["Referrer-Policy"] == "no-referrer"
                    assert "noindex" in headers["X-Robots-Tag"]
                    assert "base-uri 'none'" in headers["Content-Security-Policy"]
            for path in ["/empresas/unknown/", "/empresas/assets/missing.js", "/empresas/organizacoes/foreign/", "/empresas/_headers"]:
                code, headers, raw = request(path)
                assert code == 404 and b'gina n' in raw and headers["Cache-Control"] == "no-store" and not headers.get("ETag"), "Unknown company path became a cacheable success"
                assert not headers.get("Last-Modified"), "No-store company 404 exposes Last-Modified"
            assets = tree / "empresas/assets"
            for asset in [next(assets.glob(pattern)) for pattern in ["main.*.js", "business.*.css", "favicon.*.svg", "fonts/*.woff2"]]:
                code, headers, _ = request("/empresas/assets/" + asset.relative_to(assets).as_posix())
                assert code == 200 and headers["Cache-Control"] == "public, max-age=31536000, immutable", "Fingerprinted asset lost immutable caching"
            # Unhashed licence texts can change in place, so they revalidate.
            licences = sorted((assets / "fonts").glob("*.txt"))
            assert licences, "Font licence notices missing from the company build"
            for licence in licences:
                code, headers, raw = request("/empresas/assets/fonts/" + licence.name)
                assert code == 200 and raw == licence.read_bytes(), "Font licence notice not served"
                assert headers["Cache-Control"] == "public, max-age=3600, must-revalidate", "Unhashed licence cached as immutable"
            assert request("/")[2] == b"Synthetic landing sentinel"
            assert request("/admin/login/")[2] == b"Synthetic admin sentinel"
            for path in ["/", "/admin/login/"]:
                _, headers, _ = request(path)
                assert headers["Referrer-Policy"] == "strict-origin-when-cross-origin"
                assert "base-uri 'self'" in headers["Content-Security-Policy"]
                assert "img-src 'self' data:" in headers["Content-Security-Policy"]
            assert request("/v1/org-access/session/read")[0] == 404, "Static tier started proxying the API"
            # Official origin indexing: public pages are indexable, private
            # surfaces stay noindex, robots lists both sitemaps.
            _, headers, _ = request("/")
            assert not headers.get("X-Robots-Tag"), "Public landing still sends X-Robots-Tag"
            _, headers, _ = request("/admin/login/")
            assert headers.get("X-Robots-Tag") == "noindex, nofollow", "Admin panel lost noindex"
            code, _, raw = request("/robots.txt")
            robots = raw.decode()
            assert code == 200 and "User-agent: *\nAllow: /\n" in robots, "Robots does not allow the public site"
            for rule in ["Disallow: /admin/", "Disallow: /empresas/", "Disallow: /dados/busca/",
                         "User-agent: GPTBot\nDisallow: /\n", "User-agent: ClaudeBot\nDisallow: /\n",
                         "Sitemap: https://abastevo.com.br/sitemap.xml\n",
                         "Sitemap: https://abastevo.com.br/sitemap-portal.xml\n"]:
                assert rule in robots, "Robots rule missing: " + rule
            assert "Disallow: /\n" not in robots.split("User-agent: GPTBot")[0], "Robots still refuses every crawler"
            assert request("/sitemap.xml")[:3:2] == (200, b"Synthetic landing sitemap sentinel"), "Landing sitemap not served"
            assert request("/sitemap-portal.xml")[:3:2] == (200, b"Synthetic portal sitemap sentinel"), "Portal sitemap not served"
            print("PASS: Caddy company entry, canonical deep links, private cache/conditional rules without validators, fingerprinted-only immutable assets, revalidated licences, real 404s, public/admin separation, official-origin indexing with private noindex")
        finally:
            subprocess.run(["docker", "rm", "-f", name], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    run()
