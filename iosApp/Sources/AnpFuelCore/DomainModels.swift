import Foundation

/// Complete domain models, business rules and exact calculators for Abastevo iOS,
/// mirroring `domain/` and `application/` on `main`.
/// Zero postfix unwrap operators anywhere in this file.

public enum FuelProduct: String, CaseIterable, Codable, Identifiable, Hashable, Sendable {
    case gasolineRegular = "GASOLINE_REGULAR"
    case gasolinePremium = "GASOLINE_PREMIUM"
    case gasolinePremiumGrade = "GASOLINE_PREMIUM_GRADE"
    case ethanol = "ETHANOL"
    case dieselS10 = "DIESEL_S10"
    case dieselS500 = "DIESEL_S500"
    case cng = "CNG"
    case lpgP13 = "LPG_P13"

    public var id: String { rawValue }

    public var wireCode: String {
        self == .gasolinePremium ? "GASOLINE_ADDITIVED" : rawValue
    }

    public static func fromWireCode(_ code: String) -> FuelProduct? {
        allCases.first { $0.wireCode == code }
    }

    public var unitWire: String {
        switch self {
        case .cng: return "M3"
        case .lpgP13: return "KG_13"
        default: return "L"
        }
    }

    public var shortUnitLabel: String {
        switch self {
        case .cng: return "m3"
        case .lpgP13: return "13 kg"
        default: return "L"
        }
    }

    public var localizedName: String {
        switch self {
        case .gasolineRegular: return String(localized: "android.fuel_product_gasoline_regular", bundle: CoreLocalization.bundle)
        case .gasolinePremium: return String(localized: "android.fuel_product_gasoline_premium", bundle: CoreLocalization.bundle)
        case .gasolinePremiumGrade: return String(localized: "android.fuel_product_gasoline_premium_grade", bundle: CoreLocalization.bundle)
        case .ethanol: return String(localized: "android.fuel_product_ethanol", bundle: CoreLocalization.bundle)
        case .dieselS10: return String(localized: "android.fuel_product_diesel_s10", bundle: CoreLocalization.bundle)
        case .dieselS500: return String(localized: "android.fuel_product_diesel_s500", bundle: CoreLocalization.bundle)
        case .cng: return String(localized: "android.fuel_product_cng", bundle: CoreLocalization.bundle)
        case .lpgP13: return String(localized: "android.fuel_product_lpg_p13", bundle: CoreLocalization.bundle)
        }
    }

    public var tintHexLight: UInt32 {
        switch self {
        case .gasolinePremiumGrade: return 0xC2185B
        case .ethanol: return 0x2E7D32
        case .gasolineRegular: return 0x1565C0
        case .gasolinePremium: return 0x0D47A1
        case .dieselS500: return 0xE65100
        case .dieselS10: return 0xBF360C
        case .cng: return 0x00838F
        case .lpgP13: return 0xD84315
        }
    }

    public var tintHexDark: UInt32 {
        switch self {
        case .gasolinePremiumGrade: return 0xF48FB1
        case .ethanol: return 0xA5D6A7
        case .gasolineRegular: return 0x90CAF9
        case .gasolinePremium: return 0x90CAF9
        case .dieselS500: return 0xFFB74D
        case .dieselS10: return 0xFFB74D
        case .cng: return 0x4DD0E1
        case .lpgP13: return 0xFFAB40
        }
    }

    public static func fromOcr(_ product: OcrFuelProduct) -> FuelProduct {
        return FuelProduct(rawValue: product.rawValue) ?? .gasolineRegular
    }
}

public enum Propulsion: String, CaseIterable, Codable, Identifiable, Sendable {
    case combustion = "COMBUSTION"
    case flex = "FLEX"
    case hybrid = "HYBRID"
    case plugInHybrid = "PLUG_IN_HYBRID"
    case electric = "ELECTRIC"

    public var id: String { rawValue }

    public var localizedLabel: String {
        switch self {
        case .combustion: return String(localized: "android.vehicle_propulsion_ice", bundle: CoreLocalization.bundle)
        case .flex: return String(localized: "vehicle.propulsion.flex", bundle: CoreLocalization.bundle)
        case .hybrid: return String(localized: "android.vehicle_propulsion_hev", bundle: CoreLocalization.bundle)
        case .plugInHybrid: return String(localized: "android.vehicle_propulsion_phev", bundle: CoreLocalization.bundle)
        case .electric: return String(localized: "android.vehicle_propulsion_bev", bundle: CoreLocalization.bundle)
        }
    }

    public var supportsFuel: Bool { self != .electric }
    public var supportsElectric: Bool { self == .electric || self == .plugInHybrid }
}

public enum BrazilianRegion: String, CaseIterable, Codable, Identifiable, Sendable {
    case norte = "Norte"
    case nordeste = "Nordeste"
    case centroOeste = "Centro-Oeste"
    case sudeste = "Sudeste"
    case sul = "Sul"

    public var id: String { rawValue }
}

public enum BrazilianState: String, CaseIterable, Codable, Hashable, Identifiable, Sendable {
    case AC, AL, AP, AM, BA, CE, DF, ES, GO, MA, MT, MS, MG, PA, PB, PR, PE, PI, RJ, RN, RS, RO, RR, SC, SP, SE, TO

    public var id: String { rawValue }

    public var fullName: String {
        switch self {
        case .AC: return "Acre"
        case .AL: return "Alagoas"
        case .AP: return "Amapá"
        case .AM: return "Amazonas"
        case .BA: return "Bahia"
        case .CE: return "Ceará"
        case .DF: return "Distrito Federal"
        case .ES: return "Espírito Santo"
        case .GO: return "Goiás"
        case .MA: return "Maranhão"
        case .MT: return "Mato Grosso"
        case .MS: return "Mato Grosso do Sul"
        case .MG: return "Minas Gerais"
        case .PA: return "Pará"
        case .PB: return "Paraíba"
        case .PR: return "Paraná"
        case .PE: return "Pernambuco"
        case .PI: return "Piauí"
        case .RJ: return "Rio de Janeiro"
        case .RN: return "Rio Grande do Norte"
        case .RS: return "Rio Grande do Sul"
        case .RO: return "Rondônia"
        case .RR: return "Roraima"
        case .SC: return "Santa Catarina"
        case .SP: return "São Paulo"
        case .SE: return "Sergipe"
        case .TO: return "Tocantins"
        }
    }

