import AnpFuelCore
import Foundation
import Observation

@MainActor
@Observable
public final class TrainingConsentModel {
    public private(set) var status: TrainingConsentStatus?
    public private(set) var receipt: TrainingConsentReceipt?
    public private(set) var refusal: TrainingConsentError?
    public private(set) var review: TrainingConsentReview?
    public private(set) var reviewID = UUID()
    public private(set) var isBusy = false
    public private(set) var isPendingRevocation = false
    public private(set) var isOffline = false
    public private(set) var termsSuperseded = false

    private struct Context: Equatable {
        let scope: TrainingConsentScope
        let familyID: String
    }

    private enum Action: Equatable {
        case read
        case grant
        case revoke
    }

    @ObservationIgnored private let journal: TrainingConsentJournal
    @ObservationIgnored private var client: CommunityWriteClient?
    @ObservationIgnored private var context: Context?
    @ObservationIgnored private var generation = UUID()
    @ObservationIgnored private var operation: Task<Void, Never>?
    @ObservationIgnored private var wakeup: Task<Void, Never>?
    @ObservationIgnored private var isForeground = false

    public init(journal: TrainingConsentJournal = TrainingConsentJournal()) {
        self.journal = journal
    }

    public var hasOwner: Bool { review != nil }

    public var confirmedGrant: Bool {
        !isPendingRevocation && (status?.granted == true || receipt?.state == .granted)
    }

    public var canGrant: Bool {
        hasOwner && !termsSuperseded && !isOffline && !isBusy && !isPendingRevocation && !confirmedGrant
    }

    public func configure(
        client: CommunityWriteClient, origin: String, ownerScope: String?, familyId: String?, offline: Bool
    ) {
        let next: Context?
        if let ownerScope, let familyId, !familyId.isEmpty,
           client.transport.origin == origin,
           let scope = TrainingConsentScope(ownerScope: ownerScope, origin: origin) {
            next = Context(scope: scope, familyID: familyId)
        } else {
            next = nil
        }
        self.client = client
        guard next != context || offline != isOffline else { return }
        let identityChanged = next != context
        invalidateOperation()
        context = next
        isOffline = offline
        status = nil
        receipt = nil
        refusal = nil
        isPendingRevocation = false
        if identityChanged {
            review = next.map { TrainingConsentReview(scope: $0.scope) }
            reviewID = UUID()
            termsSuperseded = false
        }
        resumePendingRevocation()
    }

    /// Invalidates every in-flight response before the caller changes account or server.
    /// Logout passes `keepPendingRevocations` so an undelivered OFF survives until the same
    /// owner signs in again; account deletion erases everything.
    @discardableResult
    public func purgeProtectedState(keepPendingRevocations: Bool = false) -> Bool {
        invalidateOperation()
        context = nil
        status = nil
        receipt = nil
        refusal = nil
        isPendingRevocation = false
        review = nil
        reviewID = UUID()
        termsSuperseded = false
        do {
            if keepPendingRevocations {
                try journal.purgeKeepingPendingRevocations()
            } else {
                try journal.purge()
            }
            return true
        } catch {
            refusal = .localStorage
            return false
        }
    }

    public func foregrounded() {
        isForeground = true
        resumePendingRevocation()
    }

    public func backgrounded() {
        isForeground = false
        wakeup?.cancel()
        wakeup = nil
    }

    public func refresh() {
        guard let context, let local = readJournal(context.scope) else { return }
        if local.pending?.decision == .revoked {
            resumePendingRevocation()
        } else if !isOffline {
            start(.read)
        }
    }

    public func finishReading() {
        review?.finishReading()
    }

    public func checkTerms(_ checked: Bool) {
        review?.check(checked)
    }

    public func grant() {
        guard canGrant, review?.canConfirm == true else { return }
        review?.check(false)
        start(.grant)
    }

    public func revoke() {
        guard let context else { return }
        do {
            try journal.update(context.scope) { $0.requestRevocation(id: UUID()) }
            status = nil
            receipt = nil
            refusal = nil
            review?.check(false)
            isPendingRevocation = true
            resumePendingRevocation()
        } catch {
            refusal = .localStorage
        }
    }

    private var now: Int64 { Int64(Date().timeIntervalSince1970) }

    private func readJournal(_ scope: TrainingConsentScope) -> TrainingConsentJournalState? {
        do {
            let local = try journal.load(scope)
            isPendingRevocation = local.pending?.decision == .revoked
            return local
        } catch {
            refusal = .localStorage
            return nil
        }
    }

    private func resumePendingRevocation() {
        guard let context, let local = readJournal(context.scope), isPendingRevocation,
              !isOffline, isForeground, operation == nil else { return }
        let remaining = local.retryAt - now
        if remaining > 0 {
            scheduleWakeup(after: remaining)
        } else {
            start(.revoke)
        }
    }

    private func scheduleWakeup(after delay: Int64) {
        guard wakeup == nil else { return }
        let token = generation
        wakeup = Task { [weak self] in
            do {
                try await Task.sleep(for: .seconds(min(delay, 1800)))
            } catch {
                return
            }
            guard let self, token == generation else { return }
            wakeup = nil
            resumePendingRevocation()
        }
    }

    private func invalidateOperation() {
        generation = UUID()
        operation?.cancel()
        wakeup?.cancel()
        operation = nil
        wakeup = nil
        isBusy = false
    }

