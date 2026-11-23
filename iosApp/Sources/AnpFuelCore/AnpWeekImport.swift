import Foundation

/// Imported ANP week rows. Swift port of Android `WeeklySummarySheetParser`,
/// `StationDetailSheetParser` and `EntityDomainMapper` row mapping.
/// Unknown product or state labels are quarantined (counted, never guessed);
/// prices use Android `PriceAmount` half-up centavo rounding in integer milli-BRL.
/// Zero postfix unwrap operators anywhere in this file.

/// One municipality average from the weekly summary sheet ("MUNICIPIOS").
public struct AnpMunicipalityAverage: Codable, Equatable, Sendable {
    public let state: BrazilianState
    public let municipality: String
    public let normalizedMunicipality: String
    public let product: FuelProduct
    public let stationCount: Int?
    public let unit: String?
    public let averageMilli: Int64?
    public let minimumMilli: Int64?
    public let maximumMilli: Int64?

    public init(
        state: BrazilianState,
        municipality: String,
        product: FuelProduct,
        stationCount: Int?,
        unit: String?,
        averageMilli: Int64?,
        minimumMilli: Int64?,
        maximumMilli: Int64?
    ) {
        self.state = state
        self.municipality = municipality
        self.normalizedMunicipality = AnpTextNormalizer.municipality(municipality)
        self.product = product
        self.stationCount = stationCount
        self.unit = unit
        self.averageMilli = averageMilli
        self.minimumMilli = minimumMilli
        self.maximumMilli = maximumMilli
    }

    public func matches(_ city: Municipality) -> Bool {
        state == city.state && normalizedMunicipality == AnpTextNormalizer.municipality(city.name)
    }
}

/// One station price from the station detail sheet ("POSTOS REVENDEDORES").
public struct AnpStationWeekPrice: Codable, Equatable, Sendable {
    public let cnpj: String
    public let tradeName: String?
    public let legalName: String?
    public let brand: String?
    public let address: String
    public let municipality: String
    public let normalizedMunicipality: String
    public let state: BrazilianState
    public let product: FuelProduct
    public let priceMilli: Int64
    public let collectedOnIso: String?

    public init(
        cnpj: String,
        tradeName: String?,
        legalName: String?,
        brand: String?,
        address: String,
        municipality: String,
        state: BrazilianState,
        product: FuelProduct,
        priceMilli: Int64,
        collectedOnIso: String?
    ) {
        self.cnpj = cnpj
        self.tradeName = tradeName
        self.legalName = legalName
        self.brand = brand
        self.address = address
        self.municipality = municipality
        self.normalizedMunicipality = AnpTextNormalizer.municipality(municipality)
        self.state = state
        self.product = product
        self.priceMilli = priceMilli
        self.collectedOnIso = collectedOnIso
    }

    public var displayName: String {
        let trade = tradeName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !trade.isEmpty { return trade }
        let legal = legalName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return legal.isEmpty ? cnpj : legal
    }

    public func matches(_ city: Municipality) -> Bool {
        state == city.state && normalizedMunicipality == AnpTextNormalizer.municipality(city.name)
    }
}

public struct AnpSummaryImport: Equatable, Sendable {
    public let week: AnpSurveyWeekRef
    public let averages: [AnpMunicipalityAverage]
    public let quarantinedRows: Int
}

public struct AnpStationImport: Equatable, Sendable {
    public let stations: [AnpStationWeekPrice]
    public let quarantinedRows: Int
}

public enum AnpWeekImporter {
    public static let summarySheet = "MUNICIPIOS"
    public static let stationSheet = "POSTOS REVENDEDORES"
    public static let headerRow = 10
    public static let firstDataRow = 11
    public static let maxSummaryRows = 200_000
    public static let maxStationRows = 400_000

    /// Imports the weekly summary. The first data row fixes the survey week and must
    /// match the catalog week the file was downloaded for.
    public static func importSummary(xlsx: Data, expectedWeek: AnpSurveyWeekRef) throws -> AnpSummaryImport {
        var week: AnpSurveyWeekRef?
        var mismatch = false
        var quarantined = 0
        var seenRows = 0
        var byKey: [String: AnpMunicipalityAverage] = [:]
        var order: [String] = []
        do {
            try AnpXlsxReader.readSheet(
                xlsx: xlsx, sheetName: summarySheet, firstDataRow: firstDataRow, headerRow: headerRow
            ) { row in
                guard row.rowNumber >= AnpWeekImporter.firstDataRow else { return true }
                seenRows += 1
                guard seenRows <= AnpWeekImporter.maxSummaryRows else { return false }
                switch AnpWeekImporter.summaryRow(row) {
                case .skipped:
                    return true
                case .quarantined:
                    quarantined += 1
                    return true
                case .average(let rowWeek, let average):
                    if week == nil {
                        week = rowWeek
                        if rowWeek != expectedWeek {
                            mismatch = true
                            return false
                        }
                    }
                    guard rowWeek == week else {
                        quarantined += 1
                        return true
                    }
                    let key = "\(average.state.rawValue)|\(average.normalizedMunicipality)|\(average.product.rawValue)"
                    if byKey[key] == nil { order.append(key) }
                    byKey[key] = average
                    return true
                }
            }
        } catch {
            if mismatch { throw AnpXlsxError.weekMismatch }
            throw error
        }
        if mismatch { throw AnpXlsxError.weekMismatch }
        guard let importedWeek = week, !order.isEmpty else { throw AnpXlsxError.noDataRows }
        return AnpSummaryImport(
            week: importedWeek,
            averages: order.compactMap { byKey[$0] },
            quarantinedRows: quarantined
        )
    }

