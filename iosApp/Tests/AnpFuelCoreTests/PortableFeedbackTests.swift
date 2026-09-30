import XCTest
@testable import AnpFuelCore

/// Swift unit tests for portable feedback values (P14-T01).
///
/// 1:1 transcription of `PortableFeedbackTest`: star range, floor
/// agreement, zero-votes nil. Negative counts hit `precondition`
/// (mirroring Kotlin `require`), so they stay untriggered here by
/// design and are covered JVM-side instead.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableFeedbackTests: XCTestCase {
    func testAcceptsOneToFiveStars() {
        for stars in 1...5 {
            XCTAssertTrue(PortableFeedback.isValidRating(stars))
        }
        XCTAssertFalse(PortableFeedback.isValidRating(0))
        XCTAssertFalse(PortableFeedback.isValidRating(6))
        XCTAssertFalse(PortableFeedback.isValidRating(-1))
    }

    func testFloorsAgreementBasisPoints() {
        XCTAssertEqual(PortableFeedback.agreementBasisPoints(valid: 2, invalid: 1), 6666)
        XCTAssertEqual(PortableFeedback.agreementBasisPoints(valid: 0, invalid: 3), 0)
        XCTAssertEqual(PortableFeedback.agreementBasisPoints(valid: 3, invalid: 0), 10000)
        XCTAssertEqual(PortableFeedback.agreementBasisPoints(valid: 1, invalid: 1), 5000)
    }

    func testZeroVotesIsNilNeverZeroPercent() {
        XCTAssertNil(PortableFeedback.agreementBasisPoints(valid: 0, invalid: 0))
    }

    func testTextLimitStaysShared() {
        XCTAssertEqual(PortableText.maxCommentScalars, 280)
        XCTAssertTrue(PortableText.isValidComment(String(repeating: "x", count: 280)))
        XCTAssertFalse(PortableText.isValidComment(String(repeating: "x", count: 281)))
    }
}
