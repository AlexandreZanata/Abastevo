import hashlib
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import ev_national_sources as source


class Response(io.BytesIO):
    def __init__(self, raw, headers=None):
        super().__init__(raw)
        self.status = 200
        self.headers = headers or {"Content-Length": str(len(raw))}


class NationalAcquisitionTests(unittest.TestCase):
    def test_osm_pin_truncation_and_last_good(self):
        for fault in [None, "etag", "length", "truncated", "oversize"]:
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as root:
                target = Path(root) / "source"
                raw = b"synthetic-pbf-bytes"
                headers = {"Content-Length": str(len(raw)), "ETag": "synthetic", "Last-Modified": "synthetic-date"}
                body = raw
                if fault == "etag": headers["ETag"] = "changed"
                if fault == "length": headers["Content-Length"] = "999"
                if fault == "truncated": body = raw[:-1]
                if fault == "oversize": body = raw + b"extra"
                fetch = lambda req: Response(body, headers)
                if fault:
                    with self.assertRaises(source.SourceError):
                        source.download_osm(target, len(raw), "synthetic", "synthetic-date", fetch)
                    self.assertFalse((target / "brazil.osm.pbf").exists())
                    self.assertFalse((target / "brazil.osm.pbf.part").exists())
                else:
                    pin = source.download_osm(target, len(raw), "synthetic", "synthetic-date", fetch)
                    self.assertEqual(hashlib.sha256(raw).hexdigest(), pin["sha256"])
                    self.assertEqual(0o600, (target / "brazil.osm.pbf").stat().st_mode & 0o777)
                    with self.assertRaises(FileExistsError):
                        source.download_osm(target, len(raw), "synthetic", "synthetic-date", fetch)
                    self.assertEqual(raw, (target / "brazil.osm.pbf").read_bytes())

    def test_ocm_scope_rights_and_secret_safe_header(self):
        reference = {"DataProviders": [{"ID": 1, "Title": "[P25-TEST] Provider", "License": "Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0)"}, {"ID": 40, "License": "CC-0"}, {"ID": 57, "License": "CC0"}]}
        row = {"ID": 100, "DataProviderID": 1, "AddressInfo": {"CountryID": 36, "Title": "[P25-TEST] Site"}}
        for fault in [None, "license", "country", "provider", "duplicate"]:
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as root:
                refs = json.loads(json.dumps(reference))
                page = [json.loads(json.dumps(row))]
                if fault == "license": refs["DataProviders"][0]["License"] = "restricted"
                if fault == "country": page[0]["AddressInfo"]["CountryID"] = 1
                if fault == "provider": page[0]["DataProviderID"] = 999
                if fault == "duplicate": page += page
                calls = []
                def fetch(req):
                    calls.append(req.full_url)
                    self.assertNotIn("synthetic-key", req.full_url)
                    self.assertEqual("synthetic-key", req.get_header("X-api-key"))
                    value = refs if "referencedata" in req.full_url else page
                    return Response(json.dumps(value).encode())
                target = Path(root) / "ocm"
                if fault:
                    with self.assertRaises(source.SourceError):
                        source.collect_ocm(target, "synthetic-key", fetch, lambda _: None)
                    self.assertFalse(target.exists())
                else:
                    pin = source.collect_ocm(target, "synthetic-key", fetch, lambda _: None)
                    self.assertEqual(1, pin["rows"])
                    self.assertTrue(pin["complete_pagination"])
                    self.assertNotIn("synthetic-key", (target / "source-pin.json").read_text())
                    self.assertIn("countrycode=BR", calls[-1])
                    self.assertIn("opendata=true", calls[-1])

    def test_foreign_origin_and_redirect_refused(self):
        for url in ["http://api.openchargemap.io/v3/poi", "https://example.invalid/poi", "https://key@example.invalid/poi"]:
            with self.assertRaises(source.SourceError): source.request(url, "synthetic-key")
        with self.assertRaises(source.SourceError):
            source.PublisherRedirects().redirect_request(None, None, 302, "redirect", {}, "https://example.invalid")


if __name__ == "__main__": unittest.main()

