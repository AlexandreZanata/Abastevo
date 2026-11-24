import Foundation

public struct LedgerReportPeriod: Equatable, Sendable {
    public let year: Int
    public let month: Int
    public let timezone: String

    public init?(year: Int, month: Int, timezone: String) {
        guard (2000...2100).contains(year), (1...12).contains(month), timezone.utf8.count <= 64,
              TimeZone(identifier: timezone) != nil else { return nil }
        self.year = year
        self.month = month
        self.timezone = timezone
    }

    public var interval: DateInterval? {
        guard let zone = TimeZone(identifier: timezone) else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        guard let start = calendar.date(from: DateComponents(year: year, month: month, day: 1)),
              let end = calendar.date(byAdding: .month, value: 1, to: start) else { return nil }
        return DateInterval(start: start, end: end)
    }

    fileprivate var fields: [String: Any] { ["year": year, "month": month, "timezone": timezone] }
}

public struct LedgerServerReport: Decodable, Equatable, Sendable {
    public struct Totals: Decodable, Equatable, Sendable {
        public let purchaseCentavos: Int64
        public let expenseCentavos: Int64
        public let recordCount: Int64
        public let byVehicle: [String: Int64]
        public let byCategory: [String: Int64]
        public let coverage: [String: String]
        public let chargeEnergyWh: Int64?
        public let chargeCostCentavos: Int64?
        public let chargeFeesCentavos: Int64?
        public let energyCoverage: [String: String]?
        public let combinedCostPerKm: [String: String]?

        fileprivate var valid: Bool {
            [purchaseCentavos, expenseCentavos, recordCount].allSatisfy { $0 >= 0 }
                && [chargeEnergyWh, chargeCostCentavos, chargeFeesCentavos].compactMap { $0 }.allSatisfy { $0 >= 0 }
                && byVehicle.keys.allSatisfy(CommunityWire.isUuid)
                && byVehicle.values.allSatisfy { $0 >= 0 }
                && byCategory.values.allSatisfy { $0 >= 0 }
                && [coverage, energyCoverage ?? [:], combinedCostPerKm ?? [:]].allSatisfy {
                    $0.keys.allSatisfy(CommunityWire.isUuid) && $0.values.allSatisfy { $0.utf8.count <= 128 }
                }
        }
    }

    public let year: Int
    public let month: Int
    public let timezone: String
    public let totals: Totals
    public let revision: Int64
    public let generatedAt: String

    public var generatedDate: Date? { AbastevoHttpClient.readIsoDate(generatedAt) }
}

public struct LedgerComparisonSide: Equatable, Sendable {
    public let vehicleId: String
    public let from: Date
    public let to: Date
    public let timezone: String

    public init?(vehicleId: String, from: Date, to: Date, timezone: String) {
        let duration = to.timeIntervalSince(from)
        guard CommunityWire.isUuid(vehicleId), vehicleId != "00000000-0000-0000-0000-000000000000",
              from.timeIntervalSince1970.isFinite, to.timeIntervalSince1970.isFinite,
              duration > 0, duration <= 366 * 24 * 60 * 60, timezone.utf8.count <= 64,
              TimeZone(identifier: timezone) != nil else { return nil }
        self.vehicleId = vehicleId.lowercased()
        self.from = from
        self.to = to
        self.timezone = timezone
    }

    public init?(vehicleId: String, period: LedgerReportPeriod) {
        guard let interval = period.interval else { return nil }
        self.init(vehicleId: vehicleId, from: interval.start, to: interval.end, timezone: period.timezone)
    }

    fileprivate var fields: [String: String] {
        [
            "vehicle_id": vehicleId, "from": from.ISO8601Format(), "to": to.ISO8601Format(),
            "timezone": timezone, "method": "fuel-purchase-interval",
        ]
    }
}

/// The API returns ratios as exact rational text. Only presentation rounds; the wire value stays intact.
public struct LedgerReportedRatio: Equatable, Sendable {
    public let raw: String
    public let decimal: Decimal

