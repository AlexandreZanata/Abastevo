#!/usr/bin/env python3
"""Search Console report by locale and page family.

A read-only tool for the owner; it never runs in CI or on the VPS. It reads
Search Analytics and URL Inspection with the access token in GSC_ACCESS_TOKEN,
reads the public sitemaps of the origin and writes report.md, pages.csv,
queries.csv and inspection.csv. See README.md in this directory.
"""
import argparse
from collections import Counter, defaultdict
import csv
from datetime import date, datetime, timedelta
from http.client import HTTPException
import json
import os
from pathlib import Path
import re
import statistics
import sys
import time
from urllib.error import HTTPError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener
import xml.etree.ElementTree as XML


ROOT = Path(__file__).resolve().parents[2]
LOCALES_FILE = ROOT / "contracts/i18n/locales.json"

DEFAULT_SITE = "sc-domain:abastevo.com.br"
DEFAULT_ORIGIN = "https://abastevo.com.br"
DEFAULT_OUT = ".local/seo/"
SITEMAPS = ("/sitemap.xml", "/sitemap-portal.xml")
ANALYTICS_URL = "https://www.googleapis.com/webmasters/v3/sites/{site}/searchAnalytics/query"
INSPECTION_URL = "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"
# The public edge answers 403 to the default Python-urllib signature, so every
# request names this client instead.
USER_AGENT = "abastevo-search-report/1 (+https://abastevo.com.br/)"
ROW_LIMIT = 25_000
INSPECT_CAP = 500
RETRY_SECONDS = 5
TIMEOUT_SECONDS = 60
MIN_IMPRESSIONS = 50
TOP_QUERIES = 20

FAMILIES = (
    "home", "guias", "ferramentas", "como-usar", "glossario", "institucional",
    "precos-hub", "combustivel", "estado", "cidade", "combustivel-estado",
    "combustivel-cidade", "paridade", "posto", "forum", "fiscalizacao", "dados",
    "consultar", "metodologia", "curiosidades", "outros",
)
# The Slug values of fuelVocabulary in
# backend/internal/modules/enterpriseexport/portalgen/seo_head.go. The test
# reads that file and fails when the two lists differ.
FUEL_SLUGS = (
    "gasolina-comum", "gasolina-aditivada", "gasolina-premium", "etanol",
    "diesel-s10", "diesel-s500", "gnv", "glp",
)
PARITY_SLUG = "etanol-ou-gasolina"
# First path segment to family, for routes whose family does not depend on the
# rest of the path. Guides, tools and /precos/ are decided in classify().
SECTIONS = {
    "como-usar": "como-usar",
    "glossario": "glossario",
    "sobre": "institucional",
    "politica-editorial": "institucional",
    "privacidade": "institucional",
    "termos": "institucional",
    "excluir-conta": "institucional",
    "contribuidores": "institucional",
    "postos": "posto",
    "forum": "forum",
    "fiscalizacao": "fiscalizacao",
    "dados": "dados",
    "consultar": "consultar",
    "metodologia": "metodologia",
    "curiosidades": "curiosidades",
}
UF = re.compile(r"[a-z]{2}")
SLUG = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*")


class ReportError(Exception):
    """A failure the owner can act on: printed as one line, never a traceback."""


def load_locales(path=LOCALES_FILE):
    """The default locale, each locale's segment names and the URL prefixes."""
    registry = json.loads(path.read_text(encoding="utf-8"))
    segments = {locale["code"]: locale["segments"] for locale in registry["locales"]}
    prefixes = {locale["prefix"].strip("/"): locale["code"]
                for locale in registry["locales"] if locale["prefix"]}
    return registry["default"], segments, prefixes


DEFAULT_LOCALE, LOCALE_SEGMENTS, LOCALE_PREFIXES = load_locales()
LOCALES = tuple(LOCALE_SEGMENTS)


def price_family(rest):
    """Family of a /precos/ route, from the segments after `precos`."""
    if not rest:
        return "precos-hub"
    if rest == [PARITY_SLUG]:
        return "paridade"
    fuel = rest[0] in FUEL_SLUGS
    place = rest[1:] if fuel else rest
    if (len(place) > 2 or (place and not UF.fullmatch(place[0]))
            or (len(place) == 2 and not SLUG.fullmatch(place[1]))):
        return "outros"
    if fuel:
        return ("combustivel", "combustivel-estado", "combustivel-cidade")[len(place)]
    return ("estado", "cidade")[len(place) - 1]


