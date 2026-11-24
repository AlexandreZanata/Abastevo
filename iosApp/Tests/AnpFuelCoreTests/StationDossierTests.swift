import Foundation
import XCTest
@testable import AnpFuelCore

final class StationDossierTests: XCTestCase {
    private let stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private func fixture(_ directory: String, _ name: String) throws -> [String: Any] {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/\(directory)/\(name).json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        let data = try Data(contentsOf: url)
        let shape = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        return shape["value"] as? [String: Any] ?? shape
    }

    private func data(_ object: [String: Any]) throws -> Data {
        try JSONSerialization.data(withJSONObject: object, options: .sortedKeys)
    }

    func testCanonicalAndLegacyRoutesRequireTheExactSameIdentity() throws {
        let fixture = try self.fixture("api", "station-valid")
        let cnpj = try XCTUnwrap(fixture["cnpj_normalized"] as? String)
        let canonical = StationDossierIdentity.canonical(stationId.uppercased())
        let legacy = StationDossierIdentity.legacyCNPJ(cnpj)
        let wire = try data(fixture)

        XCTAssertEqual(canonical.path, "/v1/stations/\(stationId)")
        XCTAssertEqual(legacy.path, "/v1/stations/by-cnpj/\(cnpj)")
        let byId = try StationDossierCodec.identity(wire, expected: canonical)
        let byCNPJ = try StationDossierCodec.identity(wire, expected: legacy)
        XCTAssertEqual(byId.station, byCNPJ.station)
        XCTAssertEqual(byId.station.id, stationId)
        XCTAssertNil(byId.officialAnchor, "The earlier public contract lacks an official anchor.")
        XCTAssertTrue(byId.station.pinTier.allowsMapPin)
        XCTAssertNil(StationDossierIdentity.legacyCNPJ("04218406000100").path)
        XCTAssertThrowsError(try StationDossierCodec.identity(wire, expected: .canonical(UUID().uuidString)))
        XCTAssertThrowsError(try StationDossierCodec.identity(wire, expected: .legacyCNPJ("12ABC34501DE35")))
    }

    func testRegistryFixtureNamesAndLocationClaimsDoNotGainUnsupportedAuthority() throws {
        let registry = try fixture("registry", "registry-api-sample")
        let records = try XCTUnwrap(registry["items"] as? [[String: Any]])
        for record in records {
            let cnpj = try XCTUnwrap(record["cnpj"] as? String)
            let legal = try XCTUnwrap(record["razaoSocial"] as? String)
            let trade = try XCTUnwrap(record["nomeFantasia"] as? String)
            let quality = try XCTUnwrap(record["location_quality"] as? String)
            var station = try fixture("api", "station-valid")
            station["cnpj_normalized"] = cnpj
            station["legal_name"] = legal
            station["display_name"] = trade
            station["address"] = record["endereco"]
            station["address_source"] = quality == "reviewed" ? "anp" : "cnefe"
            station["location_quality"] = quality
            station["coordinates"] = record["coordenadas"]
            if !(record["autorizacao"] is NSNull) {
                station["official"] = [
                    "source": "registry-api", "display_name": legal, "state": "SP",
                    "finished_at": try XCTUnwrap(registry["generated_at"] as? String),
                ]
            }

            let identity = StationDossierIdentity.legacyCNPJ(cnpj)
            XCTAssertNotNil(identity.path, "Keep the entire CNPJ, including letters and leading zeros.")
            let dossier = try StationDossierCodec.identity(data(station), expected: identity)
            XCTAssertEqual(dossier.station.cnpj, cnpj)
            XCTAssertEqual(dossier.station.name, trade)
            XCTAssertEqual(dossier.secondaryLegalName, legal)
            XCTAssertEqual(dossier.station.pinTier.allowsMapPin, quality == "reviewed")
            if record["autorizacao"] is NSNull {
                XCTAssertNil(dossier.officialAnchor)
            } else {
                XCTAssertEqual(dossier.officialAnchor?.source, "registry-api")
                XCTAssertNotNil(dossier.officialAnchor?.finishedDate)
            }
        }
    }

    func testDirectoryAddressDoesNotReadCoordinatesIBGEOrRepeatedLegalName() throws {
        let fields = ["street": "lat -23.55 lon -46.63", "number": "100", "municipio": "3550308/SP"]
        XCTAssertTrue(StationDossierAddress.line(fields, state: .SP).isEmpty)
        let supported = ["logradouro": "Avenida Central", "numero": "100", "bairro": "Centro", "city": "São Paulo"]
        let line = StationDossierAddress.line(supported, state: .SP)
        XCTAssertTrue(line.contains("Avenida Central, 100"))
        XCTAssertTrue(line.contains("Centro"))
        XCTAssertTrue(line.contains("São Paulo/SP"))

        var station = try fixture("api", "station-valid")
        station["legal_name"] = "  POSTO   CENTRAL  "
        station["address_source"] = "future-source"
        let parsed = try StationDossierCodec.identity(data(station), expected: .canonical(stationId))
        XCTAssertNil(parsed.secondaryLegalName)
        XCTAssertNil(parsed.addressSource)
    }

