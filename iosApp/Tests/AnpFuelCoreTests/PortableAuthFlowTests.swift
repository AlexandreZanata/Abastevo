import XCTest
@testable import AnpFuelCore

final class PortableAuthFlowTests: XCTestCase {
    private final class StubClock: AuthWallClock {
        var now: Int64 = 1_700_000_000
        func nowEpochSeconds() -> Int64 { now }
    }

    private final class StubNonce: PortableNonceSource {
        var counter = 0
        func nextNonce() -> String {
            counter += 1
            return "nonce-\(counter)"
        }
    }

    private final class StubAuthApi: AuthAccountApi {
        var refreshResult: AuthApiResult<PortableAuth.Session>?
        var loginKeyResult: AuthApiResult<AuthKeyLogin>?
        var deleteResult: AuthApiResult<EmptyAuthValue> = .ok(EmptyAuthValue())

        func requestCode(email: String) -> AuthApiResult<EmptyAuthValue> { .ok(EmptyAuthValue()) }

        func consumeCode(email: String, code: String) -> AuthApiResult<AuthConsumeOk> {
            let s = PortableAuth.Session(
                familyId: "fam-1",
                accountId: "acc-1",
                accessToken: "at-1",
                refreshToken: "rt-1",
                accessExpiresAt: 1_700_000_900,
                absoluteExpiresAt: 1_702_592_000
            )
            return .ok(AuthConsumeOk(session: s, created: true))
        }

        func createKeyAccount(username: String) -> AuthApiResult<AuthKeyIssued> {
            return .ok(AuthKeyIssued(username: username, accountKey: "abv-secret-key"))
        }

        func loginWithKey(accountKey: String) -> AuthApiResult<AuthKeyLogin> {
            if let custom = loginKeyResult { return custom }
            let s = PortableAuth.Session(
                familyId: "fam-k",
                accountId: "acc-1",
                accessToken: "at-k",
                refreshToken: "rt-k",
                accessExpiresAt: 1_700_000_900,
                absoluteExpiresAt: 1_702_592_000
            )
            return .ok(AuthKeyLogin(session: s, username: "maria"))
        }

        func refresh(familyId: String, refreshToken: String) -> AuthApiResult<PortableAuth.Session> {
            return refreshResult ?? .err(PortableAuth.Verdict.sessionExpired)
        }

        func revokeAll(familyId: String, accessToken: String) -> AuthApiResult<EmptyAuthValue> {
            return .ok(EmptyAuthValue())
        }

        func linkProvider(session: PortableAuth.Session, provider: String, idToken: String, nonce: String) -> AuthApiResult<EmptyAuthValue> {
            return .ok(EmptyAuthValue())
        }

        func deleteAccount(session: PortableAuth.Session) -> AuthApiResult<EmptyAuthValue> {
            return deleteResult
        }
    }

    func testEmailCodeLoginAndLogoutClearsLocalAuth() {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let clock = StubClock()
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: clock, nonces: StubNonce(), api: api))

        let res = flow.consumeEmailCode(email: "ana@example.com", code: "123456")
        if case .ok(let outcome) = res {
            XCTAssertTrue(outcome.created)
            XCTAssertEqual(outcome.session.accountId, "acc-1")
        } else {
            XCTFail("Expected login success")
        }
        XCTAssertEqual(flow.rehydrate(), .active(store.load() ?? PortableAuth.Session(familyId: "", accountId: "", accessToken: "", refreshToken: "", accessExpiresAt: 0, absoluteExpiresAt: 0)))

        let logout = flow.logout()
        XCTAssertEqual(logout, .ok(EmptyAuthValue()))
        XCTAssertEqual(flow.rehydrate(), .loggedOut)
    }

    func testRevokedRefreshClearsWithoutKeyFallback() {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let clock = StubClock()
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: clock, nonces: StubNonce(), api: api))

        _ = flow.loginWithKey(rawKey: "abv-secret-key")
        clock.now = 1_700_000_950
        api.refreshResult = .err(PortableAuth.Verdict.sessionRevoked)

        let refreshRes = flow.refreshSession()
        XCTAssertEqual(refreshRes, .err(PortableAuth.Verdict.sessionRevoked))
        XCTAssertNil(store.load())
        XCTAssertNil(store.loadKey())
    }

    func testProviderNonceReplayIsRefused() throws {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let clock = StubClock()
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: clock, nonces: StubNonce(), api: api))

        let session = PortableAuth.Session(familyId: "f", accountId: "a", accessToken: "t", refreshToken: "r", accessExpiresAt: 1_700_000_900, absoluteExpiresAt: 1_702_000_000)
        guard case .ok(let attempt) = flow.beginProviderLogin(provider: PortableAuth.providerApple) else {
            XCTFail("Expected attempt")
            return
        }
        let callback = PortableAuth.ProviderCallback(provider: PortableAuth.providerApple, idToken: "jwt-tok", nonce: attempt.nonce, state: attempt.state)
        XCTAssertEqual(flow.completeProviderLogin(session: session, attempt: attempt, callback: callback), .ok(EmptyAuthValue()))
        XCTAssertEqual(flow.completeProviderLogin(session: session, attempt: attempt, callback: callback), .err(PortableAuth.Verdict.codeUnknown))
    }
}
