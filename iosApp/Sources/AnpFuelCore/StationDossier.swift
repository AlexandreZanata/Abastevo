import Foundation

/// A survey CNPJ is a legacy reference, not a canonical Directory or contribution ID.
public enum StationDossierIdentity: Equatable, Hashable, Sendable {
    case canonical(String)
    case legacyCNPJ(String)

    public static func from(_ station: StationRecord) -> Self? {
        if CommunityWire.isUuid(station.id) { return .canonical(station.id.lowercased()) }
        guard let cnpj = BrazilianCNPJ.normalized(station.cnpj) else { return nil }
        return .legacyCNPJ(cnpj)
    }

    var path: String? {
        switch self {
        case .canonical(let id):
            guard CommunityWire.isUuid(id) else { return nil }
            return "/v1/stations/" + id.lowercased()
        case .legacyCNPJ(let cnpj):
            guard BrazilianCNPJ.normalized(cnpj) == cnpj else { return nil }
            return "/v1/stations/by-cnpj/" + cnpj
        }
    }

    func admits(id: String, cnpj: String?) -> Bool {
        switch self {
        case .canonical(let expected): return id.caseInsensitiveCompare(expected) == .orderedSame
        case .legacyCNPJ(let expected): return cnpj == expected
        }
    }
}

public enum StationDossierAddressSource: String, Codable, Sendable {
    case anp, rfb, osm, cnefe, municipality

    public var isApproximate: Bool { self != .anp }
}

public struct StationRegistryAnchor: Decodable, Sendable {
    public let source: String
    public let displayName: String
    public let municipalityCode: String?
    public let state: String?
    public let finishedAt: String?

    public var finishedDate: Date? { finishedAt.flatMap(AbastevoHttpClient.readIsoDate) }
}

/// Directory-owned identity and admitted provenance, without inferring authorization or ownership.
public struct StationDossier: Sendable {
    public let station: StationRecord
    public let legalName: String?
    public let addressSource: StationDossierAddressSource?
    public let locationQuality: String
    public let revisionId: String?
    public let officialAnchor: StationRegistryAnchor?
    public let state: BrazilianState?

    public var secondaryLegalName: String? {
        guard let legalName, !legalName.isEmpty else { return nil }
        let normalized: (String) -> String = { value in
            value.split(whereSeparator: \.isWhitespace).joined(separator: " ")
        }
        return normalized(legalName).caseInsensitiveCompare(normalized(station.name)) == .orderedSame ? nil : legalName
    }
}

private struct StationDossierWire: Decodable {
    struct Point: Decodable {
        let lat: Double
        let lon: Double
    }

    struct Address: Decodable {
        private struct Key: CodingKey {
            let stringValue: String
            let intValue: Int? = nil

            init?(intValue: Int) { return nil }
            init?(stringValue: String) { self.stringValue = stringValue }
        }

        let fields: [String: String]

        init(from decoder: Decoder) throws {
            let values = try decoder.container(keyedBy: Key.self)
            var fields: [String: String] = [:]
            for key in values.allKeys {
                if let value = try? values.decode(String.self, forKey: key) {
                    fields[key.stringValue] = value
                }
            }
            self.fields = fields
        }
    }

    let stationId: String
    let displayName: String
    let legalName: String?
    let addressSource: String?
    let address: Address?
    let cnpjNormalized: String?
    let municipalityCode: String?
    let state: String?
    let locationQuality: String
    let coordinates: Point?
    let currentRevisionId: String?
    let official: StationRegistryAnchor?
}

public enum StationDossierAddress {
    private static let placeholders = [
        "NULL", "N/A", "NAO INFORMADO", "NÃO INFORMADO", "SEM ENDERECO", "SEM ENDEREÇO", "SN", "S/N",
    ]
    private static let coordinatePair = #"^[-+]?[0-9]{1,3}(?:[.,][0-9]+)?\s*[,;/]\s*[-+]?[0-9]{1,3}(?:[.,][0-9]+)?$"#
    private static let coordinateLabels = #"\b(?:lat(?:itude)?|lon(?:gitude)?|lng|gps|geo)\b"#
    private static let municipalityCode = #"^[0-9]{7}(?:\s*/\s*[a-z]{2})?$"#

    private static func coordinates(_ value: String) -> Bool {
        if value.range(of: coordinatePair, options: .regularExpression) != nil { return true }
        let withoutLabels = value.replacingOccurrences(of: coordinateLabels, with: "", options: [.regularExpression, .caseInsensitive])
        return withoutLabels != value && withoutLabels.contains(where: \.isNumber)
            && !withoutLabels.contains(where: \.isLetter)
    }

