import Foundation
import XCTest
@testable import AnpFuelCore

final class TrainingConsentTests: XCTestCase {
    private let origin = "https://abastevo.com.br"
    private let session = PortableAuth.Session(
        familyId: "11111111-1111-4111-8111-111111111111",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "fixture-access-token",
        refreshToken: "fixture-refresh-token",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )

    private func scope(account: String? = nil, origin: String? = nil) throws -> TrainingConsentScope {
        try XCTUnwrap(TrainingConsentScope(
            ownerScope: "account:\(account ?? session.accountId):\(IdentityVectors.fingerprint)",
            origin: origin ?? self.origin
        ))
    }

    private func fixture(_ name: String, directory: String = "api") throws -> (valid: Bool, value: Data) {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/\(directory)/\(name).json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        let document = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
        return (
            try XCTUnwrap(document["valid"] as? Bool),
            try JSONSerialization.data(withJSONObject: XCTUnwrap(document["value"]), options: .sortedKeys)
        )
    }

    func testReviewedTermsAndWireFixtures() throws {
        var review = TrainingConsentReview(scope: try scope())
        review.check(true)
        XCTAssertFalse(review.canConfirm)
        review.finishReading()
        XCTAssertFalse(review.canConfirm)
        review.check(true)
        XCTAssertTrue(review.canConfirm)
        review.finishReading()
        XCTAssertFalse(review.canConfirm)

        var local = TrainingConsentJournalState()
        try local.requestGrant(id: UUID(), serverRevision: 0)
        let grant = try XCTUnwrap(local.pending)
        let validGrant = try fixture("training-consent-grant-valid")
        XCTAssertTrue(validGrant.valid)
        XCTAssertEqual(try TrainingConsentWire.body(for: grant), validGrant.value)
        let stale = try fixture("training-consent-grant-stale-terms-invalid")
        XCTAssertFalse(stale.valid)
        XCTAssertNotEqual(try TrainingConsentWire.body(for: grant), stale.value)

        local.requestRevocation(id: UUID())
        let off = try XCTUnwrap(local.pending)
        XCTAssertThrowsError(try TrainingConsentWire.body(for: off))
        try local.bindRevocation(off, serverRevision: 1)
        let revoke = try XCTUnwrap(local.pending)
        XCTAssertEqual(try TrainingConsentWire.body(for: revoke), try fixture("training-consent-revoke-valid").value)

        let status = try TrainingConsentWire.status(from: fixture("training-consent-status-valid").value)
        XCTAssertTrue(status.granted)
        XCTAssertNotNil(status.decidedAt)
        XCTAssertEqual(status.termsDigest, TrainingTermsContract.digestSha256)
    }

