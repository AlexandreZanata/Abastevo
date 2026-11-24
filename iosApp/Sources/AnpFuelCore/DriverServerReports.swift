import Foundation

public enum DriverServerError: Error, Equatable, Sendable {
    case invalid
    case authRequired
    case ownerChanged
    case capabilityRequired
    case disabled
    case notFound
    case stale
    case reportTooLarge
    case conflict
    case unavailable(retryAfterSeconds: Int64?)
}

public enum DriverReportPeriod: String, Codable, CaseIterable, Hashable, Identifiable, Sendable {
    case day = "DAY"
    case week = "WEEK"
    case month = "MONTH"
    case custom = "CUSTOM"
    public var id: String { rawValue }
}

public enum DriverReportDistanceBasis: String, Codable, CaseIterable, Hashable, Identifiable, Sendable {
    case odometer = "ODOMETER"
    case gpsEstimate = "GPS_ESTIMATE"
    case paidReported = "PAID_REPORTED"
    case manualTrip = "MANUAL_TRIP"
    public var id: String { rawValue }
}

public struct DriverReportRequest: Codable, Equatable, Sendable {
    public let vehicleId: String
    public let timezone: String
    public let period: DriverReportPeriod
    public let anchor: String?
    public let from: String?
    public let to: String?
    public let platform: DriverPlatform?
    public let basis: DriverReportDistanceBasis
    public let expectedRevision: String?

    public init(vehicleId: String, timezone: String, period: DriverReportPeriod, anchor: String? = nil,
                from: String? = nil, to: String? = nil, platform: DriverPlatform? = nil,
                basis: DriverReportDistanceBasis, expectedRevision: String? = nil) {
        self.vehicleId = vehicleId
        self.timezone = timezone
        self.period = period
        self.anchor = anchor
        self.from = from
        self.to = to
        self.platform = platform
        self.basis = basis
        self.expectedRevision = expectedRevision
    }

    public func expecting(_ revision: String?) -> Self {
        Self(vehicleId: vehicleId, timezone: timezone, period: period, anchor: anchor, from: from, to: to,
             platform: platform, basis: basis, expectedRevision: revision)
    }

    public static func day(_ date: Date, timezone: String) -> String? {
        guard let zone = DriverServerWire.timezone(timezone) else { return nil }
        let calendar = DriverServerWire.calendar(in: zone)
        let fields = calendar.dateComponents([.year, .month, .day], from: date)
        guard let year = fields.year, let month = fields.month, let day = fields.day, (1...9_999).contains(year) else { return nil }
        return String(format: "%04d-%02d-%02d", year, month, day)
    }

    func resolve(now: Date) throws -> Resolved {
        guard DriverServerWire.validID(vehicleId), let zone = DriverServerWire.timezone(timezone),
              expectedRevision.map(DriverServerWire.validRevision) ?? true else { throw DriverServerError.invalid }
        let calendar = DriverServerWire.calendar(in: zone)
        if period == .custom {
            guard anchor == nil, let from, let to, let start = DriverServerWire.parseDay(from, calendar: calendar),
                  let end = DriverServerWire.parseDay(to, calendar: calendar), end > start,
                  end.timeIntervalSince(start) <= 366 * 24 * 60 * 60 else { throw DriverServerError.invalid }
            return Resolved(request: self, from: from, to: to)
        }
        guard from == nil, to == nil, let pinnedDay = anchor ?? Self.day(now, timezone: timezone),
              let selected = DriverServerWire.parseDay(pinnedDay, calendar: calendar) else { throw DriverServerError.invalid }
        let start: Date?
        let end: Date?
        switch period {
        case .day:
            start = selected
            end = calendar.date(byAdding: .day, value: 1, to: selected)
        case .week:
            let offset = (calendar.component(.weekday, from: selected) + 5) % 7
            start = calendar.date(byAdding: .day, value: -offset, to: selected)
            end = start.flatMap { calendar.date(byAdding: .day, value: 7, to: $0) }
        case .month:
            let month = calendar.dateComponents([.year, .month], from: selected)
            start = calendar.date(from: month)
            end = start.flatMap { calendar.date(byAdding: .month, value: 1, to: $0) }
        case .custom:
            throw DriverServerError.invalid
        }
        guard let start, let end, let startDay = Self.day(start, timezone: timezone),
              let endDay = Self.day(end, timezone: timezone) else { throw DriverServerError.invalid }
        let request = Self(vehicleId: vehicleId, timezone: timezone, period: period, anchor: pinnedDay,
                           platform: platform, basis: basis, expectedRevision: expectedRevision)
        return Resolved(request: request, from: startDay, to: endDay)
    }

