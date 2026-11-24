import Foundation

/// Brazilian CNPJ in the directory kernel: ASCII formatting, letters, and two check digits.
public enum BrazilianCNPJ {
    public static func normalized(_ raw: String) -> String? {
        var bytes: [UInt8] = []
        for byte in raw.utf8 {
            switch byte {
            case 48...57, 65...90: bytes.append(byte)
            case 97...122: bytes.append(byte - 32)
            case 32, 45, 46, 47: continue
            default: return nil
            }
            guard bytes.count <= 14 else { return nil }
        }
        guard bytes.count == 14, (48...57).contains(bytes[12]), (48...57).contains(bytes[13]) else {
            return nil
        }
        let digits = bytes.prefix(12).map { Int($0) - 48 }
        let first = checkDigit(digits, weights: [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
        let second = checkDigit(digits + [first], weights: [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
        guard bytes[12] == UInt8(first + 48), bytes[13] == UInt8(second + 48) else { return nil }
        return String(bytes: bytes, encoding: .ascii)
    }

    private static func checkDigit(_ values: [Int], weights: [Int]) -> Int {
        let remainder = zip(values, weights).reduce(0) { $0 + $1.0 * $1.1 } % 11
        return remainder < 2 ? 0 : 11 - remainder
    }
}

public enum StationIntakeValidation: Error, Equatable, Sendable {
    case nameRequired
    case cityInvalid
    case invalidCNPJ
    case invalidChargingFields
}

public struct StationIntakeProposal: Codable, Equatable, Sendable {
    public let displayName: String
    public let municipalityCode: String
    public let state: BrazilianState
    public let cnpj: String?
    public let publicAddress: String?
    public let chargingSite: Bool

    public init(
        displayName: String, municipalityCode: String, state: BrazilianState, cnpj: String = "",
        publicAddress: String = "", chargingSite: Bool
    ) throws {
        let name = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        let municipality = municipalityCode.trimmingCharacters(in: .whitespacesAndNewlines)
        let cnpjInput = cnpj.trimmingCharacters(in: .whitespacesAndNewlines)
        let address = publicAddress.trimmingCharacters(in: .whitespacesAndNewlines)
        guard municipality.utf8.count == 7, municipality.utf8.allSatisfy({ (48...57).contains($0) }) else {
            throw StationIntakeValidation.cityInvalid
        }

        if chargingSite {
            guard cnpjInput.isEmpty, (!name.isEmpty || !address.isEmpty),
                  name.utf8.count <= 160, address.utf8.count <= 240,
                  !name.unicodeScalars.contains(where: CharacterSet.controlCharacters.contains),
                  !address.unicodeScalars.contains(where: CharacterSet.controlCharacters.contains),
                  address.isEmpty || address.unicodeScalars.contains(where: CharacterSet.letters.contains) else {
                throw StationIntakeValidation.invalidChargingFields
            }
            self.cnpj = nil
            self.publicAddress = address.isEmpty ? nil : address
        } else {
            guard !name.isEmpty else { throw StationIntakeValidation.nameRequired }
            guard address.isEmpty else { throw StationIntakeValidation.invalidChargingFields }
            if cnpjInput.isEmpty {
                self.cnpj = nil
            } else {
                guard let normalized = BrazilianCNPJ.normalized(cnpjInput) else { throw StationIntakeValidation.invalidCNPJ }
                self.cnpj = normalized
            }
            self.publicAddress = nil
        }

        self.displayName = name
        self.municipalityCode = municipality
        self.state = state
        self.chargingSite = chargingSite
    }

    public var label: String { displayName.isEmpty ? (publicAddress ?? "") : displayName }

    func wireFields() throws -> [String: String] {
        let proposal = try StationIntakeProposal(
            displayName: displayName, municipalityCode: municipalityCode, state: state,
            cnpj: cnpj ?? "", publicAddress: publicAddress ?? "", chargingSite: chargingSite
        )
        var fields = ["display_name": proposal.displayName,
                      "municipality_code": proposal.municipalityCode, "state": proposal.state.rawValue]
        if proposal.chargingSite {
            fields["kind"] = "charging_site"
            fields["public_address"] = proposal.publicAddress ?? ""
        } else if let cnpj = proposal.cnpj {
            fields["cnpj_normalized"] = cnpj
        }
        return fields
    }
}

public struct StationIntakeReceipt: Decodable, Identifiable, Equatable, Sendable {
    public enum State: String, Decodable, Sendable {
        case pending
        case cancelled
        case approved
        case rejected
    }

    public let id: String
    public let clientSubmissionId: String
    public let state: State
    public let createdAt: String

    public var createdDate: Date? { ISO8601DateFormatter().date(from: createdAt) }

    func validated() throws -> Self {
        guard CommunityWire.isUuid(id), !clientSubmissionId.isEmpty, createdDate != nil else {
            throw CommunityWriteError.invalidReceipt
        }
        return self
    }
}

enum StationIntakeWire {
    static let path = "/v1/stations/suggestions"
    static let maxOwnerItems = 20

    static func body(session: PortableAuth.Session, proposal: StationIntakeProposal, operationId: String) -> Data? {
        guard CommunityWire.isUuid(operationId), let fields = try? proposal.wireFields() else { return nil }
        guard let body = DeviceIdentityWire.encode([
            "family_id": session.familyId, "access_token": session.accessToken,
            "client_submission_id": operationId, "proposal": fields,
        ]), body.count <= 8 * 1024 else { return nil }
        return body
    }

    static func sessionBody(_ session: PortableAuth.Session) -> Data? {
        DeviceIdentityWire.encode(["family_id": session.familyId, "access_token": session.accessToken])
    }
}

extension CommunityWriteClient {
    public func submitStationSuggestion(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        guard item.origin == transport.origin, let proposal = item.stationProposal,
              CommunityWire.isUuid(item.id), (try? proposal.wireFields()) != nil else {
            throw CommunityWriteError.invalid("intake.invalid")
        }
        let response: SignedHttpResponse
        do {
            response = try await ownerPost(path: StationIntakeWire.path, expectedScope: item.ownerScope, requiresDeviceProof: false) {
                StationIntakeWire.body(session: $0, proposal: proposal, operationId: item.id)
            }
        } catch SignedTransportError.refused(429, .some("intake.quota-exceeded"), nil) {
            throw CommunityWriteError.invalid("intake.quota-exceeded")
        } catch CommunityWriteError.invalid("community.invalid-request") {
            throw CommunityWriteError.invalid("intake.invalid")
        }
        let receipt = try AbastevoHttpClient.publicDecoder().decode(StationIntakeReceipt.self, from: response.body).validated()
        guard (200...201).contains(response.status), receipt.clientSubmissionId == item.id else {
            throw CommunityWriteError.invalidReceipt
        }
        return CommunityWriteReceipt(id: receipt.id, state: receipt.state.rawValue)
    }

    public func ownedStationSuggestions(expectedScope: String) async throws -> [StationIntakeReceipt] {
        struct Suggestions: Decodable { let items: [StationIntakeReceipt] }
        let response = try await ownerPost(
            path: StationIntakeWire.path + "/mine", expectedScope: expectedScope,
            requiresDeviceProof: false, buildBody: StationIntakeWire.sessionBody
        )
        let rows = try AbastevoHttpClient.publicDecoder().decode(Suggestions.self, from: response.body).items
        guard rows.count <= StationIntakeWire.maxOwnerItems, Set(rows.map(\.id)).count == rows.count else {
            throw CommunityWriteError.invalidReceipt
        }
        return try rows.map { try $0.validated() }
    }

    public func stationSuggestionStatus(id: String, expectedScope: String) async throws -> StationIntakeReceipt {
        let response = try await stationSuggestionAction(id: id, action: "status", expectedScope: expectedScope)
        let receipt = try AbastevoHttpClient.publicDecoder().decode(StationIntakeReceipt.self, from: response.body).validated()
        guard receipt.id.lowercased() == id.lowercased() else { throw CommunityWriteError.invalidReceipt }
        return receipt
    }

    public func cancelStationSuggestion(id: String, operationId: String, expectedScope: String) async throws {
        struct Cancelled: Decodable { let status: StationIntakeReceipt.State }
        let response = try await stationSuggestionAction(
            id: id, action: "cancel", expectedScope: expectedScope, idempotencyKey: operationId
        )
        let result = try AbastevoHttpClient.publicDecoder().decode(Cancelled.self, from: response.body)
        guard result.status == .cancelled else { throw CommunityWriteError.invalidReceipt }
    }

    private func stationSuggestionAction(
        id: String, action: String, expectedScope: String, idempotencyKey: String? = nil
    ) async throws -> SignedHttpResponse {
        guard CommunityWire.isUuid(id) else { throw CommunityWriteError.invalid("intake.not-found") }
        return try await ownerPost(
            path: StationIntakeWire.path + "/" + id.lowercased() + "/" + action,
            expectedScope: expectedScope, requiresDeviceProof: false,
            idempotencyKey: idempotencyKey, buildBody: StationIntakeWire.sessionBody
        )
    }
}
