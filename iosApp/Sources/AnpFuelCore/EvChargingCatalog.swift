import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Admitted public charging catalog (backend `charging` module, EE-09/EE-18/EE-21/EE-26).
/// Swift port of Android `ChargingSnapshot`, `ChargingSnapshotJsonCodec`,
/// `ChargingCompatibility`, `ChargingSnapshotFreshness`, `ChargingMarkersSource`
/// and the `ChargingDiscoveryRepository.refresh` census checks. Availability is
/// never live: the snapshot is dated provider metadata.
/// Zero postfix unwrap operators anywhere in this file.

public enum ChargingConnector: String, CaseIterable, Codable, Identifiable, Sendable {
    case ccs2 = "CCS2"
    case type2 = "TYPE2"
    case chademo = "CHADEMO"
    case nacs = "NACS"
    case unknown = "UNKNOWN"
    case type1 = "TYPE1"
    case ccs1 = "CCS1"
    case gbtAc = "GBT_AC"
    case gbtDc = "GBT_DC"
    case nbr14136 = "NBR14136"
    case teslaLegacy = "TESLA_LEGACY"

    public var id: String { rawValue }

    public static func wire(_ raw: String) -> ChargingConnector {
        ChargingConnector(rawValue: raw) ?? .unknown
    }

    /// Connectors a vehicle can declare for planning (UNKNOWN is never a vehicle connector).
    public static var vehicleChoices: [ChargingConnector] { allCases.filter { $0 != .unknown } }

    public var label: String {
        switch self {
        case .ccs2: return "CCS2"
        case .type2: return String(localized: "ev.connector.type2", bundle: CoreLocalization.bundle)
        case .chademo: return "CHAdeMO"
        case .nacs: return "NACS"
        case .unknown: return String(localized: "ev.connector.unknown", bundle: CoreLocalization.bundle)
        case .type1: return String(localized: "ev.connector.type1", bundle: CoreLocalization.bundle)
        case .ccs1: return "CCS1"
        case .gbtAc: return "GB/T AC"
        case .gbtDc: return "GB/T DC"
        case .nbr14136: return "NBR 14136"
        case .teslaLegacy: return String(localized: "ev.connector.teslaLegacy", bundle: CoreLocalization.bundle)
        }
    }
}

public enum ChargingCurrent: String, CaseIterable, Codable, Sendable {
    case ac = "AC"
    case dc = "DC"
    case unknown = "UNKNOWN"

    public static func wire(_ raw: String) -> ChargingCurrent {
        ChargingCurrent(rawValue: raw) ?? .unknown
    }
}

public enum ChargingAccess: String, Codable, Sendable {
    case publicAccess = "PUBLIC"
    case unknown = "UNKNOWN"

    public static func wire(_ raw: String) -> ChargingAccess {
        ChargingAccess(rawValue: raw) ?? .unknown
    }
}

public enum ChargingCatalogConfidence: String, Codable, Sendable {
    case singleSource = "single-source"
    case twoSource = "two-source"
}

public struct ChargingConnectionGroup: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let entityKey: String
    public let externalId: String
    public let connector: ChargingConnector
    public let rawConnectorCode: String
    public let current: ChargingCurrent
    public let powerW: Int64?
    public let reportedQuantity: Int64?
    public let sourceKey: String?

    public init(
        id: String, entityKey: String, externalId: String, connector: ChargingConnector, rawConnectorCode: String,
        current: ChargingCurrent, powerW: Int64?, reportedQuantity: Int64?, sourceKey: String?
    ) {
        self.id = id
        self.entityKey = entityKey
        self.externalId = externalId
        self.connector = connector
        self.rawConnectorCode = rawConnectorCode
        self.current = current
        self.powerW = powerW
        self.reportedQuantity = reportedQuantity
        self.sourceKey = sourceKey
    }
}

public struct ChargingSourceAttribution: Codable, Equatable, Sendable {
    public let source: String
    public let externalId: String
    public let revision: String
    public let providerId: Int64?
    public let license: String
    public let attribution: String

    public var sourceKey: String { source == "OSM" ? "osm:\(externalId)" : "ocm:poi:\(externalId)" }
}