    private func start(_ action: Action) {
        guard operation == nil, let context, let client, !isOffline else { return }
        wakeup?.cancel()
        wakeup = nil
        let token = generation
        let scope = context.scope
        let familyID = context.familyID
        isBusy = true
        refusal = nil
        operation = Task { [weak self] in
            guard let self else { return }
            do {
                switch action {
                case .read:
                    let result = try await client.trainingConsentStatus(expectedScope: scope.ownerScope, expectedFamilyId: familyID)
                    try requireCurrent(token, client: client, scope: scope)
                    if !isPendingRevocation {
                        status = result
                        receipt = nil
                    }
                case .grant:
                    try await sendGrant(client: client, scope: scope, familyID: familyID, token: token)
                case .revoke:
                    try await sendRevocation(client: client, scope: scope, familyID: familyID, token: token)
                }
            } catch {
                if !Task.isCancelled, generation == token {
                    let error = TrainingConsentError.fromTransport(error)
                    refusal = error
                    if error == .termsOutdated { termsSuperseded = true }
                }
            }
            guard generation == token else { return }
            isBusy = false
            operation = nil
            // A user can turn OFF while a read or grant is in flight. Its durable intent takes precedence.
            if action != .revoke || refusal == nil || refusal == .offline || shouldRetry(refusal) {
                resumePendingRevocation()
            }
        }
    }

    private func sendGrant(client: CommunityWriteClient, scope: TrainingConsentScope, familyID: String, token: UUID) async throws {
        try requireCurrent(token, client: client, scope: scope)
        let local = try journal.load(scope)
        guard local.pending?.decision != .revoked else { return }
        guard local.retryAt <= now else {
            throw TrainingConsentError.unavailable(retryAfterSeconds: local.retryAt - now)
        }
        let remote = try await client.trainingConsentStatus(expectedScope: scope.ownerScope, expectedFamilyId: familyID)
        try requireCurrent(token, client: client, scope: scope)
        if isPendingRevocation { return }
        if remote.granted {
            status = remote
            return
        }
        let pending = try journal.update(scope) {
            try $0.requestGrant(id: UUID(), serverRevision: remote.revision)
        }
        guard let command = pending.pending, command.decision == .granted else { return }
        do {
            let accepted = try await client.writeTrainingConsent(command, expectedScope: scope.ownerScope, expectedFamilyId: familyID)
            try requireCurrent(token, client: client, scope: scope)
            try journal.update(scope) { $0.acknowledge(command, receipt: accepted) }
            if !isPendingRevocation {
                status = nil
                receipt = accepted
            }
        } catch {
            try requireCurrent(token, client: client, scope: scope)
            if isPendingRevocation { return }
            let error = TrainingConsentError.fromTransport(error)
            try journal.update(scope) {
                if error == .termsOutdated || error == .invalid || error.canRebindRevocation {
                    $0.discardRefusedGrant(command)
                } else {
                    $0.failed(command, now: now, retryAfterSeconds: retryDelay(error))
                }
            }
            throw error
        }
    }

    private func sendRevocation(client: CommunityWriteClient, scope: TrainingConsentScope, familyID: String, token: UUID) async throws {
        try requireCurrent(token, client: client, scope: scope)
        let local = try journal.load(scope)
        guard var command = local.pending, command.decision == .revoked, local.retryAt <= now else { return }
        do {
            if command.revision == nil {
                command = try await bindRevocation(command, confirmedConflict: false, client: client, scope: scope, familyID: familyID, token: token)
            }
            let accepted: TrainingConsentReceipt
            do {
                accepted = try await client.writeTrainingConsent(command, expectedScope: scope.ownerScope, expectedFamilyId: familyID)
            } catch let error as TrainingConsentError where error.canRebindRevocation {
                command = try await bindRevocation(command, confirmedConflict: true, client: client, scope: scope, familyID: familyID, token: token)
                accepted = try await client.writeTrainingConsent(command, expectedScope: scope.ownerScope, expectedFamilyId: familyID)
            }
            try requireCurrent(token, client: client, scope: scope)
            let updated = try journal.update(scope) { $0.acknowledge(command, receipt: accepted) }
            isPendingRevocation = updated.pending?.decision == .revoked
            if !isPendingRevocation {
                status = nil
                receipt = accepted
            }
        } catch {
            try requireCurrent(token, client: client, scope: scope)
            let error = TrainingConsentError.fromTransport(error)
            try journal.update(scope) {
                $0.failed(command, now: now, retryAfterSeconds: retryDelay(error))
            }
            throw error
        }
    }

    private func bindRevocation(
        _ command: TrainingConsentCommand, confirmedConflict: Bool, client: CommunityWriteClient,
        scope: TrainingConsentScope, familyID: String, token: UUID
    ) async throws -> TrainingConsentCommand {
        let status = try await client.trainingConsentStatus(expectedScope: scope.ownerScope, expectedFamilyId: familyID)
        try requireCurrent(token, client: client, scope: scope)
        let updated = try journal.update(scope) {
            try $0.bindRevocation(command, serverRevision: status.revision, confirmedConflict: confirmedConflict)
        }
        guard let next = updated.pending, next.id == command.id, next.revision != nil else {
            throw TrainingConsentError.conflict
        }
        return next
    }

    private func requireCurrent(_ token: UUID, client: CommunityWriteClient, scope: TrainingConsentScope) throws {
        try Task.checkCancellation()
        guard token == generation, client.transport.origin == scope.origin,
              client.transport.ownerScope() == scope.ownerScope else { throw CancellationError() }
    }

    private func retryDelay(_ error: TrainingConsentError) -> Int64? {
        if case .unavailable(let seconds) = error { return seconds }
        return nil
    }

    private func shouldRetry(_ error: TrainingConsentError?) -> Bool {
        if case .unavailable? = error { return true }
        return false
    }
}
