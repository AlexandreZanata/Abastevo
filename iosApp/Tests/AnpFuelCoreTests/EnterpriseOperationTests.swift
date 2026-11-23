import XCTest
@testable import AnpFuelCore

/// Operation shapes replayed from `contracts/testdata/enterprise` (trip-operations-v3,
/// expense-submit-v1, checklist run-v1 and the ACK rows of sync/operations-v1), copied
/// verbatim; `scripts/tests/test_ios_app_integrity.py` checks every copied row.
final class EnterpriseOperationTests: XCTestCase {
    // MARK: - Fixture envelopes

    private func envelope(prefix: String, operation: String, at: String = "2026-11-15T12:00:00.123Z",
                          zone: String = "America/Cuiaba", offset: Int64 = -14_400) throws -> EnterpriseEnvelope {
        let partition = try XCTUnwrap(EnterprisePartition(
            accountId: prefix + "000000009501", origin: "https://api.example.invalid", organizationId: prefix + "000000009502"
        ))
        return EnterpriseEnvelope(
            partition: partition,
            membershipId: prefix + "000000009503",
            enrollmentId: prefix + "000000009504",
            operationId: operation,
            declared: EnterpriseTimeDeclaration(occurredAt: at, entryZone: zone, deviceOffsetSeconds: offset)
        )
    }

    private let trip = "7b0e2a54-0000-4000-8000-"
    private let expense = "9e5b0000-0000-4000-8000-"

    private func text(_ operation: EnterpriseSealedOperation) -> String {
        return String(decoding: operation.canonical, as: UTF8.self)
    }

    // MARK: - Trips

    func testTripStartWithoutRunClockMatchesTheSharedVector() throws {
        let operation = try EnterpriseOperationBuilder.startTrip(
            envelope: envelope(prefix: trip, operation: trip + "000000009601"),
            tripId: trip + "000000009611",
            assetId: trip + "000000009621",
            driverEmployeeId: trip + "000000009631",
            evidence: .reading(meterId: trip + "000000009651", segmentId: trip + "000000009652", kind: .odometer,
                               readingId: trip + "000000009653", value: 0, modulus: nil),
            departureRunId: nil
        )
        XCTAssertEqual(text(operation), Vectors.start_reading_without_run_clock)
        XCTAssertEqual(operation.payloadDigest, "sha256:01c431e4f5b757896fabecf24e9f2d8c70f2a20f96e868e79dc4768011363754")
    }

    func testTripStartInUtcAtTheOdometerBoundMatchesTheSharedVector() throws {
        let operation = try EnterpriseOperationBuilder.startTrip(
            envelope: envelope(prefix: trip, operation: trip + "000000009601", at: "2026-11-15T00:00:00Z", zone: "UTC", offset: 0),
            tripId: trip + "000000009611",
            assetId: trip + "000000009621",
            driverEmployeeId: trip + "000000009631",
            evidence: .reading(meterId: trip + "000000009651", segmentId: trip + "000000009652", kind: .odometer,
                               readingId: trip + "000000009653", value: 1_000_000_000_000, modulus: nil),
            departureRunId: nil
        )
        XCTAssertEqual(text(operation), Vectors.start_utc_zone_zero_offset)
    }

    func testTripFinishVectorsDependOnTheStart() throws {
        let hourMeter = try EnterpriseOperationBuilder.finishTrip(
            envelope: envelope(prefix: trip, operation: trip + "000000009603"),
            tripId: trip + "000000009611",
            evidence: .reading(meterId: trip + "000000009655", segmentId: trip + "000000009656", kind: .hourMeter,
                               readingId: trip + "000000009658", value: 7200, modulus: nil),
            startOperationId: trip + "000000009601"
        )
        XCTAssertEqual(text(hourMeter), Vectors.finish_hour_meter_without_clock)
        let absent = try EnterpriseOperationBuilder.finishTrip(
            envelope: envelope(prefix: trip, operation: trip + "000000009603"),
            tripId: trip + "000000009611",
            evidence: .missing(reason: "Sem leitura final"),
            startOperationId: trip + "000000009601"
        )
        XCTAssertEqual(text(absent), Vectors.finish_absent_reading)
        XCTAssertEqual(absent.payloadDigest, "sha256:6d394bd68eb9dc42b51598fe542d3941e4ecb5a0c2386ed2c4b40898f392c711")
    }

