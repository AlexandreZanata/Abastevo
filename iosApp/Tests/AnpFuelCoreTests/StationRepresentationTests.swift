import Foundation
import XCTest
@testable import AnpFuelCore

final class StationRepresentationTests: XCTestCase {
    private static let station = "a1000000-0000-4000-8000-000000000001"
    private static let otherStation = "a1000000-0000-4000-8000-000000000002"
    private static let claim = "b1000000-0000-4000-8000-000000000001"
    private static let declaration = "b1000000-0000-4000-8000-000000000002"
    private static let nextDeclaration = "b1000000-0000-4000-8000-000000000003"
    private static let operation = "c1000000-0000-4000-8000-000000000001"
    private static let proof = "d1000000-0000-4000-8000-000000000001"
    private static let comment = "d1000000-0000-4000-8000-000000000002"

    private static func claimBody(declaration: String = StationRepresentationTests.declaration, station: String = StationRepresentationTests.station,
                                  state: String = "draft", declarationState: String = "active") -> String {
        #"{"id":"\#(claim)","station_id":"\#(station)","role":"manager","scopes":["profile.edit","reply.official"],"state":"\#(state)","version":1,"declaration_state":"\#(declarationState)","declaration_id":"\#(declaration)","declaration":"exact text\n","expires_at":"2026-10-05T12:30:00Z"}"#
    }

    private struct Harness {
        let client: CommunityWriteClient
        let exchange: ScriptedExchange
        let scope: String
    }

    private static func session(_ account: String = "a0000000-0000-4000-8000-000000000001") -> PortableAuth.Session {
        PortableAuth.Session(familyId: "f0000000-0000-4000-8000-000000000001", accountId: account,
                             accessToken: "fixture-claim-access", refreshToken: "fixture-claim-refresh",
                             accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000)
    }