    public var region: BrazilianRegion {
        switch self {
        case .AC, .AP, .AM, .PA, .RO, .RR, .TO: return .norte
        case .AL, .BA, .CE, .MA, .PB, .PE, .PI, .RN, .SE: return .nordeste
        case .DF, .GO, .MT, .MS: return .centroOeste
        case .ES, .MG, .RJ, .SP: return .sudeste
        case .PR, .RS, .SC: return .sul
        }
    }
}

public struct Municipality: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String { ibgeCode }
    public let ibgeCode: String
    public let name: String
    public let state: BrazilianState
    public init(ibgeCode: String, name: String, state: BrazilianState) {
        self.ibgeCode = ibgeCode
        self.name = name
        self.state = state
    }

    public var displayTitle: String { "\(name), \(state.rawValue)" }
}

public struct MunicipalityCatalog: Sendable {
    private struct Entry: Decodable, Sendable {
        let ibgeCode: String
        let municipality: String
        let state: BrazilianState
        let normalizedName: String

        var city: Municipality { Municipality(ibgeCode: ibgeCode, name: municipality, state: state) }
    }

    public enum CatalogError: Error, Equatable {
        case missingAsset
        case invalidCatalog
    }

    public static let resourceName = "ibge_municipalities"
    public static let empty = MunicipalityCatalog(entries: [])
    private let entries: [Entry]
    private let byCode: [String: Municipality]

    public var count: Int { entries.count }

    public init(bundle: Bundle) throws {
        guard let url = bundle.url(forResource: Self.resourceName, withExtension: "json") else {
            throw CatalogError.missingAsset
        }
        try self.init(data: Data(contentsOf: url))
    }

    public init(data: Data) throws {
        let rows = try JSONDecoder().decode([Entry].self, from: data)
        let states = Set(rows.map(\.state))
        guard rows.count >= 5_570,
              states == Set(BrazilianState.allCases),
              Set(rows.map(\.ibgeCode)).count == rows.count,
              rows.allSatisfy({ row in
                  row.ibgeCode.utf8.count == 7 && row.ibgeCode.utf8.allSatisfy({ (48...57).contains($0) })
                      && !row.municipality.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                      && !row.normalizedName.isEmpty
              }) else {
            throw CatalogError.invalidCatalog
        }
        self.init(entries: rows)
    }

    private init(entries: [Entry]) {
        self.entries = entries.sorted { lhs, rhs in
            if lhs.normalizedName != rhs.normalizedName { return lhs.normalizedName < rhs.normalizedName }
            return lhs.ibgeCode < rhs.ibgeCode
        }
        self.byCode = Dictionary(uniqueKeysWithValues: entries.map { ($0.ibgeCode, $0.city) })
    }

    public func municipality(code: String) -> Municipality? { byCode[code] }

    private static func normalizeSearch(_ text: String) -> String {
        return text
            .folding(options: [.diacriticInsensitive, .caseInsensitive], locale: Locale(identifier: "pt_BR"))
            .replacingOccurrences(of: "'", with: "")
            .replacingOccurrences(of: "’", with: "")
            .replacingOccurrences(of: "-", with: " ")
            .split(whereSeparator: \.isWhitespace)
            .joined(separator: " ")
            .uppercased()
    }

    public func search(query: String, state: BrazilianState? = nil) -> [Municipality] {
        let normalized = Self.normalizeSearch(query)
        let filtered = state.map { selectedState in entries.filter { $0.state == selectedState } } ?? entries
        if normalized.isEmpty { return filtered.map(\.city) }

        var exact: [Municipality] = []
        var prefix: [Municipality] = []
        var included: [Municipality] = []
        for row in filtered {
            if row.normalizedName == normalized || row.ibgeCode == normalized {
                exact.append(row.city)
            } else if row.normalizedName.hasPrefix(normalized) {
                prefix.append(row.city)
            } else if row.normalizedName.contains(normalized) || row.state.rawValue == normalized {
                included.append(row.city)
            }
        }
        return exact + prefix + included
    }
}

public struct SurveyWeek: Codable, Equatable, Identifiable, Hashable, Sendable {
    public let id: String
    public let label: String
    public let startDate: String
    public let endDate: String
    public let isLatest: Bool
    public let isDownloaded: Bool
    public let stationCount: Int
    public let operationalNote: String?

    public init(
        id: String,
        label: String,
        startDate: String,
        endDate: String,
        isLatest: Bool,
        isDownloaded: Bool,
        stationCount: Int,
        operationalNote: String? = nil
    ) {
        self.id = id
        self.label = label
        self.startDate = startDate
        self.endDate = endDate
        self.isLatest = isLatest
        self.isDownloaded = isDownloaded
        self.stationCount = stationCount
        self.operationalNote = operationalNote
    }
}

public struct OfficialAnpAverage: Codable, Equatable, Sendable {
    public static let anpOfficialSourceUrl = "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos"

    public let municipalityCode: String
    public let surveyWeekId: String
    public let product: FuelProduct
    public let meanMilli: Int64
    public let minMilli: Int64
    public let maxMilli: Int64
    public let stationCount: Int
    public let surveyDateLabel: String

    public init(
        municipalityCode: String,
        surveyWeekId: String,
        product: FuelProduct,
        meanMilli: Int64,
        minMilli: Int64,
        maxMilli: Int64,
        stationCount: Int,
        surveyDateLabel: String
    ) {
        self.municipalityCode = municipalityCode
        self.surveyWeekId = surveyWeekId
        self.product = product
        self.meanMilli = meanMilli
        self.minMilli = minMilli
        self.maxMilli = maxMilli
        self.stationCount = stationCount
        self.surveyDateLabel = surveyDateLabel
    }
}

public enum PriceConditionKind: String, CaseIterable, Codable, Identifiable, Sendable {
    case standard = "STANDARD"
    case cash = "CASH"
    case pix = "PIX"
    case debit = "DEBIT"
    case credit = "CREDIT"
    case app = "APP"
    case loyalty = "LOYALTY"