def classify(path):
    """Return (locale, family) for a URL path of the public origin."""
    segments = [part for part in path.split("/") if part]
    locale = LOCALE_PREFIXES.get(segments[0], DEFAULT_LOCALE) if segments else DEFAULT_LOCALE
    translated = locale != DEFAULT_LOCALE
    if translated:
        segments = segments[1:]
    if not segments:
        return locale, "home"
    head = segments[0]
    names = LOCALE_SEGMENTS[locale]
    if head == names["guias"]:
        family = "guias"
    elif head == names["ferramentas"]:
        family = "ferramentas"
    elif head == "precos":
        family = price_family(segments[1:])
    else:
        family = SECTIONS.get(head, "outros")
    # The registry has no translated page slugs: under a locale prefix, a
    # single slug is its `page` kind ({prefix}/{slug}/, such as /en/about/).
    if translated and family == "outros" and len(segments) == 1 and SLUG.fullmatch(head):
        family = "institucional"
    return locale, family


def classify_url(url, origin):
    """classify() for a URL of the origin; any other host or scheme is `outros`."""
    if url == origin or url.startswith(origin + "/"):
        return classify(urlsplit(url).path)
    return DEFAULT_LOCALE, "outros"


def group_order(key):
    locale, family = key
    return LOCALES.index(locale), FAMILIES.index(family)


class NoRedirect(HTTPRedirectHandler):
    """urllib would repeat the Authorization header to a redirect target."""

    def redirect_request(self, *_args, **_kwargs):
        return None


def urllib_transport(method, url, headers, body):
    """Send one request; return (status, body) for any HTTP answer."""
    request = Request(url, data=body, headers=headers, method=method)
    try:
        with build_opener(NoRedirect).open(request, timeout=TIMEOUT_SECONDS) as response:
            return response.status, response.read()
    except HTTPError as error:
        with error:
            return error.code, error.read()


def api_message(content):
    """The message of a Google API error body, or the start of any other body."""
    try:
        return str(json.loads(content)["error"]["message"])
    except (ValueError, KeyError, TypeError):
        text = " ".join(content.decode("utf-8", "replace").split())
        return text[:200] or "no message"


class Client:
    """The transport plus the retry and error rules. Only post() sends the token."""

    def __init__(self, transport, sleep, token):
        self.transport = transport
        self.sleep = sleep
        self.token = token

    def send(self, method, url, headers, body):
        headers = {"User-Agent": USER_AGENT, **headers}
        for attempt in (1, 2):
            try:
                status, content = self.transport(method, url, headers, body)
            except (HTTPException, OSError) as error:
                raise ReportError(f"request to {url} failed: {error}") from None
            if attempt == 2 or (status != 429 and status < 500):
                break
            self.sleep(RETRY_SECONDS)
        if status != 200:
            raise ReportError(f"HTTP {status} from {url}: {api_message(content)}")
        return content

    def get(self, url):
        """Body of a public file, requested without the token."""
        return self.send("GET", url, {}, None)

    def post(self, url, payload):
        """Decoded answer of a Google API call, requested with the token."""
        headers = {"Authorization": "Bearer " + self.token, "Content-Type": "application/json"}
        content = self.send("POST", url, headers, json.dumps(payload).encode("utf-8"))
        try:
            answer = json.loads(content)
        except ValueError:
            answer = None
        if not isinstance(answer, dict):
            raise ReportError(f"{url} did not answer with a JSON object")
        return answer


def read_sitemaps(client, origin):
    """URLs of the origin in its sitemaps, and how many other entries were ignored."""
    urls = set()
    ignored = 0

    def read(url, nested=False):
        nonlocal ignored
        try:
            root = XML.fromstring(client.get(url))
        except XML.ParseError as error:
            raise ReportError(f"{url} is not valid sitemap XML: {error}") from None
        namespace, _, kind = root.tag.rpartition("}")
        prefix = namespace + "}" if namespace else ""
        if kind not in ("urlset", "sitemapindex"):
            raise ReportError(f"{url} is not a urlset or a sitemapindex")
        if kind == "sitemapindex" and nested:
            raise ReportError(f"{url} is a sitemap index inside a sitemap index")
        for entry in root.findall(prefix + ("url" if kind == "urlset" else "sitemap")):
            loc = entry.findtext(prefix + "loc", "").strip()
            if not loc.startswith(origin + "/"):
                ignored += 1
            elif kind == "sitemapindex":
                read(loc, nested=True)
            else:
                urls.add(loc)

    for path in SITEMAPS:
        read(origin + path)
    return urls, ignored


