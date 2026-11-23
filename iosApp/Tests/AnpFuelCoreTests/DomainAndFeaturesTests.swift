import XCTest
@testable import AnpFuelCore

final class DomainAndFeaturesTests: XCTestCase {
    func testSocialReadsUsePublishedApiEnvelopesAndStationFuelScope() throws {
        let forum = Data("""
        {"items":[{"id":"00000000-0000-4000-8000-000000000001","kind":"TOPIC","title":"Tópico",
        "category":"GERAL","first_published_at":"2026-11-23T10:00:00Z","public_alias":"Pessoa","excerpt":null}]}
        """.utf8)
        let posts = try AbastevoHttpClient.decodePublicPage(forum, as: ForumPublicPost.self)
        XCTAssertEqual(posts.first?.kind, .topic)
        XCTAssertEqual(posts.first?.publicAlias, "Pessoa")

        let contributors = Data("""
        {"items":[{"display_name":"Ana","contributor_since":"2026-11","roles":["development"]}],"next_cursor":""}
        """.utf8)
        let entries = try AbastevoHttpClient.decodePublicPage(contributors, as: PublicContributor.self)
        XCTAssertEqual(entries.first?.displayName, "Ana")
        XCTAssertEqual(entries.first?.roles, ["development"])

        let stationId = "00000000-0000-4000-8000-000000000001"
        let stationComments = Data("""
        {"comments":[{"id":"00000000-0000-4000-8000-000000000002","station_id":"\(stationId)",
        "product":"ETHANOL","public_alias":"Pessoa","text":"Comentário","created_at":"2026-11-23T10:00:00Z"}],
        "next_cursor":null}
        """.utf8)
        let remarks = try AbastevoHttpClient.decodePublicStationComments(stationComments, stationId: stationId, fuel: .ethanol)
        XCTAssertEqual(remarks.first?.text, "Comentário")
        XCTAssertThrowsError(try AbastevoHttpClient.decodePublicStationComments(stationComments, stationId: stationId, fuel: .gasolineRegular))
    }

