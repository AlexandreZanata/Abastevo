import Foundation

/// One organization partition of private company data: account, canonical origin and organization.
///
/// Every queued command, cache and receipt belongs to exactly one partition
/// (ADR-025). Values alone never prove live server authority.
public struct EnterprisePartition: Hashable, Codable, Sendable {
    public let accountId: String
    public let origin: String
    public let organizationId: String

    public init?(accountId: String, origin: String, organizationId: String) {
        guard EnterpriseValues.isId(accountId), EnterpriseValues.isCanonicalOrigin(origin),
              EnterpriseValues.isId(organizationId) else {
            return nil
        }
        self.accountId = accountId
        self.origin = origin
        self.organizationId = organizationId
    }

    /// Opaque file name: no identifier appears on disk.
    public var storageKey: String {
        return EnterpriseCanonicalJson.sha256Hex(Data((accountId + "|" + origin + "|" + organizationId).utf8))
    }
}

/// The envelope facts of one device-produced operation, bound to its creation partition.
public struct EnterpriseEnvelope: Sendable {
    public let partition: EnterprisePartition
    public let membershipId: String
    public let enrollmentId: String
    public let operationId: String
    public let declared: EnterpriseTimeDeclaration

    public init(partition: EnterprisePartition, membershipId: String, enrollmentId: String, operationId: String, declared: EnterpriseTimeDeclaration) {
        self.partition = partition
        self.membershipId = membershipId
        self.enrollmentId = enrollmentId
        self.operationId = operationId
        self.declared = declared
    }
}

/// A sealed schema 1 operation: its canonical bytes include the payload digest and are sent verbatim.
public struct EnterpriseSealedOperation: Codable, Equatable, Sendable {
    public let operationId: String
    public let type: String
    public let entityType: String
    public let entityId: String
    public let payloadDigest: String
    public let canonical: Data

    public init(operationId: String, type: String, entityType: String, entityId: String, payloadDigest: String, canonical: Data) {
        self.operationId = operationId
        self.type = type
        self.entityType = entityType
        self.entityId = entityId
        self.payloadDigest = payloadDigest
        self.canonical = canonical
    }
}

public enum EnterpriseMeterKind: String, Codable, Sendable, CaseIterable {
    case odometer = "ODOMETER"
    case hourMeter = "HOUR_METER"

    public var maximum: Int64 {
        switch self {
        case .odometer: return EnterpriseValues.maxOdometerMetres
        case .hourMeter: return EnterpriseValues.maxHourMeterSeconds
        }
    }
}

/// Departure or final evidence: exactly one reading of a known meter or a written reason.
public enum EnterpriseMeterEvidence: Equatable, Sendable {
    case reading(meterId: String, segmentId: String, kind: EnterpriseMeterKind, readingId: String, value: Int64, modulus: Int64?)
    case missing(reason: String)
}

public enum EnterpriseExpensePayer: String, Codable, Sendable, CaseIterable {
    case company = "COMPANY"
    case employee = "EMPLOYEE"
}

public enum EnterpriseChecklistAnswer: String, Codable, Sendable, CaseIterable {
    case ok = "OK"
    case notOk = "NOT_OK"
    case notApplicable = "NOT_APPLICABLE"
}

public enum EnterpriseChecklistOutcome: String, Codable, Sendable {
    case completedOk = "COMPLETED_OK"
    case completedWithIssues = "COMPLETED_WITH_ISSUES"
    case blocking = "BLOCKING"
}

public enum EnterpriseOperationError: Error, Equatable, Sendable {
    case malformed
    case unsupported
}

/// Builders of the exact operation shapes pinned by `contracts/testdata/enterprise`
/// (trip-operations-v3, expense-submit-v1, checklist run-v1, operations-v1).
/// Zero postfix unwrap operators anywhere in this file.
public enum EnterpriseOperationBuilder {
    public static let tripStart = "trip.start"
    public static let tripFinish = "trip.finish"
    public static let expenseSubmit = "expense.submit"
    public static let checklistComplete = "check.run.complete"

    /// Android `ExpenseOperations.DIRECT_CATEGORIES`; FUEL is a fueling, never a direct expense.
    public static let expenseCategories = ["FINE", "INSURANCE", "LEASE", "MAINTENANCE", "OTHER", "PARKING", "TOLL"]

