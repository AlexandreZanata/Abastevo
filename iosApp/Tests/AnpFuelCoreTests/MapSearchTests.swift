import Foundation
import XCTest
@testable import AnpFuelCore

final class MapSearchTests: XCTestCase {
    private let city = Municipality(ibgeCode: "3550308", name: "São Paulo", state: .SP)

    func testSearchTextAndPublicCityBiasAreBoundedAndStationRequestUsesTheSameQuery() throws {
        XCTAssertNil(MapSearchPolicy.appleQuery(" a ", in: city))
        XCTAssertEqual(MapSearchPolicy.appleQuery("  Praça São Bento  ", in: city), "Praça São Bento, São Paulo, SP, Brasil")
        let maximum = String(repeating: "é", count: 100)
        XCTAssertEqual(MapSearchPolicy.appleQuery(maximum + "extra", in: city), maximum + ", São Paulo, SP, Brasil")

        let query = try StationDossierCodec.searchQuery(
            in: city, query: "Praça São Bento", cursor: "next", pageSize: MapSearchPolicy.directoryPageSize
        )
        XCTAssertEqual(query, [
            URLQueryItem(name: "state", value: "SP"),
            URLQueryItem(name: "municipality_code", value: "3550308"),
            URLQueryItem(name: "limit", value: "50"),
            URLQueryItem(name: "q", value: "Praça São Bento"),
            URLQueryItem(name: "cursor", value: "next"),
        ])
        XCTAssertThrowsError(try StationDossierCodec.searchQuery(in: city, query: "x", cursor: nil, pageSize: 50))
        XCTAssertThrowsError(try StationDossierCodec.searchQuery(in: city, query: "posto", cursor: nil, pageSize: 101))
        XCTAssertThrowsError(try StationDossierCodec.searchQuery(in: city, query: "posto", cursor: "", pageSize: 50))
    }

    func testSearchNeverCentersOnAStationCentroidOrInvalidPoint() throws {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/map-discovery/cases-v1.json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "cases-v1", withExtension: "json"))
        #endif
        let document = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
        let cases = try XCTUnwrap(document["cases"] as? [[String: Any]])
        for id in ["MAP-C01-ANP-ONLY", "MAP-C06-CITY-CENTROID"] {
            let vector = try XCTUnwrap(cases.first { $0["id"] as? String == id })
            let record = try XCTUnwrap(vector["station"] as? [String: Any])
            let location = try XCTUnwrap(record["location"] as? [String: Any])
            let latitude = try XCTUnwrap(location["latitude"] as? Double)
            let longitude = try XCTUnwrap(location["longitude"] as? Double)
            let tier = try XCTUnwrap(CoordinatePinTier(rawValue: XCTUnwrap(location["quality"] as? String)))
            let station = StationRecord(
                id: try XCTUnwrap(record["id"] as? String), cnpj: try XCTUnwrap(record["cnpj"] as? String),
                name: try XCTUnwrap(record["name"] as? String), brand: "", address: "", neighborhood: "",
                municipalityCode: city.ibgeCode, latitude: latitude, longitude: longitude, pinTier: tier,
                officialPricesMilli: [:], officialDateLabel: "", communityObservations: []
            )
            let expected = try XCTUnwrap(vector["expected"] as? [String: Any])
            XCTAssertEqual(MapSearchPolicy.pin(for: station) != nil, try XCTUnwrap(expected["pin"] as? Bool), id)
        }
        XCTAssertNil(MapSearchPolicy.coordinate(latitude: .infinity, longitude: 0))
        XCTAssertNil(MapSearchPolicy.coordinate(latitude: -.infinity, longitude: 0))
        XCTAssertNil(MapSearchPolicy.coordinate(latitude: 0, longitude: .nan))
        XCTAssertNil(MapSearchPolicy.coordinate(latitude: 0, longitude: 181))
    }

    func testAdmittedChargersMatchSelectedCityAndFiltersWithoutFabricatingLocation() {
        let local = charger(id: "first", municipality: "SAO PAULO", uf: "SP", access: .publicAccess)
        let other = charger(id: "second", municipality: "Santo André", uf: "SP", access: .publicAccess)
        let privatePoint = charger(id: "third", municipality: "São Paulo", uf: "SP", access: .unknown)
        let results = MapSearchPolicy.chargingMatches(
            [local, other, privatePoint], query: "gamma", municipality: city, filter: ChargingFilter(publicOnly: true)
        )
        XCTAssertEqual(results.map(\.id), [local.id])
        XCTAssertNil(results.first?.coordinate)
        XCTAssertTrue(MapSearchPolicy.chargingMatches(
            [local], query: "g", municipality: city, filter: ChargingFilter()
        ).isEmpty)
    }

    private func charger(id: String, municipality: String, uf: String, access: ChargingAccess) -> ChargingPlace {
        ChargingPlace(
            id: id, entityKey: id, externalId: id, name: "Gamma", latitude: nil, longitude: nil,
            publicAccess: access, reportedPoints: 0, groups: [], sources: [], confidence: nil,
            uf: uf, linkedStationId: nil, operatorName: nil, addressLine: nil, municipality: municipality
        )
    }
}
