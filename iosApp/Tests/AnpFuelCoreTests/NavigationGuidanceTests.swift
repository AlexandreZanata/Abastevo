import Foundation
import XCTest
@testable import AnpFuelCore

final class NavigationGuidanceTests: XCTestCase {
    private let epoch = Date(timeIntervalSince1970: 10_000)

    private func tick(_ seconds: UInt64) -> UInt64 { seconds * 1_000_000_000 }

    private func point(_ east: Double, _ north: Double = 0, label: String = "") -> EvRoutePoint {
        EvRoutePoint(latitude: north / 111_194.9266, longitude: east / 111_194.9266, label: label)
    }

    private func fix(
        _ east: Double, _ north: Double = 0, seconds: UInt64, accuracy: Double = 5, age: TimeInterval = 0
    ) -> NavigationGuidanceFix? {
        let received = epoch.addingTimeInterval(Double(seconds))
        return NavigationGuidanceFix(point: point(east, north), accuracyMeters: accuracy,
                                     timestamp: received.addingTimeInterval(-age), receivedAt: received,
                                     receivedUptimeNanos: tick(seconds))
    }

    private func route(
        input: NavigationGuidanceInput? = nil, geometry: [EvRoutePoint]? = nil, distance: Double = 1_200
    ) throws -> NavigationGuidanceRoute {
        let input = input ?? NavigationGuidanceInput(origin: point(0), destination: point(1_000))
        return try XCTUnwrap(NavigationGuidanceRoute(
            input: input, geometry: geometry ?? [input.origin, input.destination], distanceMeters: distance,
            expectedTravelTime: 120, steps: [
                NavigationGuidanceStep(instruction: "", distanceMeters: 300),
                NavigationGuidanceStep(instruction: "First turn", distanceMeters: 400),
                NavigationGuidanceStep(instruction: "Second turn", distanceMeters: 300),
            ]
        ))
    }

    private func run() throws -> NavigationGuidance {
        let route = try route()
        return try XCTUnwrap(NavigationGuidance(route: route, input: route.input))
    }

    private func deviation(in run: inout NavigationGuidance, startingAt first: UInt64 = 20) throws -> NavigationGuidanceReroute {
        for elapsed in 0..<2 {
            let now = first + UInt64(elapsed)
            let update = run.ingest(fix(100, 120, seconds: now), nowNanos: tick(now))
            XCTAssertEqual(update.state, .offRoute)
            XCTAssertNil(update.reroute)
            XCTAssertNil(update.speech)
        }
        let now = first + 2
        let update = run.ingest(fix(100, 120, seconds: now), nowNanos: tick(now))
        XCTAssertEqual(update.state, .rerouting)
        return try XCTUnwrap(update.reroute)
    }

    func testAdmissionRejectsInvalidCoordinatesAccuracyAndUTCWithInclusiveAccuracyAndAge() throws {
        let exact = try XCTUnwrap(fix(0, seconds: 20, accuracy: 50, age: 15))
        XCTAssertEqual(exact.sampledAtNanos, tick(5))
        XCTAssertTrue(NavigationGuidancePolicy.accepts(exact, nowNanos: tick(20)))
        XCTAssertFalse(NavigationGuidancePolicy.accepts(exact, nowNanos: tick(20) + 1))
        XCTAssertFalse(NavigationGuidancePolicy.accepts(exact, nowNanos: tick(5) - 1))
        for accuracy in [-1, 0, 50.01, Double.nan, .infinity] {
            XCTAssertNil(fix(0, seconds: 20, accuracy: accuracy))
        }
        XCTAssertNil(fix(0, seconds: 20, age: -1))
        XCTAssertNil(fix(0, seconds: 20, age: 15.01))
        XCTAssertNil(fix(0, seconds: 15, age: 15))
        XCTAssertNil(fix(.nan, seconds: 20))
        XCTAssertNil(fix(0, .infinity, seconds: 20))
        for (latitude, longitude) in [(91.0, 0.0), (0.0, -181.0), (Double.infinity, 0.0)] {
            XCTAssertNil(NavigationGuidanceFix(point: EvRoutePoint(latitude: latitude, longitude: longitude),
                                               accuracyMeters: 5, timestamp: epoch, receivedAt: epoch,
                                               receivedUptimeNanos: tick(20)))
        }
        let atEpoch = Date(timeIntervalSince1970: 0)
        XCTAssertNil(NavigationGuidanceFix(point: point(0), accuracyMeters: 5, timestamp: atEpoch,
                                           receivedAt: atEpoch, receivedUptimeNanos: tick(20)))
    }

