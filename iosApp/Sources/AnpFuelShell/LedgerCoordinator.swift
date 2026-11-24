import Foundation
import Combine
import SwiftUI
import AnpFuelCore

/// Vehicle ledger, monthly indicators, vehicle admission and private cloud sync for the SwiftUI shell.
///
/// Records live in `snapshot.ledgerEntries`; queue, acknowledgements, conflicts,
/// cursors and the capability lease live in `LedgerSyncStore`, one state per
/// account and origin. Network calls run on the signed transport actor and
/// URLSession; only published state changes on the main actor.
@MainActor
public final class LedgerCoordinator: ObservableObject {
    @Published public private(set) var syncState: LedgerSyncState?
    @Published public private(set) var isSyncing = false
    @Published public private(set) var isPulling = false
    @Published public private(set) var isWorking = false
    @Published public private(set) var notice: String?
    @Published public private(set) var lastPullReport: LedgerPullReport?

    private weak var host: AppStateViewModel?
    private let sessions: SignedSessionSource
    private let store: LedgerSyncStore
    private let efficiencyStore = LedgerEfficiencyPreferenceStore()
    private var dispatchTask: Task<Void, Never>?
    private var dispatchToken: UUID?
    private var wakeTask: Task<Void, Never>?
    private var migrated = false

    init(host: AppStateViewModel, sessions: SignedSessionSource, store: LedgerSyncStore = LedgerSyncStore()) {
        self.host = host
        self.sessions = sessions
        self.store = store
    }

    static func nowMillis() -> Int64 {
        return Int64(Date().timeIntervalSince1970 * 1000)
    }

    public var currentScope: LedgerSyncScope? {
        guard let host else { return nil }
        return LedgerSyncScope(accountId: host.currentAccountIdOrNil, origin: host.snapshot.apiEnvironment.origin)
    }

    private var records: [LedgerEntryRecord] {
        host?.snapshot.ledgerEntries ?? []
    }

    // MARK: - State

    /// Loads the state of the current account and origin; migrates first-ledger refuels once.
    public func refresh() {
        guard let host else { return }
        if !migrated {
            migrated = true
            let resolved = LedgerLegacyMigration.resolveFuels(host.snapshot.ledgerEntries, vehicles: host.snapshot.vehicles)
            if resolved != host.snapshot.ledgerEntries {
                host.snapshot.ledgerEntries = resolved
                host.persist()
            }
        }
        if let scope = currentScope {
            syncState = store.load(scope: scope)
        } else {
            syncState = nil
        }
    }

    private func save(_ state: LedgerSyncState) {
        store.save(state)
        if currentScope == state.scope {
            syncState = state
        }
    }

    private func mutate(_ change: (LedgerSyncState) -> LedgerSyncState) {
        guard let scope = currentScope else { return }
        save(change(store.load(scope: scope)))
    }

    public func status(of record: LedgerEntryRecord) -> LedgerRecordSyncStatus {
        return LedgerSyncEngine.status(of: record, in: syncState)
    }

    public func dismissNotice() {
        notice = nil
    }

    // MARK: - Entries

    public func save(form: LedgerEntryForm, editing existing: LedgerEntryRecord?) -> Result<LedgerEntryRecord, AppValidationError> {
        guard let host else { return .failure(AppValidationError(String(localized: "ledger.unavailable"))) }
        let id = existing?.id ?? LedgerIds.newId()
        switch form.build(id: id, vehicles: host.snapshot.vehicles, timezone: TimeZone.current.identifier, existing: existing) {
        case .failure(let error):
            return .failure(AppValidationError(error.localizedMessage))
        case .success(let record):
            var entries = host.snapshot.ledgerEntries
            if let index = entries.firstIndex(where: { $0.id == record.id }) {
                entries[index] = record
            } else {
                entries.insert(record, at: 0)
            }
            host.snapshot.ledgerEntries = entries
            host.persist()
            let now = Self.nowMillis()
            mutate { LedgerSyncEngine.reconcile($0, records: entries, nowMillis: now) }
            dispatchPending()
            return .success(record)
        }
    }