public struct ChargingPlace: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let entityKey: String
    public let externalId: String
    public let name: String
    public let latitude: String?
    public let longitude: String?
    public let publicAccess: ChargingAccess
    public let reportedPoints: Int64
    public let groups: [ChargingConnectionGroup]
    public let sources: [ChargingSourceAttribution]
    public let confidence: ChargingCatalogConfidence?
    public let uf: String?
    public let linkedStationId: String?
    public let operatorName: String?
    public let addressLine: String?
    public let municipality: String?

    public init(
        id: String, entityKey: String, externalId: String, name: String, latitude: String?, longitude: String?,
        publicAccess: ChargingAccess, reportedPoints: Int64, groups: [ChargingConnectionGroup],
        sources: [ChargingSourceAttribution], confidence: ChargingCatalogConfidence?, uf: String?,
        linkedStationId: String?, operatorName: String?, addressLine: String?, municipality: String?
    ) {
        self.id = id
        self.entityKey = entityKey
        self.externalId = externalId
        self.name = name
        self.latitude = latitude
        self.longitude = longitude
        self.publicAccess = publicAccess
        self.reportedPoints = reportedPoints
        self.groups = groups
        self.sources = sources
        self.confidence = confidence
        self.uf = uf
        self.linkedStationId = linkedStationId
        self.operatorName = operatorName
        self.addressLine = addressLine
        self.municipality = municipality
    }

    /// Admitted coordinates only; a place without them stays list-only (no centroid is invented).
    public var coordinate: EvCoordinate? {
        guard let latitude, let longitude,
              let lat = ChargingWireRules.decimal(latitude, minimum: -90, maximum: 90, maxScale: 16),
              let lon = ChargingWireRules.decimal(longitude, minimum: -180, maximum: 180, maxScale: 16) else {
            return nil
        }
        return EvCoordinate(latitude: lat, longitude: lon)
    }

    public var displayName: String {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmed.isEmpty { return trimmed }
        return operatorName ?? String(localized: "android.charging_place_unnamed", bundle: CoreLocalization.bundle)
    }

    public var maxPowerW: Int64? { groups.compactMap(\.powerW).max() }

    public var connectors: [ChargingConnector] {
        var seen: [ChargingConnector] = []
        for group in groups where !seen.contains(group.connector) { seen.append(group.connector) }
        return seen
    }

    public var confirmedByTwoSources: Bool { confidence == .twoSource }
}

public struct EvCoordinate: Codable, Equatable, Hashable, Sendable {
    public let latitude: Double
    public let longitude: Double

    public init(latitude: Double, longitude: Double) {
        self.latitude = latitude
        self.longitude = longitude
    }
}

public struct ChargingPlacePage: Equatable, Sendable {
    public let items: [ChargingPlace]
    public let nextCursor: String?
    public let generatedAt: String
}

public struct ChargingCoverage: Equatable, Sendable {
    public let profile: String
    public let sourceRevision: String
    public let places: Int64
    public let groups: Int64
    public let vehicles: Int64
    public let generatedAt: String
    public let offerNotice: String?
    public let offerData: String?

    public func sameEdition(as other: ChargingCoverage) -> Bool {
        profile == other.profile && sourceRevision == other.sourceRevision && places == other.places
            && groups == other.groups && vehicles == other.vehicles
            && offerNotice == other.offerNotice && offerData == other.offerData
    }
}

public enum ChargingWireError: Error, Equatable {
    case refused(String)
}

/// Strict EE-09 wire rules: oversize, foreign, private or malformed bodies fail closed.
public enum ChargingWireRules {
    public static let maxRows = 100
    static let privateFields: Set<String> = [
        "reporter", "price", "contributor", "contribution", "session_token", "access_token", "refresh_token", "family_id",
    ]

    public static func isCanonicalId(_ value: String) -> Bool {
        let scalars = Array(value.unicodeScalars)
        guard scalars.count == 36 else { return false }
        for (index, scalar) in scalars.enumerated() {
            if [8, 13, 18, 23].contains(index) {
                guard scalar == "-" else { return false }
            } else if !isLowerHex(scalar) {
                return false
            }
        }
        return true
    }

    public static func isEntityKey(_ value: String) -> Bool {
        value.unicodeScalars.count == 64 && value.unicodeScalars.allSatisfy(isLowerHex)
    }

    static func isLowerHex(_ scalar: Unicode.Scalar) -> Bool {
        (scalar.value >= 48 && scalar.value <= 57) || (scalar.value >= 97 && scalar.value <= 102)
    }

    static func hasControl(_ value: String) -> Bool {
        value.unicodeScalars.contains { $0.properties.generalCategory == .control }
    }