    func testRouteBindsExactRequestedCoordinatesAndRequiresProviderGeometryAndMetric() throws {
        let route = try route()
        let relabeled = NavigationGuidanceInput(origin: point(0, label: "Origin"), destination: point(1_000, label: "Destination"))
        XCTAssertNotNil(NavigationGuidance(route: route, input: relabeled))
        XCTAssertNil(NavigationGuidance(route: route, input: NavigationGuidanceInput(origin: point(1), destination: point(1_000))))
        XCTAssertNil(NavigationGuidance(route: route, input: NavigationGuidanceInput(origin: point(1_000), destination: point(0))))
        for geometry in [[point(0)], [point(0), point(0)], [point(0), point(0, .nan)]] {
            XCTAssertNil(NavigationGuidanceRoute(input: route.input, geometry: geometry, distanceMeters: 1_200,
                                                 expectedTravelTime: 120, steps: []))
        }
        XCTAssertNil(NavigationGuidanceRoute(input: route.input, geometry: route.geometry, distanceMeters: .nan,
                                             expectedTravelTime: 120, steps: []))
        XCTAssertNil(NavigationGuidanceRoute(input: route.input, geometry: route.geometry, distanceMeters: 1_200,
                                             expectedTravelTime: 0, steps: []))
        XCTAssertEqual(NavigationGuidancePolicy.nativeLocaleFor("pt"), "pt-BR")
        XCTAssertEqual(NavigationGuidancePolicy.nativeLocaleFor("zh_CN"), "zh-Hans")
        XCTAssertEqual(NavigationGuidancePolicy.nativeLocaleFor("en-CA"), "en")
        XCTAssertNil(NavigationGuidancePolicy.nativeLocaleFor("zh-TW"))
        XCTAssertNil(NavigationGuidancePolicy.nativeLocaleFor("xx"))
    }

    func testProviderDistanceAndTurnProgressNeverAdvanceForPoorOrRepeatedFix() throws {
        var run = try run()
        let start = run.ingest(fix(0, seconds: 20), nowNanos: tick(20))
        XCTAssertEqual(start.state, .following)
        XCTAssertEqual(start.progress?.remainingMeters, 1_200)
        XCTAssertEqual(start.progress?.remainingSeconds, 120)
        XCTAssertEqual(start.progress?.nextManeuver?.instruction, "First turn")
        XCTAssertNil(start.speech)
        let next = run.ingest(fix(120, seconds: 22), nowNanos: tick(22))
        XCTAssertEqual(next.speech?.instruction, "First turn")
        XCTAssertEqual(try XCTUnwrap(next.progress?.remainingMeters), 1_056, accuracy: 2)
        let duplicate = run.ingest(fix(250, seconds: 22), nowNanos: tick(22))
        XCTAssertEqual(duplicate.progress, next.progress)
        XCTAssertNil(duplicate.speech)
        let reordered = run.ingest(fix(290, seconds: 23, age: 2), nowNanos: tick(23))
        XCTAssertEqual(reordered.progress, next.progress)

        let weak = run.ingest(fix(280, seconds: 24, accuracy: 75), nowNanos: tick(24))
        XCTAssertEqual(weak.state, .weakSignal)
        XCTAssertEqual(weak.progress, next.progress)
        XCTAssertNil(weak.speech)
        let recovered = run.ingest(fix(195, seconds: 26), nowNanos: tick(26))
        XCTAssertEqual(recovered.state, .following)
        XCTAssertNil(recovered.speech)
        XCTAssertEqual(try XCTUnwrap(recovered.progress?.remainingMeters), 966, accuracy: 2)
        XCTAssertEqual(run.advance(nowNanos: tick(41)).state, .following)
        let silent = run.advance(nowNanos: tick(41) + 1)
        XCTAssertEqual(silent.state, .weakSignal)
        XCTAssertEqual(silent.progress, recovered.progress)
    }

    func testAccuracyCorridorAndThreeConsecutiveDistinctOffRouteFixes() throws {
        var run = try run()
        XCTAssertEqual(run.ingest(fix(0, seconds: 20), nowNanos: tick(20)).state, .following)
        XCTAssertEqual(run.ingest(fix(50, 60, seconds: 21, accuracy: 40), nowNanos: tick(21)).state, .following)
        XCTAssertEqual(run.ingest(fix(70, 60, seconds: 22), nowNanos: tick(22)).state, .offRoute)
        XCTAssertEqual(run.ingest(fix(75, 65, seconds: 22), nowNanos: tick(22)).state, .offRoute)
        XCTAssertEqual(run.ingest(nil, nowNanos: tick(23)).state, .weakSignal)
        let request = try deviation(in: &run, startingAt: 24)
        XCTAssertEqual(request.input.origin, point(100, 120))
        XCTAssertEqual(request.input.destination, point(1_000))
        XCTAssertEqual(request.startedAtNanos, tick(26))
        XCTAssertEqual(run.pendingReroute, request)
        XCTAssertNil(run.ingest(fix(110, 130, seconds: 27), nowNanos: tick(27)).reroute)
    }

