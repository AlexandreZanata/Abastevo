import Foundation

/// Personal export assembled only from authenticated owner reads. The archive and its
/// summary live in memory and contain neither credentials nor pagination state.
public struct AccountDataExport: Sendable {
    public enum SectionID: String, Hashable, Sendable {
        case cloudVehicles = "cloud.vehicles"
        case cloudFavorites = "cloud.favorites"
        case cloudPreferences = "cloud.preferences"
        case cloudDriverEarnings = "cloud.driver_earnings"
        case cloudDriverDistance = "cloud.driver_distance"
        case cloudDriverCosts = "cloud.driver_costs"
        case cloudDriverMaintenance = "cloud.driver_maintenance"
        case cloudStatus = "cloud.status"
        case cloudDevices = "cloud.devices"
        case cloudSnapshots = "cloud.snapshots"
        case subscriptions
        case refuels = "ledger.refuel"
        case expenses = "ledger.expense"
        case charges = "ledger.charge"
        case calculatedReports = "calculated_reports"
        case priceObservations = "price_observations"
        case forum
        case forumReactions = "forum_reactions"
        case stationClaims = "station_claims"
        case stationSuggestions = "station_suggestions"
        case stationCorrections = "station_corrections"
        case trainingConsent = "training_consent"
        case blockedUsers = "blocked_users"
        case qualityReports = "quality_reports"
        case chargingObservations = "charging_observations"
        case alertRules = "alert_rules"
        case alertInbox = "alert_inbox"
        case driverEarnings = "driver_earnings"
        case signInProviders = "sign_in_providers"
        case contributorBindings = "contributor_bindings"
    }

    public enum Status: String, Sendable {
        case complete
        case partial
        case unavailable
    }

    public enum Issue: String, Sendable {
        case noOwnerInventory = "no_owner_inventory"
        case knownForumItemsOnly = "known_forum_items_only"
        case currentContributorOnly = "current_contributor_only"
        case listLimitReached = "list_limit_reached"
        case boundCloudDeviceRequired = "bound_cloud_device_required"
        case sessionOrProofRequired = "session_or_proof_required"
        case serviceUnavailable = "service_unavailable"
        case invalidResponse = "invalid_response"
        case readFailed = "read_failed"
    }

    public struct Section: Identifiable, Sendable {
        public let id: SectionID
        public let status: Status
        public let recordCount: Int
        public let issues: [Issue]
    }

    public let json: Data
    public let sections: [Section]

    public var isPartial: Bool { sections.contains { $0.status != .complete } }
}

/// Snapshot the active private scope along with the originating client before any read.
public struct AccountDataExportScope: Equatable, Sendable {
    public let origin: String
    public let ownerScope: String
    public let sessionFamilyId: String

    public init?(origin: String, ownerScope: String, sessionFamilyId: String) {
        guard !origin.isEmpty, ownerScope.hasPrefix("account:"),
              CommunityWire.isUuid(sessionFamilyId) else { return nil }
        self.origin = origin
        self.ownerScope = ownerScope
        self.sessionFamilyId = sessionFamilyId
    }
}

extension CommunityWriteClient {
    public func exportAccountData(
        scope: AccountDataExportScope, localForumReceipts: [CommunityOutboxItem]
    ) async throws -> AccountDataExport {
        func verifyScope() async throws {
            guard transport.origin == scope.origin else { throw SignedTransportError.sessionRequired }
            _ = try await ownerSession(expectedScope: scope.ownerScope, expectedFamilyId: scope.sessionFamilyId)
        }
        try await verifyScope()

        return try await AccountExportPlan.build(
            scope: scope, localForumReceipts: localForumReceipts, verifyScope: verifyScope
        ) { read, cursor in
            let response: SignedHttpResponse
            switch read.auth {
            case .contributor:
                response = try await self.signedOwnerGet(
                    path: read.path, expectedScope: scope.ownerScope, query: try read.query(cursor: cursor),
                    expectedFamilyId: scope.sessionFamilyId
                )
            case .session, .nestedSession, .signedSession:
                response = try await self.ownerPost(
                    path: read.path, expectedScope: scope.ownerScope,
                    requiresDeviceProof: read.auth == .signedSession, requiresSameSessionFamily: true
                ) { session in
                    guard session.familyId == scope.sessionFamilyId else { return nil }
                    return read.body(session: session, cursor: cursor)
                }
            }
            guard response.status == 200 else { throw SignedTransportError.invalidResponse }
            return response.body
        }
    }
}

