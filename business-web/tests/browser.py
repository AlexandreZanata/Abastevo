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
from urllib.parse import urlparse, quote
import psycopg
from psycopg import sql
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


@contextlib.contextmanager
def ordinary_runtime():
    """Exact local C1 test grants; runtime cannot own/bypass private tables."""
    role = "emp_web_runtime_" + secrets.token_hex(8)
    password = secrets.token_hex(24)
    connection = psycopg.connect(DSN, autocommit=True)
    try:
        connection.execute(sql.SQL("CREATE ROLE {} LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD {}").format(sql.Identifier(role), sql.Literal(password)))
        grants = [
            "GRANT USAGE ON SCHEMA public TO {}",
            "GRANT SELECT,INSERT,UPDATE ON accounts,account_session_families,identity_rate_windows,organizations,org_units,org_employee_identities,org_employee_profiles,org_memberships,org_role_assignments,org_capabilities,org_settings_heads,org_invitations,org_invitation_acceptances TO {}",
            "GRANT SELECT,INSERT ON account_key_credentials,org_onboarding_commands,org_audit_events,org_employee_commands,org_settings_versions,org_settings_designations,org_settings_notices,org_invitation_commands,org_invitation_notices,org_invitation_locators,org_invitation_person_commands,org_invitation_acceptance_notices,org_role_commands TO {}",
            "GRANT SELECT,UPDATE ON org_teams,org_team_links,org_ownership_transfers,org_device_enrollments TO {}",
            "GRANT SELECT ON schema_migrations,payment_billing_exits,account_contributor_bindings,org_establishments,org_legal_entities,org_settings_commands TO {}",
            "GRANT SELECT,INSERT,UPDATE,DELETE ON account_key_admission_events,company_web_sessions TO {}",
            "GRANT USAGE ON SEQUENCE account_key_admission_events_id_seq TO {}",
        ]
        for statement in grants:
            connection.execute(sql.SQL(statement).format(sql.Identifier(role)))
        runtime = parsed._replace(netloc=quote(role) + ":" + quote(password) + "@" + parsed.hostname + ":" + str(parsed.port or 5432)).geturl()
        yield runtime
    finally:
        connection.execute(sql.SQL("DROP OWNED BY {}").format(sql.Identifier(role)))
        connection.execute(sql.SQL("DROP ROLE IF EXISTS {}").format(sql.Identifier(role)))
        connection.close()


def enable_test_operator_capability(org_id):
    # Explicit operator fixture in this disposable DB, never a runtime provider.
    with psycopg.connect(DSN, autocommit=True) as connection:
        connection.execute("INSERT INTO org_capabilities(org_id,capability,enabled) VALUES (%s,'organizations',true)", (org_id,))


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
        with ordinary_runtime() as runtime_dsn, (temp / "api.log").open("w") as log:
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
        employee_checks(browser, origin)
        browser.close()

def employee_checks(browser, origin):
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
    enable_test_operator_capability(org_id)
    activated = context.request.post(origin + "/v1/orgs/" + org_id + "/organization/activate", headers=headers,
        data={"schema_version": 1, "operation_id": str(uuid.uuid4()), "revision": organization.json()["revision"]})
    assert activated.status == 200, "Synthetic organization activation failed"
    other = context.request.post(origin + "/v1/org-access/organizations/create", headers=headers, data={
        "schema_version": 1, "operation_id": str(uuid.uuid4()), "name": "Empresa B sintética IDN016 " + secrets.token_hex(3),
        "time_zone": "America/Cuiaba", "kinds": ["OTHER"], "unit_name": "Unidade B sintética",
        "terms": {"version": terms_version, "digest": terms_digest, "accepted": True}})
    assert other.status == 201, "Second synthetic organization setup failed"
    other_org = other.json()["organization_id"]
    enable_test_operator_capability(other_org)
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
    run()
