#!/usr/bin/env python3
"""Local, bounded source acquisition. This is not a production fetch service."""
import argparse
import hashlib
import json
import os
import re
import subprocess
from pathlib import Path
import time
import urllib.error
import urllib.parse
import urllib.request

OSM_URL = "https://download.geofabrik.de/south-america/brazil-latest.osm.pbf"
OCM_ROOT = "https://api.openchargemap.io/v3/"
PROFILE = "ev-national-acquisition-v1"
MAX_OSM_BYTES = 3 << 30
MAX_PAGE_BYTES = 8 << 20
MAX_OCM_BYTES = 128 << 20
MAX_OCM_PAGES = 100
PAGE_ROWS = 1000


class SourceError(Exception):
    """Safe operator message; never include source bodies or credential values."""


class PublisherRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # The public latest alias redirects to the immutable daily BR file.
        # No credential-bearing OCM request or further redirect is eligible.
        if req is not None and req.full_url == OSM_URL and req.get_method() == "GET" and re.fullmatch(r"https://download\.geofabrik\.de/south-america/brazil-[0-9]{6}\.osm\.pbf", newurl) and not any(name.lower() == "x-api-key" for name in req.headers):
            return super().redirect_request(req, fp, code, msg, headers, newurl)
        raise SourceError("publisher redirect refused")


def transport(request):
    try:
        return urllib.request.build_opener(PublisherRedirects()).open(request, timeout=30)
    except urllib.error.HTTPError as error:
        raise SourceError(f"publisher HTTP status {error.code}") from None
    except (urllib.error.URLError, OSError):
        raise SourceError("publisher transport failed") from None


def request(url, key=None, headers=None):
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme != "https" or parsed.hostname not in {"download.geofabrik.de", "api.openchargemap.io"}:
        raise SourceError("unapproved publisher origin")
    if parsed.username or parsed.password or parsed.fragment:
        raise SourceError("invalid publisher URL")
    values = {"User-Agent": "Abastevo-EVC01-local-preparation/1", **(headers or {})}
    if key:
        if parsed.hostname != "api.openchargemap.io":
            raise SourceError("OCM credential on foreign origin refused")
        values["X-API-Key"] = key
    return urllib.request.Request(url, headers=values)


def outside_checkout(path):
    root = Path(__file__).resolve().parents[3]
    resolved = Path(path).resolve()
    result = subprocess.run(["git", "-C", str(root), "rev-parse", "--path-format=absolute", "--git-common-dir"], capture_output=True, text=True, timeout=5)
    if result.returncode:
        raise SourceError("repository boundary verification failed")
    primary = Path(result.stdout.strip()).resolve().parent
    if any(resolved == checkout or checkout in resolved.parents for checkout in [root, primary]):
        raise SourceError("raw source output must be outside the checkout")
    return resolved


def new_directory(path):
    path = outside_checkout(path)
    path.mkdir(mode=0o700, parents=False, exist_ok=False)
    return path


def write_json(path, value):
    with open(path, "x", encoding="utf-8", opener=lambda p, f: os.open(p, f, 0o600)) as output:
        json.dump(value, output, ensure_ascii=False, indent=2)
        output.write("\n")