    /// Deleting is always free: the local copy goes now and a tombstone keeps pulls from restoring it.
    public func delete(_ record: LedgerEntryRecord) {
        guard let host else { return }
        host.snapshot.ledgerEntries.removeAll { $0.id == record.id && $0.kind == record.kind }
        host.persist()
        if let dataset = record.dataset {
            let now = Self.nowMillis()
            mutate { LedgerSyncEngine.markDeleted($0, dataset: dataset, recordId: record.id, nowMillis: now) }
            dispatchPending()
        }
    }

    public func exportCsv() -> String {
        return LedgerCsvExport.build(records)
    }

    public func indicators(vehicle: VehicleRecord, year: Int, month: Int) -> Result<LedgerMonthlyIndicators, LedgerIndicatorError> {
        do {
            let result = try LedgerIndicators.monthly(
                vehicle: vehicle,
                records: records,
                year: year,
                month: month,
                timezone: TimeZone.current.identifier
            )
            return .success(result)
        } catch let error as LedgerIndicatorError {
            return .failure(error)
        } catch {
            return .failure(.inputOutOfBounds)
        }
    }

    // MARK: - Local estimated efficiency

    var estimateScope: LedgerEstimateScope? {
        guard let host, !host.isAuthWorking else { return nil }
        return LedgerEstimateScope(accountId: host.currentAccountIdOrNil, origin: host.snapshot.apiEnvironment.origin)
    }

    func estimatedEfficiency(for vehicleId: String, in scope: LedgerEstimateScope) -> LedgerEstimatedEfficiency? {
        guard scope == estimateScope, host?.snapshot.vehicles.contains(where: { $0.id == vehicleId }) == true else { return nil }
        return efficiencyStore.estimate(for: vehicleId, in: scope)
    }

    func setEstimatedEfficiency(
        _ estimate: LedgerEstimatedEfficiency?, for vehicleId: String, in scope: LedgerEstimateScope
    ) throws {
        guard scope == estimateScope, host?.snapshot.vehicles.contains(where: { $0.id == vehicleId }) == true else {
            throw LedgerEfficiencyPreferenceStore.Failure.invalidVehicle
        }
        try efficiencyStore.set(estimate, for: vehicleId, in: scope)
        objectWillChange.send()
    }

    func removeEstimatedEfficiency(for vehicleIds: Set<String>) throws {
        guard let scope = estimateScope else { throw LedgerEfficiencyPreferenceStore.Failure.unavailable }
        try efficiencyStore.removeVehicles(vehicleIds, in: scope)
        objectWillChange.send()
    }

    func eraseEstimatedEfficiency(in scope: LedgerEstimateScope) throws {
        try efficiencyStore.erase(scope)
        objectWillChange.send()
    }

    // MARK: - Vehicle admission

    public func admissionForNewVehicle() -> VehicleAdmissionOutcome {
        guard let host else { return .premiumRequired }
        let scope = currentScope
        let state: LedgerSyncState?
        if let scope {
            state = syncState?.scope == scope ? syncState : store.load(scope: scope)
        } else {
            state = nil
        }
        return VehicleAdmission.outcomeForNewVehicle(
            registered: host.snapshot.vehicles.count,
            accountId: scope?.accountId,
            origin: host.snapshot.apiEnvironment.origin,
            observation: state?.premiumObservation,
            lease: state?.vehicleLease,
            nowMillis: Self.nowMillis()
        )
    }

    public static func admissionMessage(_ outcome: VehicleAdmissionOutcome) -> String {
        switch outcome {
        case .accept:
            return String(localized: "ledger.admission_allowed")
        case .premiumRequired:
            return String(localized: "android.vehicle_premium_required_message")
        case .capacityExceeded:
            return String(localized: "android.vehicle_max_reached")
        case .hostedAuthorityRequired:
            return String(localized: "ledger.verify_subscription")
        }
    }

