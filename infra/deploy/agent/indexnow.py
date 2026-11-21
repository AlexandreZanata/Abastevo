#!/usr/bin/env python3
"""Submit changes in the public sitemaps independently of the deploy agent."""
from http.client import HTTPException
import json
import math
import os
from pathlib import Path
import re
import sys
import tempfile
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener
import xml.etree.ElementTree as XML


DEFAULT_ORIGIN = "https://abastevo.com.br"
DEFAULT_ENDPOINT = "https://api.indexnow.org/indexnow"
DEFAULT_SITEMAPS = "/sitemap.xml /sitemap-portal.xml"
DEFAULT_STATE_DIR = "/var/lib/abastevo-deploy"
BATCH_SIZE = 10_000


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):
        return None


def origin_host(origin):
    message = "ABASTEVO_INDEXNOW_ORIGIN must be an HTTPS origin without a path"
    if not re.fullmatch(r"https?://[^/?#@\s]+", origin):
        raise ValueError(message)
    try:
        parts = urlsplit(origin)
        port = parts.port
    except ValueError:
        raise ValueError(message) from None
    if parts.scheme == "http":
        if parts.hostname == "127.0.0.1" and port:
            return parts.hostname
        raise ValueError(message + " (HTTP is only allowed on 127.0.0.1:<port> for tests)")
    if not parts.hostname:
        raise ValueError(message)
    return parts.hostname


def request(opener, url, timeout, payload=None):
    method = "sitemap fetch" if payload is None else "submission"
    options = {}
    if payload is not None:
        options = {
            "data": json.dumps(payload).encode("utf-8"),
            "headers": {"Content-Type": "application/json; charset=utf-8"},
        }
    try:
        with opener.open(Request(url, **options), timeout=timeout) as response:
            if payload is None:
                return response.read()
            if response.status not in (200, 202):
                raise ValueError(f"{method} returned HTTP {response.status}")
    except HTTPError as error:
        raise ValueError(f"{method} returned HTTP {error.code}") from None
    except (HTTPException, OSError) as error:
        raise ValueError(f"{method} failed: {error}") from None


def read_sitemaps(opener, origin, paths, timeout):
    current = {}
    ignored = 0

    def read(url, nested=False):
        nonlocal ignored
        try:
            root = XML.fromstring(request(opener, url, timeout))
        except XML.ParseError as error:
            raise ValueError(f"invalid sitemap XML: {error}") from None
        namespace, _, kind = root.tag.rpartition("}")
        prefix = namespace + "}" if namespace else ""
        if kind not in ("urlset", "sitemapindex"):
            raise ValueError("expected urlset or sitemapindex")
        if kind == "sitemapindex" and nested:
            raise ValueError("nested sitemap index")
        entry_tag = "url" if kind == "urlset" else "sitemap"
        for entry in root.findall(prefix + entry_tag):
            loc = entry.findtext(prefix + "loc", "").strip()
            if not loc:
                raise ValueError("sitemap entry is missing loc")
            if not loc.startswith(origin + "/"):
                ignored += 1
                continue
            if kind == "sitemapindex":
                read(loc, nested=True)
            else:
                current[loc] = entry.findtext(prefix + "lastmod", "").strip()

    for path in paths:
        read(origin + path)
    return current, ignored


def read_state(path):
    try:
        with path.open(encoding="utf-8") as source:
            state = json.load(source)
    except FileNotFoundError:
        return {}
    if not isinstance(state, dict) or any(not isinstance(value, str) for value in state.values()):
        raise ValueError("invalid IndexNow state: expected a map of URL to lastmod")
    return state


def save_state(path, current):
    path.parent.mkdir(parents=True, exist_ok=True)
    output = tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=path.parent, delete=False)
    temporary = Path(output.name)
    try:
        with output:
            json.dump(current, output, sort_keys=True)
            output.write("\n")
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def submit(env):
    if env.get("ABASTEVO_INDEXNOW_ENABLED") != "1":
        return "disabled"

    origin = env.get("ABASTEVO_INDEXNOW_ORIGIN", DEFAULT_ORIGIN)
    host = origin_host(origin)
    key = env.get("ABASTEVO_INDEXNOW_KEY", "")
    if not re.fullmatch(r"[a-f0-9]{32}", key):
        raise ValueError("ABASTEVO_INDEXNOW_KEY must be 32 lowercase hexadecimal characters")
    paths = env.get("ABASTEVO_INDEXNOW_SITEMAPS", DEFAULT_SITEMAPS).split()
    if not paths or any(not path.startswith("/") or path.startswith("//") for path in paths):
        raise ValueError("ABASTEVO_INDEXNOW_SITEMAPS must contain sitemap paths on the origin")
    timeout = float(env.get("ABASTEVO_INDEXNOW_TIMEOUT", "20"))
    if not math.isfinite(timeout) or timeout <= 0:
        raise ValueError("ABASTEVO_INDEXNOW_TIMEOUT must be a positive number of seconds")
    state_dir = Path(env.get("ABASTEVO_STATE_DIR") or DEFAULT_STATE_DIR)
    state = Path(env.get("ABASTEVO_INDEXNOW_STATE", state_dir / "indexnow-state.json"))

    opener = build_opener(NoRedirect)
    current, ignored = read_sitemaps(opener, origin, paths, timeout)
    previous = read_state(state)
    if not current and previous:
        raise ValueError("current sitemap URL set is empty and accepted state is not empty")

    new = current.keys() - previous.keys()
    removed = previous.keys() - current.keys()
    changed = {url for url in current.keys() & previous.keys() if current[url] != previous[url]}
    urls = sorted(new | changed | removed)
    if not urls:
        return "no change"

    endpoint = env.get("ABASTEVO_INDEXNOW_ENDPOINT", DEFAULT_ENDPOINT)
    identity = {"host": host, "key": key, "keyLocation": f"{origin}/{key}.txt"}
    for offset in range(0, len(urls), BATCH_SIZE):
        request(opener, endpoint, timeout, {**identity, "urlList": urls[offset:offset + BATCH_SIZE]})
    save_state(state, current)
    return (f"submitted={len(urls)} new={len(new)} changed={len(changed)} "
            f"removed={len(removed)} ignored={ignored}")


def main():
    try:
        result = submit(os.environ)
    except (ValueError, OSError) as error:
        print("indexnow:", " ".join(str(error).split()), file=sys.stderr)
        return 1
    print("indexnow:", result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
