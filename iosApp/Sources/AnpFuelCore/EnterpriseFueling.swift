import Foundation

public enum EnterpriseFuelingProduct: String, CaseIterable, Codable, Sendable {
    case ethanol = "ETHANOL"
    case gasolineRegular = "GASOLINE_REGULAR"
    case gasolineAdditived = "GASOLINE_ADDITIVED"
    case gasolinePremiumGrade = "GASOLINE_PREMIUM_GRADE"
    case dieselS10 = "DIESEL_S10"
    case dieselS500 = "DIESEL_S500"
    case cng = "CNG"
    case electricity = "ELECTRICITY"
    case arla = "ARLA_32"

    public var unit: String {
        switch self {
        case .cng: return "M3"
        case .electricity: return "KWH"
        default: return "L"
        }
    }
}

public enum EnterpriseFuelingFill: String, CaseIterable, Codable, Sendable {
    case full = "FULL"
    case partial = "PARTIAL"
    case unknown = "UNKNOWN"
}

public enum EnterpriseFuelingPayment: String, CaseIterable, Codable, Sendable {
    case cash = "CASH"
    case pix = "PIX"
    case debitCard = "DEBIT_CARD"
    case creditCard = "CREDIT_CARD"
    case fuelCard = "FUEL_CARD"
    case supplierAccount = "SUPPLIER_ACCOUNT"
    case agreementVoucher = "AGREEMENT_VOUCHER"
    case internalStock = "INTERNAL_STOCK"
    case other = "OTHER"
}

/// Base-unit fields of the reviewed Go/Kotlin fueling.submit schema.
public struct EnterpriseFuelingInput: Sendable {
    public struct Reading: Sendable {
        public let meterId: String
        public let segmentId: String
        public let readingId: String
        public let kind: String
        public let value: String

        public init(meterId: String, segmentId: String, readingId: String, kind: String, value: String) {
            self.meterId = meterId
            self.segmentId = segmentId
            self.readingId = readingId
            self.kind = kind
            self.value = value
        }
    }

    public let assetId: String
    public let responsibleEmployeeId: String
    public let product: String
    public let unit: String
    public let quantity: String
    public let paidTotal: String
    public let discount: String?
    public let unitPrice: String?
    public let unitPriceText: String?
    public let fill: String
    public let payer: String
    public let payment: String
    public let reading: Reading?
    public let missingReadingReason: String?
    public let placeKind: String?
    public let placeName: String?
    public let tripId: String?
    public let contractedSupplierId: String?

    public init(assetId: String, responsibleEmployeeId: String, product: String, unit: String, quantity: String,
                paidTotal: String, discount: String? = nil, unitPrice: String? = nil, unitPriceText: String? = nil,
                fill: String, payer: String, payment: String, reading: Reading? = nil, missingReadingReason: String? = nil,
                placeKind: String? = nil, placeName: String? = nil, tripId: String? = nil, contractedSupplierId: String? = nil) {
        self.assetId = assetId
        self.responsibleEmployeeId = responsibleEmployeeId
        self.product = product
        self.unit = unit
        self.quantity = quantity
        self.paidTotal = paidTotal
        self.discount = discount
        self.unitPrice = unitPrice
        self.unitPriceText = unitPriceText
        self.fill = fill
        self.payer = payer
        self.payment = payment
        self.reading = reading
        self.missingReadingReason = missingReadingReason
        self.placeKind = placeKind
        self.placeName = placeName
        self.tripId = tripId
        self.contractedSupplierId = contractedSupplierId
    }
}

public struct EnterpriseFuelingFact: Equatable, Identifiable, Sendable {
    public let id: String
    public let assetId: String
    public let product: EnterpriseFuelingProduct
    public let quantityMilli: Int64
    public let paidCentavos: Int64
    public let discountCentavos: Int64?
    public let unitPriceMilli: Int64?
    public let fill: EnterpriseFuelingFill
    public let payer: EnterpriseExpensePayer
    public let payment: EnterpriseFuelingPayment
    public let reading: EnterpriseMeterEvidence
    public let placeName: String?
}

public enum EnterpriseFuelingOperations {
    public static let submit = "fueling.submit"
    public static let maxQuantityMilli: Int64 = 1_000_000_000
    public static let maxUnitPriceMilli: Int64 = 1_000_000

