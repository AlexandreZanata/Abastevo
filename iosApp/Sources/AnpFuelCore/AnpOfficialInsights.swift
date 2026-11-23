import Foundation

/// Pure official-data rules shared by the Explore card, history and alerts.
/// Swift port of Android `ActiveSurveyWeekRule`, `DefaultSurveyWeekRule`,
/// `DataReadinessRule`, `SurveyWeekFreshnessRule`, `PriceHistoryAvailabilityRule`,
/// `PriceHistoryOrderingRule`, `PriceHistoryTrendChart` bar scaling,
/// `PreviousSurveyWeekRule`, `PriceDropDetectionRule` (cheapest-station mode)
/// and the seven-day periodic `SyncWorkScheduler` cadence.
/// Zero postfix unwrap operators anywhere in this file.

public enum AnpDataReadiness: String, Equatable, Sendable {
    case empty
    case partial
    case ready
    case stale
}

public enum AnpCityAvailability: String, Equatable, Sendable {
    case hasData
    case noDataThisWeek
    case neverInAnp
}

public struct AnpCityHistoryEntry: Equatable, Identifiable, Sendable {
    public let week: AnpSurveyWeekRef
    public let averageMilli: Int64?
    public let minimumMilli: Int64?
    public let maximumMilli: Int64?
    public let stationCount: Int?

    public var id: String { week.id }

    public init(week: AnpSurveyWeekRef, averageMilli: Int64?, minimumMilli: Int64?, maximumMilli: Int64?, stationCount: Int?) {
        self.week = week
        self.averageMilli = averageMilli
        self.minimumMilli = minimumMilli
        self.maximumMilli = maximumMilli
        self.stationCount = stationCount
    }
}

public enum AnpCityHistory: Equatable, Sendable {
    /// Fewer than two imported weeks for this city and fuel (UC-006).
    case insufficient(weeks: Int)
    /// Newest week first.
    case available([AnpCityHistoryEntry])
}

public struct AnpWeekRows: Sendable {
    public let week: AnpSurveyWeekRef
    public let averages: [AnpMunicipalityAverage]
    public let stations: [AnpStationWeekPrice]

    public init(week: AnpSurveyWeekRef, averages: [AnpMunicipalityAverage], stations: [AnpStationWeekPrice] = []) {
        self.week = week
        self.averages = averages
        self.stations = stations
    }
}

public struct AnpPriceDrop: Equatable, Sendable {
    public let alertKey: String
    public let product: FuelProduct
    public let weekId: String
    public let currentMilli: Int64
    public let previousMilli: Int64
}

public enum AnpOfficialRules {
    public static let staleThresholdDays = 8
    public static let minimumHistoryWeeks = 2
    public static let periodicCheckSeconds: Int64 = 7 * 24 * 60 * 60

    /// Android `FuelProduct` declaration order, used to list averages.
    public static let productDisplayOrder: [FuelProduct] = [
        .ethanol, .gasolineRegular, .gasolinePremium, .dieselS500, .dieselS10, .cng, .lpgP13, .gasolinePremiumGrade,
    ]

    /// BR-006: the newest downloaded week by end date.
    public static func latestWeek(_ manifests: [AnpWeekManifest]) -> AnpSurveyWeekRef? {
        manifests.map(\.week).max()
    }

    /// BR-019: the chosen week while it is downloaded, otherwise the newest one.
    public static func displayWeek(activeWeekId: String, manifests: [AnpWeekManifest]) -> AnpSurveyWeekRef? {
        if let active = AnpSurveyWeekRef(id: activeWeekId), manifests.contains(where: { $0.week == active }) {
            return active
        }
        return latestWeek(manifests)
    }

    /// Data older than eight days after the survey end is stale.
    public static func isStale(weekEndIso: String, todayIso: String) -> Bool {
        guard let days = AnpCalendar.daysBetween(weekEndIso, todayIso) else { return false }
        return days >= staleThresholdDays
    }

    /// Readiness of the newest week. Station detail counts only for the selected state.
    public static func readiness(manifests: [AnpWeekManifest], scope: BrazilianState?, todayIso: String) -> AnpDataReadiness {
        guard let latest = manifests.max(by: { $0.week < $1.week }) else { return .empty }
        if !latest.hasStationData || (scope != nil && latest.stationScope != scope) { return .partial }
        if isStale(weekEndIso: latest.week.endIso, todayIso: todayIso) { return .stale }
        return .ready
    }

    public static func productOrder(_ product: FuelProduct) -> Int {
        productDisplayOrder.firstIndex(of: product) ?? productDisplayOrder.count
    }

    /// UC-005: one city's averages for a week, in product order.
    public static func cityAverages(_ averages: [AnpMunicipalityAverage], city: Municipality) -> [AnpMunicipalityAverage] {
        averages.filter { $0.matches(city) }.sorted { productOrder($0.product) < productOrder($1.product) }
    }

    public static func availability(
        displayWeek: AnpWeekRows,
        otherWeeks: [AnpWeekRows],
        city: Municipality
    ) -> AnpCityAvailability {
        if !cityAverages(displayWeek.averages, city: city).isEmpty { return .hasData }
        let seen = otherWeeks.contains { !cityAverages($0.averages, city: city).isEmpty }
        return seen ? .noDataThisWeek : .neverInAnp
    }

