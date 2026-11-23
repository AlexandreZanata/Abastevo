import Foundation
#if canImport(CryptoKit)
import CryptoKit
#endif

/// Private vehicle ledger records for iOS (B-BR-PR13/PR14, B-BR-IOS-VEHICLE).
///
/// Mirrors Android `domain/premium/LedgerDraft.kt`, `PremiumBounds.kt` and the
/// backend payloads in `backend/internal/modules/ledger/domain`. Values use
/// exact integer units only: centavos, milli-litres or milli-cubic-metres, Wh
/// and metres. Server revisions live in the account- and origin-scoped sync
/// state (`LedgerSyncState`), never in the shared local record.
/// Zero postfix unwrap operators anywhere in this file.

public enum LedgerEntryKind: String, CaseIterable, Codable, Sendable {
    case refuel = "REFUEL"
    case charge = "CHARGE"
    case expense = "EXPENSE"
    /// Odometer-only reading of the first iOS ledger. It stays local: the backend has no such dataset.
    case odometer = "ODOMETER"

    /// Kinds a new entry can take; they match the backend datasets.
    public static let recordable: [LedgerEntryKind] = [.refuel, .charge, .expense]

    public var labelPtBr: String {
        switch self {
        case .refuel: return "Abastecimento"
        case .charge: return "Recarga Elétrica"
        case .expense: return "Despesa"
        case .odometer: return "Hodômetro"
        }
    }

    public var dataset: LedgerDataset? {
        switch self {
        case .refuel: return .refuel
        case .charge: return .charge
        case .expense: return .expense
        case .odometer: return nil
        }
    }
}

/// Backend ledger datasets (`refuel|expense|charge`). Unknown wire values are refused.
public enum LedgerDataset: String, CaseIterable, Codable, Sendable {
    case refuel
    case expense
    case charge

    public var kind: LedgerEntryKind {
        switch self {
        case .refuel: return .refuel
        case .expense: return .expense
        case .charge: return .charge
        }
    }

    public var labelPtBr: String { kind.labelPtBr }
}

public enum LedgerFill: String, CaseIterable, Codable, Sendable {
    case full
    case partial
    case unknown

    public var labelPtBr: String {
        switch self {
        case .full: return "Tanque cheio"
        case .partial: return "Parcial"
        case .unknown: return "Não informado"
        }
    }
}

public enum LedgerExpenseCategory: String, CaseIterable, Codable, Sendable {
    case maintenance
    case insurance
    case taxes
    case parking
    case tolls
    case cleaning
    case other

    public var labelPtBr: String {
        switch self {
        case .maintenance: return "Manutenção"
        case .insurance: return "Seguro"
        case .taxes: return "Impostos e taxas"
        case .parking: return "Estacionamento"
        case .tolls: return "Pedágio"
        case .cleaning: return "Limpeza"
        case .other: return "Outros"
        }
    }
}

public enum LedgerUnit: String, CaseIterable, Codable, Sendable {
    case litre = "L"
    case cubicMetre = "M3"

    public var shortLabel: String {
        switch self {
        case .litre: return "L"
        case .cubicMetre: return "m³"
        }
    }
}

/// Ledger fuels with their only valid unit. The additive grade is distinct from
/// the premium grade, and LPG has no ledger unit (Android `PremiumFuel`).
public enum LedgerFuel: String, CaseIterable, Codable, Sendable {
    case ethanol = "ETHANOL"
    case gasolineRegular = "GASOLINE_REGULAR"
    case gasolineAdditived = "GASOLINE_ADDITIVED"
    case gasolinePremiumGrade = "GASOLINE_PREMIUM_GRADE"
    case dieselS500 = "DIESEL_S500"
    case dieselS10 = "DIESEL_S10"
    case cng = "CNG"

    public var unit: LedgerUnit {
        self == .cng ? .cubicMetre : .litre
    }

    /// Vehicle fuel to ledger fuel, as Android `PremiumFuel.fromVehicleProduct`: LPG stays unavailable.
    public static func from(vehicleFuel: FuelProduct) -> LedgerFuel? {
        return LedgerFuel(rawValue: vehicleFuel.wireCode)
    }

    public var labelPtBr: String {
        FuelProduct.fromWireCode(rawValue)?.displayNamePtBr ?? rawValue
    }
}

/// Exact bounds shared by Go, Kotlin and Swift (backend ledger domain and `PremiumBounds.kt`).
public enum LedgerBounds {
    public static let maxQuantityMilli: Int64 = 1_000_000_000
    public static let maxTotalCentavos: Int64 = 1_000_000_000_000
    public static let maxDistanceMetres: Int64 = 1_000_000_000_000
    public static let maxChargeOdometerMetres: Int64 = 1_000_000_000
    public static let maxChargeWh: Int64 = 1_000_000_000
    public static let maxUnitPriceMilli: Int64 = 1_000_000
    public static let maxNoteScalars = 280
    public static let maxTimezoneBytes = 64
    public static let maxTariffRevisionScalars = 128
    public static let maxCursorCharacters = 2048
    public static let calculationVersion = 1
    /// Event instants before 1970 or after year 9999 are refused.
    public static let maxEventMillis: Int64 = 253_402_300_799_999
}

public enum LedgerIds {
    /// Lowercase canonical UUID text, never the nil UUID (backend `validID`).
    public static func isCanonical(_ text: String) -> Bool {
        let scalars = Array(text.unicodeScalars)
        guard scalars.count == 36 else { return false }
        var nonZero = false
        for (index, scalar) in scalars.enumerated() {
            if index == 8 || index == 13 || index == 18 || index == 23 {
                if scalar != "-" { return false }
                continue
            }
            let value = scalar.value
            let isDigit = value >= 0x30 && value <= 0x39
            let isLowerHex = value >= 0x61 && value <= 0x66
            guard isDigit || isLowerHex else { return false }
            if value != 0x30 { nonZero = true }
        }
        return nonZero
    }

