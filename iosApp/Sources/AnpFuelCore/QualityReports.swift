import Foundation

public struct QualityReportScope: Equatable, Hashable, Sendable {
    public let origin: String
    let ownerScope: String
    let familyFingerprint: String

    public init?(origin: String, ownerScope: String, familyId: String) {
        guard ownerScope.hasPrefix("account:"), QualityWire.identifier(familyId), !origin.isEmpty else { return nil }
        self.origin = origin
        self.ownerScope = ownerScope
        familyFingerprint = IosDeviceProofSigner.sha256Hex(Data(familyId.utf8))
    }

    var key: String {
        IosDeviceProofSigner.sha256Hex(Data((origin + "\u{0}" + ownerScope + "\u{0}" + familyFingerprint).utf8))
    }

    func matches(_ session: PortableAuth.Session) -> Bool {
        ownerScope.hasPrefix("account:\(session.accountId):")
            && familyFingerprint == IosDeviceProofSigner.sha256Hex(Data(session.familyId.utf8))
    }
}

public enum QualityReportAction: String, Codable, Sendable {
    case submit = "SUBMIT"
    case edit = "EDIT"
    case withdraw = "WITHDRAW"
    case appeal = "APPEAL"
}

public enum QualityReportKind: String, Codable, Sendable {
    case experience = "EXPERIENCE"
    case linkReview = "LINK_REVIEW"
}

public enum QualityReportSymptom: String, Codable, CaseIterable, Identifiable, Sendable {
    case starting = "STARTING"
    case performance = "PERFORMANCE"
    case consumption = "CONSUMPTION"
    case other = "OTHER"

    public var id: String { rawValue }
}

public enum QualityReportError: Error, Equatable, Sendable {
    case invalid
    case authRequired
    case ownerChanged
    case conflict
    case notFound
    case quota(retryAfterSeconds: Int64?)
    case unavailable(retryAfterSeconds: Int64?)
}

public struct QualityReportCommand: Codable, Equatable, Sendable {
    public let commandId: String
    public let reportId: String
    public let action: QualityReportAction
    public let expectedRevision: Int32
    public let kind: QualityReportKind?
    public let stationId: String?
    public let fuel: String?
    public let occurredOn: String?
    public let symptom: QualityReportSymptom?
    public let text: String?
    public let officialRevisionId: String?
    public let sampleRef: String?

    public init(
        commandId: String = UUID().uuidString.lowercased(), reportId: String, action: QualityReportAction,
        expectedRevision: Int32, kind: QualityReportKind? = nil, stationId: String? = nil,
        fuel: String? = nil, occurredOn: String? = nil, symptom: QualityReportSymptom? = nil,
        text: String? = nil, officialRevisionId: String? = nil, sampleRef: String? = nil
    ) {
        self.commandId = commandId
        self.reportId = reportId
        self.action = action
        self.expectedRevision = expectedRevision
        self.kind = kind
        self.stationId = stationId
        self.fuel = fuel
        self.occurredOn = occurredOn
        self.symptom = symptom
        self.text = text
        self.officialRevisionId = officialRevisionId
        self.sampleRef = sampleRef
    }

    public func validate(now: Date = Date()) throws {
        guard QualityWire.identifier(commandId), QualityWire.identifier(reportId),
              expectedRevision >= 0, expectedRevision < Int32.max else { throw QualityReportError.invalid }
        switch action {
        case .withdraw, .appeal:
            guard expectedRevision > 0, kind == nil, stationId == nil, fuel == nil,
                  occurredOn == nil, symptom == nil, officialRevisionId == nil, sampleRef == nil else {
                throw QualityReportError.invalid
            }
            if action == .withdraw {
                guard text == nil else { throw QualityReportError.invalid }
            } else {
                guard Self.validText(text) else { throw QualityReportError.invalid }
            }
        case .submit, .edit:
            guard action == .submit ? expectedRevision == 0 : expectedRevision > 0,
                  let kind, let stationId, QualityWire.identifier(stationId), let fuel, FuelProduct.fromWireCode(fuel) != nil,
                  let occurredOn, QualityWire.day(occurredOn), occurredOn <= Self.dayUTC(now),
                  symptom != nil, Self.validText(text), (officialRevisionId == nil) == (sampleRef == nil),
                  officialRevisionId.map(QualityWire.identifier) ?? true,
                  sampleRef.map(QualityWire.validSampleReference) ?? true, kind != .linkReview || sampleRef != nil else {
                throw QualityReportError.invalid
            }
        }
    }