    func testTripEvidenceRefusesTheSharedMalformedForms() throws {
        let base = try envelope(prefix: trip, operation: trip + "000000009601")
        func start(_ evidence: EnterpriseMeterEvidence, driver: String = "7b0e2a54-0000-4000-8000-000000009631") throws {
            _ = try EnterpriseOperationBuilder.startTrip(envelope: base, tripId: trip + "000000009611", assetId: trip + "000000009621",
                                                         driverEmployeeId: driver, evidence: evidence, departureRunId: nil)
        }
        let meter = trip + "000000009651"
        let segment = trip + "000000009652"
        let reading = trip + "000000009653"
        XCTAssertThrowsError(try start(.missing(reason: "   ")))
        XCTAssertThrowsError(try start(.missing(reason: "")))
        XCTAssertThrowsError(try start(.missing(reason: String(repeating: "é", count: 281))))
        XCTAssertNoThrow(try start(.missing(reason: String(repeating: "é", count: 280))))
        XCTAssertThrowsError(try start(.missing(reason: "linha\nquebrada")))
        XCTAssertThrowsError(try start(.reading(meterId: meter, segmentId: segment, kind: .odometer, readingId: reading,
                                                value: 1_000_000_000_001, modulus: nil)))
        XCTAssertThrowsError(try start(.reading(meterId: meter, segmentId: segment, kind: .hourMeter, readingId: reading,
                                                value: 3_600_000_001, modulus: nil)))
        XCTAssertThrowsError(try start(.reading(meterId: meter, segmentId: segment, kind: .odometer, readingId: reading,
                                                value: -5, modulus: nil)))
        XCTAssertThrowsError(try start(.reading(meterId: meter.uppercased(), segmentId: segment, kind: .odometer, readingId: reading,
                                                value: 10, modulus: nil)))
        XCTAssertThrowsError(try start(.reading(meterId: meter, segmentId: segment, kind: .odometer, readingId: reading,
                                                value: 100_000, modulus: 100_000)))
        XCTAssertThrowsError(try start(.missing(reason: "x"), driver: EnterpriseValues.nilId))
    }

    // MARK: - Expenses and checklists

    func testExpenseVectorsAndRefusals() throws {
        let toll = try EnterpriseOperationBuilder.submitExpense(
            envelope: envelope(prefix: expense, operation: expense + "000000009701"), expenseId: expense + "000000009711",
            assetId: expense + "000000009721", category: "TOLL", paidCentavos: 1250, companyCentavos: 1250, payer: .company
        )
        XCTAssertEqual(text(toll), Vectors.toll_company_paid)
        let parking = try EnterpriseOperationBuilder.submitExpense(
            envelope: envelope(prefix: expense, operation: expense + "000000009701"), expenseId: expense + "000000009711",
            assetId: expense + "000000009721", category: "PARKING", paidCentavos: 2000, companyCentavos: 2000, payer: .employee
        )
        XCTAssertEqual(text(parking), Vectors.parking_employee_paid)
        let base = try envelope(prefix: expense, operation: expense + "000000009701")
        XCTAssertThrowsError(try EnterpriseOperationBuilder.submitExpense(
            envelope: base, expenseId: expense + "000000009711", assetId: expense + "000000009721", category: "TOLL",
            paidCentavos: 1000, companyCentavos: 1001, payer: .company
        )) { XCTAssertEqual($0 as? EnterpriseOperationError, .malformed) }
        XCTAssertThrowsError(try EnterpriseOperationBuilder.submitExpense(
            envelope: base, expenseId: expense + "000000009711", assetId: expense + "000000009721", category: "FUEL",
            paidCentavos: 1000, companyCentavos: 1000, payer: .company
        )) { XCTAssertEqual($0 as? EnterpriseOperationError, .unsupported) }
    }

