#!/usr/bin/env python3
"""Search Console report: classifier, paging, aggregates and token handling.

The HTTP layer is an injected function that serves fixture sitemaps and Search
Console answers, so no test reaches Google or the public site. The one test of
the real urllib transport talks to a loopback server only.
"""
import contextlib
import csv
from datetime import date
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import tempfile
import threading
import unittest
from unittest import mock
import xml.etree.ElementTree as XML


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/seo/search_report.py"
spec = importlib.util.spec_from_file_location("search_report", SCRIPT)
report = importlib.util.module_from_spec(spec)
spec.loader.exec_module(report)

ORIGIN = "https://abastevo.com.br"
SITE = "sc-domain:abastevo.com.br"
TOKEN = "fixture-access-token-0123456789"
SITES = "https://www.googleapis.com/webmasters/v3/sites/"
ANALYTICS = SITES + "sc-domain%3Aabastevo.com.br/searchAnalytics/query"
INSPECTION = "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"
NAMESPACE = "http://www.sitemaps.org/schemas/sitemap/0.9"
STATION = "/postos/6f1d3c0e-9d0b-4a55-8d3e-2b7b6f0f8a11/"

# Every family in the default locale, then the shapes that belong to none.
DEFAULT_LOCALE_ROUTES = {
    "": "home",
    "/": "home",
    "/guias/": "guias",
    "/guias/dados/": "guias",
    "/guias/etanol-ou-gasolina/": "guias",
    "/ferramentas/": "ferramentas",
    "/ferramentas/custo-de-viagem/": "ferramentas",
    "/como-usar/": "como-usar",
    "/como-usar/fotografar-preco/": "como-usar",
    "/glossario/": "glossario",
    "/glossario/paridade/": "glossario",
    "/sobre/": "institucional",
    "/politica-editorial/": "institucional",
    "/privacidade/": "institucional",
    "/termos/": "institucional",
    "/excluir-conta/": "institucional",
    "/contribuidores/": "institucional",
    "/precos/": "precos-hub",
    "/precos/gasolina-comum/": "combustivel",
    "/precos/sp/": "estado",
    "/precos/sp": "estado",
    "/precos/es/": "estado",
    "/precos/mt/sorriso/": "cidade",
    "/precos/es/vitoria/": "cidade",
    "/precos/ba/santa-terezinha-2928505/": "cidade",
    "/precos/etanol/sp/": "combustivel-estado",
    "/precos/diesel-s10/mt/sorriso/": "combustivel-cidade",
    "/precos/etanol-ou-gasolina/": "paridade",
    STATION: "posto",
    "/forum/": "forum",
    "/forum/precos/": "forum",
    "/forum/precos/etanol-sobe-no-centro-oeste-1a2b3c4d/": "forum",
    "/fiscalizacao/": "fiscalizacao",
    "/fiscalizacao/2/": "fiscalizacao",
    "/dados/": "dados",
    "/dados/precos-postos.csv": "dados",
    "/consultar/": "consultar",
    "/metodologia/": "metodologia",
    "/curiosidades/": "curiosidades",
    "/404.html": "outros",
    "/sitemap.xml": "outros",
    "/assets/styles.css": "outros",
    "/energia/": "outros",
    "/precos/uf/MT/municipio/5107925/": "outros",
    "/precos/SP/": "outros",
    "/precos/sao-paulo/": "outros",
    "/precos/sp/Sao_Paulo/": "outros",
    "/precos/sp/sao-paulo/centro/": "outros",
    "/precos/etanol/sao-paulo/": "outros",
    "/precos/etanol/sp/sao-paulo/centro/": "outros",
    "/precos/etanol-ou-gasolina/sp/": "outros",
}
LOCALE_ROUTES = {
    "/en": ("en", "home"),
    "/en/": ("en", "home"),
    "/en/guides/": ("en", "guias"),
    "/en/guides/ethanol-or-gasoline/": ("en", "guias"),
    "/en/tools/": ("en", "ferramentas"),
    "/en/tools/ethanol-or-gasoline-calculator/": ("en", "ferramentas"),
    "/en/about/": ("en", "institucional"),
    "/en/fuel-prices/": ("en", "precos-hub"),
    "/en/fuel-prices/regular-gasoline/": ("en", "combustivel"),
    "/en/fuel-prices/sp/": ("en", "estado"),
    "/en/fuel-prices/sp/sao-paulo/": ("en", "cidade"),
    "/en/fuel-prices/regular-gasoline/sp/": ("en", "combustivel-estado"),
    "/en/fuel-prices/regular-gasoline/sp/sao-paulo/": ("en", "outros"),
    "/en/fuel-prices/gasolina-comum/": ("en", "outros"),
    "/en/precos/sp/": ("en", "outros"),
    "/en/methodology/": ("en", "metodologia"),
    "/en/data/": ("en", "dados"),
    "/en/insights/": ("en", "curiosidades"),
    "/en/guias/gnv-compensa/": ("en", "outros"),
    "/en/ferramentas/custo-de-viagem/": ("en", "outros"),
    "/en/robots.txt": ("en", "outros"),
    "/es/": ("es", "home"),
    "/es/guias/": ("es", "guias"),
    "/es/guias/etanol-o-gasolina/": ("es", "guias"),
    "/es/herramientas/": ("es", "ferramentas"),
    "/es/herramientas/etanol-o-gasolina/": ("es", "ferramentas"),
    "/es/sobre/": ("es", "institucional"),
    "/es/precios/": ("es", "precos-hub"),
    "/es/precios/gasolina-comun/": ("es", "combustivel"),
    "/es/precios/pr/": ("es", "estado"),
    "/es/precios/pr/foz-do-iguacu/": ("es", "cidade"),
    "/es/precios/gasolina-comun/pr/": ("es", "combustivel-estado"),
    "/es/precios/gasolina-comun/pr/foz-do-iguacu/": ("es", "outros"),
    "/es/metodologia/": ("es", "metodologia"),
    "/es/datos/": ("es", "dados"),
    "/es/curiosidades/": ("es", "curiosidades"),
    "/es/forum/": ("es", "outros"),
    "/es/consultar/": ("es", "outros"),
    "/es/fiscalizacao/": ("es", "outros"),
    "/es/postos/": ("es", "outros"),
    "/es/ferramentas/custo-de-viagem/": ("es", "outros"),
    "/es/tools/ethanol-or-gasoline-calculator/": ("es", "outros"),
}