    func testDomainFixturesPreserveTheOffDefaultAndRequireTheFullGrantedWireShape() throws {
        let off = try fixture("consent-status-off", directory: "privacy-consent")
        XCTAssertTrue(off.valid)
        let status = try TrainingConsentWire.status(from: off.value)
        XCTAssertFalse(status.granted)
        XCTAssertNil(status.state)
        XCTAssertEqual(status.revision, 0)
        XCTAssertEqual(try TrainingConsentWire.status(from: Data(#"{"granted":false}"#.utf8)), status)

        let granted = try fixture("consent-status-granted", directory: "privacy-consent")
        XCTAssertTrue(granted.valid)
        // The domain fixture projects display state. Live HTTP grants also carry the exact digest.
        XCTAssertThrowsError(try TrainingConsentWire.status(from: granted.value))
        for name in ["consent-grant-valid", "consent-grant-stale-terms"] {
            let policy = try fixture(name, directory: "privacy-consent")
            let decision = try XCTUnwrap(JSONSerialization.jsonObject(with: policy.value) as? [String: Any])
            XCTAssertEqual(decision["purpose"] as? String, "TRAINING")
            XCTAssertEqual(decision["state"] as? String, "GRANTED")
            XCTAssertEqual(decision["terms_version"] as? String == TrainingTermsContract.version, policy.valid)
        }
    }

    func testOutdatedPreferenceAndMalformedStatusStayDistinct() throws {
        let status = try TrainingConsentWire.status(from: Data("""
            {"granted":false,"state":"GRANTED","revision":4,"terms_version":"old","terms_digest":"\(TrainingTermsContract.digestSha256)","needs_regrant":true}
            """.utf8))
        XCTAssertTrue(status.needsRegrant)
        XCTAssertFalse(status.granted)
        for body in [
            #"{"granted":true}"#,
            #"{"granted":1}"#,
            #"{"granted":false,"revision":"4"}"#,
            #"{"granted":false,"state":"UNKNOWN","revision":1}"#,
            #"{"granted":false,"state":"REVOKED","revision":0}"#,
            #"{"granted":false,"state":"REVOKED","revision":1,"decided_at":"invalid"}"#,
        ] {
            XCTAssertThrowsError(try TrainingConsentWire.status(from: Data(body.utf8)))
        }
    }

    func testRevocationWinsGrantRaceAndPersistsExactRetryUntilConfirmedConflict() throws {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: home) }
        let journal = TrainingConsentJournal(directory: home)
        let scope = try scope()
        let grantState = try journal.update(scope) { try $0.requestGrant(id: UUID(), serverRevision: 5) }
        let grant = try XCTUnwrap(grantState.pending)
        let revocationState = try journal.update(scope) { $0.requestRevocation(id: UUID()) }
        let off = try XCTUnwrap(revocationState.pending)
        XCTAssertNil(off.revision)
        let received = try TrainingConsentWire.receipt(from: Data("""
            {"state":"GRANTED","revision":6,"terms_version":"\(TrainingTermsContract.version)","decided_at":"2026-10-11T00:00:00Z","replayed":false}
            """.utf8), command: grant)
        try journal.update(scope) { $0.acknowledge(grant, receipt: received) }
        XCTAssertEqual(try journal.load(scope).pending, off)
        XCTAssertThrowsError(try journal.update(scope) { try $0.requestGrant(id: UUID(), serverRevision: 6) })

        let boundState = try journal.update(scope) { try $0.bindRevocation(off, serverRevision: 5) }
        let first = try XCTUnwrap(boundState.pending)
        XCTAssertEqual(first.revision, 7)
        try journal.update(scope) { $0.failed(first, now: 100, retryAfterSeconds: 90) }
        let restored = TrainingConsentJournal(directory: home)
        XCTAssertEqual(try restored.load(scope).pending, first)
        XCTAssertEqual(try restored.load(scope).retryAt, 190)
        try restored.update(scope) { try $0.bindRevocation(first, serverRevision: 20) }
        XCTAssertEqual(try restored.load(scope).pending, first)

        let rebound = try restored.update(scope) {
            try $0.bindRevocation(first, serverRevision: 20, confirmedConflict: true)
        }
        XCTAssertEqual(rebound.pending?.idempotencyKey, first.idempotencyKey)
        XCTAssertEqual(rebound.pending?.revision, 21)
        XCTAssertNil(try restored.load(self.scope(account: "another")).pending)
        XCTAssertNil(try restored.load(self.scope(origin: "https://another.example")).pending)
        try restored.purge()
        XCTAssertEqual(try journal.load(scope), TrainingConsentJournalState())
        try restored.purge()
    }

    func testLogoutPurgeKeepsOnlyUndeliveredRevocations() throws {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: home) }
        let journal = TrainingConsentJournal(directory: home)
        let owed = try scope()
        let granted = try scope(account: "granted")
        let settled = try scope(account: "settled")
        try journal.update(owed) { $0.requestRevocation(id: UUID()) }
        try journal.update(granted) { try $0.requestGrant(id: UUID(), serverRevision: 2) }
        let grantState = try journal.update(settled) { try $0.requestGrant(id: UUID(), serverRevision: 0) }
        let grant = try XCTUnwrap(grantState.pending)
        let receipt = try TrainingConsentWire.receipt(from: Data("""
            {"state":"GRANTED","revision":1,"terms_version":"\(TrainingTermsContract.version)","decided_at":"2026-10-11T00:00:00Z","replayed":false}
            """.utf8), command: grant)
        try journal.update(settled) { $0.acknowledge(grant, receipt: receipt) }

        try journal.purgeKeepingPendingRevocations()