    public static func newId() -> String {
        return UUID().uuidString.lowercased()
    }

    /// Java `UUID.nameUUIDFromBytes` (MD5, version 3) over the UTF-8 seed, as Android
    /// `LedgerCommandFactory`: double saves converge to one operation ID.
    public static func nameBased(_ seed: String) -> String? {
        #if canImport(CryptoKit)
        var bytes = Array(Insecure.MD5.hash(data: Data(seed.utf8)))
        guard bytes.count == 16 else { return nil }
        bytes[6] = (bytes[6] & 0x0f) | 0x30
        bytes[8] = (bytes[8] & 0x3f) | 0x80
        let hex = bytes.map { String(format: "%02x", $0) }.joined()
        let chars = Array(hex)
        guard chars.count == 32 else { return nil }
        return String(chars[0..<8]) + "-" + String(chars[8..<12]) + "-" + String(chars[12..<16])
            + "-" + String(chars[16..<20]) + "-" + String(chars[20..<32])
        #else
        return nil
        #endif
    }
}

public enum LedgerText {
    /// Go `unicode.IsControl` and Kotlin `isISOControl`: C0 and C1 controls only.
    public static func isControl(_ scalar: Unicode.Scalar) -> Bool {
        return scalar.value <= 0x1F || (scalar.value >= 0x7F && scalar.value <= 0x9F)
    }

    /// At most 280 Unicode scalars and no control characters.
    public static func isValidNote(_ note: String) -> Bool {
        var count = 0
        for scalar in note.unicodeScalars {
            if isControl(scalar) { return false }
            count += 1
            if count > LedgerBounds.maxNoteScalars { return false }
        }
        return true
    }

    public static func isValidTimezone(_ identifier: String) -> Bool {
        guard !identifier.isEmpty, identifier.utf8.count <= LedgerBounds.maxTimezoneBytes else { return false }
        return TimeZone(identifier: identifier) != nil
    }
}

/// RFC 3339 instants in UTC; whole seconds omit the fraction, like Kotlin `Instant.toString()`.
public enum LedgerTime {
    public static func format(millis: Int64) -> String {
        let text = CommunityWire.formatRfc3339Millis(millis)
        if text.hasSuffix(".000Z") {
            return String(text.dropLast(5)) + "Z"
        }
        return text
    }

    public static func parse(_ text: String) -> Int64? {
        guard !text.isEmpty, text.count <= 64 else { return nil }
        return CommunityWire.parseRfc3339Millis(text)
    }

    /// Wall-clock fields of an instant in `timeZone`.
    public static func components(millis: Int64, timeZone: TimeZone) -> DateComponents {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        let date = Date(timeIntervalSince1970: TimeInterval(millis) / 1000)
        return calendar.dateComponents([.year, .month, .day, .hour, .minute], from: date)
    }

    /// `dd/MM/yyyy HH:mm` in the record's own time zone.
    public static func label(millis: Int64, timezone: String) -> String {
        let zone = TimeZone(identifier: timezone) ?? TimeZone(secondsFromGMT: 0) ?? TimeZone.current
        let parts = components(millis: millis, timeZone: zone)
        func two(_ value: Int?) -> String {
            let number = value ?? 0
            return number < 10 ? "0\(number)" : String(number)
        }
        return "\(two(parts.day))/\(two(parts.month))/\(parts.year ?? 0) \(two(parts.hour)):\(two(parts.minute))"
    }
}

/// Exact decimal input: digits with one optional comma or period and no sign or grouping.
/// Extra fractional digits must be zeros; nothing is rounded.
public enum LedgerUnits {
    public static func parseScaled(_ text: String, fractionDigits: Int, maximum: Int64, allowZero: Bool) -> Int64? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed.count <= 64, fractionDigits >= 0, fractionDigits <= 6, maximum >= 0 else {
            return nil
        }
        var whole = ""
        var fraction = ""
        var seenSeparator = false
        for character in trimmed {
            if character == "," || character == "." {
                if seenSeparator { return nil }
                seenSeparator = true
                continue
            }
            guard let ascii = character.asciiValue, ascii >= 48, ascii <= 57 else { return nil }
            if seenSeparator {
                fraction.append(character)
            } else {
                whole.append(character)
            }
        }
        if whole.isEmpty && fraction.isEmpty { return nil }
        if fraction.count > fractionDigits {
            let extra = fraction.dropFirst(fractionDigits)
            guard extra.allSatisfy({ $0 == "0" }) else { return nil }
            fraction = String(fraction.prefix(fractionDigits))
        }
        while fraction.count < fractionDigits {
            fraction.append("0")
        }
        var value: Int64 = 0
        for character in whole + fraction {
            guard let ascii = character.asciiValue else { return nil }
            let digit = Int64(ascii - 48)
            let shifted = value.multipliedReportingOverflow(by: 10)
            if shifted.overflow { return nil }
            let added = shifted.partialValue.addingReportingOverflow(digit)
            if added.overflow { return nil }
            value = added.partialValue
            if value > maximum { return nil }
        }
        if value == 0 && !allowZero { return nil }
        return value
    }

    /// Ungrouped decimal text with trailing fractional zeros removed (safe for edit then save).
    public static func formatScaled(_ value: Int64, fractionDigits: Int, separator: String = ",") -> String {
        guard fractionDigits > 0 else { return String(value) }
        let negative = value < 0
        var digits = String(value.magnitude)
        while digits.count <= fractionDigits {
            digits = "0" + digits
        }
        let splitIndex = digits.index(digits.endIndex, offsetBy: -fractionDigits)
        let whole = String(digits[..<splitIndex])
        var fraction = String(digits[splitIndex...])
        while fraction.hasSuffix("0") {
            fraction.removeLast()
        }
        let body = fraction.isEmpty ? whole : whole + separator + fraction
        return negative ? "-" + body : body
    }

    public static func parseKilometres(_ text: String, maximumMetres: Int64) -> Int64? {
        return parseScaled(text, fractionDigits: 3, maximum: maximumMetres, allowZero: true)
    }

    public static func parseQuantityMilli(_ text: String) -> Int64? {
        return parseScaled(text, fractionDigits: 3, maximum: LedgerBounds.maxQuantityMilli, allowZero: false)
    }

    public static func parseCentavos(_ text: String) -> Int64? {
        return parseScaled(text, fractionDigits: 2, maximum: LedgerBounds.maxTotalCentavos, allowZero: true)
    }

    public static func parseWh(fromKwh text: String) -> Int64? {
        return parseScaled(text, fractionDigits: 3, maximum: LedgerBounds.maxChargeWh, allowZero: false)
    }

    public static func parseUnitPriceMilli(_ text: String) -> Int64? {
        return parseScaled(text, fractionDigits: 3, maximum: LedgerBounds.maxUnitPriceMilli, allowZero: false)
    }

    public static func parsePercent(_ text: String) -> Int? {
        guard let value = parseScaled(text, fractionDigits: 0, maximum: 100, allowZero: true) else { return nil }
        return Int(value)
    }
}

