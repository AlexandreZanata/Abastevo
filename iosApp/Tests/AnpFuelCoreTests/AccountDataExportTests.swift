import Foundation
import XCTest
@testable import AnpFuelCore

final class AccountDataExportTests: XCTestCase {
    private let origin = "https://backend.example"
    private let owner = "account:00000000-0000-4000-8000-000000000010:fixture"
    private let family = "00000000-0000-4000-8000-000000000011"
    private let rowA = "00000000-0000-4000-8000-000000000021"
    private let rowB = "00000000-0000-4000-8000-000000000022"
    private let vehicle = "00000000-0000-4000-8000-000000000023"

    func testExportFollowsAllLedgerDatasetsAndRetainsSuccessfulPagesAfterFailure() async throws {
        let scope = try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: owner, sessionFamilyId: family))
        var refuelCursors: [String] = []
        var seen: Set<AccountDataExport.SectionID> = []
        let export = try await AccountExportPlan.build(scope: scope, localForumReceipts: [], verifyScope: {}) { read, cursor in
            seen.insert(read.section)
            switch read.section {
            case .refuels:
                refuelCursors.append(cursor ?? "")
                if cursor == "page-two" {
                    return try self.ledgerPage(dataset: .refuel, id: self.rowB, next: "page-three")
                }
                if cursor == "page-three" {
                    throw SignedTransportError.refused(status: 503, code: "ledger.unavailable", retryAfterSeconds: nil)
                }
                return try self.ledgerPage(dataset: .refuel, id: self.rowA, next: "page-two")
            case .expenses:
                return try self.ledgerPage(dataset: .expense, id: self.rowB, next: "")
            case .stationClaims:
                throw SignedTransportError.refused(status: 503, code: "profile.integration-pending", retryAfterSeconds: nil)
            default:
                return try self.emptyPage(read)
            }
        }

        XCTAssertEqual(refuelCursors, ["", "page-two", "page-three"])
        XCTAssertTrue(seen.isSuperset(of: [.refuels, .expenses, .charges, .cloudVehicles, .subscriptions]))
        let refuels = try summary(.refuels, in: export)
        XCTAssertEqual(refuels.status, .partial)
        XCTAssertEqual(refuels.recordCount, 2)
        XCTAssertEqual(refuels.issues, [.serviceUnavailable])
        XCTAssertEqual(try summary(.expenses, in: export).status, .complete)
        XCTAssertEqual(try summary(.stationClaims, in: export).status, .unavailable)
        XCTAssertEqual(try summary(.trainingConsent, in: export).issues, [.currentContributorOnly])
        XCTAssertEqual(try summary(.cloudSnapshots, in: export).issues, [.boundCloudDeviceRequired])
        XCTAssertTrue(export.isPartial)

        let document = try XCTUnwrap(JSONSerialization.jsonObject(with: export.json) as? [String: Any])
        XCTAssertEqual(document["status"] as? String, "partial")
        let archive = String(decoding: export.json, as: UTF8.self)
        XCTAssertTrue(archive.contains("9007199254740993"))
        for secret in ["not-exported", "page-two", "page-three", "purchaseToken", "privateKey", "latitude", family, owner] {
            XCTAssertFalse(archive.contains(secret))
        }
    }

    func testExportStopsCursorCycleAndIdentifiesUnpaginatedListLimit() async throws {
        let scope = try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: owner, sessionFamilyId: family))
        var subscriptionCalls = 0
        let export = try await AccountExportPlan.build(scope: scope, localForumReceipts: [], verifyScope: {}) { read, _ in
            switch read.section {
            case .subscriptions:
                subscriptionCalls += 1
                return Data(#"{"items":[{"provider":"apple","product_id":"premium","state":"active","verified_at":"2026-01-01T00:00:00Z"}],"next_cursor":"repeat"}"#.utf8)
            case .stationSuggestions:
                let items = (0..<20).map { _ in ["id": UUID().uuidString, "state": "queued"] }
                return try JSONSerialization.data(withJSONObject: ["items": items])
            default:
                return try self.emptyPage(read)
            }
        }
        XCTAssertEqual(subscriptionCalls, 2)
        XCTAssertEqual(try summary(.subscriptions, in: export).recordCount, 1)
        XCTAssertEqual(try summary(.subscriptions, in: export).issues, [.invalidResponse])
        XCTAssertEqual(try summary(.stationSuggestions, in: export).issues, [.listLimitReached])
    }

    func testForumExportUsesOnlyAcceptedReceiptsForExactOwnerAndOrigin() async throws {
        let scope = try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: owner, sessionFamilyId: family))
        let otherScope = "account:00000000-0000-4000-8000-000000000032:fixture"
        let local = [
            CommunityOutboxItem(id: rowA, kind: .forumTopic, ownerScope: owner, origin: origin,
                                createdAtMillis: 0, status: .accepted, receiptId: rowA),
            CommunityOutboxItem(id: rowB, kind: .forumComment, ownerScope: otherScope, origin: origin,
                                createdAtMillis: 0, status: .accepted, receiptId: rowB),
            CommunityOutboxItem(id: rowB, kind: .forumComment, ownerScope: owner, origin: "https://other.example",
                                createdAtMillis: 0, status: .accepted, receiptId: rowB),
            CommunityOutboxItem(id: rowB, kind: .forumComment, ownerScope: owner, origin: origin,
                                createdAtMillis: 0, status: .pending, receiptId: rowB),
        ]
        var conversations: [[String: String]] = []
        let export = try await AccountExportPlan.build(scope: scope, localForumReceipts: local, verifyScope: {}) { read, _ in
            if read.section == .forum {
                conversations.append(read.parameters)
                if let topic = read.parameters["topic_id"] {
                    return Data(#"{"topic":{"id":"\#(topic)","title":"Example title","body_text":"Owned topic"}}"#.utf8)
                }
                return Data(#"{"comments":[{"id":"\#(self.rowB)","post_id":"\#(self.vehicle)","body_text":"Other conversation"}]}"#.utf8)
            }
            return try self.emptyPage(read)
        }
        XCTAssertEqual(conversations, [["topic_id": rowA], ["post_id": rowA]])
        XCTAssertEqual(try summary(.forum, in: export).status, .partial)
        XCTAssertEqual(try summary(.forum, in: export).recordCount, 1)
        XCTAssertEqual(try summary(.forum, in: export).issues, [.knownForumItemsOnly, .invalidResponse])
        XCTAssertFalse(String(decoding: export.json, as: UTF8.self).contains("Other conversation"))
    }

    func testScopeChangeAfterResponseDiscardsTheWholeArchive() async throws {
        let scope = try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: owner, sessionFamilyId: family))
        var scopeStillActive = true
        var reads = 0
        do {
            _ = try await AccountExportPlan.build(scope: scope, localForumReceipts: [], verifyScope: {
                if !scopeStillActive { throw SignedTransportError.sessionRequired }
            }) { read, _ in
                reads += 1
                scopeStillActive = false
                return try self.emptyPage(read)
            }
            XCTFail("Changing the session must discard all data collected by that export.")
        } catch let error as SignedTransportError {
            XCTAssertEqual(error, .sessionRequired)
        }
        XCTAssertEqual(reads, 1)
    }

    func testOwnerReadAuthEnvelopesAndObservationCursor() throws {
        let session = PortableAuth.Session(
            accountId: rowA, familyId: family, accessToken: "fixture-token", refreshToken: "fixture-refresh",
            accessExpiresAt: 12345, absoluteExpiresAt: 67890
        )
        let observations = try XCTUnwrap(AccountExportPlan.reads.first { $0.section == .priceObservations })
        XCTAssertEqual(observations.auth, .contributor)
        XCTAssertEqual(try observations.query(cursor: "abc-123_"), "limit=100&cursor=abc-123_")
        XCTAssertThrowsError(try observations.query(cursor: "abc&limit=1"))

        let charging = try XCTUnwrap(AccountExportPlan.reads.first { $0.section == .chargingObservations })
        XCTAssertEqual(charging.auth, .nestedSession)
        let chargeBody = try body(charging, session: session, cursor: nil)
        XCTAssertEqual(chargeBody["session"] as? [String: String], ["family_id": family, "access_token": "fixture-token"])
        XCTAssertNil(chargeBody["account_id"])
        XCTAssertNil(chargeBody["cursor"])
        XCTAssertEqual(chargeBody["limit"] as? Int, 100)

        let ledger = try XCTUnwrap(AccountExportPlan.reads.first { $0.section == .charges })
        XCTAssertEqual(ledger.auth, .session)
        let ledgerBody = try body(ledger, session: session, cursor: "page-two")
        XCTAssertEqual(ledgerBody["dataset"] as? String, "charge")
        XCTAssertEqual(ledgerBody["cursor"] as? String, "page-two")
        XCTAssertNil(ledgerBody["vehicle_id"])
        XCTAssertNil(ledgerBody["refresh_token"])

        let quality = try XCTUnwrap(AccountExportPlan.reads.first { $0.section == .qualityReports })
        XCTAssertEqual(quality.auth, .signedSession)
        let qualityBody = try body(quality, session: session, cursor: rowA)
        XCTAssertEqual(qualityBody["after_id"] as? String, rowA)
        XCTAssertEqual(qualityBody["family_id"] as? String, family)
        XCTAssertNil(qualityBody["cursor"])
        XCTAssertNil(qualityBody["limit"])
    }

    private func emptyPage(_ read: AccountExportRead) throws -> Data {
        let reply: [String: Any]
        switch read.shape {
        case .page(let items, let cursor):
            reply = [items: [], cursor: ""]
        case .list(let items, _):
            reply = [items: []]
        case .object:
            switch read.section {
            case .cloudStatus: reply = ["epoch": 1, "revision": 0, "consent_revision": 0, "enabled_datasets": []]
            case .trainingConsent: reply = ["granted": false]
            case .forumReactions: reply = ["post_reaction": "NONE", "comments": []]
            default: reply = [:]
            }
        }
        return try JSONSerialization.data(withJSONObject: reply)
    }

    private func ledgerPage(dataset: LedgerDataset, id: String, next: String) throws -> Data {
        try JSONSerialization.data(withJSONObject: [
            "items": [[
                "dataset": dataset.rawValue, "id": id, "vehicle_id": vehicle, "occurred_at": "2026-01-01T00:00:00Z",
                "revision": 1, "voided": false, "accesstoken": "not-exported",
                "payload": ["total_centavos": Int64(9_007_199_254_740_993), "privateKey": "not-exported",
                            "purchaseToken": "not-exported", "latitude": 1.23],
            ]],
            "next_cursor": next,
        ])
    }

    private func body(_ read: AccountExportRead, session: PortableAuth.Session, cursor: String?) throws -> [String: Any] {
        let data = try XCTUnwrap(read.body(session: session, cursor: cursor))
        return try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }

    private func summary(_ id: AccountDataExport.SectionID, in export: AccountDataExport) throws -> AccountDataExport.Section {
        try XCTUnwrap(export.sections.first { $0.id == id })
    }
}