class AcquisitionContinuationTests(unittest.TestCase):
    def test_raw_sources_cannot_enter_any_ancestor_checkout(self):
        checkout = Path(source.__file__).resolve().parents[3]
        import subprocess
        primary = Path(subprocess.check_output(["git", "-C", str(checkout), "rev-parse", "--path-format=absolute", "--git-common-dir"], text=True).strip()).resolve().parent
        for path in [checkout / "raw", primary / "raw"]:
            with self.subTest(path=path), self.assertRaises(source.SourceError):
                source.outside_checkout(path)

    def test_ocm_multiple_pages_caps_and_stale_cursor(self):
        from unittest.mock import patch
        reference = {"DataProviders": [{"ID": 1, "License": "Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0)"}, {"ID": 40, "License": "CC-0"}, {"ID": 57, "License": "CC0"}]}
        for fault in [None, "stale", "pages", "bytes"]:
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as root, patch.object(source, "PAGE_ROWS", 1), patch.object(source, "MAX_OCM_PAGES", 1 if fault == "pages" else 4), patch.object(source, "MAX_OCM_BYTES", 1 if fault == "bytes" else 1 << 20):
                calls = []
                def fetch(req):
                    if "referencedata" in req.full_url:
                        return Response(json.dumps(reference).encode())
                    from urllib.parse import parse_qs, urlsplit
                    after = int(parse_qs(urlsplit(req.full_url).query)["greaterthanid"][0])
                    calls.append(after)
                    identifier = 100 if after == 0 or fault == "stale" else 101
                    page = [] if after == 101 else [{"ID": identifier, "DataProviderID": 1, "AddressInfo": {"CountryID": 36}}]
                    return Response(json.dumps(page).encode())
                target = Path(root) / "ocm"
                if fault:
                    with self.assertRaises(source.SourceError): source.collect_ocm(target, "synthetic-key", fetch, lambda _: None)
                    self.assertFalse(target.exists())
                else:
                    pin = source.collect_ocm(target, "synthetic-key", fetch, lambda _: None)
                    self.assertEqual([0, 100, 101], calls)
                    self.assertEqual(2, pin["rows"])


class PublisherAliasTests(unittest.TestCase):
    def test_only_geofabrik_latest_to_its_dated_brazil_file_is_allowed(self):
        handler=source.PublisherRedirects()
        alias=source.request(source.OSM_URL)
        dated="https://download.geofabrik.de/south-america/brazil-261102.osm.pbf"
        redirected=handler.redirect_request(alias,None,307,"temporary",{},dated)
        self.assertEqual(dated,redirected.full_url)
        for origin,target in [(source.request(source.OCM_ROOT+"poi/","synthetic-key"),dated),(alias,dated.replace("brazil-","argentina-")),(alias,dated.replace("download.geofabrik.de","example.invalid")),(source.request(dated),source.OSM_URL)]:
            with self.subTest(target=target),self.assertRaises(source.SourceError):
                handler.redirect_request(origin,None,307,"temporary",{},target)

class BoundaryAcquisitionTests(unittest.TestCase):
    def test_ibge_scope_and_bounded_receipt(self):
        for fault in [None,"foreign_shape"]:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as root:
                value={"type":"FeatureCollection","features":[{"type":"Feature","properties":{"codarea":"3550308"},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}]}
                if fault:value["type"]="unexpected"
                def fetch(req):
                    self.assertTrue(req.full_url.startswith("https://servicodados.ibge.gov.br/api/v4/malhas/paises/BR?"))
                    self.assertIsNone(req.get_header("X-api-key"))
                    return Response(json.dumps(value).encode())
                target=Path(root)/"boundaries"
                if fault:
                    with self.assertRaises(source.SourceError):source.download_ibge(target,fetch)
                    self.assertFalse(target.exists())
                else:
                    pin=source.download_ibge(target,fetch)
                    self.assertEqual(1,pin["features"])
                    self.assertEqual("CC-BY-4.0",pin["license"])
                    self.assertEqual(0o600,(target/"municipalities.geojson").stat().st_mode&0o777)

    def test_ibge_gzip_preserves_document_bytes_and_bounds(self):
        import gzip
        raw = b'{"type":"FeatureCollection","features":[{"properties":{"codarea":"3550308"}}]}'
        zipped = gzip.compress(raw)
        with tempfile.TemporaryDirectory() as root:
            target = Path(root) / "boundaries"
            pin = source.download_ibge(target, lambda _: Response(zipped, {
                "Content-Length": str(len(zipped)), "Content-Encoding": "gzip",
            }))
            self.assertEqual(hashlib.sha256(raw).hexdigest(), pin["sha256"])
            self.assertEqual(raw, (target / "municipalities.geojson").read_bytes())
        for invalid in [zipped[:-4], gzip.compress(b" " * 1000)]:
            with self.assertRaises(source.SourceError):
                source.read_json_response(Response(invalid, {"Content-Encoding": "gzip"}), 128, allow_gzip=True)