    /// UC-006 / BR-003: city history for one fuel, newest survey first.
    public static func history(weeks: [AnpWeekRows], city: Municipality, product: FuelProduct) -> AnpCityHistory {
        var entries: [AnpCityHistoryEntry] = []
        var seen = Set<String>()
        for rows in weeks {
            guard !seen.contains(rows.week.id),
                  let row = cityAverages(rows.averages, city: city).first(where: { $0.product == product }) else { continue }
            seen.insert(rows.week.id)
            entries.append(AnpCityHistoryEntry(
                week: rows.week,
                averageMilli: row.averageMilli,
                minimumMilli: row.minimumMilli,
                maximumMilli: row.maximumMilli,
                stationCount: row.stationCount
            ))
        }
        guard entries.count >= minimumHistoryWeeks else { return .insufficient(weeks: entries.count) }
        return .available(entries.sorted { $0.week.startIso > $1.week.startIso })
    }

    /// Android `PriceHistoryTrendChart`: 0.15 + 0.85 x (value - min) / range,
    /// with the range rounded half-up to four decimals and one real when flat.
    public static func trendBarFractions(_ values: [Int64]) -> [Double] {
        guard let minimum = values.min(), let maximum = values.max() else { return [] }
        let range = maximum > minimum ? maximum - minimum : 1_000
        return values.map { value in
            let scaled = (value - minimum) * 10_000
            let normalized = Double((scaled * 2 + range) / (range * 2)) / 10_000.0
            return 0.15 + min(1.0, max(0.0, normalized)) * 0.85
        }
    }

    /// Android `PreviousSurveyWeekRule`.
    public static func previousWeek(current: AnpSurveyWeekRef, weeks: [AnpSurveyWeekRef]) -> AnpSurveyWeekRef? {
        let ordered = Array(Set(weeks)).sorted(by: >)
        if let index = ordered.firstIndex(of: current), index + 1 < ordered.count {
            return ordered[index + 1]
        }
        return ordered.filter { $0.endIso < current.startIso }.max()
    }

    /// BR-025 cheapest-station price: the cheapest station of the city, else the city minimum.
    public static func comparisonPrice(rows: AnpWeekRows, city: Municipality, product: FuelProduct) -> Int64? {
        let cheapest = rows.stations
            .filter { $0.product == product && $0.matches(city) }
            .map(\.priceMilli)
            .min()
        if let cheapest { return cheapest }
        return cityAverages(rows.averages, city: city).first { $0.product == product }?.minimumMilli
    }

    public static func shouldNotify(current: Int64?, previous: Int64?) -> Bool {
        guard let current, let previous else { return false }
        return current < previous
    }

    public static func alertKey(_ product: FuelProduct) -> String {
        "price-drop:\(product.rawValue)"
    }

    /// UC-014: drops for the enabled fuels, once per alert and survey week.
    public static func priceDrops(
        products: [FuelProduct],
        city: Municipality,
        current: AnpWeekRows,
        previous: AnpWeekRows?,
        notifiedWeekByAlert: [String: String]
    ) -> [AnpPriceDrop] {
        guard let previous else { return [] }
        var drops: [AnpPriceDrop] = []
        for product in products {
            let key = alertKey(product)
            guard notifiedWeekByAlert[key] != current.week.id else { continue }
            let currentPrice = comparisonPrice(rows: current, city: city, product: product)
            let previousPrice = comparisonPrice(rows: previous, city: city, product: product)
            guard shouldNotify(current: currentPrice, previous: previousPrice),
                  let currentMilli = currentPrice, let previousMilli = previousPrice else { continue }
            drops.append(AnpPriceDrop(
                alertKey: key, product: product, weekId: current.week.id,
                currentMilli: currentMilli, previousMilli: previousMilli
            ))
        }
        return drops
    }

    /// Automatic weekly check: never ran, a week has passed, or the clock moved backwards.
    public static func shouldRunAutomaticCheck(lastCheckEpochSeconds: Int64?, nowEpochSeconds: Int64) -> Bool {
        guard let last = lastCheckEpochSeconds else { return true }
        return nowEpochSeconds < last || nowEpochSeconds - last >= periodicCheckSeconds
    }

    /// Downloaded station rows as dated official prices (collection date falls back to the week start).
    public static func officialPrices(_ rows: [AnpStationWeekPrice], week: AnpSurveyWeekRef) -> [OfficialStationPrice] {
        rows.map { row in
            OfficialStationPrice(
                product: row.product,
                amountMilli: row.priceMilli,
                collectedOn: row.collectedOnIso ?? week.startIso,
                surveyStart: week.startIso,
                surveyEnd: week.endIso
            )
        }
    }

    /// Official station prices for a directory station from a downloaded week, matched by CNPJ.
    public static func stationPrices(rows: AnpWeekRows, cnpj: String) -> [AnpStationWeekPrice] {
        let digits = AnpTextNormalizer.cnpjDigits(cnpj)
        guard !cnpj.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              digits != String(repeating: "0", count: 14) else { return [] }
        return rows.stations
            .filter { $0.cnpj == digits }
            .sorted { productOrder($0.product) < productOrder($1.product) }
    }
}