    public var id: String { rawValue }

    public var localizedLabel: String {
        switch self {
        case .standard: return String(localized: "community.price.standard", bundle: CoreLocalization.bundle)
        case .cash: return String(localized: "android.map_sheet_condition_cash", bundle: CoreLocalization.bundle)
        case .pix: return String(localized: "android.enterprise_fuelings_payment_pix", bundle: CoreLocalization.bundle)
        case .debit: return String(localized: "android.enterprise_fuelings_payment_debit", bundle: CoreLocalization.bundle)
        case .credit: return String(localized: "android.enterprise_fuelings_payment_credit", bundle: CoreLocalization.bundle)
        case .app: return String(localized: "android.map_sheet_condition_app", bundle: CoreLocalization.bundle)
        case .loyalty: return String(localized: "android.map_sheet_condition_loyalty", bundle: CoreLocalization.bundle)
        }
    }

    public var wireKind: String {
        switch self {
        case .pix: return "CASH"
        default: return rawValue
        }
    }
}

public struct CommunityPriceObservation: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let stationId: String
    public let stationName: String
    public let municipalityCode: String
    public let product: FuelProduct
    public let amountMilli: Int64
    public let condition: PriceConditionKind
    public let confirmations: Int
    public let disputes: Int
    public let helpfulCount: Int
    public let notHelpfulCount: Int
    public let observedAtLabel: String
    public let observedEpochSeconds: Int64
    public let hasPhotoEvidence: Bool
    public let isOwnSubmission: Bool
    public let contribution: CommunityContributionReference?

    public init(
        id: String,
        stationId: String,
        stationName: String,
        municipalityCode: String,
        product: FuelProduct,
        amountMilli: Int64,
        condition: PriceConditionKind,
        confirmations: Int,
        disputes: Int,
        helpfulCount: Int,
        notHelpfulCount: Int,
        observedAtLabel: String,
        observedEpochSeconds: Int64,
        hasPhotoEvidence: Bool,
        isOwnSubmission: Bool = false,
        contribution: CommunityContributionReference? = nil
    ) {
        self.id = id
        self.stationId = stationId
        self.stationName = stationName
        self.municipalityCode = municipalityCode
        self.product = product
        self.amountMilli = amountMilli
        self.condition = condition
        self.confirmations = confirmations
        self.disputes = disputes
        self.helpfulCount = helpfulCount
        self.notHelpfulCount = notHelpfulCount
        self.observedAtLabel = observedAtLabel
        self.observedEpochSeconds = observedEpochSeconds
        self.hasPhotoEvidence = hasPhotoEvidence
        self.isOwnSubmission = isOwnSubmission
        self.contribution = contribution
    }
}

public enum CoordinatePinTier: String, Codable, Equatable, Sendable {
    case reviewed = "reviewed"
    case osmMapped = "osm-mapped"
    case anpReported = "anp-reported"
    case addressPoint = "address-point"
    case cityCentroid = "city-centroid"

    public var allowsMapPin: Bool {
        return self != .cityCentroid
    }

    public var localizedLabel: String {
        switch self {
        case .reviewed: return String(localized: "station.pin.reviewed", bundle: CoreLocalization.bundle)
        case .osmMapped: return String(localized: "station.pin.osm", bundle: CoreLocalization.bundle)
        case .anpReported: return String(localized: "station.pin.anp", bundle: CoreLocalization.bundle)
        case .addressPoint: return String(localized: "station.pin.address", bundle: CoreLocalization.bundle)
        case .cityCentroid: return String(localized: "station.pin.listOnly", bundle: CoreLocalization.bundle)
        }
    }
}

public struct StationQualitySummary: Codable, Equatable, Sendable {
    public let samplesTested: Int
    public let nonConformingCount: Int
    public let enforcementNotices: Int
    public let lastAuditLabel: String

    public init(samplesTested: Int, nonConformingCount: Int, enforcementNotices: Int, lastAuditLabel: String) {
        self.samplesTested = samplesTested
        self.nonConformingCount = nonConformingCount
        self.enforcementNotices = enforcementNotices
        self.lastAuditLabel = lastAuditLabel
    }
}

public struct StationRecord: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let cnpj: String
    public let name: String
    public let brand: String
    public let address: String
    public let neighborhood: String
    public let municipalityCode: String
    public let latitude: Double
    public let longitude: Double
    public let pinTier: CoordinatePinTier
    public let officialPricesMilli: [FuelProduct: Int64]
    public let officialDateLabel: String
    public let communityObservations: [CommunityPriceObservation]
    public let openingHours: String
    public let paymentMethods: [String]
    public let qualitySummary: StationQualitySummary
    public let isClaimedOwner: Bool
    public let ownerPromotionText: String?

    public init(
        id: String,
        cnpj: String,
        name: String,
        brand: String,
        address: String,
        neighborhood: String,
        municipalityCode: String,
        latitude: Double,
        longitude: Double,
        pinTier: CoordinatePinTier,
        officialPricesMilli: [FuelProduct: Int64],
        officialDateLabel: String,
        communityObservations: [CommunityPriceObservation],
        openingHours: String = "",
        paymentMethods: [String] = [],
        qualitySummary: StationQualitySummary = StationQualitySummary(samplesTested: 0, nonConformingCount: 0, enforcementNotices: 0, lastAuditLabel: ""),
        isClaimedOwner: Bool = false,
        ownerPromotionText: String? = nil
    ) {
        self.id = id
        self.cnpj = cnpj
        self.name = name
        self.brand = brand
        self.address = address
        self.neighborhood = neighborhood
        self.municipalityCode = municipalityCode
        self.latitude = latitude
        self.longitude = longitude
        self.pinTier = pinTier
        self.officialPricesMilli = officialPricesMilli
        self.officialDateLabel = officialDateLabel
        self.communityObservations = communityObservations
        self.openingHours = openingHours
        self.paymentMethods = paymentMethods
        self.qualitySummary = qualitySummary
        self.isClaimedOwner = isClaimedOwner
        self.ownerPromotionText = ownerPromotionText
    }

    public func officialPrice(for product: FuelProduct) -> Int64? {
        return officialPricesMilli[product]
    }

    public func latestCommunityPrice(for product: FuelProduct) -> CommunityPriceObservation? {
        return communityObservations
            .filter { $0.product == product }
            .sorted { $0.observedEpochSeconds > $1.observedEpochSeconds }
            .first
    }

    public func effectiveDisplayPrice(for product: FuelProduct) -> Int64? {
        return latestCommunityPrice(for: product)?.amountMilli ?? officialPrice(for: product)
    }

    public func distanceKm(fromLat: Double, fromLon: Double) -> Double {
        let earthRadiusKm = 6371.0
        let dLat = (latitude - fromLat) * .pi / 180.0
        let dLon = (longitude - fromLon) * .pi / 180.0
        let a = sin(dLat / 2) * sin(dLat / 2)
            + cos(fromLat * .pi / 180.0) * cos(latitude * .pi / 180.0) * sin(dLon / 2) * sin(dLon / 2)
        let c = 2 * atan2(sqrt(a), sqrt(max(0.0, 1.0 - a)))
        return earthRadiusKm * c
    }
}

