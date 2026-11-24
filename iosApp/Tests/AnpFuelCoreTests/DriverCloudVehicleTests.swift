import Foundation
import XCTest
@testable import AnpFuelCore

final class DriverCloudVehicleTests: XCTestCase {
    private let account = "00000000-0000-4000-8000-000000000001"
    private let family = "00000000-0000-4000-8000-000000000002"
    private let origin = ApiEnvironment.staging.origin

    func testVehicleEnrollmentKeepsExactBoundsAndAvoidsPrivateFields() throws {
        let vehicle = VehicleRecord(id: "00000000-0000-4000-8000-000000000003", name: "Fixture EV",
                                    propulsion: .electric, primaryFuel: .gasolineRegular,
                                    tankCapacityMilliLiters: 0, batteryCapacityWh: 64_000)
        let payload = try DriverCloudVehicleWire.payload(vehicle)
        XCTAssertEqual(payload["propulsion"] as? String, "BEV")
        XCTAssertEqual(payload["capacity_milli"] as? Int64, 0)
        XCTAssertEqual(payload["battery_wh"] as? Int64, 64_000)
        XCTAssertEqual(payload["price_drop_alert_enabled"] as? Bool, false)
        for key in ["photo", "estimate", "latitude", "longitude", "odometer"] {
            XCTAssertNil(payload[key])
        }
        let invalid = VehicleRecord(id: vehicle.id, name: "Fixture", propulsion: .combustion,
                                    primaryFuel: .lpgP13, tankCapacityMilliLiters: 40_000)
        XCTAssertThrowsError(try DriverCloudVehicleWire.payload(invalid))

        let scope = try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: "account:\(account):fixture",
                                                          sessionFamilyId: family))
        let first = DriverCloudVehicleWire.Status(epoch: 1, revision: 2, consentRevision: 3, enabledDatasets: ["vehicles"])
        let same = DriverCloudVehicleWire.Status(epoch: 1, revision: 4, consentRevision: 3, enabledDatasets: ["vehicles"])
        let next = DriverCloudVehicleWire.Status(epoch: 2, revision: 4, consentRevision: 3, enabledDatasets: ["vehicles"])
        let initial = try DriverCloudVehicleWire.operationId(vehicle: vehicle.id, status: first, scope: scope, payload: payload)
        XCTAssertTrue(LedgerIds.isCanonical(initial))
        XCTAssertEqual(initial, try DriverCloudVehicleWire.operationId(vehicle: vehicle.id, status: same, scope: scope, payload: payload))
        XCTAssertNotEqual(initial, try DriverCloudVehicleWire.operationId(vehicle: vehicle.id, status: next, scope: scope, payload: payload))
    }

    func testExistingCloudVehicleReturnsAfterOwnerReadAndDoesNotOverwriteIt() async throws {
        let vehicle = VehicleRecord(id: "00000000-0000-4000-8000-000000000003", name: "New local name",
                                    propulsion: .combustion, primaryFuel: .ethanol, tankCapacityMilliLiters: 40_000)
        let live = PortableAuth.Session(familyId: family, accountId: account, accessToken: "fixture-cloud-access",
                                        refreshToken: "fixture-cloud-refresh", accessExpiresAt: 2_000_000_000,
                                        absoluteExpiresAt: 2_100_000_000)
        let sessions = StaticSessions(session: live)
        let exchange = ScriptedExchange { request in
            guard request.url?.path == "/v1/cloud/records" else { return .init(500, "{}") }
            return .init(200, """
            {"items":[{"id":"\(vehicle.id)","dataset":"vehicles","revision":5,"deleted":false}],
             "next_cursor":""}
            """)
        }
        let transport = SignedRequestTransport(origin: origin, signer: try IdentityVectors.signer(),
                                               identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true),
                                               exchange: exchange, sessions: sessions)
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let scope = try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: try XCTUnwrap(transport.ownerScope()),
                                                          sessionFamilyId: family))
        try await client.registerDriverVehicle(vehicle, scope: scope)
        XCTAssertEqual(exchange.requests.map { $0.url?.path }, ["/v1/cloud/records"])
        let fields = try XCTUnwrap(exchange.requests.first.map(FakeIdentityBackend.json))
        XCTAssertEqual(fields["dataset"] as? String, "vehicles")
        XCTAssertNil(fields["payload"])
    }
}