struct AccountExportRead: Sendable {
    enum Auth: Equatable, Sendable {
        case session
        case nestedSession
        case signedSession
        case contributor
    }

    enum Shape: Sendable {
        case page(items: String, cursor: String)
        case list(items: String, maximum: Int?)
        case object(wrapper: String?)
    }

    let section: AccountDataExport.SectionID
    let path: String
    var auth: Auth = .session
    var shape: Shape = .list(items: "items", maximum: nil)
    var fields: [String]
    var required: [String] = []
    var parameters: [String: String] = [:]
    var requestedLimit: Int?
    var dataset: LedgerDataset?
    var limitation: AccountDataExport.Issue?
    var correlation: [String: String] = [:]

    func body(session: PortableAuth.Session, cursor: String?) -> Data? {
        if let dataset {
            return LedgerSyncWire.recordsBody(
                session: session, dataset: dataset, vehicleId: nil,
                limit: LedgerSyncEngine.pageLimit, cursor: cursor
            )
        }
        guard LedgerSyncWire.isSessionUsable(session) else { return nil }
        let envelope = ["family_id": session.familyId, "access_token": session.accessToken]
        var fields: [String: Any] = parameters
        if auth == .nestedSession {
            fields["session"] = envelope
        } else {
            fields["family_id"] = session.familyId
            fields["access_token"] = session.accessToken
        }
        if let requestedLimit { fields["limit"] = requestedLimit }
        if let cursor, case .page(_, let key) = shape {
            fields[key == "next_after_id" ? "after_id" : "cursor"] = cursor
        }
        return DeviceIdentityWire.encode(fields)
    }

    func query(cursor: String?) throws -> String {
        let limit = requestedLimit.map { "limit=\($0)" } ?? ""
        guard let cursor else { return limit }
        // HTTPAPI cursors are raw-base64url. Encode only its documented alphabet so
        // a received page token cannot inject or rewrite signed query parameters.
        guard cursor.utf8.allSatisfy({
            (48...57).contains($0) || (65...90).contains($0) || (97...122).contains($0) || $0 == 45 || $0 == 95
        }) else { throw CommunityWriteError.invalidReceipt }
        let query = limit + "&cursor=" + cursor
        guard SignedRequestProfile.isSignableQuery(query) else { throw CommunityWriteError.invalidReceipt }
        return query
    }

