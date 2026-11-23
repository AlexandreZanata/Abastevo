import XCTest
@testable import AnpFuelCore

/// Straight-line stand-in for MapKit in tests; production legs always come from a road router.
private struct LineRouter: EvRoadRouter {
    var unavailable = false

    func leg(points: [EvRoutePoint]) async -> EvRoadLeg {
        guard !unavailable, points.count >= 2, let first = points.first, let last = points.last else {
            return .unavailable(.noRoute)
        }
        let meters = EvRouteGeometry.distanceMeters(first.latitude, first.longitude, last.latitude, last.longitude)
        return .ready(distanceMeters: Int64(meters.rounded()), durationSeconds: Int64(meters / 20), warnings: false, geometry: points)
    }
}

final class EvTripPlanningTests: XCTestCase {
    private func render(_ estimate: EnergyEstimate) -> String {
        switch estimate {
        case .known(let value): return String(value)
        case .unknown(let reason): return "unknown:" + reason
        }
    }

    /// Replays `contracts/testdata/ev/trip-energy-v1.csv` exactly like Android `EvEnergyModelTest`.
    func testSharedEnergyVectors() throws {
        let file = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("contracts/testdata/ev/trip-energy-v1.csv")
        let lines = try String(contentsOf: file, encoding: .utf8)
            .split(whereSeparator: \.isNewline).map(String.init).filter { !$0.isEmpty }
        let header = lines[0].split(separator: ",", omittingEmptySubsequences: false).map(String.init)
        XCTAssertEqual(lines[0], "id,operation,capacity_wh,soc_bp,reserve_bp,usable_permille,cons_milli,distance_m,margin_permille,power_w,energy_wh,sessions,expected")
        let rows = lines.dropFirst().map { line -> [String: String] in
            let cells = line.split(separator: ",", omittingEmptySubsequences: false).map(String.init)
            return Dictionary(uniqueKeysWithValues: zip(header, cells))
        }
        XCTAssertEqual(rows.count, 53)
        for row in rows {
            func cell(_ key: String) -> Int64? { row[key].flatMap { Int64($0) } }
            let battery = EvBatteryInput(capacityWh: cell("capacity_wh"), socBp: cell("soc_bp"), reserveBp: cell("reserve_bp"), usablePermille: cell("usable_permille"))
            let consumption = EvConsumptionInput(consMilliWhPerKm: cell("cons_milli"), marginPermille: cell("margin_permille"))
            let result: EnergyEstimate
            switch row["operation"] ?? "" {
            case "USABLE": result = EvEnergyModel.usableEnergyWh(battery)
            case "SEGMENT": result = EvEnergyModel.segmentEnergyWh(distanceMetres: cell("distance_m"), consumption: consumption)
            case "REACH": result = EvEnergyModel.reachableDistanceMetres(battery: battery, consumption: consumption)
            case "ARRIVAL": result = EvEnergyModel.arrivalSocBp(capacityWh: cell("capacity_wh"), socBp: cell("soc_bp"), distanceMetres: cell("distance_m"), consumption: consumption)
            case "CHARGE_LB": result = EvEnergyModel.minimumChargeSeconds(energyWh: cell("energy_wh"), declaredPowerW: cell("power_w"))
            case "CALIBRATE":
                let sessions = (row["sessions"] ?? "").split(separator: ";").compactMap { pair -> EvCalibrationSession? in
                    let parts = pair.split(separator: ":")
                    guard parts.count == 2, let energy = Int64(parts[0]), let distance = Int64(parts[1]) else { return nil }
                    return EvCalibrationSession(energyWh: energy, distanceMetres: distance)
                }
                result = EvEnergyModel.calibrateConsumptionMilli(sessions)
            default:
                XCTFail("unknown operation \(row["operation"] ?? "")")
                continue
            }
            XCTAssertEqual(render(result), row["expected"], row["id"] ?? "")
        }
    }

    func testPlannerInputsParseLikeAndroid() {
        XCTAssertEqual(EvPlannerInputParser.percentBp("30", min: 0, max: 100), 3_000)
        XCTAssertEqual(EvPlannerInputParser.percentBp("30,5", min: 0, max: 100), 3_050)
        XCTAssertNil(EvPlannerInputParser.percentBp("30.555", min: 0, max: 100))
        XCTAssertNil(EvPlannerInputParser.percentBp("101", min: 0, max: 100))
        XCTAssertNil(EvPlannerInputParser.percentBp("-1", min: 0, max: 100))
        XCTAssertNil(EvPlannerInputParser.percentBp("", min: 0, max: 100))
        XCTAssertEqual(EvPlannerInputParser.consumptionMilli("180"), 180_000)
        XCTAssertEqual(EvPlannerInputParser.consumptionMilli("165.5"), 165_500)
        XCTAssertNil(EvPlannerInputParser.consumptionMilli("0"))
        XCTAssertNil(EvPlannerInputParser.consumptionMilli("150.1234"))
    }