    func testMapDiscoveryFixtureRemainsListOnlyWhenLocationIsUnknown() throws {
        let corpus = try fixture("map-discovery", "cases-v1")
        let cases = try XCTUnwrap(corpus["cases"] as? [[String: Any]])
        let missing = try XCTUnwrap(cases.first { $0["id"] as? String == "MAP-C05-MISSING-COORDINATE" })
        let raw = try XCTUnwrap(missing["station"] as? [String: Any])
        let location = try XCTUnwrap(raw["location"] as? [String: Any])
        let id = try XCTUnwrap(raw["id"] as? String)
        let name = try XCTUnwrap(raw["name"] as? String)
        let quality = try XCTUnwrap(location["quality"] as? String)
        let station: [String: Any] = [
            "station_id": id, "display_name": name, "location_quality": quality,
            "coordinates": NSNull(),
        ]
        let dossier = try StationDossierCodec.identity(data(station), expected: .canonical(id))
        let expected = try XCTUnwrap(missing["expected"] as? [String: Any])
        XCTAssertEqual(dossier.station.pinTier.allowsMapPin, expected["pin"] as? Bool)
        XCTAssertEqual(dossier.station.name, name)
    }

    func testOfficialAndCommunityPriceGroupsReplayWithoutBlending() throws {
        let wire = try fixture("api", "price-groups-valid")
        let officialOnly = try StationDossierCodec.prices(data(wire), stationId: stationId)
        XCTAssertEqual(officialOnly.official.first?.amountMilli, 6_030)
        XCTAssertTrue(officialOnly.community.isEmpty)
        XCTAssertThrowsError(try StationDossierCodec.prices(data(wire), stationId: UUID().uuidString))

        let at = Date(timeIntervalSince1970: 1_790_553_600)
        let expiry = at.addingTimeInterval(60)
        let vectors = ["high-quorum", "conflicting-prices-disputed", "no-data-unknown"]
        for name in vectors {
            let corpus = try fixture("consensus", name)
            let expected = try XCTUnwrap(corpus["expect"] as? [String: Any])
            let verdict = try XCTUnwrap(expected["verdict"] as? String)
            let availability = verdict == "PRICE" ? "AVAILABLE" : verdict
            let projected: [String: Any] = [
                "source": "COMMUNITY", "availability": availability, "currency": "BRL",
                "amount_milli_brl": expected["amount_milli"] ?? NSNull(),
                "confidence": expected["confidence"] ?? "UNKNOWN",
                "freshness": expected["freshness"] ?? "UNKNOWN",
                "independent_supporters": expected["supporters"] ?? 0,
                "confirmation_count": expected["confirmations"] ?? 0,
                "algorithm_version": try XCTUnwrap(corpus["consensus_version"]), "projection_version": 1,
                "expires_at": expiry.ISO8601Format(),
            ]
            var page = wire
            var groups = try XCTUnwrap(page["items"] as? [[String: Any]])
            groups[0]["community"] = projected
            page["items"] = groups
            let parsed = try StationDossierCodec.prices(data(page), stationId: stationId)
            let community = try XCTUnwrap(parsed.community[.gasolineRegular])
            XCTAssertEqual(community.availability.rawValue, availability)
            XCTAssertEqual(community.amount(at: at), expected["amount_milli"] as? Int64)
            XCTAssertNil(community.amount(at: expiry), "An expired community amount never becomes current.")
            XCTAssertEqual(parsed.official.first?.amountMilli, 6_030)
        }
    }

    func testExplorationFixtureRefusesWrongUnitAndMixedSources() throws {
        let exploration = try fixture("mobile-expansion", "exploration-v1")
        let cases = try XCTUnwrap(exploration["cases"] as? [[String: Any]])
        let unitCase = try XCTUnwrap(cases.first { $0["id"] as? String == "cng-litre-refused" })
        let sourceCase = try XCTUnwrap(cases.first { $0["id"] as? String == "source-mixing-refused" })
        let invalidUnit = try XCTUnwrap(unitCase["value"] as? [String: Any])
        let invalidSource = try XCTUnwrap(sourceCase["value"] as? [String: Any])
        let product = try XCTUnwrap(invalidUnit["product"] as? [String: Any])
        let source = try XCTUnwrap(invalidSource["source"] as? [String: Any])
        var wire = try fixture("api", "price-groups-valid")
        var groups = try XCTUnwrap(wire["items"] as? [[String: Any]])
        groups[0]["fuel_product"] = product["fuel"]
        groups[0]["unit"] = product["unit"]
        wire["items"] = groups
        XCTAssertThrowsError(try StationDossierCodec.prices(data(wire), stationId: stationId))
        groups = try XCTUnwrap(try fixture("api", "price-groups-valid")["items"] as? [[String: Any]])
        var official = try XCTUnwrap(groups[0]["official"] as? [String: Any])
        official["source"] = source["kind"]
        groups[0]["official"] = official
        wire["items"] = groups
        XCTAssertThrowsError(try StationDossierCodec.prices(data(wire), stationId: stationId))
    }

    func testDirectorySearchNormalizationHonorsItsServerBounds() {
        XCTAssertEqual(StationDossierCodec.normalizedSearch("  São  "), "São")
        XCTAssertEqual(StationDossierCodec.normalizedSearch("  P  "), "")
        XCTAssertEqual(StationDossierCodec.normalizedSearch(String(repeating: "ó", count: 101)).unicodeScalars.count, 100)
    }
}
