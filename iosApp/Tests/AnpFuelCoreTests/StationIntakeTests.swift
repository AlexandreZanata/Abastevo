import Foundation
import XCTest
@testable import AnpFuelCore

final class StationIntakeTests: XCTestCase {
    private static let stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private let operationId = "c0000000-0000-4000-8000-000000000004"
    private let origin = "https://abastevo.com.br"
    private let session = PortableAuth.Session(
        familyId: "f0000000-0000-4000-8000-000000000001",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "fixture-intake-access", refreshToken: "fixture-intake-refresh",
        accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000
    )

    func testRegistryCNPJHandlesLettersLeadingZerosAndChecksum() throws {
        #if SWIFT_PACKAGE
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { url.deleteLastPathComponent() }
        url = url.appendingPathComponent("contracts/testdata/registry/registry-api-sample.json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "registry-api-sample", withExtension: "json"))
        #endif
        let root = try XCTUnwrap(DeviceIdentityWire.object(Data(contentsOf: url)))
        let value = try XCTUnwrap(root["value"] as? [String: Any])
        let items = try XCTUnwrap(value["items"] as? [[String: Any]])
        XCTAssertEqual(items.count, 3)
        for item in items {
            let cnpj = try XCTUnwrap(item["cnpj"] as? String)
            XCTAssertEqual(BrazilianCNPJ.normalized(cnpj), cnpj)
            XCTAssertEqual(BrazilianCNPJ.normalized(cnpj.lowercased()), cnpj)
        }
        XCTAssertEqual(BrazilianCNPJ.normalized("04.218.406/0001-04"), "04218406000104")
        XCTAssertNil(BrazilianCNPJ.normalized("04.218.406/0001-05"))
        XCTAssertNil(BrazilianCNPJ.normalized("04.218.406/0001-04!"))
    }

    func testProposalFieldsDifferBetweenStationAndCharger() throws {
        let station = try StationIntakeProposal(displayName: " Fixture station ", municipalityCode: "3550308",
                                                state: .SP, cnpj: "04.218.406/0001-04", chargingSite: false)
        let stationFields = try station.wireFields()
        XCTAssertEqual(stationFields, ["display_name": "Fixture station", "municipality_code": "3550308",
                                       "state": "SP", "cnpj_normalized": "04218406000104"])
        let charger = try StationIntakeProposal(displayName: "", municipalityCode: "3550308", state: .SP,
                                                publicAddress: " Rua de Teste ", chargingSite: true)
        let chargerFields = try charger.wireFields()
        XCTAssertEqual(chargerFields, ["display_name": "", "municipality_code": "3550308", "state": "SP",
                                       "kind": "charging_site", "public_address": "Rua de Teste"])
        XCTAssertThrowsError(try StationIntakeProposal(displayName: "", municipalityCode: "3550308", state: .SP, chargingSite: false))
        XCTAssertThrowsError(try StationIntakeProposal(displayName: "", municipalityCode: "3550308", state: .SP, chargingSite: true))
        XCTAssertThrowsError(try StationIntakeProposal(displayName: "", municipalityCode: "3550308", state: .SP, publicAddress: "123", chargingSite: true))
        XCTAssertThrowsError(try StationIntakeProposal(displayName: "Fixture", municipalityCode: "35-50308", state: .SP, chargingSite: false))
        XCTAssertThrowsError(try StationIntakeProposal(displayName: "Fixture", municipalityCode: "3550308", state: .SP, cnpj: "12ABC34501DE35", chargingSite: true))
        XCTAssertThrowsError(try StationIntakeProposal(displayName: String(repeating: "é", count: 81), municipalityCode: "3550308",
                                                       state: .SP, chargingSite: true))
    }

    private func harness(_ sessions: StaticSessions, response: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply)
        throws -> (CommunityWriteClient, ScriptedExchange, String) {
        let exchange = ScriptedExchange(respond: response)
        let transport = SignedRequestTransport(origin: origin, signer: try IdentityVectors.signer(),
                                               identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true),
                                               exchange: exchange, sessions: sessions)
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        return (client, exchange, try XCTUnwrap(transport.ownerScope()))
    }

