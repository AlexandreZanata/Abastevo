import XCTest
@testable import AnpFuelCore

/// Swift unit tests for F03 comment rules (280 Unicode scalar values).
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableTextTests: XCTestCase {
    func testCountsAsciiAndAccentedScalars() {
        XCTAssertEqual(PortableText.countScalars("abc"), 3)
        XCTAssertEqual(PortableText.countScalars("café"), 4)
    }

    func testCountsAstralEmojiAsOneScalar() {
        XCTAssertEqual(PortableText.countScalars("🚗"), 1)
    }

    func testCountsCombiningSequenceAsBasePlusMark() {
        XCTAssertEqual(PortableText.countScalars("é"), 2)
    }

    func testNormalizesCrlfBeforeCounting() {
        XCTAssertEqual(PortableText.normalize("a\r\nb"), "a\nb")
        XCTAssertEqual(PortableText.countScalars(PortableText.normalize("a\r\nb")), 3)
    }

    func testAcceptsExactly280AndRejects281() {
        XCTAssertTrue(PortableText.isValidComment(String(repeating: "a", count: 280)))
        XCTAssertFalse(PortableText.isValidComment(String(repeating: "a", count: 281)))
    }

    func testRejectsEmptyAndBlank() {
        XCTAssertFalse(PortableText.isValidComment(""))
        XCTAssertFalse(PortableText.isValidComment("   "))
    }

    func testAcceptsEmojiCommentWithinLimit() {
        XCTAssertTrue(PortableText.isValidComment("Preço bom ⛽🚗"))
    }
}
