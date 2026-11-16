#!/usr/bin/env python3
"""Local-only Chromium check of the EMP-TRIP-022 mission board DOM.

Serves the built dist on loopback and answers the company routes from an
in-memory synthetic server through Playwright request interception, so it
needs neither the Go API nor a database. It proves the rendered board, keyboard
flow, focus, labels, responsive layout and the conflict/403 negatives; the
actual-API proof of the routes is the PostGIS/HTTP suite. Requires the built
dist and Python Playwright/Chromium. Never targets a remote host.
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


ACCOUNT, ORG, MEMBER, MISSION, VEHICLE, SPARE, IMPLEMENT, UNIT = uid(1), uid(2), uid(3), uid(10), uid(20), uid(21), uid(22), uid(30)
DRIVER, SECOND, CENTRE = uid(40), uid(41), uid(60)
AT = "2026-11-16T10:00:00Z"
CSRF = "abv_company_csrf_" + "a" * 64


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


def asset(i, kind="VEHICLE", state="ACTIVE"):
    return {"id": i, "kind": kind, "name": "Synthetic " + kind.lower(), "fleet_number": "FR-" + i[-3:], "plate": "", "serial_number": "", "usage_basis": "DISTANCE",
            "category": {"id": uid(90), "label": "Synthetic"}, "tanks": [], "expected_consumption": [], "state": state, "revision": 1, "unit_id": UNIT, "data_mode": "SYNTHETIC"}


class Synthetic:
    def __init__(self):
        self.mission = {"mission_id": MISSION, "unit_id": UNIT, "asset_id": VEHICLE, "title": "Entrega sintética no cliente", "starts_at": "2026-11-17T11:00:00Z",
                        "ends_at": "2026-11-17T13:00:00Z", "state": "DRAFT", "revision": 1, "created_at": AT, "updated_at": AT, "data_mode": "SYNTHETIC"}
        self.scenario = {}
        self.calls = []

    def advance(self, state, **change):
        self.mission = {**self.mission, **change, "state": state, "revision": self.mission["revision"] + 1}
        out = {"mission_id": MISSION, "revision": self.mission["revision"], "state": state, "replayed": False, "data_mode": "SYNTHETIC"}
        if self.mission.get("assignee_employee_id"):
            out["assignee_employee_id"] = self.mission["assignee_employee_id"]
        return out

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
            "/v1/org-access/contexts/read": {"account_id": ACCOUNT, "organization": {"id": ORG, "membership_id": MEMBER, "name": "Synthetic organization"}, "permissions": ["idn.employee.read"], "invitation_grants": [], "invitation_validity_days": 7},
        }
        if path in fixed:
            return 200, fixed[path]
        tail = path[len(f"/v1/orgs/{ORG}/"):] if path.startswith(f"/v1/orgs/{ORG}/") else None
        refusal = self.scenario.get(tail)
        if refusal == "forbidden":
            return 403, {"error": {"code": "FORBIDDEN", "message": "Organization request unavailable or refused", "details": [{"field": "permission", "code": "trip.mission.manage"}], "trace_id": ""}}
        if refusal == "conflict":
            self.advance("ASSIGNED", assignee_employee_id=DRIVER)
            return 409, {"error": {"code": "CONFLICT", "message": "Organization request unavailable or refused", "details": [], "trace_id": ""}}
        reads = {
            "employees/list": {"items": [], "next_cursor": ""},
            "missions/list": {"items": [self.mission], "next_cursor": "", "data_mode": "SYNTHETIC"},
            f"missions/{MISSION}/read": self.mission,
            f"missions/{MISSION}/assignees/list": {"items": [{"employee_id": DRIVER, "display_name": "Bruno Sintético", "registration_number": "SYN-001"}, {"employee_id": SECOND, "display_name": "Carla Sintética", "registration_number": ""}], "next_cursor": "", "data_mode": "SYNTHETIC"},
            "fleet/assets/list": {"items": [asset(VEHICLE), asset(SPARE, "MACHINE"), asset(IMPLEMENT, "IMPLEMENT")], "next_cursor": ""},
            "cost-centres/list": {"items": [{"id": CENTRE, "name": "Contrato sintético", "display_name": "Contrato sintético", "code": "CC-1", "state": "ACTIVE", "revision": 1, "data_mode": "SYNTHETIC"}], "next_cursor": ""},
            f"fleet/assets/{VEHICLE}/availability/read": {"asset_id": VEHICLE, "availability": "RESERVED", "causes": [{"availability": "RESERVED", "kind": "RESERVATION", "record_id": uid(70), "valid_from": "2026-11-17T10:00:00Z", "valid_to": "2026-11-17T12:00:00Z"}], "missing_sources": ["MAINTENANCE_HOLD"], "evaluated_at": AT},
            f"fleet/assets/{VEHICLE}/reservations/list": {"items": [{"reservation_id": uid(70), "asset_id": VEHICLE, "purpose": "PRIVATE-PURPOSE", "starts_at": "2026-11-17T10:00:00Z", "ends_at": "2026-11-17T12:00:00Z", "no_show_tolerance_minutes": 30, "state": "CONFIRMED", "requested_at": AT, "employee_id": uid(71)}], "next_cursor": "", "evaluated_at": AT},
            f"fleet/assets/{VEHICLE}/blocks/list": {"items": [{"block_id": uid(80), "asset_id": VEHICLE, "reason_kind": "SAFETY", "valid_from": "2026-11-18T00:00:00Z", "recorded_at": AT, "note": "PRIVATE-NOTE"}], "next_cursor": "", "evaluated_at": AT},
        }
        if tail in reads:
            return 200, reads[tail]
        if tail == f"missions/{MISSION}/assign":
            out = self.advance("ASSIGNED", assignee_employee_id=body["assignee_employee_id"])
            out["asset_warning"] = {"code": "ASSET_UNAVAILABLE", "availability": "RESERVED"}
            return 200, out
        if tail == f"missions/{MISSION}/cancel":
            return 200, self.advance("CANCELLED")
        return 404, {"error": {"code": "NOT_FOUND", "message": "Organization request unavailable or refused", "details": [], "trace_id": ""}}


def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), partial(Dist, directory=str(DIST)))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    origin = f"http://127.0.0.1:{server.server_address[1]}"
    api = Synthetic()

    def route(r):
        request = r.request
        status, body = api.answer(urlparse(request.url).path, request.post_data_json or {}, request.headers.get("x-csrf", ""))
        r.fulfill(status=status, headers={"Content-Type": "application/json", "Cache-Control": "no-store"}, body=json.dumps(body))

    try:
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(viewport={"width": 1440, "height": 1000})
            page = context.new_page()
            errors = []
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.on("dialog", lambda dialog: dialog.accept())
            page.route("**/v1/**", route)
            page.goto(origin + "/empresas/")
            page.locator("#account-key").fill("synthetic-account-key")
            page.locator("#account-key").press("Enter")
            expect(page.locator("#organization-select")).to_be_enabled()
            page.locator("#organization-select").select_option(ORG)
            page.locator("#organization-open").click()
            board = page.locator("#mission-board")
            expect(board).to_be_visible()
            assert not any("/missions/" in c for c in api.calls), "the board read missions before the member asked"
            page.locator("#missions-load").focus()
            page.keyboard.press("Enter")
            expect(page.locator("#mission-list li")).to_have_count(1)
            expect(page.locator("#mission-listed-at")).to_contain_text("Lista consultada em")
            opener = page.get_by_role("button", name="Abrir Entrega sintética no cliente")
            opener.focus()
            page.keyboard.press("Enter")
            expect(page.locator("#mission-detail")).to_be_visible()
            expect(page.locator("#mission-fields")).to_contain_text("Rascunho")
            expect(page.locator("#mission-detail-title")).to_be_focused()
            expect(page.locator("#agenda-availability")).to_contain_text("Reservado")
            expect(page.locator("#agenda-reservations")).to_contain_text("coincide com o horário da missão")
            text = board.inner_text()
            assert "PRIVATE-PURPOSE" not in text and "PRIVATE-NOTE" not in text, "the agenda displayed free text"
            vehicles = page.locator("#mission-vehicle option").all_inner_texts()
            assert len(vehicles) == 3 and not any("implement" in v for v in vehicles), vehicles
            people = page.locator("#assign-employee option").all_inner_texts()
            assert len(people) == 3 and "Bruno Sintético · Matrícula SYN-001" in people, people
            unlabelled = page.evaluate("[...document.querySelectorAll('#mission-board input, #mission-board select')].filter(e => e.labels.length === 0).map(e => e.id)")
            assert unlabelled == [], unlabelled
            page.set_viewport_size({"width": 390, "height": 844})
            assert page.evaluate("document.documentElement.scrollWidth <= window.innerWidth"), "horizontal scroll on mobile"
            page.set_viewport_size({"width": 1440, "height": 1000})
            page.locator("#assign-employee").select_option(DRIVER)
            page.locator("#mission-assign-submit").focus()
            page.keyboard.press("Enter")
            expect(page.locator("#mission-status")).to_contain_text("Designação registrada no servidor")
            expect(page.locator("#mission-fields")).to_contain_text("Designada — aguardando resposta")
            expect(page.locator("#mission-warning")).to_be_visible()
            assert "Aceita" not in page.locator("#mission-fields").inner_text(), "an unconfirmed state was shown as accepted"
            # A concurrent change: the server moved the mission; the board re-reads it.
            api.scenario[f"missions/{MISSION}/assign"] = "conflict"
            page.locator("#assign-employee").select_option(SECOND)
            page.locator("#mission-assign-submit").click()
            expect(page.locator("#mission-status")).to_contain_text("A missão mudou")
            expect(page.locator("#mission-list")).to_contain_text("Revisão 3")
            expect(page.locator("#mission-read-at")).to_contain_text("alteração concorrente")
            api.scenario.clear()
            api.mission = {**api.mission, "state": "ACCEPTED", "revision": 4, "answer": {"response": "ACCEPT"}}
            page.locator("#mission-reload").click()
            expect(page.locator("#mission-fields")).to_contain_text("Aceita pela pessoa designada")
            # The server refuses the write: the board clears and reveals nothing.
            api.scenario[f"missions/{MISSION}/cancel"] = "forbidden"
            page.locator("#cancel-reason").fill("Cliente cancelou")
            page.locator("#mission-cancel-submit").click()
            expect(page.locator("#mission-status")).to_contain_text("não pôde ser confirmado")
            expect(page.locator("#mission-detail")).to_be_hidden()
            assert "Entrega sintética" not in board.inner_text(), "a refused board kept private mission text"
            expect(page.locator("#missions-title")).to_be_focused()
            assert page.evaluate("localStorage.length + sessionStorage.length") == 0
            assert errors == [], errors
            browser.close()
    finally:
        server.shutdown()
    print("Mission board browser check: PASS")


if __name__ == "__main__":
    main()
