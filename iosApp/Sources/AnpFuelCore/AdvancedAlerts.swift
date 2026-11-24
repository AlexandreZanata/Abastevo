import Foundation

public enum AdvancedAlertError: Error, Equatable, Sendable {
    case invalid
    case authRequired
    case ownerChanged
    case capabilityRequired
    case notFound
    case conflict
    case rateLimited(retryAfterSeconds: Int64?)
    case unavailable(retryAfterSeconds: Int64?)
    case requestInProgress
}

public enum AdvancedAlertPredicateKind: String, Codable, CaseIterable, Hashable, Identifiable, Sendable {
    case threshold
    case change
    public var id: String { rawValue }
}

public enum AdvancedAlertSource: String, Codable, CaseIterable, Hashable, Identifiable, Sendable {
    case official
    case community
    public var id: String { rawValue }
}

public enum AdvancedAlertCondition: String, Codable, CaseIterable, Hashable, Identifiable, Sendable {
    case standard = "STANDARD"
    case cash = "CASH"
    case debit = "DEBIT"
    case credit = "CREDIT"
    case app = "APP"
    case loyalty = "LOYALTY"
    case other = "OTHER"
    public var id: String { rawValue }
}

public enum AdvancedAlertOperator: String, Codable, CaseIterable, Hashable, Identifiable, Sendable {
    case all = "AND"
    case any = "OR"
    public var id: String { rawValue }
}

public struct AdvancedAlertPredicate: Codable, Equatable, Sendable {
    public let kind: AdvancedAlertPredicateKind
    public let fuel: String
    public let unit: String
    public let condition: AdvancedAlertCondition
    public let source: AdvancedAlertSource
    public let thresholdMilli: Int64?
    public let dropMilli: Int64?
    public let dropPercentBp: Int64?
    public let baselineHours: Int64?

    public init(
        kind: AdvancedAlertPredicateKind, fuel: FuelProduct, condition: AdvancedAlertCondition, source: AdvancedAlertSource,
        thresholdMilli: Int64? = nil, dropMilli: Int64? = nil, dropPercentBp: Int64? = nil, baselineHours: Int64? = nil
    ) {
        self.kind = kind
        self.fuel = fuel.wireCode
        self.unit = fuel.unitWire
        self.condition = condition
        self.source = source
        self.thresholdMilli = thresholdMilli
        self.dropMilli = dropMilli
        self.dropPercentBp = dropPercentBp
        self.baselineHours = baselineHours
    }

    public func validate() throws {
        guard let fuel = FuelProduct.fromWireCode(fuel), fuel != .lpgP13, unit == fuel.unitWire else {
            throw AdvancedAlertError.invalid
        }
        let drop = dropMilli ?? 0
        let percent = dropPercentBp ?? 0
        switch kind {
        case .threshold:
            guard let thresholdMilli, (PortableMoney.minMilli...PortableMoney.maxMilli).contains(thresholdMilli),
                  drop == 0, percent == 0, (baselineHours ?? 0) == 0 else { throw AdvancedAlertError.invalid }
        case .change:
            guard (thresholdMilli ?? 0) == 0, (0...PortableMoney.maxMilli).contains(drop), (0...10_000).contains(percent),
                  drop > 0 || percent > 0, let baselineHours, (1...168).contains(baselineHours) else { throw AdvancedAlertError.invalid }
        }
    }

    fileprivate var canonical: Self {
        guard let product = FuelProduct.fromWireCode(fuel) else { return self }
        switch kind {
        case .threshold:
            return Self(kind: kind, fuel: product, condition: condition, source: source, thresholdMilli: thresholdMilli)
        case .change:
            return Self(kind: kind, fuel: product, condition: condition, source: source, dropMilli: dropMilli,
                        dropPercentBp: dropPercentBp, baselineHours: baselineHours)
        }
    }
}

public struct AdvancedAlertTarget: Codable, Equatable, Sendable {
    public let stationIds: [String]?
    public let city: String?

    public init(stationIds: [String]? = nil, city: String? = nil) {
        self.stationIds = stationIds
        self.city = city
    }

    public var isStationTarget: Bool { !(stationIds ?? []).isEmpty }

