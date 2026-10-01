import XCTest
@testable import AnpFuelCore

/// Swift lifecycle and privacy exit (P17-T04, B-BR-F01…F08/M05/L01…L04).
///
/// Replays the same fixture numbers as the JVM
/// `FeedbackLifecycleExitTest`: 280-scalar text, floor agreement
/// (2/1 → 6666 bp), revoked-session refusal without transport,
/// ownership/erasure kinds, 24 h transient media expiry and
/// simulated-location denial. Diagnostics carry opaque aliases and
/// counts only — no emails, GPS coordinates or photo payloads.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class FeedbackLifecycleTests: XCTestCase {
    func testSameFixturesRuleBothPlatforms() {
        XCTAssertTrue(PortableText.isValidComment(String(repeating: "x", count: 280)))
        XCTAssertFalse(PortableText.isValidComment(String(repeating: "x", count: 281)))
        XCTAssertEqual(PortableFeedback.agreementBasisPoints(valid: 2, invalid: 1), 6666)
        XCTAssertNil(PortableFeedback.agreementBasisPoints(valid: 0, invalid: 0))
    }

    func testRevokedSessionBlocksWritesWithoutTransport() {
        var flow = FeedbackFlow(isEnabled: { true })
        flow.prepare(stationId: "s-1", product: "GASOLINE", accountId: "   ")
        XCTAssertEqual(flow.submitComment(text: "good fuel"), .signInRequired)
        XCTAssertNil(flow.pendingOpId)
    }

    func testOwnershipAndErasureRefusalsCarryFixedKinds() {
        XCTAssertEqual(
            FeedbackFlow.rejectedState(kind: .notAuthor, message: "foreign"),
            .rejected(kindLabel: "NOT_AUTHOR", message: "foreign")
        )
        XCTAssertEqual(
            FeedbackFlow.rejectedState(kind: .commentNotFound, message: "gone"),
            .rejected(kindLabel: "NOT_FOUND", message: "gone")
        )
        XCTAssertEqual(
            FeedbackFlow.rejectedState(kind: .staleRevision, message: "stale"),
            .rejected(kindLabel: "STALE", message: "stale")
        )
    }

    func testTransientMediaExpiresAtExactly24Hours() {
        let ttl = PortablePhoto.transientTTLMillis
        XCTAssertFalse(PortablePhoto.isTransientExpired(capturedAtMillis: 0, nowMillis: ttl - 1))
        XCTAssertTrue(PortablePhoto.isTransientExpired(capturedAtMillis: 0, nowMillis: ttl))
    }

    func testSimulatedLocationDeniesClaims() {
        let simulated = PortableLocation.classify(PortableLocation.FixInput(
            permissionGranted: true,
            hasFix: true,
            sourceInfoPresent: true,
            simulated: true,
            accuracyMeters: 10.0,
            fixAgeSeconds: 30,
            clockSkewSeconds: 0
        ))
        XCTAssertEqual(simulated.verdict, PortableLocation.simulated)
        XCTAssertFalse(simulated.allowsClaim)
        let denied = PortableLocation.classify(PortableLocation.FixInput(
            permissionGranted: false,
            hasFix: false,
            sourceInfoPresent: false,
            simulated: false,
            accuracyMeters: nil,
            fixAgeSeconds: nil,
            clockSkewSeconds: nil
        ))
        XCTAssertEqual(denied.verdict, PortableLocation.denied)
        XCTAssertFalse(denied.allowsClaim)
    }

    func testDiagnosticsCarryAliasesAndCountsNeverPII() {
        let line = FeedbackDisplay.agreementLine(valid: 2, invalid: 1)
        XCTAssertTrue(line.contains("2 valid / 1 invalid"))
        XCTAssertFalse(line.contains("@"))
        XCTAssertFalse(FeedbackDisplay.rejectKindLabel(.gateRequired).contains("@"))
    }
}
