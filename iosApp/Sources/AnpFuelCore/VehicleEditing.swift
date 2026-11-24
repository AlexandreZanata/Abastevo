import CryptoKit
import Foundation

public enum VehiclePriceReferenceMode: String, CaseIterable, Identifiable, Sendable {
    case cheapest
    case specific

    public var id: String { rawValue }
}

public struct VehiclePricePreference: Codable, Equatable, Sendable {
    public let stationCNPJ: String?
    public let priceDropAlertEnabled: Bool

    init(stationCNPJ: String? = nil, priceDropAlertEnabled: Bool = false) {
        self.stationCNPJ = stationCNPJ
        self.priceDropAlertEnabled = priceDropAlertEnabled
    }
}

public enum VehicleEditorError: Error, Equatable, Sendable {
    case nameRequired
    case invalidTank
    case invalidBattery
    case stationRequired
    case photoInvalid
    case storageUnavailable
}

public struct VehicleEditorDraft: Equatable, Sendable {
    public var name: String
    public var propulsion: Propulsion
    public var fuel: FuelProduct
    public var tankLiters: String
    public var batteryKwh: String
    public var connector: ChargingConnector?
    public var priceMode: VehiclePriceReferenceMode
    public var stationCNPJ: String?
    public var priceDropAlertEnabled: Bool

    public init(existing: VehicleRecord? = nil, preference: VehiclePricePreference? = nil) {
        name = existing?.name ?? ""
        propulsion = existing?.propulsion ?? .flex
        fuel = existing?.primaryFuel ?? .gasolineRegular
        tankLiters = existing.flatMap {
            $0.propulsion.supportsFuel ? LedgerUnits.formatScaled($0.tankCapacityMilliLiters, fractionDigits: 3) : nil
        } ?? "50"
        batteryKwh = existing.flatMap {
            $0.propulsion.supportsElectric ? LedgerUnits.formatScaled($0.batteryCapacityWh, fractionDigits: 3) : nil
        } ?? "52"
        connector = existing?.chargeConnector
        stationCNPJ = preference?.stationCNPJ
        priceMode = stationCNPJ == nil ? .cheapest : .specific
        priceDropAlertEnabled = preference?.priceDropAlertEnabled ?? false
    }

    public func build(existing: VehicleRecord?) throws -> (vehicle: VehicleRecord, preference: VehiclePricePreference) {
        let cleanName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanName.isEmpty else { throw VehicleEditorError.nameRequired }
        let battery: Int64
        if propulsion.supportsElectric {
            guard let wh = try? PortableMoney.parseBatteryCapacityWh(batteryKwh) else {
                throw VehicleEditorError.invalidBattery
            }
            battery = wh
        } else {
            battery = 0
        }
        let tank: Int64
        if propulsion.supportsFuel {
            guard let liters = try? PortableMoney.parseCapacityMilliLiters(tankLiters.replacingOccurrences(of: ".", with: ",")) else {
                throw VehicleEditorError.invalidTank
            }
            tank = liters
        } else {
            tank = 0
        }
        let station: String?
        if propulsion.supportsFuel, priceMode == .specific {
            guard let raw = stationCNPJ, let normalized = BrazilianCNPJ.normalized(raw) else {
                throw VehicleEditorError.stationRequired
            }
            station = normalized
        } else {
            station = nil
        }
        let vehicle = VehicleRecord(
            id: existing?.id ?? UUID().uuidString.lowercased(), name: cleanName, propulsion: propulsion,
            primaryFuel: propulsion.supportsFuel ? fuel : .gasolineRegular, tankCapacityMilliLiters: tank,
            batteryCapacityWh: battery, gasolineKmPerLiter: existing?.gasolineKmPerLiter ?? 11.5,
            ethanolKmPerLiter: existing?.ethanolKmPerLiter ?? 8, whPerKm: existing?.whPerKm ?? 160,
            chargeConnector: propulsion.supportsElectric && connector != .unknown ? connector : nil
        )
        return (vehicle, VehiclePricePreference(
            stationCNPJ: station, priceDropAlertEnabled: propulsion.supportsFuel && priceDropAlertEnabled
        ))
    }
}

