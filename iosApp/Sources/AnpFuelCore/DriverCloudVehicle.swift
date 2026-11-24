import Foundation

public extension CommunityWriteError {
    /// The cloud vehicles dataset keeps the tank capacity in 10 mL steps. The local value is refused
    /// before any request and never rounded.
    static let driverVehicleTankPrecision = CommunityWriteError.invalid("cloud.tank-precision")
}

/// Enrollment for the selected local vehicle required by the driver command owner check.
public extension CommunityWriteClient {
    func registerDriverVehicle(
        _ vehicle: VehicleRecord, scope: AccountDataExportScope
    ) async throws {
        guard transport.origin == scope.origin else { throw SignedTransportError.originUnavailable }
        let payload = try DriverCloudVehicleWire.payload(vehicle)
        if try await hasDriverCloudVehicle(vehicle.id, scope: scope) { return }

        let currentData = try await driverVehiclePost("/v1/cloud/status", scope: scope, proof: false)
        var status = try DriverCloudVehicleWire.status(currentData)
        if !status.enabledDatasets.contains("vehicles") {
            let accepted = try await driverVehiclePost("/v1/cloud/consent", scope: scope, proof: true,
                                                       fields: ["datasets": ["vehicles"], "enabled": true])
            let changed = try DriverCloudVehicleWire.status(accepted)
            guard changed.epoch == status.epoch, changed.consentRevision >= status.consentRevision,
                  changed.enabledDatasets.contains("vehicles") else { throw CommunityWriteError.invalidReceipt }
            status = changed
        }

        let deviceData = try await driverVehiclePost("/v1/cloud/devices", scope: scope, proof: true)
        let device = try AbastevoHttpClient.publicDecoder().decode(DriverCloudVehicleWire.Device.self, from: deviceData)
        guard CommunityWire.isUuid(device.id), !device.revoked else { throw CommunityWriteError.invalidReceipt }

        // A lost reply can be replayed for this exact account/origin/epoch and payload.
        let id = try DriverCloudVehicleWire.operationId(vehicle: vehicle.id, status: status, scope: scope, payload: payload)
        let fields: [String: Any] = [
            "device_id": device.id, "epoch": status.epoch, "consent_revision": status.consentRevision,
            "commands": [["operation_id": id, "dataset": "vehicles", "id": vehicle.id, "expected_revision": 0,
                          "delete": false, "payload": payload]],
        ]
        do {
            let reply = try await driverVehiclePost("/v1/cloud/commands", scope: scope, proof: true,
                                                    fields: fields, operation: id)
            try DriverCloudVehicleWire.confirm(reply, operationId: id, vehicleId: vehicle.id, epoch: status.epoch)
        } catch SignedTransportError.refused(409, _, _) {
            // A different device may already own the same live vehicle. Do not replace it.
            guard try await hasDriverCloudVehicle(vehicle.id, scope: scope) else {
                throw CommunityWriteError.invalid("cloud.conflict")
            }
        }
    }

    private func hasDriverCloudVehicle(_ id: String, scope: AccountDataExportScope) async throws -> Bool {
        let data = try await driverVehiclePost("/v1/cloud/records", scope: scope, proof: false,
                                               fields: ["dataset": "vehicles", "limit": 100])
        let result = try AbastevoHttpClient.publicDecoder().decode(DriverCloudVehicleWire.Page.self, from: data)
        guard result.items.count <= 100, result.nextCursor.isEmpty,
              result.items.allSatisfy({ $0.dataset == "vehicles" && CommunityWire.isUuid($0.id) }) else {
            throw CommunityWriteError.invalidReceipt
        }
        return result.items.contains { $0.id == id && !$0.deleted }
    }

    private func driverVehiclePost(
        _ path: String, scope: AccountDataExportScope, proof: Bool, fields: [String: Any] = [:], operation: String? = nil
    ) async throws -> Data {
        let reply = try await ownerPost(path: path, expectedScope: scope.ownerScope, requiresDeviceProof: proof,
                                        idempotencyKey: operation, requiresSameSessionFamily: true) { session in
            guard session.familyId == scope.sessionFamilyId else { return nil }
            var envelope = fields
            envelope["family_id"] = session.familyId
            envelope["access_token"] = session.accessToken
            return DeviceIdentityWire.encode(envelope)
        }
        guard reply.status == 200 else { throw CommunityWriteError.invalidReceipt }
        return reply.body
    }
}