def download_osm(output, expected_bytes, etag, modified, fetch=transport):
    if not 0 < expected_bytes <= MAX_OSM_BYTES or not etag or not modified:
        raise SourceError("explicit bounded publisher pin required")
    directory = new_directory(output)
    partial = directory / "brazil.osm.pbf.part"
    destination = directory / "brazil.osm.pbf"
    total, digest = 0, hashlib.sha256()
    try:
        req = request(OSM_URL, headers={"If-Match": etag, "If-Unmodified-Since": modified})
        with fetch(req) as response:
            if response.status != 200 or response.headers.get("ETag") != etag or response.headers.get("Last-Modified") != modified:
                raise SourceError("publisher pin changed")
            final_url = getattr(response, "geturl", lambda: OSM_URL)()
            if int(response.headers.get("Content-Length", "-1")) != expected_bytes:
                raise SourceError("publisher byte pin changed")
            with open(partial, "xb", opener=lambda p, f: os.open(p, f, 0o600)) as target:
                while chunk := response.read(1 << 20):
                    total += len(chunk)
                    if total > expected_bytes:
                        raise SourceError("source exceeded byte pin")
                    digest.update(chunk)
                    target.write(chunk)
                target.flush()
                os.fsync(target.fileno())
        if total != expected_bytes:
            raise SourceError("truncated source")
        # link refuses overwrite; the source file and receipt are private.
        os.link(partial, destination)
        partial.unlink()
        pin = {"profile": PROFILE, "source": "OSM", "url": OSM_URL, "resolved_url": final_url,
               "bytes": total, "sha256": digest.hexdigest(), "etag": etag,
               "last_modified": modified, "license": "ODbL-1.0",
               "attribution": "© OpenStreetMap contributors", "acquired_at": utc_now()}
        write_json(directory / "source-pin.json", pin)
        return pin
    except Exception:
        partial.unlink(missing_ok=True)
        destination.unlink(missing_ok=True)
        (directory / "source-pin.json").unlink(missing_ok=True)
        directory.rmdir()
        raise


def read_key(path=None, variable=None):
    value = None
    if path:
        key_path = Path(path)
        if key_path.stat().st_mode & 0o077 or key_path.stat().st_size > 4096:
            raise SourceError("key file must be private and bounded")
        value = key_path.read_text().strip()
    elif variable:
        value = os.environ.get(variable, "")
    if not value or len(value) > 512 or any(ord(char) < 33 or ord(char) > 126 for char in value):
        raise SourceError("a configured private OCM API key is required")
    return value


def read_json_response(response, limit):
    if response.status != 200:
        raise SourceError("publisher request did not succeed")
    raw = response.read(limit + 1)
    if len(raw) > limit:
        raise SourceError("source response exceeded byte budget")
    declared = response.headers.get("Content-Length")
    if declared is not None and int(declared) != len(raw):
        raise SourceError("truncated publisher response")
    try:
        return raw, json.loads(raw)
    except (ValueError, UnicodeError):
        raise SourceError("publisher JSON refused") from None