    /// Seals one operation: the digest covers the canonical object without `payload_digest`.
    public static func seal(
        envelope: EnterpriseEnvelope,
        type: String,
        entityType: String,
        entityId: String,
        expectedRevision: Int64?,
        dependsOn: [String],
        softReferences: [(type: String, id: String)],
        payload: [String: EnterpriseJson]
    ) throws -> EnterpriseSealedOperation {
        let partition = envelope.partition
        guard EnterpriseValues.isId(envelope.operationId), EnterpriseValues.isId(envelope.membershipId),
              EnterpriseValues.isId(envelope.enrollmentId), EnterpriseValues.isId(entityId),
              EnterpriseCanonicalJson.isSegmented(entityType), isOperationType(type), envelope.declared.isValid else {
            throw EnterpriseOperationError.malformed
        }
        if let revision = expectedRevision, revision < 1 { throw EnterpriseOperationError.malformed }
        let dependencies = dependsOn.sorted()
        guard dependencies.allSatisfy(EnterpriseValues.isId), Set(dependencies).count == dependencies.count,
              !dependencies.contains(envelope.operationId) else {
            throw EnterpriseOperationError.malformed
        }
        let references = softReferences.sorted { lhs, rhs in
            lhs.type == rhs.type ? lhs.id < rhs.id : lhs.type < rhs.type
        }
        guard references.allSatisfy({ EnterpriseCanonicalJson.isSegmented($0.type) && EnterpriseValues.isId($0.id) }),
              Set(references.map { $0.type + "|" + $0.id }).count == references.count else {
            throw EnterpriseOperationError.malformed
        }
        var root: [String: EnterpriseJson] = [
            "schema_version": .int(1),
            "operation_id": .text(envelope.operationId),
            "owner_account_id": .text(partition.accountId),
            "api_origin": .text(partition.origin),
            "org_id": .text(partition.organizationId),
            "membership_id": .text(envelope.membershipId),
            "enrollment_id": .text(envelope.enrollmentId),
            "type": .text(type),
            "operation_schema": .int(1),
            "entity": .object([
                "type": .text(entityType),
                "id": .text(entityId),
                "expected_revision": expectedRevision.map { EnterpriseJson.int($0) } ?? .null,
            ]),
            "occurred_at": .text(envelope.declared.occurredAt),
            "entry_zone": .text(envelope.declared.entryZone),
            "device_offset_seconds": .int(envelope.declared.deviceOffsetSeconds),
            "depends_on": .array(dependencies.map { EnterpriseJson.text($0) }),
            "soft_references": .array(references.map { EnterpriseJson.object(["type": .text($0.type), "id": .text($0.id)]) }),
            "attachments": .array([]),
            "payload": .object(payload),
        ]
        let unsealed = try encodeOperation(.object(root))
        let digest = "sha256:" + EnterpriseCanonicalJson.sha256Hex(unsealed)
        root["payload_digest"] = .text(digest)
        let canonical = try encodeOperation(.object(root))
        guard canonical.count <= EnterpriseCanonicalJson.maxOperationBytes else { throw EnterpriseOperationError.malformed }
        return EnterpriseSealedOperation(
            operationId: envelope.operationId,
            type: type,
            entityType: entityType,
            entityId: entityId,
            payloadDigest: digest,
            canonical: canonical
        )
    }

    private static func encodeOperation(_ value: EnterpriseJson) throws -> Data {
        do {
            return try EnterpriseCanonicalJson.encode(value, maxDepth: EnterpriseCanonicalJson.maxOperationDepth)
        } catch {
            throw EnterpriseOperationError.malformed
        }
    }

    /// Two or more segmented names joined by dots.
    public static func isOperationType(_ value: String) -> Bool {
        let segments = value.split(separator: ".", omittingEmptySubsequences: false)
        return segments.count >= 2 && segments.allSatisfy { EnterpriseCanonicalJson.isSegmented(String($0)) }
    }

    // MARK: - Meter evidence

