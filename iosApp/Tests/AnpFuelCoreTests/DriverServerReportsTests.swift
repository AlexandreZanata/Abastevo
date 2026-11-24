import Foundation
import XCTest
@testable import AnpFuelCore

final class DriverServerReportsTests: XCTestCase {
    private static let origin = "https://abastevo.com.br"
    private static let family = "44444444-4444-4444-8444-444444444444"
    private static let account = "a0000000-0000-4000-8000-000000000001"
    private static let vehicle = "22222222-2222-4222-8222-222222222222"
    private static let operation = "99999999-9999-4999-8999-999999999999"
    private static let row = "11111111-1111-4111-8111-111111111111"
    private static let otherRow = "22222222-2222-4222-8222-222222222222"
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func fixture(_ name: String) throws -> [String: Any] {
        #if SWIFT_PACKAGE
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { root.deleteLastPathComponent() }
        let url = root.appendingPathComponent("contracts/testdata/api").appendingPathComponent(name).appendingPathExtension("json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        let doc = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
        return try XCTUnwrap(doc["value"] as? [String: Any])
    }

    private func encoded(_ object: [String: Any]) throws -> Data {
        try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys])
    }

    private func request(platform: DriverPlatform? = nil, revision: String? = nil) -> DriverReportRequest {
        DriverReportRequest(vehicleId: Self.vehicle, timezone: "America/Cuiaba", period: .day, anchor: "2026-10-26",
                            platform: platform, basis: .odometer, expectedRevision: revision)
    }

    private func signedClient(
        _ respond: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply
    ) throws -> (CommunityWriteClient, ScriptedExchange, AccountDataExportScope) {
        let sessions = StaticSessions(session: PortableAuth.Session(
            familyId: Self.family, accountId: Self.account, accessToken: "fixture-access", refreshToken: "fixture-refresh",
            accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000
        ))
        let exchange = ScriptedExchange { FakeIdentityBackend.reply($0, fallback: respond) }
        let transport = SignedRequestTransport(
            origin: Self.origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange, sessions: sessions,
            nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let owner = try XCTUnwrap(transport.ownerScope())
        let scope = try XCTUnwrap(AccountDataExportScope(origin: Self.origin, ownerScope: owner, sessionFamilyId: Self.family))
        return (client, exchange, scope)
    }

    func testSharedRequestAndReportFixturesReplayExactFieldsAndUnknownValues() throws {
        var valid = try fixture("driver-report-request-valid")
        XCTAssertEqual(valid.removeValue(forKey: "family_id") as? String, Self.family)
        XCTAssertNotNil(valid.removeValue(forKey: "access_token"))
        let decoder = AbastevoHttpClient.publicDecoder()
        let request = try decoder.decode(DriverReportRequest.self, from: encoded(valid))
        let resolved = try request.resolve(now: now)
        let fields = try DriverServerWire.reportFields(resolved)
        XCTAssertEqual(try encoded(fields), try encoded(valid))
        XCTAssertEqual(resolved.from, "2026-10-26")
        XCTAssertEqual(resolved.to, "2026-10-27")
        let invalid = try fixture("driver-report-request-invalid")
        XCTAssertThrowsError(try decoder.decode(DriverReportRequest.self, from: encoded(invalid)))
        let oversized = try fixture("driver-earnings-page-request-invalid")
        XCTAssertThrowsError(try DriverServerWire.earningsFields(limit: XCTUnwrap(oversized["limit"] as? Int), cursor: nil))
        let data = try fixture("driver-report-response-valid")
        let report = try DriverServerWire.report(encoded(data))
        XCTAssertEqual(report.snapshotVersion, 1)
        XCTAssertEqual(report.earnedCentavos, 45_000)
        XCTAssertEqual(report.modeledResultCentavos, 15_000)
        XCTAssertEqual(report.recordedResultCentavos, 25_000)
        XCTAssertEqual(report.distanceMetres, 500_000)
        XCTAssertEqual(report.earningsPerKm, "90")
        XCTAssertTrue(report.recordedResultPerHour.isEmpty)
        XCTAssertEqual(report.ratiosUnknown, "time-missing")
        var unknown = data
        unknown["distance_metres"] = NSNull()
        unknown["earnings_per_km"] = ""
        unknown["recorded_result_per_km"] = ""
        unknown["modeled_result_per_km"] = ""
        let unknownReport = try DriverServerWire.report(encoded(unknown))
        XCTAssertNil(unknownReport.distanceMetres)
        unknown["earnings_per_km"] = "1/0"
        XCTAssertThrowsError(try DriverServerWire.report(encoded(unknown)))
    }

    func testReportPeriodMondayCustomBoundAndExactResponseIdentity() throws {
        let week = DriverReportRequest(vehicleId: Self.vehicle, timezone: "America/Cuiaba", period: .week,
                                       anchor: "2026-10-25", basis: .gpsEstimate)
        let weekly = try week.resolve(now: now)
        XCTAssertEqual(weekly.from, "2026-10-19")
        XCTAssertEqual(weekly.to, "2026-10-26")
        let month = DriverReportRequest(vehicleId: Self.vehicle, timezone: "America/Cuiaba", period: .month,
                                        anchor: "2026-12-31", basis: .manualTrip)
        let monthly = try month.resolve(now: now)
        XCTAssertEqual(monthly.from, "2026-12-01")
        XCTAssertEqual(monthly.to, "2027-01-01")
        let over = DriverReportRequest(vehicleId: Self.vehicle, timezone: "UTC", period: .custom,
                                       from: "2026-01-01", to: "2027-01-03", basis: .odometer)
        XCTAssertThrowsError(try over.validate(now: now))
        let impossible = DriverReportRequest(vehicleId: Self.vehicle, timezone: "UTC", period: .day,
                                              anchor: "2026-02-31", basis: .odometer)
        XCTAssertThrowsError(try impossible.validate(now: now))
        let payload = try encoded(fixture("driver-report-response-valid"))
        XCTAssertThrowsError(try DriverServerWire.report(payload, boundTo: weekly))
        let expectedRevision = String(repeating: "b", count: 64)
        let matching = try request(platform: DriverPlatform(rawValue: "UBER"), revision: expectedRevision).resolve(now: now)
        XCTAssertNoThrow(try DriverServerWire.report(payload, boundTo: matching))
        XCTAssertThrowsError(try DriverServerWire.report(payload, boundTo: request().resolve(now: now)))
    }

    func testSignedFreeReadsAndStableReportBuildOperation() async throws {
        let report = String(decoding: try encoded(fixture("driver-report-response-valid")), as: UTF8.self)
        let (client, exchange, scope) = try signedClient { request in
            if request.url?.path == "/v1/driver/earnings" {
                return .init(200, #"{"items":[],"next_cursor":""}"#)
            }
            return .init(200, report)
        }
        let page = try await client.driverEarnings(scope: scope)
        XCTAssertTrue(page.items.isEmpty)
        XCTAssertNil(page.nextCursor)
        let query = request(platform: DriverPlatform(rawValue: "UBER"))
        let saved = try await client.driverSavedReport(query, scope: scope)
        let build1 = try await client.buildDriverReport(query, scope: scope, operationId: Self.operation)
        let build2 = try await client.buildDriverReport(query, scope: scope, operationId: Self.operation)
        XCTAssertEqual(saved, build1)
        XCTAssertEqual(build1, build2)
        let calls = exchange.requests.filter { $0.url?.path.hasPrefix("/v1/driver/") == true }
        XCTAssertEqual(calls.compactMap { $0.url?.path }, [
            "/v1/driver/earnings", "/v1/driver/reports/get", "/v1/driver/reports/build", "/v1/driver/reports/build",
        ])
        for call in calls {
            XCTAssertEqual(call.httpMethod, "POST")
            XCTAssertNotNil(call.value(forHTTPHeaderField: "Signature"))
            XCTAssertNil(call.url?.query)
            let body = FakeIdentityBackend.json(call)
            XCTAssertEqual(body["family_id"] as? String, Self.family)
            XCTAssertEqual(body["access_token"] as? String, "fixture-access")
            for secret in ["account_id", "owner", "latitude", "longitude", "refresh_token"] { XCTAssertNil(body[secret]) }
            XCTAssertEqual(call.value(forHTTPHeaderField: "Idempotency-Key"),
                           call.url?.lastPathComponent == "build" ? Self.operation : nil)
        }
        let builds = calls.filter { $0.url?.lastPathComponent == "build" }
        XCTAssertEqual(builds.first?.httpBody, builds.last?.httpBody)
    }

    func testOwnerPageRejectsReplayCursorAndKeepsExactMoney() throws {
        let reply = #"{"items":[{"id":"\#(Self.row)","vehicle_id":"\#(Self.vehicle)","platform":"99","coverage":"SUMMARY","currency":"BRL","from":"2026-10-26T12:00:00Z","to":"2026-10-26T13:00:00Z","timezone":"America/Cuiaba","money":{"basis":"INCLUSIVE_NET","net":45000},"revision":2,"adjustment_count":1,"needs_reconciliation":true}],"next_cursor":"synthetic-token"}"#
        let payload = Data(reply.utf8)
        let page = try DriverServerWire.earnings(payload, limit: 1, cursor: nil)
        let row = try XCTUnwrap(page.items.first)
        XCTAssertEqual(row.money.earnedCentavos, 45_000)
        XCTAssertNil(row.money.gross)
        XCTAssertTrue(row.needsReconciliation)
        XCTAssertEqual(page.nextCursor, "synthetic-token")
        XCTAssertThrowsError(try DriverServerWire.earnings(payload, limit: 1, cursor: "synthetic-token"))
        XCTAssertThrowsError(try DriverServerWire.earnings(payload, limit: 2, cursor: nil))
        XCTAssertThrowsError(try DriverServerWire.earningsFields(limit: 20, cursor: "bad=cursor"))
    }

    func testDisabledStaleAndSourceSizeRemainDistinctFromUnavailable() async throws {
        let refusals: [(Int, String, DriverServerError)] = [
            (400, "driver.invalid", .invalid), (401, "driver.auth-required", .authRequired),
            (403, "driver.capability-required", .capabilityRequired), (404, "driver.disabled", .disabled),
            (404, "driver.not-found", .notFound), (409, "driver.stale", .stale),
            (409, "driver.report-too-large", .reportTooLarge), (409, "driver.conflict", .conflict),
            (408, "driver.unavailable", .unavailable(retryAfterSeconds: 30)),
            (429, "driver.unavailable", .unavailable(retryAfterSeconds: 30)),
            (503, "driver.unavailable", .unavailable(retryAfterSeconds: 30)),
        ]
        for (status, code, expected) in refusals {
            let make = try signedClient { _ in .init(status, #"{"error":{"code":"\#(code)"}}"#, headers: ["Retry-After": "30"]) }
            do {
                _ = try await make.0.driverSavedReport(request(), scope: make.2)
                XCTFail("Expected \(code)")
            } catch {
                XCTAssertEqual(error as? DriverServerError, expected)
            }
            XCTAssertEqual(make.1.requests.filter { $0.url?.path == "/v1/driver/reports/get" }.count, 1)
        }
    }
}
