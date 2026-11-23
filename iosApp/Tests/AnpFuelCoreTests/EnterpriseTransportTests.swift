import XCTest
@testable import AnpFuelCore

/// Every company command travels on the device-signed plane; the cookie plane only reads
/// contexts, invitations and checklists and never carries device proof.
final class EnterpriseTransportTests: XCTestCase {
    private let session = PortableAuth.Session(
        familyId: "f0000000-0000-4000-8000-000000000001",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "access-token",
        refreshToken: "refresh-token",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )
    private let origin = "https://abastevo.com.br"
    private let org = "7b0e2a54-0000-4000-8000-000000000101"

    private func makeClient(_ exchange: ScriptedExchange, sessions: StaticSessions) throws -> EnterpriseSyncClient {
        let transport = SignedRequestTransport(
            origin: origin,
            signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true),
            exchange: exchange,
            sessions: sessions,
            nowEpochSeconds: { 1_763_892_000 }
        )
        return EnterpriseSyncClient(transport: transport, sessions: sessions, uploads: exchange,
                                    newId: { "7b0e2a54-0000-4000-8000-000000000999" })
    }

    private func operation(_ partition: EnterprisePartition) throws -> EnterpriseSealedOperation {
        let envelope = EnterpriseEnvelope(
            partition: partition,
            membershipId: "7b0e2a54-0000-4000-8000-0000000000c9",
            enrollmentId: "7b0e2a54-0000-4000-8000-00000000012d",
            operationId: "7b0e2a54-0000-4000-8000-000000000191",
            declared: EnterpriseTimeDeclaration(occurredAt: "2026-11-15T12:00:00Z", entryZone: "America/Sao_Paulo", deviceOffsetSeconds: -10_800)
        )
        return try EnterpriseOperationBuilder.submitExpense(
            envelope: envelope, expenseId: "7b0e2a54-0000-4000-8000-000000000201", assetId: "7b0e2a54-0000-4000-8000-000000009721",
            category: "TOLL", paidCentavos: 1250, companyCentavos: 1250, payer: .company
        )
    }

    private static func acknowledgement(for request: URLRequest) -> ScriptedExchange.Reply {
        guard let body = FakeIdentityBackend.json(request)["operations"] as? [[String: Any]],
              let first = body.first,
              let id = first["operation_id"] as? String,
              let digest = first["payload_digest"] as? String else {
            return ScriptedExchange.Reply(400, #"{"error":{"code":"MALFORMED"}}"#)
        }
        return ScriptedExchange.Reply(200, #"{"schema_version":1,"acknowledgements":[{"schema_version":1,"operation_id":"\#(id)","payload_digest":"\#(digest)","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-15T12:00:01Z","review_item_id":null,"warnings":[]}]}"#)
    }

    func testOperationsAreDeliveredSignedWithTheAccountProofInTheBody() async throws {
        let path = "/v1/orgs/\(org)/sync/operations"
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                request.url?.path == "/v1/orgs/7b0e2a54-0000-4000-8000-000000000101/sync/operations"
                    ? EnterpriseTransportTests.acknowledgement(for: request)
                    : ScriptedExchange.Reply(404, #"{"error":{"code":"NOT_FOUND"}}"#)
            }
        }
        let sessions = StaticSessions(session: session)
        let client = try makeClient(exchange, sessions: sessions)
        let partition = try XCTUnwrap(EnterprisePartition(accountId: session.accountId, origin: origin, organizationId: org))
        let sealed = try operation(partition)

        let acks = try await client.deliver(partition: partition, operations: [sealed])

        XCTAssertEqual(acks.map { $0.result }, [.accepted])
        let delivery = try XCTUnwrap(exchange.requests.last)
        XCTAssertEqual(delivery.url?.path, path)
        XCTAssertEqual(delivery.httpMethod, "POST")
        XCTAssertEqual(delivery.value(forHTTPHeaderField: "Signature-Keyid"), IdentityVectors.fingerprint)
        XCTAssertNotNil(delivery.value(forHTTPHeaderField: "Signature"))
        XCTAssertNotNil(delivery.value(forHTTPHeaderField: "Signature-Nonce"))
        XCTAssertNil(delivery.value(forHTTPHeaderField: "Cookie"))
        XCTAssertNil(delivery.value(forHTTPHeaderField: "X-CSRF"))
        let body = FakeIdentityBackend.json(delivery)
        XCTAssertEqual(body["family_id"] as? String, session.familyId)
        XCTAssertEqual(body["access_token"] as? String, session.accessToken)
        XCTAssertEqual(body["schema_version"] as? Int, 1)
        XCTAssertEqual(exchange.requests.map { $0.url?.path ?? "" }.filter { $0.hasPrefix("/v1/accounts/bindings") }.count, 1)
    }

    func testServerRefusalsKeepTheirEnterpriseCode() async throws {
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { _ in
                ScriptedExchange.Reply(403, #"{"error":{"code":"DEVICE_REVOKED","message":"revoked"}}"#)
            }
        }
        let sessions = StaticSessions(session: session)
        let client = try makeClient(exchange, sessions: sessions)
        let partition = try XCTUnwrap(EnterprisePartition(accountId: session.accountId, origin: origin, organizationId: org))
        do {
            _ = try await client.deliver(partition: partition, operations: [try operation(partition)])
            XCTFail("a refused delivery must throw")
        } catch let error as EnterpriseSyncError {
            XCTAssertEqual(error, .refused(status: 403, code: "DEVICE_REVOKED", retryAfterSeconds: nil))
            XCTAssertEqual(error.accessError, .deviceRevoked)
        }
    }

    func testAnotherAccountsPartitionIsNeverSentWithThisSession() async throws {
        let exchange = ScriptedExchange { _ in ScriptedExchange.Reply(500, "{}") }
        let sessions = StaticSessions(session: session)
        let client = try makeClient(exchange, sessions: sessions)
        let foreign = try XCTUnwrap(EnterprisePartition(accountId: "b0000000-0000-4000-8000-000000000001", origin: origin, organizationId: org))
        do {
            _ = try await client.deliver(partition: foreign, operations: [try operation(foreign)])
            XCTFail("a foreign partition must not be delivered")
        } catch let error as EnterpriseSyncError {
            XCTAssertEqual(error, .signInRequired)
        }
        XCTAssertTrue(exchange.requests.isEmpty)
    }

    func testDeviceIsDiscoveredOrEnrolledBeforeTheContactVerdict() async throws {
        let member = "7b0e2a54-0000-4000-8000-0000000000c9"
        let device = "7b0e2a54-0000-4000-8000-00000000012d"
        let partition = try XCTUnwrap(EnterprisePartition(accountId: session.accountId, origin: origin, organizationId: org))
        let familyId = session.familyId
        let accountId = session.accountId
        let organization = org
        let apiOrigin = origin
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                let record = #"{"id":"\#(device)","membership_id":"\#(member)","label":"iOS","state":"ENROLLED","revision":1,"revocation_class":"","enrolled_at":"2026-11-15T12:00:00Z","last_sync_at":null,"revoked_at":null,"data_mode":"SYNTHETIC"}"#
                switch request.url?.path ?? "" {
                case "/v1/orgs/\(organization)/devices/check":
                    return ScriptedExchange.Reply(404, #"{"error":{"code":"NOT_FOUND"}}"#)
                case "/v1/orgs/\(organization)/devices/enroll":
                    return ScriptedExchange.Reply(200, #"{"device":\#(record),"operation_id":"7b0e2a54-0000-4000-8000-000000000999","reason":"","replayed":false,"data_mode":"SYNTHETIC"}"#)
                case "/v1/orgs/\(organization)/devices/contact":
                    return ScriptedExchange.Reply(200, #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000000999","account_id":"\#(accountId)","api_origin":"\#(apiOrigin)","org_id":"\#(organization)","membership_id":"\#(member)","enrollment_id":"\#(device)","family_id":"\#(familyId)","enrollment_revision":"1","server_time":"2026-11-15T12:00:00Z","verdict":"ACTIVE","access_model":"EXISTING_ACCOUNT","offline_validity_seconds":2592000,"managed_unlock_seconds":null}"#)
                default:
                    return ScriptedExchange.Reply(404, "{}")
                }
            }
        }
        let client = try makeClient(exchange, sessions: StaticSessions(session: session))
        let outcome = try await client.confirmDevice(partition: partition, membershipId: member, known: nil)
        XCTAssertEqual(outcome.verdict, .active)
        XCTAssertEqual(outcome.enrollment.enrollmentId, device)
        let paths = exchange.requests.map { $0.url?.path ?? "" }.filter { $0.hasPrefix("/v1/orgs/") }
        XCTAssertEqual(paths, ["/v1/orgs/\(org)/devices/check", "/v1/orgs/\(org)/devices/enroll", "/v1/orgs/\(org)/devices/contact"])
        for request in exchange.requests where (request.url?.path ?? "").hasPrefix("/v1/orgs/") {
            XCTAssertNotNil(request.value(forHTTPHeaderField: "Signature"))
            XCTAssertNil(request.value(forHTTPHeaderField: "Cookie"))
        }
        let enroll = try XCTUnwrap(exchange.requests.first { $0.url?.path == "/v1/orgs/\(org)/devices/enroll" })
        XCTAssertEqual(FakeIdentityBackend.json(enroll)["label"] as? String, "iOS")
    }

    func testCompanyChannelUsesTheCookieAndCsrfButNeverDeviceProof() async throws {
        let secret = String(repeating: "d", count: 64)
        let csrf = "abv_company_csrf_" + String(repeating: "e", count: 64)
        let accountId = session.accountId
        let cookieHeader = "__Host-empresas-session=abv_company_\(secret); Path=/; Expires=Thu, 19 Nov 2026 20:00:00 GMT; HttpOnly; Secure; SameSite=Strict"
        let exchange = ScriptedExchange { request in
            switch request.url?.path ?? "" {
            case EnterpriseCompanyWire.loginPath:
                return ScriptedExchange.Reply(200, #"{"account_id":"\#(accountId)","csrf_token":"\#(csrf)","authenticated_at":"2026-11-15T12:00:00Z","expires_at":"2026-11-15T20:00:00Z"}"#,
                                              headers: ["Set-Cookie": cookieHeader, "Content-Type": "application/json"])
            case EnterpriseCompanyWire.contextsListPath:
                guard request.value(forHTTPHeaderField: "X-CSRF") == csrf,
                      request.value(forHTTPHeaderField: "Cookie") == "__Host-empresas-session=abv_company_\(secret)" else {
                    return ScriptedExchange.Reply(401, #"{"error":{"code":"UNAUTHENTICATED"}}"#)
                }
                return ScriptedExchange.Reply(200, #"{"account_id":"\#(accountId)","organizations":[{"id":"7b0e2a54-0000-4000-8000-000000000101","name":"Transportes A","membership_id":"7b0e2a54-0000-4000-8000-0000000000c9","state":"ACTIVE","suspension_reason":null}],"next_after_org_id":""}"#)
            default:
                return ScriptedExchange.Reply(404, "{}")
            }
        }
        let client = EnterpriseCompanyClient(origin: origin, exchange: exchange, sessions: StaticSessions(session: session))
        let contexts = try await client.listContexts()
        XCTAssertEqual(contexts.map { $0.name }, ["Transportes A"])
        XCTAssertEqual(exchange.requests.count, 2)
        for request in exchange.requests {
            XCTAssertEqual(request.value(forHTTPHeaderField: "Origin"), origin)
            XCTAssertNil(request.value(forHTTPHeaderField: "Signature"))
            XCTAssertNil(request.value(forHTTPHeaderField: "Authorization"))
        }
        let login = FakeIdentityBackend.json(exchange.requests[0])
        XCTAssertEqual(login["family_id"] as? String, session.familyId)
        XCTAssertNil(exchange.requests[0].value(forHTTPHeaderField: "Cookie"))

        _ = try await client.listContexts()
        XCTAssertEqual(exchange.requests.count, 3, "an open channel is reused")
    }

    func testCompanyRefusalsAreShownWithoutLocalEffects() async throws {
        let exchange = ScriptedExchange { request in
            if request.url?.path == EnterpriseCompanyWire.loginPath {
                return ScriptedExchange.Reply(401, #"{"error":{"code":"FRESH_AUTH_REQUIRED"}}"#)
            }
            return ScriptedExchange.Reply(500, "{}")
        }
        let client = EnterpriseCompanyClient(origin: origin, exchange: exchange, sessions: StaticSessions(session: session))
        do {
            _ = try await client.previewInvitation(code: "AIN1.7b0e2a54000040008000000000000201.secret")
            XCTFail("the refused login must surface")
        } catch let error as EnterpriseAccessError {
            XCTAssertEqual(error, .freshAuthRequired)
        }
        let signedOut = EnterpriseCompanyClient(origin: origin, exchange: exchange, sessions: StaticSessions(session: nil))
        do {
            _ = try await signedOut.listContexts()
            XCTFail("a signed-out client never calls the server")
        } catch let error as EnterpriseAccessError {
            XCTAssertEqual(error, .signInRequired)
        }
        XCTAssertEqual(exchange.requests.count, 1)
    }
}