public enum StationSortOrder: String, CaseIterable, Codable, Identifiable, Sendable {
    case lowestPrice = "LOWEST_PRICE"
    case mostRecent = "MOST_RECENT"
    case distance = "DISTANCE"
    case name = "NAME"

    public var id: String { rawValue }

    public var localizedLabel: String {
        switch self {
        case .lowestPrice: return String(localized: "station.order.lowest", bundle: CoreLocalization.bundle)
        case .mostRecent: return String(localized: "android.forum_sort_recent", bundle: CoreLocalization.bundle)
        case .distance: return String(localized: "station.order.closest", bundle: CoreLocalization.bundle)
        case .name: return String(localized: "station.order.name", bundle: CoreLocalization.bundle)
        }
    }
}

public enum FeedSortOrder: String, CaseIterable, Codable, Identifiable, Sendable {
    case recent = "RECENT"
    case cheapest = "CHEAPEST"

    public var id: String { rawValue }

    public var localizedLabel: String {
        switch self {
        case .recent: return String(localized: "android.forum_sort_recent", bundle: CoreLocalization.bundle)
        case .cheapest: return String(localized: "station.order.lowest", bundle: CoreLocalization.bundle)
        }
    }
}

public struct PriceHistoryPoint: Codable, Equatable, Identifiable, Sendable {
    public var id: String { "\(weekId)-\(product.rawValue)" }
    public let weekId: String
    public let weekLabel: String
    public let product: FuelProduct
    public let meanMilli: Int64
    public let minMilli: Int64
    public let maxMilli: Int64

    public init(weekId: String, weekLabel: String, product: FuelProduct, meanMilli: Int64, minMilli: Int64, maxMilli: Int64) {
        self.weekId = weekId
        self.weekLabel = weekLabel
        self.product = product
        self.meanMilli = meanMilli
        self.minMilli = minMilli
        self.maxMilli = maxMilli
    }
}

public struct QualityOccurrenceRecord: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let stationId: String
    public let stationName: String
    public let cityLabel: String
    public let kind: String
    public let fuelLabel: String
    public let summary: String
    public let dateLabel: String
    public let sourceLabel: String

    public init(
        id: String,
        stationId: String,
        stationName: String,
        cityLabel: String,
        kind: String,
        fuelLabel: String,
        summary: String,
        dateLabel: String,
        sourceLabel: String
    ) {
        self.id = id
        self.stationId = stationId
        self.stationName = stationName
        self.cityLabel = cityLabel
        self.kind = kind
        self.fuelLabel = fuelLabel
        self.summary = summary
        self.dateLabel = dateLabel
        self.sourceLabel = sourceLabel
    }
}

public struct StationSuggestionDraft: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let brand: String
    public let address: String
    public let municipalityCode: String
    public let cnpj: String
    public let isEvSite: Bool
    public let createdAtLabel: String

    public init(
        id: String = UUID().uuidString.lowercased(),
        name: String,
        brand: String,
        address: String,
        municipalityCode: String,
        cnpj: String,
        isEvSite: Bool,
        createdAtLabel: String
    ) {
        self.id = id
        self.name = name
        self.brand = brand
        self.address = address
        self.municipalityCode = municipalityCode
        self.cnpj = cnpj
        self.isEvSite = isEvSite
        self.createdAtLabel = createdAtLabel
    }
}

public struct StationCommentItem: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let stationId: String
    public let authorAlias: String
    public let text: String
    public let createdAtLabel: String
    public let validVotes: Int64
    public let invalidVotes: Int64
    public let revision: Int
    public let isOwn: Bool
    public let isReported: Bool

    public init(
        id: String = UUID().uuidString.lowercased(),
        stationId: String,
        authorAlias: String,
        text: String,
        createdAtLabel: String,
        validVotes: Int64 = 0,
        invalidVotes: Int64 = 0,
        revision: Int = 1,
        isOwn: Bool = false,
        isReported: Bool = false
    ) {
        self.id = id
        self.stationId = stationId
        self.authorAlias = authorAlias
        self.text = text
        self.createdAtLabel = createdAtLabel
        self.validVotes = validVotes
        self.invalidVotes = invalidVotes
        self.revision = revision
        self.isOwn = isOwn
        self.isReported = isReported
    }
}

public enum ForumPostKind: String, Codable, Equatable, Sendable {
    case editorial = "EDITORIAL"
    case community = "COMMUNITY"
}

public struct ForumCategoryItem: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let slug: String
    public let title: String
    public let postCount: Int

    public init(id: String, slug: String, title: String, postCount: Int) {
        self.id = id
        self.slug = slug
        self.title = title
        self.postCount = postCount
    }
}

