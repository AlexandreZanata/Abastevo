import XCTest
@testable import AnpFuelCore

/// Swift unit tests for portable FREE-account auth values (P13-T05A).
///
/// 1:1 transcription of `PortableAuthTest`: allowlist, code shape,
/// liveness, nonce binding and strict callback parsing.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class PortableAuthTests: XCTestCase {
    func testAllowlistsProvidersAndIssuers() {
        XCTAssertTrue(PortableAuth.isProvider("google"))
        XCTAssertTrue(PortableAuth.isProvider("apple"))
        XCTAssertFalse(PortableAuth.isProvider("github"))
        XCTAssertFalse(PortableAuth.isProvider(""))
        XCTAssertEqual(PortableAuth.issuerOf("google"), "https://accounts.google.com")
        XCTAssertEqual(PortableAuth.issuerOf("apple"), "https://appleid.apple.com")
        XCTAssertEqual(PortableAuth.issuerOf("github"), "")
    }

    func testChecksCodeShapeOnly() {
        XCTAssertTrue(PortableAuth.isCodeShape("482916"))
        XCTAssertTrue(PortableAuth.isCodeShape("000000"))
        XCTAssertFalse(PortableAuth.isCodeShape("48291"))
        XCTAssertFalse(PortableAuth.isCodeShape("4829167"))
        XCTAssertFalse(PortableAuth.isCodeShape("48a916"))
        XCTAssertFalse(PortableAuth.isCodeShape(""))
    }

    func testComputesSessionLiveness() {
        let session = PortableAuth.Session(
            familyId: "f", accountId: "a",
            accessToken: "at", refreshToken: "rt",
            accessExpiresAt: 1_000_900, absoluteExpiresAt: 4_259_200,
        )
        XCTAssertTrue(PortableAuth.isAccessLive(session, nowEpochSeconds: 1_000_000))
        XCTAssertFalse(PortableAuth.isAccessLive(session, nowEpochSeconds: 1_000_900))
        XCTAssertTrue(PortableAuth.isRefreshLive(session, nowEpochSeconds: 1_000_901))
        XCTAssertFalse(PortableAuth.isRefreshLive(session, nowEpochSeconds: 4_259_200))
    }

    func testBindsExactCallbackAndRefusesSubstitution() {
        let attempt = PortableAuth.LoginAttempt(provider: "google", nonce: "n-1", state: "s-1")
        let valid = PortableAuth.ProviderCallback(provider: "google", idToken: "tok", nonce: "n-1", state: "s-1")
        XCTAssertNotNil(PortableAuth.bind(attempt: attempt, callback: valid))
        XCTAssertNil(PortableAuth.bind(attempt: attempt, callback: .init(provider: "google", idToken: "tok", nonce: "n-2", state: "s-1")))
        XCTAssertNil(PortableAuth.bind(attempt: attempt, callback: .init(provider: "google", idToken: "tok", nonce: "n-1", state: "s-2")))
        XCTAssertNil(PortableAuth.bind(attempt: attempt, callback: .init(provider: "apple", idToken: "tok", nonce: "n-1", state: "s-1")))
        XCTAssertNil(PortableAuth.bind(attempt: attempt, callback: .init(provider: "google", idToken: "", nonce: "n-1", state: "s-1")))
    }

    func testParsesCallbackUrlStrictly() {
        let parsed = PortableAuth.parseCallback(
            "anpfuel://auth/callback?provider=google&id_token=tok&nonce=n-1&state=s-1",
        )
        XCTAssertNotNil(parsed)
        XCTAssertEqual(parsed?.provider, "google")
        XCTAssertEqual(parsed?.nonce, "n-1")
        XCTAssertEqual(parsed?.state, "s-1")
        XCTAssertNil(PortableAuth.parseCallback("https://auth/callback?provider=google&id_token=t&nonce=n&state=s"))
        XCTAssertNil(PortableAuth.parseCallback("anpfuel://other/callback?provider=google&id_token=t&nonce=n&state=s"))
        XCTAssertNil(PortableAuth.parseCallback("anpfuel://auth/callback?provider=google&nonce=n&state=s"))
        XCTAssertNil(PortableAuth.parseCallback("anpfuel://auth/callback?provider=&id_token=t&nonce=n&state=s"))
        XCTAssertNil(PortableAuth.parseCallback(
            "anpfuel://auth/callback?provider=google&provider=apple&id_token=t&nonce=n&state=s",
        ))
        XCTAssertNil(PortableAuth.parseCallback("not a url"))
    }
}
