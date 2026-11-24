import Foundation
import XCTest
@testable import AnpFuelCore

final class LedgerServerAnalysisTests: XCTestCase {
    private let familyId = "11111111-1111-4111-8111-111111111111"
    private let baselineId = "22222222-2222-4222-8222-222222222222"
    private let currentId = "33333333-3333-4333-8333-333333333333"

    private func fixture(_ name: String) throws -> [String: Any] {
        #if SWIFT_PACKAGE
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { url.deleteLastPathComponent() }
        url = url.appendingPathComponent("contracts/testdata/api/\(name).json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        let doc = try XCTUnwrap(DeviceIdentityWire.object(Data(contentsOf: url)))
        return try XCTUnwrap(doc["value"] as? [String: Any])
    }

    private var session: PortableAuth.Session {
        PortableAuth.Session(familyId: familyId, accountId: baselineId, accessToken: "synthetic", refreshToken: "fixture",
                             accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000)
    }

    func testLedgerReportPeriodAndRequestReplay() throws {
        let period = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 3, timezone: "America/Sao_Paulo"))
        let body = try XCTUnwrap(LedgerAnalysisWire.periodBody(session: session, period: period))
        let expected = try fixture("ledger-report-valid")
        XCTAssertEqual(DeviceIdentityWire.encode(expected), body)
        let invalid = try fixture("ledger-report-invalid")
        XCTAssertNil(LedgerReportPeriod(year: 1999, month: 3, timezone: period.timezone))
        XCTAssertNil(LedgerReportPeriod(year: 2026, month: 13, timezone: period.timezone))
        XCTAssertNil(LedgerReportPeriod(year: 2026, month: 3, timezone: "unverified/timezone"))
        XCTAssertNotEqual(DeviceIdentityWire.encode(invalid), body)
        XCTAssertEqual(period.interval?.start.ISO8601Format(), "2026-03-01T03:00:00Z")

