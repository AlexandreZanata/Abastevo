import Foundation

public enum EnterpriseMissionState: String, Codable, Sendable {
    case draft = "DRAFT"
    case planned = "PLANNED"
    case assigned = "ASSIGNED"
    case accepted = "ACCEPTED"
    case declined = "DECLINED"
    case inProgress = "IN_PROGRESS"
    case completed = "COMPLETED"
    case cancelled = "CANCELLED"
}

public enum EnterpriseMissionResponse: String, Codable, Sendable {
    case accept = "ACCEPT"
    case decline = "DECLINE"
}

public enum EnterpriseMissionLoad: String, Codable, Sendable {
    case none = "NONE"
    case cargo = "CARGO"
    case passengers = "PASSENGERS"
}

public enum EnterpriseMissionError: Error, Equatable, Sendable {
    case invalidPage
    case foreignBinding
    case notAnswerable
    case alreadyAnswered
    case notAccepted
    case wrongAsset
    case alreadyStarted
}

public struct EnterpriseMissionPlan: Codable, Equatable, Sendable {
    public let destinationText: String?
    public let destinationUnitId: String?
    public let reason: String
    public let load: EnterpriseMissionLoad
    public let cargo: String?
    public let passengers: Int64?
}

public struct EnterpriseMissionItem: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let unitId: String
    public let assetId: String?
    public let employeeId: String
    public let title: String
    public let startsAt: String
    public let endsAt: String
    public let state: EnterpriseMissionState
    public let revision: Int64
    public let answer: EnterpriseMissionResponse?
    public let plan: EnterpriseMissionPlan?
}

public struct EnterpriseMissionPage: Codable, Equatable, Sendable {
    public let items: [EnterpriseMissionItem]
    public let truncated: Bool
}

public enum EnterpriseMissionWire {
    public static let pageSize = 100

    public static func page(_ body: [String: Any], employeeId: String) throws -> EnterpriseMissionPage {
        guard EnterpriseValues.isId(employeeId), hasKeys(body, required: ["items", "next_cursor", "data_mode"]),
              body["data_mode"] as? String == "SYNTHETIC",
              let cursor = body["next_cursor"] as? String, cursor.utf8.count <= 4096,
              cursor.utf8.allSatisfy({ (0x21...0x7E).contains($0) }),
              let rows = body["items"] as? [[String: Any]], rows.count <= pageSize else {
            throw EnterpriseMissionError.invalidPage
        }
        let items = try rows.map { try item($0, employeeId: employeeId) }
        guard Set(items.map(\.id)).count == items.count else { throw EnterpriseMissionError.invalidPage }
        return EnterpriseMissionPage(items: items, truncated: !cursor.isEmpty)
    }

    private static func item(_ row: [String: Any], employeeId: String) throws -> EnterpriseMissionItem {
        guard hasKeys(
            row, required: ["mission_id", "unit_id", "title", "starts_at", "ends_at", "state", "revision", "created_at", "updated_at"],
            optional: ["asset_id", "assignee_employee_id", "answer", "plan"]
        ), let id = row["mission_id"] as? String, EnterpriseValues.isId(id),
           let unit = row["unit_id"] as? String, EnterpriseValues.isId(unit),
           let title = row["title"] as? String, validText(title),
           let starts = row["starts_at"] as? String, EnterpriseValues.isCanonicalInstant(starts),
           let ends = row["ends_at"] as? String, EnterpriseValues.isCanonicalInstant(ends),
           sortInstant(starts) < sortInstant(ends),
           let created = row["created_at"] as? String, EnterpriseValues.isCanonicalInstant(created),
           let updated = row["updated_at"] as? String, EnterpriseValues.isCanonicalInstant(updated),
           let rawState = row["state"] as? String, let state = EnterpriseMissionState(rawValue: rawState),
           let revision = counter(row["revision"]), revision >= 1 else {
            throw EnterpriseMissionError.invalidPage
        }
        guard row["assignee_employee_id"] as? String == employeeId else { throw EnterpriseMissionError.foreignBinding }
        let asset = try optionalId(row, key: "asset_id")
        let answer: EnterpriseMissionResponse?
        if let value = row["answer"] {
            guard let object = value as? [String: Any], hasKeys(object, required: ["response"], optional: ["reason"]),
                  let raw = object["response"] as? String, let response = EnterpriseMissionResponse(rawValue: raw) else {
                throw EnterpriseMissionError.invalidPage
            }
            if let value = object["reason"] {
                guard response == .decline, let text = value as? String, validText(text) else {
                    throw EnterpriseMissionError.invalidPage
                }
            }
            answer = response
        } else {
            answer = nil
        }
        guard state != .assigned || answer == nil, state != .accepted || answer == .accept,
              state != .declined || answer == .decline else { throw EnterpriseMissionError.invalidPage }
        let planned = try row["plan"].map { try plan($0) }
        return EnterpriseMissionItem(
            id: id, unitId: unit, assetId: asset, employeeId: employeeId, title: title, startsAt: starts, endsAt: ends,
            state: state, revision: revision, answer: answer, plan: planned
        )
    }