    public func validate(now: Date = Date()) throws { _ = try resolve(now: now) }

    struct Resolved {
        let request: DriverReportRequest
        let from: String
        let to: String
    }
}

public struct DriverServerEarning: Decodable, Equatable, Identifiable, Sendable {
    public let id: String
    public let vehicleId: String
    public let platform: DriverPlatform
    public let coverage: DriverEarningCoverage
    public let currency: String
    public let from: String
    public let to: String
    public let timezone: String
    public let money: DriverEarningsMoney
    public let revision: Int64
    public let adjustmentCount: Int64
    public let needsReconciliation: Bool

    public var fromDate: Date? { DriverServerWire.instant(from) }
    public var toDate: Date? { DriverServerWire.instant(to) }
}

public struct DriverServerEarningsPage: Equatable, Sendable {
    public let items: [DriverServerEarning]
    public let nextCursor: String?
}

public struct DriverServerReport: Decodable, Equatable, Sendable {
    public let revision: String
    public let snapshotVersion: Int64
    public let vehicleId: String
    public let timezone: String
    public let period: DriverReportPeriod
    public let from: String
    public let to: String
    public let platform: DriverPlatform?
    public let basis: DriverReportDistanceBasis
    public let currency: String
    public let earnedCentavos: Int64?
    public let adjustmentsCentavos: Int64?
    public let costsCentavos: Int64?
    public let modeledCostsCentavos: Int64?
    public let recordedResultCentavos: Int64?
    public let modeledResultCentavos: Int64?
    public let distanceMetres: Int64?
    public let earningsPerKm: String
    public let recordedResultPerKm: String
    public let recordedResultPerHour: String
    public let modeledResultPerKm: String
    public let modeledResultPerHour: String
    public let ratiosUnknown: String
    public let coverage: [String: String]
    public let methods: [String: String]
    public let generatedAt: String

    public var generatedDate: Date? { DriverServerWire.instant(generatedAt) }

    public var dayRange: (Date, Date)? {
        guard let zone = DriverServerWire.timezone(timezone) else { return nil }
        let calendar = DriverServerWire.calendar(in: zone)
        guard let start = DriverServerWire.parseDay(from, calendar: calendar),
              let end = DriverServerWire.parseDay(to, calendar: calendar) else { return nil }
        return (start, end)
    }
}

enum DriverServerWire {
    static let maximumMoney: Int64 = 1_000_000_000_000
    static let maxLimit = 100
    private static var invalidResponse: DriverServerError { .unavailable(retryAfterSeconds: nil) }

    private struct WirePage: Decodable {
        let items: [DriverServerEarning]
        let nextCursor: String
    }

    static func validID(_ value: String) -> Bool {
        value == value.lowercased() && value != "00000000-0000-0000-0000-000000000000" && CommunityWire.isUuid(value)
    }

    static func validRevision(_ value: String) -> Bool {
        value.utf8.count == 64 && value.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }

    static func timezone(_ id: String) -> TimeZone? {
        guard id != "Local", (1...128).contains(id.utf8.count) else { return nil }
        return TimeZone(identifier: id)
    }