    /// Fresh server proof for admission and capability resume: reads the Premium status and records when it was seen.
    public func refreshPremiumProof() async {
        guard let host, !host.isOfflineSimulation, let scope = currentScope else { return }
        let premium = host.premiumStore
        await premium.refresh()
        guard currentScope == scope, premium.phase == .idle, let status = premium.status else { return }
        let observation = PremiumCapabilityObservation(
            accountId: scope.accountId,
            origin: scope.origin,
            observedAtMillis: Self.nowMillis(),
            active: status.isPremiumActive,
            capabilities: status.capabilities,
            validUntilMillis: status.validUntil.flatMap { LedgerTime.parse($0) }
        )
        mutate { state in
            var next = state
            next.premiumObservation = observation
            next.vehicleLease = observation.vehicleLease()
            if observation.grants("professional.write") && next.paidUploadsPaused {
                next.paidUploadsPaused = false
                next.pauseCode = nil
            }
            return next
        }
        dispatchPending()
    }

    // MARK: - Sync

    private func makeClient(for scope: LedgerSyncScope) -> LedgerSyncClient? {
        guard let host else { return nil }
        let transport = host.identity.signedTransport
        guard transport.origin == scope.origin else { return nil }
        return LedgerSyncClient(transport: transport, sessions: sessions)
    }

    func analysisClient(for owner: AccountDataExportScope) -> (String, LedgerSyncClient)? {
        guard let host, host.accountDataExportScope == owner,
              let scope = currentScope, scope.origin == owner.origin,
              let client = makeClient(for: scope) else { return nil }
        return (scope.accountId, client)
    }

    private struct ReadyClient {
        let scope: LedgerSyncScope
        let client: LedgerSyncClient
    }

    private func readyClient() -> ReadyClient? {
        guard let host else { return nil }
        guard let scope = currentScope else {
            notice = String(localized: "ledger.sync.sign_in")
            return nil
        }
        guard !host.isOfflineSimulation else {
            notice = String(localized: "ledger.sync.offline")
            return nil
        }
        guard let client = makeClient(for: scope) else {
            notice = String(localized: "ledger.sync.invalid_origin")
            return nil
        }
        return ReadyClient(scope: scope, client: client)
    }

    /// Explicit opt-in or opt-out. The server computes the consent mask; disabling stops this device at once.
    public func setSyncEnabled(_ enabled: Bool) {
        if !enabled {
            mutate { LedgerSyncEngine.disableLocally($0) }
            stopDispatch()
        }
        guard let ready = readyClient() else { return }
        let scope = ready.scope
        let client = ready.client
        isWorking = true
        notice = nil
        Task { [weak self] in
            do {
                let status = try await client.setConsent(
                    accountId: scope.accountId,
                    datasets: LedgerDataset.allCases,
                    enabled: enabled
                )
                guard let self else { return }
                self.isWorking = false
                guard self.currentScope == scope else { return }
                let now = Self.nowMillis()
                let entries = self.records
                self.mutate { state in
                    LedgerSyncEngine.reconcile(
                        LedgerSyncEngine.applyConsent(state, status: status, enabled: enabled),
                        records: entries,
                        nowMillis: now
                    )
                }
                self.notice = enabled
                    ? String(localized: "ledger.sync.enabled_notice")
                    : String(localized: "ledger.sync.disabled_notice")
                if enabled { self.dispatchPending() }
            } catch {
                guard let self else { return }
                self.isWorking = false
                self.notice = enabled
                    ? Self.describe(error)
                    : String(localized: "ledger.sync.revoke_pending")
            }
        }
    }

    public func syncNow() {
        mutate { LedgerSyncEngine.retryNow($0) }
        dispatchPending()
    }

    public func dispatchPending() {
        guard dispatchTask == nil, let host, !host.isOfflineSimulation, let scope = currentScope,
              let client = makeClient(for: scope) else {
            return
        }
        guard LedgerSyncEngine.nextDispatchable(store.load(scope: scope), nowMillis: Self.nowMillis()) != nil else {
            scheduleWake()
            return
        }
        let token = UUID()
        dispatchToken = token
        isSyncing = true
        dispatchTask = Task { [weak self] in
            await self?.drain(scope: scope, client: client, token: token)
        }
    }

    private func stopDispatch() {
        dispatchTask?.cancel()
        dispatchTask = nil
        dispatchToken = nil
        wakeTask?.cancel()
        wakeTask = nil
        isSyncing = false
    }

