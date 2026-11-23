import Foundation
import XCTest
#if canImport(CryptoKit)
import CryptoKit
#endif
@testable import AnpFuelCore

/// Golden vectors copied verbatim from `contracts/testdata/identity` (P03-T01 frozen profile, verified by the Go
/// backend). `scripts/tests/test_ios_app_integrity.py` checks that these values still match the fixtures.
enum IdentityVectors {
    struct Tampered {
        let id: String
        let lines: [String]
        let signedLines: [String]
        let signature: String
    }

    static let jwkX = "5H8pdbqwCE7YcVh0t7VNWgcQVrZ4S66TjfwBDE8tneE"
    static let jwkY = "WpcgI0Z8gndT_z9lTn8qgoevjDFsjVAlRvfpo6-KJIs"
    static let privateD = "cI6mxcfbMMyuYzlei_T9-vnKFMRKkIhNfDXUHpESsA4"
    static let fingerprint = "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624"

    static let registerLines = ["\"@method\": POST", "\"@authority\": api.example.invalid", "\"@path\": /v1/contributors", "\"@query\": ", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""]
    static let registerSignature = "BcgVfGY2KhIi2Zt0mWS8wgV6yXNRKlka0AVkictMFbXfK1R1T3ASckgBNQApGAOkFziuuD6hYLG4-KR6fjSbyg"

    static let bodyLines = ["\"@method\": POST", "\"@authority\": api.example.invalid", "\"@path\": /v1/contributors", "\"@query\": ", "\"content-type\": application/json", "\"content-digest\": \"sha-512=:MEUCIQ==:\"", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""]
    static let bodySignature = "JSkXdMbQcTm7sA0yGD2xtEnc4UzQTCpcKxrQUTcsn21afCNQ_0Y-rM52rwGaVvl-tVg2GWDKrbGcgSN2WEt_8Q"

    static let derSignature = "MEYCIQDTMAdvnHoF_krjpzfGClOvsHPkmIKuT5SQpC-Brd---wIhALtM7uqO0OH9U2xvjhr730uHkOtq-kerAw-bgjGqVJuh"

    static let tampered: [Tampered] = [
        Tampered(
            id: "auth-tampered-method",
            lines: ["\"@method\": GET", "\"@authority\": api.example.invalid", "\"@path\": /v1/contributors", "\"@query\": ", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""],
            signedLines: registerLines,
            signature: "pBpFiRu1ZIE5NVKdJB53iblW-tCEYu5VrXKjWmDcCuEMYNO9V2QdY0w6A3HibP4C1hv0FE8wADmWmwxXU8VAPA"
        ),
        Tampered(
            id: "auth-tampered-authority",
            lines: ["\"@method\": POST", "\"@authority\": evil.example", "\"@path\": /v1/contributors", "\"@query\": ", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""],
            signedLines: registerLines,
            signature: "LtekSU_UZ9nS587eW5mtgvc8O_JzSzc8RdQgnls1vnxEVSEsP-0cv1avM2tOA7zHtL9T1FGx0RhpPS0Qq1GQ6w"
        ),
        Tampered(
            id: "auth-tampered-path",
            lines: ["\"@method\": POST", "\"@authority\": api.example.invalid", "\"@path\": /v1/observations", "\"@query\": ", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""],
            signedLines: registerLines,
            signature: "3FwqTViv9YoW5V8ti7IfF4o2BdrEEt1uSXzTnIorw3QY4utWKlNKuFbiop4aTks3Nlm2AZL9YTNguMRWdryf2Q"
        ),
        Tampered(
            id: "auth-tampered-query",
            lines: ["\"@method\": POST", "\"@authority\": api.example.invalid", "\"@path\": /v1/contributors", "\"@query\": a=b", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""],
            signedLines: registerLines,
            signature: "DI2IZbgq1ZHD_c-txK0bDJDy0PSEbp-ffKPcgH8AlvgyE5WIE5q-fCjWV6ZFiInNHN37YtT9VamrkGwKEXkDeA"
        ),
        Tampered(
            id: "auth-tampered-nonce",
            lines: ["\"@method\": POST", "\"@authority\": api.example.invalid", "\"@path\": /v1/contributors", "\"@query\": ", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-9.client-9\""],
            signedLines: registerLines,
            signature: "miGl8kRJl-3-tR50KhKQMGzb4qHmlMrRSjxWTxd6u5p-4Y7TMiAuQc0xZwIjcYmKJp04cfNmUQEi1-VPVWRwrA"
        ),
        Tampered(
            id: "auth-tampered-body",
            lines: ["\"@method\": POST", "\"@authority\": api.example.invalid", "\"@path\": /v1/contributors", "\"@query\": ", "\"content-type\": application/json", "\"content-digest\": \"sha-512=:OTHER==:\"", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""],
            signedLines: bodyLines,
            signature: "d_7LU8W9H4HJiAR-7U4QCRZ_Iah9A-kYU_rGVEB1yB8XVCQhS67bnxKjOTIWrJgQCM7Dor8bpxTBowC19g91CA"
        ),
        Tampered(
            id: "auth-reordered",
            lines: ["\"@authority\": api.example.invalid", "\"@method\": POST", "\"@path\": /v1/contributors", "\"@query\": ", "\"created\": 1735689600", "\"expires\": 1735689900", "\"keyid\": \"fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624\"", "\"nonce\": \"ch-1.client-1\""],
            signedLines: registerLines,
            signature: "amJMquuOHRYbRh7JX-r4AeZfNa8EnaMZL0h3tyZiTuJlCNhrsnMiCYnoQmpw2aAjXQopM6KAhhQmVRGVAf8SMQ"
        ),
    ]

