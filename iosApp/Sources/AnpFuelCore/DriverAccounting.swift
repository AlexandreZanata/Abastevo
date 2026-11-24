import CryptoKit
import Foundation

public enum DriverPlatform: String, Codable, CaseIterable, Sendable {
    case uber = "UBER"
    case ninetyNine = "99"
    case inDrive = "INDRIVE"
    case other = "OTHER"
}

public enum DriverEarningCoverage: String, Codable, Sendable {
    case detail = "DETAIL"
    case summary = "SUMMARY"
}

public enum DriverMoneyBasis: String, Codable, Sendable {
    case detailed = "DETAILED"
    case inclusiveNet = "INCLUSIVE_NET"
}

/// Integer centavos with the same mutually exclusive money presentations as the
/// Go driver domain. Nil means the component is unknown or outside the basis.
public struct DriverEarningsMoney: Codable, Equatable, Sendable {
    public static let maximum: Int64 = 1_000_000_000_000

    public let basis: DriverMoneyBasis
    public let gross: Int64?
    public let fees: Int64?
    public let tips: Int64?
    public let bonus: Int64?
    public let refund: Int64?
    public let net: Int64?

    public static func inclusiveNet(_ centavos: Int64) -> Self? {
        let value = Self(basis: .inclusiveNet, gross: nil, fees: nil, tips: nil, bonus: nil, refund: nil, net: centavos)
        return value.earnedCentavos == nil ? nil : value
    }

    public static func detailed(gross: Int64, fees: Int64, tips: Int64, bonus: Int64, refund: Int64) -> Self? {
        let value = Self(basis: .detailed, gross: gross, fees: fees, tips: tips, bonus: bonus, refund: refund, net: nil)
        return value.earnedCentavos == nil ? nil : value
    }

    public var earnedCentavos: Int64? {
        let bounds = 0...Self.maximum
        switch basis {
        case .inclusiveNet:
            guard let net, bounds.contains(net), [gross, fees, tips, bonus, refund].allSatisfy({ $0 == nil }) else {
                return nil
            }
            return net
        case .detailed:
            guard net == nil, let gross, let fees, let tips, let bonus, let refund,
                  [gross, fees, tips, bonus, refund].allSatisfy(bounds.contains) else { return nil }
            let total = gross - fees + tips + bonus - refund
            return (-Self.maximum...Self.maximum).contains(total) ? total : nil
        }
    }
}

public enum DriverAccountingError: Error, Equatable, Sendable {
    case invalidCommand
    case invalidReceipt
    case scopeChanged
    case localStorage
    case queueFull
    case existingCommand
    case staleClaim
}

public struct DriverCommandReceipt: Codable, Equatable, Sendable {
    public let id: String
    public let revision: Int64
    public let epoch: Int64
    public let replayed: Bool
}

public enum DriverAccountingWire {
    public static let commandsPath = "/v1/driver/commands"

    public static func receipt(from data: Data) throws -> DriverCommandReceipt {
        guard let receipt = try? JSONDecoder().decode(DriverCommandReceipt.self, from: data),
              validID(receipt.id), receipt.revision > 0, receipt.epoch >= 0 else {
            throw DriverAccountingError.invalidReceipt
        }
        return receipt
    }

    public static func errorCode(status: Int, from data: Data) throws -> String {
        struct ErrorEnvelope: Decodable {
            struct Message: Decodable {
                let code: String
                let message: String
            }
            let error: Message
        }
        guard status == 401 || status == 409,
              let response = try? JSONDecoder().decode(ErrorEnvelope.self, from: data),
              response.error.code.hasPrefix("driver."), !response.error.message.isEmpty else {
            throw DriverAccountingError.invalidReceipt
        }
        return response.error.code
    }

    static func validID(_ id: String) -> Bool {
        LedgerIds.isCanonical(id)
    }

    static func canonical<T: Encodable>(_ value: T) -> Data? {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        return try? encoder.encode(value)
    }

    static func hash(_ bytes: Data) -> String {
        SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
    }

    static func stableOperationID(_ data: Data) -> String {
        var bytes = Array(SHA256.hash(data: data).prefix(16))
        bytes[6] = (bytes[6] & 0x0f) | 0x50
        bytes[8] = (bytes[8] & 0x3f) | 0x80
        let parts = bytes.map { String(format: "%02x", $0) }
        return [parts[0..<4], parts[4..<6], parts[6..<8], parts[8..<10], parts[10..<16]]
            .map { $0.joined() }.joined(separator: "-")
    }
}

/// The accounting command stores only the explicit earnings declaration, window,
/// identifiers and server fences. Shift distances, costs, GPS and credentials
/// are separate from the earnings payload.
public struct DriverAccountingCommand: Codable, Equatable, Identifiable, Sendable {
    private struct Record: Codable, Equatable, Sendable {
        let id: String
        let vehicleId: String
        let platform: DriverPlatform
        let sourceHash: String
        let coverage: DriverEarningCoverage
        let currency: String
        let from: String
        let to: String
        let timezone: String
        let money: DriverEarningsMoney

        enum CodingKeys: String, CodingKey {
            case id, platform, coverage, currency, from, to, timezone, money
            case vehicleId = "vehicle_id"
            case sourceHash = "source_hash"
        }
    }

    private struct Payload: Codable, Sendable {
        let operationId: String
        let expectedRevision: Int64
        let expectedEpoch: Int64
        let record: Record

        enum CodingKeys: String, CodingKey {
            case operationId = "operation_id"
            case expectedRevision = "expected_revision"
            case expectedEpoch = "expected_epoch"
            case record
        }
    }