    private static let keys: Set<String> = [
        "asset_id", "responsible_employee_id", "product", "unit", "quantity", "paid_total", "discount",
        "unit_price", "unit_price_text", "fill", "payer", "payment_method", "reading", "missing_reading_reason",
        "place", "trip_id", "contracted_supplier_id",
    ]

    public static func build(
        envelope: EnterpriseEnvelope, fuelingId: String, input: EnterpriseFuelingInput
    ) throws -> EnterpriseSealedOperation {
        let payload = try validated(input).payload
        return try EnterpriseOperationBuilder.seal(
            envelope: envelope, type: submit, entityType: "fueling", entityId: fuelingId, expectedRevision: nil,
            dependsOn: [], softReferences: [], payload: payload
        )
    }

    /// Reads the canonical operation already validated by the existing partition
    /// binding. Queue receipt state stays separate from the entered fueling facts.
    public static func fact(_ operation: EnterpriseSealedOperation, partition: EnterprisePartition) -> EnterpriseFuelingFact? {
        guard operation.type == submit, operation.entityType == "fueling",
              let top = try? EnterpriseCanonicalJson.parse(
                  operation.canonical, maxBytes: EnterpriseCanonicalJson.maxOperationBytes,
                  maxDepth: EnterpriseCanonicalJson.maxOperationDepth
              ),
              top["owner_account_id"]?.textValue == partition.accountId,
              top["org_id"]?.textValue == partition.organizationId, top["api_origin"]?.textValue == partition.origin,
              let membership = top["membership_id"]?.textValue, let enrollment = top["enrollment_id"]?.textValue,
              let occurredAt = top["occurred_at"]?.textValue, let zone = top["entry_zone"]?.textValue,
              let rawOffset = top["device_offset_seconds"]?.integerLexeme, let offset = Int64(rawOffset),
              top["entity"]?["expected_revision"]?.isNull == true,
              top["attachments"]?.arrayValue?.isEmpty == true,
              top["depends_on"]?.arrayValue?.isEmpty == true,
              top["soft_references"]?.arrayValue?.isEmpty == true,
              let payload = top["payload"]?.objectValue, Set(payload.keys) == keys,
              let asset = payload["asset_id"]?.textValue, let employee = payload["responsible_employee_id"]?.textValue,
              let product = payload["product"]?.textValue, let unit = payload["unit"]?.textValue,
              let quantity = payload["quantity"]?.textValue, let paid = payload["paid_total"]?.textValue,
              let fill = payload["fill"]?.textValue, let payer = payload["payer"]?.textValue,
              let payment = payload["payment_method"]?.textValue else { return nil }
        for optional in ["discount", "unit_price", "unit_price_text", "missing_reading_reason", "trip_id", "contracted_supplier_id"] {
            guard let value = payload[optional], value.isNull || value.textValue != nil else { return nil }
        }
        let reading: EnterpriseFuelingInput.Reading?
        if payload["reading"]?.isNull == true {
            reading = nil
        } else {
            guard let fields = payload["reading"]?.objectValue,
                  Set(fields.keys) == ["meter_id", "segment_id", "reading_id", "kind", "value"],
                  let meter = fields["meter_id"]?.textValue, let segment = fields["segment_id"]?.textValue,
                  let readingId = fields["reading_id"]?.textValue, let kind = fields["kind"]?.textValue,
                  let value = fields["value"]?.textValue else { return nil }
            reading = .init(meterId: meter, segmentId: segment, readingId: readingId, kind: kind, value: value)
        }
        let place = payload["place"]?.objectValue
        if payload["place"]?.isNull != true {
            guard let place, Set(place.keys) == ["kind", "name"],
                  place["kind"]?.textValue != nil, place["name"]?.textValue != nil else { return nil }
        }
        let input = EnterpriseFuelingInput(
            assetId: asset, responsibleEmployeeId: employee, product: product, unit: unit, quantity: quantity, paidTotal: paid,
            discount: payload["discount"]?.textValue, unitPrice: payload["unit_price"]?.textValue,
            unitPriceText: payload["unit_price_text"]?.textValue, fill: fill, payer: payer, payment: payment,
            reading: reading, missingReadingReason: payload["missing_reading_reason"]?.textValue,
            placeKind: place?["kind"]?.textValue, placeName: place?["name"]?.textValue,
            tripId: payload["trip_id"]?.textValue, contractedSupplierId: payload["contracted_supplier_id"]?.textValue
        )
        let envelope = EnterpriseEnvelope(
            partition: partition, membershipId: membership, enrollmentId: enrollment, operationId: operation.operationId,
            declared: EnterpriseTimeDeclaration(occurredAt: occurredAt, entryZone: zone, deviceOffsetSeconds: offset)
        )
        guard let checked = try? validated(input),
              let restored = try? build(envelope: envelope, fuelingId: operation.entityId, input: input),
              restored == operation, let payer = EnterpriseExpensePayer(rawValue: input.payer),
              let payment = EnterpriseFuelingPayment(rawValue: input.payment) else { return nil }
        return EnterpriseFuelingFact(
            id: operation.entityId, assetId: asset, product: checked.product,
            quantityMilli: checked.quantity, paidCentavos: checked.paid,
            discountCentavos: input.discount.flatMap { Int64($0) }, unitPriceMilli: input.unitPrice.flatMap { Int64($0) },
            fill: checked.fill, payer: payer, payment: payment, reading: checked.evidence, placeName: input.placeName
        )
    }

