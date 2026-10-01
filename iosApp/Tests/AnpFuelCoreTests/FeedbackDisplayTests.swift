import XCTest
@testable import AnpFuelCore

/// Swift unit tests for feedback presentation (P17-T03).
///
/// 1:1 transcription of `FeedbackDisplayTest`: floor agreement with
/// counts, zero-votes "No votes", 280-scalar counter and fixed short
/// rejection codes.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class FeedbackDisplayTests: XCTestCase {
    func testAgreementTruncatesToOneDecimalWithCounts() {
        let line = FeedbackDisplay.agreementLine(valid: 2, invalid: 1)
        XCTAssertTrue(line.contains("66.6%"))
        XCTAssertTrue(line.contains("2"))
        XCTAssertTrue(line.contains("1"))
    }

    func testZeroVotesRendersNoVotes() {
        XCTAssertEqual(FeedbackDisplay.agreementLine(valid: 0, invalid: 0), "No votes")
    }

    func testCharsRemainingFollows280Rule() {
        XCTAssertEqual(FeedbackDisplay.charsRemaining(String(repeating: "x", count: 280)), 0)
        XCTAssertEqual(FeedbackDisplay.charsRemaining(String(repeating: "x", count: 281)), -1)
        XCTAssertEqual(FeedbackDisplay.charsRemaining("é"), 279)
    }

    func testRejectLabelsAreFixedCodes() {
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.staleRevision), "STALE")
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.selfVote), "SELF_VOTE")
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.quotaExceeded), "QUOTA")
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.transport), "TRANSPORT")
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.commentNotFound), "NOT_FOUND")
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.ratingNotFound), "NOT_FOUND")
        XCTAssertEqual(FeedbackDisplay.rejectKindLabel(.gateRequired), "SIGN_IN")
    }
}
