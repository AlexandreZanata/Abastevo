import Foundation

/// Thin iPhone social-flow state (P17-T03, B-BR-F01…F08, BUC-F01…F05).
///
/// Swift port of the Android `FeedbackViewModel` guards around the shared
/// P17-T01 use-case contract. Flag-gated: `.disabled` performs no network
/// IO and renders nothing (rollback is flag OFF). `prepare` fixes the
/// target plus the caller-held account id; a blank account surfaces
/// `.signInRequired` without IO — anonymous browsing stays available
/// through the read path, which needs no session. Over-280 or blank text
/// is rejected locally (`.rejected("INVALID")`, no IO). Terminal server
/// refusals carry the same fixed kind labels as Android for optimistic
/// rollback; outages queue with one stable op id and `retry` reuses it,
/// so retries never amplify the write. Ratings stay out of the actions:
/// the backend publishes no ratings path (recorded P14 gap), and the
/// gateway refuses explicitly with `.gateRequired` instead of faking
/// success.
///
/// Live HTTP execution (URLSession over the ten published P14 routes),
/// Keychain session storage and background outbox replay are NOT wired
/// here: this slice proves guards, labels and retry identity with pure
/// logic. The Mac run binds the transport against a synthetic server;
/// until then this file is IMPLEMENTED, NOT VERIFIED (see iosApp/README).
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum FeedbackUiState: Equatable {
    case disabled
    case idle
    case signInRequired
    case submitting
    case saved(commentId: String, revision: Int)
    case voted(valid: Int64, invalid: Int64)
    case voteRemoved
    case reported
    case commentDeleted
    case queued(opId: String)
    case rejected(kindLabel: String, message: String)
}

public struct FeedbackFlow {
    public static let voteValid = "VALID"
    public static let voteInvalid = "INVALID"

    private let enabled: () -> Bool
    private var stationId: String?
    private var product: String?
    private var accountId: String?
    private var lastOpId: String?
    private var lastWrite: String?
    public private(set) var state: FeedbackUiState

    public init(isEnabled: @escaping () -> Bool) {
        self.enabled = isEnabled
        self.state = isEnabled() ? .idle : .disabled
    }

    public mutating func prepare(stationId: String, product: String, accountId: String) {
        if !enabled() {
            self.stationId = nil
            self.product = nil
            self.accountId = nil
            self.lastOpId = nil
            self.lastWrite = nil
            self.state = .disabled
            return
        }
        self.stationId = stationId
        self.product = product
        self.accountId = accountId
        self.lastOpId = nil
        self.lastWrite = nil
        self.state = .idle
    }

    public var pendingOpId: String? { lastOpId }

    public mutating func submitComment(text: String) -> FeedbackUiState {
        guard let account = accountId, targetReady else { return state }
        if guardCommon(accountId: account) { return state }
        if guardText(text) { return state }
        let opId = UUID().uuidString
        lastOpId = opId
        lastWrite = "comment"
        state = .submitting
        return state
    }

    public mutating func reply(parentId: String, text: String) -> FeedbackUiState {
        guard let account = accountId, targetReady else { return state }
        if guardCommon(accountId: account) { return state }
        if parentId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            state = .rejected(kindLabel: "INVALID", message: "reply target is blank")
            return state
        }
        if guardText(text) { return state }
        let opId = UUID().uuidString
        lastOpId = opId
        lastWrite = "reply"
        state = .submitting
        return state
    }

    public mutating func editComment(commentId: String, text: String, expectedRevision: Int) -> FeedbackUiState {
        guard let account = accountId, targetReady else { return state }
        if guardCommon(accountId: account) { return state }
        if commentId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || expectedRevision <= 0 {
            state = .rejected(kindLabel: "INVALID", message: "comment revision is invalid")
            return state
        }
        if guardText(text) { return state }
        let opId = UUID().uuidString
        lastOpId = opId
        lastWrite = "edit"
        state = .submitting
        return state
    }

    public mutating func submitVote(commentId: String, choice: String) -> FeedbackUiState {
        guard let account = accountId, targetReady else { return state }
        if guardCommon(accountId: account) { return state }
        if commentId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || (choice != Self.voteValid && choice != Self.voteInvalid) {
            state = .rejected(kindLabel: "INVALID", message: "vote is invalid")
            return state
        }
        let opId = UUID().uuidString
        lastOpId = opId
        lastWrite = "vote"
        state = .submitting
        return state
    }

    public mutating func removeVote(commentId: String) -> FeedbackUiState {
        guard let account = accountId, targetReady else { return state }
        if guardCommon(accountId: account) { return state }
        if commentId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            state = .rejected(kindLabel: "INVALID", message: "comment is blank")
            return state
        }
        let opId = UUID().uuidString
        lastOpId = opId
        lastWrite = "removeVote"
        state = .submitting
        return state
    }

    public mutating func report(commentId: String, reason: String) -> FeedbackUiState {
        guard let account = accountId, targetReady else { return state }
        if guardCommon(accountId: account) { return state }
        if commentId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || reason.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            state = .rejected(kindLabel: "INVALID", message: "report is invalid")
            return state
        }
        let opId = UUID().uuidString
        lastOpId = opId
        lastWrite = "report"
        state = .submitting
        return state
    }

    /// Repeats the last write with the same stable op id.
    public mutating func retry() -> FeedbackUiState {
        guard targetReady else { return state }
        guard lastWrite != nil, let opId = lastOpId else { return state }
        if state == .submitting { return state }
        state = .submitting
        // The caller replays the stored op id against the transport;
        // identity is proven by `pendingOpId` staying equal across retries.
        _ = opId
        return state
    }

    /// Pure server-outcome mapper shared with the Mac transport binding.
    public static func rejectedState(kind: FeedbackRejectKind, message: String) -> FeedbackUiState {
        return .rejected(kindLabel: FeedbackDisplay.rejectKindLabel(kind), message: message)
    }

    private var targetReady: Bool {
        return stationId != nil && product != nil && accountId != nil
    }

    /// Flag + session guard shared by every write; true when handled.
    private mutating func guardCommon(accountId: String) -> Bool {
        if !enabled() {
            state = .disabled
            return true
        }
        if state == .submitting { return true }
        if accountId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            state = .signInRequired
            return true
        }
        return false
    }

    /// 280-scalar guard; true when rejected locally without IO.
    private mutating func guardText(_ text: String) -> Bool {
        if FeedbackDisplay.charsRemaining(text) < 0 || PortableText.normalize(text).isEmpty {
            state = .rejected(kindLabel: "INVALID", message: "comment text exceeds 280 characters")
            return true
        }
        return false
    }
}