    public static func quantity(_ text: String) -> Int64? {
        guard let value = scaledEntry(text, fractionDigits: 3, unit: 1000),
              (1...maxQuantityMilli).contains(value) else { return nil }
        return value
    }

    public static func money(_ text: String) -> Int64? {
        guard let value = scaledEntry(text, fractionDigits: 2, unit: 100),
              (0...EnterpriseValues.maxCentavos).contains(value) else { return nil }
        return value
    }

    public static func price(_ text: String) -> Int64? {
        guard let value = scaledEntry(text, fractionDigits: 3, unit: 1000),
              (1...maxUnitPriceMilli).contains(value) else { return nil }
        return value
    }

    private static func scaledEntry(_ raw: String, fractionDigits: Int, unit: Int64) -> Int64? {
        let text = raw.trimmingCharacters(in: .whitespacesAndNewlines).replacingOccurrences(of: ",", with: ".")
        guard (1...24).contains(text.utf16.count) else { return nil }
        let parts = text.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count <= 2, let whole = parts.first else { return nil }
        let trimmed = whole.drop(while: { $0 == "0" })
        let normalized = trimmed.isEmpty ? "0" : String(trimmed)
        if parts.count == 1 {
            return EnterpriseValues.parseScaledDecimal(normalized, fractionDigits: fractionDigits, unit: unit)
        }
        return EnterpriseValues.parseScaledDecimal(
            normalized + "." + String(parts[1]), fractionDigits: fractionDigits, unit: unit
        )
    }

    /// Same half-up purchase arithmetic as the Go/Kotlin financial profile.
    public static func arithmeticWarning(quantity: Int64, total: Int64, discount: Int64, unitPrice: Int64) -> Bool {
        guard (1...maxQuantityMilli).contains(quantity), (0...EnterpriseValues.maxCentavos).contains(total),
              (0...EnterpriseValues.maxCentavos).contains(discount), (1...maxUnitPriceMilli).contains(unitPrice) else { return false }
        let expected = (quantity * unitPrice + 5_000) / 10_000 - discount
        return abs(expected - total) > 1
    }