    func decode(_ data: Data) throws -> (rows: [[String: Any]], cursor: String?) {
        if section == .trainingConsent { _ = try TrainingConsentWire.status(from: data) }
        guard let document = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw CommunityWriteError.invalidReceipt
        }
        let rows: [[String: Any]]
        let next: String?
        switch shape {
        case .object(let wrapper):
            if let wrapper {
                guard let result = document[wrapper] as? [String: Any] else { throw CommunityWriteError.invalidReceipt }
                rows = [result]
            } else {
                rows = [document]
            }
            next = nil
        case .list(let key, let maximum):
            guard let result = document[key] as? [[String: Any]],
                  maximum.map({ result.count <= $0 }) ?? true else { throw CommunityWriteError.invalidReceipt }
            rows = result
            next = nil
        case .page(let key, let cursorKey):
            guard let result = document[key] as? [[String: Any]], result.count <= (cursorKey == "next_after_id" ? 50 : requestedLimit ?? 100) else {
                throw CommunityWriteError.invalidReceipt
            }
            rows = result
            next = try Self.pageCursor(document[cursorKey], key: cursorKey)
        }
        return (try rows.map(project), next)
    }

    private static func pageCursor(_ value: Any?, key: String) throws -> String? {
        if value == nil {
            guard key == "next_after_id" else { throw CommunityWriteError.invalidReceipt }
            return nil
        }
        if value is NSNull { return nil }
        guard let cursor = value as? String else { throw CommunityWriteError.invalidReceipt }
        if cursor.isEmpty { return nil }
        guard LedgerSyncWire.isOpaqueCursor(cursor),
              key != "next_after_id" || CommunityWire.isUuid(cursor) else { throw CommunityWriteError.invalidReceipt }
        return cursor
    }

    private func project(_ row: [String: Any]) throws -> [String: Any] {
        guard required.allSatisfy({ field in
            guard let value = row[field] else { return false }
            return field == "payload" || !(value is NSNull)
        }), correlation.allSatisfy({
            (row[$0.key] as? String)?.caseInsensitiveCompare($0.value) == .orderedSame
        }) else { throw CommunityWriteError.invalidReceipt }
        if required.contains("id") {
            guard let id = row["id"] as? String, CommunityWire.isUuid(id) else {
                throw CommunityWriteError.invalidReceipt
            }
        }
        return try Dictionary(uniqueKeysWithValues: fields.compactMap { field in
            guard let value = row[field] else { return nil }
            return (field, try Self.sanitize(value))
        })
    }

    // Cloud and ledger payloads are structured, allowlisted server data. Do not
    // admit credential or media keys even if a server later adds them to a payload.
    private static let privateFields: Set<String> = [
        "accountid", "accesstoken", "refreshtoken", "purchasetoken", "idtoken", "token", "familyid",
        "session", "sessionid", "secret", "privatekey", "signingkey", "keyfingerprint", "devicefingerprint",
        "installid", "installationid", "signature", "nonce", "challenge", "digest", "providersecret",
        "providercredential", "cursortoken", "cursor", "nextcursor", "objectkey", "objecturl", "signedurl",
        "downloadurl", "uploadurl", "latitude", "longitude", "lat", "lon", "lng", "gps", "coordinates",
        "photobytes", "photo", "photourl", "photoref", "proofpdf", "uploadtoken", "evidenceref", "evidenceid",
    ]

    private static func sanitize(_ value: Any, depth: Int = 0) throws -> Any {
        guard depth < 24 else { throw CommunityWriteError.invalidReceipt }
        if let object = value as? [String: Any] {
            var safe: [String: Any] = [:]
            for (key, node) in object {
                let normalized = key.lowercased().filter { $0.isLetter || $0.isNumber }
                guard !privateFields.contains(normalized) else { continue }
                safe[key] = try sanitize(node, depth: depth + 1)
            }
            return safe
        }
        if let list = value as? [Any] {
            return try list.map { try sanitize($0, depth: depth + 1) }
        }
        return value
    }
}

enum AccountExportPlan {
    typealias Fetch = (AccountExportRead, String?) async throws -> Data
    typealias VerifyScope = () async throws -> Void

    private struct Collected {
        var rows: [[String: Any]] = []
        var issues: [AccountDataExport.Issue] = []
        var received = false
    }

    private static let cursorPage = AccountExportRead.Shape.page(items: "items", cursor: "next_cursor")
    private static let cloudDatasets: [(AccountDataExport.SectionID, String)] = [
        (.cloudVehicles, "vehicles"), (.cloudFavorites, "favorites"), (.cloudPreferences, "preferences"),
        (.cloudDriverEarnings, "driver_earnings"), (.cloudDriverDistance, "driver_distance"),
        (.cloudDriverCosts, "driver_costs"), (.cloudDriverMaintenance, "driver_maintenance"),
    ]
    private static let ledgerDatasets: [(AccountDataExport.SectionID, LedgerDataset)] = [
        (.refuels, .refuel), (.expenses, .expense), (.charges, .charge),
    ]
    private static let cloudFields = ["dataset", "id", "revision", "deleted", "payload"]
    private static let ledgerFields = ["dataset", "id", "vehicle_id", "occurred_at", "revision", "voided", "payload"]
    private static let forumTopicFields = [
        "id", "kind", "category", "title", "body_text", "revision", "publication_state", "visibility",
        "region", "fuel_product", "station_id", "public_alias", "created_at", "updated_at", "published_at",
    ]
    private static let forumCommentFields = [
        "id", "post_id", "parent_id", "depth", "sequence", "revision", "publication_state", "visibility",
        "body_text", "created_at", "updated_at", "published_at",
    ]