    /// Plain decimal in range with at most `maxScale` significant fraction digits.
    public static func decimal(_ text: String, minimum: Double, maximum: Double, maxScale: Int) -> Double? {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, trimmed == text else { return nil }
        var body = Substring(trimmed)
        if body.hasPrefix("-") || body.hasPrefix("+") { body = body.dropFirst() }
        let parts = body.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count <= 2, AnpCalendar.isAsciiDigits(parts[0]) else { return nil }
        if parts.count == 2 {
            let fraction = parts[1]
            guard AnpCalendar.isAsciiDigits(fraction) else { return nil }
            var significant = fraction
            while significant.hasSuffix("0") { significant = significant.dropLast() }
            guard significant.count <= maxScale else { return nil }
        }
        guard let value = Double(trimmed), value.isFinite, value >= minimum, value <= maximum else { return nil }
        return value
    }

    static func isInstant(_ text: String) -> Bool {
        AbastevoHttpClient.readIsoDate(text) != nil
    }

    static func formatAddress(street: String?, number: String?, district: String?, municipio: String?, uf: String?) -> String? {
        func usable(_ value: String?) -> String? {
            guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty,
                  !hasControl(trimmed), trimmed.lowercased() != "null" else { return nil }
            return trimmed
        }
        let state = usable(uf)?.uppercased()
        let validState = state.flatMap { BrazilianState(rawValue: $0) }?.rawValue
        let locality = usable(municipio).map { city in validState.map { "\(city)/\($0)" } ?? city }
        let streetLine = usable(street).map { street in [street, usable(number)].compactMap { $0 }.joined(separator: ", ") }
        guard let line = streetLine else { return locality }
        let area = [usable(district), locality].compactMap { $0 }.joined(separator: ", ")
        return area.isEmpty ? line : "\(line) – \(area)"
    }
}

private struct ChargingPageWire: Decodable {
    let items: [ChargingPlaceWire]
    let nextCursor: String?
    let generatedAt: String
}

private struct ChargingAddressWire: Decodable {
    let street: String?
    let number: String?
    let complement: String?
    let district: String?
    let municipio: String?
    let postalCode: String?
    let uf: String?
}

private struct ChargingGroupWire: Decodable {
    let id: String
    let entityKey: String
    let externalId: String
    let connectorCode: String
    let rawConnectorCode: String?
    let current: String
    let powerW: Int64?
    let reportedQuantity: Int64?
    let sourceKey: String?
}

private struct ChargingSourceWire: Decodable {
    let source: String
    let externalId: String
    let revision: String
    let providerId: Int64?
    let license: String
    let attribution: String
}

private struct ChargingPlaceWire: Decodable {
    let id: String
    let entityKey: String
    let externalId: String
    let name: String
    let country: String
    let latitude: String?
    let longitude: String?
    let publicAccess: String
    let reportedPoints: Int64
    let groups: [ChargingGroupWire]?
    let sources: [ChargingSourceWire]?
    let confidence: String?
    let uf: String?
    let operatorName: String?
    let address: ChargingAddressWire?
    let linkedStationId: String?

    private enum CodingKeys: String, CodingKey {
        case id
        case entityKey
        case externalId
        case name
        case country
        case latitude
        case longitude
        case publicAccess
        case reportedPoints
        case groups
        case sources
        case confidence
        case uf
        case operatorName = "operator"
        case address
        case linkedStationId
    }
}

private struct ChargingCoverageWire: Decodable {
    struct Offer: Decodable {
        let notice: String
        let data: String
    }

    let profile: String
    let sourceRevision: String
    let places: Int64
    let groups: Int64
    let vehicles: Int64
    let generatedAt: String
    let offer: Offer?
}

public enum ChargingWireDecoder {
    public static func decodePage(_ data: Data) throws -> ChargingPlacePage {
        try refusePrivateFields(data)
        let wire: ChargingPageWire
        do {
            wire = try AbastevoHttpClient.publicDecoder().decode(ChargingPageWire.self, from: data)
        } catch {
            throw ChargingWireError.refused("page shape")
        }
        guard wire.items.count <= ChargingWireRules.maxRows else { throw ChargingWireError.refused("items over bound") }
        let places = try wire.items.map { try place($0, groupsRequired: $0.confidence != nil) }
        guard Set(places.map(\.id)).count == places.count else { throw ChargingWireError.refused("duplicate place id") }
        if let next = wire.nextCursor, next.isEmpty { throw ChargingWireError.refused("blank cursor") }
        guard ChargingWireRules.isInstant(wire.generatedAt) else { throw ChargingWireError.refused("generated_at") }
        return ChargingPlacePage(items: places, nextCursor: wire.nextCursor, generatedAt: wire.generatedAt)
    }