    private struct Envelope: Encodable {
        let familyId: String
        let accessToken: String
        let kind: String
        let payload: Payload

        enum CodingKeys: String, CodingKey {
            case familyId = "family_id"
            case accessToken = "access_token"
            case kind, payload
        }
    }

    public let operationId: String
    public let expectedRevision: Int64
    public let expectedEpoch: Int64
    private let record: Record

    public var id: String { operationId }
    public var recordId: String { record.id }
    public var vehicleId: String { record.vehicleId }

    private var payload: Payload {
        Payload(operationId: operationId, expectedRevision: expectedRevision, expectedEpoch: expectedEpoch, record: record)
    }

    public static func earning(
        forCompletedShift shift: DriverShiftRecord,
        platform: DriverPlatform,
        money: DriverEarningsMoney,
        timezone: String,
        expectedRevision: Int64 = 0,
        epoch: Int64 = 0
    ) -> Self? {
        guard !shift.isActive, let ended = shift.endedAtLabel,
              let from = LedgerTime.parse(shift.startedAtLabel), let to = LedgerTime.parse(ended) else { return nil }
        return earning(
            recordId: shift.id, vehicleId: shift.vehicleId, platform: platform, coverage: .summary,
            sourceHash: DriverAccountingWire.hash(Data("abastevo-ios:manual-shift:\(shift.id)".utf8)),
            fromMillis: from, toMillis: to, timezone: timezone, money: money, expectedRevision: expectedRevision, epoch: epoch
        )
    }

    public static func earning(
        recordId: String, vehicleId: String, platform: DriverPlatform, coverage: DriverEarningCoverage,
        sourceHash: String, fromMillis: Int64, toMillis: Int64, timezone: String, money: DriverEarningsMoney,
        expectedRevision: Int64 = 0, epoch: Int64 = 0
    ) -> Self? {
        guard (0...LedgerBounds.maxEventMillis).contains(fromMillis),
              (0...LedgerBounds.maxEventMillis).contains(toMillis), toMillis > fromMillis else { return nil }
        let record = Record(
            id: recordId, vehicleId: vehicleId, platform: platform, sourceHash: sourceHash, coverage: coverage,
            currency: "BRL", from: LedgerTime.format(millis: fromMillis), to: LedgerTime.format(millis: toMillis),
            timezone: timezone, money: money
        )
        guard let operationId = operation(record: record, revision: expectedRevision, epoch: epoch) else { return nil }
        let command = Self(operationId: operationId, expectedRevision: expectedRevision, expectedEpoch: epoch, record: record)
        return command.isValid ? command : nil
    }

    private static func operation(record: Record, revision: Int64, epoch: Int64) -> String? {
        // An empty operation ID is part of the stable seed, never sent to the server.
        guard let data = DriverAccountingWire.canonical(Payload(operationId: "", expectedRevision: revision, expectedEpoch: epoch, record: record)) else {
            return nil
        }
        return DriverAccountingWire.stableOperationID(data)
    }

    var isValid: Bool {
        guard DriverAccountingWire.validID(operationId), DriverAccountingWire.validID(record.id),
              DriverAccountingWire.validID(record.vehicleId), expectedRevision >= 0, expectedRevision < Int64.max,
              expectedEpoch >= 0, record.currency == "BRL", record.timezone != "Local",
              LedgerText.isValidTimezone(record.timezone), record.money.earnedCentavos != nil,
              record.sourceHash.utf8.count == 64, record.sourceHash.utf8.allSatisfy({
                  (48...57).contains($0) || (97...102).contains($0)
              }),
              let from = LedgerTime.parse(record.from), let to = LedgerTime.parse(record.to),
              from >= 0, to > from,
              operationId == Self.operation(record: record, revision: expectedRevision, epoch: expectedEpoch) else { return false }
        return true
    }

    public var canonicalPayload: Data? { isValid ? DriverAccountingWire.canonical(payload) : nil }

    func body(session: PortableAuth.Session) -> Data? {
        guard isValid, LedgerSyncWire.isSessionUsable(session) else { return nil }
        return DriverAccountingWire.canonical(Envelope(
            familyId: session.familyId, accessToken: session.accessToken, kind: "EARNING_SAVE", payload: payload
        ))
    }

    func accepts(_ receipt: DriverCommandReceipt) -> Bool {
        guard receipt.epoch == expectedEpoch else { return false }
        if receipt.replayed, expectedRevision == 0 { return receipt.revision >= 1 }
        return receipt.id == recordId && receipt.revision == expectedRevision + 1
    }
}

extension CommunityWriteClient {
    public func submitDriverAccounting(
        _ claim: DriverAccountingClaim, scope: AccountDataExportScope
    ) async throws -> DriverCommandReceipt {
        let command = claim.command
        guard command.isValid else { throw DriverAccountingError.invalidCommand }
        guard transport.origin == scope.origin, claim.scope.origin == scope.origin,
              scope.ownerScope.hasPrefix("account:\(claim.scope.accountId):") else {
            throw DriverAccountingError.scopeChanged
        }
        let result = try await ownerPost(
            path: DriverAccountingWire.commandsPath, expectedScope: scope.ownerScope,
            requiresDeviceProof: true, requiresSameSessionFamily: true
        ) { session in
            guard session.familyId == scope.sessionFamilyId else { return nil }
            return command.body(session: session)
        }
        guard result.status == 200 else { throw DriverAccountingError.invalidReceipt }
        let receipt = try DriverAccountingWire.receipt(from: result.body)
        guard command.accepts(receipt) else { throw DriverAccountingError.invalidReceipt }
        return receipt
    }
}