/// One private ledger record. Schema 2 carries the backend payload fields plus
/// local-only odometer segment, declared gap and legacy review markers.
public struct LedgerEntryRecord: Codable, Equatable, Identifiable, Sendable {
    public static let schemaVersion = 2

    public let id: String
    public let vehicleId: String
    public let kind: LedgerEntryKind
    /// Event instant in epoch milliseconds (UTC).
    public let occurredAtMillis: Int64
    /// IANA zone in which the entry was made.
    public let timezone: String
    public let fuel: LedgerFuel?
    /// Refuel quantity in milli-litres or milli-cubic-metres; zero for other kinds.
    public let quantityMilli: Int64
    /// Refuel total, expense amount or optional charge total, in centavos.
    public let totalCentavos: Int64?
    public let unitPriceMilli: Int64?
    public let fill: LedgerFill
    public let stationId: String?
    public let category: LedgerExpenseCategory?
    public let energyWh: Int64?
    public let socStartPercent: Int?
    public let socEndPercent: Int?
    public let feesCentavos: Int64?
    public let tariffId: String?
    public let tariffRevision: String?
    public let odometerMetres: Int64?
    public let note: String
    /// Local only: the odometer was reset or replaced before this reading (B-BR-PR15).
    public let opensOdometerSegment: Bool
    /// Local only: the owner declared an unrecorded fill before this entry (B-BR-PR16).
    public let missedFillBefore: Bool
    /// Migrated from the first iOS ledger: time, fill and odometer need owner review.
    public let reviewRequired: Bool

    public init(
        id: String = LedgerIds.newId(),
        vehicleId: String,
        kind: LedgerEntryKind,
        occurredAtMillis: Int64,
        timezone: String,
        fuel: LedgerFuel? = nil,
        quantityMilli: Int64 = 0,
        totalCentavos: Int64? = nil,
        unitPriceMilli: Int64? = nil,
        fill: LedgerFill = .unknown,
        stationId: String? = nil,
        category: LedgerExpenseCategory? = nil,
        energyWh: Int64? = nil,
        socStartPercent: Int? = nil,
        socEndPercent: Int? = nil,
        feesCentavos: Int64? = nil,
        tariffId: String? = nil,
        tariffRevision: String? = nil,
        odometerMetres: Int64? = nil,
        note: String = "",
        opensOdometerSegment: Bool = false,
        missedFillBefore: Bool = false,
        reviewRequired: Bool = false
    ) {
        self.id = id
        self.vehicleId = vehicleId
        self.kind = kind
        self.occurredAtMillis = occurredAtMillis
        self.timezone = timezone
        self.fuel = fuel
        self.quantityMilli = quantityMilli
        self.totalCentavos = totalCentavos
        self.unitPriceMilli = unitPriceMilli
        self.fill = fill
        self.stationId = stationId
        self.category = category
        self.energyWh = energyWh
        self.socStartPercent = socStartPercent
        self.socEndPercent = socEndPercent
        self.feesCentavos = feesCentavos
        self.tariffId = tariffId
        self.tariffRevision = tariffRevision
        self.odometerMetres = odometerMetres
        self.note = note
        self.opensOdometerSegment = opensOdometerSegment
        self.missedFillBefore = missedFillBefore
        self.reviewRequired = reviewRequired
    }

    public var dataset: LedgerDataset? { kind.dataset }

    /// `dataset|id`, the key of every sync map; nil for local-only odometer readings.
    public var syncKey: String? {
        guard let dataset else { return nil }
        return LedgerSyncKeys.key(dataset, id)
    }

    public var dateLabel: String {
        LedgerTime.label(millis: occurredAtMillis, timezone: timezone)
    }

