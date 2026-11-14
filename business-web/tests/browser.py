#!/usr/bin/env python3
"""Local-only browser acceptance against the actual API and disposable PostGIS.

Requires the built dist, Python Playwright/Chromium, Go and openssl. Never
targets a persistent API. ANPFUEL_TEST_DATABASE_URL must name disposable
loopback PostgreSQL; all API configuration is cleared before composition.
"""
import contextlib
from copy import deepcopy
import http.client
import json
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
import uuid
from urllib.parse import parse_qsl, urlparse, quote
import psycopg
from psycopg import sql
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from playwright.sync_api import Error as PlaywrightError, sync_playwright, expect

ROOT = Path(__file__).resolve().parents[2]
DIST = ROOT / "business-web/dist"
CONTROL_DSN = os.environ.get("ANPFUEL_TEST_DATABASE_URL", "")
control_location = urlparse(CONTROL_DSN)
if control_location.scheme not in ("postgres", "postgresql") or control_location.hostname not in ("127.0.0.1", "localhost"):
    raise SystemExit("An explicit disposable loopback ANPFUEL_TEST_DATABASE_URL is required")
control_options = parse_qsl(control_location.query, keep_blank_values=True)
if (len({key for key, _ in control_options}) != len(control_options) or
        any(key not in {"sslmode", "connect_timeout", "application_name"} for key, _ in control_options)):
    raise SystemExit("Disposable browser DSN cannot override its owned database or loopback destination")
if not (DIST / "index.html").is_file():
    raise SystemExit("Build business-web first")


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


@contextlib.contextmanager
def disposable_database():
    """Migrations and fixtures use one owned database, never the control DB."""
    name = "emp_web_browser_" + secrets.token_hex(12)
    with psycopg.connect(CONTROL_DSN, autocommit=True) as control:
        control.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name)))
        try:
            yield control_location._replace(path="/" + name).geturl()
        finally:
            # The literal random name is created above and never read from a URL
            # or external response. Only this harness's database is removed.
            control.execute(sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name)))


@contextlib.contextmanager
def ordinary_runtime(dsn):
    """Exact local C1 test grants; runtime cannot own/bypass private tables."""
    role = "emp_web_runtime_" + secrets.token_hex(8)
    password = secrets.token_hex(24)
    parsed = urlparse(dsn)
    connection = psycopg.connect(dsn, autocommit=True)
    try:
        connection.execute(sql.SQL("CREATE ROLE {} LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD {}").format(sql.Identifier(role), sql.Literal(password)))
        grants = [
            "GRANT USAGE ON SCHEMA public TO {}",
            "GRANT SELECT,INSERT,UPDATE ON accounts,account_session_families,identity_rate_windows,organizations,org_units,org_employee_identities,org_employee_profiles,org_memberships,org_role_assignments,org_capabilities,org_settings_heads,org_invitations,org_invitation_acceptances TO {}",
            "GRANT SELECT,INSERT ON account_key_credentials,org_onboarding_commands,org_audit_events,org_employee_commands,org_settings_commands,org_settings_versions,org_settings_designations,org_settings_notices,org_invitation_commands,org_invitation_notices,org_invitation_locators,org_invitation_person_commands,org_invitation_acceptance_notices,org_role_commands TO {}",
            "GRANT SELECT,UPDATE ON org_teams,org_team_links,org_ownership_transfers,org_device_enrollments TO {}",
            "GRANT SELECT ON schema_migrations,payment_billing_exits,account_contributor_bindings,org_establishments,org_legal_entities TO {}",
            "GRANT SELECT,INSERT,UPDATE,DELETE ON account_key_admission_events,company_web_sessions TO {}",
            "GRANT USAGE ON SEQUENCE account_key_admission_events_id_seq TO {}",
        ]
        for statement in grants:
            connection.execute(sql.SQL(statement).format(sql.Identifier(role)))
        runtime = parsed._replace(netloc=quote(role) + ":" + quote(password) + "@" + parsed.hostname + ":" + str(parsed.port or 5432)).geturl()
        with psycopg.connect(runtime) as ordinary:
            properties = ordinary.execute("SELECT current_user,rolsuper,rolbypassrls,rolcanlogin FROM pg_roles WHERE rolname=current_user").fetchone()
            assert properties == (role, False, False, True), "Browser API requires an ordinary non-bypass runtime login"
        yield runtime
    finally:
        connection.execute(sql.SQL("DROP OWNED BY {}").format(sql.Identifier(role)))
        connection.execute(sql.SQL("DROP ROLE IF EXISTS {}").format(sql.Identifier(role)))
        connection.close()


def enable_test_operator_capability(dsn, org_id):
    # Explicit operator fixture in this disposable DB, never a runtime provider.
    with psycopg.connect(dsn, autocommit=True) as connection:
        connection.execute("INSERT INTO org_capabilities(org_id,capability,enabled) VALUES (%s,'organizations',true) ON CONFLICT(org_id,capability) DO UPDATE SET enabled=true,revision=org_capabilities.revision+1", (org_id,))