    fileprivate func validate() throws {
        let stations = stationIds ?? []
        let hasCity = !(city ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        guard isStationTarget != hasCity else { throw AdvancedAlertError.invalid }
        if isStationTarget {
            guard stations.count <= 10, stations.allSatisfy(AdvancedAlertWire.validID), Set(stations).count == stations.count else {
                throw AdvancedAlertError.invalid
            }
        } else if let city, city.utf8.count > 80 {
            throw AdvancedAlertError.invalid
        }
    }
}

public struct AdvancedAlertSchedule: Codable, Equatable, Sendable {
    public let days: [Int]
    public let windowStart: String
    public let windowEnd: String
    public let timezone: String
    public let quietStart: String
    public let quietEnd: String
    public let cooldownHours: Int64

    public init(days: [Int], windowStart: String, windowEnd: String, timezone: String,
                quietStart: String, quietEnd: String, cooldownHours: Int64) {
        self.days = days
        self.windowStart = windowStart
        self.windowEnd = windowEnd
        self.timezone = timezone
        self.quietStart = quietStart
        self.quietEnd = quietEnd
        self.cooldownHours = cooldownHours
    }

    fileprivate func validate() throws {
        guard (1...7).contains(days.count), days.allSatisfy({ (0...6).contains($0) }), Set(days).count == days.count,
              [windowStart, windowEnd, quietStart, quietEnd].allSatisfy(Self.validClock),
              (1...64).contains(timezone.utf8.count), TimeZone(identifier: timezone) != nil,
              (0...168).contains(cooldownHours) else { throw AdvancedAlertError.invalid }
    }

    private static func validClock(_ text: String) -> Bool {
        let bytes = Array(text.utf8)
        guard bytes.count == 5, bytes[2] == 58, [bytes[0], bytes[1], bytes[3], bytes[4]].allSatisfy({ (48...57).contains($0) }) else {
            return false
        }
        let hour = (Int(bytes[0]) - 48) * 10 + Int(bytes[1]) - 48
        let minute = (Int(bytes[3]) - 48) * 10 + Int(bytes[4]) - 48
        return hour < 24 && minute < 60
    }
}

public struct AdvancedAlertRule: Codable, Equatable, Sendable {
    public let predicates: [AdvancedAlertPredicate]
    public let op: AdvancedAlertOperator
    public let target: AdvancedAlertTarget
    public let schedule: AdvancedAlertSchedule

    private enum CodingKeys: String, CodingKey { case predicates, op = "operator", target, schedule }

    public init(predicates: [AdvancedAlertPredicate], op: AdvancedAlertOperator, target: AdvancedAlertTarget,
                schedule: AdvancedAlertSchedule) {
        self.predicates = predicates
        self.op = op
        self.target = target
        self.schedule = schedule
    }

    public func validate() throws {
        guard (1...3).contains(predicates.count) else { throw AdvancedAlertError.invalid }
        for predicate in predicates { try predicate.validate() }
        try target.validate()
        try schedule.validate()
    }

    fileprivate var canonical: Self {
        Self(predicates: predicates.map(\.canonical), op: op, target: target, schedule: schedule)
    }
}

public struct AdvancedAlertRuleRecord: Decodable, Equatable, Identifiable, Sendable {
    public let ruleId: String
    public let revision: Int64
    public let enabled: Bool
    public let payload: AdvancedAlertRule
    public var id: String { ruleId }
}

public struct AdvancedAlertCommand: Codable, Equatable, Sendable {
    public let operationId: String
    public let ruleId: String
    public let expectedRevision: Int64
    public let delete: Bool
    public let enabled: Bool?
    public let payload: AdvancedAlertRule?

    public init(operationId: String = UUID().uuidString.lowercased(), ruleId: String, expectedRevision: Int64,
                delete: Bool = false, enabled: Bool? = nil, payload: AdvancedAlertRule? = nil) {
        self.operationId = operationId
        self.ruleId = ruleId
        self.expectedRevision = expectedRevision
        self.delete = delete
        self.enabled = enabled
        self.payload = payload
    }

    public func validate() throws {
        guard AdvancedAlertWire.validID(operationId), AdvancedAlertWire.validID(ruleId),
              (0..<Int64.max).contains(expectedRevision) else { throw AdvancedAlertError.invalid }
        if delete {
            guard expectedRevision > 0, enabled == nil, payload == nil else { throw AdvancedAlertError.invalid }
        } else if let payload {
            // The server accepts storing city rules but currently never evaluates them.
            guard payload.target.isStationTarget else { throw AdvancedAlertError.invalid }
            try payload.validate()
        } else {
            guard expectedRevision > 0, enabled != nil else { throw AdvancedAlertError.invalid }
        }
    }
}

public struct AdvancedAlertReceipt: Decodable, Equatable, Sendable {
    public let operationId: String
    public let ruleId: String
    public let revision: Int64
    public let status: String
    public let replayed: Bool
}

public enum AdvancedAlertInboxStatus: Equatable, Sendable {
    case evaluated, dispatched, sent, dropped, unknown