def sitemap(root, entry, locations):
    doc = XML.Element(root, xmlns=NAMESPACE)
    for location in locations:
        XML.SubElement(XML.SubElement(doc, entry), "loc").text = location
    return XML.tostring(doc)


def row(keys, clicks, impressions, position=1.0):
    return {"keys": keys, "clicks": clicks, "impressions": impressions,
            "ctr": clicks / impressions if impressions else 0, "position": position}


def page(route, clicks, impressions, position=1.0):
    return row([ORIGIN + route], clicks, impressions, position)


def answer(status, payload):
    return status, json.dumps(payload).encode("utf-8")


def refusal(status, message):
    return answer(status, {"error": {"code": status, "message": message}})


class SearchConsole:
    """The injected HTTP layer: fixture sitemaps and Search Console answers."""

    def __init__(self):
        self.calls = []
        self.files = {}
        self.rows = {"page": [], "page,query": []}
        self.coverage = {}
        self.failures = []
        self.publish([])

    def publish(self, landing, **portal):
        """Landing routes in /sitemap.xml; portal routes in children of the index."""
        self.files = {"/sitemap.xml": sitemap("urlset", "url", [ORIGIN + route for route in landing])}
        children = {f"/sitemaps/portal-{family}-1.xml": routes for family, routes in portal.items()}
        self.files["/sitemap-portal.xml"] = sitemap("sitemapindex", "sitemap",
                                                    [ORIGIN + path for path in children])
        for path, routes in children.items():
            self.files[path] = sitemap("urlset", "url", [ORIGIN + route for route in routes])

    def __call__(self, method, url, headers, body):
        payload = None if body is None else json.loads(body)
        self.calls.append((method, url, dict(headers), payload))
        if url.startswith(ORIGIN + "/"):
            content = self.files.get(url[len(ORIGIN):])
            return (404, b"not found") if content is None else (200, content)
        if self.failures:
            return self.failures.pop(0)
        if url.startswith(SITES) and url.endswith("/searchAnalytics/query"):
            if sum(1 for call in self.calls if call[1] == url) > 20:
                raise AssertionError("runaway paging")  # a paging bug must fail, not hang
            first = payload["startRow"]
            rows = self.rows[",".join(payload["dimensions"])][first:first + payload["rowLimit"]]
            # Like the API, an empty page has no "rows" member at all.
            return answer(200, {"rows": rows, "responseAggregationType": "byPage"} if rows else {})
        if url == INSPECTION:
            verdict, state = self.coverage.get(payload["inspectionUrl"], ("PASS", "Submitted and indexed"))
            return answer(200, {"inspectionResult": {"indexStatusResult": {
                "verdict": verdict, "coverageState": state}}})
        raise AssertionError(f"unexpected request: {method} {url}")

    def posts(self, url):
        return [payload for method, called, _, payload in self.calls if called == url]