    private static func validText(_ value: String) -> Bool {
        value.contains(where: \.isLetter)
            && value.rangeOfCharacter(from: .controlCharacters) == nil
            && !coordinates(value)
            && value.range(of: municipalityCode, options: [.regularExpression, .caseInsensitive]) == nil
            && !placeholders.contains(value.uppercased())
    }

    /// Use only textual allowlisted components. Do not use an IBGE number as a city name or a GPS point as an address.
    public static func line(_ fields: [String: String], state: BrazilianState?) -> String {
        func first(_ keys: [String], valid: (String) -> Bool = StationDossierAddress.validText) -> String? {
            keys.lazy.compactMap { fields[$0]?.trimmingCharacters(in: .whitespacesAndNewlines) }.first(where: valid)
        }
        let city = first(["municipio", "municipalityName", "municipality", "city"])
        let uf = fields["uf"].flatMap { BrazilianState(rawValue: $0.trimmingCharacters(in: .whitespaces).uppercased()) } ?? state
        let locality = city.map { name in uf.map { name + "/" + $0.rawValue } ?? name }
        let street = first(["street", "logradouro"])
        let number = first(["number", "numero"]) { value in
            !value.isEmpty && value.rangeOfCharacter(from: .controlCharacters) == nil
                && !coordinates(value) && value.caseInsensitiveCompare("null") != .orderedSame
        }
        let streetLine = street.map { value in [value, number].compactMap { $0 }.joined(separator: ", ") }
            ?? first(["raw", "endereco"])
        guard let streetLine else { return locality ?? "" }
        let district = first(["district", "bairro"])
        let area = [district, locality].compactMap { $0 }.joined(separator: ", ")
        return area.isEmpty ? streetLine : streetLine + " – " + area
    }
}

public enum StationDossierCommunityAvailability: String, Decodable, Sendable {
    case available = "AVAILABLE"
    case disputed = "DISPUTED"
    case unknown = "UNKNOWN"
}

public struct StationDossierCommunityPrice: Decodable, Sendable {
    public let source: String
    public let availability: StationDossierCommunityAvailability
    public let amountMilliBrl: Int64?
    public let currency: String
    public let confidence: String
    public let freshness: String
    public let independentSupporters: Int64
    public let confirmationCount: Int64
    public let representativeObservationId: String?
    public let lastObservationReceivedAt: String?
    public let expiresAt: String?
    public let algorithmVersion: String
    public let projectionVersion: Int64

    public var receivedDate: Date? { lastObservationReceivedAt.flatMap(AbastevoHttpClient.readIsoDate) }
    public var expiryDate: Date? { expiresAt.flatMap(AbastevoHttpClient.readIsoDate) }

    public func amount(at now: Date) -> Int64? {
        guard availability == .available, freshness == "FRESH" || freshness == "AGING",
              let amountMilliBrl, let expiry = expiryDate,
              now < expiry else { return nil }
        return amountMilliBrl
    }

    func isAdmitted() -> Bool {
        guard source == "COMMUNITY", currency == "BRL",
              independentSupporters >= 0, confirmationCount >= 0,
              !algorithmVersion.isEmpty, projectionVersion > 0,
              representativeObservationId.map(CommunityWire.isUuid) ?? true,
              lastObservationReceivedAt.map({ AbastevoHttpClient.readIsoDate($0) != nil }) ?? true,
              expiresAt.map({ AbastevoHttpClient.readIsoDate($0) != nil }) ?? true else {
            return false
        }
        if availability == .available {
            guard let amountMilliBrl else { return false }
            return (PortableMoney.minMilli...PortableMoney.maxMilli).contains(amountMilliBrl)
        }
        return amountMilliBrl == nil
    }
}

public struct StationDossierPrices: Sendable {
    public let official: [OfficialStationPrice]
    public let community: [FuelProduct: StationDossierCommunityPrice]
}