    static func validText(_ value: String?) -> Bool {
        guard let value, !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, value.unicodeScalars.count <= 1_000 else {
            return false
        }
        return value.unicodeScalars.allSatisfy { scalar in
            scalar.value == 9 || scalar.value == 10 || scalar.properties.generalCategory != .control
        }
    }

    public static func dayUTC(_ date: Date) -> String {
        AnpCalendar.isoDay(date)
    }
}

public struct QualityReport: Decodable, Equatable, Identifiable, Sendable {
    public let id: String
    public let revision: Int32
    public let kind: QualityReportKind
    public let stationId: String
    public let fuel: String
    public let occurredOn: String
    public let symptom: QualityReportSymptom
    public let text: String
    public let status: String
    public let evidenceLevel: String
    public let officialRevisionId: String?
    public let sampleRef: String?
    public let reviewReason: String?

    func validate() throws {
        guard QualityWire.identifier(id), revision > 0, QualityWire.identifier(stationId), FuelProduct.fromWireCode(fuel) != nil,
              QualityWire.day(occurredOn), text.utf8.count <= 4_000, (officialRevisionId == nil) == (sampleRef == nil),
              officialRevisionId.map(QualityWire.identifier) ?? true,
              sampleRef.map(QualityWire.validSampleReference) ?? true, evidenceLevel == "USER_CLAIM" else {
            throw QualityReportError.unavailable(retryAfterSeconds: nil)
        }
    }
}

public struct QualityReportReceipt: Decodable, Equatable, Sendable {
    public let reportId: String
    public let revision: Int32
    public let status: String
}

public struct QualityReportPage: Equatable, Sendable {
    public let items: [QualityReport]
    public let nextAfterId: String?
}

enum QualityReportWire {
    private struct Page: Decodable {
        let items: [QualityReport]
        let nextAfterId: String?
        enum CodingKeys: String, CodingKey { case items, nextAfterId }

        init(from decoder: Decoder) throws {
            let values = try decoder.container(keyedBy: CodingKeys.self)
            guard values.contains(.items) else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
            items = try values.decodeIfPresent([QualityReport].self, forKey: .items) ?? []
            nextAfterId = try values.decodeIfPresent(String.self, forKey: .nextAfterId)
        }
    }

    static let commandFields: Set<String> = [
        "command_id", "report_id", "action", "expected_revision", "kind", "station_id", "fuel",
        "occurred_on", "symptom", "text", "official_revision_id", "sample_ref",
    ]

    static func decodeCommand(_ data: Data, now: Date) throws -> QualityReportCommand {
        guard var doc = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              Set(doc.keys).isSubset(of: commandFields) else { throw QualityReportError.invalid }
        if doc["expected_revision"] == nil { doc["expected_revision"] = 0 }
        let command = try AbastevoHttpClient.publicDecoder().decode(
            QualityReportCommand.self, from: JSONSerialization.data(withJSONObject: doc)
        )
        try command.validate(now: now)
        return command
    }

    static func fields(_ command: QualityReportCommand) throws -> [String: Any] {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        guard let fields = try JSONSerialization.jsonObject(with: encoder.encode(command)) as? [String: Any],
              Set(fields.keys).isSubset(of: commandFields) else { throw QualityReportError.invalid }
        return fields
    }