    private let origin = EvRoutePoint(latitude: 0, longitude: 0, label: "A")
    private let destination = EvRoutePoint(latitude: 0, longitude: 1.8, label: "B")

    private func input(soc: Int64, connector: ChargingConnector? = .ccs2, target: Int64 = 8_000) -> EvTripInput {
        EvTripInput(
            origin: origin, destination: destination, departureAt: nil,
            battery: EvBatteryInput(capacityWh: 50_000, socBp: soc, reserveBp: 1_000),
            consumption: EvConsumptionInput(consMilliWhPerKm: 150_000), vehicleConnector: connector, chargeTargetSocBp: target
        )
    }

    private func candidate(longitude: Double, access: EvAccess = .publicAccess, connector: ChargingConnector = .ccs2) -> EvChargingCandidate {
        EvChargingCandidate(
            candidateId: "g-\(longitude)", siteId: "s-\(longitude)", siteName: "Carregador \(longitude)",
            equipmentId: "e-\(longitude)", connectorId: "g-\(longitude)", connector: connector, current: .dc,
            powerW: 100_000, access: access, sourceRevision: "rev", point: EvCoordinate(latitude: 0, longitude: longitude)
        )
    }

    func testReachableTripNeedsNoStop() async {
        let plan = await EvTripPlanner(router: LineRouter()).plan(input(soc: 9_000)) { _ in
            EvCandidateResult(candidates: [], coverage: .present)
        }
        XCTAssertEqual(plan.status, .planned)
        XCTAssertTrue(plan.stops.isEmpty)
        XCTAssertEqual(plan.arrivalSocBp, .known(2_995))
        XCTAssertEqual(plan.mainRoute?.road.distanceMeters, 200_151)
    }

    func testLongTripStopsAtTheFarthestReachableAdmittedCharger() async throws {
        let charger = candidate(longitude: 0.9)
        let plan = await EvTripPlanner(router: LineRouter()).plan(input(soc: 5_000)) { _ in
            EvCandidateResult(candidates: [charger], coverage: .present, sourceRevision: "rev")
        }
        XCTAssertEqual(plan.status, .planned)
        let stop = try XCTUnwrap(plan.stops.first)
        XCTAssertEqual(plan.stops.count, 1)
        XCTAssertEqual(stop.candidate.siteId, charger.siteId)
        XCTAssertEqual(stop.arrivalSocBp, 1_997)
        XCTAssertEqual(stop.chargeEnergyWh, 30_015)
        XCTAssertEqual(stop.minimumChargeSeconds, .known(1_080))
        XCTAssertEqual(stop.estimatedCostMilliBrl, .unknown("tariff-unknown"))
        XCTAssertEqual(plan.arrivalSocBp, .known(4_997))
        XCTAssertTrue(plan.issues.contains(EvIssue(.noEvidencedAlternative, candidateId: charger.candidateId)))
        XCTAssertEqual(plan.sourceRevision, "rev")
    }

    func testPrivateOrIncompatibleChargersAreNeverStops() async {
        let blocked = [candidate(longitude: 0.9, access: .privateAccess), candidate(longitude: 1.0, connector: .type2)]
        let plan = await EvTripPlanner(router: LineRouter()).plan(input(soc: 5_000)) { _ in
            EvCandidateResult(candidates: blocked, coverage: .present)
        }
        XCTAssertEqual(plan.status, .noCandidates)
        XCTAssertTrue(plan.stops.isEmpty)
        XCTAssertTrue(plan.issues.contains(EvIssue(.accessPrivate, candidateId: blocked[0].candidateId)))
        XCTAssertTrue(plan.issues.contains(EvIssue(.incompatibleConnector, candidateId: blocked[1].candidateId)))
    }