enum DriverCloudVehicleWire {
    struct Status: Decodable {
        let epoch: Int64
        let revision: Int64
        let consentRevision: Int64
        let enabledDatasets: [String]
    }

    struct Device: Decodable {
        let id: String
        let revoked: Bool
    }

    struct Page: Decodable {
        struct Row: Decodable {
            let id: String
            let dataset: String
            let deleted: Bool
        }
        let items: [Row]
        let nextCursor: String
    }

    private struct BatchReceipt: Decodable {
        struct Result: Decodable {
            let operationId: String
            let dataset: String
            let id: String
            let revision: Int64
            let status: String
        }
        let epoch: Int64
        let results: [Result]
    }

    static func payload(_ vehicle: VehicleRecord) throws -> [String: Any] {
        let name = vehicle.name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard CommunityWire.isUuid(vehicle.id), vehicle.id == vehicle.id.lowercased(),
              vehicle.id != "00000000-0000-0000-0000-000000000000",
              (1...100).contains(name.unicodeScalars.count),
              !name.unicodeScalars.contains(where: CharacterSet.controlCharacters.contains),
              vehicle.primaryFuel != .lpgP13 else {
            throw CommunityWriteError.invalid("cloud.invalid")
        }

        let tank = vehicle.propulsion == .electric ? 0 : vehicle.tankCapacityMilliLiters
        if vehicle.propulsion.supportsFuel {
            guard (10...200_000).contains(tank) else { throw CommunityWriteError.invalid("cloud.invalid") }
            guard tank % 10 == 0 else { throw CommunityWriteError.driverVehicleTankPrecision }
        }
        let propulsion: String
        switch vehicle.propulsion {
        case .combustion, .flex: propulsion = "ICE"
        case .hybrid: propulsion = "HEV"
        case .plugInHybrid: propulsion = "PHEV"
        case .electric: propulsion = "BEV"
        }

        var fields: [String: Any] = [
            "id": vehicle.id, "display_name": name, "capacity_milli": tank, "fuel_product": vehicle.primaryFuel.wireCode,
            "unit": vehicle.primaryFuel.unitWire, "price_source": "CHEAPEST_STATION", "price_drop_alert_enabled": false,
            "sort_order": 0, "archived": false, "propulsion": propulsion,
        ]
        if vehicle.propulsion.supportsElectric {
            guard (1...1_000_000_000).contains(vehicle.batteryCapacityWh) else {
                throw CommunityWriteError.invalid("cloud.invalid")
            }
            fields["battery_wh"] = vehicle.batteryCapacityWh
        }
        return fields
    }

    static func status(_ data: Data) throws -> Status {
        let value = try AbastevoHttpClient.publicDecoder().decode(Status.self, from: data)
        guard value.epoch >= 0, value.revision >= 0, value.consentRevision >= 0,
              Set(value.enabledDatasets).count == value.enabledDatasets.count else { throw CommunityWriteError.invalidReceipt }
        return value
    }

    static func operationId(vehicle: String, status: Status, scope: AccountDataExportScope,
                            payload: [String: Any]) throws -> String {
        guard let data = DeviceIdentityWire.encode(payload) else { throw CommunityWriteError.invalidReceipt }
        let digest = IosDeviceProofSigner.sha256Hex(data)
        let fields = ["driver-vehicle", scope.origin, scope.ownerScope, vehicle, "\(status.epoch)",
                      "\(status.consentRevision)", digest]
        guard let id = LedgerIds.nameBased(fields.joined(separator: "\u{0}")) else {
            throw CommunityWriteError.invalidReceipt
        }
        return id
    }

    static func confirm(_ data: Data, operationId: String, vehicleId: String, epoch: Int64) throws {
        let receipt = try AbastevoHttpClient.publicDecoder().decode(BatchReceipt.self, from: data)
        guard receipt.epoch == epoch, receipt.results.count == 1, let result = receipt.results.first,
              result.operationId == operationId, result.dataset == "vehicles", result.id == vehicleId,
              result.revision > 0, result.status == "accepted" else { throw CommunityWriteError.invalidReceipt }
    }
}