    static func page(_ data: Data, after: String?) throws -> QualityReportPage {
        let page = try AbastevoHttpClient.publicDecoder().decode(Page.self, from: data)
        let ids = page.items.map(\.id)
        guard page.items.count <= 50, Set(ids).count == ids.count,
              zip(ids, ids.dropFirst()).allSatisfy({ $0.0 < $0.1 }),
              after.map({ cursor in ids.allSatisfy { $0 > cursor } }) ?? true,
              page.nextAfterId.map({ next in page.items.count == 50 && next == ids.last && next != after }) ?? true else {
            throw QualityReportError.unavailable(retryAfterSeconds: nil)
        }
        for item in page.items { try item.validate() }
        return QualityReportPage(items: page.items, nextAfterId: page.nextAfterId)
    }

    static func receipt(_ data: Data, command: QualityReportCommand) throws -> QualityReportReceipt {
        let receipt = try AbastevoHttpClient.publicDecoder().decode(QualityReportReceipt.self, from: data)
        guard command.expectedRevision < Int32.max, receipt.reportId == command.reportId,
              receipt.revision == command.expectedRevision + 1,
              receipt.status == (command.action == .withdraw ? "WITHDRAWN" : "PENDING_REVIEW") else {
            throw QualityReportError.unavailable(retryAfterSeconds: nil)
        }
        return receipt
    }
}

public extension CommunityWriteClient {
    func executeQualityReport(_ command: QualityReportCommand, scope: QualityReportScope) async throws -> QualityReportReceipt {
        try command.validate()
        let data = try await qualityReportPost("commands", fields: QualityReportWire.fields(command),
                                               operationId: command.commandId, scope: scope)
        return try QualityReportWire.receipt(data, command: command)
    }

    func qualityReport(id: String, scope: QualityReportScope) async throws -> QualityReport {
        guard QualityWire.identifier(id) else { throw QualityReportError.invalid }
        let data = try await qualityReportPost("get", fields: ["report_id": id], scope: scope)
        let row = try AbastevoHttpClient.publicDecoder().decode(QualityReport.self, from: data)
        guard row.id == id else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
        try row.validate()
        return row
    }

    func qualityReports(after id: String? = nil, scope: QualityReportScope) async throws -> QualityReportPage {
        if let id, !QualityWire.identifier(id) { throw QualityReportError.invalid }
        let fields = id.map { ["after_id": $0] } ?? [:]
        let data = try await qualityReportPost("list", fields: fields, scope: scope)
        return try QualityReportWire.page(data, after: id)
    }

    private func qualityReportPost(
        _ route: String, fields: [String: Any], operationId: String? = nil, scope: QualityReportScope
    ) async throws -> Data {
        guard transport.origin == scope.origin, transport.ownerScope() == scope.ownerScope else {
            throw QualityReportError.ownerChanged
        }
        do {
            let result = try await ownerPost(path: "/v1/quality/reports/" + route, expectedScope: scope.ownerScope,
                                             requiresDeviceProof: true, idempotencyKey: operationId,
                                             requiresSameSessionFamily: true) { session in
                guard scope.matches(session) else { return nil }
                var body = fields
                body["family_id"] = session.familyId
                body["access_token"] = session.accessToken
                return try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
            }
            guard result.status == 200 else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
            return result.body
        } catch let error as CommunityWriteError {
            switch error {
            case .signInRequired: throw QualityReportError.authRequired
            case .invalid: throw QualityReportError.ownerChanged
            default: throw QualityReportError.unavailable(retryAfterSeconds: nil)
            }
        } catch let error as SignedTransportError {
            switch error {
            case .sessionRequired:
                throw transport.ownerScope() == scope.ownerScope ? QualityReportError.authRequired : QualityReportError.ownerChanged
            case .refused(let status, _, let delay):
                switch status {
                case 400: throw QualityReportError.invalid
                case 401, 403: throw QualityReportError.authRequired
                case 404: throw QualityReportError.notFound
                case 409: throw QualityReportError.conflict
                case 429: throw QualityReportError.quota(retryAfterSeconds: delay)
                default: throw QualityReportError.unavailable(retryAfterSeconds: delay)
                }
            default: throw QualityReportError.unavailable(retryAfterSeconds: nil)
            }
        }
    }
}
