import Foundation
import XCTest
@testable import AnpFuelCore

/// Wire shapes checked against `contracts/testdata/api` fixtures (values copied verbatim).
final class CommunityWriteWireTests: XCTestCase {
    private let stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private let session = PortableAuth.Session(
        familyId: "11111111-1111-4111-8111-111111111111",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "fixture-access-token",
        refreshToken: "refresh",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )

    private func object(_ data: Data?) throws -> [String: Any] {
        let bytes = try XCTUnwrap(data)
        return try XCTUnwrap((try JSONSerialization.jsonObject(with: bytes)) as? [String: Any])
    }

    func testObservationBodyMatchesTheSubmitFixture() throws {
        let draft = CommunityPriceDraft(
            stationId: stationId,
            stationName: "Posto",
            product: .gasolineRegular,
            amountMilli: 5999,
            condition: .standard,
            capturedAtMillis: 1_759_017_600_000
        )
        let body = try object(CommunityWire.observationBody(clientSubmissionId: "sub-2026-09-28-001", price: draft))
        XCTAssertEqual(body["client_submission_id"] as? String, "sub-2026-09-28-001")
        XCTAssertEqual(body["station_id"] as? String, stationId)
        XCTAssertEqual(body["fuel_product"] as? String, "GASOLINE_REGULAR")
        let price = try XCTUnwrap(body["price"] as? [String: Any])
        XCTAssertEqual(price["amount_milli_brl"] as? Int64, 5999)
        XCTAssertEqual(price["currency"] as? String, "BRL")
        XCTAssertEqual(price["unit"] as? String, "L")
        XCTAssertEqual((body["condition"] as? [String: Any])?["kind"] as? String, "STANDARD")
        XCTAssertEqual(body["claimed_captured_at"] as? String, "2025-09-28T00:00:00.000Z")
        XCTAssertNil(body["evidence_id"])
        XCTAssertNil(body["photo_capture_id"])
        XCTAssertNil(body["location"], "no device location leaves the phone")
        let rawText = String(decoding: try XCTUnwrap(CommunityWire.observationBody(clientSubmissionId: "s", price: draft)), as: UTF8.self)
        XCTAssertTrue(rawText.contains("\"amount_milli_brl\":5999"), "amounts stay canonical integers")
    }