    func testMissingCatalogRoadFailureAndInvalidInput() async {
        let missing = await EvTripPlanner(router: LineRouter()).plan(input(soc: 5_000)) { _ in
            EvCandidateResult(candidates: [], coverage: .missing)
        }
        XCTAssertEqual(missing.status, .insufficientData)
        XCTAssertTrue(missing.issues.contains(EvIssue(.snapshotMissing)))

        let noRoad = await EvTripPlanner(router: LineRouter(unavailable: true)).plan(input(soc: 5_000)) { _ in
            EvCandidateResult(candidates: [], coverage: .present)
        }
        XCTAssertEqual(noRoad.status, .noRoute)
        XCTAssertTrue(noRoad.issues.contains(EvIssue(.noRoadRoute)))

        let invalid = await EvTripPlanner(router: LineRouter()).plan(input(soc: 5_000, connector: nil, target: 500)) { _ in
            EvCandidateResult(candidates: [], coverage: .present)
        }
        XCTAssertEqual(invalid.status, .invalidInput)
        XCTAssertTrue(invalid.issues.contains(EvIssue(.vehicleConnectorMissing)))
        XCTAssertTrue(invalid.issues.contains(EvIssue(.invalidTargetSoc)))
        XCTAssertEqual(invalid.routeCalls, 0)
    }

    func testFailedPrimaryGivesNoEnergyOnReplan() async {
        let charger = candidate(longitude: 0.9)
        var replan = input(soc: 5_000)
        replan.initialPoint = EvRoutePoint(latitude: 0, longitude: 0.9, label: charger.siteName)
        replan.initialSocBp = 1_997
        replan.failures = [EvFailure(scope: .site, siteId: charger.siteId)]
        let plan = await EvTripPlanner(router: LineRouter()).plan(replan) { _ in
            EvCandidateResult(candidates: [charger], coverage: .present)
        }
        XCTAssertEqual(plan.status, .noCandidates)
        XCTAssertTrue(plan.issues.contains(EvIssue(.duplicateCandidate, candidateId: charger.candidateId)))
    }

    func testCandidateMapperKeepsAdmittedCoordinatesNearTheRoute() throws {
        let page = try ChargingWireDecoder.decodePage(EvCatalogTests.nationalPage())
        let route = [EvRoutePoint(latitude: -23.4, longitude: -46.6), EvRoutePoint(latitude: -23.7, longitude: -46.6)]
        let ccs = EvCandidateMapper.candidates(places: page.items, connector: .ccs2, sourceRevision: "rev", route: route, corridorRadiusMeters: 25_000, limit: 10)
        XCTAssertEqual(ccs.map(\.siteId), [EvCatalogTests.placeId])
        XCTAssertEqual(ccs.first?.powerW, 150_000)
        XCTAssertEqual(ccs.first?.access, .publicAccess)
        XCTAssertTrue(EvCandidateMapper.candidates(places: page.items, connector: .type2, sourceRevision: "rev", route: route, corridorRadiusMeters: 25_000, limit: 10).isEmpty)
        let far = [EvRoutePoint(latitude: -10, longitude: -40), EvRoutePoint(latitude: -10.5, longitude: -40)]
        XCTAssertTrue(EvCandidateMapper.candidates(places: page.items, connector: .ccs2, sourceRevision: "rev", route: far, corridorRadiusMeters: 25_000, limit: 10).isEmpty)
    }

    func testBestPerSiteAndEnergyToTarget() {
        let slow = EvChargingCandidate(
            candidateId: "a", siteId: "site", siteName: "Site", equipmentId: "e1", connectorId: "a", connector: .ccs2,
            current: .ac, powerW: 22_000, access: .publicAccess, sourceRevision: "r", point: EvCoordinate(latitude: 0, longitude: 0)
        )
        let fast = EvChargingCandidate(
            candidateId: "b", siteId: "site", siteName: "Site", equipmentId: "e2", connectorId: "b", connector: .ccs2,
            current: .dc, powerW: 150_000, access: .publicAccess, sourceRevision: "r", point: EvCoordinate(latitude: 0, longitude: 0)
        )
        XCTAssertEqual(EvChargingPlanRules.bestPerSite([slow, fast]).map(\.candidateId), ["b"])
        let battery = EvBatteryInput(capacityWh: 60_000, socBp: 2_000, reserveBp: 1_000)
        XCTAssertEqual(EvChargingPlanRules.energyToTargetWh(battery: battery, currentSocBp: 2_000, targetSocBp: 8_000), .known(36_000))
        XCTAssertEqual(EvChargingPlanRules.energyToTargetWh(battery: battery, currentSocBp: 8_000, targetSocBp: 8_000), .known(0))
        XCTAssertEqual(EvChargingPlanRules.chargeCostMilliBrl(energyWh: 36_000, tariffMilliBrlPerKwh: 2_150), .known(77_400))
    }
}