    static func evidencePayload(_ evidence: EnterpriseMeterEvidence) throws -> (reading: EnterpriseJson, reason: EnterpriseJson) {
        switch evidence {
        case .reading(let meterId, let segmentId, let kind, let readingId, let value, let modulus):
            guard EnterpriseValues.isId(meterId), EnterpriseValues.isId(segmentId), EnterpriseValues.isId(readingId),
                  value >= 0, value <= kind.maximum else {
                throw EnterpriseOperationError.malformed
            }
            if let modulus, modulus <= 0 || value >= modulus { throw EnterpriseOperationError.malformed }
            let reading: EnterpriseJson = .object([
                "kind": .text(kind.rawValue),
                "meter_id": .text(meterId),
                "reading_id": .text(readingId),
                "segment_id": .text(segmentId),
                "value": .text(String(value)),
            ])
            return (reading, .null)
        case .missing(let reason):
            guard EnterpriseValues.isValidText(reason, maxScalars: EnterpriseValues.maxReasonScalars) else {
                throw EnterpriseOperationError.malformed
            }
            return (.null, .text(reason))
        }
    }

    // MARK: - Trips

    /// `trip.start`: the driver's own start of an assigned asset with honest departure evidence.
    /// iOS records no run clock (`run_clock` null) and no tracking.
    public static func startTrip(
        envelope: EnterpriseEnvelope,
        tripId: String,
        assetId: String,
        driverEmployeeId: String,
        evidence: EnterpriseMeterEvidence,
        departureRunId: String?,
        missionId: String? = nil
    ) throws -> EnterpriseSealedOperation {
        guard EnterpriseValues.isId(assetId), EnterpriseValues.isId(driverEmployeeId) else {
            throw EnterpriseOperationError.malformed
        }
        let parts = try evidencePayload(evidence)
        var references: [(type: String, id: String)] = []
        if let run = departureRunId {
            guard EnterpriseValues.isId(run) else { throw EnterpriseOperationError.malformed }
            references.append((type: "checklist_run", id: run))
        }
        if let missionId {
            guard EnterpriseValues.isId(missionId) else { throw EnterpriseOperationError.malformed }
            references.append((type: "mission", id: missionId))
        }
        return try seal(
            envelope: envelope,
            type: tripStart,
            entityType: "trip",
            entityId: tripId,
            expectedRevision: nil,
            dependsOn: [],
            softReferences: references,
            payload: [
                "asset_id": .text(assetId),
                "driver_employee_id": .text(driverEmployeeId),
                "missing_reading_reason": parts.reason,
                "reading": parts.reading,
                "run_clock": .null,
            ]
        )
    }

    /// `trip.finish`: final evidence; it depends on the start while the start is not yet acknowledged.
    public static func finishTrip(
        envelope: EnterpriseEnvelope,
        tripId: String,
        evidence: EnterpriseMeterEvidence,
        startOperationId: String?,
        returnRunId: String? = nil
    ) throws -> EnterpriseSealedOperation {
        let parts = try evidencePayload(evidence)
        return try seal(
            envelope: envelope,
            type: tripFinish,
            entityType: "trip",
            entityId: tripId,
            expectedRevision: nil,
            dependsOn: startOperationId.map { [$0] } ?? [],
            softReferences: returnRunId.map { [(type: "checklist_run", id: $0)] } ?? [],
            payload: [
                "missing_reading_reason": parts.reason,
                "reading": parts.reading,
                "run_clock": .null,
            ]
        )
    }

    // MARK: - Expenses

    /// `expense.submit` (schema 1): a direct v1 category, exact centavos, the payer and the asset; no trip link.
    public static func submitExpense(
        envelope: EnterpriseEnvelope,
        expenseId: String,
        assetId: String,
        category: String,
        paidCentavos: Int64,
        companyCentavos: Int64,
        payer: EnterpriseExpensePayer
    ) throws -> EnterpriseSealedOperation {
        guard EnterpriseValues.isId(assetId), paidCentavos >= 0, paidCentavos <= EnterpriseValues.maxCentavos,
              companyCentavos >= 0, companyCentavos <= paidCentavos else {
            throw EnterpriseOperationError.malformed
        }
        guard expenseCategories.contains(category) else { throw EnterpriseOperationError.unsupported }
        return try seal(
            envelope: envelope,
            type: expenseSubmit,
            entityType: "expense",
            entityId: expenseId,
            expectedRevision: nil,
            dependsOn: [],
            softReferences: [],
            payload: [
                "asset_id": .text(assetId),
                "category": .text(category),
                "company_amount": .text(String(companyCentavos)),
                "paid_total": .text(String(paidCentavos)),
                "payer": .text(payer.rawValue),
                "trip_id": .null,
            ]
        )
    }

