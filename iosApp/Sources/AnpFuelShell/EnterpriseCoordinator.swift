import Foundation
import Combine
import SwiftUI
import AnpFuelCore

/// The company selected in this app session after a server read and an ACTIVE device contact.
public struct EnterpriseActiveCompany: Equatable {
    public let partition: EnterprisePartition
    public let summary: EnterpriseContextSummary
    public let employeeId: String?
    public let enrollment: EnterpriseEnrollment
    public let confirmedAtMillis: Int64
    /// ACCESS_ENDED: held work is still offered to the server; no new command is accepted.
    public let locked: Bool
}

/// Abastevo Empresas on iOS: company access, device enrollment, the reference feed and
/// the per-organization protected queue of signed company commands.
///
/// Personal and company contexts never mix (ADR-025): every record belongs to the
/// partition it was created in, delivery runs only for the selected company, and a
/// switch or another account leaves earlier work isolated in its own partition.
/// The selection is memory-only and requires an online confirmation, as on Android.
/// iOS records no trip track or background location.
@MainActor
public final class EnterpriseCoordinator: ObservableObject {
    @Published public private(set) var contexts: [EnterpriseContextSummary] = []
    @Published public private(set) var contextsLoaded = false
    @Published public private(set) var isBusy = false
    @Published public private(set) var accessError: EnterpriseAccessError?
    @Published public private(set) var notice: String?
    @Published public private(set) var active: EnterpriseActiveCompany?
    @Published public private(set) var snapshot: EnterprisePartitionSnapshot?
    @Published public private(set) var isDelivering = false
    @Published public private(set) var isRefreshingAssets = false
    @Published private(set) var isRefreshingMissions = false
    @Published private(set) var missionAccessError: EnterpriseAccessError?

    @Published public private(set) var invitationPreview: EnterpriseInvitationPreview?
    @Published public private(set) var invitationResult: EnterpriseInvitationAcknowledgement?
    @Published public private(set) var invitationError: EnterpriseAccessError?
    @Published public private(set) var invitationBusy = false
    @Published public private(set) var invitationRetryAccept: Bool?

    /// Set by the owning view model; an offline simulation pauses network work.
    public var isOfflineProbe: @MainActor () -> Bool = { false }

    private let sessions: AuthFlowSessionSource
    private let identity: IdentityCoordinator
    private let store: EnterprisePartitionStore
    private let uploads: UrlSessionSignedExchange
    private let companyExchange: UrlSessionSignedExchange
    private var company: EnterpriseCompanyClient
    private var generation = UUID()
    private var missionTask: Task<Void, Never>?
    private static let missionProcessID = UUID()
    private var deliveryTask: Task<Void, Never>?
    private var deliveryToken: UUID?
    private var wakeTask: Task<Void, Never>?
    private var invitationCode: String?
    private var invitationDecision: (accept: Bool, operationId: String)?

    init(sessions: AuthFlowSessionSource, identity: IdentityCoordinator, store: EnterprisePartitionStore = EnterprisePartitionStore()) {
        let exchange = UrlSessionSignedExchange()
        self.sessions = sessions
        self.identity = identity
        self.store = store
        self.uploads = UrlSessionSignedExchange(timeout: 60)
        self.companyExchange = exchange
        self.company = EnterpriseCompanyClient(origin: identity.signedTransport.origin, exchange: exchange, sessions: sessions)
    }

    // MARK: - Session and origin

    public var accountId: String? { sessions.currentAccountId() }

    public var origin: String { identity.signedTransport.origin }

    /// Company data needs a canonical HTTPS origin; local development origins stay personal only.
    public var isOriginSupported: Bool { EnterpriseValues.isCanonicalOrigin(origin) }

    private var syncClient: EnterpriseSyncClient {
        EnterpriseSyncClient(transport: identity.signedTransport, sessions: sessions, uploads: uploads)
    }

    private func companyClient() -> EnterpriseCompanyClient {
        if company.origin != origin {
            company = EnterpriseCompanyClient(origin: origin, exchange: companyExchange, sessions: sessions)
        }
        return company
    }

    private static func nowMillis() -> Int64 {
        Int64(Date().timeIntervalSince1970 * 1000)
    }

    /// Withdraws the selection when the account or origin changed under it.
    public func revalidate() {
        guard let current = active else { return }
        if accountId != current.partition.accountId || origin != current.partition.origin {
            leaveCompany()
            contexts = []
            contextsLoaded = false
            accessError = .stale
        }
    }

    /// Logout, account deletion or an origin switch: all company data leaves this device.
    public func purgeOwnedData() {
        cancelMissionRefresh()
        stopDelivery()
        generation = UUID()
        active = nil
        snapshot = nil
        contexts = []
        contextsLoaded = false
        accessError = nil
        notice = nil
        isBusy = false
        isRefreshingAssets = false
        restartInvitation()
        store.purgeAll()
        let client = company
        Task { await client.reset() }
    }

    // MARK: - Contexts

    public func refreshContexts() {
        revalidate()
        guard !isBusy else { return }
        guard accountId != nil else {
            accessError = .signInRequired
            return
        }
        guard isOriginSupported else {
            accessError = .unavailable
            return
        }
        guard !isOfflineProbe() else {
            accessError = .offline
            return
        }
        let client = companyClient()
        let token = generation
        isBusy = true
        accessError = nil
        Task { [weak self] in
            let result: Result<[EnterpriseContextSummary], Error>
            do {
                result = .success(try await client.listContexts())
            } catch {
                result = .failure(error)
            }
            guard let self, self.generation == token else { return }
            self.isBusy = false
            switch result {
            case .success(let list):
                self.contexts = list
                self.contextsLoaded = true
            case .failure(let error):
                self.accessError = EnterpriseCoordinator.accessError(error)
            }
        }
    }

    /// Reads current membership, confirms the device and only then enters the company.
    public func select(_ summary: EnterpriseContextSummary) {
        revalidate()
        guard !isBusy, let account = accountId else {
            if accountId == nil { accessError = .signInRequired }
            return
        }
        guard !isOfflineProbe() else {
            accessError = .offline
            return
        }
        guard let partition = EnterprisePartition(accountId: account, origin: origin, organizationId: summary.organizationId) else {
            accessError = .unavailable
            return
        }
        leaveCompany()
        let token = generation
        let client = companyClient()
        let sync = syncClient
        let store = self.store
        isBusy = true
        accessError = nil
        notice = nil
        Task { [weak self] in
            let result: Result<(EnterpriseContextDetail, EnterpriseContactOutcome, EnterprisePartitionSnapshot), Error>
            do {
                let detail = try await client.readContext(summary)
                store.purgeAccounts(except: account)
                var held = store.load(partition) ?? EnterprisePartitionSnapshot(partition: partition, membershipId: summary.membershipId)
                if held.membershipId != summary.membershipId {
                    held.membershipId = summary.membershipId
                    held.enrollment = nil
                    held.reference = nil
                    held.missionInbox = nil
                }
                let known = held.enrollment
                let outcome: EnterpriseContactOutcome
                do {
                    outcome = try await sync.confirmDevice(partition: partition, membershipId: summary.membershipId, known: known)
                } catch EnterpriseSyncError.refused(let status, _, _) where (status == 403 || status == 404) && known != nil {
                    outcome = try await sync.confirmDevice(partition: partition, membershipId: summary.membershipId, known: nil)
                }
                result = .success((detail, outcome, held))
            } catch {
                result = .failure(error)
            }
            guard let self, self.generation == token else { return }
            self.isBusy = false
            switch result {
            case .success(let value):
                self.applyContact(value.1, detail: value.0, held: value.2)
            case .failure(let error):
                self.accessError = EnterpriseCoordinator.accessError(error)
            }
        }
    }