        let restored = TrainingConsentJournal(directory: home)
        XCTAssertEqual(try restored.load(owed).pending?.decision, .revoked)
        XCTAssertEqual(try restored.load(granted), TrainingConsentJournalState())
        XCTAssertEqual(try restored.load(settled), TrainingConsentJournalState())
        try restored.purge()
        XCTAssertEqual(try restored.load(owed), TrainingConsentJournalState())
        try TrainingConsentJournal(directory: home.appendingPathComponent("missing")).purgeKeepingPendingRevocations()
    }

    func testLocalFailureDoesNotAcknowledgeAnUnsavedRevocation() throws {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: home) }
        try Data().write(to: home)
        let journal = TrainingConsentJournal(directory: home)
        let scope = try scope()
        XCTAssertThrowsError(try journal.update(scope) { $0.requestRevocation(id: UUID()) }) {
            XCTAssertEqual($0 as? TrainingConsentError, .localStorage)
        }
    }

    func testReceiptRequiresMatchingDecisionRevisionAndGrantTerms() throws {
        var local = TrainingConsentJournalState()
        try local.requestGrant(id: UUID(), serverRevision: 0)
        let grant = try XCTUnwrap(local.pending)
        for body in [
            #"{"state":"GRANTED","revision":1,"terms_version":"stale","decided_at":"2026-10-11T00:00:00Z","replayed":false}"#,
            #"{"state":"REVOKED","revision":1,"decided_at":"2026-10-11T00:00:00Z","replayed":false}"#,
            """
            {"state":"GRANTED","revision":2,"terms_version":"\(TrainingTermsContract.version)","decided_at":"2026-10-11T00:00:00Z","replayed":false}
            """,
        ] {
            XCTAssertThrowsError(try TrainingConsentWire.receipt(from: Data(body.utf8), command: grant))
        }
    }

    func testPrivacyRefusalsHonorRetryAfterAndOnlyExplicitConflictCanRebind() {
        let cases: [(Int, String?, Int64?, TrainingConsentError)] = [
            (400, "privacy.terms-outdated", nil, .termsOutdated),
            (400, "privacy.bad-body", nil, .invalid),
            (401, "privacy.auth-required", nil, .signInRequired),
            (403, nil, nil, .signInRequired),
            (409, "privacy.stale-revision", nil, .staleRevision),
            (409, "privacy.conflict", nil, .conflict),
            (408, nil, nil, .unavailable(retryAfterSeconds: nil)),
            (429, nil, 99, .unavailable(retryAfterSeconds: 99)),
            (503, "privacy.unavailable", 5, .unavailable(retryAfterSeconds: 5)),
        ]
        for (status, code, retry, expected) in cases {
            let refusal = TrainingConsentError.fromTransport(
                SignedTransportError.refused(status: status, code: code, retryAfterSeconds: retry)
            )
            XCTAssertEqual(refusal, expected)
            XCTAssertEqual(refusal.canRebindRevocation, status == 409)
        }
        XCTAssertEqual(TrainingConsentError.fromTransport(SignedTransportError.offline), .offline)
    }

    func testStatusAndRevocationUseBoundProofAndStableRetryBytes() async throws {
        let sessions = StaticSessions(session: session)
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                switch request.url?.path {
                case TrainingConsentWire.statusPath: return .init(200, #"{"granted":false}"#)
                case TrainingConsentWire.revokePath:
                    return .init(200, """
                        {"state":"REVOKED","revision":1,"terms_version":"","decided_at":"2026-10-11T00:00:00Z","replayed":true,"delete_owned_training_assets":true}
                        """)
                default: return .init(404, "{}")
                }
            }
        }
        let transport = SignedRequestTransport(
            origin: origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange,
            sessions: sessions, nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let owner = try XCTUnwrap(transport.ownerScope())
        let status = try await client.trainingConsentStatus(expectedScope: owner)
        XCTAssertFalse(status.granted)
        var local = TrainingConsentJournalState()
        local.requestRevocation(id: UUID())
        try local.bindRevocation(XCTUnwrap(local.pending), serverRevision: status.revision)
        let command = try XCTUnwrap(local.pending)
        let first = try await client.writeTrainingConsent(command, expectedScope: owner)
        let second = try await client.writeTrainingConsent(command, expectedScope: owner)
        XCTAssertEqual(first, second)
        XCTAssertTrue(first.deleteOwnedTrainingAssets)
        XCTAssertTrue(second.replayed)
        let requests = exchange.requests.filter { $0.url?.path.hasPrefix(TrainingConsentWire.statusPath) == true }
        XCTAssertEqual(requests.compactMap(\.httpMethod), ["GET", "POST", "POST"])
        XCTAssertTrue(requests.allSatisfy {
            $0.value(forHTTPHeaderField: "Signature") != nil && $0.value(forHTTPHeaderField: "Cache-Control") == "no-store"
        })
        let writes = requests.dropFirst()
        XCTAssertEqual(writes.compactMap { $0.value(forHTTPHeaderField: "Idempotency-Key") }, Array(repeating: command.idempotencyKey, count: 2))
        XCTAssertEqual(writes.compactMap(\.httpBody), Array(repeating: try TrainingConsentWire.body(for: command), count: 2))
        XCTAssertEqual(exchange.requests.filter {
            $0.url?.path == "/v1/identity/challenges" && FakeIdentityBackend.json($0)["purpose"] as? String == "SIGN"
        }.count, 3)

        sessions.set(nil)
        let previous = exchange.requests.count
        do {
            _ = try await client.trainingConsentStatus(expectedScope: owner)
            XCTFail("A missing owner must refuse before a request")
        } catch {
            XCTAssertEqual(error as? TrainingConsentError, .signInRequired)
        }
        XCTAssertEqual(exchange.requests.count, previous)
    }
}