    func testChecklistRunVectors() throws {
        let base = try envelope(prefix: trip, operation: trip + "000000009701")
        func run(_ responses: [EnterpriseOperationBuilder.ChecklistResponse], outcome: EnterpriseChecklistOutcome,
                 acknowledged: Bool) throws -> String {
            text(try EnterpriseOperationBuilder.completeChecklist(
                envelope: base, runId: trip + "000000009711", assetId: trip + "000000009721", context: "DEPARTURE",
                templateId: trip + "000000009731", templateVersion: 1, policyRevision: 1, startedAt: "2026-11-15T11:40:00Z",
                responses: responses, outcome: outcome, acknowledged: acknowledged
            ))
        }
        let ok = EnterpriseOperationBuilder.ChecklistResponse(answer: .ok)
        XCTAssertEqual(try run([ok, ok, ok], outcome: .completedOk, acknowledged: false), Vectors.run_departure_all_ok)
        XCTAssertEqual(try run([ok, .init(answer: .notOk, note: "Freios rangendo"), ok], outcome: .blocking, acknowledged: false),
                       Vectors.run_blocking_critical_with_note)
        XCTAssertEqual(try run([ok, .init(answer: .notOk), .init(answer: .notOk)], outcome: .completedWithIssues, acknowledged: true),
                       Vectors.run_warn_acknowledged)
    }

    func testChecklistEvaluationFollowsTheAndroidRules() throws {
        let items = [
            EnterpriseChecklistItem(position: 1, label: "Pneus", critical: false, optional: true, photoOnNotOk: false),
            EnterpriseChecklistItem(position: 2, label: "Freios", critical: true, optional: false, photoOnNotOk: false),
            EnterpriseChecklistItem(position: 3, label: "Luzes", critical: false, optional: false, photoOnNotOk: true),
        ]
        let blocking = try EnterpriseChecklistRules.evaluate(items: items, policyMode: "BLOCK", answers: [.ok, .notOk, .ok])
        XCTAssertEqual(blocking.outcome, .blocking)
        XCTAssertFalse(blocking.acknowledgementRequired)
        let warned = try EnterpriseChecklistRules.evaluate(items: items, policyMode: "WARN", answers: [.notApplicable, .notOk, .ok])
        XCTAssertEqual(warned.outcome, .completedWithIssues)
        XCTAssertTrue(warned.acknowledgementRequired)
        XCTAssertEqual(warned.critical, [2])
        XCTAssertEqual(try EnterpriseChecklistRules.evaluate(items: items, policyMode: "BLOCK", answers: [.ok, .ok, .ok]).outcome, .completedOk)
        XCTAssertThrowsError(try EnterpriseChecklistRules.evaluate(items: items, policyMode: "BLOCK", answers: [.ok, .notApplicable, .ok])) {
            XCTAssertEqual($0 as? EnterpriseChecklistFailure, .answerNotAllowed)
        }
        XCTAssertThrowsError(try EnterpriseChecklistRules.evaluate(items: items, policyMode: "BLOCK", answers: [.ok, .ok, .notOk])) {
            XCTAssertEqual($0 as? EnterpriseChecklistFailure, .photoRequired)
        }
        XCTAssertThrowsError(try EnterpriseChecklistRules.evaluate(items: items, policyMode: "BLOCK", answers: [.ok, nil, .ok])) {
            XCTAssertEqual($0 as? EnterpriseChecklistFailure, .incomplete)
        }
    }

    // MARK: - Delivery wire

    struct AckVector {
        let id: String
        let expected: String
        let raw: String
    }

    static let ackOperation = EnterpriseSealedOperation(
        operationId: "7b0e2a54-0000-4000-8000-000000000191", type: "trip.start", entityType: "trip",
        entityId: "7b0e2a54-0000-4000-8000-0000000001f5",
        payloadDigest: "sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02", canonical: Data()
    )