    static func calendar(in zone: TimeZone) -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.locale = Locale(identifier: "en_US_POSIX")
        calendar.timeZone = zone
        return calendar
    }

    static func parseDay(_ text: String, calendar: Calendar) -> Date? {
        let bytes = Array(text.utf8)
        guard bytes.count == 10, bytes[4] == 45, bytes[7] == 45,
              bytes.enumerated().allSatisfy({ index, byte in index == 4 || index == 7 || (48...57).contains(byte) }) else { return nil }
        let parts = text.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3, (1...9_999).contains(parts[0]), (1...12).contains(parts[1]), (1...31).contains(parts[2]),
              let day = calendar.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2])) else { return nil }
        let resolved = calendar.dateComponents([.year, .month, .day], from: day)
        guard resolved.year == parts[0], resolved.month == parts[1], resolved.day == parts[2] else { return nil }
        return day
    }

    static func instant(_ value: String) -> Date? {
        guard let millis = CommunityWire.millis(value) else { return nil }
        return Date(timeIntervalSince1970: Double(millis) / 1_000)
    }

    static func validCursor(_ token: String) -> Bool {
        (1...2_048).contains(token.utf8.count) && token.utf8.allSatisfy { byte in
            (48...57).contains(byte) || (65...90).contains(byte) || (97...122).contains(byte) || byte == 45 || byte == 95
        }
    }

    static func earningsFields(limit: Int, cursor: String?) throws -> [String: Any] {
        guard (1...maxLimit).contains(limit), cursor.map(validCursor) ?? true else { throw DriverServerError.invalid }
        var fields: [String: Any] = ["limit": limit]
        fields["cursor"] = cursor
        return fields
    }

    static func earnings(_ data: Data, limit: Int, cursor: String?) throws -> DriverServerEarningsPage {
        let page = try decode(WirePage.self, data)
        let ids = page.items.map(\.id)
        guard page.items.count <= limit,
              zip(ids, ids.dropFirst()).allSatisfy({ $0.0 < $0.1 }),
              page.nextCursor.isEmpty || (page.items.count == limit && page.nextCursor != cursor && validCursor(page.nextCursor)) else {
            throw invalidResponse
        }
        for row in page.items {
            guard validID(row.id), validID(row.vehicleId), row.currency == "BRL", timezone(row.timezone) != nil,
                  row.revision > 0, row.adjustmentCount >= 0, row.money.earnedCentavos != nil,
                  let start = row.fromDate, let end = row.toDate, end >= start else { throw invalidResponse }
        }
        return DriverServerEarningsPage(items: page.items, nextCursor: page.nextCursor.isEmpty ? nil : page.nextCursor)
    }

    static func reportFields(_ request: DriverReportRequest.Resolved) throws -> [String: Any] {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        guard let fields = try JSONSerialization.jsonObject(with: encoder.encode(request.request)) as? [String: Any] else {
            throw DriverServerError.invalid
        }
        return fields
    }

    static func report(_ data: Data, boundTo request: DriverReportRequest.Resolved? = nil) throws -> DriverServerReport {
        let report = try decode(DriverServerReport.self, data)
        guard validID(report.vehicleId), validRevision(report.revision), report.snapshotVersion > 0, report.currency == "BRL",
              let range = report.dayRange, range.1 > range.0, report.generatedDate != nil else { throw invalidResponse }
        let results = [report.earnedCentavos, report.adjustmentsCentavos, report.recordedResultCentavos, report.modeledResultCentavos]
        let nonnegative = [report.costsCentavos, report.modeledCostsCentavos, report.distanceMetres]
        let ratios = [report.earningsPerKm, report.recordedResultPerKm, report.recordedResultPerHour,
                      report.modeledResultPerKm, report.modeledResultPerHour]
        guard results.compactMap({ $0 }).allSatisfy({ (-maximumMoney...maximumMoney).contains($0) }),
              nonnegative.compactMap({ $0 }).allSatisfy({ (0...maximumMoney).contains($0) }),
              ratios.allSatisfy({ $0.isEmpty || LedgerReportedRatio($0) != nil }),
              report.ratiosUnknown.utf8.count <= 512,
              [report.coverage, report.methods].allSatisfy({ map in
                  map.count <= 24 && map.allSatisfy { $0.key.utf8.count <= 80 && $0.value.utf8.count <= 120 }
              }) else { throw invalidResponse }
        if let request {
            let selected = request.request
            guard report.vehicleId == selected.vehicleId, report.timezone == selected.timezone,
                  report.period == selected.period, report.from == request.from, report.to == request.to,
                  report.platform == selected.platform, report.basis == selected.basis,
                  selected.expectedRevision.map({ report.revision == $0 }) ?? true else { throw invalidResponse }
        }
        return report
    }

    private static func decode<T: Decodable>(_ type: T.Type, _ data: Data) throws -> T {
        do { return try AbastevoHttpClient.publicDecoder().decode(type, from: data) }
        catch { throw invalidResponse }
    }
}

