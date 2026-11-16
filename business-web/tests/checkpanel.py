#!/usr/bin/env python3
"""Local-only Chromium check of the EMP-CHECK-004 checklist and incident panel.

Serves the built dist on loopback and answers the company routes from an
in-memory synthetic server through Playwright request interception, so it
needs neither the Go API nor a database. It proves the rendered panel,
keyboard flow, focus, labels, responsive layout, the server-read asset block,
SELF_APPROVAL, CONFLICT, a vanished report and the content action that exists
only with the content hint; the actual-API proof of the routes is the
PostGIS/HTTP suite. Requires the built dist and Python Playwright/Chromium.
Never targets a remote host.
"""
import json
import threading
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright, expect

DIST = Path(__file__).resolve().parents[1] / "dist"
if not (DIST / "index.html").is_file():
    raise SystemExit("Build business-web first")


def uid(n):
    return f"7b0e2a54-0000-4000-8000-{n:012d}"


ACCOUNT, ORG, MEMBER, VEHICLE, SPARE = uid(1), uid(2), uid(3), uid(20), uid(21)
BLOCKING, REVIEW, PLAIN = uid(30), uid(31), uid(32)
DAMAGE, CLOSED, STORED, UPLOADING, REPORTER, HANDLER = uid(40), uid(41), uid(50), uid(51), uid(70), uid(71)
AT = "2026-11-16T10:00:00Z"
CSRF = "abv_company_csrf_" + "a" * 64
JPEG = "/9j/2Q=="


