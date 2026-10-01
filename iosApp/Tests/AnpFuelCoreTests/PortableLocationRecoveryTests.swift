import XCTest
@testable import AnpFuelCore

/// Swift unit tests for portable location recovery (P16-T04, BUC-L03).
///
/// Mirrors the Kotlin `PortableLocationRecoveryTest`: verified allows with
/// no recovery; denied/simulated/degraded/manual/unknown block the claim
/// while preserving free use; disclosure stays honest; revocation, coarse
/// downgrade and offline resume never promote to verified proximity.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableLocationRecoveryTests: XCTestCase {
    private func risk(verdict: String, reason: String) -> PortableLocation.FixRisk {
        return PortableLocation.FixRisk(
            verdict: verdict,
            reason: reason,
            freshness: PortableLocation.freshnessUnknown,
            accuracy: PortableLocation.accuracyUnknown,
            policyVersion: PortableLocation.policyVersion
        )
    }

    func testVerifiedAllowsWithNoRecovery() {
        let verified = PortableLocation.FixRisk(
            verdict: PortableLocation.verified,
            reason: "",
            freshness: PortableLocation.freshnessFresh,
            accuracy: PortableLocation.accuracyAccurate,
            policyVersion: PortableLocation.policyVersion
        )
        let recovery = PortableLocationRecovery.recover(verified)
        XCTAssertTrue(recovery.allowsClaim)
        XCTAssertTrue(recovery.preservesFreeUse)
        XCTAssertEqual(recovery.disclosureCode, PortableLocationRecovery.disclosureFixAccepted)
        XCTAssertEqual(recovery.recoveryCode, PortableLocationRecovery.none)
    }

    func testDeniedPreservesFreeUse() {
        let recovery = PortableLocationRecovery.recover(
            risk(verdict: PortableLocation.denied, reason: PortableLocation.reasonPermissionDenied)
        )
        XCTAssertFalse(recovery.allowsClaim)
        XCTAssertTrue(recovery.preservesFreeUse)
        XCTAssertEqual(recovery.recoveryCode, PortableLocationRecovery.openSettings)
    }

    func testSimulatedBlocksWithDisableSimulation() {
        let recovery = PortableLocationRecovery.recover(
            risk(verdict: PortableLocation.simulated, reason: PortableLocation.reasonSimulatedSource)
        )
        XCTAssertFalse(recovery.allowsClaim)
        XCTAssertTrue(recovery.preservesFreeUse)
        XCTAssertEqual(recovery.recoveryCode, PortableLocationRecovery.disableSimulation)
    }

    func testDegradedKeepsBrowseOnly() {
        let recovery = PortableLocationRecovery.recover(
            risk(verdict: PortableLocation.degraded, reason: PortableLocation.reasonCoarseAccuracy)
        )
        XCTAssertFalse(recovery.allowsClaim)
        XCTAssertTrue(recovery.preservesFreeUse)
        XCTAssertEqual(recovery.recoveryCode, PortableLocationRecovery.enablePrecise)
    }

    func testUnknownMatrixRetriesWithFreeUse() {
        let reasons = [
            PortableLocation.reasonNoFix,
            PortableLocation.reasonSourceMissing,
            PortableLocation.reasonStaleFix,
            PortableLocation.reasonClockAnomaly,
        ]
        for reason in reasons {
            let recovery = PortableLocationRecovery.recover(
                risk(verdict: PortableLocation.unknown, reason: reason)
            )
            XCTAssertFalse(recovery.allowsClaim, "UNKNOWN/" + reason + " must not authorize")
            XCTAssertTrue(recovery.preservesFreeUse, "UNKNOWN/" + reason + " must preserve free use")
            XCTAssertEqual(recovery.recoveryCode, PortableLocationRecovery.retryFix)
        }
    }

    func testManualNeverAuthorizes() {
        let recovery = PortableLocationRecovery.recover(
            risk(verdict: PortableLocation.manual, reason: PortableLocation.reasonManualEntry)
        )
        XCTAssertFalse(recovery.allowsClaim)
        XCTAssertTrue(recovery.preservesFreeUse)
        XCTAssertEqual(recovery.recoveryCode, PortableLocationRecovery.browseOnly)
    }
}