    private static func plan(_ value: Any) throws -> EnterpriseMissionPlan {
        guard let object = value as? [String: Any],
              hasKeys(object, required: ["revision", "destination", "reason", "load", "planned_at"], optional: ["cost_centre_id", "stops"]),
              let revision = counter(object["revision"]), revision >= 2,
              let planned = object["planned_at"] as? String, EnterpriseValues.isCanonicalInstant(planned),
              let reason = object["reason"] as? String, validPlanText(reason),
              let dest = object["destination"] as? [String: Any],
              let rawKind = dest["kind"] as? String,
              let load = object["load"] as? [String: Any], let rawLoad = load["kind"] as? String,
              let kind = EnterpriseMissionLoad(rawValue: rawLoad) else { throw EnterpriseMissionError.invalidPage }
        _ = try optionalId(object, key: "cost_centre_id")
        if let value = object["stops"] {
            guard let stops = value as? [[String: Any]], (1...20).contains(stops.count),
                  stops.enumerated().allSatisfy({ offset, stop in
                      hasKeys(stop, required: ["sequence", "kind", "text"]) && counter(stop["sequence"]) == Int64(offset + 1)
                          && stop["kind"] as? String == "TEXT" && (stop["text"] as? String).map(validPlanText) == true
                  }) else { throw EnterpriseMissionError.invalidPage }
        }
        let text: String?
        let unit: String?
        switch rawKind {
        case "TEXT":
            guard hasKeys(dest, required: ["kind", "text"]), let destination = dest["text"] as? String,
                  validPlanText(destination) else { throw EnterpriseMissionError.invalidPage }
            text = destination
            unit = nil
        case "UNIT":
            guard hasKeys(dest, required: ["kind", "unit_id"]), let id = dest["unit_id"] as? String,
                  EnterpriseValues.isId(id) else { throw EnterpriseMissionError.invalidPage }
            text = nil
            unit = id
        default:
            throw EnterpriseMissionError.invalidPage
        }
        switch kind {
        case .none:
            guard hasKeys(load, required: ["kind"]) else { throw EnterpriseMissionError.invalidPage }
            return EnterpriseMissionPlan(destinationText: text, destinationUnitId: unit, reason: reason, load: kind, cargo: nil, passengers: nil)
        case .cargo:
            guard hasKeys(load, required: ["kind", "description"]), let cargo = load["description"] as? String,
                  validPlanText(cargo) else { throw EnterpriseMissionError.invalidPage }
            return EnterpriseMissionPlan(destinationText: text, destinationUnitId: unit, reason: reason, load: kind, cargo: cargo, passengers: nil)
        case .passengers:
            guard hasKeys(load, required: ["kind", "count"]), let count = counter(load["count"]),
                  (1...999).contains(count) else { throw EnterpriseMissionError.invalidPage }
            return EnterpriseMissionPlan(destinationText: text, destinationUnitId: unit, reason: reason, load: kind, cargo: nil, passengers: count)
        }
    }

    static func validText(_ text: String) -> Bool {
        EnterpriseValues.isValidText(text, maxScalars: EnterpriseValues.maxReasonScalars)
    }

    private static func validPlanText(_ text: String) -> Bool {
        validText(text) && text.range(
            of: #"(^|[^0-9A-Za-z])([0-9]{11}|[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2})([^0-9A-Za-z]|$)"#,
            options: .regularExpression
        ) == nil
    }

