import Foundation
import XCTest
@testable import AnpFuelCore

final class EnterpriseFuelingTests: XCTestCase {
    private let account = "7b0e2a54-0000-4000-8000-000000009501"
    private let organization = "7b0e2a54-0000-4000-8000-000000009502"
    private let membership = "7b0e2a54-0000-4000-8000-000000009503"
    private let enrollment = "7b0e2a54-0000-4000-8000-000000009504"
    private let origin = "https://api.example.invalid"

    private func fixture(_ name: String, folder: String, digest: String) throws -> String {
        #if SWIFT_PACKAGE
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { url.deleteLastPathComponent() }
        url = url.appendingPathComponent("contracts/testdata/enterprise/\(folder)/\(name).psv")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "psv"))
        #endif
        let content = try Data(contentsOf: url)
        XCTAssertEqual(EnterpriseCanonicalJson.sha256Hex(content), digest)
        return try XCTUnwrap(String(data: content, encoding: .utf8))
    }

    private func fields(_ raw: String) throws -> [String: String] {
        var fields: [String: String] = [:]
        for item in raw.split(separator: ";") {
            let pair = item.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
            guard pair.count == 2, fields.updateValue(String(pair[1]), forKey: String(pair[0])) == nil else {
                throw EnterpriseOperationError.malformed
            }
        }
        return fields
    }

    private func hex(_ raw: String) throws -> Data {
        let bytes = Array(raw)
        guard bytes.count.isMultiple(of: 2) else { throw EnterpriseOperationError.malformed }
        return try Data(stride(from: 0, to: bytes.count, by: 2).map { offset in
            try XCTUnwrap(UInt8(String(bytes[offset...offset + 1]), radix: 16))
        })
    }

    private func input(_ args: [String: String]) throws -> EnterpriseFuelingInput {
        func required(_ key: String) throws -> String { try XCTUnwrap(args[key], key) }
        func text(_ key: String) throws -> String? {
            guard let value = args[key] else { return nil }
            return try XCTUnwrap(String(data: hex(value), encoding: .utf8), key)
        }
        let reading: EnterpriseFuelingInput.Reading?
        if let meter = args["meter"] {
            reading = try .init(meterId: meter, segmentId: required("segment"), readingId: required("reading"),
                                kind: required("kind"), value: required("value"))
        } else {
            reading = nil
        }
        return try EnterpriseFuelingInput(
            assetId: required("asset"), responsibleEmployeeId: required("responsible"),
            product: required("product"), unit: required("unit"), quantity: required("quantity"), paidTotal: required("paid"),
            discount: args["discount"], unitPrice: args["price"], unitPriceText: text("price_text"),
            fill: required("fill"), payer: required("payer"), payment: required("payment"),
            reading: reading, missingReadingReason: text("reason"), placeKind: args["place_kind"],
            placeName: text("place_name"), tripId: args["trip"], contractedSupplierId: args["supplier"]
        )
    }

    func testEverySharedFuelingSubmitVector() throws {
        let text = try fixture("fueling-submit-v1", folder: "fueling",
                               digest: "67068cfba629ff0fe6ec6299fb2c892bcc2c6e001424ebb0a012d6a1ecf5f0b8")
        let lines = text.split(whereSeparator: \.isNewline)
        XCTAssertEqual(lines.first, "id|operation|args|expected|bytes_hex")
        let partition = try XCTUnwrap(EnterprisePartition(accountId: account, origin: origin, organizationId: organization))
        let other = try XCTUnwrap(EnterprisePartition(accountId: enrollment, origin: origin, organizationId: organization))
        var coverage: [String: Int] = [:]
        for line in lines.dropFirst() {
            let row = line.split(separator: "|", omittingEmptySubsequences: false).map(String.init)
            guard row.count == 5, row[1] == "SUBMIT" else { throw EnterpriseOperationError.malformed }
            let args = try fields(row[2])
            coverage[row[3], default: 0] += 1
            let envelope = EnterpriseEnvelope(
                partition: partition, membershipId: membership, enrollmentId: enrollment,
                operationId: try XCTUnwrap(args["op"]),
                declared: EnterpriseTimeDeclaration(occurredAt: args["at"] ?? "2026-11-15T12:00:00.123Z",
                    entryZone: args["zone"] ?? "America/Cuiaba", deviceOffsetSeconds: try XCTUnwrap(Int64(args["offset"] ?? "-14400")))
            )
            let fuelingId = try XCTUnwrap(args["fueling"])
            let command = try input(args)
            let expected = try hex(row[4])
            if row[3] == "OK" {
                let built = try EnterpriseFuelingOperations.build(envelope: envelope, fuelingId: fuelingId, input: command)
                XCTAssertEqual(built.canonical, expected, row[0])
                let fact = try XCTUnwrap(EnterpriseFuelingOperations.fact(built, partition: partition), row[0])
                XCTAssertEqual(fact.id, fuelingId, row[0])
                XCTAssertEqual(fact.assetId, args["asset"], row[0])
                XCTAssertEqual(fact.product.rawValue, args["product"], row[0])
                XCTAssertEqual(fact.product.unit, args["unit"], row[0])
                XCTAssertEqual(String(fact.quantityMilli), args["quantity"], row[0])
                XCTAssertEqual(String(fact.paidCentavos), args["paid"], row[0])
                XCTAssertEqual(fact.payment.rawValue, args["payment"], row[0])
                XCTAssertNil(EnterpriseFuelingOperations.fact(built, partition: other), row[0])
            } else {
                let refusal: EnterpriseOperationError = row[3] == "UNSUPPORTED" ? .unsupported : .malformed
                XCTAssertThrowsError(try EnterpriseFuelingOperations.build(envelope: envelope, fuelingId: fuelingId, input: command), row[0]) {
                    XCTAssertEqual($0 as? EnterpriseOperationError, refusal, row[0])
                }
                let sealed = EnterpriseSealedOperation(operationId: envelope.operationId, type: EnterpriseFuelingOperations.submit,
                    entityType: "fueling", entityId: fuelingId, payloadDigest: "", canonical: expected)
                XCTAssertNil(EnterpriseFuelingOperations.fact(sealed, partition: partition), row[0])
            }
        }
        XCTAssertEqual(coverage, ["OK": 15, "MALFORMED": 39, "UNSUPPORTED": 5])
    }

    func testDriverAuthorizationNoticeExpiryFromSharedVector() throws {
        let text = try fixture("voucher-limits-v1", folder: "agreement",
                               digest: "80d9fedb7df8230feee0d5df9c9ae8e2daf4a29f6b270f1f3fdc1e841554cf5e")
        let dates = ISO8601DateFormatter()
        var checked = 0
        for line in text.split(whereSeparator: \.isNewline).dropFirst() {
            let row = line.split(separator: "|").map(String.init)
            guard row.count == 4, row[1] == "ELIGIBLE" else { continue }
            let args = try fields(row[2])
            let expected = try fields(row[3])
            let now = try XCTUnwrap(dates.date(from: XCTUnwrap(args["now"])))
            let expiry = try XCTUnwrap(dates.date(from: XCTUnwrap(args["valid_until"])))
            let state = try XCTUnwrap(EnterpriseFuelingAuthorizationNotice.State(rawValue: XCTUnwrap(args["state"])))
            let notice = try XCTUnwrap(EnterpriseFuelingAuthorizationNotice(
                id: membership, product: "DIESEL_S10", unit: "L", maximumQuantityMilli: 200_000, maximumCentavos: 120_000,
                validUntilEpochSeconds: Int64(expiry.timeIntervalSince1970), storedState: state
            ))
            let nowSeconds = Int64(now.timeIntervalSince1970)
            XCTAssertEqual(notice.state(at: nowSeconds).rawValue, expected["derived_state"], row[0])
            XCTAssertEqual(EnterpriseFuelingAuthorizationNotice.nearestUsable([notice], at: nowSeconds) != nil,
                           expected["eligible"] == "true", row[0])
            if state == .issued {
                XCTAssertEqual(notice.state(at: notice.validUntilEpochSeconds), .issued)
                XCTAssertEqual(notice.state(at: notice.validUntilEpochSeconds + 1), .expired)
            }
            checked += 1
        }
        XCTAssertEqual(checked, 5)
    }

    private let tripId = "7b0e2a54-0000-4000-8000-000000009511"
    private let requestId = "7b0e2a54-0000-4000-8000-000000009512"
    private let voucherId = "7b0e2a54-0000-4000-8000-000000009513"

    private func tripFieldResponse(voucher: String? = nil, plan: String = "null", counts: String? = nil,
                                   trip: String? = nil, request: String? = nil, extra: String = "") -> Data {
        let notice = voucher ?? #"{"id":"\#(voucherId)","product":"DIESEL_S10","unit":"L","max_quantity_milli":150000,"max_centavos":90000,"valid_until":1800000000,"state":"ISSUED"}"#
        let numbers = counts ?? #""expired":1,"due_soon":2,"nearest_days":5,"open_maintenance":1"#
        // The Go encoder's field order and trailing newline (orgsync reference handler).
        return Data((#"{"schema_version":1,"request_id":"\#(request ?? requestId)","membership_id":"\#(membership)","enrollment_id":"\#(enrollment)","field":{"trip_id":"\#(trip ?? tripId)","plan":\#(plan),"#
            + numbers + #","voucher":\#(notice)\#(extra)}}"# + "\n").utf8)
    }

    private func parseTripField(_ data: Data) throws -> EnterpriseTripField {
        try EnterpriseTripFieldWire.parse(data, requestId: requestId, membershipId: membership, enrollmentId: enrollment, tripId: tripId)
    }

    func testTripFieldRequestNamesOnlyTheOpenTripWithoutACursor() throws {
        let body = try EnterpriseTripFieldWire.requestBody(
            requestId: requestId, familyId: account, accessToken: "access-token", membershipId: membership,
            enrollmentId: enrollment, tripId: tripId
        )
        let fields = try XCTUnwrap(EnterpriseCanonicalJson.parse(body).objectValue)
        XCTAssertEqual(Set(fields.keys), ["schema_version", "request_id", "family_id", "access_token", "membership_id",
                                          "enrollment_id", "cursor", "trip_id"])
        XCTAssertEqual(fields["cursor"], .null)
        XCTAssertEqual(fields["trip_id"], .text(tripId))
        XCTAssertEqual(fields["schema_version"], .int(1))
        XCTAssertThrowsError(try EnterpriseTripFieldWire.requestBody(
            requestId: requestId, familyId: account, accessToken: "access-token", membershipId: membership,
            enrollmentId: enrollment, tripId: tripId.uppercased()
        ))
    }

    func testTripFieldKeepsOnlyTheDriverSafeAuthorizationNotice() throws {
        let field = try parseTripField(tripFieldResponse(
            plan: #"{"id":"\#(voucherId)","waypoints":[{"lat_e7":-155000000,"label":"Base"}],"heavy_asset_notice":false}"#
        ))
        XCTAssertEqual(field.tripId, tripId)
        XCTAssertTrue(field.hasRoutePlan)
        XCTAssertEqual(field.expiredDocuments, 1)
        XCTAssertEqual(field.documentsDueSoon, 2)
        XCTAssertEqual(field.nearestDueDays, 5)
        XCTAssertEqual(field.openMaintenanceRequests, 1)
        let notice = try XCTUnwrap(field.authorization)
        XCTAssertEqual(notice.id, voucherId)
        XCTAssertEqual(notice.product, "DIESEL_S10")
        XCTAssertEqual(notice.unit, "L")
        XCTAssertEqual(notice.maximumQuantityMilli, 150_000)
        XCTAssertEqual(notice.maximumCentavos, 90_000)
        XCTAssertEqual(notice.validUntilEpochSeconds, 1_800_000_000)
        XCTAssertEqual(notice.state(at: 1_800_000_000), .issued)
        XCTAssertEqual(notice.state(at: 1_800_000_001), .expired)

        let none = try parseTripField(tripFieldResponse(voucher: "null", counts: #""expired":0,"due_soon":0,"nearest_days":null,"open_maintenance":0"#))
        XCTAssertNil(none.authorization)
        XCTAssertFalse(none.hasRoutePlan)
        XCTAssertNil(none.nearestDueDays)
        for state in ["REDEEMED", "CANCELLED", "LOCKED"] {
            let used = try parseTripField(tripFieldResponse(voucher: #"{"id":"\#(voucherId)","product":"LPG_P13","unit":"KG_13","max_quantity_milli":2000,"max_centavos":24000,"valid_until":1800000000,"state":"\#(state)"}"#))
            XCTAssertEqual(used.authorization?.state(at: 1_900_000_000).rawValue, state)
        }
    }

    func testTripFieldRefusesCodesUnknownShapesAndForeignBindings() throws {
        let invalid: [Data] = [
            // A field that would carry a redemption code is never a driver projection.
            tripFieldResponse(voucher: #"{"id":"\#(voucherId)","product":"DIESEL_S10","unit":"L","max_quantity_milli":1,"max_centavos":1,"valid_until":1800000000,"state":"ISSUED","code":"123456"}"#),
            tripFieldResponse(voucher: #"{"id":"\#(voucherId)","product":"DIESEL_S10","unit":"L","max_quantity_milli":1,"max_centavos":1,"valid_until":1800000000,"state":"EXPIRED"}"#),
            tripFieldResponse(voucher: #"{"id":"\#(voucherId)","product":"ARLA_32","unit":"L","max_quantity_milli":1,"max_centavos":1,"valid_until":1800000000,"state":"ISSUED"}"#),
            tripFieldResponse(voucher: #"{"id":"\#(voucherId)","product":"DIESEL_S10","unit":"L","max_quantity_milli":1.5,"max_centavos":1,"valid_until":1800000000,"state":"ISSUED"}"#),
            tripFieldResponse(voucher: #"{"id":"\#(voucherId)","product":"DIESEL_S10","unit":"L","max_quantity_milli":0,"max_centavos":1,"valid_until":1800000000,"state":"ISSUED"}"#),
            tripFieldResponse(counts: #""expired":0,"due_soon":1,"nearest_days":null,"open_maintenance":0"#),
            tripFieldResponse(counts: #""expired":-1,"due_soon":0,"nearest_days":null,"open_maintenance":0"#),
            tripFieldResponse(counts: #""expired":true,"due_soon":0,"nearest_days":null,"open_maintenance":0"#),
            tripFieldResponse(plan: "[]"),
            tripFieldResponse(extra: #","promotions":[]"#),
            Data(),
            Data(repeating: 0x20, count: EnterpriseTripFieldWire.maxResponseBytes + 1),
        ]
        for (index, data) in invalid.enumerated() {
            XCTAssertThrowsError(try parseTripField(data), "case \(index)")
        }
        XCTAssertThrowsError(try parseTripField(tripFieldResponse(trip: voucherId))) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .binding)
        }
        XCTAssertThrowsError(try parseTripField(tripFieldResponse(request: voucherId))) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .binding)
        }
    }

    func testReceiptCapabilityStopsOnOwnerSwitchWithoutSendingAccountProof() async throws {
        let original = session(account: account)
        let sessions = StaticSessions(session: original)
        let body = "{\"document\":{\"id\":\"\(enrollment)\",\"statuses\":{\"storage\":\"RECEIVED\"},\"revisions\":{\"storage\":2}}}"
        let result = ScriptedExchange { _ in ScriptedExchange.Reply(200, body) }
        let transport = try signedTransport(sessions, exchange: result)
        let client = EnterpriseSyncClient(transport: transport, sessions: sessions, uploads: result)
        let partition = try XCTUnwrap(EnterprisePartition(accountId: account, origin: origin, organizationId: organization))
        let content = Data([0xFF, 0xD8, 0xFF, 0xD9])
        let storage = try await client.upload(partition: partition, documentId: enrollment, capability: "test-token", content: content)
        XCTAssertEqual(storage, .received)
        let request = try XCTUnwrap(result.requests.first)
        XCTAssertEqual(request.url?.path, "/v1/orgs/\(organization)/documents/\(enrollment)/content/upload")
        XCTAssertEqual(request.httpBody, content)
        XCTAssertFalse(request.httpShouldHandleCookies)
        XCTAssertEqual(request.value(forHTTPHeaderField: EnterpriseAttachmentWire.capabilityHeader), "test-token")
        for header in ["Authorization", "Cookie", "Origin", "Signature", "Signature-Input", "Content-Digest"] {
            XCTAssertNil(request.value(forHTTPHeaderField: header))
        }

        let switched = session(account: membership)
        let duringUpload = ScriptedExchange { _ in
            sessions.set(switched)
            return ScriptedExchange.Reply(200, body)
        }
        let interrupted = EnterpriseSyncClient(transport: transport, sessions: sessions, uploads: duringUpload)
        do {
            _ = try await interrupted.upload(partition: partition, documentId: enrollment, capability: "test-token", content: content)
            XCTFail("A response after an account switch must not update the prior receipt")
        } catch {
            XCTAssertEqual(error as? EnterpriseSyncError, .signInRequired)
        }
        do {
            _ = try await interrupted.upload(partition: partition, documentId: enrollment, capability: "test-token", content: content)
            XCTFail("A capability upload must not start under another account")
        } catch {
            XCTAssertEqual(error as? EnterpriseSyncError, .signInRequired)
        }
        XCTAssertEqual(duringUpload.requests.count, 1)
    }

    func testSignedAttachmentDoesNotSendProofFromRotatedSessionFamily() async throws {
        let sessions = StaticSessions(session: session(account: account))
        let rotated = session(account: account, family: membership)
        let exchange = ScriptedExchange { request in
            let reply = FakeIdentityBackend.reply(request) { _ in ScriptedExchange.Reply(500, "{}") }
            if request.url?.path == "/v1/identity/challenges", FakeIdentityBackend.json(request)["purpose"] as? String == "SIGN" {
                sessions.set(rotated)
            }
            return reply
        }
        let transport = try signedTransport(sessions, exchange: exchange)
        let client = EnterpriseSyncClient(transport: transport, sessions: sessions, uploads: exchange)
        let partition = try XCTUnwrap(EnterprisePartition(accountId: account, origin: origin, organizationId: organization))
        do {
            _ = try await client.openAttachment(
                partition: partition, enrollment: EnterpriseEnrollment(membershipId: membership, enrollmentId: enrollment, revision: 1),
                declarationId: account, documentId: enrollment, linkId: membership,
                sha256: String(repeating: "a", count: 64), bytes: 4, targetKind: "FUELING", targetId: organization
            )
            XCTFail("A changed session family must retire the signed attachment request")
        } catch {
            XCTAssertEqual(error as? EnterpriseSyncError, .signInRequired)
        }
        XCTAssertTrue(exchange.requests.contains { FakeIdentityBackend.json($0)["purpose"] as? String == "SIGN" })
        XCTAssertFalse(exchange.requests.contains { $0.url?.path.hasSuffix("/sync/attachments") == true })
    }

    private func session(account: String, family: String = "f0000000-0000-4000-8000-000000000001") -> PortableAuth.Session {
        PortableAuth.Session(familyId: family, accountId: account, accessToken: "access-token", refreshToken: "refresh-token",
                             accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000)
    }

    private func signedTransport(_ sessions: StaticSessions, exchange: ScriptedExchange) throws -> SignedRequestTransport {
        SignedRequestTransport(origin: origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange, sessions: sessions,
            nowEpochSeconds: { 1_763_892_000 })
    }
}