    fileprivate init(_ value: String) {
        switch value {
        case "evaluated": self = .evaluated
        case "dispatched": self = .dispatched
        case "sent": self = .sent
        case "dropped": self = .dropped
        default: self = .unknown
        }
    }
}

public struct AdvancedAlertInboxItem: Equatable, Identifiable, Sendable {
    public let id: String
    public let ruleId: String
    public let status: AdvancedAlertInboxStatus
    public let stationId: String?
    public let observedAt: Date?
    public let source: AdvancedAlertSource?

    public var canNotify: Bool {
        (status == .dispatched || status == .sent) && stationId != nil && source != nil && observedAt != nil
    }
}

public struct AdvancedAlertInbox: Equatable, Sendable {
    public let items: [AdvancedAlertInboxItem]
    public let limitReached: Bool
}

enum AdvancedAlertWire {
    static let maximumInboxLimit = 100

    private struct RulePage: Decodable { let rules: [AdvancedAlertRuleRecord] }
    private struct CommandPage: Decodable { let results: [AdvancedAlertReceipt] }
    private struct EraseResult: Decodable { let status: String }
    private struct InboxPage: Decodable {
        struct Row: Decodable {
            struct Candidate: Decodable {
                let ruleId: String?
                let stationId: String?
                let source: String?
                let at: String?
            }
            let inboxId: String
            let ruleId: String
            let status: String
            let payload: Candidate?
        }
        let items: [Row]
    }

    static func validID(_ id: String) -> Bool {
        id == id.lowercased() && id != "00000000-0000-0000-0000-000000000000" && CommunityWire.isUuid(id)
    }

    static func commandBody(_ command: AdvancedAlertCommand) throws -> [String: Any] {
        try command.validate()
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        let canonical = AdvancedAlertCommand(
            operationId: command.operationId, ruleId: command.ruleId, expectedRevision: command.expectedRevision,
            delete: command.delete, enabled: command.enabled, payload: command.payload?.canonical
        )
        guard let fields = try JSONSerialization.jsonObject(with: encoder.encode(canonical)) as? [String: Any] else {
            throw AdvancedAlertError.invalid
        }
        return ["commands": [fields]]
    }

    static func rules(_ data: Data) throws -> [AdvancedAlertRuleRecord] {
        let rows = try decode(RulePage.self, data).rules
        guard rows.count <= 20, Set(rows.map(\.id)).count == rows.count else { throw invalidReply }
        for row in rows {
            guard validID(row.ruleId), row.revision > 0 else { throw invalidReply }
            do { try row.payload.validate() } catch { throw invalidReply }
        }
        return rows
    }

    static func inbox(_ data: Data, limit: Int) throws -> AdvancedAlertInbox {
        let rows = try decode(InboxPage.self, data).items
        guard (1...maximumInboxLimit).contains(limit), rows.count <= limit,
              Set(rows.map(\.inboxId)).count == rows.count else { throw invalidReply }
        let items = try rows.map { row -> AdvancedAlertInboxItem in
            guard validID(row.inboxId), validID(row.ruleId) else { throw invalidReply }
            let projection = row.payload.flatMap { $0.ruleId == row.ruleId ? $0 : nil }
            let station = (projection?.stationId).flatMap { validID($0) ? $0 : nil }
            let observedAt = (projection?.at).flatMap { timestamp -> Date? in
                guard let ticks = CommunityWire.millis(timestamp), ticks > 0 else { return nil }
                return Date(timeIntervalSince1970: Double(ticks) / 1_000)
            }
            return AdvancedAlertInboxItem(id: row.inboxId, ruleId: row.ruleId, status: AdvancedAlertInboxStatus(row.status),
                                          stationId: station, observedAt: observedAt,
                                          source: (projection?.source).flatMap(AdvancedAlertSource.init(rawValue:)))
        }
        return AdvancedAlertInbox(items: items, limitReached: rows.count == limit)
    }