    // MARK: - Checklists

    /// One answered item; photos are not declared from iOS.
    public struct ChecklistResponse: Equatable, Sendable {
        public let answer: EnterpriseChecklistAnswer
        public let note: String

        public init(answer: EnterpriseChecklistAnswer, note: String = "") {
            self.answer = answer
            self.note = note
        }
    }

    /// `check.run.complete` (schema 1). The device outcome is only a claim; the server rechecks it.
    public static func completeChecklist(
        envelope: EnterpriseEnvelope,
        runId: String,
        assetId: String,
        context: String,
        templateId: String,
        templateVersion: Int64,
        policyRevision: Int64,
        startedAt: String,
        responses: [ChecklistResponse],
        outcome: EnterpriseChecklistOutcome,
        acknowledged: Bool
    ) throws -> EnterpriseSealedOperation {
        guard EnterpriseValues.isId(assetId), EnterpriseValues.isId(templateId), ["DEPARTURE", "RETURN", "AD_HOC"].contains(context),
              (1...2_147_483_647).contains(templateVersion), (1...2_147_483_647).contains(policyRevision),
              EnterpriseValues.isCanonicalInstant(startedAt), let started = EnterpriseValues.parseInstantMillis(startedAt),
              let completed = EnterpriseValues.parseInstantMillis(envelope.declared.occurredAt), started <= completed,
              !acknowledged || outcome == .completedWithIssues, !responses.isEmpty, responses.count <= 40,
              responses.allSatisfy({ EnterpriseValues.isValidText($0.note, maxScalars: 140, required: false) }) else {
            throw EnterpriseOperationError.malformed
        }
        let answers: [EnterpriseJson] = responses.map { response -> EnterpriseJson in
            EnterpriseJson.object([
                "answer": .text(response.answer.rawValue),
                "note": response.note.isEmpty ? EnterpriseJson.null : EnterpriseJson.text(response.note),
                "photo": .bool(false),
            ])
        }
        return try seal(
            envelope: envelope,
            type: checklistComplete,
            entityType: "checklist_run",
            entityId: runId,
            expectedRevision: nil,
            dependsOn: [],
            softReferences: [],
            payload: [
                "acknowledged": .bool(acknowledged),
                "answers": .array(answers),
                "asset_id": .text(assetId),
                "context": .text(context),
                "device_outcome": .text(outcome.rawValue),
                "policy_revision": .int(policyRevision),
                "started_at": .text(startedAt),
                "template_id": .text(templateId),
                "template_version": .int(templateVersion),
            ]
        )
    }
}

// MARK: - Operation delivery wire

public enum EnterpriseSyncResult: String, Codable, Sendable {
    case accepted = "ACCEPTED"
    case acceptedForReview = "ACCEPTED_FOR_REVIEW"
    case rejected = "REJECTED"
    case retry = "RETRY"
}

/// One validated server acknowledgement; an untrusted receipt is never proof of authority.
public struct EnterpriseAcknowledgement: Equatable, Sendable {
    public let operationId: String
    public let payloadDigest: String
    public let result: EnterpriseSyncResult
    public let code: String?
    public let entityRevision: Int64?
    public let receivedAt: String?
    public let reviewItemId: String?
    public let warnings: [String]
}

/// `POST /v1/orgs/{org_id}/sync/operations` body and response (Android `SyncBatch`, `SyncBatchResponse`).
public enum EnterpriseSyncWire {
    public static let maxOperations = 50
    static let reviewCodes: Set<String> = [
        "ORPHAN_REFERENCE", "ACCESS_ENDED_BEFORE_RECEIPT", "PERMISSION_CHANGED", "ASSET_UNAVAILABLE", "ASSET_BUSY",
        "TRIP_ALREADY_CLOSED", "PERIOD_LOCKED", "STALE_REVISION", "ORG_SUSPENDED_GRACE", "PLAN_CHANGED",
    ]
    static let rejectionCodes: Set<String> = [
        "OPERATION_ID_REUSED", "MALFORMED", "DEVICE_REVOKED", "MEMBERSHIP_ENDED", "INTAKE_WINDOW_CLOSED", "FORBIDDEN", "ORG_SUSPENDED",
    ]
    static let retryCodes: Set<String> = ["DEPENDENCY_PENDING", "UNAVAILABLE"]