    private func applyContact(_ outcome: EnterpriseContactOutcome, detail: EnterpriseContextDetail, held: EnterprisePartitionSnapshot) {
        var snapshot = held
        if let mission = snapshot.missionInbox,
           !mission.matches(membershipId: detail.summary.membershipId, enrollmentId: outcome.enrollment.enrollmentId,
                            employeeId: detail.employeeId ?? "") {
            snapshot.missionInbox = nil
        }
        switch outcome.verdict {
        case .erase:
            store.erase(snapshot.partition)
            notice = String(localized: "enterprise.custody.erased")
            return
        case .lock:
            snapshot.locked = true
            snapshot.enrollment = outcome.enrollment
            notice = String(localized: "enterprise.custody.locked_delivery")
        case .active:
            snapshot.locked = false
            snapshot.enrollment = outcome.enrollment
        }
        store.save(snapshot)
        self.snapshot = snapshot
        active = EnterpriseActiveCompany(
            partition: snapshot.partition,
            summary: detail.summary,
            employeeId: detail.employeeId,
            enrollment: outcome.enrollment,
            confirmedAtMillis: Self.nowMillis(),
            locked: snapshot.locked
        )
        if !snapshot.locked {
            refreshAssets()
        }
        deliverNow()
    }

    /// Back to the personal context; company work stays in its own partition.
    public func leaveCompany() {
        cancelMissionRefresh()
        stopDelivery()
        generation = UUID()
        active = nil
        snapshot = nil
        isRefreshingAssets = false
        isBusy = false
    }

    // MARK: - Reference feed

    public func refreshAssets() {
        guard let current = active, !current.locked, !isRefreshingAssets, !isOfflineProbe() else { return }
        let token = generation
        let sync = syncClient
        let prior = snapshot?.reference
        isRefreshingAssets = true
        let received = Self.nowMillis()
        Task { [weak self] in
            let result: Result<EnterpriseReferenceCache, Error>
            do {
                result = .success(try await sync.pullReference(partition: current.partition, enrollment: current.enrollment,
                                                               prior: prior, nowMillis: received))
            } catch {
                result = .failure(error)
            }
            guard let self, self.generation == token else { return }
            self.isRefreshingAssets = false
            switch result {
            case .success(let cache):
                self.mutate(current.partition) { $0.reference = cache }
            case .failure(let error):
                self.notice = EnterpriseCoordinator.syncMessage(error)
            }
        }
    }

    /// Assets of the current feed; an unconfirmed list or an ended assignment needs the person's review.
    public var assets: [EnterpriseReferenceAsset] {
        let values = snapshot?.reference?.assets ?? []
        return values.sorted { lhs, rhs in
            lhs.name.lowercased() == rhs.name.lowercased() ? lhs.id < rhs.id : lhs.name.lowercased() < rhs.name.lowercased()
        }
    }

    public var assetsConfirmed: Bool {
        snapshot?.reference?.isConfirmed(nowMillis: Self.nowMillis()) ?? false
    }

    public func findAsset(_ id: String) -> EnterpriseReferenceAsset? {
        snapshot?.reference?.assets.first { $0.id == id }
    }

    // MARK: - Commands

    private func makeEnvelope(_ current: EnterpriseActiveCompany, now: Int64) -> EnterpriseEnvelope {
        EnterpriseEnvelope(
            partition: current.partition,
            membershipId: current.summary.membershipId,
            enrollmentId: current.enrollment.enrollmentId,
            operationId: EnterpriseValues.newId(),
            declared: EnterpriseTimeDeclaration.at(epochMillis: now, zone: TimeZone.current)
        )
    }

    private func writableCompany() throws -> EnterpriseActiveCompany {
        revalidate()
        guard let current = active else { throw AppValidationError(String(localized: "enterprise.context.choose_before_recording")) }
        if current.locked {
            throw AppValidationError(String(localized: "enterprise.custody.locked_new"))
        }
        return current
    }

    // MARK: - Missions

    private func cancelMissionRefresh() {
        missionTask?.cancel()
        missionTask = nil
        isRefreshingMissions = false
        missionAccessError = nil
    }

    private var boundMissionInbox: EnterpriseMissionInbox? {
        guard let current = active, !current.locked, current.partition.accountId == accountId,
              current.partition.origin == origin, let held = snapshot, held.partition == current.partition,
              let employee = current.employeeId, let inbox = held.missionInbox,
              inbox.matches(membershipId: current.summary.membershipId, enrollmentId: current.enrollment.enrollmentId, employeeId: employee) else {
            return nil
        }
        return inbox
    }

    var missionRows: [EnterpriseMissionRow] {
        guard let inbox = boundMissionInbox, let held = snapshot else { return [] }
        return inbox.rows(queue: held.queue, trips: held.trips)
    }

    var missionsLoaded: Bool { boundMissionInbox != nil }
    var missionAgeMillis: Int64? {
        guard let inbox = boundMissionInbox else { return nil }
        return Self.missionClock().age(of: inbox.received)
    }
    var missionsTruncated: Bool { boundMissionInbox?.page.truncated == true }
    var canRefreshMissions: Bool {
        !isRefreshingMissions && !isOfflineProbe() && active?.locked == false && active?.employeeId != nil
    }

    func missionAssetId(_ id: String) -> String? {
        boundMissionInbox?.page.items.first { $0.id == id }?.assetId
    }

    private static func missionClock() -> EnterpriseMissionClock {
        EnterpriseMissionClock(
            processID: missionProcessID, elapsedMillis: Int64(ProcessInfo.processInfo.systemUptime * 1000),
            wallMillis: nowMillis()
        )
    }

    func refreshMissions() {
        revalidate()
        guard canRefreshMissions, let current = active, let employee = current.employeeId else { return }
        let token = generation
        let selected = companyClient()
        isRefreshingMissions = true
        missionAccessError = nil
        missionTask = Task { [weak self] in
            let result: Result<EnterpriseMissionPage, Error>
            do {
                result = .success(try await selected.missions(partition: current.partition, employeeId: employee))
            } catch {
                result = .failure(error)
            }
            guard let self, generation == token else { return }
            isRefreshingMissions = false
            missionTask = nil
            guard !Task.isCancelled, !isOfflineProbe(), let selected = active, !selected.locked,
                  selected.partition == current.partition, selected.summary.membershipId == current.summary.membershipId,
                  selected.enrollment.enrollmentId == current.enrollment.enrollmentId, selected.employeeId == employee,
                  current.partition.accountId == accountId, current.partition.origin == origin,
                  var held = snapshot, held.partition == current.partition else { return }
            do {
                let page = try result.get()
                let received = Self.missionClock()
                if var inbox = boundMissionInbox {
                    try inbox.refresh(page, received: received)
                    held.missionInbox = inbox
                } else {
                    held.missionInbox = try EnterpriseMissionInbox(
                        membershipId: current.summary.membershipId, enrollmentId: current.enrollment.enrollmentId,
                        employeeId: employee, page: page, received: received
                    )
                }
                guard store.save(held) else { throw EnterpriseAccessError.unavailable }
                snapshot = held
            } catch {
                missionAccessError = Self.accessError(error)
            }
        }
    }