    static func signer() throws -> IosDeviceProofSigner {
        let raw = try XCTUnwrap(PortableBase64Url.decode(privateD))
        return IosDeviceProofSigner(service: "test.vectors", accountKey: UUID().uuidString, privateKeyRaw: raw)
    }

    static func publicKey() throws -> P256.Signing.PublicKey {
        let x = try XCTUnwrap(PortableBase64Url.decode(jwkX))
        let y = try XCTUnwrap(PortableBase64Url.decode(jwkY))
        return try P256.Signing.PublicKey(rawRepresentation: x + y)
    }

    /// Backend rule: SHA-512 over the newline-joined base, raw 64-byte r||s signature.
    static func verify(lines: [String], signature: String) throws -> Bool {
        guard let raw = PortableBase64Url.decode(signature), raw.count == 64 else { return false }
        let parsed = try P256.Signing.ECDSASignature(rawRepresentation: raw)
        let digest = SHA512.hash(data: Data(PortableAnonymousProof.buildBase(lines).utf8))
        return try publicKey().isValidSignature(parsed, for: digest)
    }

    /// Backend `keyprover` rule: SHA-256 over the binding statement, raw 64-byte r||s signature.
    static func verifyBinding(statement: String, signature: String) throws -> Bool {
        guard let raw = PortableBase64Url.decode(signature), raw.count == 64 else { return false }
        let parsed = try P256.Signing.ECDSASignature(rawRepresentation: raw)
        return try publicKey().isValidSignature(parsed, for: SHA256.hash(data: Data(statement.utf8)))
    }
}

final class SignedRequestProfileTests: XCTestCase {
    func testVectorKeyDerivesTheVectorCoordinatesAndFingerprint() throws {
        let jwk = try XCTUnwrap(IdentityVectors.signer().ensurePublicJwk())
        XCTAssertEqual(jwk.x, IdentityVectors.jwkX)
        XCTAssertEqual(jwk.y, IdentityVectors.jwkY)
        XCTAssertEqual(jwk.fingerprint, IdentityVectors.fingerprint)
    }

    func testGoSignedVectorsVerifyOverSha512() throws {
        XCTAssertTrue(try IdentityVectors.verify(lines: IdentityVectors.registerLines, signature: IdentityVectors.registerSignature))
        XCTAssertTrue(try IdentityVectors.verify(lines: IdentityVectors.bodyLines, signature: IdentityVectors.bodySignature))
    }

    func testTamperedVectorsFailWhileTheirSignaturesStillCoverTheOriginalBase() throws {
        for vector in IdentityVectors.tampered {
            XCTAssertFalse(try IdentityVectors.verify(lines: vector.lines, signature: vector.signature), vector.id)
            XCTAssertTrue(try IdentityVectors.verify(lines: vector.signedLines, signature: vector.signature), vector.id)
        }
    }

