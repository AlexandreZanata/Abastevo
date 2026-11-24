import Foundation

public struct QualitySelection: Equatable, Sendable {
    public let stationId: String
    public let fuel: FuelProduct
    public let revisionId: String?

    public init(stationId: String, fuel: FuelProduct, revisionId: String? = nil) {
        self.stationId = stationId
        self.fuel = fuel
        self.revisionId = revisionId
    }

    func query() throws -> [URLQueryItem] {
        guard QualityWire.identifier(stationId), revisionId.map(QualityWire.identifier) ?? true else {
            throw QualityReadError.invalid
        }
        return QualityWire.query([("station_id", stationId), ("fuel", fuel.wireCode), ("revision_id", revisionId)])
    }
}

public struct QualityLease: Decodable, Equatable, Sendable {
    public let source: String
    public let stream: String
    public let edition: String
    public let revisionId: String?
    public let publishedAt: String?
    public let available: Bool
    public let stale: Bool
    public let revisionStatus: String
    public let provenanceState: String?
    public let isCurrentStream: Bool?
    public let coverageNote: String?

    func validate(source: String, stream: String, revision: String?) throws {
        guard self.source == source, self.stream == stream, edition.utf8.count <= 8_192,
              revision.map({ $0 == revisionId }) ?? true, revisionId.map(QualityWire.identifier) ?? true,
              publishedAt.map({ AbastevoHttpClient.readIsoDate($0) != nil }) ?? true,
              !available || (revisionId != nil && publishedAt != nil
                && ["published", "superseded"].contains(revisionStatus) && provenanceState == "VERIFIED") else {
            throw QualityReadError.invalid
        }
    }
}

public struct QualityAssayCell: Decodable, Equatable, Sendable {
    public let fuelProduct: String
    public let conformity: String
    public let count: Int64
    public let lastObserved: String
}

public struct QualityDimensions: Decodable, Equatable, Sendable {
    public let fuelProduct: String
    public let samples: Int64
    public let visits: Int64
    public let conforming: Int64
    public let nonconforming: Int64
    public let unresolved: Int64
    public let oldestObservedOn: String?
    public let newestObservedOn: String?
    public let scoreState: String

    private enum CodingKeys: String, CodingKey {
        case fuelProduct, samples, visits, conforming, nonconforming, unresolved, oldestObservedOn, newestObservedOn, scoreState, score
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        guard values.contains(.score), try values.decodeNil(forKey: .score) else { throw QualityReadError.invalid }
        fuelProduct = try values.decode(String.self, forKey: .fuelProduct)
        samples = try values.decode(Int64.self, forKey: .samples)
        visits = try values.decode(Int64.self, forKey: .visits)
        conforming = try values.decode(Int64.self, forKey: .conforming)
        nonconforming = try values.decode(Int64.self, forKey: .nonconforming)
        unresolved = try values.decode(Int64.self, forKey: .unresolved)
        oldestObservedOn = try values.decodeIfPresent(String.self, forKey: .oldestObservedOn)
        newestObservedOn = try values.decodeIfPresent(String.self, forKey: .newestObservedOn)
        scoreState = try values.decode(String.self, forKey: .scoreState)
        guard samples >= 0, visits >= 0, visits <= samples, conforming >= 0, conforming <= samples,
              nonconforming >= 0, nonconforming <= samples - conforming,
              unresolved == samples - conforming - nonconforming,
              QualityWire.optionalDay(oldestObservedOn), QualityWire.optionalDay(newestObservedOn) else {
            throw QualityReadError.invalid
        }
    }
}

public struct QualitySummary: Equatable, Sendable {
    public let lease: QualityLease
    public let cells: [QualityAssayCell]
    public let sampleDimensions: [QualityDimensions]?
    public let stale: Bool
}

public struct QualityAssay: Decodable, Equatable, Identifiable, Sendable {
    public let id: String
    public let sampleRef: String
    public let assayRef: String
    public let observedOn: String
    public let fuelProduct: String
    public let resultText: String
    public let unit: String
    public let conformity: String
    public let sourceObservedAt: String?

    func validate(fuel: FuelProduct, sampleRef: String? = nil) throws {
        guard QualityWire.identifier(id), QualityWire.validSampleReference(self.sampleRef),
              sampleRef.map({ $0 == self.sampleRef }) ?? true, fuelProduct == fuel.wireCode,
              QualityWire.day(observedOn), assayRef.utf8.count <= 8_192, resultText.utf8.count <= 8_192,
              unit.utf8.count <= 8_192 else {
            throw QualityReadError.invalid
        }
    }
}