    private func harness(sessions: StaticSessions? = nil, origin: String = "https://abastevo.com.br",
                         reply: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply) throws -> Harness {
        let sessionSource = sessions ?? StaticSessions(session: Self.session())
        let exchange = ScriptedExchange(respond: reply)
        let transport = SignedRequestTransport(
            origin: origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange, sessions: sessionSource
        )
        return Harness(client: CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessionSource, photoBytes: { _ in nil }),
                       exchange: exchange, scope: try XCTUnwrap(transport.ownerScope()))
    }

    func testAnonymousProjectionKeepsOnlyBusinessFieldsAndRequiresVerifiedSource() throws {
        func body(source: String, badge: Bool) -> Data {
            Data(#"{"station_id":"\#(Self.station)","display_name":"Example station","revision":4,"business":{"opening_hours":"24h","services":"fuel","unexpected_field":"ignore"},"operator":{"source":"\#(source)"},"has_badge":\#(badge)}"#.utf8)
        }
        for source in ["registry", "dou", "review"] {
            let profile = try StationRepresentationWire.profile(body(source: source, badge: true), stationId: Self.station)
            XCTAssertTrue(profile.verifiedRepresentation)
            XCTAssertEqual(profile.business, [.openingHours: "24h", .services: "fuel"])
        }
        XCTAssertFalse(try StationRepresentationWire.profile(body(source: "unreviewed", badge: true), stationId: Self.station).verifiedRepresentation)
        XCTAssertFalse(try StationRepresentationWire.profile(body(source: "registry", badge: false), stationId: Self.station).verifiedRepresentation)
        XCTAssertThrowsError(try StationRepresentationWire.profile(body(source: "registry", badge: true), stationId: Self.otherStation))
        let absent = Data(#"{"station_id":"\#(Self.station)","display_name":"Example station"}"#.utf8)
        XCTAssertNil(try StationRepresentationWire.profile(absent, stationId: Self.station).revision)
    }

    func testExactDeclarationAndBoundedDocumentRules() throws {
        XCTAssertEqual(try StationRepresentationRules.declarationData(" Exact server text\r\n"),
                       Data(" Exact server text\r\n".utf8))
        XCTAssertThrowsError(try StationRepresentationRules.declarationData(" \n"))
        XCTAssertTrue(StationRepresentationRules.isSignedPdfCandidate(Data("%PDF-1.7\nexample".utf8)))
        XCTAssertFalse(StationRepresentationRules.isSignedPdfCandidate(Data("%pdf-1.7\nexample".utf8)))
        var over = Data("%PDF-".utf8)
        over.append(Data(repeating: 0, count: StationRepresentationRules.maxProofBytes))
        XCTAssertFalse(StationRepresentationRules.isSignedPdfCandidate(over))
        let temporary = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("pdf")
        defer { try? FileManager.default.removeItem(at: temporary) }
        let exact = Data("%PDF-1.7\r\nexample\n".utf8)
        try exact.write(to: temporary)
        XCTAssertEqual(try StationRepresentationRules.readPdf(from: temporary), exact)
        try over.write(to: temporary)
        XCTAssertThrowsError(try StationRepresentationRules.readPdf(from: temporary))
    }

    func testBusinessFieldsUseScalarBytesAndServiceBounds() {
        XCTAssertTrue(StationRepresentationRules.validateBusinessFields([.openingHours: "24h", .services: "fuel, parking"]))
        XCTAssertFalse(StationRepresentationRules.validateBusinessFields([.services: "fuel,"]))
        XCTAssertFalse(StationRepresentationRules.validateBusinessFields([.services: "fuel, unsupported"]))
        XCTAssertTrue(StationRepresentationRules.validateBusinessFields([.description: String(repeating: "🚲", count: 280)]))
        XCTAssertFalse(StationRepresentationRules.validateBusinessFields([.description: String(repeating: "e\u{301}", count: 141)]))
        XCTAssertTrue(StationRepresentationRules.validateBusinessFields([.website: String(repeating: "é", count: 128)]))
        XCTAssertFalse(StationRepresentationRules.validateBusinessFields([.website: String(repeating: "é", count: 129)]))
        XCTAssertFalse(StationRepresentationRules.validateBusinessFields([:]))
    }

    func testClaimRoutesUseSessionEnvelopeAndSameKeyOnReplay() async throws {
        let fixture = try harness { request in
            let path = request.url?.path ?? ""
            if path.hasSuffix("/mine") { return .init(200, #"{"items":[{"id":"\#(Self.claim)","state":"draft"}]}"#) }
            if path.hasSuffix("/reissue") { return .init(200, Self.claimBody(declaration: Self.nextDeclaration)) }
            if path.hasSuffix("/cancel") { return .init(200, #"{"status":"cancelled"}"#) }
            return .init(path.hasSuffix("/claims") ? 201 : 200, Self.claimBody())
        }
        let scopes: Set<String> = ["reply.official", "profile.edit"]
        let first = try await fixture.client.openStationClaim(stationId: Self.station, role: .manager, scopes: scopes,
                                                              clientKey: Self.operation, expectedScope: fixture.scope)
        let repeatClaim = try await fixture.client.openStationClaim(stationId: Self.station, role: .manager, scopes: scopes,
                                                                    clientKey: Self.operation, expectedScope: fixture.scope)
        XCTAssertEqual(first, repeatClaim)
        let claims = try await fixture.client.stationClaims(stationId: Self.station, expectedScope: fixture.scope)
        XCTAssertEqual(claims, [first])
        let reissued = try await fixture.client.reissueStationClaim(id: first.id, operationId: Self.operation, expectedScope: fixture.scope)
        XCTAssertEqual(reissued.declarationId, Self.nextDeclaration)
        try await fixture.client.cancelStationClaim(id: first.id, operationId: Self.operation, expectedScope: fixture.scope)

        let calls = fixture.exchange.requests
        XCTAssertEqual(calls.map { $0.url?.path }, [
            "/v1/stations/\(Self.station)/claims", "/v1/stations/\(Self.station)/claims",
            "/v1/profile/claims/mine", "/v1/profile/claims/\(Self.claim)/status",
            "/v1/profile/claims/\(Self.claim)/reissue", "/v1/profile/claims/\(Self.claim)/cancel",
        ])
        for call in calls {
            XCTAssertEqual(call.value(forHTTPHeaderField: "Cache-Control"), "no-store")
            XCTAssertNil(call.value(forHTTPHeaderField: "Signature"))
            XCTAssertEqual(FakeIdentityBackend.json(call)["family_id"] as? String, Self.session().familyId)
        }
        let opened = FakeIdentityBackend.json(try XCTUnwrap(calls.first))
        XCTAssertEqual(Set(opened.keys), ["role", "scopes", "client_key", "access_token", "family_id"])
        XCTAssertEqual(opened["scopes"] as? [String], ["profile.edit", "reply.official"])
        XCTAssertEqual(opened["client_key"] as? String, Self.operation)
        XCTAssertEqual(calls.first?.httpBody, calls.dropFirst().first?.httpBody)
    }

    func testPdfSubmitRechecksDeclarationAndKeepsExactBytesAndScanRetention() async throws {
        let fixture = try harness { request in
            if request.url?.path.hasSuffix("/proof") == true {
                return .init(201, #"{"id":"\#(Self.proof)","status":"received","expires_at":"2026-10-06T12:00:00Z"}"#)
            }
            return .init(200, Self.claimBody())
        }
        let claim = try await fixture.client.stationClaim(id: Self.claim, expectedScope: fixture.scope)
        let exact = Data("%PDF-1.7\r\nexample\n".utf8)
        let receipt = try await fixture.client.submitStationClaimProof(
            claim: claim, pdf: exact, operationId: Self.operation, expectedScope: fixture.scope,
            now: { Date(timeIntervalSince1970: 1_763_892_000) }
        )
        XCTAssertEqual(receipt.status, "received")
        let calls = fixture.exchange.requests
        XCTAssertEqual(calls.map { $0.url?.path }, [
            "/v1/profile/claims/\(Self.claim)/status", "/v1/profile/claims/\(Self.claim)/status",
            "/v1/profile/claims/\(Self.claim)/proof",
        ])
        let submitted = FakeIdentityBackend.json(try XCTUnwrap(calls.last))
        XCTAssertEqual(submitted["kind"] as? String, "scan")
        XCTAssertEqual(submitted["filename"] as? String, "signed.pdf")
        XCTAssertEqual(submitted["content_type"] as? String, "application/pdf")
        XCTAssertEqual(submitted["declaration_id"] as? String, Self.declaration)
        XCTAssertEqual(submitted["content_base64"] as? String, exact.base64EncodedString())
        XCTAssertNil(submitted["operation_id"])
        XCTAssertEqual(calls.last?.value(forHTTPHeaderField: "Idempotency-Key"), Self.operation)
    }

    func testStaleAndExpiredDeclarationsNeverUpload() async throws {
        let previous = try StationRepresentationWire.claim(
            SignedHttpResponse(status: 200, body: Data(Self.claimBody().utf8))
        )
        let expired = try harness { _ in .init(200, Self.claimBody()) }
        let rotated = try harness { _ in .init(200, Self.claimBody(declaration: Self.nextDeclaration)) }
        let closed = try harness { _ in .init(200, Self.claimBody(state: "approved", declarationState: "consumed")) }
        for (fixture, now) in [(expired, 2_000_000_000.0), (rotated, 1_763_892_000), (closed, 1_763_892_000)] {
            do {
                _ = try await fixture.client.submitStationClaimProof(
                    claim: previous, pdf: Data("%PDF-".utf8), operationId: Self.operation, expectedScope: fixture.scope,
                    now: { Date(timeIntervalSince1970: now) }
                )
                XCTFail("Stale, expired and consumed declarations cannot bind proof")
            } catch {
                XCTAssertEqual(StationRepresentationError.from(error), .invalidInput)
            }
            XCTAssertEqual(fixture.exchange.requests.map { $0.url?.path }, ["/v1/profile/claims/\(Self.claim)/status"])
        }
    }

    func testProfileAndReplyUseScopedRouteAndRevision() async throws {
        let fixture = try harness { request in
            if request.url?.path.hasSuffix("/replies") == true {
                return .init(201, #"{"comment_id":"\#(Self.comment)"}"#)
            }
            return .init(200, #"{"revision":2,"business":{"phone":"123"}}"#)
        }
        let revision = try await fixture.client.editStationBusinessProfile(
            stationId: Self.station, revision: 1, fields: [.phone: "123"], operationId: Self.operation, expectedScope: fixture.scope
        )
        let comment = try await fixture.client.replyAsStation(
            stationId: Self.station, product: .gasolinePremium, text: "  Official reply  ", operationId: Self.operation, expectedScope: fixture.scope
        )
        XCTAssertEqual(revision, 2)
        XCTAssertEqual(comment, Self.comment)
        let edit = FakeIdentityBackend.json(try XCTUnwrap(fixture.exchange.requests.first))
        XCTAssertEqual(edit["expected_revision"] as? Int, 1)
        XCTAssertEqual(edit["fields"] as? [String: String], ["phone": "123"])
        let reply = FakeIdentityBackend.json(try XCTUnwrap(fixture.exchange.requests.last))
        XCTAssertEqual(reply["product"] as? String, "GASOLINE_ADDITIVED")
        XCTAssertEqual(reply["text"] as? String, "Official reply")
    }

    func testContainmentStorageAndOtherRefusalsArePreservedWithoutAutomaticReplay() async throws {
        let refusals: [(Int, String)] = [
            (400, "claim.invalid"), (401, "claim.session-invalid"), (403, "profile.forbidden"),
            (404, "claim.not-found"), (408, "profile.timeout"), (409, "claim.idempotency-conflict"),
            (409, "claim.busy-retry"), (409, "proof.closed"), (409, "profile.stale"), (429, "claim.quota-exceeded"),
            (503, "profile.integration-pending"), (503, "proof.storage-unavailable"),
        ]
        for (status, code) in refusals {
            let fixture = try harness { _ in
                .init(status, #"{"error":{"code":"\#(code)"}}"#, headers: status == 429 ? ["Retry-After": "60"] : [:])
            }
            do {
                _ = try await fixture.client.ownedStationClaims(expectedScope: fixture.scope)
                XCTFail("Expected refusal \(code)")
            } catch {
                let refusal = StationRepresentationError.from(error)
                XCTAssertEqual(refusal, .refused(status: status, code: code, retryAfterSeconds: status == 429 ? 60 : nil))
                XCTAssertEqual(refusal.integrationPending, code == "profile.integration-pending")
                XCTAssertEqual(refusal.proofStorageUnavailable, code == "proof.storage-unavailable")
            }
            XCTAssertEqual(fixture.exchange.requests.count, 1)
        }
    }

    func testOldOwnerResultsAreRejectedAndRestartReadsTheServer() async throws {
        let sessions = StaticSessions(session: Self.session())
        let original = try harness(sessions: sessions) { _ in
            sessions.set(Self.session("a0000000-0000-4000-8000-000000000002"))
            return .init(200, #"{"items":[{"id":"\#(Self.claim)","state":"draft"}]}"#)
        }
        do {
            _ = try await original.client.ownedStationClaims(expectedScope: original.scope)
            XCTFail("Do not expose previous owner's private claims")
        } catch {
            XCTAssertEqual(StationRepresentationError.from(error), .signInRequired)
        }
        let restarted = try harness(sessions: sessions, origin: "https://teste.abastevo.com.br") { _ in .init(200, #"{"items":[]}"#) }
        let reads = try await restarted.client.ownedStationClaims(expectedScope: restarted.scope)
        XCTAssertTrue(reads.isEmpty)
        XCTAssertEqual(restarted.exchange.requests.first?.url?.host, "teste.abastevo.com.br")
    }
}