    /// Copy with another fuel (legacy migration and editor reuse).
    public func with(fuel newFuel: LedgerFuel?) -> LedgerEntryRecord {
        return LedgerEntryRecord(
            id: id, vehicleId: vehicleId, kind: kind, occurredAtMillis: occurredAtMillis, timezone: timezone,
            fuel: newFuel, quantityMilli: quantityMilli, totalCentavos: totalCentavos, unitPriceMilli: unitPriceMilli,
            fill: fill, stationId: stationId, category: category, energyWh: energyWh,
            socStartPercent: socStartPercent, socEndPercent: socEndPercent, feesCentavos: feesCentavos,
            tariffId: tariffId, tariffRevision: tariffRevision, odometerMetres: odometerMetres, note: note,
            opensOdometerSegment: opensOdometerSegment, missedFillBefore: missedFillBefore, reviewRequired: reviewRequired
        )
    }

    /// A server payload adopted locally keeps this device's local-only markers.
    public func keepingLocalMarkers(of local: LedgerEntryRecord?) -> LedgerEntryRecord {
        guard let local else { return self }
        return LedgerEntryRecord(
            id: id, vehicleId: vehicleId, kind: kind, occurredAtMillis: occurredAtMillis, timezone: timezone,
            fuel: fuel, quantityMilli: quantityMilli, totalCentavos: totalCentavos, unitPriceMilli: unitPriceMilli,
            fill: fill, stationId: stationId, category: category, energyWh: energyWh,
            socStartPercent: socStartPercent, socEndPercent: socEndPercent, feesCentavos: feesCentavos,
            tariffId: tariffId, tariffRevision: tariffRevision, odometerMetres: odometerMetres, note: note,
            opensOdometerSegment: local.opensOdometerSegment, missedFillBefore: local.missedFillBefore, reviewRequired: false
        )
    }

    // MARK: Codable (schema 2, reads the first iOS ledger forward)

    private enum CodingKeys: String, CodingKey {
        case schema
        case id
        case vehicleId
        case kind
        case occurredAtMillis
        case timezone
        case fuel
        case quantityMilli
        case totalCentavos
        case unitPriceMilli
        case fill
        case stationId
        case category
        case energyWh
        case socStartPercent
        case socEndPercent
        case feesCentavos
        case tariffId
        case tariffRevision
        case odometerMetres
        case note
        case opensOdometerSegment
        case missedFillBefore
        case reviewRequired
        case dateLabel
        case monthKey
        case odometerKm
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        let schema = try container.decodeIfPresent(Int.self, forKey: .schema) ?? 1
        let id = try container.decode(String.self, forKey: .id)
        let vehicleId = try container.decode(String.self, forKey: .vehicleId)
        let kind = try container.decode(LedgerEntryKind.self, forKey: .kind)
        if schema < 2 {
            self = LedgerLegacyMigration.migrate(
                id: id,
                vehicleId: vehicleId,
                kind: kind,
                dateLabel: try container.decodeIfPresent(String.self, forKey: .dateLabel) ?? "",
                monthKey: try container.decodeIfPresent(String.self, forKey: .monthKey) ?? "",
                odometerKm: try container.decodeIfPresent(Int64.self, forKey: .odometerKm),
                quantityMilli: try container.decodeIfPresent(Int64.self, forKey: .quantityMilli) ?? 0,
                totalCentavos: try container.decodeIfPresent(Int64.self, forKey: .totalCentavos) ?? 0,
                note: try container.decodeIfPresent(String.self, forKey: .note) ?? "",
                timeZone: TimeZone.current
            )
            return
        }
        let fuelRaw = try container.decodeIfPresent(String.self, forKey: .fuel)
        let fillRaw = try container.decodeIfPresent(String.self, forKey: .fill)
        let categoryRaw = try container.decodeIfPresent(String.self, forKey: .category)
        self = LedgerEntryRecord(
            id: id,
            vehicleId: vehicleId,
            kind: kind,
            occurredAtMillis: try container.decodeIfPresent(Int64.self, forKey: .occurredAtMillis) ?? 0,
            timezone: try container.decodeIfPresent(String.self, forKey: .timezone) ?? TimeZone.current.identifier,
            fuel: fuelRaw.flatMap { LedgerFuel(rawValue: $0) },
            quantityMilli: try container.decodeIfPresent(Int64.self, forKey: .quantityMilli) ?? 0,
            totalCentavos: try container.decodeIfPresent(Int64.self, forKey: .totalCentavos),
            unitPriceMilli: try container.decodeIfPresent(Int64.self, forKey: .unitPriceMilli),
            fill: fillRaw.flatMap { LedgerFill(rawValue: $0) } ?? .unknown,
            stationId: try container.decodeIfPresent(String.self, forKey: .stationId),
            category: categoryRaw.flatMap { LedgerExpenseCategory(rawValue: $0) },
            energyWh: try container.decodeIfPresent(Int64.self, forKey: .energyWh),
            socStartPercent: try container.decodeIfPresent(Int.self, forKey: .socStartPercent),
            socEndPercent: try container.decodeIfPresent(Int.self, forKey: .socEndPercent),
            feesCentavos: try container.decodeIfPresent(Int64.self, forKey: .feesCentavos),
            tariffId: try container.decodeIfPresent(String.self, forKey: .tariffId),
            tariffRevision: try container.decodeIfPresent(String.self, forKey: .tariffRevision),
            odometerMetres: try container.decodeIfPresent(Int64.self, forKey: .odometerMetres),
            note: try container.decodeIfPresent(String.self, forKey: .note) ?? "",
            opensOdometerSegment: try container.decodeIfPresent(Bool.self, forKey: .opensOdometerSegment) ?? false,
            missedFillBefore: try container.decodeIfPresent(Bool.self, forKey: .missedFillBefore) ?? false,
            reviewRequired: try container.decodeIfPresent(Bool.self, forKey: .reviewRequired) ?? false
        )
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(Self.schemaVersion, forKey: .schema)
        try container.encode(id, forKey: .id)
        try container.encode(vehicleId, forKey: .vehicleId)
        try container.encode(kind, forKey: .kind)
        try container.encode(occurredAtMillis, forKey: .occurredAtMillis)
        try container.encode(timezone, forKey: .timezone)
        try container.encodeIfPresent(fuel?.rawValue, forKey: .fuel)
        try container.encode(quantityMilli, forKey: .quantityMilli)
        try container.encodeIfPresent(totalCentavos, forKey: .totalCentavos)
        try container.encodeIfPresent(unitPriceMilli, forKey: .unitPriceMilli)
        try container.encode(fill.rawValue, forKey: .fill)
        try container.encodeIfPresent(stationId, forKey: .stationId)
        try container.encodeIfPresent(category?.rawValue, forKey: .category)
        try container.encodeIfPresent(energyWh, forKey: .energyWh)
        try container.encodeIfPresent(socStartPercent, forKey: .socStartPercent)
        try container.encodeIfPresent(socEndPercent, forKey: .socEndPercent)
        try container.encodeIfPresent(feesCentavos, forKey: .feesCentavos)
        try container.encodeIfPresent(tariffId, forKey: .tariffId)
        try container.encodeIfPresent(tariffRevision, forKey: .tariffRevision)
        try container.encodeIfPresent(odometerMetres, forKey: .odometerMetres)
        try container.encode(note, forKey: .note)
        try container.encode(opensOdometerSegment, forKey: .opensOdometerSegment)
        try container.encode(missedFillBefore, forKey: .missedFillBefore)
        try container.encode(reviewRequired, forKey: .reviewRequired)
    }
}