    public static func decodePlace(_ data: Data) throws -> ChargingPlace {
        try refusePrivateFields(data)
        let wire: ChargingPlaceWire
        do {
            wire = try AbastevoHttpClient.publicDecoder().decode(ChargingPlaceWire.self, from: data)
        } catch {
            throw ChargingWireError.refused("place shape")
        }
        return try place(wire, groupsRequired: true)
    }

    public static func decodeCoverage(_ data: Data) throws -> ChargingCoverage {
        let wire: ChargingCoverageWire
        do {
            wire = try AbastevoHttpClient.publicDecoder().decode(ChargingCoverageWire.self, from: data)
        } catch {
            throw ChargingWireError.refused("coverage shape")
        }
        guard !wire.profile.isEmpty, !wire.sourceRevision.isEmpty, wire.places >= 0, wire.groups >= 0, wire.vehicles >= 0,
              ChargingWireRules.isInstant(wire.generatedAt) else {
            throw ChargingWireError.refused("coverage values")
        }
        return ChargingCoverage(
            profile: wire.profile, sourceRevision: wire.sourceRevision, places: wire.places, groups: wire.groups,
            vehicles: wire.vehicles, generatedAt: wire.generatedAt, offerNotice: wire.offer?.notice, offerData: wire.offer?.data
        )
    }