    /// The delivery proof stays outside the hashed intent: `{access_token, family_id, operations, schema_version}`.
    public static func batchBody(familyId: String, accessToken: String, operations: [EnterpriseSealedOperation]) throws -> Data {
        guard EnterpriseValues.isId(familyId), isAccessToken(accessToken), !operations.isEmpty,
              operations.count <= maxOperations, Set(operations.map { $0.operationId }).count == operations.count else {
            throw EnterpriseProtocolFailure.invalid
        }
        let values = try operations.map { operation -> EnterpriseJson in
            try EnterpriseCanonicalJson.parse(
                operation.canonical,
                maxBytes: EnterpriseCanonicalJson.maxOperationBytes,
                maxDepth: EnterpriseCanonicalJson.maxOperationDepth
            )
        }
        let body = try EnterpriseCanonicalJson.encode(.object([
            "schema_version": .int(1),
            "family_id": .text(familyId),
            "access_token": .text(accessToken),
            "operations": .array(values),
        ]))
        if body.count > EnterpriseCanonicalJson.maxBatchBytes { throw EnterpriseProtocolFailure.limit }
        return body
    }

    /// Graphic ASCII, 1..512 characters.
    public static func isAccessToken(_ token: String) -> Bool {
        let bytes = Array(token.utf8)
        return !bytes.isEmpty && bytes.count <= 512 && bytes.allSatisfy { $0 >= 0x21 && $0 <= 0x7E }
    }

    /// Complete preflight: one acknowledgement per sent operation, in order, bound to its id and digest.
    public static func parseBatchResponse(_ data: Data, expected: [EnterpriseSealedOperation]) throws -> [EnterpriseAcknowledgement] {
        let root = try EnterpriseCanonicalJson.parse(data)
        guard let fields = root.objectValue, Set(fields.keys) == ["schema_version", "acknowledgements"],
              fields["schema_version"] == EnterpriseJson.int(1), let values = fields["acknowledgements"]?.arrayValue else {
            throw EnterpriseProtocolFailure.invalid
        }
        guard values.count == expected.count else { throw EnterpriseProtocolFailure.binding }
        return try zip(values, expected).map { pair -> EnterpriseAcknowledgement in
            let (value, operation) = pair
            let ack = try parseAcknowledgement(value)
            if ack.operationId != operation.operationId { throw EnterpriseProtocolFailure.binding }
            if ack.payloadDigest != operation.payloadDigest { throw EnterpriseProtocolFailure.digest }
            return ack
        }
    }

    static func parseAcknowledgement(_ value: EnterpriseJson) throws -> EnterpriseAcknowledgement {
        let keys: Set<String> = [
            "schema_version", "operation_id", "payload_digest", "result", "code", "entity_revision", "received_at",
            "review_item_id", "warnings",
        ]
        guard let fields = value.objectValue, Set(fields.keys) == keys, fields["schema_version"] == EnterpriseJson.int(1),
              let operationId = fields["operation_id"]?.textValue, EnterpriseValues.isId(operationId),
              let digest = fields["payload_digest"]?.textValue, isDigest(digest),
              let rawResult = fields["result"]?.textValue, let result = EnterpriseSyncResult(rawValue: rawResult),
              let warningValues = fields["warnings"]?.arrayValue else {
            throw EnterpriseProtocolFailure.invalid
        }
        let code = try optionalText(fields["code"])
        let revision = try optionalPositive(fields["entity_revision"])
        let received = try optionalText(fields["received_at"])
        let review = try optionalText(fields["review_item_id"])
        // Go's unset time is never a server receipt.
        if let received, !EnterpriseValues.isCanonicalInstant(received) || received == "0001-01-01T00:00:00Z" {
            throw EnterpriseProtocolFailure.invalid
        }
        if let review, !EnterpriseValues.isId(review) { throw EnterpriseProtocolFailure.invalid }
        var warnings: [String] = []
        for warning in warningValues {
            guard let text = warning.textValue, text == "CLOCK_SKEW", !warnings.contains(text) else {
                throw EnterpriseProtocolFailure.invalid
            }
            warnings.append(text)
        }
        switch result {
        case .retry:
            guard let retryCode = code, retryCodes.contains(retryCode), revision == nil, received == nil, review == nil else {
                throw EnterpriseProtocolFailure.invalid
            }
        case .accepted:
            guard received != nil, code == nil, revision != nil, review == nil else { throw EnterpriseProtocolFailure.invalid }
        case .acceptedForReview:
            guard received != nil, let reviewCode = code, reviewCodes.contains(reviewCode), review != nil else {
                throw EnterpriseProtocolFailure.invalid
            }
        case .rejected:
            guard received != nil, let rejectionCode = code, rejectionCodes.contains(rejectionCode), revision == nil, review == nil else {
                throw EnterpriseProtocolFailure.invalid
            }
        }
        return EnterpriseAcknowledgement(
            operationId: operationId,
            payloadDigest: digest,
            result: result,
            code: code,
            entityRevision: revision,
            receivedAt: received,
            reviewItemId: review,
            warnings: warnings
        )
    }