class ReportCase(unittest.TestCase):
    def setUp(self):
        self.http = SearchConsole()
        self.sleeps = []
        self.out = Path(self.enterContext(tempfile.TemporaryDirectory())) / "seo"

    def run_report(self, *arguments, expected=0, env=None):
        stdout, stderr = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            code = report.main(["--out", str(self.out), *arguments],
                               env={"GSC_ACCESS_TOKEN": TOKEN} if env is None else env,
                               transport=self.http, sleep=self.sleeps.append, today=date(2026, 11, 21))
        output = stdout.getvalue() + stderr.getvalue()
        self.assertEqual(code, expected, output)
        return output

    def csv(self, name):
        with (self.out / name).open(encoding="utf-8", newline="") as source:
            return list(csv.DictReader(source))

    def markdown(self):
        return (self.out / "report.md").read_text(encoding="utf-8")


class Classifier(unittest.TestCase):
    def test_every_family_of_the_default_locale(self):
        for path, family in DEFAULT_LOCALE_ROUTES.items():
            with self.subTest(path=path):
                self.assertEqual(report.classify(path), ("pt-BR", family))
        for slug in report.FUEL_SLUGS:
            with self.subTest(fuel=slug):
                self.assertEqual(report.classify(f"/precos/{slug}/"), ("pt-BR", "combustivel"))
                self.assertEqual(report.classify(f"/precos/{slug}/go/"), ("pt-BR", "combustivel-estado"))
                self.assertEqual(report.classify(f"/precos/{slug}/go/goiania/"),
                                 ("pt-BR", "combustivel-cidade"))
        self.assertEqual(set(DEFAULT_LOCALE_ROUTES.values()), set(report.FAMILIES))
        self.assertEqual(len(report.FAMILIES), 21)

    def test_locale_prefixes_use_their_own_segment_names(self):
        for path, expected in LOCALE_ROUTES.items():
            with self.subTest(path=path):
                self.assertEqual(report.classify(path), expected)

    def test_shared_route_vectors(self):
        families = {"guide": "guias", "tool": "ferramentas", "page": "institucional",
                    "methodology": "metodologia", "data": "dados", "insights": "curiosidades"}
        vectors = json.loads((ROOT / "contracts/testdata/i18n/routes.json").read_text(encoding="utf-8"))
        self.assertEqual({vector["locale"] for vector in vectors}, {"pt-BR", "en", "es"})
        for vector in vectors:
            with self.subTest(route=vector["route"]):
                if vector["kind"] == "prices":
                    family = {
                        (False, False, False): "precos-hub",
                        (True, False, False): "combustivel",
                        (False, True, False): "estado",
                        (True, True, False): "combustivel-estado",
                        (False, True, True): "cidade",
                        (True, True, True): "combustivel-cidade",
                    }[tuple(bool(vector.get(part)) for part in ("slug", "area", "city"))]
                else:
                    family = families[vector["kind"]]
                self.assertEqual(report.classify(vector["route"]), (vector["locale"], family))

    def test_locales_come_from_the_shared_registry(self):
        registry = json.loads((ROOT / "contracts/i18n/locales.json").read_text(encoding="utf-8"))
        self.assertEqual(report.DEFAULT_LOCALE, registry["default"])
        self.assertEqual(report.LOCALES, tuple(locale["code"] for locale in registry["locales"]))
        self.assertEqual(report.LOCALE_PREFIXES, {"en": "en", "es": "es"})
        for locale in registry["locales"]:
            self.assertEqual(report.LOCALE_FUELS[locale["code"]], tuple(locale["fuelSlugs"].values()))
            for slug in locale["fuelSlugs"].values():
                self.assertFalse(report.UF.fullmatch(slug))

    def test_fuel_slugs_match_the_portal_vocabulary(self):
        source = (ROOT / "backend/internal/modules/enterpriseexport/portalgen/seo_head.go").read_text(
            encoding="utf-8")
        fields = re.findall(r"^\t(\w+)\s+string\b", re.search(
            r"type fuelWords struct \{\n(.*?)\n\}", source, re.S).group(1), re.M)
        entries = re.search(r"var fuelVocabulary = map\[string\]fuelWords\{\n(.*?)\n\}", source,
                            re.S).group(1).splitlines()
        slugs = []
        for entry in entries:
            values = re.fullmatch(r'\t"[A-Z0-9_]+":\s*\{("[^"]*"(?:, "[^"]*")*)\},', entry)
            self.assertIsNotNone(values, f"fuelVocabulary has an entry this test cannot read: {entry}")
            values = re.findall(r'"([^"]*)"', values.group(1))
            self.assertEqual(len(values), len(fields), entry)
            slugs.append(values[fields.index("Slug")])
        self.assertEqual(sorted(slugs), sorted(report.FUEL_SLUGS))
        # A fuel slug shaped like a UF would make /precos/{x}/ ambiguous.
        self.assertFalse([slug for slug in slugs if report.UF.fullmatch(slug)])
        self.assertNotIn(report.PARITY_SLUG, slugs)

    def test_only_urls_of_the_origin_are_classified_by_path(self):
        for url, family in [
            (ORIGIN, "home"),
            (ORIGIN + "/precos/sp/?utm_source=x#topo", "estado"),
            ("https://www.abastevo.com.br/precos/sp/", "outros"),
            ("http://abastevo.com.br/precos/sp/", "outros"),
            ("https://abastevo.com.br.example.invalid/precos/sp/", "outros"),
        ]:
            with self.subTest(url=url):
                self.assertEqual(report.classify_url(url, ORIGIN), ("pt-BR", family))


