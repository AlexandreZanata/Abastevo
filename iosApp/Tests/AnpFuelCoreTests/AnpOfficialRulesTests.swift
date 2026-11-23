import XCTest
@testable import AnpFuelCore

final class AnpOfficialRulesTests: XCTestCase {
    private let city = Municipality(ibgeCode: "5107925", name: "Sorriso", state: .MT)

    private func week(_ start: String, _ end: String) -> AnpSurveyWeekRef {
        AnpSurveyWeekRef(startIso: start, endIso: end)!
    }

    private func manifest(_ week: AnpSurveyWeekRef, scope: BrazilianState? = .MT) -> AnpWeekManifest {
        AnpWeekManifest(
            week: week,
            summarySource: AnpSourceFileDigest(url: "https://www.gov.br/anp/s.xlsx", sha256: "a", bytes: 10),
            stationSource: scope == nil ? nil : AnpSourceFileDigest(url: "https://www.gov.br/anp/r.xlsx", sha256: "b", bytes: 20),
            stationScope: scope,
            averageRowCount: 1,
            stationRowCount: scope == nil ? 0 : 1,
            quarantinedRowCount: 0,
            averagesFileSha256: "c",
            averagesFileBytes: 5,
            stationsFileSha256: scope == nil ? nil : "d",
            stationsFileBytes: scope == nil ? nil : 6,
            downloadedAtEpochSeconds: 1,
            publishedOnIso: nil,
            operationalNote: nil
        )
    }

    private func average(_ product: FuelProduct, avg: Int64?, min: Int64? = nil, municipality: String = "SORRISO") -> AnpMunicipalityAverage {
        AnpMunicipalityAverage(state: .MT, municipality: municipality, product: product, stationCount: 5, unit: "R$/l", averageMilli: avg, minimumMilli: min, maximumMilli: nil)
    }

    private func station(_ cnpj: String, _ product: FuelProduct, _ price: Int64) -> AnpStationWeekPrice {
        AnpStationWeekPrice(cnpj: cnpj, tradeName: "Posto", legalName: nil, brand: nil, address: "", municipality: "SORRISO", state: .MT, product: product, priceMilli: price, collectedOnIso: nil)
    }

    func testActiveWeekFallsBackToNewestDownloadAndStalenessUsesEightDays() {
        let older = week("2026-11-02", "2026-11-08")
        let newer = week("2026-11-09", "2026-11-15")
        let manifests = [manifest(older), manifest(newer)]
        XCTAssertEqual(AnpOfficialRules.latestWeek(manifests), newer)
        XCTAssertEqual(AnpOfficialRules.displayWeek(activeWeekId: older.id, manifests: manifests), older)
        XCTAssertEqual(AnpOfficialRules.displayWeek(activeWeekId: "2025-01-01_2025-01-07", manifests: manifests), newer)
        XCTAssertEqual(AnpOfficialRules.displayWeek(activeWeekId: "", manifests: manifests), newer)
        XCTAssertNil(AnpOfficialRules.displayWeek(activeWeekId: "", manifests: []))

        XCTAssertFalse(AnpOfficialRules.isStale(weekEndIso: "2026-11-15", todayIso: "2026-11-22"))
        XCTAssertTrue(AnpOfficialRules.isStale(weekEndIso: "2026-11-15", todayIso: "2026-11-23"))
        XCTAssertEqual(AnpOfficialRules.readiness(manifests: [], scope: .MT, todayIso: "2026-11-16"), .empty)
        XCTAssertEqual(AnpOfficialRules.readiness(manifests: manifests, scope: .MT, todayIso: "2026-11-16"), .ready)
        XCTAssertEqual(AnpOfficialRules.readiness(manifests: manifests, scope: .SP, todayIso: "2026-11-16"), .partial)
        XCTAssertEqual(AnpOfficialRules.readiness(manifests: [manifest(newer, scope: nil)], scope: .MT, todayIso: "2026-11-16"), .partial)
        XCTAssertEqual(AnpOfficialRules.readiness(manifests: manifests, scope: .MT, todayIso: "2026-11-30"), .stale)
    }

