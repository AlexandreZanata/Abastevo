import XCTest
@testable import AnpFuelCore

/// Swift unit tests for the portable location risk contract (P16-T01).
///
/// 1:1 transcription of `PortableLocationTest`: verdict matrix, inclusive
/// boundaries, forged-flag refusal, unknown-never-verified and precedence
/// rules. Golden vectors live in `contracts/testdata/location/risk-v1.json`
/// (replayed by Go).
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableLocationTests: XCTestCase {
    private func input(
        permissionGranted: Bool = true,
        hasFix: Bool = true,
        sourceInfoPresent: Bool = true,
        simulated: Bool = false,
        accuracyMeters: Double? = 25.0,
        fixAgeSeconds: Int64? = 30,
        clockSkewSeconds: Int64? = 10,
        manual: Bool = false
    ) -> PortableLocation.FixInput {
        return PortableLocation.FixInput(
            permissionGranted: permissionGranted,
            hasFix: hasFix,
            sourceInfoPresent: sourceInfoPresent,
            simulated: simulated,
            accuracyMeters: accuracyMeters,
            fixAgeSeconds: fixAgeSeconds,
            clockSkewSeconds: clockSkewSeconds,
            manual: manual
        )
    }

    func testBudgetsMirrorFrozenContract() {
        XCTAssertEqual(PortableLocation.policyVersion, "location-v1")
        XCTAssertEqual(PortableLocation.maxFixAgeSeconds, 120)
        XCTAssertEqual(PortableLocation.maxClockSkewSeconds, 300)
        XCTAssertEqual(PortableLocation.maxClaimAccuracyM, 100.0)
    }

    func testVerifiedOnlyOnFreshAccurateNonSimulatedSource() {
        let risk = PortableLocation.classify(input())
        XCTAssertEqual(risk.verdict, PortableLocation.verified)
        XCTAssertEqual(risk.reason, "")
        XCTAssertEqual(risk.freshness, PortableLocation.freshnessFresh)
        XCTAssertEqual(risk.accuracy, PortableLocation.accuracyAccurate)
        XCTAssertTrue(risk.allowsClaim)
    }

    func testBoundariesStayInclusive() {
        XCTAssertTrue(PortableLocation.classify(input(fixAgeSeconds: 120)).allowsClaim)
        XCTAssertTrue(PortableLocation.classify(input(accuracyMeters: 100.0)).allowsClaim)
        XCTAssertTrue(PortableLocation.classify(input(clockSkewSeconds: 300)).allowsClaim)
        XCTAssertTrue(PortableLocation.classify(input(clockSkewSeconds: -300)).allowsClaim)
    }

    func testSimulatedInputIsBlocked() {
        let risk = PortableLocation.classify(input(simulated: true))
        XCTAssertEqual(risk.verdict, PortableLocation.simulated)
        XCTAssertEqual(risk.reason, PortableLocation.reasonSimulatedSource)
        XCTAssertFalse(risk.allowsClaim)
    }

    func testMissingSourceInfoBlocksForgedClientFlag() {
        let risk = PortableLocation.classify(input(sourceInfoPresent: false, simulated: false))
        XCTAssertEqual(risk.verdict, PortableLocation.unknown)
        XCTAssertEqual(risk.reason, PortableLocation.reasonSourceMissing)
        XCTAssertFalse(risk.allowsClaim)
        let forged = PortableLocation.classify(input(sourceInfoPresent: false, simulated: true))
        XCTAssertEqual(forged.reason, PortableLocation.reasonSourceMissing)
        XCTAssertFalse(forged.allowsClaim)
    }

    func testUnknownCannotBecomeVerifiedProximity() {
        let cases: [(PortableLocation.FixInput, String)] = [
            (input(hasFix: false, accuracyMeters: nil, fixAgeSeconds: nil), PortableLocation.reasonNoFix),
            (input(accuracyMeters: nil), PortableLocation.reasonNoFix),
            (input(fixAgeSeconds: nil), PortableLocation.reasonNoFix),
            (input(fixAgeSeconds: 121), PortableLocation.reasonStaleFix),
            (input(fixAgeSeconds: -5), PortableLocation.reasonClockAnomaly),
            (input(clockSkewSeconds: 301), PortableLocation.reasonClockAnomaly),
        ]
        for (inputCase, reason) in cases {
            let risk = PortableLocation.classify(inputCase)
            XCTAssertEqual(risk.verdict, PortableLocation.unknown, reason)
            XCTAssertEqual(risk.reason, reason)
            XCTAssertFalse(risk.allowsClaim, reason)
        }
    }

    func testCoarseAccuracyIsDegradedNeverVerified() {
        let risk = PortableLocation.classify(input(accuracyMeters: 250.0))
        XCTAssertEqual(risk.verdict, PortableLocation.degraded)
        XCTAssertEqual(risk.reason, PortableLocation.reasonCoarseAccuracy)
        XCTAssertEqual(risk.accuracy, PortableLocation.accuracyCoarse)
        XCTAssertFalse(risk.allowsClaim)
    }

    func testPrecedenceNeverPromotesWeakerSignals() {
        let simCoarse = PortableLocation.classify(input(simulated: true, accuracyMeters: 500.0))
        XCTAssertEqual(simCoarse.verdict, PortableLocation.simulated)
        XCTAssertEqual(simCoarse.accuracy, PortableLocation.accuracyCoarse)
        let deniedStale = PortableLocation.classify(input(permissionGranted: false, fixAgeSeconds: 600))
        XCTAssertEqual(deniedStale.verdict, PortableLocation.denied)
        XCTAssertEqual(deniedStale.freshness, PortableLocation.freshnessStale)
    }
}