class Requests(ReportCase):
    def test_requests_follow_the_search_console_shapes(self):
        self.http.publish(["/"], precos=["/precos/sp/"])
        self.run_report()
        period = {"startDate": "2026-10-22", "endDate": "2026-11-18", "rowLimit": 25000, "startRow": 0}
        self.assertEqual([(method, url, payload) for method, url, _, payload in self.http.calls], [
            ("GET", ORIGIN + "/sitemap.xml", None),
            ("GET", ORIGIN + "/sitemap-portal.xml", None),
            ("GET", ORIGIN + "/sitemaps/portal-precos-1.xml", None),
            ("POST", ANALYTICS, {**period, "dimensions": ["page"]}),
            ("POST", ANALYTICS, {**period, "dimensions": ["page", "query"]}),
            ("POST", INSPECTION, {"inspectionUrl": ORIGIN + "/", "siteUrl": SITE}),
            ("POST", INSPECTION, {"inspectionUrl": ORIGIN + "/precos/sp/", "siteUrl": SITE}),
        ])
        for method, url, headers, _ in self.http.calls:
            with self.subTest(url=url):
                # The public edge refuses the default urllib signature.
                self.assertEqual(headers["User-Agent"], report.USER_AGENT)
                self.assertEqual(headers.get("Authorization"),
                                 "Bearer " + TOKEN if method == "POST" else None)

    def test_period_property_and_origin_can_be_chosen(self):
        self.http.files = {"/sitemap.xml": sitemap("urlset", "url", []),
                           "/sitemap-portal.xml": sitemap("urlset", "url", [])}
        self.run_report("--start", "2026-10-01", "--end", "2026-10-07", "--origin", ORIGIN + "/",
                        env={"GSC_ACCESS_TOKEN": TOKEN, "GSC_SITE": "https://abastevo.com.br/"})
        url = SITES + "https%3A%2F%2Fabastevo.com.br%2F/searchAnalytics/query"
        self.assertEqual([call[1] for call in self.http.calls if call[0] == "POST"], [url, url])
        self.assertEqual(self.http.calls[2][3]["startDate"], "2026-10-01")
        self.assertEqual(self.http.calls[2][3]["endDate"], "2026-10-07")
        self.assertIn("- Period: 2026-10-01 to 2026-10-07", self.markdown())

    def test_help_and_bad_arguments_need_no_token_and_no_request(self):
        for arguments, code in [(["--help"], 0), (["--start", "2026-11-02", "--end", "2026-11-01"], 2),
                                (["--origin", "abastevo.com.br"], 2), (["--inspect", "-1"], 2),
                                (["--end", "18/11/2026"], 2)]:
            with self.subTest(arguments=arguments):
                with self.assertRaises(SystemExit) as stopped, \
                        contextlib.redirect_stdout(io.StringIO()) as stdout, \
                        contextlib.redirect_stderr(io.StringIO()):
                    report.main(arguments, env={}, transport=self.http)
                self.assertEqual(stopped.exception.code, code)
                self.assertEqual("GSC_ACCESS_TOKEN" in stdout.getvalue(), code == 0)
        self.assertEqual(self.http.calls, [])

    def test_sitemap_index_is_followed_one_level(self):
        self.http.publish(["/", "/guias/"], precos=["/precos/sp/", "/precos/rj/"], postos=[STATION])
        self.http.files["/sitemap.xml"] = sitemap(
            "urlset", "url", [ORIGIN + "/", ORIGIN + "/guias/", "https://other.invalid/"])
        output = self.run_report("--inspect", "0")
        self.assertIn("sitemap_urls=5 ", output)
        self.assertIn("- URLs in the sitemaps: 5\n- Sitemap entries outside the origin, ignored: 1\n",
                      self.markdown())
        self.assertIn("| estado | 2 | 0 | 0 | 0 | 0.00% | - |", self.markdown())
        self.assertIn("| posto | 1 | 0 | 0 | 0 | 0.00% | - |", self.markdown())
        self.assertIn("| Total | 5 | 0 | 0 | 0 | 0.00% | - |", self.markdown())
        self.assertIn("### en\n\nNo URL in the sitemaps and no impressions.", self.markdown())

        self.http.files["/sitemaps/portal-precos-1.xml"] = sitemap(
            "sitemapindex", "sitemap", [ORIGIN + "/deeper.xml"])
        self.assertIn("sitemap index inside a sitemap index", self.run_report(expected=1))
        self.assertNotIn(ORIGIN + "/deeper.xml", [call[1] for call in self.http.calls])
        self.http.files["/sitemaps/portal-precos-1.xml"] = b"<urlset"
        self.assertIn("not valid sitemap XML", self.run_report(expected=1))

    def test_localized_portal_sitemaps_are_counted_by_locale_and_family(self):
        self.http.publish(["/", "/en/", "/es/"], **{
            "precos-en": ["/en/fuel-prices/pr/", "/en/fuel-prices/regular-gasoline/pr/"],
            "precos-es": ["/es/precios/pr/foz-do-iguacu/"],
            "hubs-es": ["/es/metodologia/"],
        })
        self.assertIn("sitemap_urls=7 ", self.run_report("--inspect", "0"))
        text = self.markdown()
        english, spanish = text.split("### en\n", 1)[1].split("### es\n", 1)
        self.assertIn("| estado | 1 |", english)
        self.assertIn("| combustivel-estado | 1 |", english)
        self.assertIn("| cidade | 1 |", spanish)
        self.assertIn("| metodologia | 1 |", spanish)

    def test_pagination_stops_at_a_short_page(self):
        self.http.rows["page"] = [page(f"/precos/sp/cidade-{index:05}/", 0, 1) for index in range(25_003)]
        self.http.rows["page,query"] = [row([ORIGIN + "/precos/sp/", f"consulta {index:05}"], 0, 1)
                                        for index in range(25_000)]
        output = self.run_report("--inspect", "0")
        self.assertEqual([(payload["dimensions"], payload["startRow"], payload["rowLimit"])
                          for payload in self.http.posts(ANALYTICS)],
                         [(["page"], 0, 25000), (["page"], 25000, 25000),
                          (["page", "query"], 0, 25000), (["page", "query"], 25000, 25000)])
        self.assertIn("pages=25003 query_rows=25000", output)
        pages = self.csv("pages.csv")
        self.assertEqual(len(pages), 25_003)
        self.assertEqual(len({line["url"] for line in pages}), 25_003)
        self.assertEqual(len(self.csv("queries.csv")), 25_000)