    static let reads: [AccountExportRead] =
        cloudDatasets.map { section, dataset in
            AccountExportRead(
                section: section, path: "/v1/cloud/export", shape: cursorPage, fields: cloudFields,
                required: cloudFields, parameters: ["dataset": dataset], requestedLimit: 100,
                correlation: ["dataset": dataset]
            )
        }
        + ledgerDatasets.map { section, dataset in
            AccountExportRead(
                section: section, path: LedgerSyncWire.recordsPath, shape: cursorPage, fields: ledgerFields,
                required: ledgerFields, requestedLimit: LedgerSyncEngine.pageLimit, dataset: dataset,
                correlation: ["dataset": dataset.rawValue]
            )
        }
        + [
            AccountExportRead(
                section: .cloudStatus, path: "/v1/cloud/status", shape: .object(wrapper: nil),
                fields: ["epoch", "revision", "consent_revision", "enabled_datasets"], required: ["epoch", "revision"]
            ),
            AccountExportRead(
                section: .cloudDevices, path: "/v1/cloud/devices/list", shape: cursorPage,
                fields: ["id", "origin", "created_at", "last_success_at", "revoked"], required: ["id"], requestedLimit: 100
            ),
            AccountExportRead(
                section: .subscriptions, path: "/v1/premium/export", shape: cursorPage,
                fields: ["provider", "product_id", "base_plan_id", "state", "valid_until", "verified_at", "retired", "acknowledged"],
                required: ["provider", "product_id", "state", "verified_at"], requestedLimit: 100
            ),
            AccountExportRead(
                section: .priceObservations, path: CommunityWire.ownObservationsPath, auth: .contributor, shape: cursorPage,
                fields: ["id", "station_id", "fuel_product", "unit", "amount_milli_brl", "condition", "validation_state", "received_at", "policy_version"],
                required: ["id", "station_id", "fuel_product"], requestedLimit: 100, limitation: .currentContributorOnly
            ),
            AccountExportRead(
                section: .stationClaims, path: "/v1/profile/claims/mine", shape: .list(items: "items", maximum: 20),
                fields: ["id", "state"], required: ["id", "state"]
            ),
            AccountExportRead(
                section: .stationSuggestions, path: "/v1/stations/suggestions/mine", shape: .list(items: "items", maximum: 20),
                fields: ["id", "client_submission_id", "state", "created_at"], required: ["id", "state"]
            ),
            AccountExportRead(
                section: .stationCorrections, path: "/v1/stations/corrections/mine", shape: .list(items: "items", maximum: 20),
                fields: ["id", "station_id", "field", "status", "created_at", "decided_at", "reason_codes", "client_submission_id"],
                required: ["id", "station_id", "status"]
            ),
            AccountExportRead(
                section: .trainingConsent, path: "/v1/privacy/training-consent", auth: .contributor, shape: .object(wrapper: nil),
                fields: ["granted", "state", "terms_version", "terms_digest", "revision", "decided_at", "needs_regrant"],
                required: ["granted"], limitation: .currentContributorOnly
            ),
            AccountExportRead(
                section: .blockedUsers, path: "/v1/community/blocks/list", auth: .signedSession,
                fields: ["id", "label", "source_kind", "created_at"], required: ["id", "source_kind"]
            ),
            AccountExportRead(
                section: .qualityReports, path: "/v1/quality/reports/list", auth: .signedSession,
                shape: .page(items: "items", cursor: "next_after_id"),
                fields: ["id", "revision", "kind", "station_id", "fuel", "occurred_on", "symptom", "text",
                         "official_revision_id", "sample_ref", "status", "evidence_level", "review_reason"],
                required: ["id", "status"], requestedLimit: nil
            ),
            AccountExportRead(
                section: .chargingObservations, path: "/v1/charging/observations/mine", auth: .nestedSession,
                shape: .list(items: "items", maximum: 100),
                fields: ["id", "connector_code", "condition", "observed_day", "note", "revision",
                         "place_id", "price_milli_brl_per_kwh", "operational"],
                required: ["id", "observed_day"], requestedLimit: 100
            ),
            AccountExportRead(
                section: .alertRules, path: "/v1/alerts/rules", shape: .list(items: "rules", maximum: nil),
                fields: ["rule_id", "revision", "enabled", "payload"], required: ["rule_id", "payload"]
            ),
            AccountExportRead(
                section: .alertInbox, path: "/v1/alerts/inbox", shape: .list(items: "items", maximum: 100),
                fields: ["inbox_id", "rule_id", "status", "payload"], required: ["inbox_id", "rule_id"], requestedLimit: 100
            ),
            AccountExportRead(
                section: .driverEarnings, path: "/v1/driver/earnings", auth: .signedSession, shape: cursorPage,
                fields: ["id", "vehicle_id", "platform", "coverage", "currency", "from", "to", "timezone",
                         "money", "revision", "adjustment_count", "needs_reconciliation"],
                required: ["id", "money", "revision"], requestedLimit: 100
            ),
            AccountExportRead(
                section: .signInProviders, path: "/v1/accounts/providers/list", shape: .list(items: "providers", maximum: nil),
                fields: ["provider", "linked_at"], required: ["provider", "linked_at"]
            ),
            AccountExportRead(
                section: .contributorBindings, path: "/v1/accounts/bindings/list", shape: .list(items: "bindings", maximum: nil),
                fields: ["contributor_id", "bound_at"], required: ["contributor_id", "bound_at"]
            ),
        ]