    fileprivate static func sortInstant(_ text: String) -> String {
        let fraction = text.dropFirst(19).dropLast().dropFirst()
        return String(text.prefix(19)) + String(fraction).padding(toLength: 9, withPad: "0", startingAt: 0)
    }

    private static func optionalId(_ body: [String: Any], key: String) throws -> String? {
        guard let value = body[key] else { return nil }
        guard let id = value as? String, EnterpriseValues.isId(id) else { throw EnterpriseMissionError.invalidPage }
        return id
    }

    private static func counter(_ value: Any?) -> Int64? {
        guard let number = value as? NSNumber, !["d", "f"].contains(String(cString: number.objCType)) else { return nil }
        return EnterpriseCompanyWire.integer(value)
    }

    private static func hasKeys(_ value: [String: Any], required: Set<String>, optional: Set<String> = []) -> Bool {
        let keys = Set(value.keys)
        return keys.isSuperset(of: required) && keys.isSubset(of: required.union(optional))
    }
}

public struct EnterpriseMissionClock: Codable, Equatable, Sendable {
    public let processID: UUID
    public let elapsedMillis: Int64
    public let wallMillis: Int64

    public init(processID: UUID, elapsedMillis: Int64, wallMillis: Int64) {
        self.processID = processID
        self.elapsedMillis = elapsedMillis
        self.wallMillis = wallMillis
    }

    public func age(of receipt: EnterpriseMissionClock) -> Int64? {
        let current = processID == receipt.processID ? elapsedMillis : wallMillis
        let origin = processID == receipt.processID ? receipt.elapsedMillis : receipt.wallMillis
        guard current >= origin, origin >= 0 else { return nil }
        return current - origin
    }
}

public struct EnterpriseMissionAnswerLine: Codable, Equatable, Sendable {
    public let missionId: String
    public let operationId: String
    public let revision: Int64
    public let response: EnterpriseMissionResponse
}

public enum EnterpriseMissionAnswerStatus: Equatable, Sendable {
    case none, saved, accepted, declined, refused, changed, missing, outsidePage
}

public struct EnterpriseMissionRow: Equatable, Identifiable, Sendable {
    public let id: String
    public let item: EnterpriseMissionItem?
    public let state: EnterpriseMissionState?
    public let response: EnterpriseMissionAnswerStatus
    public let refusalCode: String?
    public let queuedResponse: EnterpriseMissionResponse?
    public let canAnswer: Bool
    public let tripId: String?

    public var canStartTrip: Bool { state == .accepted && tripId == nil }
}

public struct EnterpriseMissionInbox: Codable, Equatable, Sendable {
    public static let staleAfterMillis = EnterpriseReferenceCache.confirmationMillis

    public let membershipId: String
    public let enrollmentId: String
    public let employeeId: String
    public private(set) var page: EnterpriseMissionPage
    public private(set) var received: EnterpriseMissionClock
    public private(set) var answers: [EnterpriseMissionAnswerLine] = []

    public init(membershipId: String, enrollmentId: String, employeeId: String, page: EnterpriseMissionPage, received: EnterpriseMissionClock) throws {
        guard [membershipId, enrollmentId, employeeId].allSatisfy(EnterpriseValues.isId),
              page.items.allSatisfy({ $0.employeeId == employeeId }) else { throw EnterpriseMissionError.foreignBinding }
        self.membershipId = membershipId
        self.enrollmentId = enrollmentId
        self.employeeId = employeeId
        self.page = page
        self.received = received
    }

    public func matches(membershipId: String, enrollmentId: String, employeeId: String) -> Bool {
        self.membershipId == membershipId && self.enrollmentId == enrollmentId && self.employeeId == employeeId
    }

    public mutating func refresh(_ next: EnterpriseMissionPage, received: EnterpriseMissionClock) throws {
        guard next.items.allSatisfy({ $0.employeeId == employeeId }) else { throw EnterpriseMissionError.foreignBinding }
        page = next
        self.received = received
    }

    public mutating func addAnswer(missionId: String, operationId: String, revision: Int64, response: EnterpriseMissionResponse) throws {
        guard let item = page.items.first(where: { $0.id == missionId }), item.state == .assigned, item.revision == revision,
              EnterpriseValues.isId(operationId) else { throw EnterpriseMissionError.notAnswerable }
        guard !answers.contains(where: { $0.operationId == operationId || ($0.missionId == missionId && $0.revision == revision) }),
              answers.count < 1000 else { throw EnterpriseMissionError.alreadyAnswered }
        answers.append(EnterpriseMissionAnswerLine(missionId: missionId, operationId: operationId, revision: revision, response: response))
    }