    func respondToMission(_ id: String, response: EnterpriseMissionResponse, reason: String?) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard var inbox = boundMissionInbox, let item = inbox.page.items.first(where: { $0.id == id }) else {
                throw EnterpriseMissionError.notAnswerable
            }
            let now = Self.nowMillis()
            let operation = try EnterpriseOperationBuilder.respondToMission(
                envelope: makeEnvelope(current, now: now), missionId: id, revision: item.revision, response: response, reason: reason
            )
            try inbox.addAnswer(missionId: id, operationId: operation.operationId, revision: item.revision, response: response)
            let summary = response == .accept
                ? String(localized: "android.enterprise_missions_accept")
                : String(localized: "android.enterprise_missions_decline")
            try enqueue(
                EnterpriseQueueItem(operation: operation, kind: .missionResponse, summary: summary, createdAtMillis: now),
                partition: current.partition
            ) { held in
                held.missionInbox = inbox
            }
            return .success(())
        } catch let error as EnterpriseMissionError {
            return .failure(Self.missionValidation(error))
        } catch EnterpriseOperationError.malformed {
            return .failure(AppValidationError(String(localized: "enterprise.meter.reason_invalid")))
        } catch {
            return .failure(Self.validation(error))
        }
    }

    private static func missionValidation(_ error: EnterpriseMissionError) -> AppValidationError {
        let message: String
        switch error {
        case .notAccepted: message = String(localized: "android.enterprise_missions_start_not_confirmed")
        case .alreadyStarted: message = String(localized: "android.enterprise_missions_start_already")
        case .wrongAsset: message = String(localized: "android.enterprise_missions_refusal_wrong_vehicle")
        case .notAnswerable: message = String(localized: "android.enterprise_missions_refusal_not_answerable")
        case .alreadyAnswered: message = String(localized: "android.enterprise_missions_refusal_answered")
        case .invalidPage, .foreignBinding: message = String(localized: "android.enterprise_missions_unavailable")
        }
        return AppValidationError(message)
    }

    /// `trip.start` for an assigned asset with a reading or a written reason.
    public func startTrip(
        assetId: String, evidence: EnterpriseMeterEvidence, departureRunId: String?, missionId: String? = nil
    ) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard let employee = current.employeeId else {
                throw AppValidationError(String(localized: "enterprise.driver.unknown"))
            }
            guard let asset = findAsset(assetId) else { throw AppValidationError(String(localized: "enterprise.assets.choose_from_list")) }
            if snapshot?.openTrip != nil {
                throw AppValidationError(String(localized: "enterprise.trip.already_open"))
            }
            if let missionId {
                guard let held = snapshot, let inbox = boundMissionInbox else { throw EnterpriseMissionError.notAccepted }
                try inbox.readyTrip(missionId: missionId, assetId: assetId, queue: held.queue, trips: held.trips)
            }
            let now = Self.nowMillis()
            let tripId = EnterpriseValues.newId()
            let operation = try EnterpriseOperationBuilder.startTrip(
                envelope: makeEnvelope(current, now: now), tripId: tripId, assetId: asset.id, driverEmployeeId: employee,
                evidence: evidence, departureRunId: departureRunId, missionId: missionId
            )
            let label = Self.assetLabel(asset)
            var meterId: String?
            if case .reading(let id, _, _, _, _, _) = evidence { meterId = id }
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .tripStart, summary: String(localized: "enterprise.queue.trip_start", defaultValue: "Trip start · \(label)"),
                                            createdAtMillis: now), partition: current.partition) { snapshot in
                snapshot.trips.append(EnterpriseTripRecord(id: tripId, assetId: asset.id, assetLabel: label,
                                                           startOperationId: operation.operationId, startedAtMillis: now,
                                                           startMeterId: meterId, missionId: missionId))
            }
            return .success(())
        } catch let error as EnterpriseMissionError {
            return .failure(Self.missionValidation(error))
        } catch {
            return .failure(Self.validation(error))
        }
    }

    /// `trip.finish` of the open trip; it depends on the start until the server received it.
    func finishTrip(
        evidence: EnterpriseMeterEvidence, tripId: String, returnRunId: String?,
        expectedBinding: EnterpriseReturnChecklistBinding
    ) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard let trip = snapshot?.openTrip, trip.id == tripId,
                  returnChecklistBinding(tripId: trip.id) == expectedBinding else {
                throw AppValidationError(String(localized: "enterprise.trip.none_open"))
            }
            if let returnRunId {
                guard let completed = returnChecklistCompletion(tripId: trip.id, runId: returnRunId),
                      snapshot?.queue.first(where: { $0.id == completed.operationId })?.status == .accepted else {
                    throw AppValidationError(String(localized: "enterprise.trip_transition.wait_return"))
                }
            }
            let startItem = snapshot?.queue.first { $0.id == trip.startOperationId }
            if startItem?.status == .rejected {
                throw AppValidationError(String(localized: "enterprise.trip.start_refused"))
            }
            let dependency: String? = startItem?.status.isReceived == true ? nil : trip.startOperationId
            let now = Self.nowMillis()
            guard now >= (trip.lifecycle?.last?.declaredAtMillis ?? trip.startedAtMillis) else {
                throw AppValidationError(String(localized: "enterprise.trip_transition.changed"))
            }
            let operation = try EnterpriseOperationBuilder.finishTrip(
                envelope: makeEnvelope(current, now: now), tripId: trip.id, evidence: evidence,
                startOperationId: dependency, returnRunId: returnRunId
            )
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .tripFinish,
                                            summary: String(localized: "enterprise.queue.trip_finish", defaultValue: "Trip end · \(trip.assetLabel)"), createdAtMillis: now),
                        partition: current.partition) { snapshot in
                if let index = snapshot.trips.firstIndex(where: { $0.id == trip.id }) {
                    snapshot.trips[index].finishOperationId = operation.operationId
                    snapshot.trips[index].finishedAtMillis = now
                }
            }
            return .success(())
        } catch {
            return .failure(Self.validation(error))
        }
    }

    public func transitionTrip(
        _ action: EnterpriseTripTransitionAction, tripId: String, reasonText: String,
        expectedPartition: EnterprisePartition
    ) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard current.partition == expectedPartition, let original = snapshot?.openTrip,
                  original.id == tripId, original.admits(action) else {
                throw AppValidationError(String(localized: "enterprise.trip_transition.changed"))
            }
            let start = snapshot?.queue.first { $0.id == original.startOperationId }
            if start?.status == .rejected {
                throw AppValidationError(String(localized: "enterprise.trip.start_refused"))
            }
            let reason = reasonText.trimmingCharacters(in: .whitespacesAndNewlines)
            if action == .cancel && reason.isEmpty {
                throw AppValidationError(String(localized: "enterprise.trip_transition.reason_required"))
            }
            let now = Self.nowMillis()
            let operation = try EnterpriseOperationBuilder.transitionTrip(
                envelope: makeEnvelope(current, now: now), tripId: tripId, action: action,
                expectedRevision: original.nextLifecycleRevision, reason: reason.isEmpty ? nil : reason,
                startOperationId: start?.status.isReceived == true ? nil : original.startOperationId
            )
            var changed = original
            try changed.recordTransition(action, operationId: operation.operationId, declaredAtMillis: now)
            let label = EnterpriseTripLifecycleLabels.action(action)
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .tripLifecycle, summary: label, createdAtMillis: now),
                        partition: current.partition) { snapshot in
                if let index = snapshot.trips.firstIndex(where: { $0.id == tripId }) {
                    snapshot.trips[index] = changed
                }
            }
            return .success(())
        } catch {
            return .failure(Self.validation(error))
        }
    }

    /// Removes a trip whose start the server refused; nothing else is ever discarded locally.
    public func dismissRefusedTrip(_ tripId: String) {
        guard let current = active, let trip = snapshot?.trips.first(where: { $0.id == tripId }),
              snapshot?.queue.first(where: { $0.id == trip.startOperationId })?.status == .rejected else {
            return
        }
        mutate(current.partition) { snapshot in
            snapshot.trips.removeAll { $0.id == tripId }
        }
    }

    /// `expense.submit` with an optional receipt photo delivered after the server receives the expense.
    public func submitExpense(assetId: String, category: String, paidText: String, companyText: String,
                              payer: EnterpriseExpensePayer, receipt: Data?) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard let asset = findAsset(assetId) else { throw AppValidationError(String(localized: "enterprise.assets.choose_from_list")) }
            guard let paid = try? PortableMoney.parseBrlCentavos(paidText) else {
                throw AppValidationError(String(localized: "android.enterprise_expenses_capture_invalid"))
            }
            let trimmedCompany = companyText.trimmingCharacters(in: .whitespacesAndNewlines)
            let companyAmount: Int64
            if trimmedCompany.isEmpty {
                companyAmount = paid
            } else if let parsed = try? PortableMoney.parseBrlCentavos(trimmedCompany) {
                companyAmount = parsed
            } else {
                throw AppValidationError(String(localized: "android.enterprise_expenses_capture_invalid"))
            }
            if companyAmount > paid {
                throw AppValidationError(String(localized: "android.enterprise_expenses_capture_invalid"))
            }
            if let receipt, receipt.isEmpty || receipt.count > EnterpriseAttachmentWire.maxBytes {
                throw AppValidationError(String(localized: "android.enterprise_receipts_refused_too_large"))
            }
            let now = Self.nowMillis()
            let expenseId = EnterpriseValues.newId()
            let operation = try EnterpriseOperationBuilder.submitExpense(
                envelope: makeEnvelope(current, now: now), expenseId: expenseId, assetId: asset.id, category: category,
                paidCentavos: paid, companyCentavos: companyAmount, payer: payer
            )
            var attachment: EnterpriseAttachmentItem?
            if let receipt {
                let documentId = EnterpriseValues.newId()
                guard store.saveReceipt(receipt, partition: current.partition, documentId: documentId) else {
                    throw AppValidationError(String(localized: "android.enterprise_receipts_local_invalid"))
                }
                attachment = EnterpriseAttachmentItem(
                    documentId: documentId, declarationId: EnterpriseValues.newId(), linkId: EnterpriseValues.newId(),
                    sha256: EnterpriseCanonicalJson.sha256Hex(receipt), bytes: Int64(receipt.count),
                    targetKind: "EXPENSE", targetId: expenseId, ownerOperationId: operation.operationId
                )
            }
            let label = "\(Self.categoryLabel(category)) · \(AbastevoTheme.formatCentavos(paid))"
            do {
                try enqueue(EnterpriseQueueItem(operation: operation, kind: .expense, summary: String(localized: "enterprise.queue.expense", defaultValue: "Expense · \(label)"),
                                                createdAtMillis: now), partition: current.partition) { snapshot in
                    if let attachment { snapshot.attachments.append(attachment) }
                }
            } catch {
                if let attachment { store.deleteReceipt(partition: current.partition, documentId: attachment.documentId) }
                throw error
            }
            return .success(())
        } catch {
            return .failure(Self.validation(error))
        }
    }

    /// Writes the fueling and its optional, separate receipt target before delivery.
    func submitFueling(
        partition: EnterprisePartition, membershipId: String, assetId: String, form: EnterpriseFuelingFormInput,
        evidence: EnterpriseMeterEvidence, receipt: Data?
    ) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard current.partition == partition, current.summary.membershipId == membershipId,
                  let responsibleId = current.employeeId else {
                throw AppValidationError(String(localized: "android.enterprise_fuelings_responsible_unknown"))
            }
            guard let asset = findAsset(assetId) else {
                throw AppValidationError(String(localized: "enterprise.assets.choose_from_list"))
            }
            let input = try form.makeInput(asset: asset, responsibleId: responsibleId, evidence: evidence)
            let now = Self.nowMillis()
            let id = EnterpriseValues.newId()
            let operation = try EnterpriseFuelingOperations.build(
                envelope: makeEnvelope(current, now: now), fuelingId: id, input: input
            )
            let attachment = try receipt.map {
                try prepareFuelingReceipt($0, partition: partition, operation: operation)
            }
            let label = Self.assetLabel(asset)
            do {
                try enqueue(EnterpriseQueueItem(
                    operation: operation, kind: .fueling,
                    summary: String(localized: "enterprise.fueling.queue_title", defaultValue: "Fueling · \(label)"),
                    createdAtMillis: now
                ), partition: current.partition) { snapshot in
                    if let attachment { snapshot.attachments.append(attachment) }
                }
            } catch {
                if let attachment { store.deleteReceipt(partition: partition, documentId: attachment.documentId) }
                throw error
            }
            return .success(())
        } catch is EnterpriseOperationError {
            return .failure(AppValidationError(String(localized: "android.enterprise_fuelings_capture_invalid")))
        } catch {
            return .failure(Self.validation(error))
        }
    }

    func attachFuelingReceipt(
        _ bytes: Data, operationId: String, partition: EnterprisePartition, membershipId: String
    ) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard current.partition == partition, current.summary.membershipId == membershipId,
                  var held = snapshot, let owner = held.queue.first(where: { $0.id == operationId && $0.kind == .fueling }),
                  owner.status != .rejected else {
                throw AppValidationError(String(localized: "android.enterprise_receipts_local_unknown_record"))
            }
            var attachment = try prepareFuelingReceipt(bytes, partition: partition, operation: owner.operation)
            if owner.status.isReceived { attachment.status = .pending }
            held.attachments.append(attachment)
            guard store.save(held) else {
                store.deleteReceipt(partition: partition, documentId: attachment.documentId)
                throw AppValidationError(String(localized: "android.enterprise_receipts_local_invalid"))
            }
            snapshot = held
            deliverNow()
            return .success(())
        } catch {
            return .failure(Self.validation(error))
        }
    }

    private func prepareFuelingReceipt(
        _ receipt: Data, partition: EnterprisePartition, operation: EnterpriseSealedOperation
    ) throws -> EnterpriseAttachmentItem {
        guard !receipt.isEmpty, receipt.count <= EnterpriseAttachmentWire.maxBytes else {
            throw AppValidationError(String(localized: "android.enterprise_receipts_refused_too_large"))
        }
        guard receipt.count >= 4, receipt.starts(with: [0xff, 0xd8, 0xff]), receipt.suffix(2).elementsEqual([0xff, 0xd9]) else {
            throw AppValidationError(String(localized: "android.enterprise_receipts_refused_unsupported"))
        }
        let attachments = snapshot?.attachments ?? []
        guard attachments.count < 500 else {
            throw AppValidationError(String(localized: "android.enterprise_receipts_local_full"))
        }
        let pendingBytes = attachments.filter {
            $0.status == .waitingOwner || $0.status == .pending || $0.status == .exhausted
        }.reduce(Int64(0)) { total, item in total + item.bytes }
        guard pendingBytes + Int64(receipt.count) <= 16 * 1024 * 1024 else {
            throw AppValidationError(String(localized: "android.enterprise_receipts_local_budget"))
        }
        let documentId = EnterpriseValues.newId()
        guard store.saveReceipt(receipt, partition: partition, documentId: documentId) else {
            throw AppValidationError(String(localized: "android.enterprise_receipts_local_invalid"))
        }
        return EnterpriseAttachmentItem(
            documentId: documentId, declarationId: EnterpriseValues.newId(), linkId: EnterpriseValues.newId(),
            sha256: EnterpriseCanonicalJson.sha256Hex(receipt), bytes: Int64(receipt.count),
            targetKind: "FUELING", targetId: operation.entityId, ownerOperationId: operation.operationId
        )
    }

    /// Reads the applicable departure checklist of an asset on the company channel.
    public func loadChecklist(assetId: String, context: String) async -> Result<EnterpriseChecklistApplicable, AppValidationError> {
        guard let current = active, !isOfflineProbe() else {
            return .failure(AppValidationError(String(localized: "enterprise.checklist.connect")))
        }
        do {
            let applicable = try await companyClient().applicableChecklist(
                organizationId: current.partition.organizationId, assetId: assetId, context: context
            )
            return .success(applicable)
        } catch {
            return .failure(AppValidationError(Self.accessMessage(Self.accessError(error))))
        }
    }

    /// `check.run.complete`; returns the run id for the trip start's departure reference.
    public func completeChecklist(_ applicable: EnterpriseChecklistApplicable, answers: [EnterpriseChecklistAnswer?], notes: [String],
                                  acknowledged: Bool, startedAt: String) -> Result<(runId: String, outcome: EnterpriseChecklistOutcome), AppValidationError> {
        do {
            let current = try writableCompany()
            guard applicable.required, let templateId = applicable.templateId, let version = applicable.version,
                  let revision = applicable.policyRevision, let mode = applicable.policyMode, notes.count == answers.count else {
                throw AppValidationError(String(localized: "android.enterprise_checklists_none_required"))
            }
            let evaluation = try EnterpriseChecklistRules.evaluate(items: applicable.items, policyMode: mode, answers: answers)
            if evaluation.acknowledgementRequired && !acknowledged {
                throw AppValidationError(String(localized: "android.enterprise_checklists_refusal_ack"))
            }
            let responses = zip(answers, notes).map { pair -> EnterpriseOperationBuilder.ChecklistResponse in
                EnterpriseOperationBuilder.ChecklistResponse(
                    answer: pair.0 ?? EnterpriseChecklistAnswer.ok,
                    note: pair.0 == EnterpriseChecklistAnswer.notOk ? pair.1.trimmingCharacters(in: .whitespacesAndNewlines) : ""
                )
            }
            let now = Self.nowMillis()
            let runId = EnterpriseValues.newId()
            let operation = try EnterpriseOperationBuilder.completeChecklist(
                envelope: makeEnvelope(current, now: now), runId: runId, assetId: applicable.assetId, context: applicable.context,
                templateId: templateId, templateVersion: version, policyRevision: revision, startedAt: startedAt,
                responses: responses, outcome: evaluation.outcome, acknowledged: evaluation.acknowledgementRequired && acknowledged
            )
            let label = findAsset(applicable.assetId).map { Self.assetLabel($0) } ?? (applicable.name ?? "")
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .checklist, summary: String(localized: "enterprise.queue.checklist", defaultValue: "Departure checklist · \(label)"),
                                            createdAtMillis: now), partition: current.partition) { _ in }
            return .success((runId, evaluation.outcome))
        } catch let failure as EnterpriseChecklistFailure {
            switch failure {
            case .photoRequired:
                return .failure(AppValidationError(String(localized: "enterprise.checklist.photo_contact_fleet")))
            case .answerNotAllowed:
                return .failure(AppValidationError(String(localized: "android.enterprise_checklists_refusal_not_allowed")))
            case .incomplete:
                return .failure(AppValidationError(String(localized: "android.enterprise_checklists_refusal_incomplete")))
            case .invalid:
                return .failure(AppValidationError(String(localized: "enterprise.checklist.invalid")))
            }
        } catch {
            return .failure(Self.validation(error))
        }
    }

    func returnChecklistBinding(tripId: String) -> EnterpriseReturnChecklistBinding? {
        guard let current = active, current.partition.accountId == accountId, current.partition.origin == origin,
              !current.locked, snapshot?.partition == current.partition, let trip = snapshot?.openTrip, trip.id == tripId else { return nil }
        return EnterpriseReturnChecklistBinding(partition: current.partition, membershipId: current.summary.membershipId,
                                                enrollmentId: current.enrollment.enrollmentId, tripId: trip.id,
                                                assetId: trip.assetId, generation: generation)
    }

    func returnChecklistBook(_ binding: EnterpriseReturnChecklistBinding) -> EnterpriseReturnChecklistBook? {
        guard returnChecklistBinding(tripId: binding.tripId) == binding,
              let book = snapshot?.returnChecklists, book.matches(membershipId: binding.membershipId, enrollmentId: binding.enrollmentId) else {
            return nil
        }
        return book
    }

    func refreshReturnChecklist(_ binding: EnterpriseReturnChecklistBinding) async -> Result<Void, AppValidationError> {
        guard returnChecklistBinding(tripId: binding.tripId) == binding, !isOfflineProbe() else {
            return .failure(AppValidationError(String(localized: "enterprise.checklist.connect")))
        }
        do {
            let applicable = try await companyClient().applicableChecklist(organizationId: binding.partition.organizationId,
                                                                           assetId: binding.assetId, context: "RETURN")
            guard !Task.isCancelled else { return .success(()) }
            try changeReturnChecklist(binding) { book, _ in try book.remember(applicable) }
            return .success(())
        } catch {
            if !Task.isCancelled { return .failure(returnChecklistFailure(error)) }
            return .success(())
        }
    }

    func startReturnChecklist(_ binding: EnterpriseReturnChecklistBinding) -> Result<Void, AppValidationError> {
        do {
            let rejected = (returnChecklistBook(binding)?.draft?.completion).flatMap { completed in
                snapshot?.queue.first(where: { $0.id == completed.operationId })?.status == .rejected ? completed.runId : nil
            }
            try changeReturnChecklist(binding) { book, trip in
                _ = try book.start(trip: trip, nowMillis: Self.nowMillis(), rejectedRunId: rejected)
            }
            return .success(())
        } catch { return .failure(returnChecklistFailure(error)) }
    }

    func updateReturnChecklist(_ binding: EnterpriseReturnChecklistBinding, draftId: String,
                               position: Int, answer: EnterpriseChecklistAnswer?, note: String) -> Result<Void, AppValidationError> {
        do {
            try changeReturnChecklist(binding) { book, trip in
                guard var draft = book.draft, draft.id == draftId else { throw EnterpriseReturnChecklistError.binding }
                try draft.update(position: position, answer: answer, note: note, nowMillis: Self.nowMillis())
                try book.replace(draft, trip: trip)
            }
            return .success(())
        } catch { return .failure(returnChecklistFailure(error)) }
    }

    func acknowledgeReturnChecklist(_ binding: EnterpriseReturnChecklistBinding, draftId: String,
                                    acknowledged: Bool) -> Result<Void, AppValidationError> {
        do {
            try changeReturnChecklist(binding) { book, trip in
                guard var draft = book.draft, draft.id == draftId else { throw EnterpriseReturnChecklistError.binding }
                try draft.acknowledge(acknowledged, nowMillis: Self.nowMillis())
                try book.replace(draft, trip: trip)
            }
            return .success(())
        } catch { return .failure(returnChecklistFailure(error)) }
    }

    func completeReturnChecklist(_ binding: EnterpriseReturnChecklistBinding, draftId: String) -> Result<Void, AppValidationError> {
        do {
            let company = try writableCompany()
            guard returnChecklistBinding(tripId: binding.tripId) == binding, let trip = snapshot?.openTrip,
                  var book = returnChecklistBook(binding), var draft = book.draft, draft.id == draftId, draft.matches(trip) else {
                throw EnterpriseReturnChecklistError.binding
            }
            let now = Self.nowMillis()
            let operation = try draft.complete(envelope: makeEnvelope(company, now: now), nowMillis: now)
            try book.replace(draft, trip: trip)
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .checklist,
                                            summary: String(localized: "enterprise.checklist.return_title"), createdAtMillis: now),
                        partition: binding.partition) { snapshot in
                snapshot.returnChecklists = book
            }
            return .success(())
        } catch { return .failure(returnChecklistFailure(error)) }
    }

    public func returnChecklistCompletion(tripId: String, runId: String) -> EnterpriseReturnChecklistCompletion? {
        guard let binding = returnChecklistBinding(tripId: tripId), let trip = snapshot?.openTrip,
              let draft = returnChecklistBook(binding)?.draft, draft.matches(trip),
              let completed = draft.completion, completed.tripId == trip.id, completed.runId == runId,
              completed.completedAtMillis >= trip.startedAtMillis, completed.completedAtMillis <= Self.nowMillis(),
              let item = snapshot?.queue.first(where: { $0.id == completed.operationId }), item.kind == .checklist,
              item.operation.entityId == completed.runId, item.operation.type == EnterpriseOperationBuilder.checklistComplete,
              item.status != .rejected,
              let fields = try? EnterpriseCanonicalJson.parse(item.operation.canonical).objectValue,
              fields["membership_id"]?.textValue == binding.membershipId,
              fields["enrollment_id"]?.textValue == binding.enrollmentId,
              fields["owner_account_id"]?.textValue == binding.partition.accountId,
              fields["api_origin"]?.textValue == binding.partition.origin,
              fields["org_id"]?.textValue == binding.partition.organizationId,
              let payload = fields["payload"]?.objectValue, payload["context"]?.textValue == "RETURN",
              payload["asset_id"]?.textValue == trip.assetId else { return nil }
        return completed
    }

    func refreshReturnChecklistRun(_ binding: EnterpriseReturnChecklistBinding,
                                   completion: EnterpriseReturnChecklistCompletion) async -> Result<Void, AppValidationError> {
        guard !isOfflineProbe(), returnChecklistCompletion(tripId: binding.tripId, runId: completion.runId) == completion,
              snapshot?.queue.first(where: { $0.id == completion.operationId })?.status == .accepted else {
            return .failure(AppValidationError(String(localized: "enterprise.checklist.connect")))
        }
        do {
            let run = try await companyClient().returnChecklistRun(organizationId: binding.partition.organizationId,
                                                                  assetId: binding.assetId, runId: completion.runId)
            guard !Task.isCancelled else { return .success(()) }
            try changeReturnChecklist(binding) { book, _ in try book.rememberRun(run) }
            return .success(())
        } catch {
            if !Task.isCancelled { return .failure(returnChecklistFailure(error)) }
            return .success(())
        }
    }

    private func changeReturnChecklist(_ binding: EnterpriseReturnChecklistBinding,
                                       _ change: (inout EnterpriseReturnChecklistBook, EnterpriseTripRecord) throws -> Void) throws {
        _ = try writableCompany()
        guard returnChecklistBinding(tripId: binding.tripId) == binding, var held = snapshot, let trip = held.openTrip else {
            throw EnterpriseReturnChecklistError.binding
        }
        var book = held.returnChecklists ?? EnterpriseReturnChecklistBook(membershipId: binding.membershipId, enrollmentId: binding.enrollmentId)
        guard book.matches(membershipId: binding.membershipId, enrollmentId: binding.enrollmentId) else {
            throw EnterpriseReturnChecklistError.binding
        }
        try change(&book, trip)
        held.returnChecklists = book
        guard store.save(held) else { throw EnterpriseReturnChecklistError.unavailable }
        snapshot = held
    }

    private func returnChecklistFailure(_ error: Error) -> AppValidationError {
        if let validation = error as? AppValidationError { return validation }
        if let issue = error as? EnterpriseReturnChecklistError {
            switch issue {
            case .binding, .unavailable:
                return AppValidationError(String(localized: "android.enterprise_checklists_save_failed"))
            case .notRequired:
                return AppValidationError(String(localized: "android.enterprise_checklists_none_required"))
            case .expired:
                return AppValidationError(String(localized: "android.enterprise_checklists_expired"))
            case .beforeStart:
                return AppValidationError(String(localized: "android.enterprise_checklists_refusal_before_start"))
            case .acknowledgementRequired:
                return AppValidationError(String(localized: "android.enterprise_checklists_refusal_ack"))
            case .invalid:
                return AppValidationError(String(localized: "android.enterprise_checklists_refusal_invalid"))
            }
        }
        if let issue = error as? EnterpriseChecklistFailure {
            switch issue {
            case .photoRequired:
                return AppValidationError(String(localized: "enterprise.checklist.photo_contact_fleet"))
            case .answerNotAllowed:
                return AppValidationError(String(localized: "android.enterprise_checklists_refusal_not_allowed"))
            case .incomplete:
                return AppValidationError(String(localized: "android.enterprise_checklists_refusal_incomplete"))
            case .invalid:
                return AppValidationError(String(localized: "android.enterprise_checklists_refusal_invalid"))
            }
        }
        if error is EnterpriseAccessError { return AppValidationError(Self.accessMessage(Self.accessError(error))) }
        return Self.validation(error)
    }

    private func enqueue(_ item: EnterpriseQueueItem, partition: EnterprisePartition,
                         update: (inout EnterprisePartitionSnapshot) -> Void) throws {
        guard var current = snapshot, current.partition == partition else {
            throw AppValidationError(String(localized: "enterprise.context.choose_before_recording"))
        }
        do {
            try EnterpriseQueue.enqueue(item, into: &current)
        } catch EnterpriseQueueError.full {
            throw AppValidationError(String(localized: "enterprise.queue.full"))
        } catch EnterpriseQueueError.locked {
            throw AppValidationError(String(localized: "enterprise.custody.locked_new"))
        }
        update(&current)
        guard store.save(current) else { throw AppValidationError(Self.accessMessage(.unavailable)) }
        snapshot = current
        deliverNow()
    }

    public func retryNow(_ id: String) {
        guard let current = active else { return }
        mutate(current.partition) { EnterpriseQueue.retryNow(id: id, in: &$0) }
        deliverNow()
    }

    public func dismiss(_ id: String) {
        guard let current = active else { return }
        mutate(current.partition) { EnterpriseQueue.dismiss(id: id, in: &$0) }
    }

    /// Applies a change to a partition: the selected one in memory, any other on disk only.
    private func mutate(_ partition: EnterprisePartition, _ change: (inout EnterprisePartitionSnapshot) -> Void) {
        let isSelected = snapshot?.partition == partition
        guard var current = isSelected ? snapshot : store.load(partition) else { return }
        change(&current)
        store.save(current)
        if isSelected {
            snapshot = current
        }
    }

    // MARK: - Delivery

    public func deliverNow() {
        guard deliveryTask == nil, let current = active, !isOfflineProbe(), accountId == current.partition.accountId else { return }
        let token = UUID()
        deliveryToken = token
        isDelivering = true
        wakeTask?.cancel()
        wakeTask = nil
        deliveryTask = Task { [weak self] in
            await self?.drain(current, token: token)
        }
    }

    private func stopDelivery() {
        deliveryTask?.cancel()
        deliveryTask = nil
        deliveryToken = nil
        wakeTask?.cancel()
        wakeTask = nil
        isDelivering = false
    }

    private func drain(_ current: EnterpriseActiveCompany, token: UUID) async {
        defer {
            if deliveryToken == token {
                deliveryToken = nil
                deliveryTask = nil
                isDelivering = false
                scheduleWake()
            }
        }
        let sync = syncClient
        var askContact = false
        for _ in 0..<10 {
            guard deliveryToken == token, !Task.isCancelled, !isOfflineProbe(),
                  let held = store.load(current.partition) else { return }
            let batch = EnterpriseQueue.dueBatch(held, nowMillis: Self.nowMillis())
            if batch.isEmpty { break }
            do {
                let acks = try await sync.deliver(partition: current.partition, operations: batch.map { $0.operation })
                guard deliveryToken == token, !Task.isCancelled else { return }
                mutate(current.partition) { EnterpriseQueue.apply(acks, to: &$0, nowMillis: Self.nowMillis()) }
            } catch {
                guard deliveryToken == token, !Task.isCancelled else { return }
                let failure = EnterpriseSyncError(errorValue: error)
                let ids = batch.map { $0.id }
                switch failure {
                case .offline:
                    mutate(current.partition) {
                        EnterpriseQueue.applyFailure(ids: ids, code: "OFFLINE", countsAttempt: false, to: &$0,
                                                     nowMillis: Self.nowMillis(), retryAfterMillis: nil)
                    }
                    return
                case .signInRequired:
                    notice = String(localized: "enterprise.queue.sign_in")
                    return
                case .refused(let status, let code, let retryAfter):
                    mutate(current.partition) {
                        EnterpriseQueue.applyFailure(ids: ids, code: code ?? "HTTP_\(status)", countsAttempt: true, to: &$0,
                                                     nowMillis: Self.nowMillis(), retryAfterMillis: retryAfter.map { $0 * 1000 })
                    }
                    askContact = status == 403 || status == 404
                    if askContact || status == 401 || status == 429 || status >= 500 { break }
                    continue
                case .unavailable, .invalidResponse:
                    mutate(current.partition) {
                        EnterpriseQueue.applyFailure(ids: ids, code: "UNAVAILABLE", countsAttempt: true, to: &$0,
                                                     nowMillis: Self.nowMillis(), retryAfterMillis: nil)
                    }
                }
                break
            }
        }
        await deliverAttachments(current, token: token)
        if askContact {
            await recheckDevice(current, token: token)
        }
    }

    private func deliverAttachments(_ current: EnterpriseActiveCompany, token: UUID) async {
        let sync = syncClient
        for _ in 0..<5 {
            guard attachmentDeliveryIsCurrent(current, token: token),
                  let held = store.load(current.partition),
                  let item = EnterpriseQueue.dueAttachment(held, nowMillis: Self.nowMillis()) else { return }
            guard let content = store.loadReceipt(partition: current.partition, documentId: item.documentId),
                  EnterpriseCanonicalJson.sha256Hex(content) == item.sha256 else {
                finishAttachment(item, partition: current.partition, storage: nil, refused: "LOCAL_COPY_MISSING", failure: nil, retryAfter: nil)
                continue
            }
            var refusalSource = EnterpriseAttachmentWire.RefusalSource.signedSession
            do {
                let session = try await sync.openAttachment(
                    partition: current.partition, enrollment: current.enrollment, declarationId: item.declarationId,
                    documentId: item.documentId, linkId: item.linkId, sha256: item.sha256, bytes: item.bytes,
                    targetKind: item.targetKind, targetId: item.targetId
                )
                guard attachmentDeliveryIsCurrent(current, token: token) else { return }
                var storage = session.storage
                if storage == .uploading {
                    guard let capability = session.capability else {
                        finishAttachment(item, partition: current.partition, storage: nil, refused: nil, failure: "UPLOAD_WINDOW_CLOSED", retryAfter: nil)
                        continue
                    }
                    refusalSource = .capabilityUpload
                    storage = try await sync.upload(partition: current.partition, documentId: item.documentId,
                                                    capability: capability, content: content)
                }
                guard attachmentDeliveryIsCurrent(current, token: token) else { return }
                finishAttachment(item, partition: current.partition, storage: storage, refused: nil, failure: nil, retryAfter: nil)
            } catch {
                guard attachmentDeliveryIsCurrent(current, token: token) else { return }
                let failure = EnterpriseSyncError(errorValue: error)
                switch failure {
                case .offline, .signInRequired:
                    return
                case .refused(let status, let code, let retryAfter):
                    let isFinalRefusal = EnterpriseAttachmentWire.isFinalRefusal(status: status, code: code, source: refusalSource)
                    finishAttachment(item, partition: current.partition, storage: nil, refused: isFinalRefusal ? (code ?? "REFUSED") : nil,
                                     failure: code ?? "HTTP_\(status)", retryAfter: retryAfter.map { $0 * 1000 })
                case .unavailable, .invalidResponse:
                    finishAttachment(item, partition: current.partition, storage: nil, refused: nil, failure: "UNAVAILABLE", retryAfter: nil)
                }
            }
        }
    }

    private func attachmentDeliveryIsCurrent(_ current: EnterpriseActiveCompany, token: UUID) -> Bool {
        guard let active else { return false }
        return deliveryToken == token && !Task.isCancelled && !isOfflineProbe() && !active.locked &&
            accountId == current.partition.accountId && active.partition == current.partition &&
            active.enrollment.enrollmentId == current.enrollment.enrollmentId &&
            active.enrollment.membershipId == current.enrollment.membershipId
    }

    private func finishAttachment(_ item: EnterpriseAttachmentItem, partition: EnterprisePartition, storage: EnterpriseAttachmentStorage?,
                                  refused: String?, failure: String?, retryAfter: Int64?) {
        let isSelected = snapshot?.partition == partition
        guard var held = isSelected ? snapshot : store.load(partition) else { return }
        EnterpriseQueue.applyAttachment(id: item.id, storage: storage, refusedCode: refused, failureCode: failure, to: &held,
                                        nowMillis: Self.nowMillis(), retryAfterMillis: retryAfter)
        guard store.save(held) else { return }
        if isSelected { snapshot = held }
        if let status = held.attachments.first(where: { $0.id == item.id })?.status, status == .delivered || status == .refused {
            store.deleteReceipt(partition: partition, documentId: item.documentId)
        }
    }

    /// A whole-delivery 403/404 only asks the exact device contact once; only its verdict changes custody.
    private func recheckDevice(_ current: EnterpriseActiveCompany, token: UUID) async {
        guard deliveryToken == token else { return }
        do {
            let outcome = try await syncClient.confirmDevice(partition: current.partition, membershipId: current.summary.membershipId,
                                                             known: current.enrollment)
            guard deliveryToken == token, active?.partition == current.partition else { return }
            switch outcome.verdict {
            case .active:
                break
            case .lock:
                cancelMissionRefresh()
                mutate(current.partition) { $0.locked = true }
                active = EnterpriseActiveCompany(partition: current.partition, summary: current.summary, employeeId: current.employeeId,
                                                 enrollment: outcome.enrollment, confirmedAtMillis: Self.nowMillis(), locked: true)
                notice = String(localized: "enterprise.custody.locked_retained")
            case .erase:
                cancelMissionRefresh()
                stopDelivery()
                store.erase(current.partition)
                active = nil
                snapshot = nil
                notice = String(localized: "enterprise.custody.erased")
            }
        } catch {
            // Generic refusals, outages and malformed answers never become an erase or lock.
        }
    }

    private func scheduleWake() {
        wakeTask?.cancel()
        wakeTask = nil
        guard active != nil, let held = snapshot, let wake = EnterpriseQueue.nextWakeMillis(held) else { return }
        let delay = min(max(wake - Self.nowMillis(), 1_000), 300_000)
        wakeTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(delay) * 1_000_000)
            if Task.isCancelled { return }
            self?.deliverNow()
        }
    }

    // MARK: - Invitations

    public func previewInvitation(code: String) {
        guard !invitationBusy, invitationDecision == nil else { return }
        restartInvitation()
        let normalized = EnterpriseCompanyWire.normalizeInvitationCode(code)
        guard !normalized.isEmpty, normalized.count <= 160 else {
            invitationError = .invalidCode
            return
        }
        guard accountId != nil else {
            invitationError = .signInRequired
            return
        }
        guard isOriginSupported, !isOfflineProbe() else {
            invitationError = isOriginSupported ? .offline : .unavailable
            return
        }
        let client = companyClient()
        let token = generation
        invitationBusy = true
        Task { [weak self] in
            let result: Result<EnterpriseInvitationPreview, Error>
            do {
                result = .success(try await client.previewInvitation(code: normalized))
            } catch {
                result = .failure(error)
            }
            guard let self, self.generation == token else { return }
            self.invitationBusy = false
            switch result {
            case .success(let preview):
                self.invitationCode = normalized
                self.invitationPreview = preview
            case .failure(let error):
                self.invitationError = EnterpriseCoordinator.accessError(error)
            }
        }
    }

    /// One operation id per decision; a retry repeats the same decision with the same id.
    public func decideInvitation(accept: Bool) {
        guard !invitationBusy, let code = invitationCode, let preview = invitationPreview, invitationResult == nil else { return }
        if let pending = invitationDecision, pending.accept != accept { return }
        guard !isOfflineProbe() else {
            invitationError = .offline
            return
        }
        let operationId = invitationDecision?.operationId ?? EnterpriseValues.newId()
        invitationDecision = (accept, operationId)
        invitationRetryAccept = accept
        invitationError = nil
        invitationBusy = true
        let client = companyClient()
        let token = generation
        Task { [weak self] in
            let result: Result<EnterpriseInvitationAcknowledgement, Error>
            do {
                if accept {
                    result = .success(try await client.acceptInvitation(code: code, consentDigest: preview.consentDigest,
                                                                        operationId: operationId))
                } else {
                    result = .success(try await client.declineInvitation(code: code, operationId: operationId))
                }
            } catch {
                result = .failure(error)
            }
            guard let self, self.generation == token else { return }
            self.invitationBusy = false
            switch result {
            case .success(let ack):
                self.invitationResult = ack
                self.invitationDecision = nil
                self.invitationRetryAccept = nil
                self.contextsLoaded = false
            case .failure(let error):
                let failure = EnterpriseCoordinator.accessError(error)
                self.invitationError = failure
                if failure == .invalidCode || failure == .alreadyMember || failure == .conflict {
                    self.invitationDecision = nil
                    self.invitationRetryAccept = nil
                }
            }
        }
    }

    public func restartInvitation() {
        invitationCode = nil
        invitationPreview = nil
        invitationResult = nil
        invitationError = nil
        invitationDecision = nil
        invitationRetryAccept = nil
        invitationBusy = false
    }

    // MARK: - Labels

    static func accessError(_ error: Error) -> EnterpriseAccessError {
        if let access = error as? EnterpriseAccessError { return access }
        if let sync = error as? EnterpriseSyncError { return sync.accessError }
        return .unavailable
    }

    static func validation(_ error: Error) -> AppValidationError {
        if let validation = error as? AppValidationError { return validation }
        if let operation = error as? EnterpriseOperationError {
            switch operation {
            case .malformed: return AppValidationError(String(localized: "android.enterprise_trips_capture_invalid"))
            case .unsupported: return AppValidationError(String(localized: "enterprise.queue.unsupported"))
            }
        }
        return AppValidationError(String(localized: "enterprise.queue.save_failed"))
    }

    public static func accessMessage(_ error: EnterpriseAccessError) -> String {
        switch error {
        case .signInRequired: return String(localized: "enterprise.access.sign_in")
        case .freshAuthRequired: return String(localized: "android.enterprise_invitation_fresh_required")
        case .invalidCode: return String(localized: "android.enterprise_invitation_invalid")
        case .unavailable: return String(localized: "android.enterprise_invitation_unavailable")
        case .conflict: return String(localized: "enterprise.access.conflict")
        case .alreadyMember: return String(localized: "android.enterprise_invitation_already_member")
        case .managedRestricted: return String(localized: "enterprise.access.managed")
        case .rateLimited: return String(localized: "android.enterprise_invitation_limited")
        case .stale: return String(localized: "enterprise.access.stale")
        case .accessDenied: return String(localized: "android.enterprise_context_denied")
        case .offline: return String(localized: "enterprise.access.offline")
        case .entitlementRequired: return String(localized: "enterprise.access.plan_required")
        case .membershipSuspended: return String(localized: "enterprise.access.membership_ended")
        case .organizationSuspended: return String(localized: "enterprise.access.company_suspended")
        case .deviceRevoked: return String(localized: "android.enterprise_sync_reason_device")
        }
    }

    static func syncMessage(_ error: Error) -> String {
        accessMessage(accessError(error))
    }

    public static func statusLabel(_ item: EnterpriseQueueItem) -> String {
        switch item.status {
        case .pending:
            if item.attempts == 0 { return String(localized: "android.enterprise_sync_saved") }
            return String(localized: "enterprise.queue.waiting_retry", defaultValue: "Waiting to retry (attempts: \(item.attempts.formatted())) · \(codeLabel(item.lastCode))")
        case .accepted:
            return String(localized: "enterprise.queue.accepted")
        case .inReview:
            return String(localized: "enterprise.queue.review", defaultValue: "Received · under company review (\(codeLabel(item.lastCode)))")
        case .rejected:
            return String(localized: "enterprise.queue.rejected", defaultValue: "Rejected by the server · \(codeLabel(item.lastCode))")
        case .exhausted:
            return String(localized: "enterprise.queue.paused", defaultValue: "Sending paused. Attempts: \(item.attempts.formatted()) · \(codeLabel(item.lastCode))")
        }
    }

    public static func attachmentLabel(_ item: EnterpriseAttachmentItem) -> String {
        switch item.status {
        case .waitingOwner: return String(localized: "enterprise.receipt.waiting_for_expense")
        case .pending: return item.attempts == 0 ? String(localized: "enterprise.receipt.waiting") : String(localized: "enterprise.receipt.retrying", defaultValue: "Photo waiting to retry (attempts: \(item.attempts.formatted()))")
        case .delivered: return String(localized: "enterprise.receipt.delivered")
        case .refused: return String(localized: "enterprise.receipt.rejected", defaultValue: "Photo rejected · \(codeLabel(item.lastCode))")
        case .exhausted: return String(localized: "enterprise.receipt.paused", defaultValue: "Photo upload paused. Attempts: \(item.attempts.formatted())")
        }
    }

    public static func codeLabel(_ code: String?) -> String {
        switch code ?? "" {
        case "OFFLINE": return String(localized: "enterprise.code.offline")
        case "UNAVAILABLE": return String(localized: "enterprise.code.unavailable")
        case "DEPENDENCY_PENDING": return String(localized: "enterprise.code.dependency_pending")
        case "MALFORMED": return String(localized: "android.enterprise_sync_reason_malformed")
        case "FORBIDDEN": return String(localized: "android.enterprise_assets_forbidden_title")
        case "DEVICE_REVOKED": return String(localized: "android.enterprise_sync_reason_device")
        case "MEMBERSHIP_ENDED": return String(localized: "android.enterprise_sync_reason_membership")
        case "INTAKE_WINDOW_CLOSED": return String(localized: "android.enterprise_sync_reason_window")
        case "ORG_SUSPENDED": return String(localized: "android.enterprise_sync_reason_suspended")
        case "OPERATION_ID_REUSED": return String(localized: "android.enterprise_sync_reason_reused")
        case "ENTITLEMENT_REQUIRED": return String(localized: "enterprise.code.entitlement_required")
        case "ASSET_UNAVAILABLE": return String(localized: "android.enterprise_sync_reason_asset_unavailable")
        case "ASSET_BUSY": return String(localized: "android.enterprise_sync_reason_asset_busy")
        case "TRIP_ALREADY_CLOSED": return String(localized: "android.enterprise_sync_reason_trip_closed")
        case "PERMISSION_CHANGED": return String(localized: "android.enterprise_sync_reason_permission_changed")
        case "PERIOD_LOCKED": return String(localized: "android.enterprise_sync_reason_period_locked")
        case "STALE_REVISION": return String(localized: "android.enterprise_sync_reason_revision")
        case "ORPHAN_REFERENCE": return String(localized: "android.enterprise_sync_reason_orphan")
        case "OWNER_REJECTED": return String(localized: "android.enterprise_receipts_refusal_owner")
        case "LOCAL_COPY_MISSING": return String(localized: "android.enterprise_sync_file_missing")
        case "UPLOAD_WINDOW_CLOSED": return String(localized: "android.enterprise_sync_reason_window")
        case "": return String(localized: "enterprise.code.none")
        default: return code ?? ""
        }
    }

    public static func categoryLabel(_ category: String) -> String {
        switch category {
        case "FINE": return String(localized: "android.enterprise_expenses_category_fine")
        case "INSURANCE": return String(localized: "android.enterprise_expenses_category_insurance")
        case "LEASE": return String(localized: "android.enterprise_expenses_category_lease")
        case "MAINTENANCE": return String(localized: "android.enterprise_expenses_category_maintenance")
        case "OTHER": return String(localized: "android.enterprise_expenses_category_other")
        case "PARKING": return String(localized: "android.enterprise_expenses_category_parking")
        case "TOLL": return String(localized: "android.enterprise_expenses_category_toll")
        default: return category
        }
    }

    public static func roleLabel(_ role: String) -> String {
        switch role {
        case "OWNER": return String(localized: "android.enterprise_role_owner")
        case "ADMIN": return String(localized: "android.enterprise_role_admin")
        case "FLEET_MANAGER": return String(localized: "android.enterprise_role_fleet_manager")
        case "SUPERVISOR": return String(localized: "android.enterprise_role_supervisor")
        case "FINANCE": return String(localized: "android.enterprise_role_finance")
        case "APPROVER": return String(localized: "android.enterprise_role_approver")
        case "DRIVER": return String(localized: "android.enterprise_role_driver")
        case "OPERATIONS_STAFF": return String(localized: "android.enterprise_role_operations_staff")
        case "AUDITOR": return String(localized: "android.enterprise_role_auditor")
        case "DATA_MANAGER": return String(localized: "android.enterprise_role_data_manager")
        default: return String(localized: "android.enterprise_role_unknown")
        }
    }

    public static func assetLabel(_ asset: EnterpriseReferenceAsset) -> String {
        var parts = [asset.name]
        if !asset.fleetNumber.isEmpty { parts.append(asset.fleetNumber) }
        if !asset.plate.isEmpty { parts.append(asset.plate) }
        return parts.joined(separator: " · ")
    }
}

extension EnterpriseSyncError {
    /// Any thrown value as a delivery failure class.
    init(errorValue: Error) {
        if let known = errorValue as? EnterpriseSyncError {
            self = known
        } else if errorValue is EnterpriseAccessError {
            self = .unavailable
        } else {
            self = .invalidResponse
        }
    }
}
