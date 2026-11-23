import Foundation

/// Editable ledger entry text, turned into an exact record (Android `LedgerEntriesViewModel.buildDraft`).
///
/// The user must choose a registered vehicle. Refuels need fuel propulsion and a
/// ledger fuel; charges need electric propulsion. Decimal inputs accept a comma or
/// a period and never round: extra non-zero digits refuse the save. Blank optional
/// fields stay absent, never zero-filled. Saving through the form completes the
/// review of a migrated record.
/// Zero postfix unwrap operators anywhere in this file.
public enum LedgerFormError: String, Error, Equatable, Sendable {
    case vehicleRequired = "vehicle-required"
    case kindUnsupported = "kind-unsupported"
    case refuelUnavailable = "refuel-unavailable"
    case chargeUnavailable = "charge-unavailable"
    case fuelRequired = "fuel-required"
    case dateInvalid = "date-invalid"
    case quantityInvalid = "quantity-invalid"
    case totalInvalid = "total-invalid"
    case unitPriceInvalid = "unit-price-invalid"
    case energyInvalid = "energy-invalid"
    case socInvalid = "soc-invalid"
    case feesInvalid = "fees-invalid"
    case odometerInvalid = "odometer-invalid"
    case noteInvalid = "note-invalid"
    case recordInvalid = "record-invalid"

    public var messagePtBr: String {
        switch self {
        case .vehicleRequired: return "Selecione um veículo cadastrado."
        case .kindUnsupported: return "Este tipo de lançamento não pode ser editado."
        case .refuelUnavailable: return "Abastecimento indisponível para este veículo."
        case .chargeUnavailable: return "Recarga elétrica indisponível para este veículo."
        case .fuelRequired: return "Escolha o combustível abastecido."
        case .dateInvalid: return "Informe uma data e hora válidas."
        case .quantityInvalid: return "Informe a quantidade (até 3 casas decimais, maior que zero)."
        case .totalInvalid: return "Informe o valor pago em reais (até 2 casas decimais)."
        case .unitPriceInvalid: return "Preço por unidade inválido (até 3 casas decimais)."
        case .energyInvalid: return "Energia inválida em kWh (até 3 casas decimais)."
        case .socInvalid: return "Carga da bateria deve ser um número inteiro de 0 a 100."
        case .feesInvalid: return "Taxas inválidas em reais (até 2 casas decimais)."
        case .odometerInvalid: return "Hodômetro inválido em quilômetros (até 3 casas decimais)."
        case .noteInvalid: return "Use até 280 caracteres de texto na observação."
        case .recordInvalid: return "Revise os valores do lançamento."
        }
    }
}

public struct LedgerEntryForm: Equatable, Sendable {
    public var vehicleId: String?
    public var kind: LedgerEntryKind
    public var occurredAtMillis: Int64
    public var fuel: LedgerFuel?
    public var quantityText: String
    public var totalText: String
    public var unitPriceText: String
    public var fill: LedgerFill
    public var category: LedgerExpenseCategory
    public var energyKwhText: String
    public var socStartText: String
    public var socEndText: String
    public var feesText: String
    public var odometerKmText: String
    public var note: String
    public var opensOdometerSegment: Bool
    public var missedFillBefore: Bool

    public init(
        vehicleId: String? = nil,
        kind: LedgerEntryKind = .refuel,
        occurredAtMillis: Int64,
        fuel: LedgerFuel? = nil,
        quantityText: String = "",
        totalText: String = "",
        unitPriceText: String = "",
        fill: LedgerFill = .full,
        category: LedgerExpenseCategory = .maintenance,
        energyKwhText: String = "",
        socStartText: String = "",
        socEndText: String = "",
        feesText: String = "",
        odometerKmText: String = "",
        note: String = "",
        opensOdometerSegment: Bool = false,
        missedFillBefore: Bool = false
    ) {
        self.vehicleId = vehicleId
        self.kind = kind
        self.occurredAtMillis = occurredAtMillis
        self.fuel = fuel
        self.quantityText = quantityText
        self.totalText = totalText
        self.unitPriceText = unitPriceText
        self.fill = fill
        self.category = category
        self.energyKwhText = energyKwhText
        self.socStartText = socStartText
        self.socEndText = socEndText
        self.feesText = feesText
        self.odometerKmText = odometerKmText
        self.note = note
        self.opensOdometerSegment = opensOdometerSegment
        self.missedFillBefore = missedFillBefore
    }

    /// Prefilled text for an existing record (exact values, comma decimals).
    public static func editing(_ record: LedgerEntryRecord) -> LedgerEntryForm {
        func money(_ value: Int64?) -> String {
            value.map { LedgerUnits.formatScaled($0, fractionDigits: 2) } ?? ""
        }
        func milli(_ value: Int64?) -> String {
            value.map { LedgerUnits.formatScaled($0, fractionDigits: 3) } ?? ""
        }
        return LedgerEntryForm(
            vehicleId: record.vehicleId,
            kind: record.kind,
            occurredAtMillis: record.occurredAtMillis,
            fuel: record.fuel,
            quantityText: record.kind == .refuel && record.quantityMilli > 0 ? milli(record.quantityMilli) : "",
            totalText: money(record.totalCentavos),
            unitPriceText: milli(record.unitPriceMilli),
            fill: record.fill,
            category: record.category ?? .other,
            energyKwhText: milli(record.energyWh),
            socStartText: record.socStartPercent.map { String($0) } ?? "",
            socEndText: record.socEndPercent.map { String($0) } ?? "",
            feesText: money(record.feesCentavos),
            odometerKmText: milli(record.odometerMetres),
            note: record.note,
            opensOdometerSegment: record.opensOdometerSegment,
            missedFillBefore: record.missedFillBefore
        )
    }