public struct ForumCommentItem: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let topicId: String
    public let authorId: String
    public let authorName: String
    public let body: String
    public let createdAtLabel: String
    public let helpfulCount: Int
    public let isOwn: Bool

    public init(
        id: String = UUID().uuidString.lowercased(),
        topicId: String,
        authorId: String,
        authorName: String,
        body: String,
        createdAtLabel: String,
        helpfulCount: Int = 0,
        isOwn: Bool = false
    ) {
        self.id = id
        self.topicId = topicId
        self.authorId = authorId
        self.authorName = authorName
        self.body = body
        self.createdAtLabel = createdAtLabel
        self.helpfulCount = helpfulCount
        self.isOwn = isOwn
    }
}

public struct ForumTopicItem: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let categorySlug: String
    public let kind: ForumPostKind
    public let title: String
    public let summary: String
    public let body: String
    public let authorId: String
    public let authorName: String
    public let regionLabel: String
    public let createdAtLabel: String
    public let externalReferenceUrl: String?
    public let reactionCount: Int
    public let myReacted: Bool
    public let comments: [ForumCommentItem]

    public init(
        id: String,
        categorySlug: String,
        kind: ForumPostKind,
        title: String,
        summary: String,
        body: String,
        authorId: String,
        authorName: String,
        regionLabel: String,
        createdAtLabel: String,
        externalReferenceUrl: String? = nil,
        reactionCount: Int = 0,
        myReacted: Bool = false,
        comments: [ForumCommentItem] = []
    ) {
        self.id = id
        self.categorySlug = categorySlug
        self.kind = kind
        self.title = title
        self.summary = summary
        self.body = body
        self.authorId = authorId
        self.authorName = authorName
        self.regionLabel = regionLabel
        self.createdAtLabel = createdAtLabel
        self.externalReferenceUrl = externalReferenceUrl
        self.reactionCount = reactionCount
        self.myReacted = myReacted
        self.comments = comments
    }
}

public enum ForumSafeLinks {
    public static func isAllowedHttpsLink(_ urlText: String) -> Bool {
        let trimmed = urlText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = URL(string: trimmed),
              let scheme = url.scheme?.lowercased(),
              scheme == "https",
              let host = url.host,
              !host.isEmpty,
              url.user == nil,
              url.password == nil else {
            return false
        }
        return true
    }
}

public struct BlockedUserEntry: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let displayAlias: String
    public let blockedAtLabel: String

    public init(id: String, displayAlias: String, blockedAtLabel: String) {
        self.id = id
        self.displayAlias = displayAlias
        self.blockedAtLabel = blockedAtLabel
    }
}

public struct ContributorEntry: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let alias: String
    public let cityLabel: String
    public let acceptedPrices: Int
    public let confirmationsGiven: Int
    public let badgeTitle: String

    public init(
        id: String,
        alias: String,
        cityLabel: String,
        acceptedPrices: Int,
        confirmationsGiven: Int,
        badgeTitle: String
    ) {
        self.id = id
        self.alias = alias
        self.cityLabel = cityLabel
        self.acceptedPrices = acceptedPrices
        self.confirmationsGiven = confirmationsGiven
        self.badgeTitle = badgeTitle
    }
}

public struct MyContributionEntry: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let commandId: String
    public let stationId: String
    public let stationName: String
    public let product: FuelProduct
    public let amountMilli: Int64
    public let condition: PriceConditionKind
    public let statusLabel: String
    public let locationVerdict: String
    public let photoCacheId: String?
    public let photoExpiresAtMillis: Int64?
    public let createdAtLabel: String

    public init(
        id: String = UUID().uuidString.lowercased(),
        commandId: String,
        stationId: String,
        stationName: String,
        product: FuelProduct,
        amountMilli: Int64,
        condition: PriceConditionKind,
        statusLabel: String,
        locationVerdict: String,
        photoCacheId: String? = nil,
        photoExpiresAtMillis: Int64?,
        createdAtLabel: String
    ) {
        self.id = id
        self.commandId = commandId
        self.stationId = stationId
        self.stationName = stationName
        self.product = product
        self.amountMilli = amountMilli
        self.condition = condition
        self.statusLabel = statusLabel
        self.locationVerdict = locationVerdict
        self.photoCacheId = photoCacheId
        self.photoExpiresAtMillis = photoExpiresAtMillis
        self.createdAtLabel = createdAtLabel
    }
}

public struct VehicleRecord: Codable, Equatable, Identifiable, Sendable {
    public static let maxFreeVehicles = 1

    public let id: String
    public let name: String
    public let propulsion: Propulsion
    public let primaryFuel: FuelProduct
    public let tankCapacityMilliLiters: Int64
    public let batteryCapacityWh: Int64
    public let gasolineKmPerLiter: Double
    public let ethanolKmPerLiter: Double
    public let whPerKm: Int64
    public let chargeConnector: ChargingConnector?

    public init(
        id: String = UUID().uuidString.lowercased(),
        name: String,
        propulsion: Propulsion,
        primaryFuel: FuelProduct,
        tankCapacityMilliLiters: Int64,
        batteryCapacityWh: Int64 = 0,
        gasolineKmPerLiter: Double = 11.5,
        ethanolKmPerLiter: Double = 8.0,
        whPerKm: Int64 = 160,
        chargeConnector: ChargingConnector? = nil
    ) {
        self.id = id
        self.name = name
        self.propulsion = propulsion
        self.primaryFuel = primaryFuel
        self.tankCapacityMilliLiters = tankCapacityMilliLiters
        self.batteryCapacityWh = batteryCapacityWh
        self.gasolineKmPerLiter = gasolineKmPerLiter
        self.ethanolKmPerLiter = ethanolKmPerLiter
        self.whPerKm = whPerKm
        self.chargeConnector = chargeConnector
    }

    public func ethanolBreakEvenRatio() -> Double {
        guard gasolineKmPerLiter > 0, ethanolKmPerLiter > 0 else { return 0.70 }
        return ethanolKmPerLiter / gasolineKmPerLiter
    }
}

/// Exact integer rational math mirroring `PremiumBounds.kt` and `PremiumEnergy.kt`
/// and passing `contracts/testdata/premium/energy-v1.json`.
public enum PremiumEnergyCalculator {
    public static let maxEnergyWh: Int64 = 1_000_000_000_000
    public static let maxTotalCentavos: Int64 = 1_000_000_000_000
    public static let maxDistanceMetres: Int64 = 1_000_000_000_000