class Aggregates(ReportCase):
    def test_position_is_weighted_by_impressions(self):
        rows = [{"impressions": 300, "position": 2.0}, {"impressions": 100, "position": 10.0}]
        self.assertEqual(report.weighted_position(rows), 4.0)
        self.assertIsNone(report.weighted_position([]))
        self.assertIsNone(report.weighted_position([{"impressions": 0, "position": 7.0}]))

        self.http.publish([], precos=["/precos/sp/", "/precos/rj/", "/precos/mg/"])
        self.http.rows["page"] = [page("/precos/sp/", 30, 300, 2.0), page("/precos/rj/", 10, 100, 10.0),
                                  page("/en/guides/ethanol-or-gasoline/", 1, 10, 3.0)]
        self.run_report("--inspect", "0")
        # The plain mean of the two positions would be 6.0.
        self.assertIn("| estado | 3 | 2 | 40 | 400 | 10.00% | 4.0 |", self.markdown())
        self.assertIn("| guias | 0 | 1 | 1 | 10 | 10.00% | 3.0 |", self.markdown().split("### en")[1])
        self.assertEqual(self.csv("pages.csv")[0], {
            "url": ORIGIN + "/precos/sp/", "locale": "pt-BR", "family": "estado", "in_sitemap": "yes",
            "clicks": "30", "impressions": "300", "ctr": "0.1", "position": "2.0"})
        self.assertEqual([(line["locale"], line["family"], line["in_sitemap"])
                          for line in self.csv("pages.csv")],
                         [("pt-BR", "estado", "yes"), ("pt-BR", "estado", "yes"), ("en", "guias", "no")])

    def test_pages_below_the_median_of_their_family(self):
        self.http.rows["page"] = [
            page("/precos/sp/campinas/", 10, 100),     # 10%
            page("/precos/sp/santos/", 10, 200),       # 5%, the family median
            page("/precos/sp/sorocaba/", 8, 400),      # 2%: below
            # Under 50 impressions: never listed, and left out of the median,
            # which they would otherwise pull down to zero.
            *[page(f"/precos/sp/pequena-{index}/", 0, 49) for index in range(5)],
            page("/precos/sp/", 1, 500),               # alone in its family
            page("/guias/pesquisa-anp/", 6, 60),       # 10%
            page("/guias/como-ler-precos/", 1, 50),    # 2%: below, at exactly 50
        ]
        self.run_report("--inspect", "0")
        section = self.markdown().split("## Pages below their family's median CTR")[1]
        listed = [line.split(" | ")[2] for line in section.splitlines() if line.startswith("| pt-BR")]
        self.assertEqual(listed, [ORIGIN + "/precos/sp/sorocaba/", ORIGIN + "/guias/como-ler-precos/"])
        self.assertIn("| 8 | 400 | 2.00% | 1.0 | 5.00% |", section)
        self.assertIn("| 1 | 50 | 2.00% | 1.0 | 6.00% |", section)

        self.http.rows["page"] = [page("/precos/sp/", 1, 500)]
        self.run_report("--inspect", "0")
        self.assertIn("No page is below its family's median.", self.markdown())

    def test_top_queries_are_summed_over_the_pages_of_a_family(self):
        self.http.rows["page,query"] = [
            row([ORIGIN + "/precos/sp/campinas/", "preço gasolina"], 3, 300, 2.0),
            row([ORIGIN + "/precos/sp/santos/", "preço gasolina"], 1, 100, 10.0),
            row([ORIGIN + "/precos/sp/", "preço gasolina"], 9, 90, 1.0),
            row([ORIGIN + "/precos/sp/santos/", "a|b"], 0, 5, 1.0),
            *[row([ORIGIN + "/precos/sp/santos/", f"consulta {index:02}"], 100 - index, 1000)
              for index in range(25)],
        ]
        self.run_report("--inspect", "0")
        sections = self.markdown().split("## Top 20 queries by family")[1].split("## Pages below")[0]
        city = sections.split("### pt-BR / cidade")[1]
        listed = [line.split(" | ")[0][2:] for line in city.splitlines() if line.startswith("| ")][2:]
        self.assertEqual(listed, [f"consulta {index:02}" for index in range(20)])
        totals = dict(report.top_queries([report.record(line, ("url", "query"), ORIGIN)
                                          for line in self.http.rows["page,query"][:4]]))
        self.assertEqual(totals["pt-BR", "cidade"], [
            ("preço gasolina", {"clicks": 4, "impressions": 400, "ctr": 0.01, "position": 4.0}),
            ("a|b", {"clicks": 0, "impressions": 5, "ctr": 0.0, "position": 1.0}),
        ])
        self.assertEqual(totals["pt-BR", "estado"][0][1]["clicks"], 9)

    def test_query_text_cannot_break_the_table_or_run_as_a_formula(self):
        self.http.rows["page,query"] = [row([ORIGIN + "/", "a|b\nc"], 2, 10),
                                        row([ORIGIN + "/", "<b>[x](y) `z` *w* _v_"], 1, 10),
                                        row([ORIGIN + "/", "=HYPERLINK(1)"], 0, 10),
                                        row([ORIGIN + "/", "@abastevo"], 0, 9)]
        self.run_report("--inspect", "0")
        self.assertIn(r"| a\|b c | 2 | 10 | 20.00% | 1.0 |", self.markdown())
        self.assertIn(r"| \<b\>\[x\](y) \`z\` \*w\* \_v\_ | 1 | 10 | 10.00% | 1.0 |", self.markdown())
        self.assertEqual([line["query"] for line in self.csv("queries.csv")],
                         ["a|b\nc", "<b>[x](y) `z` *w* _v_", "'=HYPERLINK(1)", "'@abastevo"])