    static func build(
        scope: AccountDataExportScope,
        localForumReceipts: [CommunityOutboxItem],
        now: Date = Date(),
        verifyScope: VerifyScope,
        fetch: Fetch
    ) async throws -> AccountDataExport {
        var sections: [AccountDataExport.Section] = []
        var payload: [[String: Any]] = []

        func append(_ id: AccountDataExport.SectionID, _ collected: Collected) {
            let status: AccountDataExport.Status
            if collected.issues.isEmpty {
                status = .complete
            } else {
                status = collected.received ? .partial : .unavailable
            }
            sections.append(.init(id: id, status: status, recordCount: collected.rows.count, issues: collected.issues))
            payload.append([
                "id": id.rawValue, "status": status.rawValue,
                "issues": collected.issues.map(\.rawValue), "records": collected.rows,
            ])
        }

        for read in reads {
            append(read.section, try await collect(read, verifyScope: verifyScope, fetch: fetch))
        }
        let readings = forumReads(items: localForumReceipts, scope: scope)
        append(.forum, try await collectForum(readings.content, verifyScope: verifyScope, fetch: fetch))
        append(.forumReactions, try await collectForum(readings.reactions, verifyScope: verifyScope, fetch: fetch))
        append(.cloudSnapshots, Collected(issues: [.boundCloudDeviceRequired]))
        append(.calculatedReports, Collected(issues: [.noOwnerInventory]))

        try await verifyScope()
        let archive: [String: Any] = [
            "format_version": 1,
            "generated_at": ISO8601DateFormatter().string(from: now),
            "status": sections.contains { $0.status != .complete } ? "partial" : "complete",
            "sections": payload,
        ]
        let json = try JSONSerialization.data(withJSONObject: archive, options: [.prettyPrinted, .sortedKeys])
        try await verifyScope()
        return AccountDataExport(json: json, sections: sections)
    }

    private static func collect(
        _ read: AccountExportRead, verifyScope: VerifyScope, fetch: Fetch
    ) async throws -> Collected {
        var collected = Collected(issues: read.limitation.map { [$0] } ?? [])
        var cursor: String?
        var seenCursors: Set<String> = []
        repeat {
            try await verifyScope()
            do {
                let data = try await fetch(read, cursor)
                try await verifyScope()
                let page = try read.decode(data)
                if let next = page.cursor, !seenCursors.insert(next).inserted {
                    throw CommunityWriteError.invalidReceipt
                }
                collected.received = true
                collected.rows.append(contentsOf: page.rows)
                if case .list(_, let max) = read.shape, let max, page.rows.count == max {
                    collected.issues.append(.listLimitReached)
                }
                cursor = page.cursor
            } catch {
                try Task.checkCancellation()
                try await verifyScope()
                collected.issues.append(issue(for: error))
                return collected
            }
        } while cursor != nil
        return collected
    }

