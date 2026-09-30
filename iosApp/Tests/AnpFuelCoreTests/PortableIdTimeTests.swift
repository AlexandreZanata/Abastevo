import XCTest
@testable import AnpFuelCore

/// Swift unit tests for canonical UUID and UTC instant shapes.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableIdTimeTests: XCTestCase {
    func testAcceptsBackendKernelUuidVector() {
        XCTAssertTrue(PortableIdTime.isUuid("123e4567-e89b-12d3-a456-426614174000"))
    }

    func testRejectsUppercaseAndNonHexUuid() {
        XCTAssertFalse(PortableIdTime.isUuid("123E4567-E89B-12D3-A456-426614174000"))
        XCTAssertFalse(PortableIdTime.isUuid("xyz"))
        XCTAssertFalse(PortableIdTime.isUuid(""))
    }

    func testAcceptsUtcInstant() {
        XCTAssertTrue(PortableIdTime.isUtcInstant("2026-06-07T00:00:00Z"))
    }

    func testAcceptsLeapDayAndRejectsNonLeapFeb29() {
        XCTAssertTrue(PortableIdTime.isUtcInstant("2024-02-29T12:00:00Z"))
        XCTAssertFalse(PortableIdTime.isUtcInstant("2025-02-29T00:00:00Z"))
    }

    func testRejectsBadMonthAndMissingDesignator() {
        XCTAssertFalse(PortableIdTime.isUtcInstant("2026-13-01T00:00:00Z"))
        XCTAssertFalse(PortableIdTime.isUtcInstant("2026-06-07T00:00:00"))
    }
}