    /// A blank optional field is absent; text that does not parse is invalid.
    enum OptionalField<T> {
        case absent
        case value(T)
        case invalid

        var isInvalid: Bool {
            if case .invalid = self { return true }
            return false
        }

        var value: T? {
            if case .value(let wrapped) = self { return wrapped }
            return nil
        }
    }

    static func field<T>(_ text: String, _ parse: (String) -> T?) -> OptionalField<T> {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return .absent }
        guard let parsed = parse(trimmed) else { return .invalid }
        return .value(parsed)
    }

    /// Builds the record to save; `existing` keeps the identifier, station and tariff reference.
    public func build(
        id: String,
        vehicles: [VehicleRecord],
        timezone: String,
        existing: LedgerEntryRecord?
    ) -> Result<LedgerEntryRecord, LedgerFormError> {
        guard let vehicleId, let vehicle = vehicles.first(where: { $0.id == vehicleId }) else {
            return .failure(.vehicleRequired)
        }
        guard LedgerEntryKind.recordable.contains(kind) else { return .failure(.kindUnsupported) }
        let occurred = occurredAtMillis - (occurredAtMillis % 1000)
        guard occurred > 0, occurred <= LedgerBounds.maxEventMillis else { return .failure(.dateInvalid) }
        guard LedgerText.isValidNote(note) else { return .failure(.noteInvalid) }
        let maxOdometer = kind == .charge ? LedgerBounds.maxChargeOdometerMetres : LedgerBounds.maxDistanceMetres
        let odometerField = Self.field(odometerKmText) { LedgerUnits.parseKilometres($0, maximumMetres: maxOdometer) }
        if odometerField.isInvalid { return .failure(.odometerInvalid) }
        let odometer = odometerField.value
        let record: LedgerEntryRecord
        switch kind {
        case .refuel:
            guard vehicle.propulsion.supportsFuel else { return .failure(.refuelUnavailable) }
            guard let chosen = fuel ?? LedgerFuel.from(vehicleFuel: vehicle.primaryFuel) else {
                return .failure(.fuelRequired)
            }
            guard let quantity = LedgerUnits.parseQuantityMilli(quantityText) else { return .failure(.quantityInvalid) }
            guard let total = LedgerUnits.parseCentavos(totalText) else { return .failure(.totalInvalid) }
            let priceField = Self.field(unitPriceText, LedgerUnits.parseUnitPriceMilli)
            if priceField.isInvalid { return .failure(.unitPriceInvalid) }
            let price = priceField.value
            record = LedgerEntryRecord(
                id: id, vehicleId: vehicle.id, kind: .refuel, occurredAtMillis: occurred, timezone: timezone,
                fuel: chosen, quantityMilli: quantity, totalCentavos: total, unitPriceMilli: price, fill: fill,
                stationId: existing?.stationId, odometerMetres: odometer, note: note,
                opensOdometerSegment: opensOdometerSegment, missedFillBefore: missedFillBefore
            )
        case .expense:
            guard let amount = LedgerUnits.parseCentavos(totalText) else { return .failure(.totalInvalid) }
            record = LedgerEntryRecord(
                id: id, vehicleId: vehicle.id, kind: .expense, occurredAtMillis: occurred, timezone: timezone,
                totalCentavos: amount, category: category, odometerMetres: odometer, note: note,
                opensOdometerSegment: opensOdometerSegment, missedFillBefore: missedFillBefore
            )
        case .charge:
            guard vehicle.propulsion.supportsElectric else { return .failure(.chargeUnavailable) }
            let energyField = Self.field(energyKwhText, LedgerUnits.parseWh(fromKwh:))
            if energyField.isInvalid { return .failure(.energyInvalid) }
            let totalField = Self.field(totalText, LedgerUnits.parseCentavos)
            if totalField.isInvalid { return .failure(.totalInvalid) }
            let socStartField = Self.field(socStartText, LedgerUnits.parsePercent)
            let socEndField = Self.field(socEndText, LedgerUnits.parsePercent)
            if socStartField.isInvalid || socEndField.isInvalid { return .failure(.socInvalid) }
            let feesField = Self.field(feesText, LedgerUnits.parseCentavos)
            if feesField.isInvalid { return .failure(.feesInvalid) }
            let energy = energyField.value
            let total = totalField.value
            let socStart = socStartField.value
            let socEnd = socEndField.value
            let fees = feesField.value
            record = LedgerEntryRecord(
                id: id, vehicleId: vehicle.id, kind: .charge, occurredAtMillis: occurred, timezone: timezone,
                totalCentavos: total, energyWh: energy, socStartPercent: socStart, socEndPercent: socEnd,
                feesCentavos: fees, tariffId: existing?.tariffId, tariffRevision: existing?.tariffRevision,
                odometerMetres: odometer, note: note,
                opensOdometerSegment: opensOdometerSegment, missedFillBefore: missedFillBefore
            )
        case .odometer:
            return .failure(.kindUnsupported)
        }
        guard LedgerRecordRules.issue(record) == nil else { return .failure(.recordInvalid) }
        return .success(record)
    }
}
