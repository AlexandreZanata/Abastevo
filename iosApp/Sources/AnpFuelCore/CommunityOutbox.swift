import Foundation

/// Durable community write queue with server receipts (P12-T03 outbox applied to P13/P14/P15 writes).
///
/// Every write is one item with a stable client id used as the
/// `client_submission_id`, `Idempotency-Key` or forum `operation_id`, so a
/// retry replays the same command. States are honest: `draft` waits for a
/// sign-in, `pending` waits for a server receipt, `accepted` carries the
/// receipt id, `refused` carries the server code, and `expired` covers a
/// lapsed photo window or the 25-attempt cap from `PortableOutbox`.
/// `unconfirmed` holds a create the server cannot deduplicate whose outcome is
/// unknown: it is never resent automatically, only after the user reloads and asks.
/// Zero postfix unwrap operators anywhere in this file.
public enum CommunityWriteKind: String, Codable, CaseIterable, Sendable {
    case priceObservation = "PRICE_OBSERVATION"
    case stationComment = "STATION_COMMENT"
    case forumTopic = "FORUM_TOPIC"
    case forumComment = "FORUM_COMMENT"
    case stationSuggestion = "STATION_SUGGESTION"

    /// Price observations are signed by the device contributor; the social routes need an account session.
    public var requiresAccount: Bool {
        return self != .priceObservation
    }
}

public enum CommunityItemStatus: String, Codable, Sendable {
    case draft = "DRAFT"
    case pending = "PENDING"
    case accepted = "ACCEPTED"
    case refused = "REFUSED"
    case expired = "EXPIRED"
    case unconfirmed = "UNCONFIRMED"

    public var isTerminal: Bool {
        switch self {
        case .accepted, .refused, .expired: return true
        case .draft, .pending, .unconfirmed: return false
        }
    }
}

/// Photo evidence authorized by `POST /v1/photo-captures` and captured inside its camera window.
public struct CommunityPhotoEvidence: Codable, Equatable, Sendable {
    public let photoId: String
    public let captureId: String
    public let capturedAtMillis: Int64
    public let captureExpiresAtMillis: Int64
    public var uploadSessionId: String?
    public var evidenceId: String?

    public init(
        photoId: String,
        captureId: String,
        capturedAtMillis: Int64,
        captureExpiresAtMillis: Int64,
        uploadSessionId: String? = nil,
        evidenceId: String? = nil
    ) {
        self.photoId = photoId
        self.captureId = captureId
        self.capturedAtMillis = capturedAtMillis
        self.captureExpiresAtMillis = captureExpiresAtMillis
        self.uploadSessionId = uploadSessionId
        self.evidenceId = evidenceId
    }
}

public struct CommunityPriceDraft: Codable, Equatable, Sendable {
    public let stationId: String
    public let stationName: String
    public let product: FuelProduct
    public let amountMilli: Int64
    public let condition: PriceConditionKind
    public let capturedAtMillis: Int64
    public var photo: CommunityPhotoEvidence?

    public init(
        stationId: String,
        stationName: String,
        product: FuelProduct,
        amountMilli: Int64,
        condition: PriceConditionKind,
        capturedAtMillis: Int64,
        photo: CommunityPhotoEvidence? = nil
    ) {
        self.stationId = stationId
        self.stationName = stationName
        self.product = product
        self.amountMilli = amountMilli
        self.condition = condition
        self.capturedAtMillis = capturedAtMillis
        self.photo = photo
    }
}

public struct CommunityTextDraft: Codable, Equatable, Sendable {
    public let text: String
    public let stationId: String?
    public let product: FuelProduct?
    public let postId: String?
    public let parentId: String?
    public let categoryCode: String?
    public let title: String?
    public let regionUf: String?
    public let regionMunicipalityIbge: Int?

