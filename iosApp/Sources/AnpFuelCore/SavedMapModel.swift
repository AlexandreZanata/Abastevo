import CryptoKit
import Foundation

public enum SavedMapBounds {
    public static let places = 5_000
    public static let trips = 500
    public static let trackPoints = 10_000
    public static let routePoints = 102
    public static let name = 120
    public static let note = 500
    static let sourceID = 128
    public static let fileBytes = 5 * 1024 * 1024
    static let xmlDepth = 64
    static let xmlElements = 200_000
}

public enum SavedMapError: Error, Equatable, Sendable {
    case fileEmpty, fileTooLarge, unsupportedFormat, noRecords, tooManyRecords, duplicateRecordID
    case xmlMalformed, xmlEncodingUnsupported, xmlPIRefused, xmlDoctypeRefused, xmlEntityRefused
    case xmlTooDeep, xmlTooLarge, coordinateMalformed, coordinateOutOfRange, invalidRecord
    case tooManyPoints, placeQuota, tripQuota, restoreConfirmationRequired, scopeChanged
    case nothingToExport, localStorage
}

public struct SavedGeoPoint: Codable, Hashable, Sendable {
    public let latitudeMicro: Int64
    public let longitudeMicro: Int64

    public var latitude: Double { Double(latitudeMicro) / 1_000_000 }
    public var longitude: Double { Double(longitudeMicro) / 1_000_000 }

    public init(latitudeMicro: Int64, longitudeMicro: Int64) throws {
        guard (-90_000_000...90_000_000).contains(latitudeMicro),
              (-180_000_000...180_000_000).contains(longitudeMicro) else {
            throw SavedMapError.coordinateOutOfRange
        }
        self.latitudeMicro = latitudeMicro
        self.longitudeMicro = longitudeMicro
    }

    public init(latitude: Double, longitude: Double) throws {
        guard latitude.isFinite, longitude.isFinite,
              abs(latitude) <= 91, abs(longitude) <= 181 else { throw SavedMapError.coordinateOutOfRange }
        try self.init(
            latitudeMicro: Int64((latitude * 1_000_000).rounded(.toNearestOrAwayFromZero)),
            longitudeMicro: Int64((longitude * 1_000_000).rounded(.toNearestOrAwayFromZero))
        )
    }

    public static func parse(latitude: String, longitude: String) throws -> Self {
        try Self(latitudeMicro: microdegrees(latitude), longitudeMicro: microdegrees(longitude))
    }

    private static func microdegrees(_ value: String) throws -> Int64 {
        let text = value.trimmingCharacters(in: .whitespacesAndNewlines)
        let negative = text.hasPrefix("-")
        let unsigned = text.hasPrefix("+") || negative ? text.dropFirst() : text[...]
        let parts = unsigned.split(separator: ".", maxSplits: 1, omittingEmptySubsequences: false)
        guard let integral = parts.first, (1...3).contains(integral.count),
              integral.utf8.allSatisfy(digit), let degrees = Int64(integral) else {
            throw SavedMapError.coordinateMalformed
        }
        var fractionMicro: Int64 = 0
        if parts.count == 2 {
            let fraction = parts[1]
            guard (1...18).contains(fraction.count), fraction.utf8.allSatisfy(digit) else {
                throw SavedMapError.coordinateMalformed
            }
            let leading = fraction.utf8.prefix(6)
            for byte in leading {
                fractionMicro = 10 * fractionMicro + Int64(byte - 48)
            }
            for _ in leading.count..<6 { fractionMicro *= 10 }
            if let seventh = fraction.utf8.dropFirst(6).first, seventh >= 53 { fractionMicro += 1 }
        }
        let micro = degrees * 1_000_000 + fractionMicro
        return negative ? -micro : micro
    }

    private static func digit(_ byte: UInt8) -> Bool { (48...57).contains(byte) }
}

public enum SavedMapSource: String, Codable, Sendable {
    case anp = "ANP"
    case community = "COMMUNITY"
    case manual = "MANUAL"
    case imported = "IMPORT"
}

public struct SavedMapPlace: Identifiable, Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let point: SavedGeoPoint
    public let stationId: String?
    public let source: SavedMapSource
    public let sourceId: String?
    public let note: String?
    public let createdAtMillis: Int64

    public init(id: String, name: String, point: SavedGeoPoint, stationId: String?, source: SavedMapSource,
                sourceId: String?, note: String?, createdAtMillis: Int64) throws {
        guard SavedMapText.validID(id), stationId.map(SavedMapText.validID) ?? true,
              SavedMapText.valid(name, limit: SavedMapBounds.name),
              sourceId.map({ SavedMapText.valid($0, limit: SavedMapBounds.sourceID) }) ?? true,
              note.map({ SavedMapText.valid($0, limit: SavedMapBounds.note, allowBlank: true) }) ?? true,
              createdAtMillis >= 0 else { throw SavedMapError.invalidRecord }
        self.id = id
        self.name = name
        self.point = point
        self.stationId = stationId
        self.source = source
        self.sourceId = sourceId
        self.note = note
        self.createdAtMillis = createdAtMillis
    }
}