    /// Rows and groups carrying private fields are refused, never stripped.
    private static func refusePrivateFields(_ data: Data) throws {
        guard let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw ChargingWireError.refused("json")
        }
        let rows = (root["items"] as? [[String: Any]]) ?? [root]
        for row in rows {
            if row.keys.contains(where: { ChargingWireRules.privateFields.contains($0) }) {
                throw ChargingWireError.refused("private field")
            }
            for group in (row["groups"] as? [[String: Any]]) ?? [] {
                if group.keys.contains(where: { ChargingWireRules.privateFields.contains($0) }) {
                    throw ChargingWireError.refused("private field")
                }
            }
        }
    }

    private static func place(_ wire: ChargingPlaceWire, groupsRequired: Bool) throws -> ChargingPlace {
        let confidence: ChargingCatalogConfidence?
        switch wire.confidence {
        case nil: confidence = nil
        case "single-source": confidence = .singleSource
        case "two-source": confidence = .twoSource
        default: throw ChargingWireError.refused("confidence")
        }
        let groupsWire = wire.groups ?? []
        if groupsRequired && wire.groups == nil { throw ChargingWireError.refused("groups missing") }
        guard groupsWire.count <= ChargingWireRules.maxRows else { throw ChargingWireError.refused("groups over bound") }
        let groups = try (groupsRequired ? groupsWire : []).map(group)
        let sources = try (wire.sources ?? []).map(source)
        let coordinateScale = confidence == nil ? 12 : 16
        let latitude = wire.latitude.flatMap { $0.isEmpty ? nil : $0 }
        let longitude = wire.longitude.flatMap { $0.isEmpty ? nil : $0 }
        guard ChargingWireRules.isCanonicalId(wire.id),
              ChargingWireRules.isEntityKey(wire.entityKey),
              !wire.externalId.isEmpty, wire.externalId.count <= 256,
              wire.name.count <= 512,
              !wire.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || (confidence != nil && wire.name.isEmpty),
              wire.country == "BR",
              wire.reportedPoints >= 0,
              latitude.map({ ChargingWireRules.decimal($0, minimum: -90, maximum: 90, maxScale: coordinateScale) != nil }) ?? true,
              longitude.map({ ChargingWireRules.decimal($0, minimum: -180, maximum: 180, maxScale: coordinateScale) != nil }) ?? true,
              Set(groups.map(\.id)).count == groups.count,
              sources.count <= 100,
              Set(sources.map { "\($0.source)|\($0.externalId)" }).count == sources.count else {
            throw ChargingWireError.refused("place values")
        }
        if let linked = wire.linkedStationId {
            guard confidence != nil, ChargingWireRules.isCanonicalId(linked) else { throw ChargingWireError.refused("linked station") }
        }
        if let operatorName = wire.operatorName {
            guard !operatorName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, operatorName.count <= 512,
                  !ChargingWireRules.hasControl(operatorName) else {
                throw ChargingWireError.refused("operator")
            }
        }
        if confidence == nil {
            guard groups.allSatisfy({ $0.sourceKey == nil }) else { throw ChargingWireError.refused("unowned group source") }
        } else {
            guard !sources.isEmpty else { throw ChargingWireError.refused("sources missing") }
            if confidence == .twoSource {
                guard Set(sources.map(\.source)) == Set(["OSM", "OCM"]) else { throw ChargingWireError.refused("two-source") }
            }
            let owners = Set(sources.map(\.sourceKey))
            guard groups.allSatisfy({ $0.sourceKey.map { owners.contains($0) } ?? false }) else {
                throw ChargingWireError.refused("group owner")
            }
        }
        let address = wire.address
        for component in [address?.street, address?.number, address?.district, address?.municipio, address?.uf] {
            if let component, component.count > 256 { throw ChargingWireError.refused("address component") }
        }
        let addressLine = address.flatMap {
            ChargingWireRules.formatAddress(street: $0.street, number: $0.number, district: $0.district, municipio: $0.municipio, uf: $0.uf)
        }
        return ChargingPlace(
            id: wire.id,
            entityKey: wire.entityKey,
            externalId: wire.externalId,
            name: wire.name,
            latitude: latitude,
            longitude: longitude,
            publicAccess: ChargingAccess.wire(wire.publicAccess),
            reportedPoints: wire.reportedPoints,
            groups: groups,
            sources: sources,
            confidence: confidence,
            uf: wire.uf,
            linkedStationId: wire.linkedStationId,
            operatorName: wire.operatorName,
            addressLine: addressLine,
            municipality: address?.municipio?.trimmingCharacters(in: .whitespacesAndNewlines)
        )
    }

    private static func group(_ wire: ChargingGroupWire) throws -> ChargingConnectionGroup {
        guard let raw = wire.rawConnectorCode else { throw ChargingWireError.refused("raw connector") }
        let rawValid: Bool
        if wire.sourceKey == nil {
            rawValid = !raw.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && raw.count <= 128
        } else {
            rawValid = raw.count <= 256 && !ChargingWireRules.hasControl(raw)
        }
        guard ChargingWireRules.isCanonicalId(wire.id), ChargingWireRules.isEntityKey(wire.entityKey),
              !wire.externalId.isEmpty, wire.externalId.count <= 256, rawValid,
              wire.powerW.map({ $0 >= 1 }) ?? true, wire.reportedQuantity.map({ $0 >= 1 }) ?? true else {
            throw ChargingWireError.refused("group values")
        }
        return ChargingConnectionGroup(
            id: wire.id, entityKey: wire.entityKey, externalId: wire.externalId,
            connector: ChargingConnector.wire(wire.connectorCode), rawConnectorCode: raw,
            current: ChargingCurrent.wire(wire.current), powerW: wire.powerW,
            reportedQuantity: wire.reportedQuantity, sourceKey: wire.sourceKey
        )
    }

    private static func source(_ wire: ChargingSourceWire) throws -> ChargingSourceAttribution {
        func bounded(_ value: String, _ limit: Int) -> Bool {
            !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && value.count <= limit && !ChargingWireRules.hasControl(value)
        }
        let providerValid = wire.source == "OCM" ? (wire.providerId.map { $0 > 0 } ?? false) : wire.providerId == nil
        guard wire.source == "OSM" || wire.source == "OCM", providerValid,
              bounded(wire.externalId, 128), bounded(wire.revision, 128),
              bounded(wire.license, 512), bounded(wire.attribution, 512) else {
            throw ChargingWireError.refused("source attribution")
        }
        return ChargingSourceAttribution(
            source: wire.source, externalId: wire.externalId, revision: wire.revision,
            providerId: wire.providerId, license: wire.license, attribution: wire.attribution
        )
    }
}

public extension AbastevoHttpClient {
    func fetchChargingCoverage(profile: String) async throws -> ChargingCoverage {
        let data = try await publicRead(path: "/v1/charging/coverage", query: [URLQueryItem(name: "profile", value: profile)])
        return try ChargingWireDecoder.decodeCoverage(data)
    }

