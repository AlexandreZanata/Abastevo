import XCTest
@testable import AnpFuelCore

/// Swift unit tests mirroring `contracts/testdata/compat/money-portable-v1.json`
/// and the Kotlin `PortableMoneyTest` vectors.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableMoneyTests: XCTestCase {
    func testParsesExactThreeDecimalAnpText() throws {
        XCTAssertEqual(try PortableMoney.parse("5,999"), 5999)
        XCTAssertEqual(try PortableMoney.parse("4,599"), 4599)
        XCTAssertEqual(try PortableMoney.parse("109,90"), 109900)
    }

    func testParsesTwoDecimalIntegerAndPaddedText() throws {
        XCTAssertEqual(try PortableMoney.parse("5,99"), 5990)
        XCTAssertEqual(try PortableMoney.parse("6"), 6000)
        XCTAssertEqual(try PortableMoney.parse("  5,999  "), 5999)
        XCTAssertEqual(try PortableMoney.parse("5,9990"), 5999)
    }

    func testAcceptsRangeBoundaries() throws {
        XCTAssertEqual(try PortableMoney.parse("0,001"), 1)
        XCTAssertEqual(try PortableMoney.parse("1000,00"), 1_000_000)
        XCTAssertEqual(try PortableMoney.parse("1000,000"), 1_000_000)
    }

    func testRejectsEmptyZeroAndNegative() {
        XCTAssertEqual(PortableMoney.code(of: ""), "missing-price")
        XCTAssertEqual(PortableMoney.code(of: "   "), "missing-price")
        XCTAssertEqual(PortableMoney.code(of: "0,00"), "zero-price")
        XCTAssertEqual(PortableMoney.code(of: "-1,00"), "negative-price")
        XCTAssertEqual(PortableMoney.code(of: "-0,00"), "negative-price")
    }

    func testRejectsOverPrecisionDotSeparatorsAndSigns() {
        XCTAssertEqual(PortableMoney.code(of: "5,9999"), "over-precision")
        XCTAssertEqual(PortableMoney.code(of: "5.999"), "invalid-price")
        XCTAssertEqual(PortableMoney.code(of: "1.099,90"), "invalid-price")
        XCTAssertEqual(PortableMoney.code(of: "+5,00"), "invalid-price")
        XCTAssertEqual(PortableMoney.code(of: "cinco"), "invalid-price")
        XCTAssertEqual(PortableMoney.code(of: "5,"), "invalid-price")
        XCTAssertEqual(PortableMoney.code(of: ",5"), "invalid-price")
        XCTAssertEqual(PortableMoney.code(of: "1000,01"), "over-range")
    }

    func testFormatsCanonicalCommaText() throws {
        XCTAssertEqual(try PortableMoney.format(5999), "5,999")
        XCTAssertEqual(try PortableMoney.format(5990), "5,990")
        XCTAssertEqual(try PortableMoney.format(6000), "6,000")
        XCTAssertEqual(try PortableMoney.format(1), "0,001")
        XCTAssertEqual(try PortableMoney.format(1_000_000), "1000,000")
    }

    func testMultipliesTankFillExactly() throws {
        XCTAssertEqual(try PortableMoney.multiplyTankFill(unitMilli: 5499, capacityMilliLiters: 50_000), 274950)
        XCTAssertEqual(try PortableMoney.multiplyTankFill(unitMilli: 5990, capacityMilliLiters: 50_000), 299500)
        XCTAssertEqual(try PortableMoney.multiplyTankFill(unitMilli: 3100, capacityMilliLiters: 40_000), 124000)
    }

    func testMultipliesTankFillWithHalfUpRounding() throws {
        XCTAssertEqual(try PortableMoney.multiplyTankFill(unitMilli: 1001, capacityMilliLiters: 500), 501)
    }

    func testParsesTankCapacityUpTo200Liters() throws {
        XCTAssertEqual(try PortableMoney.parseCapacityMilliLiters("50"), 50_000)
        XCTAssertEqual(try PortableMoney.parseCapacityMilliLiters("50,5"), 50_500)
        XCTAssertEqual(try PortableMoney.parseCapacityMilliLiters("200"), 200_000)
        XCTAssertThrowsError(try PortableMoney.parseCapacityMilliLiters("0"))
        XCTAssertThrowsError(try PortableMoney.parseCapacityMilliLiters("200,001"))
    }
}