    /// Imports station prices of one state only (the downloaded scope).
    public static func importStations(xlsx: Data, scope: BrazilianState) throws -> AnpStationImport {
        var quarantined = 0
        var seenRows = 0
        var byKey: [String: AnpStationWeekPrice] = [:]
        var order: [String] = []
        try AnpXlsxReader.readSheet(
            xlsx: xlsx, sheetName: stationSheet, firstDataRow: firstDataRow, headerRow: headerRow
        ) { row in
            guard row.rowNumber >= AnpWeekImporter.firstDataRow else { return true }
            seenRows += 1
            guard seenRows <= AnpWeekImporter.maxStationRows else { return false }
            switch AnpWeekImporter.stationRow(row, scope: scope) {
            case .outOfScope:
                return true
            case .quarantined:
                quarantined += 1
                return true
            case .price(let price):
                let key = "\(price.cnpj)|\(price.product.rawValue)"
                if byKey[key] == nil { order.append(key) }
                byKey[key] = price
                return true
            }
        }
        return AnpStationImport(stations: order.compactMap { byKey[$0] }, quarantinedRows: quarantined)
    }

    enum SummaryRowOutcome: Equatable {
        case skipped
        case quarantined
        case average(AnpSurveyWeekRef, AnpMunicipalityAverage)
    }

    static func summaryRow(_ row: AnpSheetRow) -> SummaryRowOutcome {
        guard let label = row.cell(4), !label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              let start = AnpCalendar.excelSerialToIso(row.cell(0)),
              let end = AnpCalendar.excelSerialToIso(row.cell(1)) else {
            return .skipped
        }
        guard let week = AnpSurveyWeekRef(startIso: start, endIso: end),
              let state = AnpTextNormalizer.state(anpLabel: row.cell(2) ?? ""),
              let product = AnpTextNormalizer.fuelProduct(anpLabel: label) else {
            return .quarantined
        }
        let municipality = (row.cell(3) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !municipality.isEmpty else { return .quarantined }
        let unit = row.cell(6)?.trimmingCharacters(in: .whitespacesAndNewlines)
        let average = AnpMunicipalityAverage(
            state: state,
            municipality: municipality,
            product: product,
            stationCount: row.cell(5).flatMap { Int($0.trimmingCharacters(in: .whitespacesAndNewlines)) },
            unit: unit?.isEmpty == true ? nil : unit,
            averageMilli: AnpPriceRounding.milli(fromCell: row.cell(7)),
            minimumMilli: AnpPriceRounding.milli(fromCell: row.cell(9)),
            maximumMilli: AnpPriceRounding.milli(fromCell: row.cell(10))
        )
        return .average(week, average)
    }

    enum StationRowOutcome: Equatable {
        case outOfScope
        case quarantined
        case price(AnpStationWeekPrice)
    }

    static func stationRow(_ row: AnpSheetRow, scope: BrazilianState) -> StationRowOutcome {
        guard let state = AnpTextNormalizer.state(anpLabel: row.cell(9) ?? "") else { return .quarantined }
        guard state == scope else { return .outOfScope }
        guard let product = AnpTextNormalizer.fuelProduct(anpLabel: row.cell(11) ?? ""),
              let price = AnpPriceRounding.milli(fromCell: row.cell(13)) else {
            return .quarantined
        }
        let cnpj = AnpTextNormalizer.cnpjDigits(row.cell(0) ?? "")
        let municipality = (row.cell(8) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard cnpj != String(repeating: "0", count: 14), !municipality.isEmpty else { return .quarantined }
        return .price(AnpStationWeekPrice(
            cnpj: cnpj,
            tradeName: trimmedOrNil(row.cell(2)),
            legalName: trimmedOrNil(row.cell(1)),
            brand: trimmedOrNil(row.cell(10)),
            address: formatAddress(
                street: row.cell(3), number: row.cell(4), complement: row.cell(5), neighborhood: row.cell(6)
            ),
            municipality: municipality,
            state: state,
            product: product,
            priceMilli: price,
            collectedOnIso: AnpCalendar.excelSerialToIso(row.cell(14))
        ))
    }

    /// Android `EntityDomainMapper.formatStationAddress`.
    static func formatAddress(street: String?, number: String?, complement: String?, neighborhood: String?) -> String {
        var parts = [(street ?? "").trimmingCharacters(in: .whitespacesAndNewlines)]
        if let number = trimmedOrNil(number) { parts.append("nº \(number)") }
        if let complement = trimmedOrNil(complement) { parts.append(complement) }
        if let neighborhood = trimmedOrNil(neighborhood) { parts.append(neighborhood) }
        return parts.filter { !$0.isEmpty }.joined(separator: ", ")
    }

    private static func trimmedOrNil(_ text: String?) -> String? {
        guard let value = text?.trimmingCharacters(in: .whitespacesAndNewlines), !value.isEmpty else { return nil }
        return value
    }
}
