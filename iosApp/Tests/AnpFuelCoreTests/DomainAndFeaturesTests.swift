import XCTest
@testable import AnpFuelCore

final class DomainAndFeaturesTests: XCTestCase {
    func testPremiumEnergyVectorsMatchGoldenCorpus() {
        let c1 = PremiumEnergyCalculator.compute(
            PremiumEnergyCalculator.IntervalInput(
                purchasedWh: 40_000,
                costCentavos: 4_000,
                distanceMetres: 150_000,
                hasEnergy: true,
                complete: true
            )
        )
        XCTAssertEqual(c1.whPerKm, "800/3")
        XCTAssertEqual(c1.centavosPerKm, "80/3")
        XCTAssertEqual(c1.unknown, "")

        let c2 = PremiumEnergyCalculator.compute(
            PremiumEnergyCalculator.IntervalInput(
                purchasedWh: 15_000,
                costCentavos: 1_275,
                distanceMetres: 100_000,
                hasEnergy: true,
                complete: true
            )
        )
        XCTAssertEqual(c2.whPerKm, "150")
        XCTAssertEqual(c2.centavosPerKm, "51/4")

        let c3 = PremiumEnergyCalculator.compute(
            PremiumEnergyCalculator.IntervalInput(
                purchasedWh: 15_000,
                costCentavos: 1_275,
                distanceMetres: 100_000,
                hasEnergy: true,
                complete: true,
                mixedCarrier: true
            )
        )
        XCTAssertEqual(c3.unknown, "phev-shared-distance")
    }

    func testTrainingTermsSha256MatchesExactBackendContract() {
        XCTAssertEqual(TrainingTermsContract.version, "training-terms-v1")
        XCTAssertTrue(TrainingTermsContract.verifiedDigestMatches())
    }

    func testMunicipalitySearchIsDiacriticInsensitive() {
        let matches = Municipality.rankMatches(query: "sao paulo", stateFilter: nil, catalog: SeedCatalog.municipalities)
        XCTAssertEqual(matches.first?.ibgeCode, "3550308")
        let sorriso = Municipality.rankMatches(query: "sorriso", stateFilter: .MT, catalog: SeedCatalog.municipalities)
        XCTAssertEqual(sorriso.first?.ibgeCode, "5107925")
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
}