/// Moves first-ledger records (`dateLabel`, `odometerKm`, untyped quantity) forward without losing them.
///
/// Old refuels keep quantity and total and get an unknown fill; old charges keep their
/// Wh (zero meant not measured); old expenses become `other`; old odometer readings stay
/// local. The date keeps its day at local noon. Every migrated record requires review
/// before upload, and its odometer counts as unverified until then (Android `LegacyOdometerReview`).
public enum LedgerLegacyMigration {
    public static func migrate(
        id: String,
        vehicleId: String,
        kind: LedgerEntryKind,
        dateLabel: String,
        monthKey: String,
        odometerKm: Int64?,
        quantityMilli: Int64,
        totalCentavos: Int64,
        note: String,
        timeZone: TimeZone
    ) -> LedgerEntryRecord {
        var odometerMetres: Int64?
        if let km = odometerKm, km >= 0 {
            let product = km.multipliedReportingOverflow(by: 1000)
            if !product.overflow {
                odometerMetres = product.partialValue
            }
        }
        let occurred = occurredAtMillis(dateLabel: dateLabel, monthKey: monthKey, timeZone: timeZone)
        switch kind {
        case .refuel:
            return LedgerEntryRecord(
                id: id, vehicleId: vehicleId, kind: .refuel, occurredAtMillis: occurred, timezone: timeZone.identifier,
                quantityMilli: max(0, quantityMilli), totalCentavos: totalCentavos, fill: .unknown,
                odometerMetres: odometerMetres, note: note, reviewRequired: true
            )
        case .charge:
            return LedgerEntryRecord(
                id: id, vehicleId: vehicleId, kind: .charge, occurredAtMillis: occurred, timezone: timeZone.identifier,
                totalCentavos: totalCentavos, energyWh: quantityMilli > 0 ? quantityMilli : nil,
                odometerMetres: odometerMetres, note: note, reviewRequired: true
            )
        case .expense:
            return LedgerEntryRecord(
                id: id, vehicleId: vehicleId, kind: .expense, occurredAtMillis: occurred, timezone: timeZone.identifier,
                totalCentavos: totalCentavos, category: .other, odometerMetres: odometerMetres, note: note,
                reviewRequired: true
            )
        case .odometer:
            return LedgerEntryRecord(
                id: id, vehicleId: vehicleId, kind: .odometer, occurredAtMillis: occurred, timezone: timeZone.identifier,
                odometerMetres: odometerMetres, note: note, reviewRequired: true
            )
        }
    }

    /// `dd/MM/yyyy` (or `yyyy-MM`, first day) at 12:00 in `timeZone`; 0 when neither parses.
    public static func occurredAtMillis(dateLabel: String, monthKey: String, timeZone: TimeZone) -> Int64 {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        var components = DateComponents()
        components.hour = 12
        let dayParts = dateLabel.split(separator: "/").map(String.init)
        if dayParts.count == 3, let day = Int(dayParts[0]), let month = Int(dayParts[1]), let year = Int(dayParts[2]),
           (1...31).contains(day), (1...12).contains(month), (1970...9999).contains(year) {
            components.year = year
            components.month = month
            components.day = day
        } else {
            let monthParts = monthKey.split(separator: "-").map(String.init)
            guard monthParts.count == 2, let year = Int(monthParts[0]), let month = Int(monthParts[1]),
                  (1...12).contains(month), (1970...9999).contains(year) else {
                return 0
            }
            components.year = year
            components.month = month
            components.day = 1
        }
        guard let date = calendar.date(from: components) else { return 0 }
        return Int64((date.timeIntervalSince1970 * 1000).rounded())
    }