def search_analytics(client, site, start, end, dimensions):
    """Every row of one Search Analytics query, paged with startRow."""
    url = ANALYTICS_URL.format(site=quote(site, safe=""))
    rows = []
    while True:
        batch = client.post(url, {
            "startDate": start.isoformat(),
            "endDate": end.isoformat(),
            "dimensions": dimensions,
            "rowLimit": ROW_LIMIT,
            "startRow": len(rows),
        }).get("rows", [])
        rows.extend(batch)
        if len(batch) < ROW_LIMIT:
            return rows


def record(row, names, origin):
    """One API row as a flat record with the locale and family of its page."""
    try:
        keys = row["keys"]
        if len(keys) != len(names):
            raise KeyError("keys")
        locale, family = classify_url(keys[0], origin)
        return {**dict(zip(names, keys)), "locale": locale, "family": family,
                "clicks": row["clicks"], "impressions": row["impressions"],
                "ctr": row["ctr"], "position": row["position"]}
    except (KeyError, TypeError, AttributeError):
        raise ReportError("Search Analytics returned a row without the expected fields") from None


def inspect(client, site, url):
    """(verdict, coverageState) of one URL from URL Inspection."""
    answer = client.post(INSPECTION_URL, {"inspectionUrl": url, "siteUrl": site})
    try:
        status = answer["inspectionResult"]["indexStatusResult"]
        return status.get("verdict") or "unknown", status.get("coverageState") or "unknown"
    except (KeyError, TypeError, AttributeError):
        return "unknown", "unknown"