    func testDerEncodedSignatureIsRefused() throws {
        XCTAssertFalse(try IdentityVectors.verify(lines: IdentityVectors.registerLines, signature: IdentityVectors.derSignature))
    }

    func testProfileRebuildsTheGoldenBaseAndCoversBodiesOnlyWhenPresent() {
        let register = SignedRequestProfile.baseLines(
            method: "post",
            authority: "api.example.invalid",
            path: "/v1/contributors",
            query: "",
            body: nil,
            created: 1_735_689_600,
            expires: 1_735_689_900,
            keyId: IdentityVectors.fingerprint,
            nonce: "ch-1.client-1"
        )
        XCTAssertEqual(register, IdentityVectors.registerLines)

        let withBody = SignedRequestProfile.baseLines(
            method: "POST",
            authority: "api.example.invalid",
            path: "/v1/contributors",
            query: "",
            body: Data("{}".utf8),
            created: 1_735_689_600,
            expires: 1_735_689_900,
            keyId: IdentityVectors.fingerprint,
            nonce: "ch-1.client-1"
        )
        XCTAssertEqual(withBody.count, 10)
        XCTAssertEqual(withBody[4], "\"content-type\": application/json")
        XCTAssertEqual(withBody[5], "\"content-digest\": \"sha-512=:J8dGcK23UHX60FjVzq97IMTneGyDuuijL2Jvl4KvNMmjPCBG72D9Knh403jin+yFGAa72aZ4ePOp8c2kgwdj/Q==:\"")
        XCTAssertEqual(Array(withBody[0..<4]), Array(register[0..<4]))
        XCTAssertEqual(Array(withBody[6..<10]), Array(register[4..<8]))

        let emptyBody = SignedRequestProfile.baseLines(
            method: "POST",
            authority: "api.example.invalid",
            path: "/v1/contributors",
            query: "",
            body: Data(),
            created: 1_735_689_600,
            expires: 1_735_689_900,
            keyId: IdentityVectors.fingerprint,
            nonce: "ch-1.client-1"
        )
        XCTAssertEqual(emptyBody, register)
    }

    func testProductionSignerOutputVerifiesAsRawUnpaddedBase64Url() throws {
        let signature = try XCTUnwrap(IdentityVectors.signer().signProofLines(IdentityVectors.bodyLines))
        XCTAssertFalse(signature.contains("="))
        XCTAssertFalse(signature.contains("+"))
        XCTAssertFalse(signature.contains("/"))
        XCTAssertEqual(PortableBase64Url.decode(signature)?.count, 64)
        XCTAssertTrue(try IdentityVectors.verify(lines: IdentityVectors.bodyLines, signature: signature))
        XCTAssertNil(try IdentityVectors.signer().signProofLines(Array(IdentityVectors.registerLines.reversed())))
    }

    func testBindingStatementSignatureVerifiesOverSha256() throws {
        let statement = PortableAccountBinding.statement(
            accountId: "a0000000-0000-4000-8000-000000000001",
            contributorId: "c0000000-0000-4000-8000-000000000001",
            expiry: 1_735_689_720
        )
        let signature = try XCTUnwrap(IdentityVectors.signer().signBindingStatement(statement))
        XCTAssertTrue(try IdentityVectors.verifyBinding(statement: statement, signature: signature))
        XCTAssertFalse(try IdentityVectors.verifyBinding(statement: statement + "x", signature: signature))
        let proof = try XCTUnwrap(PortableAccountBinding.proof(fingerprint: IdentityVectors.fingerprint, expiry: 1_735_689_720, signatureB64Url: signature))
        XCTAssertEqual(proof.split(separator: ".").count, 3)
        XCTAssertTrue(proof.hasPrefix(IdentityVectors.fingerprint + ".1735689720."))
    }