public struct QualityRevision: Decodable, Equatable, Identifiable, Sendable {
    public var id: String { revisionId }
    public let revisionId: String
    public let edition: String
    public let revisionStatus: String
    public let provenanceState: String
    public let isCurrentStream: Bool

    public var isSelectable: Bool {
        provenanceState == "VERIFIED" && ["published", "superseded"].contains(revisionStatus)
    }
}

public struct QualityResource: Decodable, Equatable, Identifiable, Sendable {
    public var id: String { url.absoluteString }
    public let url: URL
    public let sha256: String
    public let memberName: String?

    func validate() throws {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false), parts.scheme == "https",
              parts.host == "www.gov.br", parts.port == nil || parts.port == 443,
              parts.user == nil, parts.password == nil, parts.query == nil, parts.fragment == nil,
              parts.percentEncodedPath.hasPrefix("/anp/"),
              sha256.range(of: #"^[a-f0-9]{64}$"#, options: .regularExpression) != nil else {
            throw QualityReadError.invalid
        }
    }
}

public struct QualityPage<Value: Equatable & Sendable>: Equatable, Sendable {
    public let items: [Value]
    public let nextCursor: String?
    public let lease: QualityLease?
    public let stale: Bool
}

public struct QualitySample: Equatable, Sendable {
    public let sampleRef: String
    public let assays: [QualityAssay]
    public let lease: QualityLease
    public let stale: Bool
}

public struct QualityProvenance: Equatable, Sendable {
    public let lease: QualityLease
    public let resources: [QualityResource]
    public let stale: Bool
}

public enum QualityWire {
    static let assaySource = "pmqc"
    static let assayStream = "pmqc-assay-v1"
    static let occurrenceSource = "enforcement"
    static let occurrenceStream = "enforcement-v1"

    static func identifier(_ value: String) -> Bool {
        CommunityWire.isUuid(value) && value == value.lowercased()
    }

    static func day(_ value: String) -> Bool {
        value.utf8.count == 10 && AnpCalendar.parseIsoDay(value) != nil
    }

    static func optionalDay(_ value: String?) -> Bool {
        value.map { $0.isEmpty || day($0) } ?? true
    }

    static func validSampleReference(_ value: String) -> Bool {
        !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && value.utf8.count <= 128
            && !value.unicodeScalars.contains(where: CharacterSet.controlCharacters.contains)
    }

    static func query(_ fields: [(String, String?)]) -> [URLQueryItem] {
        fields.compactMap { name, value in value.map { URLQueryItem(name: name, value: $0) } }
    }

    static func pagedQuery(_ limit: Int, cursor: String?) throws -> [URLQueryItem] {
        guard (1...100).contains(limit), validCursor(cursor, previous: nil) else { throw QualityReadError.invalid }
        return query([("limit", String(limit)), ("cursor", cursor)])
    }