    func fetchChargingPlaces(profile: String, revision: String?, limit: Int, cursor: String?) async throws -> ChargingPlacePage {
        guard !profile.isEmpty, (1...ChargingWireRules.maxRows).contains(limit), cursor.map({ !$0.isEmpty }) ?? true else {
            throw HttpClientError.invalidRequest
        }
        var query = [URLQueryItem(name: "profile", value: profile), URLQueryItem(name: "limit", value: String(limit))]
        if let revision { query.append(URLQueryItem(name: "revision", value: revision)) }
        if let cursor { query.append(URLQueryItem(name: "cursor", value: cursor)) }
        let data = try await publicRead(path: "/v1/charging/places", query: query, maxBytes: 1024 * 1024)
        let page = try ChargingWireDecoder.decodePage(data)
        if let cursor, page.nextCursor == cursor { throw ChargingWireError.refused("cursor repeat") }
        return page
    }

    func fetchChargingPlace(id: String) async throws -> ChargingPlace {
        guard ChargingWireRules.isCanonicalId(id) else { throw HttpClientError.invalidRequest }
        let data = try await publicRead(path: "/v1/charging/places/\(id)", maxBytes: 1024 * 1024)
        let place = try ChargingWireDecoder.decodePlace(data)
        guard place.id == id else { throw ChargingWireError.refused("identity") }
        return place
    }
}

public enum ChargingCompatibilityVerdict: String, Equatable, Sendable {
    case compatible
    case unknown
    case incompatible
}

public enum EvSnapshotAge: String, Equatable, Sendable {
    case unknown
    case fresh
    case stale
}

public struct ChargingFilter: Equatable, Sendable {
    public var connector: ChargingConnector?
    public var current: ChargingCurrent?
    public var publicOnly: Bool
    public var minPowerKw: Int?
    public var query: String

    public init(connector: ChargingConnector? = nil, current: ChargingCurrent? = nil, publicOnly: Bool = false, minPowerKw: Int? = nil, query: String = "") {
        self.connector = connector
        self.current = current
        self.publicOnly = publicOnly
        self.minPowerKw = minPowerKw
        self.query = query
    }
}

public enum ChargingRules {
    public static let snapshotStaleSeconds: Int64 = 24 * 60 * 60

    /// EE-21: compatible when a group carries the connector; unknown is never upgraded.
    public static func assess(vehicleConnector: ChargingConnector?, groups: [ChargingConnectionGroup]) -> ChargingCompatibilityVerdict {
        guard let vehicleConnector, !groups.isEmpty else { return .unknown }
        if groups.contains(where: { $0.connector == vehicleConnector }) { return .compatible }
        if groups.contains(where: { $0.connector == .unknown }) { return .unknown }
        return .incompatible
    }

    /// Android `ChargingCompatibility.matches`: one group must satisfy every chosen filter.
    public static func matches(
        _ place: ChargingPlace,
        connector: ChargingConnector?,
        current: ChargingCurrent?,
        access: ChargingAccess?,
        minPowerW: Int64?
    ) -> Bool {
        if let access, place.publicAccess != access { return false }
        if connector == nil && current == nil && minPowerW == nil { return true }
        return place.groups.contains { group in
            (connector == nil || group.connector == connector)
                && (current == nil || group.current == current)
                && (minPowerW.map { minimum in group.powerW.map { $0 >= minimum } ?? false } ?? true)
        }
    }

    /// Android `matchesChargingQuery`: national rows filter power strictly; other
    /// profiles keep unknown power visible; text matches name or external id.
    public static func matches(_ place: ChargingPlace, filter: ChargingFilter) -> Bool {
        let minimumW = filter.minPowerKw.flatMap { $0 > 0 ? Int64($0) * 1_000 : nil }
        let national = place.confidence != nil
        let access: ChargingAccess? = filter.publicOnly ? .publicAccess : nil
        guard matches(place, connector: filter.connector, current: filter.current, access: access, minPowerW: national ? minimumW : nil) else {
            return false
        }
        if !national, let minimumW {
            let powerMatches = place.groups.isEmpty || place.groups.contains { group in group.powerW.map { $0 >= minimumW } ?? true }
            if !powerMatches { return false }
        }
        let query = filter.query.trimmingCharacters(in: .whitespacesAndNewlines)
        if !query.isEmpty {
            let haystack = [place.name, place.externalId, place.operatorName ?? "", place.addressLine ?? ""]
            if !haystack.contains(where: { $0.localizedCaseInsensitiveContains(query) }) { return false }
        }
        return true
    }

