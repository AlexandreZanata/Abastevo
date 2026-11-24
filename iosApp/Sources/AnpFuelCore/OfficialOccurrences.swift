import Foundation

public enum OccurrenceSubject: String, CaseIterable, Identifiable, Sendable {
    case company = "company-shaped"
    case personal = "personal-shaped"
    case other

    public var id: String { rawValue }
}

public struct OccurrenceFilter: Equatable, Sendable {
    public var state: BrazilianState?
    public var municipality: String?
    public var segment: String?
    public var subject: OccurrenceSubject?
    public var fromDay: String?
    public var toDay: String?

    public init(
        state: BrazilianState? = nil, municipality: String? = nil, segment: String? = nil,
        subject: OccurrenceSubject? = nil, fromDay: String? = nil, toDay: String? = nil
    ) {
        self.state = state
        self.municipality = municipality
        self.segment = segment
        self.subject = subject
        self.fromDay = fromDay
        self.toDay = toDay
    }

    func query(stationList: Bool) throws -> [URLQueryItem] {
        guard fromDay.map(QualityWire.day) ?? true, toDay.map(QualityWire.day) ?? true,
              [municipality, segment].allSatisfy({ value in
                  value.map { $0.utf8.count <= 256 && !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty } ?? true
              }) else { throw QualityReadError.invalid }
        if let fromDay, let toDay, fromDay > toDay { throw QualityReadError.invalid }
        if stationList && (municipality != nil || subject != nil) { throw QualityReadError.invalid }
        return QualityWire.query([
            (stationList ? "state" : "uf", state?.rawValue), ("municipality", municipality),
            ("segment", segment), ("subject_kind", subject?.rawValue), ("from_day", fromDay), ("to_day", toDay),
        ])
    }
}

public struct OfficialOccurrence: Decodable, Equatable, Identifiable, Sendable {
    public let id: String
    public let nativeKey: String
    public let sourceProfile: String
    public let subjectKind: String
    public let uf: String?
    public let municipality: String?
    public let segment: String?
    public let documentDay: String
    public let dateBasis: String
    public let documentNumber: String
    public let procedure: String
    public let result: String
    public let evidenceRef: String

    public var isCompanyShaped: Bool { subjectKind == OccurrenceSubject.company.rawValue }

    func validate() throws {
        guard QualityWire.identifier(id), sourceProfile == "anp-enforcement-xlsx-2019-v1",
              QualityWire.day(documentDay), dateBasis == "document_day",
              OccurrenceSubject(rawValue: subjectKind) != nil, !nativeKey.isEmpty,
              [documentNumber, procedure, result, evidenceRef, municipality ?? "", segment ?? ""].allSatisfy({
                  $0.utf8.count <= 65_536
              }) else { throw QualityReadError.invalid }
    }
}

public struct OccurrenceStation: Decodable, Equatable, Identifiable, Sendable {
    public let id: String
    public let displayName: String
    public let municipality: String?
    public let state: String?
    public let status: String
    public let recordCount: Int64
}

public struct OccurrenceCoverage: Decodable, Equatable, Sendable {
    public let observations: Int64
    public let cases: Int64
    public let firstDay: String?
    public let lastDay: String?
    public let dateBasis: String
    public let lease: QualityLease
}

public struct OccurrenceStationCoverage: Decodable, Equatable, Sendable {
    public let stations: Int64
    public let records: Int64
    public let lease: QualityLease
}

public struct OfficialOccurrenceDetail: Equatable, Sendable {
    public let record: OfficialOccurrence
    public let lease: QualityLease
    public let stale: Bool
}

enum OccurrenceWire {
    private struct Page<Value: Decodable>: Decodable {
        let items: [Value]
        let nextCursor: String?
        let lease: QualityLease
    }

    private struct DetailLease: Decodable {
        let lease: QualityLease
    }

    private static let allowedKeys: Set<String> = [
        "id", "native_key", "scope", "status", "occurred_on", "as_of", "source_profile", "subject_kind",
        "uf", "municipality", "segment", "document_day", "date_basis", "document_number",
        "procedure", "result", "evidence_ref", "lease", "generated_at",
    ]

    private static func admitted(_ fields: [String: Any]) -> Bool {
        Set(fields.keys).isSubset(of: allowedKeys)
    }

    static func validate(_ lease: QualityLease, revision: String?) throws {
        try lease.validate(source: QualityWire.occurrenceSource, stream: QualityWire.occurrenceStream, revision: revision)
    }

    static func actions(_ data: Data, revision: String?, cursor: String?, stale: Bool) throws -> QualityPage<OfficialOccurrence> {
        let page = try AbastevoHttpClient.publicDecoder().decode(Page<OfficialOccurrence>.self, from: data)
        try validate(page.lease, revision: revision)
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              let raw = object["items"] as? [[String: Any]], raw.allSatisfy(admitted),
              page.items.count <= 20, Set(page.items.map(\.id)).count == page.items.count,
              QualityWire.validCursor(page.nextCursor, previous: cursor),
              page.lease.available || (page.items.isEmpty && page.nextCursor == nil) else {
            throw QualityReadError.invalid
        }
        for row in page.items { try row.validate() }
        return QualityPage(items: page.items, nextCursor: page.nextCursor, lease: page.lease, stale: stale || page.lease.stale)
    }