class Inspection(ReportCase):
    GROUPS = {
        "cidade": "/precos/sp/cidade-{:03}/",
        "combustivel-cidade": "/precos/etanol/sp/cidade-{:03}/",
        "posto": "/postos/00000000-0000-4000-8000-000000000{:03}/",
        "forum": "/forum/precos/assunto-{:03}/",
        "guias": "/guias/guia-{:03}/",
        "como-usar": "/como-usar/passo-{:03}/",
        "glossario": "/glossario/termo-{:03}/",
        "fiscalizacao": "/fiscalizacao/{:03}/",
    }

    def publish_groups(self):
        self.http.publish(["/"], todos=[shape.format(index) for shape in self.GROUPS.values()
                                        for index in range(100)])

    def test_default_takes_twenty_urls_spread_over_each_family(self):
        self.publish_groups()
        self.run_report()
        inspected = self.csv("inspection.csv")
        self.assertEqual(len(self.http.posts(INSPECTION)), 161)
        self.assertEqual({family: sum(1 for line in inspected if line["family"] == family)
                          for family in ["home", *self.GROUPS]},
                         {"home": 1, **dict.fromkeys(self.GROUPS, 20)})
        self.assertEqual([line["url"] for line in inspected if line["family"] == "cidade"],
                         [ORIGIN + f"/precos/sp/cidade-{index:03}/" for index in range(0, 100, 5)])

    def test_total_is_capped_at_five_hundred(self):
        self.publish_groups()
        output = self.run_report("--inspect", "100")
        self.assertIn("inspected=500 ", output)
        payloads = self.http.posts(INSPECTION)
        self.assertEqual(len(payloads), 500)
        self.assertEqual(len({payload["inspectionUrl"] for payload in payloads}), 500)
        counts = [sum(1 for line in self.csv("inspection.csv") if line["family"] == family)
                  for family in ["home", *self.GROUPS]]
        self.assertEqual(counts[0], 1)
        self.assertEqual(sorted(set(counts[1:])), [62, 63])
        self.assertEqual(sum(counts), 500)

    def test_plan_is_bounded_by_the_group_the_quota_and_the_cap(self):
        families = {f"{ORIGIN}/precos/sp/cidade-{index}/": ("pt-BR", "cidade") for index in range(9)}
        families |= {f"{ORIGIN}/en/guides/guide-{index}/": ("en", "guias") for index in range(3)}
        self.assertEqual(report.inspection_plan(families, 0), [])
        self.assertEqual(len(report.inspection_plan(families, 2)), 4)
        self.assertEqual(sorted(report.inspection_plan(families, 10**9)), sorted(families))
        self.assertEqual(len(report.inspection_plan(families, 9, cap=7)), 7)
        self.assertEqual(sum("/en/" in url for url in report.inspection_plan(families, 9, cap=4)), 2)
        self.assertEqual(report.INSPECT_CAP, 500)

    def test_states_are_counted_per_family(self):
        self.http.publish(["/"], precos=["/precos/sp/", "/precos/rj/", "/precos/mg/", "/precos/mt/sorriso/"])
        self.http.coverage = {
            ORIGIN + "/precos/rj/": ("NEUTRAL", "Crawled - currently not indexed"),
            ORIGIN + "/precos/mg/": ("NEUTRAL", "Crawled - currently not indexed"),
            ORIGIN + "/precos/mt/sorriso/": (None, None),
        }
        self.run_report()
        section = self.markdown().split("## Inspected URLs by coverage state")[1].split("## Top")[0]
        self.assertEqual([line for line in section.splitlines() if line.startswith("| pt-BR")], [
            "| pt-BR | home | Submitted and indexed | 1 |",
            "| pt-BR | estado | Crawled - currently not indexed | 2 |",
            "| pt-BR | estado | Submitted and indexed | 1 |",
            "| pt-BR | cidade | unknown | 1 |",
        ])
        self.assertIn({"url": ORIGIN + "/precos/rj/", "locale": "pt-BR", "family": "estado",
                       "verdict": "NEUTRAL", "coverage_state": "Crawled - currently not indexed"},
                      self.csv("inspection.csv"))

        self.run_report("--inspect", "0")
        self.assertIn("No URL was inspected.", self.markdown())
        self.assertEqual(self.csv("inspection.csv"), [])


