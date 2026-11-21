#!/usr/bin/env python3
"""Exercise the submitter with fixture sitemaps and a loopback endpoint."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
from pathlib import Path
import runpy
import stat
import subprocess
import sys
import tempfile
import threading
import unittest
import xml.etree.ElementTree as XML


SCRIPT = Path(__file__).resolve().parents[1] / "indexnow.py"
KEY = "a" * 32
USER_AGENT = "abastevo-indexnow/1 (+https://abastevo.com.br/)"
NAMESPACE = "http://www.sitemaps.org/schemas/sitemap/0.9"


class IndexNow(unittest.TestCase):
    def setUp(self):
        self.requests = []
        self.agents = []
        self.posts = []
        self.statuses = []
        self.sitemaps = {}
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def refused(self):
                # The public edge answers 403 to the default urllib signature.
                agent = self.headers.get("User-Agent", "")
                fixture.agents.append(agent)
                if not agent.startswith("Python-urllib"):
                    return False
                self.send_response(403)
                self.end_headers()
                return True

            def do_GET(self):
                fixture.requests.append(self.path)
                if self.refused():
                    return
                body = fixture.sitemaps.get(self.path)
                self.send_response(404 if body is None else 200)
                self.end_headers()
                if body is not None:
                    self.wfile.write(body)

            def do_POST(self):
                fixture.requests.append(self.path)
                body = self.rfile.read(int(self.headers["Content-Length"]))
                if self.refused():
                    return
                fixture.posts.append((self.headers["Content-Type"], json.loads(body)))
                self.send_response(fixture.statuses.pop(0) if fixture.statuses else 202)
                self.end_headers()

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01})
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(thread.join)
        self.addCleanup(server.shutdown)
        self.origin = f"http://127.0.0.1:{server.server_port}"
        self.state = Path(self.enterContext(tempfile.TemporaryDirectory())) / "state.json"
        self.env = {name: value for name, value in os.environ.items()
                    if not name.startswith("ABASTEVO_INDEXNOW_")}
        self.env.update(
            ABASTEVO_INDEXNOW_ENABLED="1",
            ABASTEVO_INDEXNOW_ORIGIN=self.origin,
            ABASTEVO_INDEXNOW_KEY=KEY,
            ABASTEVO_INDEXNOW_ENDPOINT=self.origin + "/submit",
            ABASTEVO_INDEXNOW_STATE=str(self.state),
            NO_PROXY="127.0.0.1",
        )
        self.set_urls("/sitemap.xml", [("/first/", "2026-11-19")])
        self.set_urls("/sitemap-portal.xml", [("/second/", "")])

    def sitemap(self, root, entry, rows):
        doc = XML.Element(root, xmlns=NAMESPACE)
        for route, lastmod in rows:
            row = XML.SubElement(doc, entry)
            XML.SubElement(row, "loc").text = self.origin + route if route.startswith("/") else route
            if lastmod:
                XML.SubElement(row, "lastmod").text = lastmod
        return XML.tostring(doc)

    def set_urls(self, path, rows):
        self.sitemaps[path] = self.sitemap("urlset", "url", rows)

    def run_submitter(self, expected_code=0, **overrides):
        result = subprocess.run([sys.executable, SCRIPT], env=self.env | overrides,
                                text=True, capture_output=True, timeout=5)
        message = (result.stdout + result.stderr).strip()
        self.assertEqual(result.returncode, expected_code, message)
        self.assertEqual(len(message.splitlines()), 1, message)
        return message

    def accepted_state(self):
        self.run_submitter()
        self.posts.clear()
        return self.state.read_bytes()

    def posted_urls(self):
        return [url for _, payload in self.posts for url in payload["urlList"]]

    def test_first_run_submits_current_urls_and_saves_private_state(self):
        self.statuses = [200]
        self.assertEqual(self.run_submitter(), "indexnow: submitted=2 new=2 changed=0 removed=0 ignored=0")
        self.assertEqual(self.posts, [("application/json; charset=utf-8", {
            "host": "127.0.0.1", "key": KEY, "keyLocation": self.origin + "/" + KEY + ".txt",
            "urlList": [self.origin + "/first/", self.origin + "/second/"],
        })])
        self.assertEqual(json.loads(self.state.read_text()), {
            self.origin + "/first/": "2026-11-19", self.origin + "/second/": "",
        })
        self.assertEqual(stat.S_IMODE(self.state.stat().st_mode), 0o600)

    def test_every_request_names_the_client(self):
        self.run_submitter()
        self.assertEqual(self.requests, ["/sitemap.xml", "/sitemap-portal.xml", "/submit"])
        self.assertEqual(self.agents, [USER_AGENT] * 3)

    def test_no_change_makes_no_post(self):
        saved = self.accepted_state()
        self.assertEqual(self.run_submitter(), "indexnow: no change")
        self.assertEqual(self.posts, [])
        self.assertEqual(self.state.read_bytes(), saved)

    def test_changed_lastmod_submits_only_that_url(self):
        self.accepted_state()
        self.set_urls("/sitemap.xml", [("/first/", "2026-11-20")])
        self.assertEqual(self.run_submitter(), "indexnow: submitted=1 new=0 changed=1 removed=0 ignored=0")
        self.assertEqual(self.posted_urls(), [self.origin + "/first/"])
        self.assertEqual(json.loads(self.state.read_text())[self.origin + "/first/"], "2026-11-20")

    def test_removed_url_is_submitted_once(self):
        self.accepted_state()
        self.set_urls("/sitemap-portal.xml", [])
        self.assertEqual(self.run_submitter(), "indexnow: submitted=1 new=0 changed=0 removed=1 ignored=0")
        self.assertEqual(self.posted_urls(), [self.origin + "/second/"])
        self.assertNotIn(self.origin + "/second/", json.loads(self.state.read_text()))
        self.posts.clear()
        self.assertEqual(self.run_submitter(), "indexnow: no change")
        self.assertEqual(self.posts, [])

    def test_batches_are_sorted_and_limited_to_ten_thousand(self):
        self.set_urls("/sitemap.xml", [(f"/page/{index:05}/", "") for index in reversed(range(10001))])
        self.run_submitter()
        self.assertEqual([len(post["urlList"]) for _, post in self.posts], [10000, 2])
        self.assertEqual(len(set(self.posted_urls())), 10002)
        self.assertEqual(self.posted_urls(), sorted(self.posted_urls()))

    def test_foreign_urls_and_sitemaps_are_ignored(self):
        self.sitemaps["/sitemap-portal.xml"] = self.sitemap("sitemapindex", "sitemap", [
            ("/nested.xml", ""), ("https://other.invalid/sitemap.xml", ""),
        ])
        self.set_urls("/nested.xml", [("/second/", ""), ("https://other.invalid/ignored/", "")])
        self.assertEqual(self.run_submitter(), "indexnow: submitted=2 new=2 changed=0 removed=0 ignored=2")
        self.assertEqual(self.requests, ["/sitemap.xml", "/sitemap-portal.xml", "/nested.xml", "/submit"])
        self.assertEqual(self.posted_urls(), [self.origin + "/first/", self.origin + "/second/"])

    def test_refused_second_batch_keeps_only_the_accepted_batch(self):
        saved = json.loads(self.accepted_state())
        self.set_urls("/sitemap.xml", [(f"/page/{index:05}/", "") for index in range(10001)])
        self.statuses = [202, 403]
        self.assertIn("HTTP 403", self.run_submitter(1))
        self.assertEqual(len(self.posts), 2)
        first = self.posts[0][1]["urlList"]
        expected = dict(saved)
        for url in first:
            expected.pop(url, None) if url == self.origin + "/first/" else expected.update({url: ""})
        self.assertEqual(json.loads(self.state.read_bytes()), expected)
        self.posts.clear()
        # The batch that carried the removal and 9,999 new pages is not resent.
        self.assertEqual(self.run_submitter(), "indexnow: submitted=2 new=2 changed=0 removed=0 ignored=0")
        self.assertEqual(self.posted_urls(), [self.origin + "/page/09999/", self.origin + "/page/10000/"])

    def test_unexpected_success_code_does_not_save_state(self):
        self.statuses = [204]
        self.assertIn("HTTP 204", self.run_submitter(1))
        self.assertFalse(self.state.exists())

    def test_failed_sitemap_fetch_preserves_accepted_state(self):
        saved = self.accepted_state()
        del self.sitemaps["/sitemap-portal.xml"]
        self.assertIn("HTTP 404", self.run_submitter(1))
        self.assertEqual(self.posts, [])
        self.assertEqual(self.state.read_bytes(), saved)

    def test_malformed_or_nested_index_preserves_accepted_state(self):
        saved = self.accepted_state()
        self.sitemaps["/sitemap-portal.xml"] = b"<urlset"
        self.assertIn("sitemap", self.run_submitter(1))
        self.sitemaps["/sitemap-portal.xml"] = self.sitemap("sitemapindex", "sitemap", [("/nested.xml", "")])
        self.sitemaps["/nested.xml"] = self.sitemap("sitemapindex", "sitemap", [("/third.xml", "")])
        self.assertIn("nested sitemap index", self.run_submitter(1))
        self.assertNotIn("/third.xml", self.requests)
        self.assertEqual(self.posts, [])
        self.assertEqual(self.state.read_bytes(), saved)

    def test_empty_current_map_does_not_remove_every_url(self):
        saved = self.accepted_state()
        self.set_urls("/sitemap.xml", [])
        self.set_urls("/sitemap-portal.xml", [])
        self.assertIn("empty", self.run_submitter(1))
        self.assertEqual(self.posts, [])
        self.assertEqual(self.state.read_bytes(), saved)

    def test_disabled_makes_no_request(self):
        self.assertEqual(self.run_submitter(ABASTEVO_INDEXNOW_ENABLED="0", ABASTEVO_INDEXNOW_KEY=""),
                         "indexnow: disabled")
        self.assertEqual(self.requests, [])
        self.assertFalse(self.state.exists())

    def test_invalid_configuration_makes_no_request(self):
        self.assertIn("ABASTEVO_INDEXNOW_KEY", self.run_submitter(1, ABASTEVO_INDEXNOW_KEY="bad"))
        for origin in ("http://example.invalid", self.origin + "/path", "https://example.invalid?"):
            with self.subTest(origin=origin):
                self.assertIn("ABASTEVO_INDEXNOW_ORIGIN", self.run_submitter(1, ABASTEVO_INDEXNOW_ORIGIN=origin))
        self.assertEqual(self.requests, [])

    def test_invalid_endpoint_makes_no_request(self):
        for endpoint in ("file:///etc/hostname", "http://example.invalid/indexnow", self.origin + "/submit?x=1"):
            with self.subTest(endpoint=endpoint):
                self.assertIn("ABASTEVO_INDEXNOW_ENDPOINT",
                              self.run_submitter(1, ABASTEVO_INDEXNOW_ENDPOINT=endpoint))
        self.assertEqual(self.requests, [])

    def test_oversized_sitemap_is_refused(self):
        read_limited = runpy.run_path(str(SCRIPT))["read_limited"]
        self.assertEqual(read_limited(io.BytesIO(b"x" * 10), 10), b"x" * 10)
        with self.assertRaisesRegex(ValueError, "exceeded 10 bytes"):
            read_limited(io.BytesIO(b"x" * 11), 10)


if __name__ == "__main__":
    unittest.main()