    static func record(_ data: Data) throws -> OfficialOccurrence {
        guard let raw = try JSONSerialization.jsonObject(with: data) as? [String: Any], admitted(raw) else {
            throw QualityReadError.invalid
        }
        let item = try AbastevoHttpClient.publicDecoder().decode(OfficialOccurrence.self, from: data)
        try item.validate()
        return item
    }

    static func detail(_ data: Data, id: String, revision: String?, stale: Bool) throws -> OfficialOccurrenceDetail {
        let item = try record(data)
        let lease = try AbastevoHttpClient.publicDecoder().decode(DetailLease.self, from: data).lease
        try validate(lease, revision: revision)
        guard item.id == id, lease.available else { throw QualityReadError.invalid }
        return OfficialOccurrenceDetail(record: item, lease: lease, stale: stale || lease.stale)
    }

    static func stations(_ data: Data, revision: String?, cursor: String?, stale: Bool) throws -> QualityPage<OccurrenceStation> {
        let page = try AbastevoHttpClient.publicDecoder().decode(Page<OccurrenceStation>.self, from: data)
        try validate(page.lease, revision: revision)
        guard page.items.count <= 20, Set(page.items.map(\.id)).count == page.items.count,
              page.items.allSatisfy({ QualityWire.identifier($0.id) && $0.recordCount > 0 && !$0.displayName.isEmpty }),
              QualityWire.validCursor(page.nextCursor, previous: cursor),
              page.lease.available || (page.items.isEmpty && page.nextCursor == nil) else {
            throw QualityReadError.invalid
        }
        return QualityPage(items: page.items, nextCursor: page.nextCursor, lease: page.lease, stale: stale || page.lease.stale)
    }
}

public extension QualityPublicReader {
    func occurrences(
        filter: OccurrenceFilter, revision: String? = nil, cursor: String? = nil
    ) async throws -> QualityPage<OfficialOccurrence> {
        guard revision.map(QualityWire.identifier) ?? true, cursor == nil || revision != nil else {
            throw QualityReadError.invalid
        }
        let query = try filter.query(stationList: false) + QualityWire.pagedQuery(20, cursor: cursor)
            + QualityWire.query([("revision_id", revision)])
        return try await read("actions", query: query) { try OccurrenceWire.actions($0, revision: revision, cursor: cursor, stale: $1) }
    }

    func occurrence(id: String, revision: String?) async throws -> OfficialOccurrenceDetail {
        guard QualityWire.identifier(id), revision.map(QualityWire.identifier) ?? true else { throw QualityReadError.invalid }
        return try await read("actions", query: QualityWire.query([("revision_id", revision)]), detail: id) {
            try OccurrenceWire.detail($0, id: id, revision: revision, stale: $1)
        }
    }

    func occurrenceCoverage(revision: String?) async throws -> OccurrenceCoverage {
        guard revision.map(QualityWire.identifier) ?? true else { throw QualityReadError.invalid }
        return try await read("actions/coverage", query: QualityWire.query([("revision_id", revision)])) { data, _ in
            let value = try AbastevoHttpClient.publicDecoder().decode(OccurrenceCoverage.self, from: data)
            try OccurrenceWire.validate(value.lease, revision: revision)
            guard value.dateBasis == "document_day", value.cases >= 0, value.observations >= value.cases,
                  QualityWire.optionalDay(value.firstDay), QualityWire.optionalDay(value.lastDay) else {
                throw QualityReadError.invalid
            }
            return value
        }
    }

    func occurrenceStations(
        filter: OccurrenceFilter, revision: String? = nil, cursor: String? = nil
    ) async throws -> QualityPage<OccurrenceStation> {
        guard revision.map(QualityWire.identifier) ?? true, cursor == nil || revision != nil else {
            throw QualityReadError.invalid
        }
        let query = try filter.query(stationList: true) + QualityWire.pagedQuery(20, cursor: cursor)
            + QualityWire.query([("revision_id", revision)])
        return try await read("enforcement/stations", query: query) {
            try OccurrenceWire.stations($0, revision: revision, cursor: cursor, stale: $1)
        }
    }

    func occurrenceStationCoverage(revision: String?) async throws -> OccurrenceStationCoverage {
        guard revision.map(QualityWire.identifier) ?? true else { throw QualityReadError.invalid }
        return try await read("enforcement/stations/coverage", query: QualityWire.query([("revision_id", revision)])) { data, _ in
            let value = try AbastevoHttpClient.publicDecoder().decode(OccurrenceStationCoverage.self, from: data)
            try OccurrenceWire.validate(value.lease, revision: revision)
            guard value.stations >= 0, value.records >= value.stations else { throw QualityReadError.invalid }
            return value
        }
    }
}