    func testCityAveragesAvailabilityAndHistory() {
        let w1 = week("2026-10-26", "2026-11-01")
        let w2 = week("2026-11-02", "2026-11-08")
        let w3 = week("2026-11-09", "2026-11-15")
        let rows = [
            AnpWeekRows(week: w2, averages: [average(.gasolineRegular, avg: 6_190), average(.ethanol, avg: 4_290)]),
            AnpWeekRows(week: w3, averages: [average(.gasolineRegular, avg: 6_290), average(.ethanol, avg: 4_190, municipality: "Sorriso")]),
            AnpWeekRows(week: w1, averages: [average(.ethanol, avg: 4_390)]),
        ]
        XCTAssertEqual(AnpOfficialRules.cityAverages(rows[0].averages, city: city).map(\.product), [.ethanol, .gasolineRegular])
        XCTAssertEqual(AnpOfficialRules.history(weeks: rows, city: city, product: .ethanol),
                       .available([
                           AnpCityHistoryEntry(week: w3, averageMilli: 4_190, minimumMilli: nil, maximumMilli: nil, stationCount: 5),
                           AnpCityHistoryEntry(week: w2, averageMilli: 4_290, minimumMilli: nil, maximumMilli: nil, stationCount: 5),
                           AnpCityHistoryEntry(week: w1, averageMilli: 4_390, minimumMilli: nil, maximumMilli: nil, stationCount: 5),
                       ]))
        XCTAssertEqual(AnpOfficialRules.history(weeks: [rows[0]], city: city, product: .ethanol), .insufficient(weeks: 1))
        XCTAssertEqual(AnpOfficialRules.history(weeks: rows, city: city, product: .cng), .insufficient(weeks: 0))

        let cuiaba = Municipality(ibgeCode: "5103403", name: "Cuiabá", state: .MT)
        XCTAssertEqual(AnpOfficialRules.availability(displayWeek: rows[1], otherWeeks: [rows[0]], city: city), .hasData)
        XCTAssertEqual(AnpOfficialRules.availability(displayWeek: rows[1], otherWeeks: [rows[0]], city: cuiaba), .neverInAnp)
        let cuiabaOnlyBefore = AnpWeekRows(week: w1, averages: [average(.ethanol, avg: 4_000, municipality: "CUIABA")])
        XCTAssertEqual(AnpOfficialRules.availability(displayWeek: rows[1], otherWeeks: [cuiabaOnlyBefore], city: cuiaba), .noDataThisWeek)
    }

    func testTrendBarsFollowAndroidScaling() {
        XCTAssertEqual(AnpOfficialRules.trendBarFractions([]), [])
        XCTAssertEqual(AnpOfficialRules.trendBarFractions([5_000, 5_000]), [0.15, 0.15])
        let bars = AnpOfficialRules.trendBarFractions([3_400, 3_520, 3_460])
        XCTAssertEqual(bars[0], 0.15, accuracy: 1e-9)
        XCTAssertEqual(bars[1], 1.0, accuracy: 1e-9)
        XCTAssertEqual(bars[2], 0.15 + 0.5 * 0.85, accuracy: 1e-9)
    }

    func testPreviousWeekRule() {
        let w1 = week("2026-10-26", "2026-11-01")
        let w2 = week("2026-11-02", "2026-11-08")
        let w3 = week("2026-11-09", "2026-11-15")
        XCTAssertEqual(AnpOfficialRules.previousWeek(current: w3, weeks: [w1, w3, w2, w3]), w2)
        XCTAssertNil(AnpOfficialRules.previousWeek(current: w1, weeks: [w1, w2]))
        let missing = week("2026-11-16", "2026-11-22")
        XCTAssertEqual(AnpOfficialRules.previousWeek(current: missing, weeks: [w1, w3]), w3)
    }