    public init?(_ raw: String) {
        guard raw.utf8.count <= 96,
              raw.range(of: #"^-?[0-9]+(?:/[0-9]+)?$"#, options: .regularExpression) != nil else { return nil }
        let components = raw.split(separator: "/", omittingEmptySubsequences: false)
        let posix = Locale(identifier: "en_US_POSIX")
        guard let first = components.first, let top = Decimal(string: String(first), locale: posix) else { return nil }
        let denominator: Decimal
        if let last = components.dropFirst().first {
            guard let divisor = Decimal(string: String(last), locale: posix), divisor > 0 else { return nil }
            denominator = divisor
        } else {
            denominator = 1
        }
        let value = top / denominator
        guard !value.isNaN else { return nil }
        self.raw = raw
        decimal = value
    }
}

public struct LedgerServerComparison: Equatable, Sendable {
    public let compatible: Bool
    public let baseline: LedgerReportedRatio?
    public let current: LedgerReportedRatio?
    public let changePercent: LedgerReportedRatio?
    public let issue: String?
}

enum LedgerAnalysisWire {
    enum Route: String {
        case build = "/v1/ledger/reports/build"
        case get = "/v1/ledger/reports/get"
        case csv = "/v1/ledger/reports/csv"
        case compare = "/v1/metrics/compare"

        var requiresDeviceProof: Bool { self != .get }
    }

    static func periodBody(session: PortableAuth.Session, period: LedgerReportPeriod) -> Data? {
        body(session, fields: period.fields)
    }

    static func comparisonBody(session: PortableAuth.Session, baseline: LedgerComparisonSide, current: LedgerComparisonSide) -> Data? {
        body(session, fields: ["baseline": baseline.fields, "current": current.fields])
    }

    private static func body(_ session: PortableAuth.Session, fields: [String: Any]) -> Data? {
        guard LedgerSyncWire.isSessionUsable(session) else { return nil }
        var envelope = fields
        envelope["family_id"] = session.familyId
        envelope["access_token"] = session.accessToken
        return DeviceIdentityWire.encode(envelope)
    }

    static func report(_ data: Data, period: LedgerReportPeriod) throws -> LedgerServerReport {
        guard let report = try? AbastevoHttpClient.publicDecoder().decode(LedgerServerReport.self, from: data),
              report.year == period.year, report.month == period.month, report.timezone == period.timezone,
              report.revision > 0, report.generatedDate != nil, report.totals.valid else {
            throw LedgerSyncError.invalidReceipt
        }
        return report
    }

    static func csv(_ data: Data, period: LedgerReportPeriod) throws -> Data {
        let header = "year,month,timezone,revision,metric,value\n"
        let monthPrefix = "\(period.year),\(period.month),\(period.timezone),"
        guard data.count <= 512 * 1024, let text = String(data: data, encoding: .utf8),
              text.hasPrefix(header), text.hasSuffix("\n"),
              !text.unicodeScalars.contains(where: { $0.value < 32 && $0.value != 10 }),
              !text.dropFirst(header.count).isEmpty,
              text.dropFirst(header.count).split(separator: "\n").allSatisfy({ $0.hasPrefix(monthPrefix) }) else {
            throw LedgerSyncError.invalidReceipt
        }
        return data
    }

    static func comparison(_ data: Data) throws -> LedgerServerComparison {
        struct Wire: Decodable {
            let compatible: Bool
            let baseline: String?
            let current: String?
            let changePercent: String?
            let issue: String?
        }
        guard let receipt = try? AbastevoHttpClient.publicDecoder().decode(Wire.self, from: data),
              let issue = receipt.issue, issue.utf8.count <= 128 else { throw LedgerSyncError.invalidReceipt }
        if !receipt.compatible {
            guard !issue.isEmpty else { throw LedgerSyncError.invalidReceipt }
            return LedgerServerComparison(compatible: false, baseline: nil, current: nil, changePercent: nil, issue: issue)
        }
        guard let rawBaseline = receipt.baseline, let baseline = LedgerReportedRatio(rawBaseline), baseline.decimal > 0,
              let rawCurrent = receipt.current, let current = LedgerReportedRatio(rawCurrent), current.decimal > 0 else {
            throw LedgerSyncError.invalidReceipt
        }
        let change = receipt.changePercent.flatMap(LedgerReportedRatio.init)
        guard change != nil || (!issue.isEmpty && (receipt.changePercent ?? "").isEmpty) else {
            throw LedgerSyncError.invalidReceipt
        }
        return LedgerServerComparison(compatible: true, baseline: baseline, current: current, changePercent: change,
                                      issue: issue.isEmpty ? nil : issue)
    }
}