    /// Old refuels had no fuel: Android derives the ledger fuel from the vehicle, so the migration does too.
    public static func resolveFuels(_ entries: [LedgerEntryRecord], vehicles: [VehicleRecord]) -> [LedgerEntryRecord] {
        return entries.map { entry in
            guard entry.kind == .refuel, entry.fuel == nil,
                  let vehicle = vehicles.first(where: { $0.id == entry.vehicleId }),
                  vehicle.propulsion.supportsFuel,
                  let fuel = LedgerFuel.from(vehicleFuel: vehicle.primaryFuel) else {
                return entry
            }
            return entry.with(fuel: fuel)
        }
    }
}

/// Stable record and conflict keys shared by the sync state and the views.
public enum LedgerSyncKeys {
    public static func key(_ dataset: LedgerDataset, _ id: String) -> String {
        return dataset.rawValue + "|" + id
    }
}

public enum LedgerRecordIssue: String, Error, Equatable, Sendable {
    case invalidId = "invalid-id"
    case invalidVehicle = "invalid-vehicle"
    case invalidTime = "invalid-time"
    case invalidTimezone = "invalid-timezone"
    case fuelRequired = "fuel-required"
    case quantityOutOfBounds = "quantity-out-of-bounds"
    case totalOutOfBounds = "total-out-of-bounds"
    case unitPriceOutOfBounds = "unit-price-out-of-bounds"
    case odometerOutOfBounds = "odometer-out-of-bounds"
    case categoryRequired = "category-required"
    case energyOutOfBounds = "energy-out-of-bounds"
    case socOutOfBounds = "soc-out-of-bounds"
    case feesOutOfBounds = "fees-out-of-bounds"
    case invalidTariff = "invalid-tariff"
    case invalidStation = "invalid-station"
    case invalidNote = "invalid-note"
    case notSyncable = "not-syncable"
}

/// Backend `NormalizeRefuel`, `NormalizeExpense` and `NormalizeCharge`, plus the Android draft bounds.
public enum LedgerRecordRules {
    public static func issue(_ record: LedgerEntryRecord) -> LedgerRecordIssue? {
        guard LedgerIds.isCanonical(record.id) else { return .invalidId }
        guard LedgerIds.isCanonical(record.vehicleId) else { return .invalidVehicle }
        guard record.occurredAtMillis > 0, record.occurredAtMillis <= LedgerBounds.maxEventMillis else { return .invalidTime }
        guard LedgerText.isValidTimezone(record.timezone) else { return .invalidTimezone }
        guard LedgerText.isValidNote(record.note) else { return .invalidNote }
        switch record.kind {
        case .refuel:
            guard record.fuel != nil else { return .fuelRequired }
            guard (1...LedgerBounds.maxQuantityMilli).contains(record.quantityMilli) else { return .quantityOutOfBounds }
            guard let total = record.totalCentavos, (0...LedgerBounds.maxTotalCentavos).contains(total) else {
                return .totalOutOfBounds
            }
            if let price = record.unitPriceMilli, !(1...LedgerBounds.maxUnitPriceMilli).contains(price) {
                return .unitPriceOutOfBounds
            }
            if let odometer = record.odometerMetres, !(0...LedgerBounds.maxDistanceMetres).contains(odometer) {
                return .odometerOutOfBounds
            }
            if let station = record.stationId, !LedgerIds.isCanonical(station) { return .invalidStation }
            return nil
        case .expense:
            guard let amount = record.totalCentavos, (0...LedgerBounds.maxTotalCentavos).contains(amount) else {
                return .totalOutOfBounds
            }
            guard record.category != nil else { return .categoryRequired }
            if let odometer = record.odometerMetres, !(0...LedgerBounds.maxDistanceMetres).contains(odometer) {
                return .odometerOutOfBounds
            }
            return nil
        case .charge:
            if let energy = record.energyWh, !(0...LedgerBounds.maxChargeWh).contains(energy) { return .energyOutOfBounds }
            if let total = record.totalCentavos, !(0...LedgerBounds.maxTotalCentavos).contains(total) {
                return .totalOutOfBounds
            }
            for soc in [record.socStartPercent, record.socEndPercent] {
                if let value = soc, !(0...100).contains(value) { return .socOutOfBounds }
            }
            if let odometer = record.odometerMetres, !(0...LedgerBounds.maxChargeOdometerMetres).contains(odometer) {
                return .odometerOutOfBounds
            }
            if let fees = record.feesCentavos, !(0...LedgerBounds.maxTotalCentavos).contains(fees) { return .feesOutOfBounds }
            if (record.tariffId == nil) != (record.tariffRevision == nil) { return .invalidTariff }
            if let tariff = record.tariffId, !LedgerIds.isCanonical(tariff) { return .invalidTariff }
            if let revision = record.tariffRevision {
                let count = revision.unicodeScalars.count
                if count < 1 || count > LedgerBounds.maxTariffRevisionScalars { return .invalidTariff }
            }
            return nil
        case .odometer:
            if let odometer = record.odometerMetres, !(0...LedgerBounds.maxDistanceMetres).contains(odometer) {
                return .odometerOutOfBounds
            }
            return .notSyncable
        }
    }

    /// Whether the record may be sent: a valid server dataset and no pending legacy review.
    public static func isSyncable(_ record: LedgerEntryRecord) -> Bool {
        return record.dataset != nil && !record.reviewRequired && issue(record) == nil
    }
}

/// Canonical payload bodies with Android's fixed key order, and strict server payload decoding.
public enum LedgerWire {
    static let refuelKeys: Set<String> = [
        "id", "vehicle_id", "occurred_at", "timezone", "fuel", "unit", "quantity_milli", "total_centavos",
        "unit_price_milli", "odometer_metres", "fill", "station_id", "note",
    ]
    static let expenseKeys: Set<String> = [
        "id", "vehicle_id", "occurred_at", "timezone", "amount_centavos", "category", "odometer_metres", "note",
    ]
    static let chargeKeys: Set<String> = [
        "id", "vehicle_id", "occurred_at", "timezone", "energy_wh", "total_centavos", "soc_start_percent",
        "soc_end_percent", "odometer_metres", "fees_centavos", "tariff_id", "tariff_revision", "note",
    ]

