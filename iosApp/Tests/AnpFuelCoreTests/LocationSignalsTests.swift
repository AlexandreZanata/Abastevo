import XCTest
@testable import AnpFuelCore

/// Swift unit tests for native iOS location signals (P16-T02).
///
/// Mirrors the Android boundary matrix: denied short-circuit, absent
/// fix, verbatim real-fix mapping, negative accuracy folding,
/// simulation passthrough, missing source info (older iOS) staying
/// unavailable, and unknown age reaching the contract.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class LocationSignalsTests: XCTestCase {
    func testDeniedShortCircuits() {
        let signal = LocationReading.fromFix(
            permissionGranted: false,
            isSimulatedBySoftware: false,
            horizontalAccuracy: nil,
            fixAgeSeconds: nil
        )
        XCTAssertFalse(signal.permissionGranted)
        XCTAssertFalse(signal.sourceInfoAvailable)
    }

    func testMapsRealFixVerbatim() {
        let signal = LocationReading.fromFix(
            permissionGranted: true,
            isSimulatedBySoftware: false,
            horizontalAccuracy: 25.0,
            fixAgeSeconds: 30
        )
        XCTAssertTrue(signal.permissionGranted)
        XCTAssertTrue(signal.sourceInfoAvailable)
        XCTAssertFalse(signal.simulated)
        XCTAssertTrue(signal.hasAccuracy)
        XCTAssertEqual(signal.accuracyMeters, 25.0)
        XCTAssertEqual(signal.fixAgeSeconds, 30)
    }

    func testNegativeAccuracyFoldsToMissing() {
        let signal = LocationReading.fromFix(
            permissionGranted: true,
            isSimulatedBySoftware: false,
            horizontalAccuracy: -1.0,
            fixAgeSeconds: 5
        )
        XCTAssertFalse(signal.hasAccuracy)
    }

    func testSimulationFlagPassesThroughUntouched() {
        let signal = LocationReading.fromFix(
            permissionGranted: true,
            isSimulatedBySoftware: true,
            horizontalAccuracy: 10.0,
            fixAgeSeconds: 5
        )
        XCTAssertTrue(signal.simulated)
        XCTAssertTrue(signal.sourceInfoAvailable)
    }

    func testMissingSourceInfoStaysUnavailable() {
        let signal = LocationReading.fromFix(
            permissionGranted: true,
            isSimulatedBySoftware: nil,
            horizontalAccuracy: 10.0,
            fixAgeSeconds: 5
        )
        XCTAssertFalse(signal.sourceInfoAvailable)
        XCTAssertFalse(signal.simulated)
    }

    func testUnknownAgePassesThroughForContract() {
        let signal = LocationReading.fromFix(
            permissionGranted: true,
            isSimulatedBySoftware: false,
            horizontalAccuracy: 25.0,
            fixAgeSeconds: nil
        )
        XCTAssertNil(signal.fixAgeSeconds)
    }
}