    static func receipt(_ data: Data, command: AdvancedAlertCommand) throws -> AdvancedAlertReceipt {
        let results = try decode(CommandPage.self, data).results
        guard results.count == 1, let receipt = results.first, receipt.operationId == command.operationId,
              receipt.ruleId == command.ruleId, receipt.revision > 0, receipt.status == "accepted",
              command.delete || receipt.revision == command.expectedRevision + 1 else { throw invalidReply }
        return receipt
    }

    static func erased(_ data: Data) throws {
        guard try decode(EraseResult.self, data).status == "erased" else { throw invalidReply }
    }

    private static var invalidReply: AdvancedAlertError { .unavailable(retryAfterSeconds: nil) }

    private static func decode<T: Decodable>(_ type: T.Type, _ data: Data) throws -> T {
        do { return try AbastevoHttpClient.publicDecoder().decode(type, from: data) }
        catch { throw invalidReply }
    }
}

public extension CommunityWriteClient {
    func advancedAlertRules(scope: AccountDataExportScope) async throws -> [AdvancedAlertRuleRecord] {
        let response = try await advancedAlertPost("rules", fields: [:], scope: scope)
        return try AdvancedAlertWire.rules(response)
    }

    func advancedAlertInbox(limit: Int = 100, scope: AccountDataExportScope) async throws -> AdvancedAlertInbox {
        guard (1...AdvancedAlertWire.maximumInboxLimit).contains(limit) else { throw AdvancedAlertError.invalid }
        let response = try await advancedAlertPost("inbox", fields: ["limit": limit], scope: scope)
        return try AdvancedAlertWire.inbox(response, limit: limit)
    }

    func executeAdvancedAlert(_ command: AdvancedAlertCommand, scope: AccountDataExportScope) async throws -> AdvancedAlertReceipt {
        let response = try await advancedAlertPost("commands", fields: AdvancedAlertWire.commandBody(command),
                                                   operationId: command.operationId, scope: scope)
        return try AdvancedAlertWire.receipt(response, command: command)
    }

    /// The backend does not deduplicate an erase against later changes on another device.
    /// A lost response can be checked through reads before the user confirms another erase.
    func eraseAdvancedAlerts(operationId: String, scope: AccountDataExportScope) async throws {
        guard AdvancedAlertWire.validID(operationId) else { throw AdvancedAlertError.invalid }
        let response = try await advancedAlertPost("erase", fields: [:], operationId: operationId, scope: scope)
        try AdvancedAlertWire.erased(response)
    }

    private func advancedAlertPost(
        _ route: String, fields: [String: Any], operationId: String? = nil, scope: AccountDataExportScope
    ) async throws -> Data {
        guard transport.origin == scope.origin, transport.ownerScope() == scope.ownerScope else {
            throw AdvancedAlertError.ownerChanged
        }
        do {
            let result = try await ownerPost(path: "/v1/alerts/" + route, expectedScope: scope.ownerScope,
                                             requiresDeviceProof: operationId != nil, idempotencyKey: operationId,
                                             requiresSameSessionFamily: true) { session in
                guard session.familyId == scope.sessionFamilyId else { return nil }
                var body = fields
                body["family_id"] = session.familyId
                body["access_token"] = session.accessToken
                return try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
            }
            guard result.status == 200 else { throw AdvancedAlertError.unavailable(retryAfterSeconds: nil) }
            return result.body
        } catch let error as CommunityWriteError {
            switch error {
            case .invalid: throw AdvancedAlertError.ownerChanged
            case .signInRequired: throw AdvancedAlertError.authRequired
            default: throw AdvancedAlertError.unavailable(retryAfterSeconds: nil)
            }
        } catch let error as SignedTransportError {
            switch error {
            case .sessionRequired:
                throw transport.ownerScope() == scope.ownerScope ? AdvancedAlertError.authRequired : AdvancedAlertError.ownerChanged
            case .refused(let status, _, let hint):
                switch status {
                case 400: throw AdvancedAlertError.invalid
                case 401: throw AdvancedAlertError.authRequired
                case 403: throw AdvancedAlertError.capabilityRequired
                case 404: throw AdvancedAlertError.notFound
                case 409: throw AdvancedAlertError.conflict
                case 429: throw AdvancedAlertError.rateLimited(retryAfterSeconds: hint)
                default: throw AdvancedAlertError.unavailable(retryAfterSeconds: hint)
                }
            default: throw AdvancedAlertError.unavailable(retryAfterSeconds: nil)
            }
        }
    }
}
