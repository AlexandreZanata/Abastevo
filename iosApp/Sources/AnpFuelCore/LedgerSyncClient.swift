import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Wire shapes of the private ledger routes (`backend/internal/modules/ledger/adapters/http`,
/// `contracts/openapi/v1.yaml`, `contracts/testdata/api/ledger-*.json`).
///
/// Every request carries the live account session (`family_id`, `access_token`).
/// Commands, consent and erasure are additionally signed with the bound device key
/// over the exact body; record pages are unsigned session reads, free after a lapse.
/// Command payloads are embedded byte for byte so a retry keeps the replay hash.
/// Zero postfix unwrap operators anywhere in this file.
public enum LedgerSyncWire {
    public static let commandsPath = "/v1/ledger/commands"
    public static let recordsPath = "/v1/ledger/records"
    public static let consentPath = "/v1/ledger/consent"
    public static let erasePath = "/v1/ledger/erase"
    public static let maxAccessTokenCharacters = 512
    public static let maxPayloadBytes = 8 * 1024

    static func isSessionUsable(_ session: PortableAuth.Session) -> Bool {
        return session.familyId.count == 36 && !session.accessToken.isEmpty
            && session.accessToken.count <= maxAccessTokenCharacters
    }

    /// One-command batch: `{"family_id","access_token","commands":[{operation_id,dataset,id,expected_revision,delete,void,payload}]}`.
    public static func commandBody(session: PortableAuth.Session, command: LedgerQueuedCommand) -> Data? {
        guard isSessionUsable(session),
              LedgerIds.isCanonical(command.operationId),
              LedgerIds.isCanonical(command.recordId),
              command.expectedRevision >= 0 else {
            return nil
        }
        let payload: String
        if command.isDelete {
            guard command.payloadJson.isEmpty else { return nil }
            payload = "null"
        } else {
            guard !command.payloadJson.isEmpty, command.payloadJson.utf8.count <= maxPayloadBytes,
                  LedgerWire.decodePayload(json: command.payloadJson, dataset: command.dataset)?.id == command.recordId else {
                return nil
            }
            payload = command.payloadJson
        }
        var text = "{\"family_id\":" + LedgerWire.quote(session.familyId)
        text += ",\"access_token\":" + LedgerWire.quote(session.accessToken)
        text += ",\"commands\":[{\"operation_id\":" + LedgerWire.quote(command.operationId)
        text += ",\"dataset\":" + LedgerWire.quote(command.dataset.rawValue)
        text += ",\"id\":" + LedgerWire.quote(command.recordId)
        text += ",\"expected_revision\":\(command.expectedRevision)"
        text += ",\"delete\":" + (command.isDelete ? "true" : "false")
        text += ",\"void\":false,\"payload\":" + payload + "}]}"
        return Data(text.utf8)
    }

    public static func recordsBody(
        session: PortableAuth.Session,
        dataset: LedgerDataset,
        vehicleId: String?,
        limit: Int,
        cursor: String?
    ) -> Data? {
        guard isSessionUsable(session), (1...LedgerSyncEngine.pageLimit).contains(limit) else { return nil }
        var object: [String: Any] = [
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "dataset": dataset.rawValue,
            "limit": limit,
        ]
        if let vehicleId {
            guard LedgerIds.isCanonical(vehicleId) else { return nil }
            object["vehicle_id"] = vehicleId
        }
        if let cursor, !cursor.isEmpty {
            guard isOpaqueCursor(cursor) else { return nil }
            object["cursor"] = cursor
        }
        return DeviceIdentityWire.encode(object)
    }

    public static func consentBody(session: PortableAuth.Session, datasets: [LedgerDataset], enabled: Bool) -> Data? {
        let unique = LedgerDataset.allCases.filter { datasets.contains($0) }
        guard isSessionUsable(session), !unique.isEmpty, unique.count == datasets.count else { return nil }
        return DeviceIdentityWire.encode([
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "datasets": unique.map { $0.rawValue },
            "enabled": enabled,
        ])
    }

    public static func eraseBody(session: PortableAuth.Session) -> Data? {
        guard isSessionUsable(session) else { return nil }
        return DeviceIdentityWire.encode([
            "family_id": session.familyId,
            "access_token": session.accessToken,
        ])
    }

    /// Opaque server cursor: returned verbatim, bounded, no control characters (Android `PremiumCursor`).
    public static func isOpaqueCursor(_ cursor: String) -> Bool {
        guard !cursor.isEmpty, cursor.count <= LedgerBounds.maxCursorCharacters else { return false }
        return !cursor.unicodeScalars.contains { LedgerText.isControl($0) }
    }

    /// Integral JSON numbers only (Foundation bridging refuses fractional values).
    static func int64(_ value: Any?) -> Int64? {
        return value as? Int64
    }

    /// The single result of a one-command batch, matched to the command sent.
    public static func decodeReceipt(_ data: Data, command: LedgerQueuedCommand) -> LedgerCommandReceipt? {
        guard let json = DeviceIdentityWire.object(data),
              let results = json["results"] as? [[String: Any]],
              results.count == 1 else {
            return nil
        }
        let row = results[0]
        guard let operation = row["operation_id"] as? String, operation == command.operationId,
              let datasetRaw = row["dataset"] as? String, let dataset = LedgerDataset(rawValue: datasetRaw),
              dataset == command.dataset,
              let id = row["id"] as? String, id == command.recordId,
              let revision = int64(row["revision"]), revision >= 0,
              let status = row["status"] as? String, status == "accepted" else {
            return nil
        }
        if !command.isDelete && revision < 1 { return nil }
        let replayed = row["replayed"] as? Bool ?? false
        return LedgerCommandReceipt(
            operationId: operation,
            dataset: dataset,
            recordId: id,
            revision: revision,
            replayed: replayed
        )
    }

    /// A bounded page of one dataset: no foreign dataset, vehicle or duplicate rows. Rows whose
    /// payload cannot be represented keep `record == nil` and are reported, never invented.
    public static func decodeRecordsPage(
        _ data: Data,
        dataset: LedgerDataset,
        vehicleId: String?,
        limit: Int
    ) -> LedgerRecordsPage? {
        guard let json = DeviceIdentityWire.object(data),
              let rows = json["items"] as? [[String: Any]],
              rows.count <= limit, rows.count <= LedgerSyncEngine.pageLimit else {
            return nil
        }
        var seen = Set<String>()
        var items: [LedgerServerRecord] = []
        for row in rows {
            guard let rowDataset = row["dataset"] as? String, rowDataset == dataset.rawValue,
                  let id = row["id"] as? String, LedgerIds.isCanonical(id), seen.insert(id).inserted,
                  let owner = row["vehicle_id"] as? String, LedgerIds.isCanonical(owner),
                  let occurredText = row["occurred_at"] as? String,
                  let occurred = LedgerTime.parse(occurredText),
                  let revision = int64(row["revision"]), revision >= 1,
                  let voided = row["voided"] as? Bool,
                  let payloadObject = row["payload"] as? [String: Any] else {
                return nil
            }
            if let vehicleId, vehicleId != owner { return nil }
            var record: LedgerEntryRecord?
            if JSONSerialization.isValidJSONObject(payloadObject),
               let payloadData = try? JSONSerialization.data(withJSONObject: payloadObject),
               let decoded = LedgerWire.decodePayload(payloadData, dataset: dataset),
               decoded.id == id, decoded.vehicleId == owner {
                record = decoded
            }
            items.append(
                LedgerServerRecord(
                    dataset: dataset,
                    id: id,
                    vehicleId: owner,
                    occurredAtMillis: occurred,
                    revision: revision,
                    voided: voided,
                    record: record
                )
            )
        }
        let rawCursor = json["next_cursor"]
        var next: String?
        if let cursor = rawCursor as? String {
            if !cursor.isEmpty {
                guard isOpaqueCursor(cursor) else { return nil }
                next = cursor
            }
        } else if rawCursor != nil && !(rawCursor is NSNull) {
            return nil
        }
        return LedgerRecordsPage(items: items, nextCursor: next)
    }

    public static func decodeConsent(_ data: Data) -> LedgerConsentStatus? {
        guard let json = DeviceIdentityWire.object(data),
              let revision = int64(json["revision"]), revision >= 0,
              let raw = json["datasets"] as? [String] else {
            return nil
        }
        var datasets: [LedgerDataset] = []
        for value in raw {
            guard let dataset = LedgerDataset(rawValue: value), !datasets.contains(dataset) else { return nil }
            datasets.append(dataset)
        }
        return LedgerConsentStatus(revision: revision, datasets: datasets)
    }

    public static func decodeErase(_ data: Data) -> Bool {
        guard let json = DeviceIdentityWire.object(data), let status = json["status"] as? String else { return false }
        return status == "erased"
    }
}

/// URLSession gateway for the private ledger. Network work runs on the transport actor and
/// URLSession, never on the main actor. A session of another account is refused.
public final class LedgerSyncClient: LedgerSyncGateway, @unchecked Sendable {
    public let transport: SignedRequestTransport
    private let sessions: SignedSessionSource

    public init(transport: SignedRequestTransport, sessions: SignedSessionSource) {
        self.transport = transport
        self.sessions = sessions
    }

    public var origin: String { transport.origin }

    private func session(for accountId: String) async throws -> PortableAuth.Session {
        guard let session = await sessions.liveSession() else { throw LedgerSyncError.signInRequired }
        guard session.accountId == accountId else { throw LedgerSyncError.ownerMismatch }
        return session
    }

    public func submit(_ command: LedgerQueuedCommand, accountId: String) async throws -> LedgerCommandReceipt {
        _ = try await transport.ensureIdentity()
        let live = try await session(for: accountId)
        guard let body = LedgerSyncWire.commandBody(session: live, command: command) else {
            throw LedgerSyncError.invalidCommand
        }
        let response = try await transport.signedRequest(method: "POST", path: LedgerSyncWire.commandsPath, body: body)
        guard let receipt = LedgerSyncWire.decodeReceipt(response.body, command: command) else {
            throw LedgerSyncError.invalidReceipt
        }
        return receipt
    }

    public func listRecords(
        accountId: String,
        dataset: LedgerDataset,
        vehicleId: String?,
        limit: Int,
        cursor: String?
    ) async throws -> LedgerRecordsPage {
        let live = try await session(for: accountId)
        guard let body = LedgerSyncWire.recordsBody(
            session: live,
            dataset: dataset,
            vehicleId: vehicleId,
            limit: limit,
            cursor: cursor
        ) else {
            throw LedgerSyncError.invalidCommand
        }
        let response = try await transport.plainPost(path: LedgerSyncWire.recordsPath, body: body)
        guard let page = LedgerSyncWire.decodeRecordsPage(response.body, dataset: dataset, vehicleId: vehicleId, limit: limit) else {
            throw LedgerSyncError.invalidPage
        }
        return page
    }

    public func setConsent(accountId: String, datasets: [LedgerDataset], enabled: Bool) async throws -> LedgerConsentStatus {
        _ = try await transport.ensureIdentity()
        let live = try await session(for: accountId)
        guard let body = LedgerSyncWire.consentBody(session: live, datasets: datasets, enabled: enabled) else {
            throw LedgerSyncError.invalidCommand
        }
        let response = try await transport.signedRequest(method: "POST", path: LedgerSyncWire.consentPath, body: body)
        guard let status = LedgerSyncWire.decodeConsent(response.body) else { throw LedgerSyncError.invalidReceipt }
        return status
    }

    public func eraseAll(accountId: String) async throws {
        _ = try await transport.ensureIdentity()
        let live = try await session(for: accountId)
        guard let body = LedgerSyncWire.eraseBody(session: live) else { throw LedgerSyncError.invalidCommand }
        let response = try await transport.signedRequest(method: "POST", path: LedgerSyncWire.erasePath, body: body)
        guard LedgerSyncWire.decodeErase(response.body) else { throw LedgerSyncError.invalidReceipt }
    }
}

// The report paths share the ledger client, exact owner/family scope, and the existing transport.
extension LedgerSyncClient {
    public func buildMonthlyReport(
        _ period: LedgerReportPeriod, accountId: String, scope: AccountDataExportScope, operationId: String
    ) async throws -> LedgerServerReport {
        let data = try await analysis(.build, accountId: accountId, scope: scope, operationId: operationId) {
            LedgerAnalysisWire.periodBody(session: $0, period: period)
        }
        return try LedgerAnalysisWire.report(data, period: period)
    }

    public func monthlyReport(
        _ period: LedgerReportPeriod, accountId: String, scope: AccountDataExportScope
    ) async throws -> LedgerServerReport {
        let data = try await analysis(.get, accountId: accountId, scope: scope) {
            LedgerAnalysisWire.periodBody(session: $0, period: period)
        }
        return try LedgerAnalysisWire.report(data, period: period)
    }

    public func monthlyReportCsv(
        _ period: LedgerReportPeriod, accountId: String, scope: AccountDataExportScope, operationId: String
    ) async throws -> Data {
        let data = try await analysis(.csv, accountId: accountId, scope: scope, operationId: operationId) {
            LedgerAnalysisWire.periodBody(session: $0, period: period)
        }
        return try LedgerAnalysisWire.csv(data, period: period)
    }

    public func compare(
        baseline: LedgerComparisonSide, current: LedgerComparisonSide, accountId: String, scope: AccountDataExportScope
    ) async throws -> LedgerServerComparison {
        let data = try await analysis(.compare, accountId: accountId, scope: scope) {
            LedgerAnalysisWire.comparisonBody(session: $0, baseline: baseline, current: current)
        }
        return try LedgerAnalysisWire.comparison(data)
    }

    private func analysis(
        _ route: LedgerAnalysisWire.Route, accountId: String, scope: AccountDataExportScope, operationId: String? = nil,
        buildBody: (PortableAuth.Session) -> Data?
    ) async throws -> Data {
        guard origin == scope.origin, scope.ownerScope.hasPrefix("account:\(accountId):") else {
            throw LedgerSyncError.ownerMismatch
        }
        if let operationId, !CommunityWire.isUuid(operationId) { throw LedgerSyncError.invalidCommand }
        try transport.requireOwnerScope(scope.ownerScope)
        let live = try await session(for: accountId)
        guard live.familyId == scope.sessionFamilyId else { throw LedgerSyncError.ownerMismatch }
        guard let body = buildBody(live) else { throw LedgerSyncError.invalidCommand }
        let response: SignedHttpResponse
        if route.requiresDeviceProof {
            response = try await transport.signedRequest(
                method: "POST", path: route.rawValue, body: body, idempotencyKey: operationId,
                expectedOwnerScope: scope.ownerScope, expectedSessionFamilyId: scope.sessionFamilyId
            )
        } else {
            response = try await transport.plainPost(
                path: route.rawValue, body: body, expectedOwnerScope: scope.ownerScope,
                expectedSessionFamilyId: scope.sessionFamilyId
            )
        }
        guard response.status == 200 else { throw LedgerSyncError.invalidReceipt }
        return response.body
    }
}