public enum StationDossierCodec {
    public static func identity(_ data: Data, expected: StationDossierIdentity) throws -> StationDossier {
        let row = try AbastevoHttpClient.publicDecoder().decode(StationDossierWire.self, from: data)
        let name = row.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard CommunityWire.isUuid(row.stationId), !name.isEmpty,
              expected.admits(id: row.stationId, cnpj: row.cnpjNormalized),
              row.currentRevisionId.map(CommunityWire.isUuid) ?? true else {
            throw HttpClientError.invalidJson
        }
        if let point = row.coordinates, !(-90...90).contains(point.lat) || !(-180...180).contains(point.lon) {
            throw HttpClientError.invalidJson
        }
        let candidateTier = CoordinatePinTier(rawValue: row.locationQuality)
        let point = candidateTier?.allowsMapPin == true ? row.coordinates : nil

        let state = row.state.flatMap(BrazilianState.init(rawValue:))
        let record = StationRecord(
            id: row.stationId.lowercased(), cnpj: row.cnpjNormalized ?? "", name: name, brand: "",
            address: StationDossierAddress.line(row.address?.fields ?? [:], state: state), neighborhood: "",
            municipalityCode: row.municipalityCode ?? "",
            latitude: point?.lat ?? 0, longitude: point?.lon ?? 0,
            pinTier: point == nil ? .cityCentroid : candidateTier ?? .cityCentroid,
            officialPricesMilli: [:], officialDateLabel: "", communityObservations: []
        )
        let anchor = row.official.flatMap { $0.source.isEmpty ? nil : $0 }
        return StationDossier(
            station: record, legalName: row.legalName?.trimmingCharacters(in: .whitespacesAndNewlines),
            addressSource: row.addressSource.flatMap(StationDossierAddressSource.init(rawValue:)),
            locationQuality: row.locationQuality, revisionId: row.currentRevisionId,
            officialAnchor: anchor, state: state
        )
    }

    public static func prices(_ data: Data, stationId: String) throws -> StationDossierPrices {
        struct Group: Decodable {
            struct Condition: Decodable { let kind: String }
            let stationId: String
            let fuelProduct: String
            let unit: String
            let condition: Condition
            let community: StationDossierCommunityPrice?
        }
        struct Page: Decodable { let items: [Group] }

        let page = try AbastevoHttpClient.publicDecoder().decode(Page.self, from: data)
        let official = try AbastevoHttpClient.decodeOfficialPrices(data, stationId: stationId)
        var community: [FuelProduct: StationDossierCommunityPrice] = [:]
        for group in page.items {
            guard group.stationId == stationId,
                  let product = FuelProduct.fromWireCode(group.fuelProduct), product.unitWire == group.unit,
                  group.condition.kind == "STANDARD" else {
                throw HttpClientError.invalidJson
            }
            guard let projection = group.community else { continue }
            guard projection.isAdmitted(), community[product] == nil else { throw HttpClientError.invalidJson }
            community[product] = projection
        }
        guard official.allSatisfy({ (PortableMoney.minMilli...PortableMoney.maxMilli).contains($0.amountMilli) }) else {
            throw HttpClientError.invalidJson
        }
        return StationDossierPrices(official: official, community: community)
    }

    static func searchQuery(
        in municipality: Municipality, query: String, cursor: String?, pageSize: Int
    ) throws -> [URLQueryItem] {
        guard query == normalizedSearch(query), !query.isEmpty, (1...100).contains(pageSize),
              cursor.map({ !$0.isEmpty && $0.utf8.count <= 4_096 }) ?? true else {
            throw HttpClientError.invalidRequest
        }
        var items = [
            URLQueryItem(name: "state", value: municipality.state.rawValue),
            URLQueryItem(name: "municipality_code", value: municipality.ibgeCode),
            URLQueryItem(name: "limit", value: String(pageSize)),
            URLQueryItem(name: "q", value: query),
        ]
        if let cursor { items.append(URLQueryItem(name: "cursor", value: cursor)) }
        return items
    }

    public static func normalizedSearch(_ raw: String) -> String {
        let clean = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard clean.unicodeScalars.count >= 2 else { return "" }
        return String(String.UnicodeScalarView(clean.unicodeScalars.prefix(100)))
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

extension AbastevoHttpClient {
    public func fetchStationDossier(_ identity: StationDossierIdentity) async throws -> StationDossier {
        guard let path = identity.path else { throw HttpClientError.invalidRequest }
        let data = try await publicRead(path: path, preserveHttpErrors: true)
        return try StationDossierCodec.identity(data, expected: identity)
    }

    public func fetchStationDossierPrices(stationId: String) async throws -> StationDossierPrices {
        guard CommunityWire.isUuid(stationId) else { throw HttpClientError.invalidRequest }
        let id = stationId.lowercased()
        let data = try await publicRead(path: "/v1/stations/\(id)/prices", preserveHttpErrors: true)
        return try StationDossierCodec.prices(data, stationId: id)
    }

    public func searchStationDirectory(
        in municipality: Municipality, query: String, cursor: String? = nil, pageSize: Int = 20
    ) async throws -> StationDirectoryPage {
        let items = try StationDossierCodec.searchQuery(in: municipality, query: query, cursor: cursor, pageSize: pageSize)
        let data = try await publicRead(path: "/v1/stations", query: items, preserveHttpErrors: true)
        let page = try Self.decodeStations(data, in: municipality)
        guard page.stations.count <= pageSize,
              page.nextCursor.map({ !$0.isEmpty && $0.utf8.count <= 4_096 }) ?? true else {
            throw HttpClientError.invalidJson
        }
        return page
    }
}