    /// BR-025: cheapest station of the city, else the city minimum; once per alert and week.
    func testPriceDropUsesCheapestStationThenCityMinimum() {
        let previous = AnpWeekRows(
            week: week("2026-11-02", "2026-11-08"),
            averages: [average(.gasolineRegular, avg: 6_300, min: 6_200), average(.ethanol, avg: 4_300, min: 4_100)],
            stations: [station("11111111000111", .gasolineRegular, 6_150), station("22222222000122", .gasolineRegular, 6_250)]
        )
        let current = AnpWeekRows(
            week: week("2026-11-09", "2026-11-15"),
            averages: [average(.gasolineRegular, avg: 6_200, min: 6_100), average(.ethanol, avg: 4_400, min: 4_200)],
            stations: [station("11111111000111", .gasolineRegular, 6_090)]
        )
        XCTAssertEqual(AnpOfficialRules.comparisonPrice(rows: current, city: city, product: .gasolineRegular), 6_090)
        XCTAssertEqual(AnpOfficialRules.comparisonPrice(rows: current, city: city, product: .ethanol), 4_200)
        XCTAssertNil(AnpOfficialRules.comparisonPrice(rows: current, city: city, product: .cng))

        let drops = AnpOfficialRules.priceDrops(
            products: [.gasolineRegular, .ethanol, .cng], city: city, current: current, previous: previous, notifiedWeekByAlert: [:]
        )
        XCTAssertEqual(drops, [AnpPriceDrop(
            alertKey: AnpOfficialRules.alertKey(.gasolineRegular), product: .gasolineRegular,
            weekId: current.week.id, currentMilli: 6_090, previousMilli: 6_150
        )])
        XCTAssertTrue(AnpOfficialRules.priceDrops(
            products: [.gasolineRegular], city: city, current: current, previous: previous,
            notifiedWeekByAlert: [AnpOfficialRules.alertKey(.gasolineRegular): current.week.id]
        ).isEmpty)
        XCTAssertTrue(AnpOfficialRules.priceDrops(products: [.gasolineRegular], city: city, current: current, previous: nil, notifiedWeekByAlert: [:]).isEmpty)
        XCTAssertFalse(AnpOfficialRules.shouldNotify(current: 6_000, previous: 6_000))
        XCTAssertFalse(AnpOfficialRules.shouldNotify(current: nil, previous: 6_000))
    }

    func testAutomaticWeeklyCheckAndStationMatchByCnpj() {
        XCTAssertTrue(AnpOfficialRules.shouldRunAutomaticCheck(lastCheckEpochSeconds: nil, nowEpochSeconds: 100))
        XCTAssertFalse(AnpOfficialRules.shouldRunAutomaticCheck(lastCheckEpochSeconds: 100, nowEpochSeconds: 100 + 6 * 86_400))
        XCTAssertTrue(AnpOfficialRules.shouldRunAutomaticCheck(lastCheckEpochSeconds: 100, nowEpochSeconds: 100 + 7 * 86_400))
        XCTAssertTrue(AnpOfficialRules.shouldRunAutomaticCheck(lastCheckEpochSeconds: 1_000, nowEpochSeconds: 10))

        let rows = AnpWeekRows(week: week("2026-11-09", "2026-11-15"), averages: [], stations: [
            station("04218406000104", .ethanol, 4_190), station("04218406000104", .gasolineRegular, 6_190),
        ])
        XCTAssertEqual(AnpOfficialRules.stationPrices(rows: rows, cnpj: "04.218.406/0001-04").map(\.product), [.ethanol, .gasolineRegular])
        XCTAssertTrue(AnpOfficialRules.stationPrices(rows: rows, cnpj: "").isEmpty)
        let prices = AnpOfficialRules.officialPrices(AnpOfficialRules.stationPrices(rows: rows, cnpj: "04218406000104"), week: rows.week)
        XCTAssertEqual(prices.first?.collectedOn, "2026-11-09")
        XCTAssertEqual(prices.first?.surveyEnd, "2026-11-15")
    }
}