public extension CommunityWriteClient {
    func driverEarnings(limit: Int = 20, cursor: String? = nil, scope: AccountDataExportScope) async throws -> DriverServerEarningsPage {
        let fields = try DriverServerWire.earningsFields(limit: limit, cursor: cursor)
        let response = try await driverServerPost("earnings", fields: fields, scope: scope)
        return try DriverServerWire.earnings(response, limit: limit, cursor: cursor)
    }

    func driverSavedReport(_ request: DriverReportRequest, scope: AccountDataExportScope) async throws -> DriverServerReport {
        let fixed = try request.resolve(now: Date())
        let response = try await driverServerPost("reports/get", fields: DriverServerWire.reportFields(fixed), scope: scope)
        return try DriverServerWire.report(response, boundTo: fixed)
    }

    /// A lost build response can be followed by a free retained read. The server versions each build.
    func buildDriverReport(_ request: DriverReportRequest, scope: AccountDataExportScope, operationId: String) async throws -> DriverServerReport {
        guard DriverServerWire.validID(operationId) else { throw DriverServerError.invalid }
        let fixed = try request.resolve(now: Date())
        let response = try await driverServerPost("reports/build", fields: DriverServerWire.reportFields(fixed),
                                                 scope: scope, operationId: operationId)
        return try DriverServerWire.report(response, boundTo: fixed)
    }

    private func driverServerPost(
        _ route: String, fields: [String: Any], scope: AccountDataExportScope, operationId: String? = nil
    ) async throws -> Data {
        guard transport.origin == scope.origin, transport.ownerScope() == scope.ownerScope else { throw DriverServerError.ownerChanged }
        do {
            let response = try await ownerPost(path: "/v1/driver/" + route, expectedScope: scope.ownerScope,
                                               requiresDeviceProof: true, idempotencyKey: operationId,
                                               requiresSameSessionFamily: true) { session in
                guard session.familyId == scope.sessionFamilyId else { return nil }
                var body = fields
                body["family_id"] = session.familyId
                body["access_token"] = session.accessToken
                return try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
            }
            guard response.status == 200 else { throw DriverServerError.unavailable(retryAfterSeconds: nil) }
            return response.body
        } catch let error as CommunityWriteError {
            switch error {
            case .signInRequired: throw DriverServerError.authRequired
            case .invalid: throw DriverServerError.ownerChanged
            default: throw DriverServerError.unavailable(retryAfterSeconds: nil)
            }
        } catch let error as SignedTransportError {
            switch error {
            case .sessionRequired:
                throw transport.ownerScope() == scope.ownerScope ? DriverServerError.authRequired : DriverServerError.ownerChanged
            case .refused(let status, let code, let hint):
                switch (status, code) {
                case (400, _): throw DriverServerError.invalid
                case (401, _): throw DriverServerError.authRequired
                case (403, _): throw DriverServerError.capabilityRequired
                case (404, "driver.disabled"): throw DriverServerError.disabled
                case (404, _): throw DriverServerError.notFound
                case (409, "driver.stale"): throw DriverServerError.stale
                case (409, "driver.report-too-large"): throw DriverServerError.reportTooLarge
                case (409, _): throw DriverServerError.conflict
                default: throw DriverServerError.unavailable(retryAfterSeconds: hint)
                }
            default: throw DriverServerError.unavailable(retryAfterSeconds: nil)
            }
        }
    }
}