    static func validCursor(_ cursor: String?, previous: String?) -> Bool {
        guard let cursor else { return true }
        return cursor != previous && cursor.utf8.count <= 8_192
            && !cursor.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    private struct SummaryWire: Decodable {
        struct Dimensions: Decodable {
            let available: Bool
            let items: [QualityDimensions]
        }

        let stationId: String
        let lease: QualityLease
        let fuelCells: [QualityAssayCell]
        let sampleDimensions: Dimensions?
    }

    private struct AssaysWire: Decodable {
        let stationId: String
        let lease: QualityLease
        let items: [QualityAssay]
        let nextCursor: String?
    }

    private struct RevisionsWire: Decodable {
        let source: String
        let stream: String
        let items: [QualityRevision]
        let nextCursor: String?
    }

    private struct SampleWire: Decodable {
        let sampleRef: String
        let lease: QualityLease
        let assays: [QualityAssay]
    }

    private struct ProvenanceWire: Decodable {
        let lease: QualityLease
        let resources: [QualityResource]
    }

    static func summary(_ data: Data, selection: QualitySelection, stale: Bool) throws -> QualitySummary {
        let row = try AbastevoHttpClient.publicDecoder().decode(SummaryWire.self, from: data)
        try row.lease.validate(source: assaySource, stream: assayStream, revision: selection.revisionId)
        let dimensions = row.sampleDimensions?.items ?? []
        guard row.stationId == selection.stationId, row.fuelCells.count <= 32, dimensions.count <= 8,
              row.fuelCells.allSatisfy({ $0.fuelProduct == selection.fuel.wireCode && $0.count >= 0 && day($0.lastObserved) }),
              dimensions.allSatisfy({ $0.fuelProduct == selection.fuel.wireCode }),
              (row.sampleDimensions?.available ?? false) == !dimensions.isEmpty,
              row.lease.available || (row.fuelCells.isEmpty && dimensions.isEmpty) else {
            throw QualityReadError.invalid
        }
        return QualitySummary(lease: row.lease, cells: row.fuelCells, sampleDimensions: dimensions.isEmpty ? nil : dimensions,
                              stale: stale || row.lease.stale)
    }

    static func assays(_ data: Data, selection: QualitySelection, cursor: String?, stale: Bool) throws -> QualityPage<QualityAssay> {
        let row = try AbastevoHttpClient.publicDecoder().decode(AssaysWire.self, from: data)
        try row.lease.validate(source: assaySource, stream: assayStream, revision: selection.revisionId)
        guard row.stationId == selection.stationId, row.items.count <= 50, validCursor(row.nextCursor, previous: cursor),
              Set(row.items.map(\.id)).count == row.items.count,
              row.lease.available || (row.items.isEmpty && row.nextCursor == nil) else {
            throw QualityReadError.invalid
        }
        for assay in row.items { try assay.validate(fuel: selection.fuel) }
        return QualityPage(items: row.items, nextCursor: row.nextCursor, lease: row.lease, stale: stale || row.lease.stale)
    }

    static func revisions(_ data: Data, cursor: String?, stale: Bool) throws -> QualityPage<QualityRevision> {
        let row = try AbastevoHttpClient.publicDecoder().decode(RevisionsWire.self, from: data)
        guard row.source == assaySource, row.stream == assayStream, row.items.count <= 20,
              row.items.allSatisfy({ identifier($0.id) }), Set(row.items.map(\.id)).count == row.items.count,
              validCursor(row.nextCursor, previous: cursor) else { throw QualityReadError.invalid }
        return QualityPage(items: row.items, nextCursor: row.nextCursor, lease: nil, stale: stale)
    }

    static func sample(_ data: Data, ref: String, revision: String, fuel: FuelProduct, stale: Bool) throws -> QualitySample {
        let row = try AbastevoHttpClient.publicDecoder().decode(SampleWire.self, from: data)
        try row.lease.validate(source: assaySource, stream: assayStream, revision: revision)
        guard row.sampleRef == ref, row.lease.available, !row.assays.isEmpty, row.assays.count <= 1_000,
              Set(row.assays.map(\.id)).count == row.assays.count else { throw QualityReadError.invalid }
        for assay in row.assays { try assay.validate(fuel: fuel, sampleRef: ref) }
        return QualitySample(sampleRef: ref, assays: row.assays, lease: row.lease, stale: stale || row.lease.stale)
    }

    static func provenance(_ data: Data, revision: String, stale: Bool) throws -> QualityProvenance {
        let row = try AbastevoHttpClient.publicDecoder().decode(ProvenanceWire.self, from: data)
        try row.lease.validate(source: assaySource, stream: assayStream, revision: revision)
        guard row.resources.count <= 32, !row.lease.available || !row.resources.isEmpty else { throw QualityReadError.invalid }
        for resource in row.resources { try resource.validate() }
        return QualityProvenance(lease: row.lease, resources: row.resources, stale: stale || row.lease.stale)
    }
}

public extension QualityPublicReader {
    func summary(_ selection: QualitySelection) async throws -> QualitySummary {
        try await read("summary", query: selection.query()) { try QualityWire.summary($0, selection: selection, stale: $1) }
    }

    func assays(_ selection: QualitySelection, cursor: String? = nil) async throws -> QualityPage<QualityAssay> {
        guard selection.revisionId != nil else { throw QualityReadError.invalid }
        let query = try selection.query() + QualityWire.pagedQuery(50, cursor: cursor)
        return try await read("assays", query: query) { try QualityWire.assays($0, selection: selection, cursor: cursor, stale: $1) }
    }

    func revisions(cursor: String? = nil) async throws -> QualityPage<QualityRevision> {
        let query = try QualityWire.query([("stream", QualityWire.assayStream)]) + QualityWire.pagedQuery(20, cursor: cursor)
        return try await read("revisions", query: query) { try QualityWire.revisions($0, cursor: cursor, stale: $1) }
    }

    func sample(ref: String, revision: String, fuel: FuelProduct) async throws -> QualitySample {
        guard QualityWire.identifier(revision) else { throw QualityReadError.invalid }
        return try await read("samples", query: QualityWire.query([("revision_id", revision)]), detail: ref) {
            try QualityWire.sample($0, ref: ref, revision: revision, fuel: fuel, stale: $1)
        }
    }

    func provenance(revision: String) async throws -> QualityProvenance {
        guard QualityWire.identifier(revision) else { throw QualityReadError.invalid }
        return try await read("revisions", query: QualityWire.query([("stream", QualityWire.assayStream)]), detail: revision) {
            try QualityWire.provenance($0, revision: revision, stale: $1)
        }
    }
}