    public struct IntervalInput: Equatable {
        public let purchasedWh: Int64
        public let costCentavos: Int64
        public let distanceMetres: Int64
        public let hasEnergy: Bool
        public let complete: Bool
        public let hasGap: Bool
        public let crossesSegment: Bool
        public let odometerMissing: Bool
        public let socInconsistent: Bool
        public let mixedCarrier: Bool

        public init(
            purchasedWh: Int64,
            costCentavos: Int64,
            distanceMetres: Int64,
            hasEnergy: Bool,
            complete: Bool,
            hasGap: Bool = false,
            crossesSegment: Bool = false,
            odometerMissing: Bool = false,
            socInconsistent: Bool = false,
            mixedCarrier: Bool = false
        ) {
            self.purchasedWh = purchasedWh
            self.costCentavos = costCentavos
            self.distanceMetres = distanceMetres
            self.hasEnergy = hasEnergy
            self.complete = complete
            self.hasGap = hasGap
            self.crossesSegment = crossesSegment
            self.odometerMissing = odometerMissing
            self.socInconsistent = socInconsistent
            self.mixedCarrier = mixedCarrier
        }
    }

    public struct EnergyResult: Equatable {
        public let purchasedWh: Int64
        public let costCentavos: Int64
        public let distanceMetres: Int64
        public let whPerKm: String
        public let centavosPerKm: String
        public let unknown: String

        public init(
            purchasedWh: Int64 = 0,
            costCentavos: Int64 = 0,
            distanceMetres: Int64 = 0,
            whPerKm: String = "",
            centavosPerKm: String = "",
            unknown: String = ""
        ) {
            self.purchasedWh = purchasedWh
            self.costCentavos = costCentavos
            self.distanceMetres = distanceMetres
            self.whPerKm = whPerKm
            self.centavosPerKm = centavosPerKm
            self.unknown = unknown
        }
    }

    public static func compute(_ input: IntervalInput) -> EnergyResult {
        if input.purchasedWh < 0 || input.purchasedWh > maxEnergyWh
            || input.costCentavos < 0 || input.costCentavos > maxTotalCentavos
            || input.distanceMetres < 0 || input.distanceMetres > maxDistanceMetres {
            return EnergyResult(unknown: "input-out-of-bounds")
        }
        let unknownReason: String?
        if input.odometerMissing {
            unknownReason = "odometer-missing"
        } else if input.hasGap {
            unknownReason = "declared-fill-gap"
        } else if input.crossesSegment {
            unknownReason = "odometer-segment-boundary"
        } else if input.socInconsistent {
            unknownReason = "soc-inconsistent"
        } else if input.mixedCarrier {
            unknownReason = "phev-shared-distance"
        } else if input.distanceMetres <= 0 {
            unknownReason = "distance-nonpositive"
        } else if !input.hasEnergy {
            unknownReason = "energy-unmeasured"
        } else if !input.complete {
            unknownReason = "partial-measurement"
        } else {
            unknownReason = nil
        }
        if let reason = unknownReason {
            return EnergyResult(
                purchasedWh: input.purchasedWh,
                costCentavos: input.costCentavos,
                distanceMetres: input.distanceMetres,
                unknown: reason
            )
        }
        return EnergyResult(
            purchasedWh: input.purchasedWh,
            costCentavos: input.costCentavos,
            distanceMetres: input.distanceMetres,
            whPerKm: scaledRational(num: input.purchasedWh, scale: 1000, den: input.distanceMetres),
            centavosPerKm: scaledRational(num: input.costCentavos, scale: 1000, den: input.distanceMetres),
            unknown: ""
        )
    }

    public static func scaledRational(num: Int64, scale: Int64, den: Int64) -> String {
        if den <= 0 { return "" }
        let product = mulExact(Int128Pair.from(num), Int128Pair.from(scale))
        let absNum = product.magnitude
        let denVal = UInt64(den)
        let divisor = gcd(absNum, denVal)
        let nMag = divByUInt64(absNum, divisor)
        let dMag = denVal / divisor
        let sign = product.isNegative && !(nMag.hi == 0 && nMag.lo == 0) ? "-" : ""
        let numStr = "\(sign)\(formatU128(nMag))"
        return dMag == 1 ? numStr : "\(numStr)/\(dMag)"
    }

    private struct U128 {
        var hi: UInt64
        var lo: UInt64
    }

    private struct Int128Pair {
        let isNegative: Bool
        let magnitude: U128
        static func from(_ v: Int64) -> Int128Pair {
            if v >= 0 {
                return Int128Pair(isNegative: false, magnitude: U128(hi: 0, lo: UInt64(v)))
            }
            let mag = v == Int64.min ? UInt64(Int64.max) + 1 : UInt64(-v)
            return Int128Pair(isNegative: true, magnitude: U128(hi: 0, lo: mag))
        }
    }

    private static func mulExact(_ a: Int128Pair, _ b: Int128Pair) -> Int128Pair {
        let (hi, lo) = a.magnitude.lo.multipliedFullWidth(by: b.magnitude.lo)
        return Int128Pair(isNegative: a.isNegative != b.isNegative, magnitude: U128(hi: hi, lo: lo))
    }

    private static func modU128(_ n: U128, _ d: UInt64) -> UInt64 {
        let remHi = n.hi % d
        let combined = (remHi, n.lo)
        let (_, r) = d.dividingFullWidth(combined)
        return r
    }

    private static func divByUInt64(_ n: U128, _ d: UInt64) -> U128 {
        let qHi = n.hi / d
        let remHi = n.hi % d
        let (qLo, _) = d.dividingFullWidth((remHi, n.lo))
        return U128(hi: qHi, lo: qLo)
    }

    private static func gcd(_ a: U128, _ b: UInt64) -> UInt64 {
        var x = b
        var y = modU128(a, b)
        while y != 0 {
            let t = x % y
            x = y
            y = t
        }
        return max(1, x)
    }

    private static func formatU128(_ n: U128) -> String {
        if n.hi == 0 { return String(n.lo) }
        var current = n
        var digits: [Character] = []
        while current.hi != 0 || current.lo != 0 {
            let rem = modU128(current, 10)
            if let scalar = UnicodeScalar(48 + UInt32(rem)) {
                digits.append(Character(scalar))
            }
            current = divByUInt64(current, 10)
        }
        return String(digits.reversed())
    }
}