def collect_ocm(output, key, fetch=transport, pause=time.sleep):
    directory = new_directory(output)
    completed = False
    try:
        with fetch(request(OCM_ROOT + "referencedata/?output=json", key)) as response:
            reference, decoded = read_json_response(response, MAX_PAGE_BYTES)
        if not isinstance(decoded, dict):
            raise SourceError("OCM reference object required")
        provider_rows = decoded.get("DataProviders")
        if not isinstance(provider_rows, list) or any(not isinstance(item, dict) for item in provider_rows):
            raise SourceError("OCM provider reference shape refused")
        providers = {}
        for item in provider_rows:
            identifier = item.get("ID")
            if type(identifier) is not int or identifier in providers:
                raise SourceError("OCM provider reference identity refused")
            providers[identifier] = item
        expected = {1: "Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0)", 40: "CC-0", 57: "CC0"}
        rights = []
        for identifier, license_text in expected.items():
            provider = providers.get(identifier, {})
            if provider.get("License") != license_text:
                raise SourceError("OCM provider rights changed; new admission required")
            rights.append({"provider_id": identifier, "title": provider.get("Title"),
                           "license": license_text, "license_sha256": hashlib.sha256(license_text.encode()).hexdigest()})
        refs = directory / "referencedata.json"
        with open(refs, "xb", opener=lambda p, f: os.open(p, f, 0o600)) as target:
            target.write(reference)
        files, total, rows, after, seen = [], len(reference), 0, 0, set()
        for page_number in range(MAX_OCM_PAGES):
            params = {"output": "json", "countrycode": "BR", "dataproviderid": "1,40,57",
                      "opendata": "true", "maxresults": PAGE_ROWS, "sortby": "id_asc",
                      "greaterthanid": after, "compact": "true", "verbose": "false", "includecomments": "false"}
            url = OCM_ROOT + "poi/?" + urllib.parse.urlencode(params)
            with fetch(request(url, key)) as response:
                raw, page = read_json_response(response, MAX_PAGE_BYTES)
            if not isinstance(page, list) or len(page) > PAGE_ROWS:
                raise SourceError("OCM page shape/budget refused")
            total += len(raw)
            if total > MAX_OCM_BYTES:
                raise SourceError("OCM cumulative byte budget exceeded")
            previous = after
            for item in page:
                if not isinstance(item, dict) or type(item.get("ID")) is not int or item["ID"] <= after or item["ID"] in seen:
                    raise SourceError("OCM pagination/identity refused")
                if item.get("DataProviderID") not in expected or not isinstance(item.get("AddressInfo"), dict) or item["AddressInfo"].get("CountryID") != 36:
                    raise SourceError("OCM country/provider scope refused")
                seen.add(item["ID"])
                after = item["ID"]
            name = f"page-{page_number:03}.json"
            with open(directory / name, "xb", opener=lambda p, f: os.open(p, f, 0o600)) as target:
                target.write(raw)
            files.append({"file": name, "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest(), "rows": len(page), "after_id": previous})
            rows += len(page)
            if len(page) < PAGE_ROWS:
                break
            pause(1)
        else:
            raise SourceError("OCM page cap reached; no complete snapshot")
        pin = {"profile": PROFILE, "source": "OCM", "country": "BR", "acquired_at": utc_now(),
               "rows": rows, "bytes": total, "complete_pagination": True, "files": files,
               "reference_sha256": hashlib.sha256(reference).hexdigest(), "providers": rights}
        write_json(directory / "source-pin.json", pin)
        completed = True
        return pin
    finally:
        if not completed:
            for child in directory.iterdir():
                if child.is_file():
                    child.unlink()
            directory.rmdir()


def utc_now():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument("--plan", action="store_true")
    action.add_argument("--fetch-osm", action="store_true")
    action.add_argument("--collect-ocm", action="store_true")
    parser.add_argument("--output")
    parser.add_argument("--expected-bytes", type=int)
    parser.add_argument("--etag")
    parser.add_argument("--last-modified")
    credentials = parser.add_mutually_exclusive_group()
    credentials.add_argument("--key-file")
    credentials.add_argument("--key-env")
    args = parser.parse_args()
    if args.plan:
        print(json.dumps({"profile": PROFILE, "osm_url": OSM_URL, "osm_max_bytes": MAX_OSM_BYTES,
                          "ocm_country": "BR", "ocm_provider_ids": [1, 40, 57], "ocm_max_bytes": MAX_OCM_BYTES,
                          "ocm_page_cap": MAX_OCM_PAGES, "ocm_rows_per_page": PAGE_ROWS}))
        return
    if not args.output:
        raise SourceError("explicit private output directory required")
    modes = [os.environ.get(key, "").lower() for key in ["ANPFUEL_ENV", "ANPFUEL_RUN_ENV", "DEPLOY_ENV"]]
    if any(mode in {"prod", "production", "stage", "staging"} for mode in modes) or os.environ.get("KUBERNETES_SERVICE_HOST"):
        raise SourceError("national acquisition is local-only")
    if args.fetch_osm:
        pin = download_osm(args.output, args.expected_bytes or 0, args.etag, args.last_modified)
    else:
        pin = collect_ocm(args.output, read_key(args.key_file, args.key_env))
    print(json.dumps({"profile": pin["profile"], "source": pin["source"], "bytes": pin["bytes"], "rows": pin.get("rows"), "sha256": pin.get("sha256")}))


if __name__ == "__main__":
    try:
        main()
    except SourceError as error:
        raise SystemExit(str(error)) from None
    except (OSError, ValueError, TypeError):
        raise SystemExit("source acquisition failed; private details withheld") from None