public struct VehicleLocalScope: Equatable, Sendable {
    public let accountId: String?
    public let origin: String

    public init(accountId: String?, origin: String) {
        self.accountId = accountId
        self.origin = origin
    }

    fileprivate var storageKey: String {
        let account = accountId.map { "account:\($0)" } ?? "guest"
        return vehicleStorageDigest(account + "\u{0}" + origin)
    }
}

private func vehicleStorageDigest(_ value: String) -> String {
    SHA256.hash(data: Data(value.utf8)).map { String(format: "%02x", $0) }.joined()
}

public struct VehicleLocalPhoto: Equatable, Sendable {
    public static let inputLimit = 16 * 1024 * 1024
    public static let maximumSide = 1024
    private static let maxImagePixels = 100_000_000
    fileprivate static let storedLimit = 4 * 1024 * 1024
    public let jpeg: Data

    public init(sanitizing bytes: Data) throws {
        let codec = IosImageIoPhotoCodec()
        guard !bytes.isEmpty, bytes.count <= Self.inputLimit, let size = codec.probeDims(bytes: bytes),
              size.width <= Self.maxImagePixels / size.height,
              let jpeg = IosImageIoPhotoCodec.encodeJpeg(source: bytes, maxSide: Self.maximumSide, quality: 0.85),
              !jpeg.isEmpty, jpeg.count <= Self.storedLimit else {
            throw VehicleEditorError.photoInvalid
        }
        self.jpeg = jpeg
    }
}

public enum VehiclePhotoChange: Equatable, Sendable {
    case keep
    case remove
    case replace(VehicleLocalPhoto)
}

public struct VehicleLocalDetails: Codable, Equatable, Sendable {
    public fileprivate(set) var preference: VehiclePricePreference = VehiclePricePreference()
    public fileprivate(set) var jpeg: Data?
    public fileprivate(set) var lastNotifiedWeek: String?

    public init() {}
}