    func testAuthorityHeadersPathsAndErrorParsing() {
        XCTAssertEqual(SignedRequestProfile.authority(origin: "https://abastevo.com.br"), "abastevo.com.br")
        XCTAssertEqual(SignedRequestProfile.authority(origin: "https://abastevo.com.br/"), "abastevo.com.br")
        XCTAssertEqual(SignedRequestProfile.authority(origin: "http://127.0.0.1:18080"), "127.0.0.1:18080")
        XCTAssertNil(SignedRequestProfile.authority(origin: "ftp://abastevo.com.br"))
        XCTAssertNil(SignedRequestProfile.authority(origin: "https://user:secret@abastevo.com.br"))

        XCTAssertEqual(
            SignedRequestProfile.headers(signature: "sig", created: 10, expires: 130, keyId: "fp:k", nonce: "c.n"),
            [
                "Signature": "sig",
                "Signature-Created": "10",
                "Signature-Expires": "130",
                "Signature-Keyid": "fp:k",
                "Signature-Nonce": "c.n",
            ]
        )
        XCTAssertTrue(SignedRequestProfile.isSignablePath("/v1/observations"))
        XCTAssertFalse(SignedRequestProfile.isSignablePath("/v1/observations?limit=1"))
        XCTAssertFalse(SignedRequestProfile.isSignablePath("/v2/observations"))
        XCTAssertFalse(SignedRequestProfile.isSignablePath("/v1/obs ervations"))
        XCTAssertTrue(SignedRequestProfile.isSignableQuery("limit=50"))
        XCTAssertFalse(SignedRequestProfile.isSignableQuery("limit=50 OR 1"))

        let refusal = Data("{\"error\":{\"code\":\"community.quota-exceeded\",\"message\":\"quota\"}}".utf8)
        XCTAssertEqual(SignedRequestProfile.errorCode(from: refusal), "community.quota-exceeded")
        XCTAssertNil(SignedRequestProfile.errorCode(from: Data("{\"error\":{\"code\":\"Not Stable\"}}".utf8)))
        XCTAssertNil(SignedRequestProfile.errorCode(from: Data("not json".utf8)))
        XCTAssertEqual(SignedRequestProfile.retryAfterSeconds("120", nowEpochSeconds: 0), 120)
        XCTAssertEqual(SignedRequestProfile.retryAfterSeconds("Wed, 21 Oct 2015 07:28:00 GMT", nowEpochSeconds: 1_445_412_400), 80)
        XCTAssertNil(SignedRequestProfile.retryAfterSeconds("soon", nowEpochSeconds: 0))
        XCTAssertNil(SignedRequestProfile.retryAfterSeconds(nil, nowEpochSeconds: 0))
    }
}

/// Records every request and answers from a scripted backend.
final class ScriptedExchange: SignedHttpExchange, @unchecked Sendable {
    struct Reply {
        let status: Int
        let body: String
        let headers: [String: String]

        init(_ status: Int, _ body: String, headers: [String: String] = [:]) {
            self.status = status
            self.body = body
            self.headers = headers
        }
    }

    private let lock = NSLock()
    private var recorded: [URLRequest] = []
    private let respond: @Sendable (URLRequest) -> Reply

    init(respond: @escaping @Sendable (URLRequest) -> Reply) {
        self.respond = respond
    }

    var requests: [URLRequest] {
        lock.withLock { recorded }
    }

    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        lock.withLock { recorded.append(request) }
        let reply = respond(request)
        guard let url = request.url,
              let response = HTTPURLResponse(url: url, statusCode: reply.status, httpVersion: "HTTP/1.1", headerFields: reply.headers) else {
            throw URLError(.badServerResponse)
        }
        return (Data(reply.body.utf8), response)
    }
}

final class StaticSessions: SignedSessionSource, @unchecked Sendable {
    private let lock = NSLock()
    private var session: PortableAuth.Session?

    init(session: PortableAuth.Session?) {
        self.session = session
    }

    func set(_ next: PortableAuth.Session?) {
        lock.withLock { session = next }
    }

    func currentAccountId() -> String? {
        lock.withLock { session?.accountId }
    }

    func liveSession() async -> PortableAuth.Session? {
        lock.withLock { session }
    }
}

/// Minimal identity backend: issues challenges bound to the presented fingerprint, registers and binds.
enum FakeIdentityBackend {
    static let registerChallengeId = "11111111-1111-4111-8111-111111111111"
    static let signChallengeId = "22222222-2222-4222-8222-222222222222"
    static let contributorId = "c0000000-0000-4000-8000-000000000001"
    static let keyId = "aac027be-d331-40e4-8d63-52ec3b8d2f41"

    static func json(_ request: URLRequest) -> [String: Any] {
        guard let body = request.httpBody,
              let object = (try? JSONSerialization.jsonObject(with: body)) as? [String: Any] else {
            return [:]
        }
        return object
    }