    public init(
        text: String,
        stationId: String? = nil,
        product: FuelProduct? = nil,
        postId: String? = nil,
        parentId: String? = nil,
        categoryCode: String? = nil,
        title: String? = nil,
        regionUf: String? = nil,
        regionMunicipalityIbge: Int? = nil
    ) {
        self.text = text
        self.stationId = stationId
        self.product = product
        self.postId = postId
        self.parentId = parentId
        self.categoryCode = categoryCode
        self.title = title
        self.regionUf = regionUf
        self.regionMunicipalityIbge = regionMunicipalityIbge
    }
}

public struct CommunityOutboxItem: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let kind: CommunityWriteKind
    public var ownerScope: String
    public let origin: String
    public let createdAtMillis: Int64
    public var price: CommunityPriceDraft?
    public var text: CommunityTextDraft?
    public var stationProposal: StationIntakeProposal?
    public var status: CommunityItemStatus
    public var attempts: Int
    public var nextEligibleMillis: Int64
    public var receiptId: String?
    public var serverState: String?
    public var lastCode: String?

    public init(
        id: String,
        kind: CommunityWriteKind,
        ownerScope: String,
        origin: String,
        createdAtMillis: Int64,
        price: CommunityPriceDraft? = nil,
        text: CommunityTextDraft? = nil,
        stationProposal: StationIntakeProposal? = nil,
        status: CommunityItemStatus = .pending,
        attempts: Int = 0,
        nextEligibleMillis: Int64 = 0,
        receiptId: String? = nil,
        serverState: String? = nil,
        lastCode: String? = nil
    ) {
        self.id = id
        self.kind = kind
        self.ownerScope = ownerScope
        self.origin = origin
        self.createdAtMillis = createdAtMillis
        self.price = price
        self.text = text
        self.stationProposal = stationProposal
        self.status = status
        self.attempts = attempts
        self.nextEligibleMillis = nextEligibleMillis
        self.receiptId = receiptId
        self.serverState = serverState
        self.lastCode = lastCode
    }
}

public struct CommunityWriteReceipt: Equatable, Sendable {
    public let id: String
    public let state: String

    public init(id: String, state: String) {
        self.id = id
        self.state = state
    }
}

public struct EvidenceProgress: Equatable, Sendable {
    public let uploadSessionId: String
    public let evidenceId: String?

    public init(uploadSessionId: String, evidenceId: String?) {
        self.uploadSessionId = uploadSessionId
        self.evidenceId = evidenceId
    }
}

public enum CommunityWriteError: Error, Equatable, Sendable {
    case invalid(String)
    case photoExpired
    case photoRejected
    case mediaPending
    case signInRequired
    case invalidReceipt
}