/// A single atomic, file-protected record holds the sanitized photo and local settings for each vehicle.
public final class VehicleDetailsStore: @unchecked Sendable {
    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        self.directory = (directory ?? IosLocalStore.defaultDirectory())
            .appendingPathComponent("vehicle-details-v1", isDirectory: true)
    }

    public func load(id: String, scope: VehicleLocalScope) throws -> VehicleLocalDetails {
        lock.lock()
        defer { lock.unlock() }
        return try readUnlocked(path(id, scope: scope))
    }

    public func save(
        id: String, scope: VehicleLocalScope, preference: VehiclePricePreference, photo: VehiclePhotoChange
    ) throws {
        lock.lock()
        defer { lock.unlock() }
        let file = path(id, scope: scope)
        var details = try readUnlocked(file)
        details.preference = preference
        switch photo {
        case .keep: break
        case .remove: details.jpeg = nil
        case .replace(let sanitized): details.jpeg = sanitized.jpeg
        }
        try writeUnlocked(details, to: file)
    }

    public func restore(id: String, scope: VehicleLocalScope, details: VehicleLocalDetails) throws {
        lock.lock()
        defer { lock.unlock() }
        let file = path(id, scope: scope)
        if details == VehicleLocalDetails() {
            try removeUnlocked(file)
        } else {
            try writeUnlocked(details, to: file)
        }
    }

    public func recordNotified(
        id: String, scope: VehicleLocalScope, week: String, matching preference: VehiclePricePreference
    ) throws -> Bool {
        lock.lock()
        defer { lock.unlock() }
        let file = path(id, scope: scope)
        var details = try readUnlocked(file)
        guard details.preference == preference, preference.priceDropAlertEnabled, details.lastNotifiedWeek != week else {
            return false
        }
        details.lastNotifiedWeek = week
        try writeUnlocked(details, to: file)
        return true
    }

    public func remove(id: String) throws {
        lock.lock()
        defer { lock.unlock() }
        guard FileManager.default.fileExists(atPath: directory.path) else { return }
        do {
            let scopes = try FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)
            for scope in scopes {
                try removeUnlocked(scope.appendingPathComponent(vehicleStorageDigest(id) + ".json"))
            }
        } catch {
            throw VehicleEditorError.storageUnavailable
        }
    }

    public func purge() throws {
        lock.lock()
        defer { lock.unlock() }
        try removeUnlocked(directory)
    }

    private func path(_ id: String, scope: VehicleLocalScope) -> URL {
        directory.appendingPathComponent(scope.storageKey, isDirectory: true)
            .appendingPathComponent(vehicleStorageDigest(id) + ".json")
    }

    private func readUnlocked(_ file: URL) throws -> VehicleLocalDetails {
        do {
            let maxBytes = VehicleLocalPhoto.storedLimit * 2
            guard let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize, size <= maxBytes else {
                throw VehicleEditorError.storageUnavailable
            }
            let details = try JSONDecoder().decode(VehicleLocalDetails.self, from: Data(contentsOf: file))
            guard (details.jpeg?.count ?? 0) <= VehicleLocalPhoto.storedLimit,
                  (details.lastNotifiedWeek?.utf8.count ?? 0) <= 64,
                  details.preference.stationCNPJ.map({ BrazilianCNPJ.normalized($0) == $0 }) ?? true else {
                throw VehicleEditorError.storageUnavailable
            }
            return details
        } catch let error as CocoaError where error.code == .fileNoSuchFile || error.code == .fileReadNoSuchFile {
            return VehicleLocalDetails()
        } catch {
            throw VehicleEditorError.storageUnavailable
        }
    }

    private func writeUnlocked(_ details: VehicleLocalDetails, to file: URL) throws {
        do {
            try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
            var options = URLResourceValues()
            options.isExcludedFromBackup = true
            var root = directory
            try root.setResourceValues(options)
            let data = try JSONEncoder().encode(details)
            #if os(iOS)
            try data.write(to: file, options: [.atomic, .completeFileProtection])
            #else
            try data.write(to: file, options: .atomic)
            #endif
        } catch {
            throw VehicleEditorError.storageUnavailable
        }
    }

    private func removeUnlocked(_ url: URL) throws {
        do {
            try FileManager.default.removeItem(at: url)
        } catch let error as CocoaError where error.code == .fileNoSuchFile {
            return
        } catch {
            throw VehicleEditorError.storageUnavailable
        }
    }
}

public enum VehiclePriceDropRule {
    public static func notificationIdentifier(vehicleId: String) -> String {
        "vehicle-price-drop:" + vehicleStorageDigest(vehicleId)
    }

    public static func detect(
        vehicle: VehicleRecord, details: VehicleLocalDetails, city: Municipality, current: AnpWeekRows, previous: AnpWeekRows?
    ) -> AnpPriceDrop? {
        guard vehicle.propulsion.supportsFuel, details.preference.priceDropAlertEnabled,
              details.lastNotifiedWeek != current.week.id, let previous else { return nil }
        let product = vehicle.primaryFuel
        let station = details.preference.stationCNPJ
        let currentPrice = comparisonPrice(rows: current, city: city, product: product, station: station)
        let previousPrice = comparisonPrice(rows: previous, city: city, product: product, station: station)
        guard AnpOfficialRules.shouldNotify(current: currentPrice, previous: previousPrice),
              let currentPrice, let previousPrice else { return nil }
        return AnpPriceDrop(
            alertKey: notificationIdentifier(vehicleId: vehicle.id), product: product, weekId: current.week.id,
            currentMilli: currentPrice, previousMilli: previousPrice
        )
    }

    private static func comparisonPrice(rows: AnpWeekRows, city: Municipality, product: FuelProduct, station: String?) -> Int64? {
        if let station, let matched = rows.stations.first(where: {
            $0.cnpj == station && $0.matches(city) && $0.product == product
        }) {
            return matched.priceMilli
        }
        return AnpOfficialRules.comparisonPrice(rows: rows, city: city, product: product)
    }
}