class Dist(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        path = urlparse(self.path).path
        if path.startswith("/empresas/assets/") or path in ("/empresas/robots.txt",):
            self.path = path[len("/empresas"):]
            return super().do_GET()
        if path == "/empresas/" or path.startswith("/empresas/organizacoes/") or path == "/empresas/entrar/":
            self.path = "/index.html"
            return super().do_GET()
        self.send_error(404)


def run(i, **over):
    value = {"run_id": i, "asset_id": VEHICLE, "template_id": uid(60), "context": "DEPARTURE", "template_version": 1, "policy_revision": 1, "server_policy_revision": 1, "revision": 1,
             "started_at": "2026-11-16T09:40:00Z", "completed_at": "2026-11-16T09:50:00.123Z", "received_at": "2026-11-16T09:51:00Z", "device_outcome": "BLOCKING",
             "recheck_outcome": "BLOCKING", "outcome": "BLOCKING", "outcome_differs": False, "review_required": False, "template_superseded": False, "acknowledged": False,
             "issues": [2], "critical_issues": [2], "data_mode": "SYNTHETIC"}
    value.update(over)
    return value


def report(i, **over):
    value = {"report_id": i, "report_type": "DAMAGE", "asset_id": VEHICLE, "reporter_membership_id": REPORTER, "blocks_use": True, "state": "REPORTED", "revision": 1,
             "occurred_at": AT, "received_at": AT, "updated_at": AT, "description": "Lanterna traseira trincada ao manobrar", "location": "Doca 2", "history": [],
             "attachments": [{"document_id": STORED, "storage_state": "STORED", "linked_at": AT}, {"document_id": UPLOADING, "storage_state": "UPLOADING", "linked_at": AT}],
             "data_mode": "SYNTHETIC"}
    value.update(over)
    return value


def error(status, code):
    return status, {"error": {"code": code, "message": "Organization request unavailable or refused", "details": [], "trace_id": ""}}


class Synthetic:
    def __init__(self, permissions):
        self.permissions = permissions
        self.runs = {BLOCKING: run(BLOCKING), REVIEW: run(REVIEW, device_outcome="COMPLETED_OK", recheck_outcome="COMPLETED_WITH_ISSUES", outcome="COMPLETED_WITH_ISSUES",
                                                          outcome_differs=True, review_required=True, issues=[3], critical_issues=[]),
                     PLAIN: run(PLAIN, asset_id=SPARE, device_outcome="COMPLETED_OK", recheck_outcome="COMPLETED_OK", outcome="COMPLETED_OK", issues=[], critical_issues=[])}
        self.reports = {CLOSED: report(CLOSED, state="RESOLVED", revision=2, blocks_use=False, attachments=[], history=[
            {"revision": 2, "action": "RESOLVE", "from_state": "REPORTED", "to_state": "RESOLVED", "note": "Retrovisor ajustado", "actor_membership_id": HANDLER, "recorded_at": AT}]),
            DAMAGE: report(DAMAGE)}
        self.blocked = True
        self.scenario = {}
        self.calls = []

    def availability(self, asset):
        if asset == VEHICLE and self.blocked:
            return {"asset_id": asset, "availability": "BLOCKED", "causes": [{"availability": "BLOCKED", "kind": "CHECKLIST_RUN", "record_id": BLOCKING, "valid_from": "2026-11-16T09:50:00.123Z"}],
                    "missing_sources": [], "evaluated_at": AT}
        return {"asset_id": asset, "availability": "AVAILABLE", "causes": [], "missing_sources": [], "evaluated_at": "2026-11-16T10:06:00Z"}

    def step(self, rid, action, note, by=HANDLER):
        now = self.reports[rid]
        to = {"ACKNOWLEDGE": "ACKNOWLEDGED", "TREAT": "IN_TREATMENT", "RESOLVE": "RESOLVED", "DISMISS": "DISMISSED"}[action]
        item = {"revision": now["revision"] + 1, "action": action, "from_state": now["state"], "to_state": to, "actor_membership_id": by, "recorded_at": "2026-11-16T10:07:00Z"}
        if note:
            item["note"] = note
        self.reports[rid] = {**now, "state": to, "revision": item["revision"], "updated_at": item["recorded_at"], "history": now["history"] + [item]}
        return {"report_id": rid, "action": action, "state": to, "revision": item["revision"], "recorded_at": item["recorded_at"], "replayed": False}

    def answer(self, path, body, csrf):
        self.calls.append(path)
        if path == "/v1/org-access/session/read" and not csrf:
            return 401, {"error": {"code": "UNAUTHENTICATED"}}
        view = {"account_id": ACCOUNT, "authenticated_at": AT, "expires_at": "2099-01-01T00:00:00Z"}
        fixed = {
            "/v1/accounts/keys/login": {"account": {"account_id": ACCOUNT}, "session": {"family_id": uid(9), "access_token": "synthetic"}},
            "/v1/org-access/session/login": {**view, "csrf_token": CSRF},
            "/v1/org-access/session/read": view,
            "/v1/org-access/contexts/list": {"account_id": ACCOUNT, "organizations": [{"id": ORG, "membership_id": MEMBER, "name": "Synthetic organization"}], "next_after_org_id": ""},
            "/v1/org-access/contexts/read": {"account_id": ACCOUNT, "organization": {"id": ORG, "membership_id": MEMBER, "name": "Synthetic organization"}, "permissions": self.permissions, "invitation_grants": [], "invitation_validity_days": 7},
        }
        if path in fixed:
            return 200, fixed[path]
        tail = path[len(f"/v1/orgs/{ORG}/"):] if path.startswith(f"/v1/orgs/{ORG}/") else None
        scenario = self.scenario.get(tail)
        if scenario == "self":
            return error(409, "SELF_APPROVAL")
        if scenario == "not-found":
            return error(404, "NOT_FOUND")
        if scenario == "conflict":
            self.step(DAMAGE, "ACKNOWLEDGE", None)
            return error(409, "CONFLICT")
        reads = {
            "employees/list": {"items": [], "next_cursor": ""},
            "checklists/runs/list": {"items": [self.runs[BLOCKING], self.runs[REVIEW], self.runs[PLAIN]] if self.runs[BLOCKING]["revision"] == 1 else [self.runs[REVIEW], self.runs[BLOCKING], self.runs[PLAIN]],
                                     "next_cursor": "", "data_mode": "SYNTHETIC"},
            "incidents/reports/list": {"items": [{k: v for k, v in r.items() if k not in ("description", "location", "history", "attachments", "data_mode")} for r in self.reports.values()],
                                       "next_cursor": "", "data_mode": "SYNTHETIC"},
            "fleet/assets/list": {"items": [{"id": VEHICLE, "kind": "VEHICLE", "name": "Caminhão sintético", "fleet_number": "FR-001", "plate": "", "state": "ACTIVE", "unit_id": uid(80), "revision": 1},
                                            {"id": SPARE, "kind": "MACHINE", "name": "Pá carregadeira sintética", "fleet_number": "FR-002", "plate": "", "state": "ACTIVE", "unit_id": uid(80), "revision": 1}], "next_cursor": ""},
            f"fleet/assets/{VEHICLE}/availability/read": self.availability(VEHICLE),
            f"fleet/assets/{SPARE}/availability/read": self.availability(SPARE),
            f"documents/{STORED}/content/access": {"document_id": STORED, "capability": "cap-synthetic-PRIVATE", "expires_at": "2026-11-16T10:05:00Z"},
            f"documents/{STORED}/content/read": {"document_id": STORED, "data_mode": "SYNTHETIC", "media_type": "image/jpeg", "bytes": 4, "content_base64": JPEG},
        }
        for i, r in self.runs.items():
            reads[f"checklists/assets/{r['asset_id']}/runs/{i}/read"] = r
        for i, r in self.reports.items():
            reads[f"incidents/reports/{i}/read"] = r
        if tail in reads:
            return 200, reads[tail]
        if tail == f"checklists/runs/{BLOCKING}/release":
            self.runs[BLOCKING] = run(BLOCKING, revision=2, outcome="COMPLETED_WITH_ISSUES", released_at="2026-11-16T10:05:00Z")
            self.blocked = False
            return 200, {**self.runs[BLOCKING], "replayed": False}
        if tail == f"incidents/reports/{DAMAGE}/decide":
            if body.get("revision") != self.reports[DAMAGE]["revision"]:
                return error(409, "CONFLICT")
            return 200, self.step(DAMAGE, body["action"], body.get("note"))
        return error(404, "NOT_FOUND")


def session(browser, origin, api, errors):
    context = browser.new_context(viewport={"width": 1440, "height": 1000}, accept_downloads=True)
    page = context.new_page()
    page.on("pageerror", lambda e: errors.append(str(e)))
    page.on("dialog", lambda dialog: dialog.accept())

    def route(r):
        status, body = api.answer(urlparse(r.request.url).path, r.request.post_data_json or {}, r.request.headers.get("x-csrf", ""))
        r.fulfill(status=status, headers={"Content-Type": "application/json", "Cache-Control": "no-store"}, body=json.dumps(body))

    page.route("**/v1/**", route)
    page.goto(origin + "/empresas/")
    page.locator("#account-key").fill("synthetic-account-key")
    page.locator("#account-key").press("Enter")
    expect(page.locator("#organization-select")).to_be_enabled()
    page.locator("#organization-select").select_option(ORG)
    page.locator("#organization-open").click()
    expect(page.locator("#checks-panel")).to_be_visible()
    return context, page


def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), partial(Dist, directory=str(DIST)))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    origin = f"http://127.0.0.1:{server.server_address[1]}"
    errors = []
    try:
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            # A FLEET_MANAGER-like member without the content hint.
            api = Synthetic(["idn.employee.read"])
            context, page = session(browser, origin, api, errors)
            panel = page.locator("#checks-panel")
            assert not any("/checklists/" in c or "/incidents/" in c for c in api.calls), "the panel read records before the member asked"
            page.locator("#checks-load").focus()
            page.keyboard.press("Enter")
            expect(page.locator("#checks-runs > li")).to_have_count(3)
            expect(page.locator("#checks-runs > li").first).to_contain_text("Bloqueando o uso")
            expect(page.locator("#checks-runs > li").nth(1)).to_contain_text("revisão necessária")
            expect(page.locator("#checks-reports > li").first).to_contain_text("Marcado pelo relator como impeditivo")
            expect(page.locator("#checks-runs-at")).to_contain_text("Lista consultada em")
            page.locator("#checks-runs > li").first.get_by_role("button").focus()
            page.keyboard.press("Enter")
            expect(page.locator("#checks-detail")).to_be_visible()
            expect(page.locator("#checks-detail-title")).to_be_focused()
            expect(page.locator("#checks-detail-title")).to_contain_text("Caminhão sintético · FR-001")
            expect(page.locator("#checks-availability")).to_contain_text("Bloqueado")
            expect(page.locator("#checks-causes")).to_contain_text("Checklist bloqueante — este checklist")
            expect(page.locator("#checks-fields")).to_contain_text("item 2")
            expect(page.locator("#checks-release-form")).to_be_visible()
            unlabelled = page.evaluate("[...document.querySelectorAll('#checks-panel input, #checks-panel select')].filter(e => e.labels.length === 0).map(e => e.id)")
            assert unlabelled == [], unlabelled
            page.set_viewport_size({"width": 390, "height": 844})
            assert page.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "horizontal scroll on mobile"
            page.set_viewport_size({"width": 1440, "height": 1000})
            # Self-release: SELF_APPROVAL is named, nothing changes, the block stays.
            api.scenario[f"checklists/runs/{BLOCKING}/release"] = "self"
            page.locator("#release-reason").fill("Freios verificados na oficina")
            page.locator("#checks-release-submit").click()
            expect(page.locator("#checks-status")).to_contain_text("outro membro")
            expect(page.locator("#checks-availability")).to_contain_text("Bloqueado")
            expect(page.locator("#checks-retry")).to_be_hidden()
            # Another member's release: the run and the availability are read again.
            api.scenario.clear()
            page.locator("#release-reason").fill("Freios verificados na oficina")
            page.locator("#checks-release-submit").click()
            expect(page.locator("#checks-status")).to_contain_text("Liberação registrada no servidor")
            expect(page.locator("#checks-fields")).to_contain_text("Liberado em")
            expect(page.locator("#checks-fields")).to_contain_text("Resultado conferido pelo servidor")
            expect(page.locator("#checks-fields")).to_contain_text("Bloqueando o uso")
            expect(page.locator("#checks-availability")).to_contain_text("Disponível")
            expect(page.locator("#checks-release-form")).to_be_hidden()
            # A report without the content hint: the photo exists, no content action.
            page.locator("#checks-reports > li").first.get_by_role("button").click()
            expect(page.locator("#checks-report-body")).to_be_visible()
            expect(page.locator("#checks-attachments")).to_contain_text("Foto 1 anexada · Armazenada")
            expect(page.locator("#checks-attachments")).to_contain_text("Foto 2 anexada · Envio em andamento")
            expect(page.locator("#checks-attachments button")).to_have_count(0)
            expect(page.locator("#checks-content-help")).to_contain_text("exige a permissão de conteúdo")
            expect(page.locator("#checks-fields")).to_contain_text("ainda não é aplicado pelo servidor")
            # A concurrent handler: CONFLICT re-reads and says "já atualizado".
            api.scenario[f"incidents/reports/{DAMAGE}/decide"] = "conflict"
            page.locator("#decide-action").select_option("DISMISS")
            page.locator("#decide-note").fill("Sem dano aparente")
            page.locator("#checks-decide-submit").click()
            expect(page.locator("#checks-status")).to_contain_text("já atualizado")
            expect(page.locator("#checks-read-at")).to_contain_text("alteração concorrente")
            expect(page.locator("#checks-fields")).to_contain_text("Ciente")
            api.scenario.clear()
            page.locator("#decide-action").select_option("RESOLVE")
            page.locator("#decide-note").fill("Lanterna trocada na oficina interna")
            page.locator("#checks-decide-submit").click()
            expect(page.locator("#checks-status")).to_contain_text("Tratamento registrado no servidor")
            expect(page.locator("#checks-history")).to_contain_text("Lanterna trocada na oficina interna")
            expect(page.locator("#checks-decide-form")).to_be_hidden()
            # The report vanished for this member: the panel clears and reveals nothing.
            api.scenario[f"incidents/reports/{DAMAGE}/read"] = "not-found"
            page.locator("#checks-reload").click()
            expect(page.locator("#checks-status")).to_contain_text("não pôde ser confirmado")
            expect(page.locator("#checks-detail")).to_be_hidden()
            text = panel.inner_text()
            assert "Lanterna" not in text and "Caminhão sintético" not in text, "a refused panel kept private text"
            expect(page.locator("#checks-title")).to_be_focused()
            assert page.evaluate("localStorage.length + sessionStorage.length") == 0
            context.close()

            # A member with the content hint saves a STORED photo through the audited path.
            api = Synthetic(["idn.employee.read", "doc.document.view_content"])
            context, page = session(browser, origin, api, errors)
            page.locator("#checks-load").click()
            expect(page.locator("#checks-reports > li")).to_have_count(2)
            page.locator("#checks-reports > li").first.get_by_role("button").click()
            expect(page.locator("#checks-attachments button")).to_have_count(1)
            assert not any("/content/" in c for c in api.calls), "content was requested before the explicit action"
            with page.expect_download() as info:
                page.locator("#checks-attachments button").click()
            download = info.value
            assert download.suggested_filename == "foto-1.jpg", download.suggested_filename
            assert Path(download.path()).read_bytes() == b"\xff\xd8\xff\xd9"
            content = [c for c in api.calls if "/documents/" in c]
            assert content == [f"/v1/orgs/{ORG}/documents/{STORED}/content/access", f"/v1/orgs/{ORG}/documents/{STORED}/content/read"], content
            expect(page.locator("#checks-status")).to_contain_text("o acesso foi registrado")
            html = page.content()
            assert "cap-synthetic-PRIVATE" not in html and JPEG not in html, "the page kept the capability or the content"
            assert page.evaluate("localStorage.length + sessionStorage.length") == 0
            context.close()
            assert errors == [], errors
            browser.close()
    finally:
        server.shutdown()
    print("Checklist and incident panel browser check: PASS")


if __name__ == "__main__":
    main()