def run(dsn):
    if not urlparse(dsn).path.removeprefix("/").startswith("emp_web_browser_"):
        raise AssertionError("Browser acceptance requires its newly created owned database")
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
                path = self.path.split("?", 1)[0]
                local = path.removeprefix("/empresas/")
                if self.path.startswith("/empresas/") and ".." not in local and "%" not in local:
                    if path == "/empresas/criar/" or re.fullmatch(r"/empresas/organizacoes/[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}/(?:configuracao/)?", path):
                        target = DIST / "index.html"
                    else:
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
            try:
                self.end_headers()
                self.wfile.write(raw)
            except (BrokenPipeError, ConnectionResetError, ssl.SSLEOFError):
                # Cancellation is intentional in lost-ack/late-response cases;
                # the browser assertions still require each delivered result.
                pass

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
        env.update(ANPFUEL_ENV="development", ANPFUEL_DATABASE_URL=dsn,
                   ANPFUEL_ANP_DISCOVERY_ENABLED="false", ANPFUEL_ENTERPRISE_ENABLED="true",
                   ANPFUEL_HTTP_ADDR="127.0.0.1:" + str(api_port),
                   ANPFUEL_CANONICAL_HOST="localhost:" + str(web.server_port))
        subprocess.run(["go", "run", "./cmd/migrate"], cwd=ROOT / "backend", env=env,
                       check=True, stdout=subprocess.DEVNULL)
        subprocess.run(["go", "build", "-o", str(temp / "api"), "./cmd/api"], cwd=ROOT / "backend", env=env, check=True)
        with ordinary_runtime(dsn) as runtime_dsn, (temp / "api.log").open("w") as log:
            env["ANPFUEL_DATABASE_URL"] = runtime_dsn
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
                        log.flush()
                        details = re.findall(r"permission denied for (?:table|sequence) [a-z0-9_]+", (temp / "api.log").read_text())
                        raise AssertionError("Local disposable API did not become ready" + (": " + ", ".join(sorted(set(details))) if details else ""))
                    time.sleep(.1)
                browser_checks(origin, dsn)
            finally:
                api.terminate()
                if api.wait(timeout=15) != 0:
                    raise AssertionError("Local API did not stop cleanly")
                web.shutdown()
                web.server_close()


def browser_checks(origin, dsn):
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(ignore_https_errors=True, viewport={"width": 1440, "height": 1000})
        page = context.new_page()
        errors = []
        page.on("pageerror", lambda error: errors.append(str(error)))
        created = context.request.post(origin + "/v1/accounts/keys", data={"username": "emp" + secrets.token_hex(6)})
        assert created.status == 201, "Synthetic account creation failed: " + str(created.status) + " " + str(created.json().get("error", {}).get("code", "unknown"))
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
        directory = context.request.post(origin + "/v1/org-access/contexts/list", data={"limit": 100}, headers={"Origin": origin, "X-CSRF": csrf})
        assert directory.status == 200, "Current context discovery failed: " + str(directory.status) + " " + str(directory.json().get("error", {}).get("code", "unknown"))
        expect(page.locator("#page-title")).to_have_text("Nenhuma empresa por aqui ainda")
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
        expect(page.locator("#page-title")).to_have_text("Nenhuma empresa por aqui ainda")
        page.reload()
        expect(page.locator("#login-view")).to_be_visible()
        assert page.locator("#workspace").is_hidden(), "Reload restored private UI without in-memory CSRF"
        page.screenshot(path="/tmp/enterprise-plat008-login-desktop.png", full_page=True)
        page.set_viewport_size({"width": 390, "height": 844})
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Mobile horizontal overflow"
        page.screenshot(path="/tmp/enterprise-plat008-login-mobile.png", full_page=True)
        print("PASS: no native key submission, refused-login focus, actual TLS/PostGIS account login, company cookie, logout/reuse, reload guard and responsive login")

        # Shell-only injected ports isolate keyboard and stale-response behavior.
        # The shipped main.ts composes the real account-bound context API.
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
            async authorize(account,id){window.authorizeCalls=(window.authorizeCalls||0)+1;if(window.fixtureMode==='foreign')return {accountId:'7b0e2a54-0000-4000-8000-000000000009',organization:organizations[0]};return {accountId:account,organization:organizations.find(o=>o.id===id)};}};
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
        # Returning to the tab rechecks authority without moving focus or
        # re-announcing an unchanged state.
        select.focus()
        page.evaluate("""() => {
          window.liveMutations = 0;
          new MutationObserver(() => window.liveMutations++).observe(document.getElementById('live-status'), {childList: true, characterData: true, subtree: true});
          window.recheckBase = window.authorizeCalls;
          document.dispatchEvent(new Event('visibilitychange'));
        }""")
        page.wait_for_function("window.authorizeCalls > window.recheckBase")
        page.wait_for_timeout(200)
        assert page.evaluate("document.activeElement.id") == "organization-select", "Tab return stole focus"
        assert page.evaluate("window.liveMutations") == 0, "Tab return re-announced an unchanged state"
        expect(page.locator("#selected-company")).to_have_text("Empresa B sintética")
        page.set_viewport_size({"width": 1440, "height": 1000})
        page.screenshot(path="/tmp/enterprise-plat008-workspace-desktop.png", full_page=True)
        page.set_viewport_size({"width": 390, "height": 844})
        page.screenshot(path="/tmp/enterprise-plat008-workspace-mobile.png", full_page=True)
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Workspace horizontal overflow"
        overview = page.locator("#overview-link")
        expect(overview).to_be_visible()
        overview.click()
        expect(page.locator("#page-title")).to_have_text("Escolha sua empresa")
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Mobile navigation overflow"
        page.evaluate("window.fixtureMode='foreign'")
        select.select_option("7b0e2a54-0000-4000-8000-000000000002")
        page.locator("#organization-open").click()
        expect(page.locator("#page-title")).to_have_text("Acesso não disponível")
        expect(page.locator("#selected-company")).to_be_hidden()
        assert "Empresa B sintética" not in page.locator("#workspace").inner_text()
        page.evaluate("window.fixtureMode='empty'; history.replaceState({},'', '/empresas/'); dispatchEvent(new PopStateEvent('popstate'))")
        expect(page.locator("#page-title")).to_have_text("Nenhuma empresa por aqui ainda")
        assert page.locator("h1:visible").count() == 1
        # A tab-return recheck that changes the state still announces and focuses it.
        page.evaluate("window.fixtureMode='ready'; document.dispatchEvent(new Event('visibilitychange'))")
        expect(page.locator("#page-title")).to_have_text("Escolha sua empresa")
        expect(page.locator("#live-status")).to_have_text("Escolha sua empresa")
        assert page.evaluate("document.activeElement.id") == "page-title", "Changed recheck lost focus"
        page.evaluate("dispatchEvent(new PageTransitionEvent('pageshow', {persisted:true}))")
        expect(page.locator("#login-view")).to_be_visible()
        expect(page.locator("#selected-company")).to_be_hidden()
        expect(page.locator("#account-key")).to_have_value("")
        assert not errors, "Browser JavaScript errors occurred"
        print("PASS: keyboard selection, A/B isolation, quiet tab-return recheck, mobile overview link, authority denial, empty state, focus, responsive shell and restored-page guard")
        context.close()
        employee_checks(browser, origin, dsn)
        setup_checks(browser, origin, dsn)
        browser.close()