    /// JSON string literal escaping identical to Android `escapeJson`.
    public static func quote(_ text: String) -> String {
        var out = "\""
        for scalar in text.unicodeScalars {
            switch scalar {
            case "\"":
                out += "\\\""
            case "\\":
                out += "\\\\"
            default:
                if scalar.value < 0x20 {
                    let hex = String(scalar.value, radix: 16)
                    out += "\\u" + String(repeating: "0", count: max(0, 4 - hex.count)) + hex
                } else {
                    out.unicodeScalars.append(scalar)
                }
            }
        }
        return out + "\""
    }

    /// The exact command payload, or nil when the record is not a valid backend record.
    public static func payloadJson(_ record: LedgerEntryRecord) -> String? {
        guard LedgerRecordRules.issue(record) == nil else { return nil }
        var out = "{\"id\":" + quote(record.id) + ",\"vehicle_id\":" + quote(record.vehicleId)
        out += ",\"occurred_at\":" + quote(LedgerTime.format(millis: record.occurredAtMillis))
        out += ",\"timezone\":" + quote(record.timezone)
        func appendNumber(_ key: String, _ value: Int64?) {
            if let value { out += ",\"\(key)\":\(value)" }
        }
        func appendText(_ key: String, _ value: String?) {
            if let value { out += ",\"\(key)\":" + quote(value) }
        }
        switch record.kind {
        case .refuel:
            guard let fuel = record.fuel, let total = record.totalCentavos else { return nil }
            out += ",\"fuel\":" + quote(fuel.rawValue) + ",\"unit\":" + quote(fuel.unit.rawValue)
            out += ",\"quantity_milli\":\(record.quantityMilli),\"total_centavos\":\(total)"
            appendNumber("unit_price_milli", record.unitPriceMilli)
            appendNumber("odometer_metres", record.odometerMetres)
            out += ",\"fill\":" + quote(record.fill.rawValue)
            appendText("station_id", record.stationId)
            appendText("note", record.note.isEmpty ? nil : record.note)
        case .expense:
            guard let amount = record.totalCentavos, let category = record.category else { return nil }
            out += ",\"amount_centavos\":\(amount),\"category\":" + quote(category.rawValue)
            appendNumber("odometer_metres", record.odometerMetres)
            appendText("note", record.note.isEmpty ? nil : record.note)
        case .charge:
            appendNumber("energy_wh", record.energyWh)
            appendNumber("total_centavos", record.totalCentavos)
            appendNumber("soc_start_percent", record.socStartPercent.map { Int64($0) })
            appendNumber("soc_end_percent", record.socEndPercent.map { Int64($0) })
            appendNumber("odometer_metres", record.odometerMetres)
            appendNumber("fees_centavos", record.feesCentavos)
            appendText("tariff_id", record.tariffId)
            appendText("tariff_revision", record.tariffRevision)
            appendText("note", record.note.isEmpty ? nil : record.note)
        case .odometer:
            return nil
        }
        return out + "}"
    }

    private struct RefuelPayload: Decodable {
        let id: String
        let vehicleId: String
        let occurredAt: String
        let timezone: String
        let fuel: String
        let unit: String
        let quantityMilli: Int64
        let totalCentavos: Int64
        let unitPriceMilli: Int64?
        let odometerMetres: Int64?
        let fill: String
        let stationId: String?
        let note: String?

        enum CodingKeys: String, CodingKey {
            case id
            case vehicleId = "vehicle_id"
            case occurredAt = "occurred_at"
            case timezone
            case fuel
            case unit
            case quantityMilli = "quantity_milli"
            case totalCentavos = "total_centavos"
            case unitPriceMilli = "unit_price_milli"
            case odometerMetres = "odometer_metres"
            case fill
            case stationId = "station_id"
            case note
        }
    }

    private struct ExpensePayload: Decodable {
        let id: String
        let vehicleId: String
        let occurredAt: String
        let timezone: String
        let amountCentavos: Int64
        let category: String
        let odometerMetres: Int64?
        let note: String?

        enum CodingKeys: String, CodingKey {
            case id
            case vehicleId = "vehicle_id"
            case occurredAt = "occurred_at"
            case timezone
            case amountCentavos = "amount_centavos"
            case category
            case odometerMetres = "odometer_metres"
            case note
        }
    }

    private struct ChargePayload: Decodable {
        let id: String
        let vehicleId: String
        let occurredAt: String
        let timezone: String
        let energyWh: Int64?
        let totalCentavos: Int64?
        let socStartPercent: Int?
        let socEndPercent: Int?
        let odometerMetres: Int64?
        let feesCentavos: Int64?
        let tariffId: String?
        let tariffRevision: String?
        let note: String?

        enum CodingKeys: String, CodingKey {
            case id
            case vehicleId = "vehicle_id"
            case occurredAt = "occurred_at"
            case timezone
            case energyWh = "energy_wh"
            case totalCentavos = "total_centavos"
            case socStartPercent = "soc_start_percent"
            case socEndPercent = "soc_end_percent"
            case odometerMetres = "odometer_metres"
            case feesCentavos = "fees_centavos"
            case tariffId = "tariff_id"
            case tariffRevision = "tariff_revision"
            case note
        }
    }

