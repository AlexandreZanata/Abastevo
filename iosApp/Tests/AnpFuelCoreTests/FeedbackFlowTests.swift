import XCTest
@testable import AnpFuelCore

/// Swift unit tests for the thin iPhone social-flow guards (P17-T03).
///
/// Mirrors the Android `FeedbackViewModelTest` local-guard slice: flag
/// OFF disables without IO, blank account needs sign-in without IO,
/// over-280/blank text is INVALID without IO, bad vote choice is
/// INVALID, retry reuses the same stable op id, and server-kind mapping
/// uses the fixed Android labels for rollback.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class FeedbackFlowTests: XCTestCase {
    func testFlagOffDisablesWithoutIO() {
        var flow = FeedbackFlow(isEnabled: { false })
        XCTAssertEqual(flow.state, .disabled)
        flow.prepare(stationId: "s1", product: "GASOLINE", accountId: "a1")
        XCTAssertEqual(flow.state, .disabled)
    }

    func testBlankAccountNeedsSignInWithoutIO() {
        var flow = FeedbackFlow(isEnabled: { true })
        flow.prepare(stationId: "s1", product: "GASOLINE", accountId: "   ")
        XCTAssertEqual(flow.submitComment(text: "ok"), .signInRequired)
    }

    func testOver280AndBlankTextAreInvalidWithoutIO() {
        var flow = FeedbackFlow(isEnabled: { true })
        flow.prepare(stationId: "s1", product: "GASOLINE", accountId: "a1")
        XCTAssertEqual(
            flow.submitComment(text: String(repeating: "x", count: 281)),
            .rejected(kindLabel: "INVALID", message: "comment text exceeds 280 characters")
        )
        XCTAssertEqual(
            flow.submitComment(text: "   "),
            .rejected(kindLabel: "INVALID", message: "comment text exceeds 280 characters")
        )
    }

    func testBadVoteChoiceIsInvalid() {
        var flow = FeedbackFlow(isEnabled: { true })
        flow.prepare(stationId: "s1", product: "GASOLINE", accountId: "a1")
        XCTAssertEqual(
            flow.submitVote(commentId: "c1", choice: "MAYBE"),
            .rejected(kindLabel: "INVALID", message: "vote is invalid")
        )
    }

    func testRetryReusesStableOpId() {
        var flow = FeedbackFlow(isEnabled: { true })
        flow.prepare(stationId: "s1", product: "GASOLINE", accountId: "a1")
        XCTAssertEqual(flow.submitComment(text: "ok"), .submitting)
        let first = flow.pendingOpId
        XCTAssertNotNil(first)
        XCTAssertEqual(flow.retry(), .submitting)
        XCTAssertEqual(flow.pendingOpId, first)
    }

    func testServerKindMappingUsesFixedLabels() {
        XCTAssertEqual(
            FeedbackFlow.rejectedState(kind: .staleRevision, message: "stale"),
            .rejected(kindLabel: "STALE", message: "stale")
        )
        XCTAssertEqual(
            FeedbackFlow.rejectedState(kind: .gateRequired, message: "login"),
            .rejected(kindLabel: "SIGN_IN", message: "login")
        )
    }
}