public struct DriverShiftRecord: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let vehicleId: String
    public let startedAtLabel: String
    public let endedAtLabel: String?
    public let startOdometerKm: Int64
    public let endOdometerKm: Int64?
    public let grossEarningsCentavos: Int64
    public let fuelCostCentavos: Int64
    public let otherExpenseCentavos: Int64
    public let activeMinutes: Int
    public let isActive: Bool

    public var distanceKm: Int64? {
        guard let end = endOdometerKm, startOdometerKm >= 0, end >= startOdometerKm else { return nil }
        return end - startOdometerKm
    }

    public var netEarningsCentavos: Int64? {
        let inputAmounts = [grossEarningsCentavos, fuelCostCentavos, otherExpenseCentavos]
        guard inputAmounts.allSatisfy({ (0...PortableMoney.maxRecordCentavos).contains($0) }) else { return nil }
        return grossEarningsCentavos - fuelCostCentavos - otherExpenseCentavos
    }

    public init(
        id: String = UUID().uuidString.lowercased(),
        vehicleId: String,
        startedAtLabel: String,
        endedAtLabel: String? = nil,
        startOdometerKm: Int64,
        endOdometerKm: Int64? = nil,
        grossEarningsCentavos: Int64 = 0,
        fuelCostCentavos: Int64 = 0,
        otherExpenseCentavos: Int64 = 0,
        activeMinutes: Int = 0,
        isActive: Bool = true
    ) {
        self.id = id
        self.vehicleId = vehicleId
        self.startedAtLabel = startedAtLabel
        self.endedAtLabel = endedAtLabel
        self.startOdometerKm = startOdometerKm
        self.endOdometerKm = endOdometerKm
        self.grossEarningsCentavos = grossEarningsCentavos
        self.fuelCostCentavos = fuelCostCentavos
        self.otherExpenseCentavos = otherExpenseCentavos
        self.activeMinutes = activeMinutes
        self.isActive = isActive
    }
}

public struct SavedPlaceRecord: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let subtitle: String
    public let latitude: Double
    public let longitude: Double
    public let stationId: String?

    public init(id: String = UUID().uuidString.lowercased(), title: String, subtitle: String, latitude: Double, longitude: Double, stationId: String? = nil) {
        self.id = id
        self.title = title
        self.subtitle = subtitle
        self.latitude = latitude
        self.longitude = longitude
        self.stationId = stationId
    }
}

public struct SavedTripRecord: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let originLabel: String
    public let destinationLabel: String
    public let distanceKm: Double

    public init(id: String = UUID().uuidString.lowercased(), title: String, originLabel: String, destinationLabel: String, distanceKm: Double) {
        self.id = id
        self.title = title
        self.originLabel = originLabel
        self.destinationLabel = destinationLabel
        self.distanceKm = distanceKm
    }
}

public struct TrainingConsentRecord: Codable, Equatable, Sendable {
    public let ownerKey: String
    public let origin: String
    public let granted: Bool
    public let pendingSync: Bool
    public let termsVersion: String
    public let termsDigest: String
    public let revision: Int64
    public let updatedAtLabel: String

    public init(
        ownerKey: String,
        origin: String,
        granted: Bool = false,
        pendingSync: Bool = false,
        termsVersion: String = TrainingTermsContract.version,
        termsDigest: String = TrainingTermsContract.digestSha256,
        revision: Int64 = 0,
        updatedAtLabel: String = ""
    ) {
        self.ownerKey = ownerKey
        self.origin = origin
        self.granted = granted
        self.pendingSync = pendingSync
        self.termsVersion = termsVersion
        self.termsDigest = termsDigest
        self.revision = revision
        self.updatedAtLabel = updatedAtLabel
    }
}

/// RVA08 owner-scoped training consent contract matching `training-consent-pt-BR.txt` byte-for-byte.
public enum TrainingTermsContract {
    public static let version = "training-terms-v1"
    public static let digestSha256 = "4a9763192620d267311f49fd43c9f4502144fa3bf78edc04b4112982e87bf8ce"

