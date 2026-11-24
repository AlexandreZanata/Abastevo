import Foundation
import XCTest
@testable import AnpFuelCore

/// Verified sign-up, key recovery and the private profile over the real account bridge
/// (`AbastevoHttpClient` with a scripted exchange), mirroring Android `AccountHttpApiTest`
/// and `AccountRecoveryFlowTest`.
final class AccountAccessTests: XCTestCase {
    private static let accountId = "a0000000-0000-4000-8000-000000000001"
    private static let familyId = "f0000000-0000-4000-8000-000000000001"
    private static let key = "abcdefghjkmnpqrstuvwxyz23456789a"

    private final class FixedClock: AuthWallClock {
        // 2030-01-01T00:00:00Z, 15 minutes before the scripted access expiry.
        func nowEpochSeconds() -> Int64 { 1_893_456_000 }
    }

    private final class CountingNonce: PortableNonceSource {
        func nextNonce() -> String { UUID().uuidString }
    }

    private final class ScriptedAccountExchange: AuthHttpExchange {
        var replies: [String: AuthHttpReply] = [:]
        private(set) var requests: [URLRequest] = []

        func reply(_ path: String, status: Int, _ json: String) {
            replies[path] = AuthHttpReply(status: status, body: Data(json.utf8))
        }

        func send(_ request: URLRequest) -> AuthHttpReply? {
            requests.append(request)
            guard let path = request.url?.path else { return nil }
            return replies[path]
        }

        var paths: [String] { requests.compactMap { $0.url?.path } }

        func body(_ index: Int) -> [String: String] {
            guard requests.indices.contains(index),
                  let data = requests[index].httpBody,
                  let json = try? JSONSerialization.jsonObject(with: data) as? [String: String] else {
                return [:]
            }
            return json
        }
    }

    /// Accepts sessions but refuses to keep a key backup.
    private final class RefusingKeyStore: AuthKeyStore {
        func saveKey(_ backup: AuthKeyBackup) throws {
            throw NSError(domain: "AccountAccessTests", code: 1)
        }
        func loadKey() -> AuthKeyBackup? { nil }
        func clearKey() throws {}
    }

    private var exchange = ScriptedAccountExchange()
    private var store = IosKeychainSecretStore(useMemoryOnly: true)

    override func setUp() {
        super.setUp()
        exchange = ScriptedAccountExchange()
        store = IosKeychainSecretStore(useMemoryOnly: true)
    }

    private func makeFlow(keys: AuthKeyStore? = nil) -> AuthFlow {
        let client = AbastevoHttpClient(environment: .staging)
        client.authExchange = exchange
        let keyStore: AuthKeyStore = keys ?? store
        return AuthFlow(ports: AuthPorts(
            store: store,
            keys: keyStore,
            clock: FixedClock(),
            nonces: CountingNonce(),
            api: client
        ))
    }

    private static func sessionJson(
        account: String = AccountAccessTests.accountId,
        family: String = AccountAccessTests.familyId
    ) -> String {
        """
        {"family_id":"\(family)","account_id":"\(account)","access_token":"access-1","refresh_token":"refresh-1",\
        "access_expires_at":"2030-01-01T00:15:00Z","absolute_expires_at":"2030-01-31T00:00:00Z"}
        """
    }

    private static let liveSession = PortableAuth.Session(
        familyId: familyId,
        accountId: accountId,
        accessToken: "access-0",
        refreshToken: "refresh-0",
        accessExpiresAt: 1_893_456_900,
        absoluteExpiresAt: 1_896_048_000
    )

    private static func profileJson(name: String = "\"Ana Lima\"", hint: String = "\"a***@example.invalid\"") -> String {
        """
        {"account_id":"\(accountId)","public_name":"Ana Lima","name":\(name),"profile_complete":true,\
        "recovery_email_hint":\(hint),"has_account_key":true,"providers":["apple"]}
        """
    }

    private static func error(_ code: String) -> String {
        #"{"error":{"code":"\#(code)","message":"safe text","trace_id":"req-1"}}"#
    }

    // MARK: - Wire bodies