    private static func validated(_ input: EnterpriseFuelingInput) throws
        -> (payload: [String: EnterpriseJson], product: EnterpriseFuelingProduct, quantity: Int64, paid: Int64,
            fill: EnterpriseFuelingFill, evidence: EnterpriseMeterEvidence) {
        guard let product = EnterpriseFuelingProduct(rawValue: input.product), product.unit == input.unit,
              let quantity = integer(input.quantity, min: 1, max: maxQuantityMilli),
              let paid = integer(input.paidTotal, min: 0, max: EnterpriseValues.maxCentavos),
              let fill = EnterpriseFuelingFill(rawValue: input.fill), EnterpriseExpensePayer(rawValue: input.payer) != nil,
              EnterpriseFuelingPayment(rawValue: input.payment) != nil,
              EnterpriseValues.isId(input.assetId), EnterpriseValues.isId(input.responsibleEmployeeId),
              input.discount.map({ integer($0, min: 0, max: EnterpriseValues.maxCentavos) != nil }) ?? true,
              input.unitPrice.map({ integer($0, min: 1, max: maxUnitPriceMilli) != nil }) ?? true,
              input.unitPriceText.map({ input.unitPrice != nil && !$0.isEmpty && !$0.contains("\u{0}") }) ?? true,
              input.tripId.map(EnterpriseValues.isId) ?? true,
              input.contractedSupplierId.map(EnterpriseValues.isId) ?? true else { throw EnterpriseOperationError.malformed }

        let evidence: EnterpriseMeterEvidence
        switch (input.reading, input.missingReadingReason) {
        case (let reading?, nil):
            guard let kind = EnterpriseMeterKind(rawValue: reading.kind),
                  let value = integer(reading.value, min: 0, max: kind.maximum) else { throw EnterpriseOperationError.malformed }
            evidence = .reading(meterId: reading.meterId, segmentId: reading.segmentId, kind: kind,
                                readingId: reading.readingId, value: value, modulus: nil)
        case (nil, let reason?):
            evidence = .missing(reason: reason)
        default:
            throw EnterpriseOperationError.malformed
        }
        let meters = try EnterpriseOperationBuilder.evidencePayload(evidence)

        let place: EnterpriseJson
        var retained = false
        switch input.placeKind {
        case nil:
            guard input.placeName == nil else { throw EnterpriseOperationError.malformed }
            place = .null
        case "NOT_FOUND":
            guard let name = input.placeName, EnterpriseValues.isValidText(name, maxScalars: EnterpriseCanonicalJson.maxOperationBytes) else {
                throw EnterpriseOperationError.malformed
            }
            place = .object(["kind": .text("NOT_FOUND"), "name": .text(name)])
        case "CATALOG_STATION", "ORG_SUPPLIER", "INTERNAL_TANK":
            retained = true
            place = .null
        default:
            throw EnterpriseOperationError.malformed
        }
        guard !retained, input.tripId == nil, input.contractedSupplierId == nil else {
            throw EnterpriseOperationError.unsupported
        }
        return ([
            "asset_id": .text(input.assetId), "responsible_employee_id": .text(input.responsibleEmployeeId),
            "product": .text(input.product), "unit": .text(input.unit), "quantity": .text(input.quantity), "paid_total": .text(input.paidTotal),
            "discount": .optionalText(input.discount), "unit_price": .optionalText(input.unitPrice),
            "unit_price_text": .optionalText(input.unitPriceText), "fill": .text(input.fill), "payer": .text(input.payer),
            "payment_method": .text(input.payment), "reading": meters.reading, "missing_reading_reason": meters.reason,
            "place": place, "trip_id": .null, "contracted_supplier_id": .null,
        ], product, quantity, paid, fill, evidence)
    }

    private static func integer(_ raw: String, min: Int64, max: Int64) -> Int64? {
        guard let value = Int64(raw), (min...max).contains(value), String(value) == raw else { return nil }
        return value
    }
}

/// Field-safe advisory projection containing public limits and an expiration.
public struct EnterpriseFuelingAuthorizationNotice: Codable, Equatable, Identifiable, Sendable {
    public enum State: String, Codable, CaseIterable, Sendable {
        case issued = "ISSUED"
        case redeemed = "REDEEMED"
        case cancelled = "CANCELLED"
        case locked = "LOCKED"
        case expired = "EXPIRED"
    }

    public let id: String
    public let product: String
    public let unit: String
    public let maximumQuantityMilli: Int64
    public let maximumCentavos: Int64
    public let validUntilEpochSeconds: Int64
    public let storedState: State

    private static let products: Set<String> = [
        "ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "DIESEL_S500", "DIESEL_S10", "CNG", "LPG_P13",
    ]
    private static let units: Set<String> = ["L", "M3", "KG_13"]

    public init?(id: String, product: String, unit: String, maximumQuantityMilli: Int64, maximumCentavos: Int64,
                 validUntilEpochSeconds: Int64, storedState: State) {
        guard EnterpriseValues.isId(id), Self.products.contains(product), Self.units.contains(unit),
              maximumQuantityMilli >= 1, maximumCentavos >= 1,
              (0...253_402_300_799).contains(validUntilEpochSeconds), storedState != .expired else { return nil }
        self.id = id
        self.product = product
        self.unit = unit
        self.maximumQuantityMilli = maximumQuantityMilli
        self.maximumCentavos = maximumCentavos
        self.validUntilEpochSeconds = validUntilEpochSeconds
        self.storedState = storedState
    }

    public func state(at nowEpochSeconds: Int64) -> State {
        storedState == .issued && nowEpochSeconds > validUntilEpochSeconds ? .expired : storedState
    }

    public static func nearestUsable(
        _ notices: [Self], at nowEpochSeconds: Int64
    ) -> Self? {
        guard notices.count <= 100 else { return nil }
        return notices.filter { $0.state(at: nowEpochSeconds) == .issued }.min {
            $0.validUntilEpochSeconds < $1.validUntilEpochSeconds
        }
    }
}