    static func reply(_ request: URLRequest, fallback: (URLRequest) -> ScriptedExchange.Reply) -> ScriptedExchange.Reply {
        let path = request.url?.path ?? ""
        let body = json(request)
        switch path {
        case "/v1/identity/challenges":
            let purpose = body["purpose"] as? String ?? ""
            let fingerprint = body["fingerprint"] as? String ?? ""
            let id = purpose == "REGISTER" ? registerChallengeId : signChallengeId
            return ScriptedExchange.Reply(201, "{\"challenge_id\":\"\(id)\",\"nonce\":\"\(id).abc123\",\"fingerprint\":\"\(fingerprint)\",\"purpose\":\"\(purpose)\",\"expires_at\":\"2025-11-23T10:05:00Z\"}")
        case "/v1/contributors":
            return ScriptedExchange.Reply(201, "{\"contributor_id\":\"\(contributorId)\",\"key_id\":\"\(keyId)\",\"existed\":false}")
        case "/v1/accounts/bindings/bind":
            return ScriptedExchange.Reply(200, "{\"binding\":{\"contributor_id\":\"\(contributorId)\",\"key_fingerprint\":\"\(IdentityVectors.fingerprint)\",\"bound_at\":\"2025-11-23T10:00:00Z\"}}")
        default:
            return fallback(request)
        }
    }
}

final class SignedRequestTransportTests: XCTestCase {
    private let now: Int64 = 1_763_892_000
    private let session = PortableAuth.Session(
        familyId: "f0000000-0000-4000-8000-000000000001",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "access-token",
        refreshToken: "refresh-token",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )

    private func makeTransport(
        sessions: StaticSessions,
        store: IosDeviceIdentityKeychainStore,
        exchange: ScriptedExchange
    ) throws -> SignedRequestTransport {
        let clock = now
        return SignedRequestTransport(
            origin: "https://abastevo.com.br",
            signer: try IdentityVectors.signer(),
            identityStore: store,
            exchange: exchange,
            sessions: sessions,
            nowEpochSeconds: { clock }
        )
    }

    func testSignedPostRegistersBindsAndSignsTheExactBody() async throws {
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { _ in
                ScriptedExchange.Reply(201, "{\"id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad27\",\"validation_state\":\"RECEIVED\"}")
            }
        }
        let store = IosDeviceIdentityKeychainStore(useMemoryOnly: true)
        let transport = try makeTransport(sessions: StaticSessions(session: session), store: store, exchange: exchange)
        let body = Data("{\"a\":1}".utf8)

        let response = try await transport.signedRequest(method: "POST", path: "/v1/observations", body: body, idempotencyKey: "cmd-1")

        XCTAssertEqual(response.status, 201)
        let requests = exchange.requests
        XCTAssertEqual(requests.map { $0.url?.path ?? "" }, [
            "/v1/identity/challenges",
            "/v1/contributors",
            "/v1/accounts/bindings/bind",
            "/v1/identity/challenges",
            "/v1/observations",
        ])
        XCTAssertEqual(FakeIdentityBackend.json(requests[0])["purpose"] as? String, "REGISTER")
        XCTAssertEqual(FakeIdentityBackend.json(requests[3])["purpose"] as? String, "SIGN")

        let registration = FakeIdentityBackend.json(requests[1])
        XCTAssertEqual(registration["challenge_id"] as? String, FakeIdentityBackend.registerChallengeId)
        let jwk = try XCTUnwrap(registration["public_jwk"] as? [String: Any])
        XCTAssertEqual(jwk["kty"] as? String, "EC")
        XCTAssertEqual(jwk["crv"] as? String, "P-256")
        XCTAssertEqual(jwk["x"] as? String, IdentityVectors.jwkX)
        XCTAssertEqual(jwk["y"] as? String, IdentityVectors.jwkY)
        let proof = try XCTUnwrap(registration["proof"] as? [String: Any])
        let proofLines = try XCTUnwrap(proof["lines"] as? [String])
        XCTAssertEqual(proofLines, [
            "\"@method\": POST",
            "\"@authority\": abastevo.com.br",
            "\"@path\": /v1/contributors",
            "\"@query\": ",
            "\"created\": 1763892000",
            "\"expires\": 1763892120",
            "\"keyid\": \"\(IdentityVectors.fingerprint)\"",
            "\"nonce\": \"\(FakeIdentityBackend.registerChallengeId).abc123\"",
        ])
        XCTAssertTrue(try IdentityVectors.verify(lines: proofLines, signature: proof["signature"] as? String ?? ""))