def setup_checks(browser, origin, dsn):
    """Actual setup/settings HTTP and browser behavior; all records are synthetic."""
    context = browser.new_context(ignore_https_errors=True, viewport={"width": 1440, "height": 1000})
    page = context.new_page()
    errors = []
    page.on("pageerror", lambda _error: errors.append("setup browser error"))
    page.on("dialog", lambda dialog: dialog.accept())

    def create_account(prefix):
        response = context.request.post(origin + "/v1/accounts/keys", data={"username": prefix[:8] + secrets.token_hex(6)})
        assert response.status == 201, "Synthetic setup account creation failed"
        created = response.json()
        personal = context.request.post(origin + "/v1/accounts/keys/login", data={"account_key": created["account_key"]})
        assert personal.status == 200, "Synthetic setup account identity was not issued by its source"
        return {"account_key": created["account_key"], "account_id": personal.json()["account"]["account_id"]}

    owner = create_account("setupowner")
    owner_key = owner["account_key"]
    csrf = ""

    def sign_in(key):
        nonlocal csrf
        try:
            page.locator("#account-key").fill(key)
        except PlaywrightError:
            raise AssertionError("Synthetic setup sign-in field unavailable") from None
        with page.expect_response(lambda response: response.url.endswith("/org-access/session/login")) as logged_in:
            page.locator("#login-button").click()
        assert logged_in.value.status == 200, "Actual setup company login failed"
        csrf = logged_in.value.json()["csrf_token"]

    def post(path, body=None, status=200):
        response = context.request.post(origin + path, data=body or {}, headers={"Origin": origin, "X-CSRF": csrf})
        assert response.status == status, "Setup source request failed: " + path + " status " + str(response.status)
        assert response.headers.get("cache-control") == "no-store", "Setup source response is cacheable"
        return response.json()

    def read(org_id):
        return post("/v1/orgs/" + org_id + "/settings/read")

    def database_value(statement, args=()):
        with psycopg.connect(dsn) as connection:
            return connection.execute(statement, args).fetchone()[0]

    def change_fixture(statement, args=(), org_id=None):
        with psycopg.connect(dsn) as connection:
            if org_id:
                connection.execute("SELECT set_config('anpfuel.org_id',%s,true)", (org_id,))
            connection.execute(statement, args)

    def no_private_storage():
        assert page.evaluate("localStorage.length + sessionStorage.length") == 0, "Private browser state was persisted"
        assert owner_key not in page.locator("body").text_content(), "Account key entered private UI"

    def create_draft(name):
        page.locator("#setup-name").fill(name)
        page.locator("#setup-time-zone").fill("America/Cuiaba")
        page.locator("#setup-unit-name").fill("Unidade sintética de configuração")
        for kind in ["delivery", "carrier", "services", "farm"]:
            page.locator("#setup-kind-" + kind).uncheck()
        page.locator("#setup-kind-other").check()
        page.locator("#setup-terms").check()

    # The first acknowledgement is dropped only after the real server commits.
    # The retry must preserve the exact submitted bytes, not just the visible fields.
    creation_requests, creations = [], []
    def creation_reply(route):
        creation_requests.append(route.request.post_data)
        response = route.fetch()
        assert response.status == 201, "Source creation failed before the simulated lost acknowledgement"
        creations.append(response.json())
        if len(creation_requests) in (1, 3):
            route.abort("failed")
        else:
            route.fulfill(response=response)

    page.route("**/v1/org-access/organizations/create", creation_reply)
    page.goto(origin + "/empresas/criar/")
    expect(page.locator("#login-view")).to_be_visible()
    sign_in(owner_key)
    expect(page.locator("#setup-create-form")).to_be_visible()
    expect(page.locator("h1:visible")).to_have_count(1)
    assert page.evaluate("document.activeElement.id") == "setup-title", "Setup entry did not focus its visible primary heading"
    name_a = "Empresa sintética de configuração A " + secrets.token_hex(3)
    create_draft(name_a)
    page.locator("#setup-create-submit").click()
    expect(page.locator("#setup-retry")).to_be_visible()
    page.locator("#setup-retry").click()
    expect(page.locator("#setup-detail")).to_be_visible()
    assert len(creation_requests) == 2 and creation_requests[0] == creation_requests[1], "Creation retry changed its immutable intent"
    org_a = creations[0]["organization_id"]
    creator_member = creations[0]["membership_id"]
    assert creations[1]["organization_id"] == org_a and creations[1].get("replayed") is True
    assert database_value("SELECT count(*) FROM org_onboarding_commands WHERE org_id=%s AND kind='CREATED'", (org_a,)) == 1
    expect(page.locator("#setup-organization-name")).to_have_text(name_a)
    assert read(org_a)["organization"]["state"] == "PENDING_SETUP"
    assert read(org_a)["context"]["capability_enabled"] is False
    expect(page.locator("#setup-capability-note")).to_be_visible()
    expect(page.locator("#setup-activate")).to_be_disabled()
    native = post("/v1/org-access/contexts/list", {"limit": 100})
    assert all(item["id"] != org_a for item in native["organizations"]), "Setup changed operational context eligibility"
    no_private_storage()

    # A second committed creation loses its answer across reload. The fresh
    # directory resumes its persisted organization without a replacement create.
    page.locator("#create-company-link").click()
    expect(page.locator("#setup-create-form")).to_be_visible()
    name_b = "Empresa sintética de configuração B " + secrets.token_hex(3)
    create_draft(name_b)
    page.locator("#setup-create-submit").click()
    expect(page.locator("#setup-retry")).to_be_visible()
    assert len(creation_requests) == 3
    org_b = creations[-1]["organization_id"]
    page.reload()
    expect(page.locator("#login-view")).to_be_visible()
    assert name_a not in page.locator("body").text_content() and name_b not in page.locator("body").text_content()
    sign_in(owner_key)
    expect(page.locator("#setup-directory-list").get_by_role("button", name="Retomar " + name_b, exact=True)).to_be_visible()
    page.locator("#setup-directory-list").get_by_role("button", name="Retomar " + name_b, exact=True).click()
    expect(page.locator("#setup-organization-name")).to_have_text(name_b)
    assert len(creation_requests) == 3, "Reload recovery created a replacement organization"
    assert database_value("SELECT count(*) FROM org_onboarding_commands WHERE actor_id=%s AND kind='CREATED'", (read(org_b)["context"]["account_id"],)) == 2
    assert page.url.endswith("/empresas/organizacoes/" + org_b + "/configuracao/")
    page.reload()
    expect(page.locator("#login-view")).to_be_visible()
    sign_in(owner_key)
    expect(page.locator("#setup-detail")).to_be_visible()
    expect(page.locator("#setup-organization-name")).to_have_text(name_b)

    # The operator fixture, not the panel, enables only the owned test roots.
    enable_test_operator_capability(dsn, org_a)
    enable_test_operator_capability(dsn, org_b)
    change_fixture("UPDATE organizations SET display_name='',revision=revision+1 WHERE org_id=%s AND state='PENDING_SETUP'", (org_b,), org_b)
    page.locator("#setup-refresh").click()
    expect(page.locator("#setup-panel")).to_have_attribute("aria-busy", "false")
    assert "name" in read(org_b)["setup"]["missing"], "Persisted missing-name checklist was hidden"
    assert page.locator("#setup-checklist").inner_text().strip(), "Incomplete checklist is inaccessible"
    incomplete = post("/v1/orgs/" + org_b + "/organization/activate", {
        "schema_version": 1, "operation_id": str(uuid.uuid4()), "revision": read(org_b)["root_revision"]}, 409)
    assert incomplete["error"]["code"] == "ORG_SETUP_INCOMPLETE"
    assert read(org_b)["organization"]["state"] == "PENDING_SETUP", "Incomplete source activated early"

    page.evaluate("path => {history.pushState({},'',path); dispatchEvent(new PopStateEvent('popstate'));}", "/empresas/organizacoes/" + org_a + "/configuracao/")
    expect(page.locator("#setup-organization-name")).to_have_text(name_a)
    expect(page.locator("#setup-activate")).to_be_enabled()
    activation_requests = []
    def activation_reply(route):
        activation_requests.append(route.request.post_data)
        response = route.fetch()
        assert response.status == 200, "Source activation failed"
        if len(activation_requests) == 1:
            route.abort("failed")
        else:
            route.fulfill(response=response)
    page.route("**/v1/orgs/" + org_a + "/organization/activate", activation_reply)
    page.locator("#setup-activate").click()
    expect(page.locator("#setup-retry")).to_be_visible()
    page.locator("#setup-retry").click()
    expect(page.locator("#setup-panel")).to_have_attribute("aria-busy", "false")
    expect(page.locator("#setup-retry")).to_be_hidden()
    assert len(activation_requests) == 2 and activation_requests[0] == activation_requests[1], "Activation retry changed identity or root revision"
    assert read(org_a)["organization"]["state"] == "ACTIVE"
    assert database_value("SELECT count(*) FROM org_onboarding_commands WHERE org_id=%s AND kind='ACTIVATED'", (org_a,)) == 1

    def named_member(account_id, label, role):
        employee, member, assignment = str(uuid.uuid4()), str(uuid.uuid4()), str(uuid.uuid4())
        with psycopg.connect(dsn) as connection:
            connection.execute("SELECT set_config('anpfuel.org_id',%s,true)", (org_a,))
            connection.execute("INSERT INTO org_employee_identities(org_id,id,state) VALUES(%s,%s,'ACTIVE')", (org_a, employee))
            connection.execute("INSERT INTO org_employee_profiles(org_id,employee_id,display_name) VALUES(%s,%s,%s)", (org_a, employee, label))
            connection.execute("INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES(%s,%s,%s,%s,'EXISTING_ACCOUNT','INVITED')", (org_a, member, employee, account_id))
            connection.execute("INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES(%s,%s,%s,%s,%s,%s,clock_timestamp(),%s,'Synthetic browser member')", (org_a, assignment, member, role, "SELF" if role == "DRIVER" else "ORG", member if role == "DRIVER" else org_a, creator_member))
            connection.execute("UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=%s AND id=%s", (org_a, member))
        return member

    auditor, driver = create_account("setupreader"), create_account("setupdriver")
    auditor_id = auditor.get("account_id") or auditor.get("account", {}).get("account_id")
    driver_id = driver.get("account_id") or driver.get("account", {}).get("account_id")
    assert auditor_id and driver_id, "Synthetic account response lacks account binding"
    auditor_member = named_member(auditor_id, "Pessoa sintética de consulta", "AUDITOR")
    driver_member = named_member(driver_id, "Pessoa sintética substituta", "DRIVER")
    page.locator("#setup-refresh").click()
    expect(page.locator("#setup-panel")).to_have_attribute("aria-busy", "false")
    page.locator("#setup-members-load").click()
    expect(page.locator("#setup-billing-primary option[value='" + auditor_member + "']")).to_have_count(1)
    expected_values = deepcopy(read(org_a)["values"])
    expected_values.update(time_zone="America/Sao_Paulo", consumption_display="L_PER_100KM", approval_levels_default=3, closure_period_days=45)
    expected_values["review_required"] = {"trips": False, "fuelings": False, "expenses": False}
    expected_values["policies"].update(intake_window_days=7, clock_skew_limit_seconds="0", delegation_maximum_days=120, invitation_validity_days=14, activation_validity_hours=72, require_invitation_confirmation=True)
    for responsibility in ["review_fallback", "platform_notices", "data_protection", "billing"]:
        expected_values["responsibilities"][responsibility] = {"primary": auditor_member, "substitute": driver_member}
        page.locator("#setup-" + responsibility + "-primary").select_option(auditor_member)
        page.locator("#setup-" + responsibility + "-substitute").select_option(driver_member)
    page.locator("#setup-billing-primary").focus()
    page.keyboard.press("Home")
    page.keyboard.press("ArrowDown")
    assert page.evaluate("document.activeElement.id") == "setup-billing-primary", "Named choice stole keyboard focus"
    assert page.url.endswith("/empresas/organizacoes/" + org_a + "/configuracao/"), "Named choice changed organization"
    page.locator("#setup-billing-primary").select_option(auditor_member)
    for field, value in {"setup-settings-time-zone": "America/Sao_Paulo", "setup-approval-levels": "3", "setup-closure-days": "45", "setup-intake-days": "7", "setup-clock-skew": "0", "setup-delegation-days": "120", "setup-invitation-days": "14", "setup-activation-hours": "72"}.items():
        page.locator("#" + field).fill(value)
    page.locator("#setup-consumption-display").select_option("L_PER_100KM")
    for field in ["setup-review-trips", "setup-review-fuelings", "setup-review-expenses"]:
        page.locator("#" + field).uncheck()
    page.locator("#setup-invitation-confirmation").check()
    page.locator("#setup-reason").fill("Alteração sintética completa de configuração")
    settings_requests, settings_responses = [], []
    role_state = database_value("SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM org_role_assignments r WHERE org_id=%s", (org_a,))
    def settings_reply(route):
        settings_requests.append(route.request.post_data)
        response = route.fetch()
        settings_responses.append(response.json())
        assert response.status == 200, "Actual settings command failed"
        if len(settings_requests) == 1:
            route.abort("failed")
        else:
            route.fulfill(response=response)
    page.route("**/v1/orgs/" + org_a + "/settings/change", settings_reply)
    page.locator("#setup-save").click()
    expect(page.locator("#setup-retry")).to_be_visible()
    page.locator("#setup-retry").click()
    expect(page.locator("#setup-panel")).to_have_attribute("aria-busy", "false")
    expect(page.locator("#setup-retry")).to_be_hidden()
    assert len(settings_requests) == 2 and settings_requests[0] == settings_requests[1], "Settings retry changed complete intent"
    assert settings_responses[-1].get("replayed") is True
    assert read(org_a)["values"] == expected_values, "One or more current settings fields were lost or changed"
    assert database_value("SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM org_role_assignments r WHERE org_id=%s", (org_a,)) == role_state, "Naming responsibilities granted or changed authority"
    assert auditor_member not in page.locator("body").inner_text() and driver_member not in page.locator("body").inner_text(), "Picker exposed editable membership UUIDs"
    page.unroute("**/v1/orgs/" + org_a + "/settings/change", settings_reply)
    page.locator("#setup-history-load").click()
    expect(page.locator("#setup-history-list").get_by_role("button").first).to_be_visible()
    history = post("/v1/orgs/" + org_a + "/settings/history", {"limit": 100})
    assert len(history["items"]) >= 2, "Settings history omitted persisted versions"
    page.locator("#setup-history-list").get_by_role("button").last.click()
    expect(page.locator("#setup-history-detail")).to_be_visible()
    assert page.locator("#setup-history-values").inner_text().strip(), "Historical values are not accessible"
    assert read(org_a)["organization"]["state"] == "ACTIVE"

    # Another valid writer advances the server revision before this draft saves.
    page.locator("#setup-closure-days").fill("46")
    page.locator("#setup-reason").fill("Rascunho sintético em conflito")
    concurrent = read(org_a)
    changed = deepcopy(concurrent["values"])
    changed["closure_period_days"] = 47
    post("/v1/orgs/" + org_a + "/settings/change", {"schema_version": 1, "operation_id": str(uuid.uuid4()), "revision": concurrent["revision"], "values": changed, "reason": "Alteração concorrente sintética"})
    with page.expect_response(lambda response: response.url.endswith("/settings/change")) as conflicted:
        page.locator("#setup-save").click()
    assert conflicted.value.status == 409, "Stale settings draft did not conflict"
    expect(page.locator("#setup-panel")).to_have_attribute("aria-busy", "false")
    expect(page.locator("#setup-status")).not_to_have_text("")
    assert read(org_a)["values"]["closure_period_days"] == 47, "Stale draft overwrote current settings"
    page.locator("#setup-refresh").click()
    expect(page.locator("#setup-closure-days")).to_have_value("47")
    no_private_storage()
    page.screenshot(path="/tmp/enterprise-org014-settings-desktop.png", full_page=True)
    page.set_viewport_size({"width": 390, "height": 844})
    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Setup/settings mobile overflow"
    page.locator("#setup-history-load").focus()
    page.keyboard.press("Enter")
    expect(page.locator("#setup-history-list").get_by_role("button").first).to_be_visible()
    page.screenshot(path="/tmp/enterprise-org014-settings-mobile.png", full_page=True)
    page.evaluate("path => {history.pushState({},'',path); dispatchEvent(new PopStateEvent('popstate'));}", "/empresas/organizacoes/" + org_b + "/configuracao/")
    expect(page.locator("#setup-detail")).to_be_visible()
    expect(page.locator("#setup-consumption-display")).to_have_value("KM_PER_L")
    expect(page.locator("#setup-reason")).to_have_value("")
    assert page.locator("#setup-billing-primary option[value='" + auditor_member + "']").count() == 0, "Organization A's named choices reached B"
    page.evaluate("path => {history.pushState({},'',path); dispatchEvent(new PopStateEvent('popstate'));}", "/empresas/organizacoes/" + org_a + "/configuracao/")
    expect(page.locator("#setup-organization-name")).to_have_text(name_a)
    expect(page.locator("#setup-consumption-display")).to_have_value("L_PER_100KM")

    # The UI was authorized before the server proof became stale; the real write
    # refusal must clear every private draft instead of leaving a reusable form.
    change_fixture("UPDATE company_web_sessions SET authenticated_at=authenticated_at-interval '20 minutes',created_at=created_at-interval '20 minutes',expires_at=expires_at-interval '20 minutes' WHERE account_id=%s AND revoked_at IS NULL", (read(org_a)["context"]["account_id"],))
    page.locator("#setup-reason").fill("Tentativa sintética com confirmação vencida")
    with page.expect_response(lambda response: response.url.endswith("/settings/change")) as stale:
        page.locator("#setup-save").click()
    assert stale.value.status == 401 and stale.value.json()["error"]["code"] == "FRESH_AUTH_REQUIRED"
    expect(page.locator("#login-view")).to_be_visible()
    expect(page.locator("#setup-detail")).to_be_hidden()
    assert name_a not in page.locator("body").text_content()
    assert "Pessoa sintética de consulta" not in page.locator("body").text_content()
    expect(page.locator("#setup-name")).to_have_value("")
    expect(page.locator("#setup-reason")).to_have_value("")
    no_private_storage()
    sign_in(auditor["account_key"])
    page.evaluate("path => {history.pushState({},'',path); dispatchEvent(new PopStateEvent('popstate'));}", "/empresas/organizacoes/" + org_a + "/configuracao/")
    expect(page.locator("#setup-organization-name")).to_have_text(name_a)
    assert page.locator("#setup-save").is_hidden() or page.locator("#setup-save").is_disabled(), "Read-only member received write controls"
    page.locator("#setup-history-load").click()
    expect(page.locator("#setup-history-list").get_by_role("button").first).to_be_visible()
    page.locator("#logout-button").click()
    expect(page.locator("#login-view")).to_be_visible()
    expect(page.locator("#setup-name")).to_have_value("")
    expect(page.locator("#setup-reason")).to_have_value("")
    assert "Pessoa sintética de consulta" not in page.locator("body").text_content()
    no_private_storage()

    # Replaced-account, foreign URL and late response checks use the real API.
    outsider = create_account("setupoutsider")
    sign_in(owner_key)
    page.evaluate("path => {history.pushState({},'',path); dispatchEvent(new PopStateEvent('popstate'));}", "/empresas/organizacoes/" + org_a + "/configuracao/")
    expect(page.locator("#setup-detail")).to_be_visible()
    held = []
    def hold_read(route):
        response = route.fetch()
        assert response.status == 200
        held.append((route, response))
    page.route("**/v1/orgs/" + org_a + "/settings/read", hold_read)
    page.locator("#setup-refresh").click()
    deadline = time.monotonic() + 10
    while not held:
        assert time.monotonic() < deadline, "Late-response fixture did not reach the real source"
        page.wait_for_timeout(25)
    page.locator("#logout-button").click()
    expect(page.locator("#login-view")).to_be_visible()
    sign_in(outsider["account_key"])
    for route, response in held:
        try:
            route.fulfill(response=response)
        except PlaywrightError as failure:
            # The live transport aborts old work on logout; Chromium may have
            # already discarded that interception before its delayed reply.
            assert any(part in str(failure) for part in ("Invalid InterceptionId", "Request is already handled")), "Unexpected delayed-response completion failure"
    page.unroute("**/v1/orgs/" + org_a + "/settings/read", hold_read)
    page.wait_for_timeout(100)
    assert name_a not in page.locator("body").text_content(), "Late prior-account settings reappeared"
    with page.expect_response(lambda response: response.url.endswith("/settings/read")) as foreign:
        page.evaluate("path => {history.pushState({},'',path); dispatchEvent(new PopStateEvent('popstate'));}", "/empresas/organizacoes/" + org_a + "/configuracao/")
    assert foreign.value.status == 404, "Foreign setup selector disclosed source data"
    expect(page.locator("#setup-detail")).to_be_hidden()
    assert name_a not in page.locator("body").text_content()
    no_private_storage()

    # A dirty form must still lose private authority on tab return. Use an
    # ordinary ADMIN member so revocation never removes the last owner.
    editor = create_account("setupeditor")
    editor_id = editor.get("account_id") or editor.get("account", {}).get("account_id")
    assert editor_id, "Synthetic editor account lacks source identity"
    editor_member = named_member(editor_id, "Pessoa sintética de configuração", "ADMIN")
    probe_context = browser.new_context(ignore_https_errors=True, viewport={"width": 1280, "height": 900})
    probe = probe_context.new_page()
    probe.on("pageerror", lambda _error: errors.append("dirty setup browser error"))
    probe.goto(origin + "/empresas/organizacoes/" + org_a + "/configuracao/")
    expect(probe.locator("#login-view")).to_be_visible()
    try:
        probe.locator("#account-key").fill(editor["account_key"])
    except PlaywrightError:
        raise AssertionError("Synthetic editor sign-in field unavailable") from None
    probe.locator("#login-button").click()
    expect(probe.locator("#setup-detail")).to_be_visible()
    expect(probe.locator("#setup-save")).to_be_enabled()
    probe.locator("#setup-closure-days").fill("48")
    probe.locator("#setup-reason").fill("Rascunho privado antes de revogação")
    change_fixture("UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=%s AND membership_id=%s AND ended_at IS NULL", (org_a, editor_member), org_a)
    with probe.expect_response(lambda response: response.url.endswith("/settings/read")) as rechecked:
        probe.evaluate("document.dispatchEvent(new Event('visibilitychange'))")
    assert rechecked.value.status == 403, "Dirty tab return skipped current role authority"
    expect(probe.locator("#setup-detail")).to_be_hidden()
    expect(probe.locator("#setup-reason")).to_have_value("")
    assert name_a not in probe.locator("body").text_content(), "Revoked dirty setup retained private source metadata"
    assert probe.evaluate("localStorage.length + sessionStorage.length") == 0
    probe_context.close()
    assert not errors, "Setup browser JavaScript errors occurred"
    context.close()
    print("PASS: actual TLS/ordinary PostGIS setup creation/retry/reload recovery, default-off capability, persisted incomplete checklist, activation replay, all settings/pickers/history/conflict, read-only/foreign/session/dirty-revocation/late-response isolation, keyboard and mobile")