    /// Decodes a server payload (Go marshals absent values as null or omits them). Unknown keys,
    /// unknown vocabulary and out-of-bound values are refused rather than defaulted.
    public static func decodePayload(_ data: Data, dataset: LedgerDataset) -> LedgerEntryRecord? {
        guard !data.isEmpty, data.count <= 16 * 1024,
              let object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else {
            return nil
        }
        let allowed: Set<String>
        switch dataset {
        case .refuel: allowed = refuelKeys
        case .expense: allowed = expenseKeys
        case .charge: allowed = chargeKeys
        }
        guard Set(object.keys).isSubset(of: allowed) else { return nil }
        let decoder = JSONDecoder()
        let record: LedgerEntryRecord
        switch dataset {
        case .refuel:
            guard let wire = try? decoder.decode(RefuelPayload.self, from: data),
                  let occurred = LedgerTime.parse(wire.occurredAt),
                  let fuel = LedgerFuel(rawValue: wire.fuel),
                  wire.unit == fuel.unit.rawValue,
                  let fill = LedgerFill(rawValue: wire.fill) else {
                return nil
            }
            let station = wire.stationId.flatMap { $0.isEmpty ? nil : $0 }
            let price = wire.unitPriceMilli.flatMap { $0 == 0 ? nil : $0 }
            record = LedgerEntryRecord(
                id: wire.id, vehicleId: wire.vehicleId, kind: .refuel, occurredAtMillis: occurred,
                timezone: wire.timezone, fuel: fuel, quantityMilli: wire.quantityMilli,
                totalCentavos: wire.totalCentavos, unitPriceMilli: price, fill: fill, stationId: station,
                odometerMetres: wire.odometerMetres, note: wire.note ?? ""
            )
        case .expense:
            guard let wire = try? decoder.decode(ExpensePayload.self, from: data),
                  let occurred = LedgerTime.parse(wire.occurredAt),
                  let category = LedgerExpenseCategory(rawValue: wire.category) else {
                return nil
            }
            record = LedgerEntryRecord(
                id: wire.id, vehicleId: wire.vehicleId, kind: .expense, occurredAtMillis: occurred,
                timezone: wire.timezone, totalCentavos: wire.amountCentavos, category: category,
                odometerMetres: wire.odometerMetres, note: wire.note ?? ""
            )
        case .charge:
            guard let wire = try? decoder.decode(ChargePayload.self, from: data),
                  let occurred = LedgerTime.parse(wire.occurredAt) else {
                return nil
            }
            record = LedgerEntryRecord(
                id: wire.id, vehicleId: wire.vehicleId, kind: .charge, occurredAtMillis: occurred,
                timezone: wire.timezone, totalCentavos: wire.totalCentavos, energyWh: wire.energyWh,
                socStartPercent: wire.socStartPercent, socEndPercent: wire.socEndPercent,
                feesCentavos: wire.feesCentavos,
                tariffId: wire.tariffId.flatMap { $0.isEmpty ? nil : $0 },
                tariffRevision: wire.tariffRevision.flatMap { $0.isEmpty ? nil : $0 },
                odometerMetres: wire.odometerMetres, note: wire.note ?? ""
            )
        }
        guard LedgerRecordRules.issue(record) == nil else { return nil }
        return record
    }

    public static func decodePayload(json: String, dataset: LedgerDataset) -> LedgerEntryRecord? {
        return decodePayload(Data(json.utf8), dataset: dataset)
    }
}

/// Formula-safe CSV export of local records, free of charge (B-BR-PR12, Android `MonthlyIndicatorsCsv.escape`).
public enum LedgerCsvExport {
    public static let header = "id,kind,vehicle_id,occurred_at,timezone,fuel,unit,quantity_milli,total_centavos,"
        + "unit_price_milli,fill,category,energy_wh,soc_start_percent,soc_end_percent,fees_centavos,"
        + "odometer_metres,station_id,note,review_required"

    public static func escape(_ field: String) -> String {
        guard let first = field.first else { return field }
        let risky = first == "=" || first == "+" || first == "-" || first == "@"
            || field.contains(",") || field.contains("\"") || field.contains("\n") || field.contains("\r")
        guard risky else { return field }
        return "\"'" + field.replacingOccurrences(of: "\"", with: "\"\"") + "\""
    }

    public static func build(_ records: [LedgerEntryRecord]) -> String {
        var out = header + "\n"
        let ordered = records.sorted { lhs, rhs in
            lhs.occurredAtMillis != rhs.occurredAtMillis ? lhs.occurredAtMillis < rhs.occurredAtMillis : lhs.id < rhs.id
        }
        for record in ordered {
            func number(_ value: Int64?) -> String { value.map { String($0) } ?? "" }
            let cells: [String] = [
                escape(record.id),
                record.kind.rawValue,
                escape(record.vehicleId),
                LedgerTime.format(millis: record.occurredAtMillis),
                escape(record.timezone),
                record.fuel?.rawValue ?? "",
                record.fuel?.unit.rawValue ?? "",
                record.kind == .refuel ? String(record.quantityMilli) : "",
                number(record.totalCentavos),
                number(record.unitPriceMilli),
                record.kind == .refuel ? record.fill.rawValue : "",
                record.category?.rawValue ?? "",
                number(record.energyWh),
                record.socStartPercent.map { String($0) } ?? "",
                record.socEndPercent.map { String($0) } ?? "",
                number(record.feesCentavos),
                number(record.odometerMetres),
                escape(record.stationId ?? ""),
                escape(record.note),
                record.reviewRequired ? "1" : "0",
            ]
            out += cells.joined(separator: ",") + "\n"
        }
        return out
    }
}
