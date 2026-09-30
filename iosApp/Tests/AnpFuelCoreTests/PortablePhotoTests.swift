import XCTest
@testable import AnpFuelCore

/// Swift unit tests for portable photo values (P15-T02A).
///
/// 1:1 transcription of `PortablePhotoTest`: frozen budgets, exact intent
/// allowlist, power-of-2 sample plans inside both pixel bounds, wire-cap
/// edges and capture+24 h expiry. Non-positive frames hit `precondition`
/// (mirroring Kotlin `require`), so they stay untriggered here by design
/// and are covered JVM-side instead.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortablePhotoTests: XCTestCase {
    func testBudgetsMirrorFrozenForwardContract() {
        XCTAssertEqual(PortablePhoto.wireMime, "image/jpeg")
        XCTAssertEqual(PortablePhoto.targetBytes, 153600)
        XCTAssertEqual(PortablePhoto.capBytes, 262144)
        XCTAssertEqual(PortablePhoto.maxEdgePixels, 1600)
        XCTAssertEqual(PortablePhoto.maxPixels, 2000000)
        XCTAssertEqual(PortablePhoto.maxAttempts, 3)
        XCTAssertEqual(PortablePhoto.workingMemoryHypothesisBytes, 33554432)
        XCTAssertEqual(PortablePhoto.transientTTLMillis, 86400000)
    }

    func testIntentAllowlistIsExact() {
        XCTAssertTrue(PortablePhoto.isSupportedIntentMime("image/jpeg"))
        XCTAssertTrue(PortablePhoto.isSupportedIntentMime("image/png"))
        XCTAssertTrue(PortablePhoto.isSupportedIntentMime("image/heic"))
        XCTAssertTrue(PortablePhoto.isSupportedIntentMime("image/heif"))
        XCTAssertFalse(PortablePhoto.isSupportedIntentMime("image/gif"))
        XCTAssertFalse(PortablePhoto.isSupportedIntentMime("image/webp"))
        XCTAssertFalse(PortablePhoto.isSupportedIntentMime(""))
    }

    func testSampleSizeKeepsDecodedFrameInsideBudgets() {
        XCTAssertEqual(PortablePhoto.sampleSizeForBounds(width: 1200, height: 900), 1)
        XCTAssertEqual(PortablePhoto.sampleSizeForBounds(width: 4000, height: 3000), 4)
        XCTAssertEqual(PortablePhoto.sampleSizeForBounds(width: 8000, height: 6000), 8)
        XCTAssertEqual(PortablePhoto.sampleSizeForBounds(width: 3200, height: 400), 2)
    }

    func testWireCapBoundsBytes() {
        XCTAssertTrue(PortablePhoto.fitsWireCap(1))
        XCTAssertTrue(PortablePhoto.fitsWireCap(262144))
        XCTAssertFalse(PortablePhoto.fitsWireCap(0))
        XCTAssertFalse(PortablePhoto.fitsWireCap(262145))
    }

    func testTransientExpiryIsCapturePlus24h() {
        XCTAssertFalse(PortablePhoto.isTransientExpired(capturedAtMillis: 0, nowMillis: 86399999))
        XCTAssertTrue(PortablePhoto.isTransientExpired(capturedAtMillis: 0, nowMillis: 86400000))
    }
}