public struct SavedMapTripPoint: Codable, Equatable, Sendable {
    public let point: SavedGeoPoint
    public let elevationCm: Int?
    public let timeMillis: Int64?

    public init(point: SavedGeoPoint, elevationCm: Int? = nil, timeMillis: Int64? = nil) throws {
        guard elevationCm.map({ (-1_000_000...10_000_000).contains($0) }) ?? true,
              timeMillis.map({ $0 >= 0 }) ?? true else { throw SavedMapError.invalidRecord }
        self.point = point
        self.elevationCm = elevationCm
        self.timeMillis = timeMillis
    }
}

public struct SavedMapTrip: Identifiable, Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case routePlan = "ROUTE_PLAN"
        case recordedTrack = "RECORDED_TRACK"
        case importedTrack = "IMPORTED_TRACK"
    }

    public let id: String
    public let kind: Kind
    public let name: String
    public let points: [SavedMapTripPoint]
    public let avoid: Set<String>
    public let sourceId: String?
    public let note: String?
    public let createdAtMillis: Int64

    static let avoidance: Set<String> = ["TOLL", "MOTORWAY", "FERRY", "UNPAVED"]

    public init(id: String, kind: Kind, name: String, points: [SavedMapTripPoint], avoid: Set<String> = [],
                sourceId: String? = nil, note: String? = nil, createdAtMillis: Int64) throws {
        guard SavedMapText.validID(id), SavedMapText.valid(name, limit: SavedMapBounds.name),
              sourceId.map({ SavedMapText.valid($0, limit: SavedMapBounds.sourceID) }) ?? true,
              note.map({ SavedMapText.valid($0, limit: SavedMapBounds.note, allowBlank: true) }) ?? true,
              createdAtMillis >= 0, avoid.isSubset(of: Self.avoidance) else { throw SavedMapError.invalidRecord }
        if kind == .routePlan {
            guard (2...SavedMapBounds.routePoints).contains(points.count), Set(points.map(\.point)).count == points.count,
                  points.allSatisfy({ $0.elevationCm == nil && $0.timeMillis == nil }) else {
                throw SavedMapError.invalidRecord
            }
        } else {
            guard (1...SavedMapBounds.trackPoints).contains(points.count), avoid.isEmpty else {
                throw SavedMapError.invalidRecord
            }
        }
        self.id = id
        self.kind = kind
        self.name = name
        self.points = points
        self.avoid = avoid
        self.sourceId = sourceId
        self.note = note
        self.createdAtMillis = createdAtMillis
    }
}

public struct SavedMapScope: Codable, Hashable, Sendable {
    public static let guest = "guest"
    public let owner: String
    public let origin: String

    public init?(owner: String, origin: String) {
        guard !owner.isEmpty, owner.utf8.count <= 128,
              !origin.isEmpty, origin.utf8.count <= 256,
              !owner.contains("\n"), !origin.contains("\n") else { return nil }
        self.owner = owner
        self.origin = origin
    }

    var fileKey: String { SavedMapText.fingerprint(Data("\(owner)\n\(origin)".utf8)) }
}

enum SavedMapText {
    static func validID(_ value: String) -> Bool {
        value == value.lowercased() && UUID(uuidString: value)?.uuidString.lowercased() == value
    }

    static func valid(_ value: String, limit: Int, allowBlank: Bool = false) -> Bool {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return (allowBlank || !trimmed.isEmpty) && trimmed.utf16.count <= limit
            && trimmed.unicodeScalars.allSatisfy(isSafe)
    }

    static func cleaned(_ value: String) -> String {
        String(String.UnicodeScalarView(value.unicodeScalars.filter(isSafe)))
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    static func limited(_ text: String, limit: Int) -> String {
        var units = 0
        let prefix = text.unicodeScalars.prefix(while: { scalar in
            units += scalar.value > 0xffff ? 2 : 1
            return units <= limit
        })
        return String(String.UnicodeScalarView(prefix))
    }

    private static func isSafe(_ scalar: Unicode.Scalar) -> Bool {
        let value = scalar.value
        return value == 9 || value == 10 || (value >= 32 && !(0x7f...0x9f).contains(value)
            && value != 0xfffe && value != 0xffff)
    }

    static func fingerprint(_ bytes: Data) -> String {
        SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
    }

    static func deterministicID(_ value: String) -> String {
        var digest = Array(SHA256.hash(data: Data(value.utf8)).prefix(16))
        digest[6] = (digest[6] & 0x0f) | 0x50
        digest[8] = (digest[8] & 0x3f) | 0x80
        let symbols = digest.map { String(format: "%02x", $0) }
        return [symbols[0..<4], symbols[4..<6], symbols[6..<8], symbols[8..<10], symbols[10..<16]]
            .map { $0.joined() }.joined(separator: "-")
    }
}