def employee_checks(browser, origin, dsn):
    import json
    import uuid
    context = browser.new_context(ignore_https_errors=True, viewport={"width": 1440, "height": 1000})
    page = context.new_page()
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.on("dialog", lambda dialog: dialog.accept())
    owner = context.request.post(origin + "/v1/accounts/keys", data={"username": "own" + secrets.token_hex(6)})
    assert owner.status == 201, "Synthetic manager creation failed"
    page.clock.install()
    page.goto(origin + "/empresas/")
    page.locator("#account-key").fill(owner.json()["account_key"])
    with page.expect_response(lambda response: response.url.endswith("/org-access/session/login")) as logged_in:
        page.locator("#login-button").click()
    csrf = logged_in.value.json()["csrf_token"]
    headers = {"Origin": origin, "X-CSRF": csrf}
    terms_source = (ROOT / "backend/internal/modules/organization/domain/onboarding.go").read_text()
    terms_version = re.search(r'SyntheticTermsVersion = "([^"]+)"', terms_source)[1]
    terms_digest = re.search(r'SyntheticTermsDigest = "([^"]+)"', terms_source)[1]
    organization = context.request.post(origin + "/v1/org-access/organizations/create", headers=headers, data={
        "schema_version": 1, "operation_id": str(uuid.uuid4()), "name": "Empresa de teste IDN016 " + secrets.token_hex(3),
        "time_zone": "America/Cuiaba", "kinds": ["OTHER"], "unit_name": "Unidade sintética",
        "terms": {"version": terms_version, "digest": terms_digest, "accepted": True}})
    assert organization.status == 201, "Synthetic organization setup failed"
    org_id = organization.json()["organization_id"]
    enable_test_operator_capability(dsn, org_id)
    activated = context.request.post(origin + "/v1/orgs/" + org_id + "/organization/activate", headers=headers,
        data={"schema_version": 1, "operation_id": str(uuid.uuid4()), "revision": organization.json()["revision"]})
    assert activated.status == 200, "Synthetic organization activation failed"
    other = context.request.post(origin + "/v1/org-access/organizations/create", headers=headers, data={
        "schema_version": 1, "operation_id": str(uuid.uuid4()), "name": "Empresa B sintética IDN016 " + secrets.token_hex(3),
        "time_zone": "America/Cuiaba", "kinds": ["OTHER"], "unit_name": "Unidade B sintética",
        "terms": {"version": terms_version, "digest": terms_digest, "accepted": True}})
    assert other.status == 201, "Second synthetic organization setup failed"
    other_org = other.json()["organization_id"]
    enable_test_operator_capability(dsn, other_org)
    other_active = context.request.post(origin + "/v1/orgs/" + other_org + "/organization/activate", headers=headers,
        data={"schema_version": 1, "operation_id": str(uuid.uuid4()), "revision": other.json()["revision"]})
    assert other_active.status == 200, "Second synthetic organization activation failed"
    page.locator("#retry-button").click()
    expect(page.locator("#page-title")).to_have_text("Escolha sua empresa")
    page.locator("#organization-select").select_option(org_id)
    page.locator("#organization-open").click()
    expect(page.locator("#employee-access")).to_be_visible()
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    created_employees = []

    def add_employee(suffix):
        name = "Pessoa sintética IDN016 " + suffix + secrets.token_hex(3)
        page.locator("#employee-create").evaluate("element => element.open = true")
        page.locator("#employee-name").fill(name)
        page.locator("#employee-registration").fill("SYN-" + secrets.token_hex(4))
        page.locator("#employee-email").fill("")
        page.locator("#employee-phone").fill("")
        page.locator("#employee-job").fill("Motorista sintético")
        page.locator("#employee-review").click()
        expect(page.locator("#employee-status")).to_have_text("Nenhum possível duplicado encontrado. Confirme para salvar.")
        with page.expect_response(lambda response: response.url.endswith("/employees/create")) as saved:
            page.locator("#employee-save").click()
        assert saved.value.status == 201, "Panel employee creation failed"
        created_employees.append(saved.value.json()["employee_id"])
        expect(page.locator("#employee-detail-name")).to_have_text(name)
        expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
        return name

    def issue(role):
        page.locator("#invitation-role").select_option(role)
        with page.expect_response(lambda response: response.url.endswith("/invitations/issue")) as issued:
            page.locator("#invitation-issue").click()
        assert issued.value.status == 200, "Panel invitation issue failed"
        value = issued.value.json()
        expect(page.locator("#invitation-delivery")).to_be_visible()
        expect(page.locator("#delivery-code")).to_have_value(value["delivery"]["code"])
        assert value["delivery"]["code"] not in page.url, "Invitation code reached navigation"
        assert page.evaluate("localStorage.length + sessionStorage.length") == 0
        assert page.evaluate("document.activeElement.id") == "delivery-title"
        expect(page.locator("#employee-fields")).to_contain_text("Convidado")
        return value

    def close_delivery():
        page.locator("#delivery-close").click()
        expect(page.locator("#delivery-code")).to_have_value("")
        expect(page.locator("#delivery-link")).to_have_value("")

    first_name = add_employee("A")
    first = issue("DRIVER:SELF")
    first_code = first["delivery"]["code"]
    close_delivery()
    with page.expect_response(lambda response: response.url.endswith("/resend")) as resent:
        page.get_by_role("button", name="Reemitir convite", exact=True).click()
    assert resent.value.status == 200, "Panel invitation resend failed"
    second_code = resent.value.json()["delivery"]["code"]
    assert second_code != first_code, "Resend reused the old secret"
    expect(page.locator("#delivery-code")).to_have_value(second_code)
    page.locator("#organization-select").select_option(other_org)
    page.locator("#organization-open").click()
    expect(page.locator("#employee-access")).to_be_visible()
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    expect(page.locator("#delivery-code")).to_have_value("")
    assert first_name not in page.locator("#workspace").inner_text(), "Prior organization employee survived the switch"
    page.locator("#organization-select").select_option(org_id)
    page.locator("#organization-open").click()
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    page.get_by_role("button", name="Abrir " + first_name, exact=True).click()
    expect(page.locator("#employee-detail-name")).to_have_text(first_name)
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    with page.expect_response(lambda response: response.url.endswith("/revoke")) as revoked:
        page.get_by_role("button", name="Revogar convite", exact=True).click()
    assert revoked.value.status == 200, "Panel invitation revocation failed"
    expect(page.locator("#employee-status")).to_have_text("Revogação confirmada no servidor.")
    expect(page.locator("#employee-fields")).to_contain_text("Sem acesso")
    assert first_code not in page.locator("body").inner_text() and second_code not in page.locator("body").inner_text()

    def redeem(value):
        recipient = browser.new_context(ignore_https_errors=True)
        created = recipient.request.post(origin + "/v1/accounts/keys", data={"username": "rec" + secrets.token_hex(6)})
        assert created.status == 201, "Synthetic recipient creation failed"
        personal = recipient.request.post(origin + "/v1/accounts/keys/login", data={"account_key": created.json()["account_key"]})
        assert personal.status == 200
        proof = personal.json()["session"]
        signed_in = recipient.request.post(origin + "/v1/org-access/session/login", headers={"Origin": origin}, data={"family_id": proof["family_id"], "access_token": proof["access_token"]})
        assert signed_in.status == 200
        recipient_headers = {"Origin": origin, "X-CSRF": signed_in.json()["csrf_token"]}
        code = value["delivery"]["code"]
        preview = recipient.request.post(origin + "/v1/org-access/invitations/preview", headers=recipient_headers, data={"code": code})
        assert preview.status == 200
        accepted = recipient.request.post(origin + "/v1/org-access/invitations/accept", headers=recipient_headers, data={"schema_version": 1, "operation_id": str(uuid.uuid4()), "code": code, "consent": True, "consent_digest": preview.json()["consent_digest"]})
        assert accepted.status == 200 and accepted.json()["membership_state"] == "INVITED", "Privileged invitation skipped manager confirmation"
        recipient.close()

    def open_employee(name):
        page.locator("#employees-refresh").click()
        expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
        page.get_by_role("button", name="Abrir " + name, exact=True).click()
        expect(page.locator("#employee-detail-name")).to_have_text(name)
        expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")

    for action, result in [("confirm", "Confirmação registrada pelo servidor."), ("reject", "Recusa registrada pelo servidor.")]:
        name = add_employee(action)
        value = issue("ADMIN:ORG")
        close_delivery()
        redeem(value)
        open_employee(name)
        page.get_by_role("button", name="Consultar aceite", exact=True).click()
        expect(page.locator("#invitation-admission")).to_be_visible()
        expect(page.locator("#admission-description")).to_contain_text("Aguardando confirmação")
        with page.expect_response(lambda response: response.url.endswith("/" + action)) as managed:
            page.locator("#admission-" + action).click()
        assert managed.value.status == 200, "Manager admission action failed"
        expect(page.locator("#employee-status")).to_have_text(result)
        expect(page.locator("#employee-fields")).to_contain_text("Sem acesso" if action == "reject" else "Ativo")

    page.set_viewport_size({"width": 390, "height": 844})
    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Employee panel mobile overflow"
    page.screenshot(path="/tmp/enterprise-idn016-employees-mobile.png", full_page=True)
    page.set_viewport_size({"width": 1440, "height": 1000})
    page.screenshot(path="/tmp/enterprise-idn016-employees-desktop.png", full_page=True)
    # Keyboard reaches the selected employee and native actions remain operable.
    page.locator("#employees-refresh").focus()
    page.keyboard.press("Enter")
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    assert page.locator("#employee-detail").is_hidden()
    page.clock.fast_forward(20 * 60 * 1000)
    page.locator("#employees-refresh").click()
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    page.clock.fast_forward(11 * 60 * 1000)
    expect(page.locator("#employee-access")).to_be_visible()
    expect(page.locator("#login-view")).to_be_hidden()
    page.locator("#logout-button").click()
    expect(page.locator("#employee-access")).to_be_hidden()
    assert first_name not in page.locator("body").inner_text()
    assert page.locator("#delivery-code").input_value() == ""
    page.locator("#account-key").fill(owner.json()["account_key"])
    page.locator("#login-button").click()
    expect(page.locator("#page-title")).to_have_text("Escolha sua empresa")
    page.locator("#organization-select").select_option(org_id)
    page.locator("#organization-open").click()
    expect(page.locator("#employee-access")).to_be_visible()
    expect(page.locator("#employee-access")).to_have_attribute("aria-busy", "false")
    add_employee("fresh-refusal")
    issue("DRIVER:SELF")
    page.route("**/v1/orgs/" + org_id + "/employees/list", lambda route: route.fulfill(status=401, content_type="application/json", headers={"Cache-Control": "no-store"}, body='{"error":{"code":"FRESH_AUTH_REQUIRED"}}'))
    page.locator("#employees-refresh").click()
    expect(page.locator("#login-view")).to_be_visible()
    expect(page.locator("#employee-access")).to_be_hidden()
    expect(page.locator("#selected-company")).to_be_hidden()
    expect(page.locator("#delivery-code")).to_have_value("")
    assert not errors, "Employee panel JavaScript errors occurred"
    print("PASS: actual account/context/employee APIs, employee create, current grant options, first-only delivery, resend/revoke, WAITING confirmation/rejection, keyboard actions, responsive panel, active idle deadline, logout and immediate fresh-refusal clearing")
    context.close()


if __name__ == "__main__":
    with disposable_database() as test_dsn:
        run(test_dsn)