    func testCodeRequestsCarryPurposeAndLocale() {
        exchange.reply("/v1/accounts/email/codes", status: 202, #"{"status":"sent"}"#)
        let flow = makeFlow()

        XCTAssertEqual(flow.requestSignUpCode(name: "Ana Lima", email: " ana@example.invalid ", locale: "pt-BR"), .ok(EmptyAuthValue()))
        XCTAssertEqual(flow.requestCode(email: "ana@example.invalid", purpose: .recovery, locale: "en"), .ok(EmptyAuthValue()))
        XCTAssertEqual(flow.requestCode(email: "ana@example.invalid", purpose: .attach, locale: "es"), .ok(EmptyAuthValue()))

        XCTAssertEqual(exchange.paths, Array(repeating: "/v1/accounts/email/codes", count: 3))
        XCTAssertEqual(exchange.body(0), ["email": "ana@example.invalid", "purpose": "signup", "locale": "pt-BR"])
        XCTAssertEqual(exchange.body(1), ["email": "ana@example.invalid", "purpose": "recovery", "locale": "en"])
        XCTAssertEqual(exchange.body(2), ["email": "ana@example.invalid", "purpose": "attach", "locale": "es"])
        XCTAssertEqual(exchange.requests.first?.httpMethod, "POST")
        XCTAssertEqual(exchange.requests.first?.value(forHTTPHeaderField: "Content-Type"), "application/json")
    }

    func testSignUpSendsTheVerifiedProofAndKeepsKeyAndSession() {
        exchange.reply("/v1/accounts/keys/signup", status: 201, """
        {"account":{"account_id":"\(Self.accountId)","public_name":"Ana Lima"},"session":\(Self.sessionJson()),\
        "account_key":"\(Self.key)","created":true}
        """)
        let flow = makeFlow()

        let result = flow.signUp(name: "  Ana   Lima ", email: " ana@example.invalid ", code: "482916")

        let backup = AuthKeyBackup(username: "Ana Lima", accountKey: Self.key)
        XCTAssertEqual(result, .ok(AuthNewKey(backup: backup, sessionSaved: true)))
        XCTAssertEqual(exchange.paths, ["/v1/accounts/keys/signup"])
        XCTAssertEqual(exchange.body(0), ["name": "Ana Lima", "email": "ana@example.invalid", "code": "482916"])
        XCTAssertEqual(store.loadKey(), backup)
        XCTAssertEqual(store.load()?.accountId, Self.accountId)
        guard case .active = flow.rehydrate() else {
            return XCTFail("Sign-up must leave a live session")
        }
    }

    func testRecoveryReplacesTheKeyBackupAndOpensANewSession() throws {
        try store.saveKey(AuthKeyBackup(username: "Ana Lima", accountKey: "old-key"))
        exchange.reply("/v1/accounts/recovery", status: 200, """
        {"account":{"account_id":"\(Self.accountId)","public_name":"Ana Lima"},"session":\(Self.sessionJson()),\
        "account_key":"\(Self.key)","sessions_revoked":true}
        """)
        let flow = makeFlow()

        let result = flow.recover(email: "ana@example.invalid", code: "482916")

        let backup = AuthKeyBackup(username: "Ana Lima", accountKey: Self.key)
        XCTAssertEqual(result, .ok(AuthNewKey(backup: backup, sessionSaved: true)))
        XCTAssertEqual(exchange.paths, ["/v1/accounts/recovery"])
        XCTAssertEqual(exchange.body(0), ["email": "ana@example.invalid", "code": "482916"])
        XCTAssertEqual(store.loadKey(), backup)
    }

    func testIssuedKeyReplyMustMatchItsSessionOwnerAndFlag() {
        exchange.reply("/v1/accounts/recovery", status: 200, """
        {"account":{"account_id":"another-account","public_name":"Ana Lima"},"session":\(Self.sessionJson()),\
        "account_key":"\(Self.key)","sessions_revoked":true}
        """)
        exchange.reply("/v1/accounts/keys/signup", status: 201, """
        {"account":{"account_id":"\(Self.accountId)","public_name":"Ana Lima"},"session":\(Self.sessionJson()),\
        "account_key":"\(Self.key)","created":false}
        """)
        let flow = makeFlow()

        XCTAssertEqual(flow.recover(email: "ana@example.invalid", code: "482916"), .err(PortableAuth.unavailable))
        XCTAssertEqual(flow.signUp(name: "Ana Lima", email: "ana@example.invalid", code: "482916"), .err(PortableAuth.unavailable))
        XCTAssertNil(store.load())
        XCTAssertNil(store.loadKey())
    }

    func testStorageFailureStillReturnsTheOnlyDisplayOfTheKey() {
        exchange.reply("/v1/accounts/keys/signup", status: 201, """
        {"account":{"account_id":"\(Self.accountId)","public_name":"Ana Lima"},"session":\(Self.sessionJson()),\
        "account_key":"\(Self.key)","created":true}
        """)
        let flow = makeFlow(keys: RefusingKeyStore())

        let result = flow.signUp(name: "Ana Lima", email: "ana@example.invalid", code: "482916")

        XCTAssertEqual(result, .ok(AuthNewKey(backup: AuthKeyBackup(username: "Ana Lima", accountKey: Self.key), sessionSaved: false)))
        XCTAssertNil(store.load())
    }

    func testProfileReadsAndChangesSendTheLiveSessionProof() throws {
        try store.save(Self.liveSession)
        exchange.reply("/v1/accounts/profile", status: 200, Self.profileJson(name: "null", hint: "null"))
        exchange.reply("/v1/accounts/profile/name", status: 200, Self.profileJson())
        exchange.reply("/v1/accounts/recovery-email", status: 200, Self.profileJson())
        let flow = makeFlow()

        XCTAssertEqual(flow.profile(), .ok(PortableAuth.Profile(
            accountId: Self.accountId, publicName: "Ana Lima", name: nil, profileComplete: true,
            recoveryEmailHint: nil, hasAccountKey: true, providers: ["apple"]
        )))
        let named = PortableAuth.Profile(
            accountId: Self.accountId, publicName: "Ana Lima", name: "Ana Lima", profileComplete: true,
            recoveryEmailHint: "a***@example.invalid", hasAccountKey: true, providers: ["apple"]
        )
        XCTAssertEqual(flow.updateName(" Ana  Lima "), .ok(named))
        XCTAssertEqual(flow.setRecoveryEmail(email: " ana@example.invalid ", code: "123456"), .ok(named))

        XCTAssertEqual(exchange.paths, ["/v1/accounts/profile", "/v1/accounts/profile/name", "/v1/accounts/recovery-email"])
        let proof = ["family_id": Self.familyId, "access_token": "access-0"]
        XCTAssertEqual(exchange.body(0), proof)
        XCTAssertEqual(exchange.body(1), proof.merging(["name": "Ana Lima"]) { $1 })
        XCTAssertEqual(exchange.body(2), proof.merging(["email": "ana@example.invalid", "code": "123456"]) { $1 })
    }

    func testProfileOfAnotherAccountIsRefused() throws {
        try store.save(Self.liveSession)
        exchange.reply("/v1/accounts/profile", status: 200, """
        {"account_id":"another-account","public_name":"Ana Lima","name":null,"profile_complete":false,\
        "recovery_email_hint":null,"has_account_key":true,"providers":[]}
        """)

        XCTAssertEqual(makeFlow().profile(), .err(PortableAuth.unavailable))
    }

    func testKeyLoginPrefersThePublicName() {
        exchange.reply("/v1/accounts/keys/login", status: 200, """
        {"account":{"account_id":"\(Self.accountId)","public_alias":"ana-1234","public_name":"Ana Lima","status":"active"},\
        "session":\(Self.sessionJson()),"created":false}
        """)
        let flow = makeFlow()

        guard case .ok = flow.loginWithKey(rawKey: Self.key) else {
            return XCTFail("Expected key login")
        }
        XCTAssertEqual(store.loadKey(), AuthKeyBackup(username: "Ana Lima", accountKey: Self.key))
    }

    // MARK: - Refusals

    func testReservedOrInvalidNamesAreRefusedBeforeAnyRequest() throws {
        let flow = makeFlow()
        XCTAssertEqual(flow.requestSignUpCode(name: "Suporte Abastevo", email: "ana@example.invalid", locale: "pt-BR"),
                       .err(PortableAuth.Verdict.nameInvalid))
        XCTAssertEqual(flow.requestSignUpCode(name: "É", email: "ana@example.invalid", locale: "pt-BR"),
                       .err(PortableAuth.Verdict.nameInvalid))
        XCTAssertEqual(flow.signUp(name: "Admin", email: "ana@example.invalid", code: "482916"),
                       .err(PortableAuth.Verdict.nameInvalid))
        try store.save(Self.liveSession)
        XCTAssertEqual(flow.updateName("ANP-Brasil"), .err(PortableAuth.Verdict.nameInvalid))
        XCTAssertTrue(exchange.requests.isEmpty)
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.nameInvalid), .invalidName)
    }

    func testMalformedEmailOrCodeIsRefusedLocally() {
        let flow = makeFlow()
        XCTAssertEqual(flow.requestCode(email: "not-an-address", purpose: .recovery, locale: "en"), .err(AuthFlow.clientInvalid))
        XCTAssertEqual(flow.recover(email: "ana@example.invalid", code: "48291"), .err(AuthFlow.clientInvalid))
        XCTAssertEqual(flow.signUp(name: "Ana Lima", email: "ana@example.invalid", code: "48291a"), .err(AuthFlow.clientInvalid))
        XCTAssertTrue(exchange.requests.isEmpty)
    }

    func testConsumedCodeAndLinkedAddressRefuseWithoutKeepingAnything() {
        let flow = makeFlow()
        exchange.reply("/v1/accounts/keys/signup", status: 401, Self.error("account.code-consumed"))
        XCTAssertEqual(flow.signUp(name: "Ana Lima", email: "ana@example.invalid", code: "482916"), .err(PortableAuth.Verdict.codeConsumed))
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.codeConsumed), .wrongCode)

        exchange.reply("/v1/accounts/keys/signup", status: 409, Self.error("account.address-linked"))
        XCTAssertEqual(flow.signUp(name: "Ana Lima", email: "ana@example.invalid", code: "482916"), .err(PortableAuth.Verdict.addressLinked))
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.addressLinked), .addressLinked)

        exchange.reply("/v1/accounts/recovery", status: 404, Self.error("account.recovery-no-account"))
        XCTAssertEqual(flow.recover(email: "ana@example.invalid", code: "482916"), .err(PortableAuth.Verdict.recoveryNoAccount))
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.recoveryNoAccount), .recoveryNoAccount)

        XCTAssertNil(store.load())
        XCTAssertNil(store.loadKey())
        XCTAssertEqual(flow.rehydrate(), .loggedOut)
    }

    func testThrottledSignUpExplainsTheBudget() {
        let flow = makeFlow()
        exchange.reply("/v1/accounts/keys/signup", status: 429, Self.error("account.admission-exhausted"))
        let refusal = flow.signUp(name: "Ana Lima", email: "ana@example.invalid", code: "482916")

        XCTAssertEqual(refusal, .err(PortableAuth.Verdict.admissionExhausted))
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.admissionExhausted), .throttled)
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.sessionBudgetExhausted), .throttled)
        XCTAssertNil(store.load())
    }

    func testStaleRecoveryEmailChangeAsksForAFreshSignIn() throws {
        try store.save(Self.liveSession)
        exchange.reply("/v1/accounts/recovery-email", status: 403, Self.error("account.reauthentication-required"))

        let result = makeFlow().setRecoveryEmail(email: "ana@example.invalid", code: "123456")

        XCTAssertEqual(result, .err(PortableAuth.Verdict.reauthenticationRequired))
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.reauthenticationRequired), .reauthenticationRequired)
        XCTAssertEqual(store.load(), Self.liveSession)
    }

    func testRetiredEmailLoginAndUnknownCodesMapToClearMessages() {
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.emailLoginRetired), .emailLoginRetired)
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.unavailable), .offline)
        XCTAssertEqual(AccountAccessError(verdict: "oidc-nonce-reused"), .providerDenied)
        XCTAssertEqual(AccountAccessError(verdict: PortableAuth.Verdict.providerUnavailable), .providerUnavailable)
        XCTAssertEqual(AccountAccessError(verdict: "something-new"), .unknown)

        // A stale caller of the retired route receives the verdict without the account prefix.
        let flow = makeFlow()
        exchange.reply("/v1/accounts/recovery", status: 410, Self.error("account.email-login-retired"))
        XCTAssertEqual(flow.recover(email: "ana@example.invalid", code: "482916"), .err(PortableAuth.Verdict.emailLoginRetired))
        exchange.reply("/v1/accounts/recovery", status: 429, #"{"error":{"code":"RATE_LIMITED","message":"m","trace_id":"t"}}"#)
        XCTAssertEqual(flow.recover(email: "ana@example.invalid", code: "482916"), .err(PortableAuth.unavailable))
    }

    // MARK: - Fresh proof never switches accounts

    func testKeyReauthenticationRefusesAnotherAccount() throws {
        try store.save(Self.liveSession)
        exchange.reply("/v1/accounts/keys/login", status: 200, """
        {"account":{"account_id":"another-account","public_alias":"bob","status":"active"},\
        "session":\(Self.sessionJson(account: "another-account")),"created":false}
        """)
        let flow = makeFlow()

        XCTAssertEqual(flow.loginWithKey(rawKey: Self.key, expectedAccountId: Self.accountId), .err(AuthFlow.clientInvalid))
        XCTAssertEqual(store.load(), Self.liveSession)
        XCTAssertNil(store.loadKey())
        XCTAssertEqual(flow.loginWithKey(rawKey: Self.key, expectedAccountId: "not-stored"), .err(AuthFlow.clientInvalid))
        XCTAssertEqual(exchange.requests.count, 1)
    }

    func testAppleReauthenticationKeepsTheStoredKeyAccount() throws {
        let backup = AuthKeyBackup(username: "Ana Lima", accountKey: Self.key)
        try store.save(Self.liveSession)
        try store.saveKey(backup)
        exchange.reply("/v1/accounts/providers/login", status: 200, """
        {"account":{"account_id":"\(Self.accountId)","public_alias":"ana-1234","status":"active"},\
        "session":\(Self.sessionJson()),"created":false}
        """)
        let flow = makeFlow()

        // A plain Apple sign-in still refuses while a key account is kept on the device.
        XCTAssertEqual(flow.loginWithProvider(provider: "apple", idToken: "a.b.c", rawNonce: "raw-nonce-0000001", authorizationCode: nil),
                       .err(AuthFlow.clientInvalid))
        guard case .ok(let outcome) = flow.loginWithProvider(
            provider: "apple", idToken: "a.b.c", rawNonce: "raw-nonce-0000002", authorizationCode: nil,
            expectedAccountId: Self.accountId
        ) else {
            return XCTFail("Expected Apple re-authentication")
        }
        XCTAssertEqual(outcome.session.accessToken, "access-1")
        XCTAssertEqual(store.load(), outcome.session)
        XCTAssertEqual(store.loadKey(), backup)
        XCTAssertEqual(exchange.body(0)["nonce"], "raw-nonce-0000002")
    }

    func testAppleReauthenticationForAnotherAccountIsNotStored() throws {
        try store.save(Self.liveSession)
        exchange.reply("/v1/accounts/providers/login", status: 200, """
        {"account":{"account_id":"another-account","public_alias":"bob","status":"active"},\
        "session":\(Self.sessionJson(account: "another-account")),"created":true}
        """)

        let result = makeFlow().loginWithProvider(
            provider: "apple", idToken: "a.b.c", rawNonce: "raw-nonce-0000003", authorizationCode: nil,
            expectedAccountId: Self.accountId
        )

        XCTAssertEqual(result, .err(AuthFlow.clientInvalid))
        XCTAssertEqual(store.load(), Self.liveSession)
    }
}