        let binding = FakeIdentityBackend.json(requests[2])
        XCTAssertEqual(binding["family_id"] as? String, session.familyId)
        XCTAssertEqual(binding["access_token"] as? String, session.accessToken)
        XCTAssertEqual(binding["contributor_id"] as? String, FakeIdentityBackend.contributorId)
        let bindingProof = try XCTUnwrap(binding["proof"] as? String)
        let parts = bindingProof.split(separator: ".").map(String.init)
        XCTAssertEqual(parts.count, 3)
        XCTAssertEqual(parts.first, IdentityVectors.fingerprint)
        XCTAssertEqual(parts.dropFirst().first, "1763892120")
        let statement = PortableAccountBinding.statement(
            accountId: session.accountId,
            contributorId: FakeIdentityBackend.contributorId,
            expiry: 1_763_892_120
        )
        XCTAssertTrue(try IdentityVectors.verifyBinding(statement: statement, signature: parts.last ?? ""))

        let signed = requests[4]
        XCTAssertEqual(signed.httpMethod, "POST")
        XCTAssertEqual(signed.httpBody, body)
        XCTAssertEqual(signed.value(forHTTPHeaderField: "Content-Type"), "application/json")
        XCTAssertEqual(signed.value(forHTTPHeaderField: "Idempotency-Key"), "cmd-1")
        XCTAssertEqual(signed.value(forHTTPHeaderField: "Signature-Keyid"), IdentityVectors.fingerprint)
        XCTAssertEqual(signed.value(forHTTPHeaderField: "Signature-Created"), "1763892000")
        XCTAssertEqual(signed.value(forHTTPHeaderField: "Signature-Expires"), "1763892120")
        let signNonce = FakeIdentityBackend.signChallengeId + ".abc123"
        XCTAssertEqual(signed.value(forHTTPHeaderField: "Signature-Nonce"), signNonce)
        let rebuilt = SignedRequestProfile.baseLines(
            method: "POST",
            authority: "abastevo.com.br",
            path: "/v1/observations",
            query: "",
            body: body,
            created: 1_763_892_000,
            expires: 1_763_892_120,
            keyId: IdentityVectors.fingerprint,
            nonce: signNonce
        )
        XCTAssertTrue(try IdentityVectors.verify(lines: rebuilt, signature: signed.value(forHTTPHeaderField: "Signature") ?? ""))
        XCTAssertEqual(store.load()?.contributorId, FakeIdentityBackend.contributorId)
        XCTAssertEqual(store.load()?.keyId, FakeIdentityBackend.keyId)
        XCTAssertEqual(store.load()?.origin, "https://abastevo.com.br")