    private func drain(scope: LedgerSyncScope, client: LedgerSyncClient, token: UUID) async {
        defer {
            if dispatchToken == token {
                dispatchToken = nil
                dispatchTask = nil
                isSyncing = false
                scheduleWake()
            }
        }
        for _ in 0..<25 {
            if Task.isCancelled || dispatchToken != token || currentScope != scope { break }
            let state = store.load(scope: scope)
            guard let command = LedgerSyncEngine.nextDispatchable(state, nowMillis: Self.nowMillis()) else { break }
            let result = await LedgerSyncDispatcher.dispatch(command, scope: scope, gateway: client, nowMillis: Self.nowMillis())
            guard dispatchToken == token, currentScope == scope else { break }
            let fresh = store.load(scope: scope)
            save(LedgerSyncDispatcher.apply(result, to: fresh, records: records, nowMillis: Self.nowMillis()))
            if case .some(.signInRequired) = result.failure { break }
            if case .some(.retry) = result.failure { break }
        }
    }

    private func scheduleWake() {
        wakeTask?.cancel()
        wakeTask = nil
        guard let scope = currentScope, let wake = LedgerSyncEngine.nextWakeMillis(store.load(scope: scope)) else { return }
        let now = Self.nowMillis()
        let delayMillis = min(max(wake - now, 1_000), 300_000)
        wakeTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(delayMillis) * 1_000_000)
            if Task.isCancelled { return }
            self?.dispatchPending()
        }
    }

    /// Paged download of the account's records (free, also after a lapse). Each page is merged and its
    /// opaque cursor saved, so an interrupted pull continues within the server's cursor window.
    public func pull() {
        guard !isPulling, let ready = readyClient() else { return }
        isPulling = true
        notice = nil
        Task { [weak self] in
            await self?.runPull(scope: ready.scope, client: ready.client)
        }
    }

    private func runPull(scope: LedgerSyncScope, client: LedgerSyncClient) async {
        var report = LedgerPullReport()
        defer {
            isPulling = false
            lastPullReport = report
        }
        for dataset in LedgerDataset.allCases {
            var cursor = LedgerSyncEngine.resumableCursor(store.load(scope: scope), dataset: dataset, nowMillis: Self.nowMillis())
            var seen = Set<String>()
            var pages = 0
            while true {
                guard currentScope == scope, !Task.isCancelled else { return }
                let page: LedgerRecordsPage
                do {
                    page = try await client.listRecords(
                        accountId: scope.accountId,
                        dataset: dataset,
                        vehicleId: nil,
                        limit: LedgerSyncEngine.pageLimit,
                        cursor: cursor
                    )
                } catch {
                    if cursor != nil, let transport = error as? SignedTransportError,
                       case .refused(let status, _, _) = transport, status == 400 {
                        // An expired cursor restarts this dataset from its first page; merging is idempotent.
                        mutate { state in
                            var next = state
                            next.pullCursors[dataset.rawValue] = nil
                            return next
                        }
                        cursor = nil
                        seen.removeAll()
                        continue
                    }
                    notice = Self.describe(error)
                    return
                }
                guard currentScope == scope, let host else { return }
                let now = Self.nowMillis()
                let merged = LedgerSyncEngine.mergePage(
                    store.load(scope: scope),
                    records: host.snapshot.ledgerEntries,
                    page: page.items,
                    report: report,
                    nowMillis: now
                )
                var nextState = merged.0
                report = merged.2
                if merged.1 != host.snapshot.ledgerEntries {
                    host.snapshot.ledgerEntries = merged.1
                    host.persist()
                }
                if let next = page.nextCursor {
                    nextState.pullCursors[dataset.rawValue] = LedgerPullCursor(cursor: next, savedAtMillis: now)
                } else {
                    nextState.pullCursors[dataset.rawValue] = nil
                }
                save(nextState)
                pages += 1
                guard let next = page.nextCursor else { break }
                if !seen.insert(next).inserted {
                    notice = String(localized: "ledger.sync.repeated_page")
                    return
                }
                if pages >= 50 {
                    report.truncated = true
                    break
                }
                cursor = next
            }
        }
        let now = Self.nowMillis()
        let entries = records
        mutate { state in
            var next = state
            next.lastPullAtMillis = now
            return LedgerSyncEngine.reconcile(next, records: entries, nowMillis: now)
        }
        dispatchPending()
    }

    // MARK: - Conflicts

    public func resolveConflict(_ conflict: LedgerConflict, choice: LedgerConflictChoice) {
        guard let host, let scope = currentScope else { return }
        let local = host.snapshot.ledgerEntries.first { $0.id == conflict.recordId && $0.dataset == conflict.dataset }
        let now = Self.nowMillis()
        switch LedgerSyncEngine.resolve(store.load(scope: scope), conflictKey: conflict.key, choice: choice, localRecord: local, nowMillis: now) {
        case .requeued(let next):
            save(LedgerSyncEngine.reconcile(next, records: host.snapshot.ledgerEntries, nowMillis: now))
            dispatchPending()
        case .adoptServer(let next, let record):
            var entries = host.snapshot.ledgerEntries
            if let index = entries.firstIndex(where: { $0.id == record.id && $0.dataset == record.dataset }) {
                entries[index] = record
            } else {
                entries.insert(record, at: 0)
            }
            host.snapshot.ledgerEntries = entries
            host.persist()
            save(LedgerSyncEngine.reconcile(next, records: entries, nowMillis: now))
        case .removeLocal(let next):
            host.snapshot.ledgerEntries.removeAll { $0.id == conflict.recordId && $0.dataset == conflict.dataset }
            host.persist()
            save(next)
        case .unavailable(let code):
            notice = Self.codeLabel(code)
        }
    }

    /// Reads the server's current copy again (for example after an offline conflict).
    public func refreshConflictCurrent(_ conflict: LedgerConflict) {
        guard let ready = readyClient() else { return }
        let scope = ready.scope
        let client = ready.client
        let vehicleId = LedgerWire.decodePayload(json: conflict.proposedPayloadJson, dataset: conflict.dataset)?.vehicleId
        isWorking = true
        Task { [weak self] in
            let current = await LedgerSyncDispatcher.lookupCurrent(
                dataset: conflict.dataset,
                recordId: conflict.recordId,
                vehicleId: vehicleId,
                scope: scope,
                gateway: client
            )
            guard let self else { return }
            self.isWorking = false
            guard self.currentScope == scope else { return }
            self.mutate { LedgerSyncEngine.updateConflictCurrent($0, conflictKey: conflict.key, current: current) }
            if current.state == .unknown {
                self.notice = String(localized: "ledger.sync.read_failed")
            }
        }
    }

    // MARK: - Free exit

    /// Erases the account's ledger in the cloud (free). Local records stay on this device and are not uploaded again.
    public func eraseCloudLedger() {
        guard let ready = readyClient() else { return }
        let scope = ready.scope
        let client = ready.client
        stopDispatch()
        isWorking = true
        notice = nil
        Task { [weak self] in
            do {
                try await client.eraseAll(accountId: scope.accountId)
                guard let self else { return }
                self.isWorking = false
                guard self.currentScope == scope else { return }
                let entries = self.records
                self.mutate { LedgerSyncEngine.applyErasure($0, records: entries) }
                self.notice = String(localized: "ledger.sync.erased_notice")
            } catch {
                guard let self else { return }
                self.isWorking = false
                self.notice = Self.describe(error)
            }
        }
    }

    /// Logout and account deletion: this owner's queue, acknowledgements, cursors and lease leave the device.
    public func purgeOwnedData(accountId: String?) {
        stopDispatch()
        if let accountId {
            store.purge(accountId: accountId)
        }
        syncState = nil
        lastPullReport = nil
        notice = nil
    }

    // MARK: - Labels

    public static func codeLabel(_ code: String?) -> String {
        switch code ?? "" {
        case "ledger.capability-required":
            return String(localized: "ledger.refusal.capability")
        case "ledger.invalid", "http-400":
            return String(localized: "ledger.refusal.invalid")
        case "ledger.auth-required", "http-401":
            return String(localized: "ledger.refusal.sign_in_again")
        case "ledger.not-found", "http-404":
            return String(localized: "ledger.refusal.missing")
        case "ledger.conflict":
            return String(localized: "ledger.refusal.conflict")
        case "ledger.offline":
            return String(localized: "ledger.refusal.offline")
        case "ledger.sign-in-required":
            return String(localized: "ledger.refusal.sign_in")
        case "ledger.current-unknown":
            return String(localized: "ledger.refusal.refresh_current")
        case "ledger.current-missing":
            return String(localized: "ledger.refusal.current_missing")
        case "ledger.current-undecodable":
            return String(localized: "ledger.refusal.current_undecodable")
        case "ledger.local-invalid":
            return String(localized: "ledger.refusal.local_invalid")
        case "":
            return String(localized: "ledger.refusal.no_detail")
        default:
            return code ?? ""
        }
    }

    public static func describe(_ error: Error) -> String {
        if let sync = error as? LedgerSyncError {
            switch sync {
            case .signInRequired: return String(localized: "ledger.sync.sign_in_continue")
            case .ownerMismatch: return String(localized: "ledger.sync.owner_changed")
            case .invalidCommand: return String(localized: "ledger.sync.invalid_command")
            case .invalidReceipt, .invalidPage: return String(localized: "ledger.sync.invalid_response")
            }
        }
        if let transport = error as? SignedTransportError, case .refused(_, let code, _) = transport, let code,
           code.hasPrefix("ledger.") {
            return String(localized: "ledger.sync.refused_server", defaultValue: "Server refused (\(codeLabel(code))).")
        }
        return IdentityCoordinator.describe(error)
    }

    public static func statusLabel(_ status: LedgerRecordSyncStatus) -> String {
        switch status {
        case .localOnly: return String(localized: "ledger.status.local_only")
        case .reviewRequired: return String(localized: "ledger.status.review_required")
        case .pending: return String(localized: "ledger.status.pending")
        case .synced(let revision): return String(localized: "ledger.status.synced", defaultValue: "Synced (revision \(revision.formatted()))")
        case .conflict: return String(localized: "ledger.status.conflict")
        case .refused(let code): return String(localized: "ledger.status.refused", defaultValue: "Refused · \(codeLabel(code))")
        case .paused: return String(localized: "ledger.status.paused")
        case .erasedFromCloud: return String(localized: "ledger.status.erased")
        }
    }

    public static func reasonLabel(_ reason: String) -> String {
        switch reason {
        case "no-records": return String(localized: "ledger.reason.no_records")
        case "odometer-missing": return String(localized: "ledger.reason.odometer_missing")
        case "odometer-unit-unverified": return String(localized: "ledger.reason.odometer_unverified")
        case "declared-fill-gap": return String(localized: "ledger.reason.declared_gap")
        case "odometer-segment-boundary": return String(localized: "ledger.reason.odometer_reset")
        case "distance-nonpositive": return String(localized: "ledger.reason.distance")
        case "full-anchors-required": return String(localized: "ledger.reason.full_tanks")
        case "unsupported-mixed-method": return String(localized: "ledger.reason.mixed_fuel")
        case "unsupported-gnv-method": return String(localized: "ledger.reason.cng")
        case "incompatible-unit": return String(localized: "ledger.reason.incompatible_unit")
        case "phev-shared-distance": return String(localized: "ledger.reason.phev_shared")
        case "energy-unmeasured": return String(localized: "ledger.reason.energy_unmeasured")
        case "partial-measurement": return String(localized: "ledger.reason.partial_energy")
        case "soc-inconsistent": return String(localized: "ledger.reason.soc")
        case "comparison-baseline-zero": return String(localized: "ledger.reason.baseline_zero")
        case "input-out-of-bounds": return String(localized: "ledger.reason.out_of_bounds")
        case "unmatched-period-window": return String(localized: "ledger.reason.period_mismatch")
        default:
            if reason.hasPrefix("baseline-incomplete: ") {
                return String(localized: "ledger.reason.baseline_incomplete", defaultValue: "previous month incomplete: \(reasonLabel(String(reason.dropFirst("baseline-incomplete: ".count))))")
            }
            if reason.hasPrefix("current-incomplete: ") {
                return String(localized: "ledger.reason.current_incomplete", defaultValue: "current month incomplete: \(reasonLabel(String(reason.dropFirst("current-incomplete: ".count))))")
            }
            return reason
        }
    }
}