    func testObservationBodyMapsUnitsAndRefusesInvalidDrafts() throws {
        func draft(_ product: FuelProduct, _ amount: Int64, _ condition: PriceConditionKind) -> CommunityPriceDraft {
            CommunityPriceDraft(stationId: stationId, stationName: "P", product: product, amountMilli: amount, condition: condition, capturedAtMillis: 0)
        }
        let premium = try object(CommunityWire.observationBody(clientSubmissionId: "s", price: draft(.gasolinePremium, 6000, .cash)))
        XCTAssertEqual(premium["fuel_product"] as? String, "GASOLINE_ADDITIVED")
        XCTAssertEqual((premium["condition"] as? [String: Any])?["kind"] as? String, "CASH")
        let cng = try object(CommunityWire.observationBody(clientSubmissionId: "s", price: draft(.cng, 4000, .standard)))
        XCTAssertEqual((cng["price"] as? [String: Any])?["unit"] as? String, "M3")
        let lpg = try object(CommunityWire.observationBody(clientSubmissionId: "s", price: draft(.lpgP13, 110_000, .standard)))
        XCTAssertEqual((lpg["price"] as? [String: Any])?["unit"] as? String, "KG_13")

        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "s", price: draft(.ethanol, 4000, .pix)))
        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "s", price: draft(.ethanol, 0, .standard)))
        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "s", price: draft(.ethanol, 1_000_001, .standard)))
        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "", price: draft(.ethanol, 4000, .standard)))
        let badStation = CommunityPriceDraft(stationId: "st-1", stationName: "P", product: .ethanol, amountMilli: 4000, condition: .standard, capturedAtMillis: 0)
        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "s", price: badStation))
    }

    func testObservationWithEvidenceCarriesCaptureIdsAndCaptureTime() throws {
        let photo = CommunityPhotoEvidence(
            photoId: "photo-1",
            captureId: "e0000000-0000-4000-8000-000000000003",
            capturedAtMillis: 1_764_000_000_123,
            captureExpiresAtMillis: 1_764_086_400_000,
            uploadSessionId: "e0000000-0000-4000-8000-000000000001",
            evidenceId: "e0000000-0000-4000-8000-000000000002"
        )
        var draft = CommunityPriceDraft(
            stationId: stationId,
            stationName: "Posto",
            product: .ethanol,
            amountMilli: 3890,
            condition: .standard,
            capturedAtMillis: 1_764_000_000_123,
            photo: photo
        )
        let body = try object(CommunityWire.observationBody(clientSubmissionId: "sub-1", price: draft))
        XCTAssertEqual(body["evidence_id"] as? String, "e0000000-0000-4000-8000-000000000002")
        XCTAssertEqual(body["photo_capture_id"] as? String, "e0000000-0000-4000-8000-000000000003")
        XCTAssertEqual(body["claimed_captured_at"] as? String, "2025-11-24T16:00:00.123Z")

        var withoutEvidence = photo
        withoutEvidence.evidenceId = nil
        draft.photo = withoutEvidence
        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "sub-1", price: draft), "a photo needs its evidence receipt first")
        draft = CommunityPriceDraft(stationId: stationId, stationName: "Posto", product: .ethanol, amountMilli: 3890, condition: .credit, capturedAtMillis: 0, photo: photo)
        XCTAssertNil(CommunityWire.observationBody(clientSubmissionId: "sub-1", price: draft), "evidence photos use the standard pump price")
    }

    func testObservationReceiptStatusAndHistoryDecodeFixtures() throws {
        let receipt = Data("{\"id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad27\",\"validation_state\":\"RECEIVED\",\"received_at\":\"2026-09-28T00:00:00Z\",\"status_url\":\"/v1/observations/d6c74c23-63db-4c24-a2e5-408cb23bad27\"}".utf8)
        XCTAssertEqual(CommunityWire.decodeObservationReceipt(receipt)?.id, "d6c74c23-63db-4c24-a2e5-408cb23bad27")
        let notReceived = Data("{\"id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad27\",\"validation_state\":\"VALIDATED\"}".utf8)
        XCTAssertNil(CommunityWire.decodeObservationReceipt(notReceived), "intake acknowledges only RECEIVED")

        let row = "{\"id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad27\",\"station_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\",\"fuel_product\":\"GASOLINE_REGULAR\",\"unit\":\"L\",\"amount_milli_brl\":5999,\"condition\":{\"kind\":\"STANDARD\",\"qualifier_id\":null},\"validation_state\":\"RECEIVED\",\"received_at\":\"2026-09-28T00:00:00Z\",\"policy_version\":\"community-pricing-v1\"}"
        let status = Data("{\"observation\":\(row),\"validation_state\":\"RECEIVED\",\"decisions\":[]}".utf8)
        let observation = try XCTUnwrap(CommunityWire.decodeObservationStatus(status))
        XCTAssertEqual(observation.amountMilli, 5999)
        XCTAssertEqual(observation.product, .gasolineRegular)
        XCTAssertEqual(observation.conditionKind, "STANDARD")
        let mismatch = Data("{\"observation\":\(row),\"validation_state\":\"VALIDATED\",\"decisions\":[]}".utf8)
        XCTAssertNil(CommunityWire.decodeObservationStatus(mismatch))

        let page = Data("{\"items\":[\(row)],\"next_cursor\":null,\"generated_at\":\"2026-09-28T00:00:00Z\"}".utf8)
        let decoded = try XCTUnwrap(CommunityWire.decodeOwnObservations(page))
        XCTAssertEqual(decoded.items.count, 1)
        XCTAssertNil(decoded.nextCursor)
        let fractional = Data("{\"items\":[{\"id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad27\",\"station_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\",\"fuel_product\":\"ETHANOL\",\"unit\":\"L\",\"amount_milli_brl\":5.5,\"condition\":{\"kind\":\"STANDARD\"},\"validation_state\":\"RECEIVED\"}]}".utf8)
        XCTAssertNil(CommunityWire.decodeOwnObservations(fractional))
    }

    func testPhotoCaptureGrantEnforcesPolicyStationAndWindows() throws {
        let issued: Int64 = 1_764_000_000_000
        func grant(policy: String = "photo-capture-community-v2", station: String? = nil, cameraSeconds: Int = 120, now: Int64? = nil) -> PhotoCaptureGrant? {
            let camera = CommunityWire.formatRfc3339Millis(issued + Int64(cameraSeconds) * 1000)
            let expires = CommunityWire.formatRfc3339Millis(issued + 86_400_000)
            let json = "{\"capture_id\":\"e0000000-0000-4000-8000-000000000003\",\"station_id\":\"\(station ?? stationId)\",\"issued_at\":\"2025-11-24T16:00:00.123456789Z\",\"camera_expires_at\":\"\(camera)\",\"expires_at\":\"\(expires)\",\"policy_version\":\"\(policy)\"}"
            return CommunityWire.decodePhotoCaptureGrant(Data(json.utf8), stationId: stationId, nowMillis: now ?? issued + 1_000)
        }
        let valid = try XCTUnwrap(grant())
        XCTAssertEqual(valid.issuedAtMillis, issued + 123)
        XCTAssertTrue(valid.admits(stationId: stationId.uppercased(), capturedAtMillis: issued + 5_000))
        XCTAssertFalse(valid.admits(stationId: stationId, capturedAtMillis: issued + 121_000))
        XCTAssertFalse(valid.admits(stationId: "d6c74c23-63db-4c24-a2e5-408cb23bad99", capturedAtMillis: issued + 5_000))
        XCTAssertNil(grant(policy: "photo-capture-ui-test-v1"))
        XCTAssertNil(grant(station: "d6c74c23-63db-4c24-a2e5-408cb23bad99"))
        XCTAssertNil(grant(cameraSeconds: 600))
        XCTAssertNil(grant(now: issued + 200_000), "the camera window already closed")
        XCTAssertNil(grant(now: issued - 120_000), "issued too far in the device's future")
        XCTAssertNotNil(CommunityWire.photoCaptureBody(clientCaptureId: "e0000000-0000-4000-8000-000000000009", stationId: stationId))
        XCTAssertNil(CommunityWire.photoCaptureBody(clientCaptureId: "capture-1", stationId: stationId))
    }

    func testUploadReserveAuthorizationAndStatus() throws {
        let now: Int64 = 1_764_000_000_000
        let reserve = try object(CommunityWire.uploadReserveBody(
            captureId: "e0000000-0000-4000-8000-000000000003",
            stationId: stationId,
            sizeBytes: 524_288,
            sha256Hex: String(repeating: "a", count: 64),
            capturedAtMillis: now
        ))
        XCTAssertEqual(reserve["client_submission_id"] as? String, "photo:e0000000-0000-4000-8000-000000000003")
        XCTAssertEqual(reserve["content_type"] as? String, "image/jpeg")
        XCTAssertEqual(reserve["size_bytes"] as? Int, 524_288)
        XCTAssertEqual(reserve["captured_at"] as? String, "2025-11-24T16:00:00.000Z")

        func authorization(url: String = "https://media.example.invalid/obj?X-Amz-Signature=abc", headers: String = "{\"Content-Type\":\"image/jpeg\"}", expiresIn: Int64 = 240_000, maxBytes: Int = 262_144) -> UploadAuthorization? {
            let expires = CommunityWire.formatRfc3339Millis(now + expiresIn)
            let session = CommunityWire.formatRfc3339Millis(now + 3_600_000)
            let json = "{\"upload_id\":\"e0000000-0000-4000-8000-000000000001\",\"method\":\"PUT\",\"url\":\"\(url)\",\"required_headers\":\(headers),\"expires_at\":\"\(expires)\",\"max_bytes\":\(maxBytes),\"session_expires_at\":\"\(session)\"}"
            return CommunityWire.decodeUploadAuthorization(Data(json.utf8), nowMillis: now, sessionDeadlineMillis: now + 86_400_000, byteCount: 150_000)
        }
        let valid = try XCTUnwrap(authorization())
        XCTAssertEqual(valid.contentTypeHeader, "Content-Type")
        XCTAssertNil(authorization(url: "http://media.example.invalid/obj?X-Amz-Signature=abc"))
        XCTAssertNil(authorization(url: "https://media.example.invalid/obj"))
        XCTAssertNil(authorization(headers: "{\"Content-Type\":\"image/jpeg\",\"X-Extra\":\"1\"}"))
        XCTAssertNil(authorization(headers: "{\"Content-Type\":\"image/png\"}"))
        XCTAssertNil(authorization(expiresIn: 3_000_000))
        XCTAssertNil(authorization(maxBytes: 100))

        let ready = Data("{\"upload_id\":\"e0000000-0000-4000-8000-000000000001\",\"state\":\"READY\",\"evidence_id\":\"e0000000-0000-4000-8000-000000000002\",\"updated_at\":\"2026-09-30T12:01:00Z\"}".utf8)
        XCTAssertEqual(CommunityWire.decodeUploadStatus(ready)?.evidenceId, "e0000000-0000-4000-8000-000000000002")
        let issued = Data("{\"upload_id\":\"e0000000-0000-4000-8000-000000000001\",\"state\":\"ISSUED\",\"evidence_id\":null,\"updated_at\":\"2026-09-30T12:00:00Z\"}".utf8)
        let issuedStatus = try XCTUnwrap(CommunityWire.decodeUploadStatus(issued))
        XCTAssertNil(issuedStatus.evidenceId)
        XCTAssertNil(try CommunityWriteClient.progress(from: issuedStatus))
        XCTAssertEqual(try CommunityWriteClient.progress(from: try XCTUnwrap(CommunityWire.decodeUploadStatus(ready)))?.evidenceId, "e0000000-0000-4000-8000-000000000002")
        XCTAssertThrowsError(try CommunityWriteClient.progress(from: UploadStatus(uploadId: "u", state: "EXPIRED", evidenceId: nil))) { error in
            XCTAssertEqual(error as? CommunityWriteError, .photoExpired)
        }
        XCTAssertThrowsError(try CommunityWriteClient.progress(from: UploadStatus(uploadId: "u", state: "REJECTED", evidenceId: nil))) { error in
            XCTAssertEqual(error as? CommunityWriteError, .photoRejected)
        }
    }

    func testStationCommentVoteAndTallyShapes() throws {
        let body = try object(CommunityWire.stationCommentBody(session: session, stationId: stationId, productWire: "GASOLINE_ADDITIVED", text: "  Preço bom ⛽\r\n"))
        XCTAssertEqual(body["family_id"] as? String, session.familyId)
        XCTAssertEqual(body["access_token"] as? String, session.accessToken)
        XCTAssertEqual(body["station_id"] as? String, stationId)
        XCTAssertEqual(body["product"] as? String, "GASOLINE_ADDITIVED")
        XCTAssertEqual(body["text"] as? String, "Preço bom ⛽")
        XCTAssertEqual(Set(body.keys), ["family_id", "access_token", "station_id", "product", "text"])
        XCTAssertNotNil(CommunityWire.stationCommentBody(session: session, stationId: stationId, productWire: "ETHANOL", text: String(repeating: "a", count: 280)))
        XCTAssertNil(CommunityWire.stationCommentBody(session: session, stationId: stationId, productWire: "ETHANOL", text: String(repeating: "a", count: 281)))
        XCTAssertNil(CommunityWire.stationCommentBody(session: session, stationId: stationId, productWire: "ETHANOL", text: "   "))

        let view = Data("{\"comment\":{\"created_at\":\"2026-10-01T00:00:00Z\",\"depth\":0,\"id\":\"bbd138ce-e442-4f57-a874-3c05d2e16f52\",\"parent_id\":null,\"product\":\"GASOLINE_ADDITIVED\",\"public_alias\":\"adore-fox-42\",\"revision\":1,\"station_id\":\"d6c74c23-63db-4c24-a2e5-408cb23bad26\",\"text\":\"Preço bom ⛽\",\"updated_at\":\"2026-10-01T00:00:00Z\"}}".utf8)
        XCTAssertEqual(CommunityWire.decodeStationCommentReceipt(view), StationCommentReceipt(id: "bbd138ce-e442-4f57-a874-3c05d2e16f52", revision: 1))

        let vote = try object(CommunityWire.voteBody(session: session, choice: "VALID"))
        XCTAssertEqual(vote["choice"] as? String, "VALID")
        XCTAssertNil(CommunityWire.voteBody(session: session, choice: "HELPFUL"))
        let tally = Data("{\"tally\":{\"comment_id\":\"bbd138ce-e442-4f57-a874-3c05d2e16f52\",\"invalid\":1,\"percentage_bps\":6666,\"revision\":1,\"total\":3,\"valid\":2}}".utf8)
        XCTAssertEqual(
            CommunityWire.decodeTally(tally),
            StationCommentTally(commentId: "bbd138ce-e442-4f57-a874-3c05d2e16f52", revision: 1, valid: 2, invalid: 1)
        )
    }

    func testForumBodiesFollowTheBackendRequestShape() throws {
        let topic = CommunityTextDraft(
            text: "Observacao sobre a paridade de precos entre etanol e gasolina nos postos da regiao de Campinas.",
            categoryCode: "FUELS",
            title: "Preco do etanol no interior paulista",
            regionUf: "SP",
            regionMunicipalityIbge: 3_509_502
        )
        let body = try object(CommunityWire.forumTopicBody(session: session, operationId: "22222222-2222-4222-8222-222222222222", draft: topic))
        XCTAssertEqual(body["operation_id"] as? String, "22222222-2222-4222-8222-222222222222")
        XCTAssertEqual(body["category"] as? String, "FUELS")
        XCTAssertEqual(body["title"] as? String, "Preco do etanol no interior paulista")
        XCTAssertEqual(body["region_uf"] as? String, "SP")
        XCTAssertEqual(body["region_municipality_ibge"] as? Int, 3_509_502)
        XCTAssertNil(body["region"], "the backend decodes flat region fields and refuses unknown ones")
        XCTAssertEqual(Set(body.keys), ["family_id", "access_token", "operation_id", "category", "title", "body_text", "region_uf", "region_municipality_ibge"])
        XCTAssertNil(CommunityWire.forumTopicBody(session: session, operationId: "op-1", draft: topic))
        let longTitle = CommunityTextDraft(text: "body", categoryCode: "FUELS", title: String(repeating: "t", count: 121))
        XCTAssertNil(CommunityWire.forumTopicBody(session: session, operationId: "22222222-2222-4222-8222-222222222222", draft: longTitle))

        let comment = CommunityTextDraft(text: "Excelente analise da formacao de precos.", postId: "44444444-4444-4444-8444-444444444444")
        let commentBody = try object(CommunityWire.forumCommentBody(session: session, operationId: "22222222-2222-4222-8222-222222222222", draft: comment))
        XCTAssertEqual(Set(commentBody.keys), ["family_id", "access_token", "operation_id", "body_text"])
        let reply = CommunityTextDraft(text: "Reply", postId: "44444444-4444-4444-8444-444444444444", parentId: "33333333-3333-4333-8333-333333333333")
        XCTAssertEqual(try object(CommunityWire.forumCommentBody(session: session, operationId: "22222222-2222-4222-8222-222222222222", draft: reply))["parent_id"] as? String, "33333333-3333-4333-8333-333333333333")
        let tooLong = CommunityTextDraft(text: String(repeating: "x", count: 281), postId: "44444444-4444-4444-8444-444444444444")
        XCTAssertNil(CommunityWire.forumCommentBody(session: session, operationId: "22222222-2222-4222-8222-222222222222", draft: tooLong))

        let topicReceipt = Data("{\"id\":\"33333333-3333-4333-8333-333333333333\",\"kind\":\"TOPIC\",\"category\":\"FUELS\",\"title\":\"Preco do etanol no interior paulista\",\"body_text\":\"Observacao.\",\"revision\":1,\"publication_state\":\"PENDING_REVIEW\",\"visibility\":\"VISIBLE\",\"region\":{\"uf\":\"SP\",\"municipality_ibge\":3509502},\"fuel_product\":\"ETHANOL\",\"station_id\":null,\"public_alias\":\"Motorista Campinas\",\"created_at\":\"2026-11-06T12:00:00Z\",\"updated_at\":\"2026-11-06T12:00:00Z\",\"published_at\":null,\"replayed\":false}".utf8)
        let decodedTopic = try XCTUnwrap(CommunityWire.decodeForumReceipt(topicReceipt, wrapper: "topic"))
        XCTAssertEqual(decodedTopic.publicationState, "PENDING_REVIEW")
        XCTAssertFalse(decodedTopic.replayed)
        let wrapped = Data("{\"comment\":{\"id\":\"33333333-3333-4333-8333-333333333333\",\"post_id\":\"44444444-4444-4444-8444-444444444444\",\"parent_id\":null,\"depth\":0,\"sequence\":1,\"revision\":1,\"publication_state\":\"PENDING_REVIEW\",\"visibility\":\"VISIBLE\",\"body_text\":\"Texto.\",\"replayed\":true}}".utf8)
        XCTAssertEqual(CommunityWire.decodeForumReceipt(wrapped, wrapper: "comment")?.replayed, true)
        XCTAssertNil(CommunityWire.decodeForumReceipt(Data("{\"id\":\"x\"}".utf8), wrapper: "topic"))
    }

    func testRfc3339MillisFormattingAndParsing() {
        XCTAssertEqual(CommunityWire.formatRfc3339Millis(1_764_000_000_123), "2025-11-24T16:00:00.123Z")
        XCTAssertEqual(CommunityWire.formatRfc3339Millis(1_759_017_600_000), "2025-09-28T00:00:00.000Z")
        XCTAssertEqual(CommunityWire.formatRfc3339Millis(1_759_017_600_007), "2025-09-28T00:00:00.007Z")
        XCTAssertEqual(CommunityWire.parseRfc3339Millis("2025-11-24T16:00:00.123Z"), 1_764_000_000_123)
        XCTAssertEqual(CommunityWire.parseRfc3339Millis("2025-11-24T16:00:00.123456789Z"), 1_764_000_000_123)
        XCTAssertEqual(CommunityWire.parseRfc3339Millis("2025-11-24T16:00:00.5Z"), 1_764_000_000_500)
        XCTAssertEqual(CommunityWire.parseRfc3339Millis("2025-11-24T16:00:00Z"), 1_764_000_000_000)
        XCTAssertEqual(CommunityWire.parseRfc3339Millis("2025-11-24T13:00:00-03:00"), 1_764_000_000_000)
        XCTAssertNil(CommunityWire.parseRfc3339Millis("2025-11-24T16:00:00.Z"))
        XCTAssertNil(CommunityWire.parseRfc3339Millis("yesterday"))
    }
}