    private static func collectForum(
        _ reads: [AccountExportRead], verifyScope: VerifyScope, fetch: Fetch
    ) async throws -> Collected {
        var result = Collected(issues: [.knownForumItemsOnly])
        var contentIDs: Set<String> = []
        for read in reads {
            let collected = try await collect(read, verifyScope: verifyScope, fetch: fetch)
            result.received = result.received || collected.received
            for issue in collected.issues where !result.issues.contains(issue) {
                result.issues.append(issue)
            }
            for var row in collected.rows {
                if let id = row["id"] as? String, !contentIDs.insert(id.lowercased()).inserted { continue }
                if let postId = read.parameters["post_id"], read.section == .forumReactions {
                    row["post_id"] = postId
                }
                result.rows.append(row)
            }
        }
        return result
    }

    private static func issue(for error: Error) -> AccountDataExport.Issue {
        if let error = error as? SignedTransportError {
            switch error {
            case .sessionRequired, .identityUnavailable, .signingUnavailable:
                return .sessionOrProofRequired
            case .offline, .originUnavailable:
                return .serviceUnavailable
            case .refused(let status, let code, _):
                if status == 401 || status == 403 { return .sessionOrProofRequired }
                if status == 404, code?.hasSuffix(".disabled") == true { return .serviceUnavailable }
                return status >= 500 ? .serviceUnavailable : .readFailed
            case .invalidRequest, .invalidResponse, .oversizedResponse:
                return .invalidResponse
            }
        }
        if let error = error as? CommunityWriteError, error == .signInRequired { return .sessionOrProofRequired }
        return .invalidResponse
    }

    private static func forumReads(
        items: [CommunityOutboxItem], scope: AccountDataExportScope
    ) -> (content: [AccountExportRead], reactions: [AccountExportRead]) {
        var topics: Set<String> = []
        var comments: Set<String> = []
        var posts: Set<String> = []
        for item in items where item.origin == scope.origin && item.ownerScope == scope.ownerScope && item.status == .accepted {
            guard let receipt = item.receiptId?.lowercased(), CommunityWire.isUuid(receipt) else { continue }
            switch item.kind {
            case .forumTopic:
                topics.insert(receipt)
                posts.insert(receipt)
            case .forumComment:
                comments.insert(receipt)
                if let post = item.text?.postId?.lowercased(), CommunityWire.isUuid(post) { posts.insert(post) }
            default:
                continue
            }
        }
        let topicsReads = topics.sorted().map { id in
            AccountExportRead(
                section: .forum, path: "/v1/forum/owner/read", auth: .signedSession, shape: .object(wrapper: "topic"),
                fields: forumTopicFields, required: ["id", "body_text", "title"], parameters: ["topic_id": id], correlation: ["id": id]
            )
        }
        let commentReads = comments.sorted().map { id in
            AccountExportRead(
                section: .forum, path: "/v1/forum/owner/read", auth: .signedSession, shape: .object(wrapper: "comment"),
                fields: forumCommentFields, required: ["id", "post_id", "body_text"], parameters: ["comment_id": id], correlation: ["id": id]
            )
        }
        let conversationReads = posts.sorted().map { id in
            AccountExportRead(
                // The owner's comments on one post are not capped by the server (forum discussion.go ownerPostComments).
                section: .forum, path: "/v1/forum/owner/read", auth: .signedSession, shape: .list(items: "comments", maximum: nil),
                fields: forumCommentFields, required: ["id", "post_id", "body_text"], parameters: ["post_id": id], correlation: ["post_id": id]
            )
        }
        let reactionReads = posts.sorted().map { id in
            AccountExportRead(
                section: .forumReactions, path: "/v1/forum/owner/reactions", auth: .signedSession, shape: .object(wrapper: nil),
                fields: ["post_reaction", "comments"], required: ["post_reaction", "comments"], parameters: ["post_id": id]
            )
        }
        return (topicsReads + commentReads + conversationReads, reactionReads)
    }
}