    public func rows(queue: [EnterpriseQueueItem], trips: [EnterpriseTripRecord]) -> [EnterpriseMissionRow] {
        let items = page.items.sorted {
            let left = EnterpriseMissionWire.sortInstant($0.startsAt)
            let right = EnterpriseMissionWire.sortInstant($1.startsAt)
            return left == right ? $0.id < $1.id : left < right
        }
        var rows = items.map { row(id: $0.id, item: $0, queue: queue, trips: trips) }
        var listed = Set(items.map(\.id))
        for line in answers.reversed() where listed.insert(line.missionId).inserted {
            let old = row(id: line.missionId, item: nil, queue: queue, trips: trips)
            if old.response != .accepted && old.response != .declined { rows.append(old) }
        }
        return rows
    }

    public func readyTrip(missionId: String, assetId: String, queue: [EnterpriseQueueItem], trips: [EnterpriseTripRecord]) throws {
        let mission = row(id: missionId, item: page.items.first { $0.id == missionId }, queue: queue, trips: trips)
        guard mission.tripId == nil else { throw EnterpriseMissionError.alreadyStarted }
        guard mission.canStartTrip, let item = mission.item else { throw EnterpriseMissionError.notAccepted }
        if let assigned = item.assetId, assigned != assetId { throw EnterpriseMissionError.wrongAsset }
    }

    public mutating func releaseDismissedAnswer(_ operationId: String) -> Bool {
        guard let index = answers.firstIndex(where: { $0.operationId == operationId }) else { return true }
        let answer = answers[index]
        if let item = page.items.first(where: { $0.id == answer.missionId }) {
            guard item.revision > answer.revision else { return false }
        } else if page.truncated {
            return false
        }
        answers.remove(at: index)
        return true
    }

    private func row(
        id: String, item: EnterpriseMissionItem?, queue: [EnterpriseQueueItem], trips: [EnterpriseTripRecord]
    ) -> EnterpriseMissionRow {
        let answered = answers.last { $0.missionId == id }
        let entry = answered.flatMap { line in queue.first { $0.id == line.operationId && $0.kind == .missionResponse } }
        let status: EnterpriseMissionAnswerStatus
        var state = item?.state
        if let answered {
            switch entry?.status {
            case .accepted:
                status = answered.response == .accept ? .accepted : .declined
                if let item, let revision = entry?.entityRevision, revision > answered.revision, revision >= item.revision {
                    state = answered.response == .accept ? .accepted : .declined
                }
            case .rejected, .inReview, nil:
                status = .refused
            default:
                if let item {
                    status = item.revision > answered.revision ? .changed : .saved
                } else {
                    status = page.truncated ? .outsidePage : .missing
                }
            }
        } else {
            status = .none
        }
        let trip = trips.last { trip in
            trip.missionId == id && queue.first { $0.id == trip.startOperationId }?.status != .rejected
        }
        let canAnswer = state == .assigned && item != nil && item?.revision != answered?.revision
        return EnterpriseMissionRow(
            id: id, item: item, state: state, response: status, refusalCode: entry?.lastCode,
            queuedResponse: answered?.response, canAnswer: canAnswer, tripId: trip?.id
        )
    }
}

public extension EnterpriseOperationBuilder {
    static let missionRespond = "trip.mission.respond"

    static func respondToMission(
        envelope: EnterpriseEnvelope, missionId: String, revision: Int64, response: EnterpriseMissionResponse, reason: String?
    ) throws -> EnterpriseSealedOperation {
        switch response {
        case .accept:
            guard reason == nil else { throw EnterpriseOperationError.malformed }
        case .decline:
            guard let reason, EnterpriseMissionWire.validText(reason) else { throw EnterpriseOperationError.malformed }
        }
        return try seal(
            envelope: envelope, type: missionRespond, entityType: "mission", entityId: missionId, expectedRevision: revision,
            dependsOn: [], softReferences: [], payload: ["response": .text(response.rawValue), "reason": reason.map(EnterpriseJson.text) ?? .null]
        )
    }
}