    func testSessionOnlySubmitReplaysExactProposalAndOwnerCanReadAndCancel() async throws {
        let row = #"{"id":"\#(Self.stationId)","client_submission_id":"\#(operationId)","state":"pending","created_at":"2026-11-24T12:00:00Z"}"#
        let sessions = StaticSessions(session: session)
        let (client, exchange, scope) = try harness(sessions) { request in
            switch request.url?.path {
            case "/v1/stations/suggestions/mine": return .init(200, #"{"items":[\#(row)]}"#)
            case "/v1/stations/suggestions/\(Self.stationId)/cancel": return .init(200, #"{"status":"cancelled"}"#)
            default: return .init(200, row)
            }
        }
        let proposal = try StationIntakeProposal(displayName: "Fixture", municipalityCode: "3550308", state: .SP, chargingSite: false)
        let item = CommunityOutboxItem(id: operationId, kind: .stationSuggestion, ownerScope: scope,
                                       origin: origin, createdAtMillis: 0, stationProposal: proposal)
        let first = try await client.submitStationSuggestion(item)
        let replay = try await client.submitStationSuggestion(item)
        XCTAssertEqual(first, replay)
        XCTAssertEqual(first.id, Self.stationId)

        let list = try await client.ownedStationSuggestions(expectedScope: scope)
        let status = try await client.stationSuggestionStatus(id: Self.stationId, expectedScope: scope)
        XCTAssertEqual(list, [status])
        try await client.cancelStationSuggestion(id: Self.stationId, operationId: operationId, expectedScope: scope)

        XCTAssertEqual(exchange.requests.count, 5)
        XCTAssertEqual(exchange.requests[0].httpBody, exchange.requests[1].httpBody)
        XCTAssertEqual(exchange.requests.last?.value(forHTTPHeaderField: "Idempotency-Key"), operationId)
        for request in exchange.requests {
            XCTAssertNil(request.value(forHTTPHeaderField: "Signature"))
            XCTAssertNil(request.url?.query)
            XCTAssertEqual(FakeIdentityBackend.json(request)["family_id"] as? String, session.familyId)
        }
        let fields = FakeIdentityBackend.json(exchange.requests[0])
        XCTAssertEqual(fields["client_submission_id"] as? String, operationId)
        let sent = try XCTUnwrap(fields["proposal"] as? [String: String])
        XCTAssertNil(sent["latitude"])
        XCTAssertNil(sent["evidence_ref"])
    }

    func testRefusalsAndWrongOwnerLeaveNoAcceptedRecord() async throws {
        let proposal = try StationIntakeProposal(displayName: "Fixture", municipalityCode: "3550308", state: .SP, chargingSite: false)
        for (status, code) in [(400, "intake.invalid"), (401, "intake.session-invalid"),
                               (403, "intake.author-forbidden"), (409, "intake.idempotency-conflict"),
                               (429, "intake.quota-exceeded")] {
            let sessions = StaticSessions(session: session)
            let (client, exchange, scope) = try harness(sessions) { _ in
                .init(status, #"{"error":{"code":"\#(code)"}}"#)
            }
            let item = CommunityOutboxItem(id: operationId, kind: .stationSuggestion, ownerScope: scope,
                                           origin: origin, createdAtMillis: 0, stationProposal: proposal)
            do {
                _ = try await client.submitStationSuggestion(item)
                XCTFail("Expected suggestion refusal \(status)")
            } catch {
                if status == 429 {
                    XCTAssertEqual(error as? CommunityWriteError, .invalid(code))
                } else {
                    XCTAssertEqual(error as? SignedTransportError, .refused(status: status, code: code, retryAfterSeconds: nil))
                }
            }
            sessions.set(nil)
            do {
                _ = try await client.ownedStationSuggestions(expectedScope: scope)
                XCTFail("Expected the former scope to refuse")
            } catch {
                XCTAssertEqual(error as? SignedTransportError, .sessionRequired)
            }
            XCTAssertEqual(exchange.requests.count, 1)
        }
    }

    func testProposalIdentitySurvivesRetryAndRemainsOwnerAndOriginScoped() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = CommunityOutboxStore(directory: directory)
        let scope = "account:fixture:fp:abcdef"
        let proposal = try StationIntakeProposal(displayName: "Fixture", municipalityCode: "3550308", state: .SP, chargingSite: false)
        let item = CommunityOutboxItem(id: operationId, kind: .stationSuggestion, ownerScope: scope,
                                       origin: origin, createdAtMillis: 0, stationProposal: proposal)
        let gateway = FakeCommunityGateway()
        gateway.receipt = .failure(SignedTransportError.refused(status: 503, code: "intake.unavailable", retryAfterSeconds: 120))
        let context = CommunityDispatchContext(nowMillis: 0, ownerScope: scope, origin: origin, hasAccount: true)
        let waiting = await CommunityOutboxDispatcher.run(item, context: context, gateway: gateway, clock: { 0 })
        XCTAssertEqual(waiting.status, .pending)
        XCTAssertEqual(waiting.nextEligibleMillis, 120_000)
        store.save([waiting])
        let reopened = CommunityOutboxStore(directory: directory)
        let restored = try XCTUnwrap(reopened.load().first)
        XCTAssertEqual(restored.id, operationId)
        XCTAssertEqual(restored.stationProposal, proposal)
        XCTAssertNil(CommunityOutbox.nextEligible([restored], nowMillis: 119_999))
        XCTAssertEqual(CommunityOutbox.nextEligible([restored], nowMillis: 120_000)?.id, operationId)
        let foreign = CommunityDispatchContext(nowMillis: 120_000, ownerScope: scope, origin: "https://other.example", hasAccount: true)
        let refused = await CommunityOutboxDispatcher.run(restored, context: foreign, gateway: gateway)
        XCTAssertEqual(refused.lastCode, CommunityOutbox.ownerChangedCode)

        gateway.receipt = .success(CommunityWriteReceipt(id: Self.stationId, state: "pending"))
        let eligible = CommunityDispatchContext(nowMillis: 120_000, ownerScope: scope, origin: origin, hasAccount: true)
        let accepted = await CommunityOutboxDispatcher.run(restored, context: eligible, gateway: gateway)
        XCTAssertEqual(accepted.status, .accepted)
        XCTAssertEqual(gateway.submittedItems.map(\.id), [operationId, operationId])
        reopened.purge()
        XCTAssertTrue(store.load().isEmpty)
    }
}