        _ = try await transport.signedRequest(method: "GET", path: "/v1/contributors/me/observations", query: "limit=5", body: nil)
        let later = exchange.requests
        XCTAssertEqual(later.count, 7, "registration and binding are reused within the process")
        let get = later[6]
        XCTAssertEqual(get.httpMethod, "GET")
        XCTAssertEqual(get.url?.query, "limit=5")
        XCTAssertNil(get.httpBody)
        XCTAssertNil(get.value(forHTTPHeaderField: "Content-Type"))
        let getLines = SignedRequestProfile.baseLines(
            method: "GET",
            authority: "abastevo.com.br",
            path: "/v1/contributors/me/observations",
            query: "limit=5",
            body: nil,
            created: 1_763_892_000,
            expires: 1_763_892_120,
            keyId: IdentityVectors.fingerprint,
            nonce: signNonce
        )
        XCTAssertEqual(getLines.count, 8)
        XCTAssertTrue(try IdentityVectors.verify(lines: getLines, signature: get.value(forHTTPHeaderField: "Signature") ?? ""))
    }

    func testAnonymousContributorNeverBindsAndOwnerScopeIsAnonymous() async throws {
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { _ in ScriptedExchange.Reply(200, "{}") }
        }
        let sessions = StaticSessions(session: nil)
        let transport = try makeTransport(sessions: sessions, store: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange)

        XCTAssertEqual(transport.ownerScope(), "anonymous:" + IdentityVectors.fingerprint)
        _ = try await transport.signedRequest(method: "POST", path: "/v1/photo-captures", body: Data("{}".utf8), idempotencyKey: "k")

        XCTAssertFalse(exchange.requests.contains { $0.url?.path == "/v1/accounts/bindings/bind" })
        sessions.set(session)
        XCTAssertEqual(transport.ownerScope(), "account:\(session.accountId):\(IdentityVectors.fingerprint)")
    }

    func testRefusalCarriesCodeAndRetryAfterAndUnauthorizedForcesReproof() async throws {
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                if request.url?.path == "/v1/observations" {
                    return ScriptedExchange.Reply(401, "{\"error\":{\"code\":\"community.auth-required\"}}")
                }
                return ScriptedExchange.Reply(429, "{\"error\":{\"code\":\"community.quota-exceeded\"}}", headers: ["Retry-After": "30"])
            }
        }
        let transport = try makeTransport(sessions: StaticSessions(session: nil), store: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange)

        do {
            _ = try await transport.signedRequest(method: "POST", path: "/v1/uploads", body: Data("{}".utf8), idempotencyKey: "photo:1")
            XCTFail("Expected a refusal")
        } catch let error as SignedTransportError {
            XCTAssertEqual(error, .refused(status: 429, code: "community.quota-exceeded", retryAfterSeconds: 30))
        }
        do {
            _ = try await transport.signedRequest(method: "POST", path: "/v1/observations", body: Data("{}".utf8), idempotencyKey: "cmd")
            XCTFail("Expected a refusal")
        } catch let error as SignedTransportError {
            XCTAssertEqual(error, .refused(status: 401, code: "community.auth-required", retryAfterSeconds: nil))
        }
        let before = exchange.requests.filter { $0.url?.path == "/v1/contributors" }.count
        _ = try? await transport.signedRequest(method: "POST", path: "/v1/uploads", body: Data("{}".utf8), idempotencyKey: "photo:2")
        let after = exchange.requests.filter { $0.url?.path == "/v1/contributors" }.count
        XCTAssertEqual(before, 1)
        XCTAssertEqual(after, 2, "a 401 forgets the registration so the next request proves it again")
    }

    func testInvalidRequestsNeverReachTheNetwork() async throws {
        let exchange = ScriptedExchange { _ in ScriptedExchange.Reply(200, "{}") }
        let transport = try makeTransport(sessions: StaticSessions(session: nil), store: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange)

        for (method, path, query) in [("POST", "/v2/observations", ""), ("DELETE", "/v1/observations", ""), ("GET", "/v1/x?y=1", ""), ("GET", "/v1/x", "a=b c")] {
            do {
                _ = try await transport.signedRequest(method: method, path: path, query: query, body: nil)
                XCTFail("Expected invalid request for \(path)")
            } catch let error as SignedTransportError {
                XCTAssertEqual(error, .invalidRequest)
            }
        }
        XCTAssertTrue(exchange.requests.isEmpty)
    }

    func testChallengeReplyMustEchoFingerprintAndBindTheNonce() {
        let fingerprint = IdentityVectors.fingerprint
        let valid = Data("{\"challenge_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\",\"nonce\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26.9f2c4a7e8b1d3f5a\",\"fingerprint\":\"\(fingerprint)\",\"purpose\":\"REGISTER\",\"expires_at\":\"2026-09-28T00:05:00Z\"}".utf8)
        XCTAssertEqual(
            DeviceIdentityWire.decodeChallenge(valid, fingerprint: fingerprint, purpose: "REGISTER"),
            IdentityChallenge(
                challengeId: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
                nonce: "d6c74c23-63db-4c24-a2e5-408cb23bad26.9f2c4a7e8b1d3f5a",
                fingerprint: fingerprint,
                purpose: "REGISTER"
            )
        )
        XCTAssertNil(DeviceIdentityWire.decodeChallenge(valid, fingerprint: fingerprint, purpose: "SIGN"))
        XCTAssertNil(DeviceIdentityWire.decodeChallenge(valid, fingerprint: "fp:" + String(repeating: "0", count: 64), purpose: "REGISTER"))
        let foreignNonce = Data("{\"challenge_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\",\"nonce\":\"other.9f2c\",\"fingerprint\":\"\(fingerprint)\",\"purpose\":\"REGISTER\"}".utf8)
        XCTAssertNil(DeviceIdentityWire.decodeChallenge(foreignNonce, fingerprint: fingerprint, purpose: "REGISTER"))
        XCTAssertNil(DeviceIdentityWire.challengeBody(fingerprint: "fp:bad", purpose: "REGISTER"))
        XCTAssertNil(DeviceIdentityWire.challengeBody(fingerprint: fingerprint, purpose: "ADMIN"))
    }

    func testRegistrationAndBindingWireShapes() throws {
        let registration = Data("{\"contributor_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\",\"key_id\":\"aac027be-d331-40e4-8d63-52ec3b8d2f41\",\"existed\":false}".utf8)
        XCTAssertEqual(
            DeviceIdentityWire.decodeRegistration(registration),
            ContributorRegistration(contributorId: "d6c74c23-63db-4c24-a2e5-408cb23bad26", keyId: "aac027be-d331-40e4-8d63-52ec3b8d2f41", existed: false)
        )
        XCTAssertNil(DeviceIdentityWire.decodeRegistration(Data("{\"contributor_id\":\"x\",\"key_id\":\"y\"}".utf8)))

        let binding = Data("{\"binding\":{\"bound_at\":\"2026-10-01T00:00:00Z\",\"contributor_id\":\"c0000000-0000-4000-8000-000000000001\",\"key_fingerprint\":\"fp:9f2c4a1b3d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8\"}}".utf8)
        XCTAssertEqual(DeviceIdentityWire.decodeBinding(binding)?.contributorId, "c0000000-0000-4000-8000-000000000001")
        XCTAssertNil(DeviceIdentityWire.decodeBinding(Data("{\"binding\":{\"contributor_id\":\"c\",\"key_fingerprint\":\"fp:short\"}}".utf8)))

        let body = try XCTUnwrap(DeviceIdentityWire.bindingBody(familyId: "f", accessToken: "t", contributorId: "c", proof: "p"))
        let object = try XCTUnwrap((try JSONSerialization.jsonObject(with: body)) as? [String: String])
        XCTAssertEqual(object, ["family_id": "f", "access_token": "t", "contributor_id": "c", "proof": "p"])
        XCTAssertNil(DeviceIdentityWire.bindingBody(familyId: "", accessToken: "t", contributorId: "c", proof: "p"))
        XCTAssertNil(DeviceIdentityWire.registrationBody(jwkX: "x", jwkY: "y", challengeId: "c", lines: ["bad"], signature: "s"))
    }

    func testIdentityRecordStoreRoundTripAndClear() throws {
        let store = IosDeviceIdentityKeychainStore(useMemoryOnly: true)
        XCTAssertNil(store.load())
        let record = DeviceIdentityRecord(origin: "https://abastevo.com.br", fingerprint: IdentityVectors.fingerprint, contributorId: "c", keyId: "k")
        try store.save(record)
        XCTAssertEqual(store.load(), record)
        store.clear()
        XCTAssertNil(store.load())
    }

    func testAppleNonceIsRandomAndHashedAsLowercaseHex() throws {
        let first = try XCTUnwrap(AppleSignInNonce.makeRawNonce())
        let second = try XCTUnwrap(AppleSignInNonce.makeRawNonce())
        XCTAssertNotEqual(first, second)
        XCTAssertEqual(PortableBase64Url.decode(first)?.count, 32)
        XCTAssertEqual(AppleSignInNonce.requestNonce(forRaw: "abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
        XCTAssertNil(AppleSignInNonce.makeRawNonce(byteCount: 8))

        let body = AccountProviderLogin.body(provider: "apple", idToken: "tok", rawNonce: "raw", authorizationCode: "")
        XCTAssertEqual(body["provider"] as? String, "apple")
        XCTAssertEqual(body["id_token"] as? String, "tok")
        XCTAssertEqual(body["nonce"] as? String, "raw")
        XCTAssertNil(body["authorization_code"])
        XCTAssertEqual(AccountProviderLogin.body(provider: "apple", idToken: "tok", rawNonce: "raw", authorizationCode: "code")["authorization_code"] as? String, "code")
        XCTAssertEqual(AccountProviderLogin.path, "/v1/accounts/providers/login")
    }
}
