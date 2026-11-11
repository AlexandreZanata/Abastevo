#!/usr/bin/env python3
"""Local-only browser acceptance against the actual API and disposable PostGIS.

Requires the built dist, Python Playwright/Chromium, Go and openssl. Never
targets a persistent API. ANPFUEL_TEST_DATABASE_URL must name disposable
loopback PostgreSQL; all API configuration is cleared before composition.
"""
import contextlib
import http.client
import mimetypes
import os
from pathlib import Path
import re
import secrets
import socket
import ssl
import subprocess
import tempfile
import threading
import time
from urllib.parse import urlparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[2]
DIST = ROOT / "business-web/dist"
DSN = os.environ.get("ANPFUEL_TEST_DATABASE_URL", "")
parsed = urlparse(DSN)
if parsed.scheme not in ("postgres", "postgresql") or parsed.hostname not in ("127.0.0.1", "localhost"):
    raise SystemExit("An explicit disposable loopback ANPFUEL_TEST_DATABASE_URL is required")
if not (DIST / "index.html").is_file():
    raise SystemExit("Build business-web first")


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def run():
    api_port = free_port()
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass  # Credentials, bodies and query strings never enter test logs.

        def serve(self):
            if self.path.startswith("/v1/"):
                connection = http.client.HTTPConnection("127.0.0.1", api_port, timeout=15)
                try:
                    body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
                    headers = {k: v for k, v in self.headers.items() if k.lower() not in ("connection", "content-length")}
                    connection.request(self.command, self.path, body=body, headers=headers)
                    result = connection.getresponse()
                    raw = result.read()
                    self.send_response(result.status)
                    for k, v in result.getheaders():
                        if k.lower() not in ("connection", "content-length", "transfer-encoding"):
                            self.send_header(k, v)
                finally:
                    connection.close()
            else:
                local = self.path.split("?", 1)[0].removeprefix("/empresas/")
                if self.path.startswith("/empresas/") and ".." not in local and "%" not in local:
                    target = DIST / (local + "index.html" if local.endswith("/") or local == "" else local)
                else:
                    target = DIST / "__absent__"
                raw = target.read_bytes() if target.is_file() else (DIST / "404.html").read_bytes()
                self.send_response(200 if target.is_file() else 404)
                self.send_header("Content-Type", mimetypes.guess_type(str(target))[0] or "text/html")
                self.send_header("Cache-Control", "no-store")
                self.send_header("X-Robots-Tag", "noindex, nofollow")
                self.send_header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        do_GET = serve
        do_POST = serve

    with tempfile.TemporaryDirectory(prefix="empresas-browser-") as directory:
        temp = Path(directory)
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                        "-subj", "/CN=localhost", "-addext", "subjectAltName=DNS:localhost",
                        "-keyout", str(temp / "key.pem"), "-out", str(temp / "cert.pem")],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        web = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(temp / "cert.pem", temp / "key.pem")
        web.socket = tls.wrap_socket(web.socket, server_side=True)
        origin = "https://localhost:" + str(web.server_port)
        thread = threading.Thread(target=web.serve_forever, daemon=True)
        thread.start()
        env = {k: v for k, v in os.environ.items() if not k.startswith("ANPFUEL_")}
        env.update(ANPFUEL_ENV="development", ANPFUEL_DATABASE_URL=DSN,
                   ANPFUEL_ANP_DISCOVERY_ENABLED="false", ANPFUEL_ENTERPRISE_ENABLED="true",
                   ANPFUEL_HTTP_ADDR="127.0.0.1:" + str(api_port),
                   ANPFUEL_CANONICAL_HOST="localhost:" + str(web.server_port))
        subprocess.run(["go", "run", "./cmd/migrate"], cwd=ROOT / "backend", env=env,
                       check=True, stdout=subprocess.DEVNULL)
        subprocess.run(["go", "build", "-o", str(temp / "api"), "./cmd/api"], cwd=ROOT / "backend", env=env, check=True)
        with (temp / "api.log").open("w") as log:
            api = subprocess.Popen([str(temp / "api")], env=env, stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 30
                while True:
                    try:
                        with contextlib.closing(http.client.HTTPConnection("127.0.0.1", api_port, timeout=1)) as client:
                            client.request("GET", "/health/ready")
                            if client.getresponse().status == 200:
                                break
                    except OSError:
                        pass
                    if api.poll() is not None or time.monotonic() > deadline:
                        raise AssertionError("Local disposable API did not become ready")
                    time.sleep(.1)
                browser_checks(origin)
            finally:
                api.terminate()
                if api.wait(timeout=15) != 0:
                    raise AssertionError("Local API did not stop cleanly")
                web.shutdown()
                web.server_close()


def browser_checks(origin):
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(ignore_https_errors=True, viewport={"width": 1440, "height": 1000})
        page = context.new_page()
        errors = []
        page.on("pageerror", lambda error: errors.append(str(error)))
        created = context.request.post(origin + "/v1/accounts/keys", data={"username": "emp" + secrets.token_hex(6)})
        assert created.status == 201, "Synthetic account creation failed"
        account_key = created.json()["account_key"]
        # Without the module, the form must not submit the key natively.
        blocked = context.new_page()
        blocked.route("**/assets/main.*.js", lambda route: route.abort())
        blocked.goto(origin + "/empresas/")
        blocked.locator("#account-key").fill(account_key)
        blocked.locator("#account-key").press("Enter")
        blocked.wait_for_timeout(300)
        assert account_key not in blocked.url and "account-key" not in blocked.url, "Key reached the URL"
        assert blocked.locator("#login-button").is_disabled(), "Native sign-in enabled without the module"
        blocked.close()
        page.goto(origin + "/empresas/")
        expect(page.locator("#login-view")).to_be_visible()
        page.locator("#account-key").fill("abv_invalid_" + secrets.token_hex(8))
        page.locator("#account-key").press("Enter")
        expect(page.locator("#login-error")).to_have_text("Chave inválida. Confira e tente novamente.")
        assert page.evaluate("document.activeElement.id") == "account-key", "Refused sign-in lost focus"
        assert page.locator("#live-status").inner_text() == "", "Refusal announced twice"
        page.locator("#account-key").fill(account_key)
        with page.expect_response(lambda r: r.url.endswith("/org-access/session/login")) as issued:
            page.locator("#account-key").press("Enter")
        assert issued.value.status == 200, "Actual company issuance failed"
        csrf = issued.value.json()["csrf_token"]
        expect(page.locator("#page-title")).to_have_text("Suas empresas estão indisponíveis")
        expect(page.locator("#account-key")).to_have_value("")
        cookies = context.cookies()
        company = next(c for c in cookies if c["name"] == "__Host-empresas-session")
        assert company["secure"] and company["httpOnly"] and company["sameSite"] == "Strict" and company["path"] == "/"
        assert page.evaluate("localStorage.length + sessionStorage.length") == 0
        assert account_key not in page.locator("body").inner_text()
        page.locator("#logout-button").click()
        expect(page.locator("#login-view")).to_be_visible()
        refused = context.request.post(origin + "/v1/org-access/session/read", data={},
                    headers={"Origin": origin, "X-CSRF": csrf, "Cookie": company["name"] + "=" + company["value"]})
        assert refused.status == 401, "Logged-out company cookie remained valid"
        assert refused.headers.get("cache-control") == "no-store"
        page.locator("#account-key").fill(account_key)
        page.locator("#login-button").click()
        expect(page.locator("#page-title")).to_have_text("Suas empresas estão indisponíveis")
        page.reload()
        expect(page.locator("#login-view")).to_be_visible()
        assert page.locator("#workspace").is_hidden(), "Reload restored private UI without in-memory CSRF"
        page.screenshot(path="/tmp/enterprise-plat008-login-desktop.png", full_page=True)
        page.set_viewport_size({"width": 390, "height": 844})
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Mobile horizontal overflow"
        page.screenshot(path="/tmp/enterprise-plat008-login-mobile.png", full_page=True)
        print("PASS: no native key submission, refused-login focus, actual TLS/PostGIS account login, company cookie, logout/reuse, reload guard and responsive login")

        # Only this test server/browser receives a synthetic membership port.
        # The shipped main.ts always composes unavailableMemberships.
        shell = re.sub(r'<script type="module"[^>]+></script>', "", (DIST / "index.html").read_text())
        page.route("**/empresas/", lambda request: request.fulfill(status=200, body=shell, content_type="text/html"))
        page.goto(origin + "/empresas/")
        app = next((DIST / "assets").glob("app.*.js")).name
        page.evaluate("""async ({app}) => {
          const {mountCompany} = await import('/empresas/assets/'+app);
          const accountId='7b0e2a54-0000-4000-8000-000000000001';
          const organizations=[
            {id:'7b0e2a54-0000-4000-8000-000000000002',membershipId:'7b0e2a54-0000-4000-8000-000000000003',name:'Empresa A sintética'},
            {id:'7b0e2a54-0000-4000-8000-000000000004',membershipId:'7b0e2a54-0000-4000-8000-000000000005',name:'Empresa B sintética'}
          ];
          const session={authenticated:true,deadline:Date.now()+3600000,async read(){return {accountId};},forget(){this.authenticated=false;},async signIn(){this.authenticated=true;return {accountId};},async signOut(){this.forget();}};
          window.fixtureMode='ready';
          const source={async list(){if(window.fixtureMode==='empty')return {accountId,organizations:[]};return {accountId,organizations};},
            async authorize(account,id){if(window.fixtureMode==='foreign')return {accountId:'7b0e2a54-0000-4000-8000-000000000009',organization:organizations[0]};return {accountId:account,organization:organizations.find(o=>o.id===id)};}};
          window.fixtureController=mountCompany(document,window,session,source);
        }""", {"app": app})
        expect(page.locator("#page-title")).to_have_text("Escolha sua empresa")
        select = page.get_by_label("Empresa atual", exact=True)
        # Arrow keys on the closed select only choose; the button opens (WCAG 3.2.2).
        select.focus()
        select.press("ArrowDown")
        select.press("ArrowDown")
        page.wait_for_timeout(200)
        expect(page.locator("#page-title")).to_have_text("Escolha sua empresa")
        assert page.evaluate("document.activeElement.id") == "organization-select", "Arrow key changed context"
        select.press("ArrowUp")
        page.keyboard.press("Tab")
        assert page.evaluate("document.activeElement.id") == "organization-open"
        page.keyboard.press("Enter")
        expect(page.locator("#selected-company")).to_have_text("Empresa A sintética")
        expect(page.locator("#selected-company")).to_be_visible()
        assert page.evaluate("document.activeElement.id") == "page-title"
        select.focus()
        select.press("ArrowDown")
        page.keyboard.press("Tab")
        page.keyboard.press("Enter")
        expect(page.locator("#selected-company")).to_have_text("Empresa B sintética")
        assert "Empresa A sintética" not in page.locator("#workspace").inner_text()
        page.set_viewport_size({"width": 1440, "height": 1000})
        page.screenshot(path="/tmp/enterprise-plat008-workspace-desktop.png", full_page=True)
        page.set_viewport_size({"width": 390, "height": 844})
        page.screenshot(path="/tmp/enterprise-plat008-workspace-mobile.png", full_page=True)
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Workspace horizontal overflow"
        page.evaluate("window.fixtureMode='foreign'")
        select.select_option("7b0e2a54-0000-4000-8000-000000000002")
        page.locator("#organization-open").click()
        expect(page.locator("#page-title")).to_have_text("Acesso não disponível")
        expect(page.locator("#selected-company")).to_be_hidden()
        assert "Empresa B sintética" not in page.locator("#workspace").inner_text()
        page.evaluate("window.fixtureMode='empty'; history.replaceState({},'', '/empresas/'); dispatchEvent(new PopStateEvent('popstate'))")
        expect(page.locator("#page-title")).to_have_text("Nenhuma empresa por aqui ainda")
        assert page.locator("h1:visible").count() == 1
        page.evaluate("dispatchEvent(new PageTransitionEvent('pageshow', {persisted:true}))")
        expect(page.locator("#login-view")).to_be_visible()
        expect(page.locator("#selected-company")).to_be_hidden()
        expect(page.locator("#account-key")).to_have_value("")
        assert not errors, "Browser JavaScript errors occurred"
        print("PASS: keyboard selection, A/B isolation, authority denial, empty state, focus, responsive shell and restored-page guard")
        context.close()
        browser.close()


if __name__ == "__main__":
    run()