    func testAcknowledgementVectorsBindToTheSentOperation() throws {
        for vector in Self.ackVectors {
            let body = Data((#"{"schema_version":1,"acknowledgements":["# + vector.raw + "]}").utf8)
            switch vector.expected {
            case "BINDING", "DIGEST", "INVALID":
                XCTAssertThrowsError(try EnterpriseSyncWire.parseBatchResponse(body, expected: [Self.ackOperation]), vector.id) { error in
                    let expected: EnterpriseProtocolFailure = vector.expected == "BINDING" ? .binding : (vector.expected == "DIGEST" ? .digest : .invalid)
                    XCTAssertEqual(error as? EnterpriseProtocolFailure, expected, vector.id)
                }
            default:
                let acks = try EnterpriseSyncWire.parseBatchResponse(body, expected: [Self.ackOperation])
                XCTAssertEqual(acks.first?.result.rawValue, vector.expected, vector.id)
            }
        }
        let empty = Data(#"{"schema_version":1,"acknowledgements":[]}"#.utf8)
        XCTAssertThrowsError(try EnterpriseSyncWire.parseBatchResponse(empty, expected: [Self.ackOperation])) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .binding)
        }
    }

    func testBatchBodyCarriesTheProofOutsideTheSealedOperations() throws {
        let operation = try EnterpriseOperationBuilder.submitExpense(
            envelope: envelope(prefix: expense, operation: expense + "000000009701"), expenseId: expense + "000000009711",
            assetId: expense + "000000009721", category: "TOLL", paidCentavos: 1250, companyCentavos: 1250, payer: .company
        )
        let body = try EnterpriseSyncWire.batchBody(familyId: expense + "000000009601", accessToken: "synthetic-access", operations: [operation])
        let text = String(decoding: body, as: UTF8.self)
        XCTAssertTrue(text.hasPrefix(#"{"access_token":"synthetic-access","family_id":"9e5b0000-0000-4000-8000-000000009601","operations":[{"api_origin""#))
        XCTAssertTrue(text.hasSuffix(#""type":"expense.submit"}],"schema_version":1}"#))
        XCTAssertTrue(text.contains(Vectors.toll_company_paid))
        XCTAssertThrowsError(try EnterpriseSyncWire.batchBody(familyId: expense + "000000009601", accessToken: "token with space", operations: [operation]))
        XCTAssertThrowsError(try EnterpriseSyncWire.batchBody(familyId: expense + "000000009601", accessToken: "t", operations: [operation, operation]))
        XCTAssertThrowsError(try EnterpriseSyncWire.batchBody(familyId: expense + "000000009601", accessToken: "t", operations: []))
    }

    // MARK: - Values

    func testValuesFollowTheEnterpriseProfile() {
        XCTAssertEqual(EnterpriseValues.formatInstant(epochMillis: 1_763_208_000_123), "2025-11-15T12:00:00.123Z")
        XCTAssertEqual(EnterpriseValues.formatInstant(epochMillis: 1_763_208_000_100), "2025-11-15T12:00:00.1Z")
        XCTAssertEqual(EnterpriseValues.formatInstant(epochMillis: 1_763_208_000_000), "2025-11-15T12:00:00Z")
        XCTAssertEqual(EnterpriseValues.parseInstantMillis("2025-11-15T12:00:00.123456789Z"), 1_763_208_000_123)
        XCTAssertNil(EnterpriseValues.parseInstantMillis("2025-11-15T12:00:00.120Z"))
        XCTAssertNil(EnterpriseValues.parseInstantMillis("2025-11-15T08:00:00-04:00"))
        XCTAssertNil(EnterpriseValues.parseInstantMillis("2025-02-29T00:00:00Z"))
        XCTAssertEqual(EnterpriseValues.parseKilometres("184320,5"), 184_320_500)
        XCTAssertEqual(EnterpriseValues.parseKilometres("12.345"), 12_345)
        XCTAssertNil(EnterpriseValues.parseKilometres("12,3456"))
        XCTAssertNil(EnterpriseValues.parseKilometres("-1"))
        XCTAssertNil(EnterpriseValues.parseKilometres("1.000,5"))
        XCTAssertEqual(EnterpriseValues.parseHours("3600,25"), 12_960_900)
        XCTAssertNil(EnterpriseValues.parseHours("1,234"))
        XCTAssertTrue(EnterpriseValues.isCanonicalOrigin("https://abastevo.com.br"))
        XCTAssertFalse(EnterpriseValues.isCanonicalOrigin("https://abastevo.com.br/"))
        XCTAssertFalse(EnterpriseValues.isCanonicalOrigin("https://Abastevo.com.br"))
        XCTAssertFalse(EnterpriseValues.isCanonicalOrigin("http://abastevo.com.br"))
        XCTAssertFalse(EnterpriseValues.isCanonicalOrigin("https://abastevo.com.br:443"))
        XCTAssertFalse(EnterpriseValues.isId(EnterpriseValues.nilId))
        XCTAssertFalse(EnterpriseValues.isId("7B0E2A54-0000-4000-8000-000000009651"))
        XCTAssertTrue(EnterpriseTimeDeclaration.isDeclarableZone("America/Sao_Paulo"))
        XCTAssertFalse(EnterpriseTimeDeclaration.isDeclarableZone("GMT"))
        XCTAssertFalse(EnterpriseTimeDeclaration.isDeclarableZone("posix/America/Sao_Paulo"))
        let declared = EnterpriseTimeDeclaration.at(epochMillis: 1_763_208_000_123, zone: TimeZone(identifier: "America/Cuiaba") ?? .current)
        XCTAssertEqual(declared.occurredAt, "2025-11-15T12:00:00.123Z")
        XCTAssertEqual(declared.entryZone, "America/Cuiaba")
        XCTAssertEqual(declared.deviceOffsetSeconds, -14_400)
    }

    // MARK: - Vectors

    enum Vectors {
        static let start_reading_without_run_clock = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":-14400,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009611","type":"trip"},"entry_zone":"America/Cuiaba","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"7b0e2a54-0000-4000-8000-000000009601","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"asset_id":"7b0e2a54-0000-4000-8000-000000009621","driver_employee_id":"7b0e2a54-0000-4000-8000-000000009631","missing_reading_reason":null,"reading":{"kind":"ODOMETER","meter_id":"7b0e2a54-0000-4000-8000-000000009651","reading_id":"7b0e2a54-0000-4000-8000-000000009653","segment_id":"7b0e2a54-0000-4000-8000-000000009652","value":"0"},"run_clock":null},"payload_digest":"sha256:01c431e4f5b757896fabecf24e9f2d8c70f2a20f96e868e79dc4768011363754","schema_version":1,"soft_references":[],"type":"trip.start"}"#
        static let start_utc_zone_zero_offset = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":0,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009611","type":"trip"},"entry_zone":"UTC","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T00:00:00Z","operation_id":"7b0e2a54-0000-4000-8000-000000009601","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"asset_id":"7b0e2a54-0000-4000-8000-000000009621","driver_employee_id":"7b0e2a54-0000-4000-8000-000000009631","missing_reading_reason":null,"reading":{"kind":"ODOMETER","meter_id":"7b0e2a54-0000-4000-8000-000000009651","reading_id":"7b0e2a54-0000-4000-8000-000000009653","segment_id":"7b0e2a54-0000-4000-8000-000000009652","value":"1000000000000"},"run_clock":null},"payload_digest":"sha256:90ac9897f104ca28d173b4f2bd3688a68a809c9136da1943bd44e09b1caf5d73","schema_version":1,"soft_references":[],"type":"trip.start"}"#
        static let finish_absent_reading = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":["7b0e2a54-0000-4000-8000-000000009601"],"device_offset_seconds":-14400,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009611","type":"trip"},"entry_zone":"America/Cuiaba","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"7b0e2a54-0000-4000-8000-000000009603","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"missing_reading_reason":"Sem leitura final","reading":null,"run_clock":null},"payload_digest":"sha256:6d394bd68eb9dc42b51598fe542d3941e4ecb5a0c2386ed2c4b40898f392c711","schema_version":1,"soft_references":[],"type":"trip.finish"}"#
        static let finish_hour_meter_without_clock = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":["7b0e2a54-0000-4000-8000-000000009601"],"device_offset_seconds":-14400,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009611","type":"trip"},"entry_zone":"America/Cuiaba","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"7b0e2a54-0000-4000-8000-000000009603","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"missing_reading_reason":null,"reading":{"kind":"HOUR_METER","meter_id":"7b0e2a54-0000-4000-8000-000000009655","reading_id":"7b0e2a54-0000-4000-8000-000000009658","segment_id":"7b0e2a54-0000-4000-8000-000000009656","value":"7200"},"run_clock":null},"payload_digest":"sha256:0efc67fc0fc06ad52d60517efc9907474ac213364918ff81df99ba03f32a73b7","schema_version":1,"soft_references":[],"type":"trip.finish"}"#
        static let toll_company_paid = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":-14400,"enrollment_id":"9e5b0000-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"9e5b0000-0000-4000-8000-000000009711","type":"expense"},"entry_zone":"America/Cuiaba","membership_id":"9e5b0000-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"9e5b0000-0000-4000-8000-000000009701","operation_schema":1,"org_id":"9e5b0000-0000-4000-8000-000000009502","owner_account_id":"9e5b0000-0000-4000-8000-000000009501","payload":{"asset_id":"9e5b0000-0000-4000-8000-000000009721","category":"TOLL","company_amount":"1250","paid_total":"1250","payer":"COMPANY","trip_id":null},"payload_digest":"sha256:a63e8ea6fbd9a0c7e05d11fc91a1053f9cbdbfe833ec5c00f3d6f35cd0981619","schema_version":1,"soft_references":[],"type":"expense.submit"}"#
        static let parking_employee_paid = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":-14400,"enrollment_id":"9e5b0000-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"9e5b0000-0000-4000-8000-000000009711","type":"expense"},"entry_zone":"America/Cuiaba","membership_id":"9e5b0000-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"9e5b0000-0000-4000-8000-000000009701","operation_schema":1,"org_id":"9e5b0000-0000-4000-8000-000000009502","owner_account_id":"9e5b0000-0000-4000-8000-000000009501","payload":{"asset_id":"9e5b0000-0000-4000-8000-000000009721","category":"PARKING","company_amount":"2000","paid_total":"2000","payer":"EMPLOYEE","trip_id":null},"payload_digest":"sha256:483c2ee8d527cc26aab047cabc2b0ea62a947a3bfee14dc4471fa35d409a927a","schema_version":1,"soft_references":[],"type":"expense.submit"}"#
        static let run_departure_all_ok = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":-14400,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009711","type":"checklist_run"},"entry_zone":"America/Cuiaba","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"7b0e2a54-0000-4000-8000-000000009701","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"acknowledged":false,"answers":[{"answer":"OK","note":null,"photo":false},{"answer":"OK","note":null,"photo":false},{"answer":"OK","note":null,"photo":false}],"asset_id":"7b0e2a54-0000-4000-8000-000000009721","context":"DEPARTURE","device_outcome":"COMPLETED_OK","policy_revision":1,"started_at":"2026-11-15T11:40:00Z","template_id":"7b0e2a54-0000-4000-8000-000000009731","template_version":1},"payload_digest":"sha256:ef36f2c223b1ef8c0a6b6e62a3a1a0ca5b13691827badac4ca86b42b715b371d","schema_version":1,"soft_references":[],"type":"check.run.complete"}"#
        static let run_blocking_critical_with_note = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":-14400,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009711","type":"checklist_run"},"entry_zone":"America/Cuiaba","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"7b0e2a54-0000-4000-8000-000000009701","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"acknowledged":false,"answers":[{"answer":"OK","note":null,"photo":false},{"answer":"NOT_OK","note":"Freios rangendo","photo":false},{"answer":"OK","note":null,"photo":false}],"asset_id":"7b0e2a54-0000-4000-8000-000000009721","context":"DEPARTURE","device_outcome":"BLOCKING","policy_revision":1,"started_at":"2026-11-15T11:40:00Z","template_id":"7b0e2a54-0000-4000-8000-000000009731","template_version":1},"payload_digest":"sha256:9c91b84b131d61d2c2e66fbdd0487ed091e348709d2889f87671c01d142fc01a","schema_version":1,"soft_references":[],"type":"check.run.complete"}"#
        static let run_warn_acknowledged = #"{"api_origin":"https://api.example.invalid","attachments":[],"depends_on":[],"device_offset_seconds":-14400,"enrollment_id":"7b0e2a54-0000-4000-8000-000000009504","entity":{"expected_revision":null,"id":"7b0e2a54-0000-4000-8000-000000009711","type":"checklist_run"},"entry_zone":"America/Cuiaba","membership_id":"7b0e2a54-0000-4000-8000-000000009503","occurred_at":"2026-11-15T12:00:00.123Z","operation_id":"7b0e2a54-0000-4000-8000-000000009701","operation_schema":1,"org_id":"7b0e2a54-0000-4000-8000-000000009502","owner_account_id":"7b0e2a54-0000-4000-8000-000000009501","payload":{"acknowledged":true,"answers":[{"answer":"OK","note":null,"photo":false},{"answer":"NOT_OK","note":null,"photo":false},{"answer":"NOT_OK","note":null,"photo":false}],"asset_id":"7b0e2a54-0000-4000-8000-000000009721","context":"DEPARTURE","device_outcome":"COMPLETED_WITH_ISSUES","policy_revision":1,"started_at":"2026-11-15T11:40:00Z","template_id":"7b0e2a54-0000-4000-8000-000000009731","template_version":1},"payload_digest":"sha256:24f0f102daa98fa495882f21ee2970de6094de17205e7f346f9e0481517fee71","schema_version":1,"soft_references":[],"type":"check.run.complete"}"#
    }

    static let ackVectors: [AckVector] = [
        AckVector(id: "ack-accepted", expected: "ACCEPTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-clock-warning", expected: "ACCEPTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":["CLOCK_SKEW"]}"#),
        AckVector(id: "ack-revision-max", expected: "ACCEPTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":9223372036854775807,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-review-orphan_reference", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"ORPHAN_REFERENCE","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-access_ended_before_receipt", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"ACCESS_ENDED_BEFORE_RECEIPT","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-permission_changed", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"PERMISSION_CHANGED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-asset_unavailable", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"ASSET_UNAVAILABLE","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-asset_busy", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"ASSET_BUSY","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-trip_already_closed", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"TRIP_ALREADY_CLOSED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-period_locked", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"PERIOD_LOCKED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-stale_revision", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"STALE_REVISION","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-org_suspended_grace", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"ORG_SUSPENDED_GRACE","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-review-plan_changed", expected: "ACCEPTED_FOR_REVIEW", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"PLAN_CHANGED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-rejected-operation_id_reused", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"OPERATION_ID_REUSED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-rejected-malformed", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"MALFORMED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-rejected-device_revoked", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"DEVICE_REVOKED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-rejected-membership_ended", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"MEMBERSHIP_ENDED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-rejected-intake_window_closed", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"INTAKE_WINDOW_CLOSED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-rejected-forbidden", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"FORBIDDEN","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-rejected-org_suspended", expected: "REJECTED", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"ORG_SUSPENDED","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-retry-dependency_pending", expected: "RETRY", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"DEPENDENCY_PENDING","entity_revision":null,"received_at":null,"review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-retry-unavailable", expected: "RETRY", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"UNAVAILABLE","entity_revision":null,"received_at":null,"review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-wrong-id", expected: "BINDING", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000192","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-wrong-digest", expected: "DIGEST", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-unknown-result", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"PENDING","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-accepted-code", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":"MALFORMED","entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-accepted-no-revision", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-accepted-review", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-invalid-accepted-no-receipt", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":null,"review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-zero-receipt", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"0001-01-01T00:00:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-noncanonical-receipt", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T08:01:00-04:00","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-revision-zero", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":0,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-revision-overflow", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":9223372036854775808,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-unknown-warning", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":["UNKNOWN"]}"#),
        AckVector(id: "ack-invalid-duplicate-warning", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":["CLOCK_SKEW","CLOCK_SKEW"]}"#),
        AckVector(id: "ack-invalid-wire-final", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[],"final":true}"#),
        AckVector(id: "ack-invalid-review-no-id", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"ASSET_BUSY","entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-review-rejected-code", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED_FOR_REVIEW","code":"MALFORMED","entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-invalid-rejected-revision", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"MALFORMED","entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-retry-final-receipt", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"UNAVAILABLE","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-retry-revision", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"UNAVAILABLE","entity_revision":1,"received_at":null,"review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-retry-review", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"UNAVAILABLE","entity_revision":null,"received_at":null,"review_item_id":"7b0e2a54-0000-4000-8000-000000000385","warnings":[]}"#),
        AckVector(id: "ack-invalid-retry-business-code", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"ASSET_BUSY","entity_revision":null,"received_at":null,"review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-accepted-empty-code", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":"","entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-accepted-empty-review", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"ACCEPTED","code":null,"entity_revision":1,"received_at":"2026-11-13T12:01:00Z","review_item_id":"","warnings":[]}"#),
        AckVector(id: "ack-invalid-rejected-empty-review", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"REJECTED","code":"FORBIDDEN","entity_revision":null,"received_at":"2026-11-13T12:01:00Z","review_item_id":"","warnings":[]}"#),
        AckVector(id: "ack-invalid-retry-empty-receipt", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"UNAVAILABLE","entity_revision":null,"received_at":"","review_item_id":null,"warnings":[]}"#),
        AckVector(id: "ack-invalid-retry-empty-review", expected: "INVALID", raw: #"{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000000191","payload_digest":"sha256:869182e24eb731898fd9d615d51db8144aac1081a3ff0a6238a87bec9190ff02","result":"RETRY","code":"DEPENDENCY_PENDING","entity_revision":null,"received_at":null,"review_item_id":"","warnings":[]}"#),
    ]
}