def inspection_plan(families, per_family, cap=INSPECT_CAP):
    """Sitemap URLs to inspect: up to per_family for each locale and family and
    at most cap in total, the quota shrinking evenly when the cap binds. Each
    group is sampled at even steps of its sorted URLs, so one state or letter
    does not fill the sample."""
    groups = defaultdict(list)
    for url in sorted(families):
        groups[families[url]].append(url)
    order = sorted(groups, key=group_order)
    quotas = dict.fromkeys(order, 0)
    budget = cap
    for _ in range(per_family):
        granted = 0
        for key in order:
            if budget > granted and quotas[key] < len(groups[key]):
                quotas[key] += 1
                granted += 1
        budget -= granted
        if not granted:
            break
    return [groups[key][index * len(groups[key]) // quotas[key]]
            for key in order for index in range(quotas[key])]


def weighted_position(rows):
    """Average position weighted by impressions; None without impressions."""
    impressions = sum(row["impressions"] for row in rows)
    if not impressions:
        return None
    return sum(row["position"] * row["impressions"] for row in rows) / impressions


def totals(rows):
    clicks = sum(row["clicks"] for row in rows)
    impressions = sum(row["impressions"] for row in rows)
    return {"clicks": clicks, "impressions": impressions,
            "ctr": clicks / impressions if impressions else 0.0,
            "position": weighted_position(rows)}


def below_family_median(pages):
    """Pages with at least 50 impressions and a CTR below their family's median.

    The median is taken over the pages of the same locale and family that also
    have at least 50 impressions: pages under that floor are mostly at a CTR of
    zero and would pull every median down to zero. Returns (page, median) pairs.
    """
    groups = defaultdict(list)
    for page in pages:
        if page["impressions"] >= MIN_IMPRESSIONS:
            groups[page["locale"], page["family"]].append(page)
    weak = []
    for group in groups.values():
        median = statistics.median(page["clicks"] / page["impressions"] for page in group)
        weak += [(page, median) for page in group
                 if page["clicks"] / page["impressions"] < median]
    return sorted(weak, key=lambda item: (-item[0]["impressions"], item[0]["url"]))


def top_queries(queries):
    """For each locale and family, its 20 queries with the most clicks."""
    grouped = defaultdict(lambda: defaultdict(list))
    for row in queries:
        grouped[row["locale"], row["family"]][row["query"]].append(row)
    return {key: sorted(((query, totals(rows)) for query, rows in by_query.items()),
                        key=lambda item: (-item[1]["clicks"], -item[1]["impressions"], item[0]))[:TOP_QUERIES]
            for key, by_query in grouped.items()}


def whole(value):
    return f"{value:,.0f}"


def percent(value):
    return f"{value * 100:.2f}%"


def decimal(value):
    return "-" if value is None else f"{value:.1f}"


def measures(summary):
    return [whole(summary["clicks"]), whole(summary["impressions"]),
            percent(summary["ctr"]), decimal(summary["position"])]


def table(header, rows, text=1):
    """Markdown table lines; the first `text` columns are text, the rest numbers.

    Search queries are typed by strangers, so every cell is one line with its
    Markdown and HTML punctuation escaped.
    """
    def line(cells):
        return "| " + " | ".join(re.sub(r"([\\`*_\[\]<>|])", r"\\\1", " ".join(str(cell).split()))
                                 for cell in cells) + " |"
    rule = "| " + " | ".join("---" if index < text else "---:" for index in range(len(header))) + " |"
    return [line(header), rule, *map(line, rows), ""]


def render(options, site, families, ignored, pages, queries, inspections):
    """The text of report.md."""
    lines = [
        "# Search Console report",
        "",
        f"- Property: `{site}`",
        f"- Origin: {options.origin}",
        f"- Period: {options.start.isoformat()} to {options.end.isoformat()}",
        f"- URLs in the sitemaps: {len(families):,}",
        f"- Sitemap entries outside the origin, ignored: {ignored:,}",
        f"- Pages with impressions: {sum(1 for page in pages if page['impressions'] > 0):,}",
        f"- Inspected URLs: {len(inspections):,}",
        "",
        "Position is the average position weighted by impressions. Query totals leave",
        "out the queries Search Console hides for privacy.",
        "",
        "## Families by locale",
        "",
    ]
    submitted = Counter(families.values())
    by_group = defaultdict(list)
    for page in pages:
        by_group[page["locale"], page["family"]].append(page)
    for locale in LOCALES:
        lines += [f"### {locale}", ""]
        keys = [(locale, family) for family in FAMILIES]
        if not any(submitted[key] or by_group[key] for key in keys):
            lines += ["No URL in the sitemaps and no impressions.", ""]
            continue
        rows = [[family, f"{submitted[locale, family]:,}",
                 f"{sum(1 for page in by_group[locale, family] if page['impressions'] > 0):,}",
                 *measures(totals(by_group[locale, family]))] for _, family in keys]
        every = [page for key in keys for page in by_group[key]]
        rows.append(["Total", f"{sum(submitted[key] for key in keys):,}",
                     f"{sum(1 for page in every if page['impressions'] > 0):,}", *measures(totals(every))])
        lines += table(["Family", "URLs in sitemaps", "URLs with impressions", "Clicks",
                        "Impressions", "CTR", "Position"], rows)

    lines += ["## Inspected URLs by coverage state", ""]
    if inspections:
        states = Counter((row["locale"], row["family"], row["coverage_state"]) for row in inspections)
        ordered = sorted(states, key=lambda key: (group_order(key[:2]), -states[key], key[2]))
        lines += table(["Locale", "Family", "Coverage state", "URLs"],
                       [[*key, f"{states[key]:,}"] for key in ordered], text=3)
    else:
        lines += ["No URL was inspected.", ""]

    lines += [f"## Top {TOP_QUERIES} queries by family", ""]
    top = top_queries(queries)
    for key in sorted(top, key=group_order):
        lines += [f"### {key[0]} / {key[1]}", ""]
        lines += table(["Query", "Clicks", "Impressions", "CTR", "Position"],
                       [[query, *measures(summary)] for query, summary in top[key]])
    if not top:
        lines += ["No query rows in the period.", ""]

    lines += [
        "## Pages below their family's median CTR",
        "",
        f"Pages with at least {MIN_IMPRESSIONS} impressions whose CTR is below the median CTR of the",
        f"pages of the same locale and family that also have at least {MIN_IMPRESSIONS} impressions.",
        "",
    ]
    weak = below_family_median(pages)
    if weak:
        lines += table(
            ["Locale", "Family", "URL", "Clicks", "Impressions", "CTR", "Position", "Family median CTR"],
            [[page["locale"], page["family"], page["url"], *measures(totals([page])), percent(median)]
             for page, median in weak], text=3)
    else:
        lines += ["No page is below its family's median.", ""]
    return "\n".join(lines)


def spreadsheet_safe(text):
    """Search queries are typed by strangers: keep a spreadsheet from running one
    as a formula, with the apostrophe the portal CSV downloads also use."""
    return "'" + text if text[:1] in ("=", "+", "-", "@", "\t", "\r") else text


def write_csv(path, columns, rows):
    with path.open("w", encoding="utf-8", newline="") as output:
        writer = csv.DictWriter(output, fieldnames=columns, lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def run(options, site, client):
    """Read everything, then write the four files. Returns the summary line."""
    origin = options.origin
    urls, ignored = read_sitemaps(client, origin)
    families = {url: classify_url(url, origin) for url in urls}
    period = (options.start, options.end)
    pages = [record(row, ("url",), origin)
             for row in search_analytics(client, site, *period, ["page"])]
    queries = [record(row, ("url", "query"), origin)
               for row in search_analytics(client, site, *period, ["page", "query"])]
    plan = inspection_plan(families, options.inspect)
    if plan:
        print(f"search-report: inspecting {len(plan)} URLs", file=sys.stderr)
    inspections = []
    for url in plan:
        verdict, coverage = inspect(client, site, url)
        locale, family = families[url]
        inspections.append({"url": url, "locale": locale, "family": family,
                            "verdict": verdict, "coverage_state": coverage})

    options.out.mkdir(parents=True, exist_ok=True)
    report = render(options, site, families, ignored, pages, queries, inspections)
    (options.out / "report.md").write_text(report, encoding="utf-8")
    measured = ["clicks", "impressions", "ctr", "position"]
    write_csv(options.out / "pages.csv", ["url", "locale", "family", "in_sitemap", *measured],
              [{**page, "in_sitemap": "yes" if page["url"] in families else "no"} for page in pages])
    write_csv(options.out / "queries.csv", ["url", "query", "locale", "family", *measured],
              [{**row, "query": spreadsheet_safe(row["query"])} for row in queries])
    write_csv(options.out / "inspection.csv", ["url", "locale", "family", "verdict", "coverage_state"],
              inspections)
    return (f"sitemap_urls={len(families)} pages={len(pages)} query_rows={len(queries)} "
            f"inspected={len(inspections)} out={options.out}")


def parse_args(argv, today):
    def day(value):
        return datetime.strptime(value, "%Y-%m-%d").date()

    def origin(value):
        value = value.rstrip("/")
        if not re.fullmatch(r"https?://[^/?#@\s]+", value):
            raise argparse.ArgumentTypeError("expected an origin such as " + DEFAULT_ORIGIN)
        return value

    def count(value):
        number = int(value)
        if number < 0:
            raise argparse.ArgumentTypeError("expected zero or more")
        return number

    parser = argparse.ArgumentParser(
        description="Write a Search Console report by locale and page family.",
        epilog="Environment: GSC_ACCESS_TOKEN (required, an OAuth access token with the "
               "webmasters.readonly scope) and GSC_SITE (default " + DEFAULT_SITE + "). "
               "The token is never printed or written. See scripts/seo/README.md.")
    parser.add_argument("--start", type=day, metavar="YYYY-MM-DD",
                        help="first day (default: 27 days before --end)")
    parser.add_argument("--end", type=day, metavar="YYYY-MM-DD",
                        help="last day (default: 3 days before today)")
    parser.add_argument("--origin", type=origin, default=DEFAULT_ORIGIN,
                        help="public origin whose sitemaps are read (default: %(default)s)")
    parser.add_argument("--out", type=Path, default=Path(DEFAULT_OUT),
                        help="output directory, never committed (default: %(default)s)")
    parser.add_argument("--inspect", type=count, default=20, metavar="N",
                        help=f"sitemap URLs to inspect per locale and family (default: %(default)s; "
                             f"at most {INSPECT_CAP} in total; 0 skips URL Inspection)")
    options = parser.parse_args(argv)
    options.end = options.end or today - timedelta(days=3)
    options.start = options.start or options.end - timedelta(days=27)
    if options.start > options.end:
        parser.error("--start must not be after --end")
    return options


def main(argv=None, env=None, transport=urllib_transport, sleep=time.sleep, today=None):
    env = os.environ if env is None else env
    options = parse_args(argv, today or date.today())
    token = env.get("GSC_ACCESS_TOKEN", "").strip()
    try:
        if not token:
            raise ReportError("GSC_ACCESS_TOKEN is not set; see scripts/seo/README.md")
        # http.client quotes a header value it refuses, so refuse it here first.
        if not re.fullmatch(r"[\x21-\x7e]+", token):
            raise ReportError("GSC_ACCESS_TOKEN must be one token of printable ASCII without spaces")
        summary = run(options, env.get("GSC_SITE") or DEFAULT_SITE, Client(transport, sleep, token))
    except (ReportError, OSError) as error:
        message = " ".join(str(error).split())
        print("search-report:", message.replace(token, "[redacted]") if token else message, file=sys.stderr)
        return 1
    print("search-report:", summary)
    return 0


if __name__ == "__main__":
    sys.exit(main())