    func testDirectoryPreservesCanonicalIdentityAndRequiresLocatedPins() throws {
        let city = Municipality(ibgeCode: "5107925", name: "Sorriso", state: .MT)
        let response = Data("""
        {
          "items": [
            {
              "station_id": "00000000-0000-4000-8000-000000000001",
              "display_name": "Test station A",
              "municipality_code": "5107925",
              "state": "MT",
              "location_quality": "osm-mapped",
              "coordinates": {"lat": -12.5, "lon": -55.7},
              "address": {"street": "Test street", "number": "12", "district": "Test district"}
            },
            {
              "station_id": "00000000-0000-4000-8000-000000000002",
              "display_name": "Test station B",
              "municipality_code": "5107925",
              "state": "MT",
              "location_quality": "unknown",
              "coordinates": null
            }
          ],
          "next_cursor": "opaque-next"
        }
        """.utf8)
        let result = try AbastevoHttpClient.decodeStations(response, in: city)
        XCTAssertEqual(result.stations.map(\.pinTier), [.osmMapped, .cityCentroid])
        XCTAssertEqual(result.stations.first?.address, "Test street, 12")
        XCTAssertEqual(result.stations.first?.id, "00000000-0000-4000-8000-000000000001")
        XCTAssertEqual(result.stations.first?.officialPricesMilli, [:])
        XCTAssertTrue(result.stations.allSatisfy { $0.openingHours.isEmpty && $0.officialDateLabel.isEmpty })
        XCTAssertEqual(result.nextCursor, "opaque-next")
        let empty = Data(#"{"items":null,"next_cursor":null}"#.utf8)
        XCTAssertTrue(try AbastevoHttpClient.decodeStations(empty, in: city).stations.isEmpty)
        XCTAssertThrowsError(try AbastevoHttpClient.decodeStations(Data("{}".utf8), in: city))

        let otherCity = Municipality(ibgeCode: "5103403", name: "Cuiabá", state: .MT)
        XCTAssertThrowsError(try AbastevoHttpClient.decodeStations(response, in: otherCity))
    }

    func testPriceGroupsUseDatedOfficialValuesSeparateFromCommunity() throws {
        let id = "00000000-0000-4000-8000-000000000001"
        let response = Data("""
        {
          "items": [
            {
              "station_id": "\(id)",
              "fuel_product": "GASOLINE_ADDITIVED",
              "unit": "L",
              "condition": {"kind": "STANDARD", "qualifier_id": null},
              "official": {
                "source": "ANP", "currency": "BRL", "amount_milli_brl": 6123,
                "collected_on": "2026-11-16",
                "survey_week": {"start": "2026-11-15", "end": "2026-11-21"}
              },
              "community": {"source": "COMMUNITY", "amount_milli_brl": 5999}
            },
            {
              "station_id": "\(id)",
              "fuel_product": "ETHANOL",
              "unit": "L",
              "condition": {"kind": "STANDARD", "qualifier_id": null},
              "official": null,
              "community": {"source": "COMMUNITY", "amount_milli_brl": 3999}
            }
          ]
        }
        """.utf8)
        let official = try AbastevoHttpClient.decodeOfficialPrices(response, stationId: id)
        XCTAssertEqual(official.map(\.product), [.gasolinePremium])
        XCTAssertEqual(official.first?.amountMilli, 6123)
        XCTAssertEqual(official.first?.collectedOn, "2026-11-16")
        XCTAssertEqual(official.first?.surveyEnd, "2026-11-21")
        XCTAssertThrowsError(try AbastevoHttpClient.decodeOfficialPrices(response, stationId: "other"))
    }

    func testCommunityFeedUsesPublishedTimeAndRejectsExpiredOrWrongScope() throws {
        let response = Data("""
        {
          "state": "MT",
          "municipality_code": "5107925",
          "fuel_product": "GASOLINE_REGULAR",
          "sort": "recent",
          "items": [{
            "station_id": "00000000-0000-4000-8000-000000000001",
            "station_name": "Test station A",
            "fuel_product": "GASOLINE_REGULAR",
            "unit": "L",
            "source": "COMMUNITY",
            "condition": "STANDARD",
            "currency": "BRL",
            "amount_milli_brl": 6123,
            "updated_at": "1970-01-01T00:01:00.250Z",
            "expires_at": "1970-01-01T00:02:00Z",
            "confirmations": 3
          }]
        }
        """.utf8)
        let result = try AbastevoHttpClient.decodeCommunityFeed(
            response, state: .MT, municipalityCode: "5107925", fuel: .gasolineRegular,
            now: Date(timeIntervalSince1970: 100)
        )
        XCTAssertEqual(result.first?.observedEpochSeconds, 60)
        XCTAssertEqual(result.first?.amountMilli, 6123)
        XCTAssertEqual(result.first?.confirmations, 3)
        XCTAssertFalse(result.first?.hasPhotoEvidence ?? true)
        XCTAssertTrue(try AbastevoHttpClient.decodeCommunityFeed(
            response, state: .MT, municipalityCode: "5107925", fuel: .gasolineRegular,
            now: Date(timeIntervalSince1970: 120)
        ).isEmpty)
        XCTAssertThrowsError(try AbastevoHttpClient.decodeCommunityFeed(
            response, state: .MT, municipalityCode: "5107925", fuel: .ethanol
        ))
    }

    func testPremiumEnergyIntervalsMatchSharedCorpus() throws {
        #if SWIFT_PACKAGE
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let fixture = root.appendingPathComponent("contracts/testdata/premium/energy-v1.json")
        #else
        let fixture = try XCTUnwrap(Bundle(for: DomainAndFeaturesTests.self).url(forResource: "energy-v1", withExtension: "json"))
        #endif
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let corpus = try decoder.decode(EnergyCorpus.self, from: Data(contentsOf: fixture))
        XCTAssertEqual(corpus.version, 1)
        let intervals = corpus.cases.filter { $0.expectedCombinedCentavosPerKm == nil }
        XCTAssertFalse(intervals.isEmpty)
        for vector in intervals {
            let input = PremiumEnergyCalculator.IntervalInput(
                purchasedWh: try XCTUnwrap(vector.purchasedWh, vector.id),
                costCentavos: try XCTUnwrap(vector.costCentavos, vector.id),
                distanceMetres: vector.distanceMetres,
                hasEnergy: try XCTUnwrap(vector.hasEnergy, vector.id),
                complete: try XCTUnwrap(vector.complete, vector.id),
                hasGap: vector.hasGap ?? false,
                crossesSegment: vector.crossesSegment ?? false,
                odometerMissing: vector.odometerMissing ?? false,
                socInconsistent: vector.socInconsistent ?? false,
                mixedCarrier: vector.mixedCarrier ?? false
            )
            let result = PremiumEnergyCalculator.compute(input)
            XCTAssertEqual(result.purchasedWh, input.purchasedWh, vector.id)
            XCTAssertEqual(result.costCentavos, input.costCentavos, vector.id)
            XCTAssertEqual(result.distanceMetres, input.distanceMetres, vector.id)
            XCTAssertEqual(result.whPerKm, try XCTUnwrap(vector.expectedWhPerKm, vector.id), vector.id)
            XCTAssertEqual(result.centavosPerKm, try XCTUnwrap(vector.expectedCentavosPerKm, vector.id), vector.id)
            XCTAssertEqual(result.unknown, try XCTUnwrap(vector.expectedUnknown, vector.id), vector.id)
        }
    }

    private struct EnergyCorpus: Decodable {
        let version: Int
        let cases: [EnergyVector]
    }

    private struct EnergyVector: Decodable {
        let id: String
        let distanceMetres: Int64
        let purchasedWh: Int64?
        let costCentavos: Int64?
        let hasEnergy: Bool?
        let complete: Bool?
        let hasGap: Bool?
        let crossesSegment: Bool?
        let odometerMissing: Bool?
        let socInconsistent: Bool?
        let mixedCarrier: Bool?
        let expectedWhPerKm: String?
        let expectedCentavosPerKm: String?
        let expectedUnknown: String?
        let expectedCombinedCentavosPerKm: String?
    }

    func testTrainingTermsSha256MatchesExactBackendContract() {
        XCTAssertEqual(TrainingTermsContract.version, "training-terms-v1")
        XCTAssertTrue(TrainingTermsContract.verifiedDigestMatches())
    }

    func testMunicipalitySearchUsesSharedNationalCatalog() throws {
        #if SWIFT_PACKAGE
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let fixture = root.appendingPathComponent("data/src/main/assets/ibge_municipalities.json")
        let catalog = try MunicipalityCatalog(data: Data(contentsOf: fixture))
        #else
        let catalog = try MunicipalityCatalog(bundle: Bundle(for: DomainAndFeaturesTests.self))
        #endif
        XCTAssertGreaterThanOrEqual(catalog.count, 5_570)
        XCTAssertEqual(Set(catalog.search(query: "").map(\.state)).count, 27)
        XCTAssertEqual(catalog.search(query: "sao paulo").first?.ibgeCode, "3550308")
        XCTAssertEqual(catalog.search(query: "sorriso", state: .MT).first?.ibgeCode, "5107925")
        XCTAssertTrue(catalog.search(query: "sorriso", state: .SP).isEmpty)
        XCTAssertEqual(catalog.search(query: "acrelandia", state: .AC).first?.ibgeCode, "1200013")
        XCTAssertEqual(catalog.search(query: "sao joao d’alianca").first?.ibgeCode, "5220009")
        XCTAssertEqual(catalog.search(query: "5101837").first?.name, "BOA ESPERANÇA DO NORTE")
        XCTAssertNil(catalog.municipality(code: "0000000"))
        XCTAssertThrowsError(try MunicipalityCatalog(data: Data("[]".utf8))) { error in
            XCTAssertEqual(error as? MunicipalityCatalog.CatalogError, .invalidCatalog)
        }
    }

    func testEvTripPlannerProducesStopsForLongDistance() {
        let req = EvTripPlanRequest(
            originName: "São Paulo",
            destinationName: "Curitiba",
            distanceKm: 408,
            batteryKwh: 45,
            startSocPercent: 80,
            reserveMinSocPercent: 15,
            whPerKm: 170
        )
        let outcome = EvTripPlanner.plan(request: req, sites: SeedCatalog.evChargingSites)
        XCTAssertFalse(outcome.stops.isEmpty)
        XCTAssertGreaterThan(outcome.totalCostCentavos, 0)
    }

    func testForumSafeLinksRefusesInsecureAndUserinfoUrls() {
        XCTAssertTrue(ForumSafeLinks.isAllowedHttpsLink("https://abastevo.com.br/guias/etanol-ou-gasolina/"))
        XCTAssertFalse(ForumSafeLinks.isAllowedHttpsLink("http://abastevo.com.br/"))
        XCTAssertFalse(ForumSafeLinks.isAllowedHttpsLink("https://user:pass@abastevo.com.br/"))
        XCTAssertFalse(ForumSafeLinks.isAllowedHttpsLink("javascript:alert(1)"))
    }

    func testArcadeGameStepAdvancesAndDetectsFuelDrain() {
        var state = ArcadeState(fuelPercent: 1.0)
        state.step(kind: .fuelRun, tick: 1)
        state.step(kind: .fuelRun, tick: 2)
        XCTAssertTrue(state.isGameOver)
    }

    func testLegacySampleEnterpriseDataIsDiscardedWithoutLosingUserPreferences() throws {
        let encoder = JSONEncoder()
        let original = PersistedAppSnapshot(termsAccepted: true, localeCode: "es")
        let originalData = try encoder.encode(original)
        var oldFields = try XCTUnwrap(JSONSerialization.jsonObject(with: originalData) as? [String: Any])
        oldFields["enterpriseOrganizations"] = [[
            "id": "sample-org", "name": "Sample organization", "cnpj": "sample",
            "roleTitle": "Driver", "status": "ACTIVE",
        ]]
        oldFields["activeOrganizationId"] = "sample-org"
        oldFields["enterpriseTrips"] = [] as [String]
        oldFields["enterpriseExpenses"] = [] as [String]
        oldFields["enterpriseChecklists"] = [] as [String]

        let oldData = try JSONSerialization.data(withJSONObject: oldFields)
        let restored = try JSONDecoder().decode(PersistedAppSnapshot.self, from: oldData)
        XCTAssertEqual(restored, original)

        let restoredData = try encoder.encode(restored)
        let restoredFields = try XCTUnwrap(JSONSerialization.jsonObject(with: restoredData) as? [String: Any])
        for sampleKey in ["enterpriseOrganizations", "activeOrganizationId", "enterpriseTrips", "enterpriseExpenses", "enterpriseChecklists"] {
            XCTAssertNil(restoredFields[sampleKey])
        }
    }

    func testLegacyManualShiftDoesNotCalculateUnrepresentableCounters() {
        let invalid = DriverShiftRecord(
            vehicleId: "legacy", startedAtLabel: "Agora", startOdometerKm: Int64.min,
            endOdometerKm: Int64.max, grossEarningsCentavos: Int64.min,
            fuelCostCentavos: Int64.max, isActive: false
        )
        XCTAssertNil(invalid.distanceKm)
        XCTAssertNil(invalid.netEarningsCentavos)

        let valid = DriverShiftRecord(
            vehicleId: "registered", startedAtLabel: "2026-11-23T11:00:00Z", startOdometerKm: 100,
            endOdometerKm: 123, grossEarningsCentavos: 500, fuelCostCentavos: 600, isActive: false
        )
        XCTAssertEqual(valid.distanceKm, 23)
        XCTAssertEqual(valid.netEarningsCentavos, -100)
    }
}