    /// EE-26: a missing or future save time is unknown, never fresh.
    public static func age(nowEpochSeconds: Int64, savedAtEpochSeconds: Int64?) -> EvSnapshotAge {
        guard let saved = savedAtEpochSeconds, saved > 0, nowEpochSeconds >= saved else { return .unknown }
        return nowEpochSeconds - saved >= snapshotStaleSeconds ? .stale : .fresh
    }

    /// Places of one municipality (address municipality and UF), accent-insensitive.
    public static func isInCity(_ place: ChargingPlace, city: Municipality) -> Bool {
        guard let uf = place.uf?.uppercased(), uf == city.state.rawValue, let municipality = place.municipality else { return false }
        return AnpTextNormalizer.municipality(municipality) == AnpTextNormalizer.municipality(city.name)
    }

    public static func distanceMeters(_ a: EvCoordinate, _ b: EvCoordinate) -> Double {
        EvRouteGeometry.distanceMeters(a.latitude, a.longitude, b.latitude, b.longitude)
    }

    /// Places with admitted coordinates, parsed once for the map.
    public static func mapPoints(_ places: [ChargingPlace]) -> [ChargingMapPoint] {
        places.compactMap { place in
            place.coordinate.map { ChargingMapPoint(place: place, coordinate: $0) }
        }
    }

    /// Map pins: admitted coordinates inside the visible bounds, nearest to the centre first.
    public static func pins(
        _ points: [ChargingMapPoint],
        center: EvCoordinate,
        latitudeDelta: Double,
        longitudeDelta: Double,
        limit: Int
    ) -> [ChargingMapPoint] {
        let halfLat = max(0.0005, abs(latitudeDelta) / 2)
        let halfLon = max(0.0005, abs(longitudeDelta) / 2)
        let visible = points.filter { point in
            abs(point.coordinate.latitude - center.latitude) <= halfLat
                && abs(point.coordinate.longitude - center.longitude) <= halfLon
        }
        let sorted = visible.sorted { distanceMeters($0.coordinate, center) < distanceMeters($1.coordinate, center) }
        return Array(sorted.prefix(max(0, limit)))
    }
}

public struct ChargingMapPoint: Identifiable, Equatable, Sendable {
    public let place: ChargingPlace
    public let coordinate: EvCoordinate

    public var id: String { place.id }

    public init(place: ChargingPlace, coordinate: EvCoordinate) {
        self.place = place
        self.coordinate = coordinate
    }
}

/// One stored catalog generation with its provenance.
public struct EvCatalogSnapshot: Codable, Equatable, Sendable {
    public let profile: String
    public let sourceRevision: String
    public let origin: String
    public let license: String
    public let generatedAt: String
    public let savedAtEpochSeconds: Int64
    public let places: [ChargingPlace]

    public var groupCount: Int { places.reduce(0) { $0 + $1.groups.count } }
}

public final class EvCatalogStore: @unchecked Sendable {
    private let fileUrl: URL
    private let lock = NSLock()

    public init(directory: URL = IosLocalStore.defaultDirectory()) {
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        fileUrl = directory.appendingPathComponent("ev-charging-snapshot-v1.json")
    }

    public func load() -> EvCatalogSnapshot? {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? Data(contentsOf: fileUrl) else { return nil }
        return try? JSONDecoder().decode(EvCatalogSnapshot.self, from: data)
    }

    public func save(_ snapshot: EvCatalogSnapshot) throws {
        let data = try JSONEncoder().encode(snapshot)
        lock.lock()
        defer { lock.unlock() }
        try data.write(to: fileUrl, options: .atomic)
        var url = fileUrl
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? url.setResourceValues(values)
    }

    public func clear() {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: fileUrl)
    }

    public func storedBytes() -> Int64 {
        let attributes = try? FileManager.default.attributesOfItem(atPath: fileUrl.path)
        return (attributes?[.size] as? NSNumber)?.int64Value ?? 0
    }
}