    public static let fullTextPtBr = """
ABASTEVO — CONSENTIMENTO OPCIONAL PARA MELHORAR O RECONHECIMENTO DE PREÇOS
Versão: training-terms-v1 | Idioma: português do Brasil | 12 de outubro de 2026

1. Responsável
O Abastevo é operado por ZAFLAS INOVA SIMPLES (I.S.), CNPJ 67.852.667/0001-93, com endereço na Rua Florenca, 922, Mont Serrat, Sorriso/MT, CEP 78891-109. Solicitações de privacidade podem ser encaminhadas por correspondência a esse endereço, aos cuidados de “Privacidade — Abastevo”. Ao solicitar atendimento, informe somente os dados necessários para localizar sua conta; nunca envie senha, chave privada ou códigos de acesso.

2. Sua escolha é livre
Esta autorização é opcional e começa desativada. Não é necessária para criar uma conta gratuita, consultar preços ou participar da comunidade. Abrir ou percorrer este documento não significa consentir. Para registrar uma autorização, é preciso marcar a caixa específica e confirmar o envio, com a conta identificada. Você pode recusar ou revogar sem prejuízo dessas funcionalidades.

3. Finalidade específica e situação atual
A finalidade proposta é melhorar o reconhecimento automático de preços e nomes de combustíveis em fotografias de placas de postos, por meio de avaliação e treinamento de modelos de reconhecimento. A coleta de imagens para treinamento está DESATIVADA nesta versão. Registrar uma preferência não ativa a coleta nem autoriza reutilizar automaticamente fotografias anteriores. A ativação dependerá da validação do tratamento das imagens e da publicação de uma nova versão destes termos, que exigirá nova autorização explícita. Não se declara aqui que essa validação ou a exclusão de todas as cópias operacionais em até 24 horas já foi comprovada em produção.

4. Limites do uso proposto
A autorização para treinamento é separada do processamento necessário para receber e verificar uma contribuição de preço. O uso para treinamento deverá se limitar a uma cópia privada, minimizada e sem identificação direta do colaborador, com remoção de metadados de localização e revisão de conteúdo identificável. Cada imagem dependerá também de uma seleção específica; o consentimento geral, sozinho, não seleciona uma fotografia. Nesta versão, essa seleção e a coleta permanecem bloqueadas. Imagens não serão publicadas como conjunto de treinamento, vendidas ou utilizadas para publicidade por esta autorização. Qualquer finalidade adicional exige informação e fundamento próprios.

5. Registro da decisão
Ao confirmar uma decisão, o serviço registra seu vínculo de proprietário, a finalidade, a versão e o resumo criptográfico deste texto, o resultado, uma revisão de ordenação e a data do servidor. Esses registros permitem verificar a decisão e impedir que uma autorização antiga substitua uma revogação mais recente. A preferência e uma solicitação pendente são armazenadas no dispositivo separadamente por conta, chave e servidor. A conta de outra pessoa não pode herdar nem enviar sua decisão.

6. Revogação e exclusão
Você pode desativar a preferência nas configurações, sem aceitar estes termos novamente. A desativação é gravada localmente antes de tentar a comunicação com o serviço. Sem conexão, o app informa que a solicitação está pendente; o recebimento pelo servidor será confirmado somente após resposta válida. Quando recebida, a revogação bloqueia novas seleções e inicia a exclusão dos materiais de treinamento vinculados. O recibo de solicitação não comprova a conclusão da exclusão física. A revogação não desfaz operações anteriores que tenham fundamento válido nem, sozinha, apaga os preços publicados na comunidade. Para excluir a conta ou solicitar outros direitos, use os controles correspondentes ou o canal de privacidade indicado na seção 1.

7. Direitos e transparência
Você pode solicitar confirmação do tratamento, acesso, correção, informações sobre compartilhamento e critérios de conservação, revogação e eliminação quando aplicável, observadas as obrigações legais e a verificação proporcional de identidade. O atendimento não depende de consentir com treinamento. Nenhum prazo de conservação de imagens de treinamento começa nesta versão, pois sua coleta está desativada. Antes de eventual ativação, serão informados os operadores, condições de conservação, medidas de minimização e eventuais transferências internacionais pertinentes, com nova decisão explícita.

8. Segurança e alterações
Os controles buscam restringir acesso e evitar reutilização indevida, sem promessa de risco zero. Não fotografe pessoas, documentos ou outros dados pessoais desnecessários. Uma alteração na finalidade ou no texto invalida a autorização anterior para a nova versão. Falha de conexão, documento divergente ou ausência de decisão válida mantém o uso para treinamento bloqueado.

Declaração opcional: “Li este documento e autorizo, de forma livre e específica, o registro da minha preferência para a finalidade descrita, ciente de que a coleta está desativada e de que sua futura ativação exigirá novos termos e nova autorização.”
"""

    /// The exact served bytes: the backend file ends with a newline, which a
    /// Swift multi-line literal omits.
    public static var canonicalBytes: Data { Data((fullTextPtBr + "\n").utf8) }

    public static func verifiedDigestMatches() -> Bool {
        let data = canonicalBytes
        let computed = IosDeviceProofSigner.sha256Hex(data)
        return computed == digestSha256
    }
}

public enum ArcadeGameKind: String, CaseIterable, Identifiable, Sendable {
    case fuelRun = "FUEL_RUN"
    case fuelFlight = "FUEL_FLIGHT"

    public var id: String { rawValue }
    public var localizedTitle: String {
        switch self {
        case .fuelRun: return String(localized: "android.games_run_title", bundle: CoreLocalization.bundle)
        case .fuelFlight: return String(localized: "android.games_flight_title", bundle: CoreLocalization.bundle)
        }
    }
}

public struct ArcadeState: Equatable {
    public var lane: Int
    public var altitude: Double
    public var score: Int
    public var fuelPercent: Double
    public var obstacleLane: Int
    public var obstacleProgress: Double
    public var pickupLane: Int
    public var pickupProgress: Double
    public var isGameOver: Bool

    public init(
        lane: Int = 1,
        altitude: Double = 0.5,
        score: Int = 0,
        fuelPercent: Double = 100.0,
        obstacleLane: Int = 0,
        obstacleProgress: Double = 0.0,
        pickupLane: Int = 2,
        pickupProgress: Double = 0.4,
        isGameOver: Bool = false
    ) {
        self.lane = lane
        self.altitude = altitude
        self.score = score
        self.fuelPercent = fuelPercent
        self.obstacleLane = obstacleLane
        self.obstacleProgress = obstacleProgress
        self.pickupLane = pickupLane
        self.pickupProgress = pickupProgress
        self.isGameOver = isGameOver
    }

    public mutating func step(kind: ArcadeGameKind, tick: Int) {
        if isGameOver { return }
        fuelPercent = max(0, fuelPercent - 0.8)
        obstacleProgress += 0.07
        pickupProgress += 0.06
        if kind == .fuelFlight {
            altitude = max(0.05, altitude - 0.03)
        }
        if obstacleProgress >= 0.85 && obstacleProgress <= 1.0 {
            let hit = kind == .fuelRun ? (lane == obstacleLane) : (abs(altitude - (Double(obstacleLane) * 0.35 + 0.15)) < 0.16)
            if hit {
                isGameOver = true
                return
            }
        }
        if pickupProgress >= 0.82 && pickupProgress <= 1.0 {
            let collected = kind == .fuelRun ? (lane == pickupLane) : (abs(altitude - (Double(pickupLane) * 0.35 + 0.15)) < 0.22)
            if collected {
                score += 25
                fuelPercent = min(100.0, fuelPercent + 20.0)
                pickupProgress = 0.0
                pickupLane = (tick + 2) % 3
            }
        }
        if obstacleProgress > 1.0 {
            obstacleProgress = 0.0
            obstacleLane = (tick * 2 + 1) % 3
            score += 10
        }
        if pickupProgress > 1.0 {
            pickupProgress = 0.0
            pickupLane = (tick + 1) % 3
        }
        if fuelPercent <= 0 {
            isGameOver = true
        }
    }
}