    func testRerouteDeadlineAndFailureRecoveryRemainIndependentOfGPSCallbacks() throws {
        var run = try run()
        let request = try deviation(in: &run)
        XCTAssertFalse(NavigationGuidancePolicy.isRerouteExpired(startedAtNanos: 100, nowNanos: 50))
        XCTAssertEqual(run.advance(nowNanos: request.startedAtNanos + tick(60)).state, .rerouting)
        let late = request.startedAtNanos + tick(60) + 1
        XCTAssertEqual(run.advance(nowNanos: late).state, .rerouteFailed)
        let replacement = try route(input: request.input)
        XCTAssertEqual(run.completeReroute(request, route: replacement, nowNanos: late).state, .rerouteFailed)
        XCTAssertNil(run.retryReroute(nowNanos: late).reroute)
        XCTAssertEqual(run.ingest(fix(100, 120, seconds: 83), nowNanos: tick(83)).state, .rerouteFailed)
        let retry = try XCTUnwrap(run.retryReroute(nowNanos: tick(83)).reroute)
        XCTAssertNotEqual(retry.id, request.id)
        XCTAssertEqual(retry.input, request.input)
        XCTAssertEqual(run.completeReroute(request, route: replacement, nowNanos: tick(83)).state, .rerouting)
        XCTAssertEqual(run.completeReroute(retry, route: nil, nowNanos: tick(84)).state, .rerouteFailed)
        let latest = try XCTUnwrap(run.retryReroute(nowNanos: tick(84)).reroute)
        XCTAssertEqual(run.completeReroute(latest, route: replacement, nowNanos: tick(85)).state, .following)
        XCTAssertEqual(run.route.input, latest.input)
        XCTAssertNil(run.pendingReroute)
    }

    func testSameRouteRecoveryCancelsRequestAndWrongRouteCannotReplaceCurrentGeometry() throws {
        var run = try run()
        let original = run.route
        let request = try deviation(in: &run)
        let regained = run.ingest(fix(100, seconds: 23), nowNanos: tick(23))
        XCTAssertEqual(regained.state, .following)
        XCTAssertNil(run.pendingReroute)
        let replacement = try route(input: request.input)
        XCTAssertEqual(run.completeReroute(request, route: replacement, nowNanos: tick(24)).state, .following)
        XCTAssertEqual(run.route, original)
        let next = try deviation(in: &run, startingAt: 25)
        XCTAssertEqual(run.completeReroute(next, route: original, nowNanos: tick(29)).state, .rerouteFailed)
        XCTAssertEqual(run.route, original)
    }

    func testArrivalRequiresConsecutiveAccurateFixesNearProviderEndpoint() throws {
        var run = try run()
        XCTAssertEqual(run.ingest(fix(0, seconds: 20), nowNanos: tick(20)).state, .following)
        XCTAssertEqual(run.ingest(fix(985, seconds: 34, accuracy: 30), nowNanos: tick(34)).state, .following)
        XCTAssertEqual(run.ingest(fix(991, seconds: 35), nowNanos: tick(35)).state, .following)
        XCTAssertEqual(run.ingest(fix(990, 25, seconds: 36), nowNanos: tick(36)).state, .following)
        XCTAssertEqual(run.ingest(fix(994, seconds: 37), nowNanos: tick(37)).state, .following)
        let arrived = run.ingest(fix(999, seconds: 38), nowNanos: tick(38))
        XCTAssertEqual(arrived.state, .arrived)
        XCTAssertEqual(arrived.progress?.remainingMeters, 0)
        XCTAssertNil(arrived.speech)
        XCTAssertNil(arrived.reroute)
        XCTAssertEqual(run.ingest(nil, nowNanos: tick(89)).state, .arrived)
    }

    func testMatchingNearPriorProgressAvoidsJumpOntoParallelReturnLeg() throws {
        let input = NavigationGuidanceInput(origin: point(0), destination: point(10, 10))
        let route = try route(input: input, geometry: [point(0), point(400), point(400, 10), point(10, 10)], distance: 800)
        var run = try XCTUnwrap(NavigationGuidance(route: route, input: input))
        XCTAssertEqual(run.ingest(fix(0, seconds: 20), nowNanos: tick(20)).state, .following)
        let projected = run.ingest(fix(95, 9, seconds: 22), nowNanos: tick(22))
        XCTAssertEqual(projected.state, .following)
        XCTAssertGreaterThan(try XCTUnwrap(projected.progress?.remainingMeters), 680)
    }
}