/// Pulls one complete catalog generation. The national census must match its
/// coverage counts and stay on the same edition from first to last page;
/// any failure leaves the previous snapshot untouched.
public final class EvCatalogService: @unchecked Sendable {
    public static let nationalProfile = "charging-br-national-v1"
    public static let ocmProfile = "ocm-br-20261021-v1"
    public static let nationalLicense = "ODbL-1.0 and source-specific OCM provider terms"
    public static let ocmLicense = "CC-BY-4.0 (provider metadata, attribution required)"
    public static let pageSize = 100
    public static let placesBudget = 10_000

    public let store: EvCatalogStore

    public init(store: EvCatalogStore = EvCatalogStore()) {
        self.store = store
    }

    /// National profile first; the legacy OCM profile only when the national read fails.
    public func refresh(
        client: AbastevoHttpClient,
        now: Date = Date(),
        progress: @escaping @Sendable (Int, Int?) -> Void
    ) async throws -> EvCatalogSnapshot {
        let snapshot: EvCatalogSnapshot
        do {
            snapshot = try await pullNational(client: client, now: now, progress: progress)
        } catch {
            if Task.isCancelled { throw CancellationError() }
            snapshot = try await pullProfile(
                client: client, profile: Self.ocmProfile, revision: nil, license: Self.ocmLicense,
                generatedFallback: nil, now: now, progress: progress
            )
        }
        try store.save(snapshot)
        return snapshot
    }

    public static func expectedOffer(revision: String) -> (notice: String, data: String)? {
        guard ChargingWireRules.isEntityKey(revision) else { return nil }
        let base = "/v1/charging/catalog/\(revision)/offer"
        return ("\(base)/derived-database-offer.json", "\(base)/derived-database.ndjson")
    }

    private func pullNational(
        client: AbastevoHttpClient,
        now: Date,
        progress: @escaping @Sendable (Int, Int?) -> Void
    ) async throws -> EvCatalogSnapshot {
        let coverage = try await client.fetchChargingCoverage(profile: Self.nationalProfile)
        guard coverage.profile == Self.nationalProfile,
              let offer = Self.expectedOffer(revision: coverage.sourceRevision),
              coverage.offerNotice == offer.notice, coverage.offerData == offer.data,
              (1...Int64(Self.placesBudget)).contains(coverage.places) else {
            throw ChargingWireError.refused("national coverage")
        }
        let snapshot = try await pullProfile(
            client: client, profile: Self.nationalProfile, revision: coverage.sourceRevision,
            license: Self.nationalLicense, generatedFallback: coverage.generatedAt, now: now, progress: progress,
            expectedPlaces: Int(coverage.places)
        )
        guard Int64(snapshot.places.count) == coverage.places, Int64(snapshot.groupCount) == coverage.groups else {
            throw ChargingWireError.refused("national counts")
        }
        let after = try await client.fetchChargingCoverage(profile: Self.nationalProfile)
        guard after.sameEdition(as: coverage) else { throw ChargingWireError.refused("national edition changed") }
        return snapshot
    }

    private func pullProfile(
        client: AbastevoHttpClient,
        profile: String,
        revision: String?,
        license: String,
        generatedFallback: String?,
        now: Date,
        progress: @escaping @Sendable (Int, Int?) -> Void,
        expectedPlaces: Int? = nil
    ) async throws -> EvCatalogSnapshot {
        var places: [ChargingPlace] = []
        var seen = Set<String>()
        var cursor: String?
        var generatedAt = generatedFallback ?? ""
        let expectedPages = expectedPlaces.map { ($0 + Self.pageSize - 1) / Self.pageSize }
        var pages = 0
        repeat {
            try Task.checkCancellation()
            let page = try await client.fetchChargingPlaces(profile: profile, revision: revision, limit: Self.pageSize, cursor: cursor)
            pages += 1
            for place in page.items {
                guard !seen.contains(place.id) else { throw ChargingWireError.refused("duplicate place across pages") }
                seen.insert(place.id)
                places.append(place)
            }
            guard places.count <= Self.placesBudget else { throw ChargingWireError.refused("places budget") }
            if generatedAt.isEmpty { generatedAt = page.generatedAt }
            progress(pages, expectedPages)
            cursor = page.nextCursor
        } while cursor != nil
        guard !places.isEmpty else { throw ChargingWireError.refused("empty catalog") }
        return EvCatalogSnapshot(
            profile: profile,
            sourceRevision: revision ?? "",
            origin: client.environment.origin,
            license: license,
            generatedAt: generatedAt,
            savedAtEpochSeconds: Int64(now.timeIntervalSince1970),
            places: places
        )
    }
}