/// Editable, human-entered fields. Conversion happens once before sealing.
public struct EnterpriseFuelingFormInput: Sendable {
    public var product: EnterpriseFuelingProduct?
    public var quantity = ""
    public var paid = ""
    public var discount = ""
    public var unitPrice = ""
    public var fill: EnterpriseFuelingFill = .unknown
    public var payer: EnterpriseExpensePayer = .company
    public var payment: EnterpriseFuelingPayment = .other
    public var place = ""

    public init() {}

    public var hasArithmeticWarning: Bool {
        guard let quantity = EnterpriseFuelingOperations.quantity(quantity),
              let paid = EnterpriseFuelingOperations.money(paid),
              let price = EnterpriseFuelingOperations.price(unitPrice),
              let discount = Self.optional(discount, convert: EnterpriseFuelingOperations.money) else { return false }
        return EnterpriseFuelingOperations.arithmeticWarning(quantity: quantity, total: paid, discount: discount ?? 0, unitPrice: price)
    }

    public func makeInput(asset: EnterpriseReferenceAsset, responsibleId: String, evidence: EnterpriseMeterEvidence) throws -> EnterpriseFuelingInput {
        guard let product, let quantity = EnterpriseFuelingOperations.quantity(quantity),
              let paid = EnterpriseFuelingOperations.money(paid),
              let discount = Self.optional(discount, convert: EnterpriseFuelingOperations.money),
              let price = Self.optional(unitPrice, convert: EnterpriseFuelingOperations.price) else {
            throw EnterpriseOperationError.malformed
        }
        let meter: EnterpriseFuelingInput.Reading?
        let reason: String?
        switch evidence {
        case .reading(let meterId, let segment, let kind, let readingId, let value, let modulus):
            guard asset.meters.contains(where: { $0.id == meterId && $0.segmentId == segment
                && $0.kind == kind && $0.modulus == modulus }) else { throw EnterpriseOperationError.malformed }
            _ = try EnterpriseOperationBuilder.evidencePayload(evidence)
            meter = .init(meterId: meterId, segmentId: segment, readingId: readingId, kind: kind.rawValue, value: String(value))
            reason = nil
        case .missing(let missing):
            meter = nil
            reason = missing
        }
        let place = place.trimmingCharacters(in: .whitespacesAndNewlines)
        return EnterpriseFuelingInput(
            assetId: asset.id, responsibleEmployeeId: responsibleId, product: product.rawValue, unit: product.unit,
            quantity: String(quantity), paidTotal: String(paid), discount: discount.map { String($0) },
            unitPrice: price.map { String($0) }, fill: fill.rawValue, payer: payer.rawValue,
            payment: payment.rawValue, reading: meter, missingReadingReason: reason,
            placeKind: place.isEmpty ? nil : "NOT_FOUND", placeName: place.isEmpty ? nil : place
        )
    }

    /// Outer nil is invalid. Inner nil is the explicitly absent optional value.
    private static func optional(_ source: String, convert: (String) -> Int64?) -> Int64?? {
        guard !source.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return .some(nil) }
        guard let value = convert(source) else { return nil }
        return .some(value)
    }
}

public enum EnterpriseFuelingReadingWarning: Sendable {
    case belowPrevious
    case unusualRate

    public static func compare(
        current: EnterpriseMeterEvidence, previous: EnterpriseMeterEvidence, elapsedMillis: Int64
    ) -> Self? {
        guard elapsedMillis > 0,
              case .reading(let meter, let segment, let kind, _, let value, _) = current,
              case .reading(let oldMeter, let oldSegment, let oldKind, _, let oldValue, _) = previous,
              meter == oldMeter, segment == oldSegment, kind == oldKind,
              (0...kind.maximum).contains(value), (0...kind.maximum).contains(oldValue) else { return nil }
        if value < oldValue { return .belowPrevious }
        let difference = value - oldValue
        if kind == .hourMeter {
            let allowed = elapsedMillis.addingReportingOverflow(300_000)
            return !allowed.overflow && difference * 1_000 > allowed.partialValue ? .unusualRate : nil
        }
        let speed = elapsedMillis.multipliedReportingOverflow(by: 150_000)
        let allowed = speed.partialValue.addingReportingOverflow(3_600_000_000)
        return !speed.overflow && !allowed.overflow && difference * 3_600_000 > allowed.partialValue ? .unusualRate : nil
    }
}