class Failures(ReportCase):
    def test_forbidden_answer_prints_the_api_message_and_writes_nothing(self):
        self.http.failures = [refusal(403, "User does not have sufficient permission for site "
                                           "'sc-domain:abastevo.com.br'.")]
        output = self.run_report(expected=1)
        self.assertEqual(output, f"search-report: HTTP 403 from {ANALYTICS}: User does not have "
                                 "sufficient permission for site 'sc-domain:abastevo.com.br'.\n")
        self.assertEqual(len(self.http.posts(ANALYTICS)), 1)
        self.assertEqual(self.sleeps, [])
        self.assertFalse(self.out.exists())

    def test_one_retry_after_five_seconds_on_429_and_5xx(self):
        for status in (429, 500, 503):
            with self.subTest(status=status):
                self.http.calls.clear()
                self.sleeps.clear()
                self.http.failures = [refusal(status, "try again")]
                self.run_report("--inspect", "0")
                self.assertEqual(self.sleeps, [5])
                self.assertEqual(len(self.http.posts(ANALYTICS)), 3)

        self.http.calls.clear()
        self.sleeps.clear()
        self.http.failures = [refusal(503, "backend error"), refusal(503, "backend error")]
        self.assertIn(f"HTTP 503 from {ANALYTICS}: backend error", self.run_report(expected=1))
        self.assertEqual(self.sleeps, [5])
        self.assertEqual(len(self.http.posts(ANALYTICS)), 2)

    def test_other_failures_are_not_retried(self):
        del self.http.files["/sitemap-portal.xml"]
        self.assertEqual(self.run_report(expected=1),
                         f"search-report: HTTP 404 from {ORIGIN}/sitemap-portal.xml: not found\n")
        self.assertEqual(self.sleeps, [])
        self.assertEqual(self.http.posts(ANALYTICS), [])

        def unreachable(*_args):
            raise OSError("connection refused")
        self.http = unreachable
        self.assertEqual(self.run_report(expected=1),
                         f"search-report: request to {ORIGIN}/sitemap.xml failed: connection refused\n")
        self.assertEqual(self.sleeps, [])

    def test_malformed_answers_fail_in_one_line(self):
        self.http.failures = [(200, b"<html>not json</html>")]
        self.assertIn("did not answer with a JSON object", self.run_report(expected=1))
        self.http.failures = [answer(200, {"rows": [{"keys": [ORIGIN + "/"], "clicks": 1}]})]
        self.assertIn("row without the expected fields", self.run_report(expected=1))
        self.assertFalse(self.out.exists())


