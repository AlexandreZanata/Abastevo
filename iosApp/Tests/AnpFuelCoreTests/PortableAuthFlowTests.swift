import Foundation
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
        var deletedSessions: [PortableAuth.Session] = []

        func requestCode(email: String, purpose: PortableAuth.CodePurpose, locale: String) -> AuthApiResult<EmptyAuthValue> {
            .ok(EmptyAuthValue())
        }

        func signUp(name: String, email: String, code: String) -> AuthApiResult<AuthIssuedKey> {
            .err(PortableAuth.unavailable)
        }

        func recover(email: String, code: String) -> AuthApiResult<AuthIssuedKey> {
            .err(PortableAuth.unavailable)
        }

        func profile(session: PortableAuth.Session) -> AuthApiResult<PortableAuth.Profile> {
            .err(PortableAuth.unavailable)
        }

        func updateName(session: PortableAuth.Session, name: String) -> AuthApiResult<PortableAuth.Profile> {
            .err(PortableAuth.unavailable)
        }

        func setRecoveryEmail(session: PortableAuth.Session, email: String, code: String) -> AuthApiResult<PortableAuth.Profile> {
            .err(PortableAuth.unavailable)
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
            deletedSessions.append(session)
            return deleteResult
        }

        struct ProviderCall: Equatable {
            let provider: String
            let idToken: String
            let nonce: String
            let authorizationCode: String?
        }

        var providerResult: AuthApiResult<AuthConsumeOk>?
        var providerCalls: [ProviderCall] = []

        func loginWithProvider(provider: String, idToken: String, nonce: String, authorizationCode: String?) -> AuthApiResult<AuthConsumeOk> {
            providerCalls.append(ProviderCall(provider: provider, idToken: idToken, nonce: nonce, authorizationCode: authorizationCode))
            if let custom = providerResult { return custom }
            let s = PortableAuth.Session(
                familyId: "fam-apple",
                accountId: "acc-apple",
                accessToken: "at-apple",
                refreshToken: "rt-apple",
                accessExpiresAt: 1_700_000_900,
                absoluteExpiresAt: 1_702_592_000
            )
            return .ok(AuthConsumeOk(session: s, created: true))
        }
    }

    func testKeyLoginAndLogoutClearsLocalAuth() {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let clock = StubClock()
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: clock, nonces: StubNonce(), api: api))

        let res = flow.loginWithKey(rawKey: "abv-secret-key")
        if case .ok(let outcome) = res {
            XCTAssertFalse(outcome.created)
            XCTAssertEqual(outcome.session.accountId, "acc-1")
        } else {
            XCTFail("Expected login success")
        }
        XCTAssertEqual(flow.rehydrate(), .active(store.load() ?? PortableAuth.Session(familyId: "", accountId: "", accessToken: "", refreshToken: "", accessExpiresAt: 0, absoluteExpiresAt: 0)))

        let logout = flow.logout()
        XCTAssertEqual(logout, .ok(EmptyAuthValue()))
        XCTAssertEqual(flow.rehydrate(), .loggedOut)
        XCTAssertNil(store.loadKey())
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

    func testAppleProviderLoginSendsRawNonceAndPersistsSession() {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: StubClock(), nonces: StubNonce(), api: api))

        let result = flow.loginWithProvider(
            provider: PortableAuth.providerApple,
            idToken: "header.payload.signature",
            rawNonce: "raw-nonce-1",
            authorizationCode: ""
        )
        guard case .ok(let outcome) = result else {
            return XCTFail("Expected provider login")
        }
        XCTAssertTrue(outcome.created)
        XCTAssertEqual(store.load(), outcome.session)
        XCTAssertEqual(flow.rehydrate(), .active(outcome.session))
        XCTAssertEqual(api.providerCalls, [
            StubAuthApi.ProviderCall(provider: "apple", idToken: "header.payload.signature", nonce: "raw-nonce-1", authorizationCode: nil),
        ])

        let replay = flow.loginWithProvider(
            provider: PortableAuth.providerApple,
            idToken: "header.payload.signature",
            rawNonce: "raw-nonce-1",
            authorizationCode: "code"
        )
        XCTAssertEqual(replay, .err(PortableAuth.Verdict.codeUnknown))
        XCTAssertEqual(api.providerCalls.count, 1)
    }

    func testAppleProviderLoginRefusesLocallyInvalidInputWithoutNetwork() throws {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: StubClock(), nonces: StubNonce(), api: api))

        XCTAssertEqual(flow.loginWithProvider(provider: "github", idToken: "t", rawNonce: "n", authorizationCode: nil), .err(AuthFlow.clientInvalid))
        XCTAssertEqual(flow.loginWithProvider(provider: "apple", idToken: "", rawNonce: "n", authorizationCode: nil), .err(AuthFlow.clientInvalid))
        XCTAssertEqual(flow.loginWithProvider(provider: "apple", idToken: "t", rawNonce: "", authorizationCode: nil), .err(AuthFlow.clientInvalid))
        let longToken = String(repeating: "a", count: AuthFlow.maxProofChars + 1)
        XCTAssertEqual(flow.loginWithProvider(provider: "apple", idToken: longToken, rawNonce: "n", authorizationCode: nil), .err(AuthFlow.clientInvalid))
        try store.saveKey(AuthKeyBackup(username: "maria", accountKey: "abv-secret-key"))
        XCTAssertEqual(flow.loginWithProvider(provider: "apple", idToken: "t", rawNonce: "n", authorizationCode: nil), .err(AuthFlow.clientInvalid))
        XCTAssertTrue(api.providerCalls.isEmpty)
        XCTAssertNil(store.load())
    }

    func testAppleProviderUnavailableKeepsDeviceSignedOut() {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let api = StubAuthApi()
        api.providerResult = .err(AccountProviderLogin.unavailableVerdict)
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: StubClock(), nonces: StubNonce(), api: api))

        XCTAssertEqual(
            flow.loginWithProvider(provider: "apple", idToken: "t", rawNonce: "n-1", authorizationCode: nil),
            .err("provider-unavailable")
        )
        XCTAssertNil(store.load())
        XCTAssertEqual(flow.rehydrate(), .loggedOut)
        api.providerResult = nil
        guard case .ok = flow.loginWithProvider(provider: "apple", idToken: "t", rawNonce: "n-1", authorizationCode: nil) else {
            return XCTFail("An unavailable reply must not spend the nonce")
        }
    }

    func testLegacyLocalAppleSessionCannotRehydrateOrAuthorizeDeletion() throws {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: StubClock(), nonces: StubNonce(), api: api))
        let legacy = PortableAuth.Session(
            familyId: "apple-legacy-family",
            accountId: "apple-legacy-account",
            accessToken: "apple-local-proof",
            refreshToken: "apple-refresh-legacy",
            accessExpiresAt: 1_700_000_900,
            absoluteExpiresAt: 1_702_592_000
        )
        let backup = AuthKeyBackup(username: "maria", accountKey: "abv-secret-key")
        try store.save(legacy)
        try store.saveKey(backup)

        XCTAssertNil(flow.currentSession())
        XCTAssertEqual(flow.rehydrate(), .pendingKeyAccount(backup))
        XCTAssertEqual(flow.deleteAccount(), .err(AuthFlow.clientInvalid))
        XCTAssertTrue(api.deletedSessions.isEmpty)
        XCTAssertEqual(store.loadKey(), backup)
    }

    func testTemporaryDeletionFailureKeepsSessionAndBackupForRetry() throws {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let api = StubAuthApi()
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: StubClock(), nonces: StubNonce(), api: api))
        guard case .ok(let outcome) = flow.loginWithKey(rawKey: "abv-secret-key") else {
            return XCTFail("Expected key login")
        }
        api.deleteResult = .err(PortableAuth.unavailable)

        XCTAssertEqual(flow.deleteAccount(), .err(PortableAuth.unavailable))
        XCTAssertEqual(store.load(), outcome.session)
        XCTAssertEqual(store.loadKey()?.accountKey, "abv-secret-key")

        api.deleteResult = .ok(EmptyAuthValue())
        XCTAssertEqual(flow.deleteAccount(), .ok(EmptyAuthValue()))
        XCTAssertEqual(api.deletedSessions, [outcome.session, outcome.session])
        XCTAssertNil(store.load())
        XCTAssertNil(store.loadKey())
    }

    func testLegacyOfflineKeyDoesNotPreventAppleLogin() throws {
        let store = IosKeychainSecretStore(useMemoryOnly: true)
        let flow = AuthFlow(ports: AuthPorts(store: store, keys: store, clock: StubClock(), nonces: StubNonce(), api: StubAuthApi()))
        try store.saveKey(AuthKeyBackup(username: "maria", accountKey: "abv-12345678-abc"))

        XCTAssertEqual(flow.rehydrate(), .loggedOut)
        guard case .ok = flow.loginWithProvider(provider: "apple", idToken: "t", rawNonce: "n-legacy", authorizationCode: nil) else {
            return XCTFail("Expected Apple login")
        }
    }

    func testOwnerPurgeUsesTheFullConsentScopePrefix() {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = IosLocalStore(directory: directory)
        let origin = ApiEnvironment.staging.origin
        let ownerScope = "account:owner:fingerprint|\(origin)"
        let differentScope = "account:owner-other:fingerprint|\(origin)"
        let differentOwner = TrainingConsentRecord(ownerKey: differentScope, origin: origin, granted: false)
        var snapshot = PersistedAppSnapshot(trainingConsentByOwner: [
            ownerScope: TrainingConsentRecord(ownerKey: ownerScope, origin: origin, granted: true),
            differentScope: differentOwner,
        ])

        store.purgeOwnedData(snapshot: &snapshot, ownerAccountId: "owner")

        XCTAssertEqual(snapshot.trainingConsentByOwner, [differentScope: differentOwner])
        XCTAssertEqual(store.load().trainingConsentByOwner, [differentScope: differentOwner])
    }
}