/// The server side of each queued write. `CommunityWriteClient` is the URLSession implementation.
public protocol CommunityWriteGateway: Sendable {
    func uploadEvidence(_ item: CommunityOutboxItem) async throws -> EvidenceProgress
    func submitObservation(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt
    func submitStationComment(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt
    func submitForumTopic(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt
    func submitForumComment(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt
    func submitStationSuggestion(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt
}

public enum CommunityFailureDisposition: Equatable, Sendable {
    case retry(code: String, notBeforeMillis: Int64?)
    case needsSignIn
    case refused(code: String)
    case expired(code: String)
}

public enum CommunityOutbox {
    public static let maxRetainedItems = 200
    public static let ownerChangedCode = "community.owner-or-origin-changed"
    public static let signInRequiredCode = "community.sign-in-required"
    public static let attemptsExhaustedCode = "outbox.attempts-exhausted"
    public static let outcomeUnknownCode = "community.outcome-unknown"

    public static func ownedStationCommentIds(
        _ items: [CommunityOutboxItem], ownerScope: String, origin: String, stationId: String, fuel: FuelProduct
    ) -> Set<String> {
        guard ownerScope.hasPrefix("account:") else { return [] }
        return Set(items.compactMap { item in
            guard item.kind == .stationComment, item.status == .accepted,
                  item.ownerScope == ownerScope, item.origin == origin,
                  item.text?.stationId?.lowercased() == stationId.lowercased(), item.text?.product == fuel,
                  let id = item.receiptId, CommunityWire.isUuid(id) else { return nil }
            return id.lowercased()
        })
    }

    public static func enqueue(_ item: CommunityOutboxItem, into items: [CommunityOutboxItem]) -> [CommunityOutboxItem] {
        var next = items.filter { $0.id != item.id }
        next.insert(item, at: 0)
        return prune(next)
    }

    /// Keeps every open item and the most recent terminal ones.
    public static func prune(_ items: [CommunityOutboxItem]) -> [CommunityOutboxItem] {
        if items.count <= maxRetainedItems { return items }
        var kept: [CommunityOutboxItem] = []
        var terminalBudget = maxRetainedItems - items.filter { !$0.status.isTerminal }.count
        for item in items {
            if !item.status.isTerminal {
                kept.append(item)
            } else if terminalBudget > 0 {
                kept.append(item)
                terminalBudget -= 1
            }
        }
        return kept
    }

    /// The oldest pending item whose backoff elapsed.
    public static func nextEligible(_ items: [CommunityOutboxItem], nowMillis: Int64) -> CommunityOutboxItem? {
        return items.reversed().first { item in
            item.status == .pending && nowMillis >= item.nextEligibleMillis
        }
    }

    /// Earliest future retry among pending items, for scheduling a wake-up.
    public static func nextWakeMillis(_ items: [CommunityOutboxItem], nowMillis: Int64) -> Int64? {
        let future = items.filter { $0.status == .pending && $0.nextEligibleMillis > nowMillis }
        return future.map(\.nextEligibleMillis).min()
    }

    public static func accept(_ item: CommunityOutboxItem, receipt: CommunityWriteReceipt) -> CommunityOutboxItem {
        var next = item
        next.status = .accepted
        next.receiptId = receipt.id
        next.serverState = receipt.state
        next.lastCode = nil
        next.nextEligibleMillis = 0
        return next
    }

    public static func refuse(_ item: CommunityOutboxItem, code: String) -> CommunityOutboxItem {
        var next = item
        next.status = .refused
        next.lastCode = code
        return next
    }

    public static func expire(_ item: CommunityOutboxItem, code: String) -> CommunityOutboxItem {
        var next = item
        next.status = .expired
        next.lastCode = code
        return next
    }

    public static func waitForSignIn(_ item: CommunityOutboxItem) -> CommunityOutboxItem {
        var next = item
        next.status = .draft
        next.lastCode = signInRequiredCode
        return next
    }

    /// Exponential backoff with deterministic jitter; the 25th failure parks the item as expired.
    public static func retryLater(
        _ item: CommunityOutboxItem,
        code: String,
        nowMillis: Int64,
        notBeforeMillis: Int64? = nil
    ) -> CommunityOutboxItem {
        let failed = PortableOutbox.markFailed(
            command: OutboxCommand(commandId: item.id, kind: item.kind.rawValue, payload: "", attempts: item.attempts),
            tickMillis: nowMillis
        )
        var next = item
        next.attempts = failed.attempts
        next.lastCode = code
        if failed.nextEligibleTick == Int64.max {
            next.status = .expired
            next.lastCode = attemptsExhaustedCode
            next.nextEligibleMillis = Int64.max
            return next
        }
        next.status = .pending
        next.nextEligibleMillis = max(failed.nextEligibleTick, notBeforeMillis ?? 0)
        return next
    }

    /// Queues items that waited for a sign-in under the new owner, keeping their retry budget.
    public static func resumeAfterSignIn(_ items: [CommunityOutboxItem], ownerScope: String, origin: String) -> [CommunityOutboxItem] {
        return items.map { item in
            guard item.status == .draft, item.origin == origin else { return item }
            var next = item
            next.status = .pending
            next.ownerScope = ownerScope
            next.nextEligibleMillis = 0
            next.lastCode = nil
            return next
        }
    }

    /// Manual retry of an open item; terminal items stay as they are. An unconfirmed create is
    /// sent again only through this explicit action.
    public static func retryNow(_ items: [CommunityOutboxItem], id: String) -> [CommunityOutboxItem] {
        return items.map { item in
            guard item.id == id, item.status == .pending || item.status == .unconfirmed else { return item }
            var next = item
            next.status = .pending
            next.nextEligibleMillis = 0
            if item.status == .unconfirmed { next.lastCode = nil }
            return next
        }
    }

    /// Station comment creates carry no operation id the feedback API could deduplicate. These
    /// failures may follow an applied create: a connection lost after sending, an unreadable or
    /// malformed 2xx reply, or a server error after the request arrived.
    public static func createOutcomeIsUnknown(_ error: Error) -> Bool {
        if error is SignedDeliveryUncertain { return true }
        if let write = error as? CommunityWriteError { return write == .invalidReceipt }
        guard let transport = error as? SignedTransportError else { return false }
        switch transport {
        case .oversizedResponse, .invalidResponse:
            return true
        case .refused(let status, _, _):
            return status >= 500
        case .invalidRequest, .originUnavailable, .identityUnavailable, .signingUnavailable,
             .sessionRequired, .offline:
            return false
        }
    }

    /// Parks a create whose outcome is unknown until the user reloads and explicitly sends it again.
    public static func awaitReload(_ item: CommunityOutboxItem) -> CommunityOutboxItem {
        var next = item
        next.status = .unconfirmed
        next.attempts = min(item.attempts + 1, PortableOutbox.maxDispatchAttempts)
        next.lastCode = outcomeUnknownCode
        next.nextEligibleMillis = 0
        return next
    }

    public static func classify(_ error: Error, nowMillis: Int64) -> CommunityFailureDisposition {
        if let write = error as? CommunityWriteError {
            switch write {
            case .invalid(let code): return .refused(code: code)
            case .photoExpired: return .expired(code: "contribution.photo-expired")
            case .photoRejected: return .refused(code: "contribution.photo-rejected")
            case .mediaPending: return .retry(code: "contribution.media-pending", notBeforeMillis: nil)
            case .signInRequired: return .needsSignIn
            case .invalidReceipt: return .retry(code: "community.invalid-receipt", notBeforeMillis: nil)
            }
        }
        if let transport = error as? SignedTransportError {
            switch transport {
            case .offline: return .retry(code: "community.offline", notBeforeMillis: nil)
            case .oversizedResponse, .invalidResponse: return .retry(code: "community.invalid-response", notBeforeMillis: nil)
            case .identityUnavailable, .signingUnavailable: return .retry(code: "community.identity-unavailable", notBeforeMillis: nil)
            case .originUnavailable: return .refused(code: "community.origin-unavailable")
            case .invalidRequest: return .refused(code: "community.invalid-request")
            case .sessionRequired: return .needsSignIn
            case .refused(let status, let code, let retryAfter):
                let notBefore = retryAfter.map { seconds -> Int64 in
                    let capped = min(seconds, 86_400)
                    return nowMillis + capped * 1000
                }
                if status == 401 || status == 408 || status == 425 || status == 429 || status >= 500 {
                    return .retry(code: code ?? "http-\(status)", notBeforeMillis: notBefore)
                }
                if code == "forum.disabled" {
                    return .retry(code: "forum.disabled", notBeforeMillis: notBefore)
                }
                return .refused(code: code ?? "http-\(status)")
            }
        }
        return .retry(code: "community.unavailable", notBeforeMillis: nil)
    }

    public static func apply(_ disposition: CommunityFailureDisposition, to item: CommunityOutboxItem, nowMillis: Int64) -> CommunityOutboxItem {
        switch disposition {
        case .retry(let code, let notBefore):
            return retryLater(item, code: code, nowMillis: nowMillis, notBeforeMillis: notBefore)
        case .needsSignIn:
            return waitForSignIn(item)
        case .refused(let code):
            return refuse(item, code: code)
        case .expired(let code):
            return expire(item, code: code)
        }
    }

    /// A photo is usable while its capture receipt and the 24-hour transient deadline are both open.
    public static func isPhotoUsable(_ photo: CommunityPhotoEvidence, nowMillis: Int64) -> Bool {
        return nowMillis < photo.captureExpiresAtMillis
            && !PortablePhoto.isTransientExpired(capturedAtMillis: photo.capturedAtMillis, nowMillis: nowMillis)
    }
}

public struct CommunityDispatchContext: Equatable, Sendable {
    public let nowMillis: Int64
    public let ownerScope: String?
    public let origin: String
    public let hasAccount: Bool

    public init(nowMillis: Int64, ownerScope: String?, origin: String, hasAccount: Bool) {
        self.nowMillis = nowMillis
        self.ownerScope = ownerScope
        self.origin = origin
        self.hasAccount = hasAccount
    }
}

public enum CommunityOutboxDispatcher {
    /// Runs one item against the gateway and returns its next state. Never throws.
    public static func run(
        _ item: CommunityOutboxItem,
        context: CommunityDispatchContext,
        gateway: CommunityWriteGateway,
        clock: @Sendable () -> Int64 = { Int64(Date().timeIntervalSince1970 * 1000) }
    ) async -> CommunityOutboxItem {
        guard item.status == .pending else { return item }
        if item.origin != context.origin || item.ownerScope != context.ownerScope {
            return CommunityOutbox.refuse(item, code: CommunityOutbox.ownerChangedCode)
        }
        if item.kind.requiresAccount && !context.hasAccount {
            return CommunityOutbox.waitForSignIn(item)
        }
        var current = item
        do {
            let receipt: CommunityWriteReceipt
            switch item.kind {
            case .priceObservation:
                guard let price = current.price else {
                    return CommunityOutbox.refuse(current, code: "contribution.invalid")
                }
                if let photo = price.photo, photo.evidenceId == nil {
                    guard CommunityOutbox.isPhotoUsable(photo, nowMillis: context.nowMillis) else {
                        return CommunityOutbox.expire(current, code: "contribution.photo-expired")
                    }
                    let progress = try await gateway.uploadEvidence(current)
                    var updatedPhoto = photo
                    updatedPhoto.uploadSessionId = progress.uploadSessionId
                    updatedPhoto.evidenceId = progress.evidenceId
                    var updatedPrice = price
                    updatedPrice.photo = updatedPhoto
                    current.price = updatedPrice
                    if progress.evidenceId == nil {
                        return CommunityOutbox.retryLater(current, code: "contribution.media-pending", nowMillis: clock())
                    }
                }
                receipt = try await gateway.submitObservation(current)
            case .stationComment:
                receipt = try await gateway.submitStationComment(current)
            case .forumTopic:
                receipt = try await gateway.submitForumTopic(current)
            case .forumComment:
                receipt = try await gateway.submitForumComment(current)
            case .stationSuggestion:
                receipt = try await gateway.submitStationSuggestion(current)
            }
            return CommunityOutbox.accept(current, receipt: receipt)
        } catch {
            if item.kind == .stationComment, CommunityOutbox.createOutcomeIsUnknown(error) {
                return CommunityOutbox.awaitReload(current)
            }
            let now = clock()
            return CommunityOutbox.apply(CommunityOutbox.classify(error, nowMillis: now), to: current, nowMillis: now)
        }
    }
}

/// Backup-excluded JSON file holding the outbox. Owner data: purged on logout and account deletion.
public final class CommunityOutboxStore: @unchecked Sendable {
    private let fileUrl: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        let baseDir = directory ?? IosLocalStore.defaultDirectory()
        try? FileManager.default.createDirectory(at: baseDir, withIntermediateDirectories: true)
        self.fileUrl = baseDir.appendingPathComponent("community-outbox-v1.json")
    }

    public func load() -> [CommunityOutboxItem] {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? Data(contentsOf: fileUrl),
              let items = try? JSONDecoder().decode([CommunityOutboxItem].self, from: data) else {
            return []
        }
        return items
    }

    public func save(_ items: [CommunityOutboxItem]) {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? JSONEncoder().encode(items) else { return }
        try? data.write(to: fileUrl, options: .atomic)
        var mutableUrl = fileUrl
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableUrl.setResourceValues(values)
    }

    public func purge() {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: fileUrl)
    }
}