class Token(ReportCase):
    def test_token_is_in_no_output_file_and_no_message(self):
        self.http.publish(["/", "/guias/"], precos=["/precos/sp/", "/precos/mt/sorriso/"], postos=[STATION])
        self.http.rows["page"] = [page("/precos/sp/", 30, 300, 2.0), page("/precos/mt/sorriso/", 1, 80, 9.0)]
        self.http.rows["page,query"] = [row([ORIGIN + "/precos/sp/", "preço gasolina sp"], 12, 120, 2.5)]
        output = self.run_report()
        self.assertEqual(sorted(path.name for path in self.out.iterdir()),
                         ["inspection.csv", "pages.csv", "queries.csv", "report.md"])
        for path in self.out.iterdir():
            with self.subTest(file=path.name):
                self.assertGreater(path.stat().st_size, 0)
                self.assertNotIn(TOKEN.encode(), path.read_bytes())
        self.assertNotIn(TOKEN, output)
        # The token goes to the two Google hosts only, never to the origin.
        sent = {url.split("/")[2] for _, url, headers, _ in self.http.calls if TOKEN in json.dumps(headers)}
        self.assertEqual(sent, {"www.googleapis.com", "searchconsole.googleapis.com"})

    def test_token_echoed_by_an_error_is_redacted(self):
        self.http.failures = [refusal(401, f"Invalid credentials: Bearer {TOKEN}")]
        output = self.run_report(expected=1)
        self.assertEqual(output, f"search-report: HTTP 401 from {ANALYTICS}: "
                                 "Invalid credentials: Bearer [redacted]\n")

    def test_missing_or_malformed_token_makes_no_request(self):
        self.assertIn("GSC_ACCESS_TOKEN is not set", self.run_report(expected=1, env={}))
        self.assertIn("GSC_ACCESS_TOKEN is not set",
                      self.run_report(expected=1, env={"GSC_ACCESS_TOKEN": " \n"}))
        for token in ("first second", "line\nbreak", "acentuação"):
            with self.subTest(token=token):
                output = self.run_report(expected=1, env={"GSC_ACCESS_TOKEN": token})
                self.assertIn("GSC_ACCESS_TOKEN must be one token", output)
                self.assertNotIn(token, output)
                self.assertNotIn(token.split()[0], output)
        self.assertEqual(self.http.calls, [])
        self.assertFalse(self.out.exists())


class Transport(unittest.TestCase):
    """The real urllib transport, against a loopback server only."""

    def setUp(self):
        seen = self.seen = []

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def reply(self):
                agent = self.headers.get("User-Agent", "")
                seen.append((self.command, self.path, agent, self.headers.get("Authorization")))
                self.rfile.read(int(self.headers.get("Content-Length", 0)))
                status, body = 200, b'{"rows": []}'
                if agent.startswith("Python-urllib"):
                    status, body = 403, b"refused"
                elif self.path == "/moved":
                    status, body = 302, b""
                elif self.path == "/denied":
                    status, body = 403, b'{"error": {"message": "denied"}}'
                self.send_response(status)
                if status == 302:
                    self.send_header("Location", "/target")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            do_GET = do_POST = reply

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01})
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(thread.join)
        self.addCleanup(server.shutdown)
        self.origin = f"http://127.0.0.1:{server.server_port}"
        self.enterContext(mock.patch.dict(os.environ, {"NO_PROXY": "127.0.0.1", "no_proxy": "127.0.0.1"}))

    def test_names_the_client_and_does_not_carry_the_token_over_a_redirect(self):
        client = report.Client(report.urllib_transport, self.fail, TOKEN)
        self.assertEqual(client.get(self.origin + "/sitemap.xml"), b'{"rows": []}')
        self.assertEqual(client.post(self.origin + "/query", {"startRow": 0}), {"rows": []})
        with self.assertRaisesRegex(report.ReportError, "^HTTP 302 from http://127.0.0.1:[0-9]+/moved: "):
            client.post(self.origin + "/moved", {})
        with self.assertRaisesRegex(report.ReportError,
                                    "^HTTP 403 from http://127.0.0.1:[0-9]+/denied: denied$"):
            client.post(self.origin + "/denied", {})
        self.assertEqual(self.seen, [
            ("GET", "/sitemap.xml", report.USER_AGENT, None),
            ("POST", "/query", report.USER_AGENT, "Bearer " + TOKEN),
            ("POST", "/moved", report.USER_AGENT, "Bearer " + TOKEN),
            ("POST", "/denied", report.USER_AGENT, "Bearer " + TOKEN),
        ])


if __name__ == "__main__":
    unittest.main()