        let change = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 3, timezone: "America/New_York"))
        XCTAssertEqual(change.interval?.duration, TimeInterval(31 * 86_400 - 3600))
    }

    func testMetricsCompareVerbatimVectorsAndIncompleteReceipt() throws {
        let january = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 1, timezone: "UTC"))
        let february = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 2, timezone: "UTC"))
        let a = try XCTUnwrap(LedgerComparisonSide(vehicleId: baselineId, period: january))
        let b = try XCTUnwrap(LedgerComparisonSide(vehicleId: currentId, period: february))
        let request = try XCTUnwrap(LedgerAnalysisWire.comparisonBody(session: session, baseline: a, current: b))
        XCTAssertEqual(request, try DeviceIdentityWire.encode(fixture("metrics-compare-valid")))
        XCTAssertNotEqual(request, try DeviceIdentityWire.encode(fixture("metrics-compare-invalid")))
        XCTAssertNil(LedgerComparisonSide(vehicleId: baselineId, from: a.to, to: a.from, timezone: "UTC"))
        XCTAssertNil(LedgerComparisonSide(vehicleId: baselineId, from: a.from, to: a.from.addingTimeInterval(367 * 86_400),
                                         timezone: "UTC"))

        let reply = try XCTUnwrap(DeviceIdentityWire.encode(fixture("metrics-compare-response-valid")))
        let comparison = try LedgerAnalysisWire.comparison(reply)
        XCTAssertTrue(comparison.compatible)
        XCTAssertEqual(comparison.baseline?.raw, "10")
        XCTAssertEqual(comparison.current?.raw, "100/7")
        XCTAssertEqual(comparison.changePercent?.raw, "300/7")

        let incomplete = Data(#"{"compatible":false,"baseline":"999","current":"0","change_percent":"-1","issue":"baseline-incomplete: declared-fill-gap"}"#.utf8)
        let result = try LedgerAnalysisWire.comparison(incomplete)
        XCTAssertFalse(result.compatible)
        XCTAssertEqual(result.issue, "baseline-incomplete: declared-fill-gap")
        XCTAssertNil(result.baseline)
        XCTAssertNil(result.current)
        XCTAssertNil(result.changePercent)

        let unequal = Data(#"{"compatible":true,"baseline":"10","current":"100/7","change_percent":"","issue":"unmatched-period-window"}"#.utf8)
        XCTAssertNil(try LedgerAnalysisWire.comparison(unequal).changePercent)
        XCTAssertNil(LedgerReportedRatio("10/0"))
        XCTAssertNil(LedgerReportedRatio("NaN"))
    }

    func testReadAndCsvRequireMatchingPeriodAndBoundedReport() throws {
        let period = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 3, timezone: "UTC"))
        let report = Data("""
        {"year":2026,"month":3,"timezone":"UTC","revision":2,"generated_at":"2026-04-01T12:00:00Z",
         "totals":{"record_count":1,"purchase_centavos":8000,"expense_centavos":0,
                   "by_vehicle":{"\(baselineId)":8000},"by_category":{},"coverage":{"\(baselineId)":"declared-fill-gap"}}}
        """.utf8)
        let result = try LedgerAnalysisWire.report(report, period: period)
        XCTAssertEqual(result.totals.byVehicle[baselineId], 8000)
        XCTAssertEqual(result.totals.coverage[baselineId], "declared-fill-gap")
        XCTAssertNil(result.totals.chargeEnergyWh)
        let other = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 4, timezone: "UTC"))
        XCTAssertThrowsError(try LedgerAnalysisWire.report(report, period: other))

        let csv = Data("year,month,timezone,revision,metric,value\n2026,3,UTC,2,purchase_centavos,8000\n".utf8)
        XCTAssertEqual(try LedgerAnalysisWire.csv(csv, period: period), csv)
        XCTAssertThrowsError(try LedgerAnalysisWire.csv(csv, period: other))
        XCTAssertThrowsError(try LedgerAnalysisWire.csv(Data("<html>".utf8), period: period))
    }

    func testExpectedScopeAndFamilyAreRequiredForAnyReportRoute() async throws {
        let sessions = StaticSessions(session: session)
        let exchange = ScriptedExchange { _ in .init(503, "{}") }
        let transport = SignedRequestTransport(origin: "https://abastevo.com.br", signer: try IdentityVectors.signer(),
                                               identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true),
                                               exchange: exchange, sessions: sessions)
        let client = LedgerSyncClient(transport: transport, sessions: sessions)
        let owner = try XCTUnwrap(transport.ownerScope())
        let wrong = try XCTUnwrap(AccountDataExportScope(
            origin: transport.origin, ownerScope: owner, sessionFamilyId: currentId
        ))
        let period = try XCTUnwrap(LedgerReportPeriod(year: 2026, month: 3, timezone: "UTC"))
        do {
            _ = try await client.monthlyReport(period, accountId: baselineId, scope: wrong)
            XCTFail("Refuse an earlier family")
        } catch {
            XCTAssertEqual(error as? LedgerSyncError, .ownerMismatch)
        }
        XCTAssertTrue(exchange.requests.isEmpty)

        let proper = try XCTUnwrap(AccountDataExportScope(origin: transport.origin, ownerScope: owner, sessionFamilyId: familyId))
        do {
            _ = try await client.monthlyReport(period, accountId: baselineId, scope: proper)
            XCTFail("Expect server refusal")
        } catch {
            XCTAssertEqual(error as? SignedTransportError, .refused(status: 503, code: nil, retryAfterSeconds: nil))
        }
        XCTAssertEqual(exchange.requests.count, 1)
        XCTAssertEqual(exchange.requests.first?.url?.path, "/v1/ledger/reports/get")
        XCTAssertNil(exchange.requests.first?.value(forHTTPHeaderField: "Signature"))
    }
}