    static func isDigest(_ value: String) -> Bool {
        guard value.hasPrefix("sha256:") else { return false }
        let hex = Array(value.utf8.dropFirst(7))
        return hex.count == 64 && hex.allSatisfy { ($0 >= 0x30 && $0 <= 0x39) || ($0 >= 0x61 && $0 <= 0x66) }
    }

    static func optionalText(_ value: EnterpriseJson?) throws -> String? {
        guard let value else { throw EnterpriseProtocolFailure.invalid }
        if value.isNull { return nil }
        guard let text = value.textValue else { throw EnterpriseProtocolFailure.invalid }
        return text
    }

    static func optionalPositive(_ value: EnterpriseJson?) throws -> Int64? {
        guard let value else { throw EnterpriseProtocolFailure.invalid }
        if value.isNull { return nil }
        guard let lexeme = value.integerLexeme, let number = Int64(lexeme), number >= 1 else {
            throw EnterpriseProtocolFailure.invalid
        }
        return number
    }
}

// MARK: - Checklist evaluation

public enum EnterpriseChecklistFailure: Error, Equatable, Sendable {
    case invalid
    case answerNotAllowed
    case photoRequired
    case incomplete
}

public struct EnterpriseChecklistEvaluation: Equatable, Sendable {
    public let outcome: EnterpriseChecklistOutcome
    public let issues: [Int]
    public let critical: [Int]
    /// A critical issue under a WARN policy must be acknowledged by the person.
    public let acknowledgementRequired: Bool
}

/// Android `Checklist.evaluate`: one answer per item, only optional items may be not applicable,
/// a NOT_OK on a photo item needs a photo (not declared from iOS), critical issues block under BLOCK.
public enum EnterpriseChecklistRules {
    public static func evaluate(items: [EnterpriseChecklistItem], policyMode: String,
                                answers: [EnterpriseChecklistAnswer?]) throws -> EnterpriseChecklistEvaluation {
        guard !items.isEmpty, items.count <= 40, answers.count == items.count, policyMode == "BLOCK" || policyMode == "WARN",
              items.allSatisfy({ !($0.critical && $0.optional) }) else {
            throw EnterpriseChecklistFailure.invalid
        }
        var notAllowed = false
        var photo = false
        var incomplete = false
        var issues: [Int] = []
        var critical: [Int] = []
        for (index, answer) in answers.enumerated() {
            let item = items[index]
            switch answer {
            case .some(.notApplicable):
                if !item.optional { notAllowed = true }
            case .some(.notOk):
                if item.photoOnNotOk { photo = true }
                issues.append(index + 1)
                if item.critical { critical.append(index + 1) }
            case .some(.ok):
                break
            case .none:
                incomplete = true
            }
        }
        if notAllowed { throw EnterpriseChecklistFailure.answerNotAllowed }
        if photo { throw EnterpriseChecklistFailure.photoRequired }
        if incomplete { throw EnterpriseChecklistFailure.incomplete }
        let outcome: EnterpriseChecklistOutcome
        if issues.isEmpty {
            outcome = .completedOk
        } else if !critical.isEmpty && policyMode == "BLOCK" {
            outcome = .blocking
        } else {
            outcome = .completedWithIssues
        }
        return EnterpriseChecklistEvaluation(
            outcome: outcome,
            issues: issues,
            critical: critical,
            acknowledgementRequired: outcome == .completedWithIssues && !critical.isEmpty
        )
    }
}
