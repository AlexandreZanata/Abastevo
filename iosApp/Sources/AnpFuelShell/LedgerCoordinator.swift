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
        guard let host else { return .failure(AppValidationError(String(localized: "Diário indisponível."))) }
        let id = existing?.id ?? LedgerIds.newId()
        switch form.build(id: id, vehicles: host.snapshot.vehicles, timezone: TimeZone.current.identifier, existing: existing) {
        case .failure(let error):
            return .failure(AppValidationError(error.messagePtBr))
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
            return String(localized: "Cadastro permitido.")
        case .premiumRequired:
            return String(localized: "O plano gratuito inclui um veículo. Veículos adicionais exigem o Premium com a capacidade vehicles.multiple verificada pelo servidor.")
        case .capacityExceeded:
            return String(localized: "Limite técnico de três veículos atingido. Remova um veículo para cadastrar outro.")
        case .hostedAuthorityRequired:
            return String(localized: "O servidor precisa confirmar a assinatura antes deste cadastro.")
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

    private struct ReadyClient {
        let scope: LedgerSyncScope
        let client: LedgerSyncClient
    }

    private func readyClient() -> ReadyClient? {
        guard let host else { return nil }
        guard let scope = currentScope else {
            notice = String(localized: "Entre na conta gratuita para sincronizar o diário.")
            return nil
        }
        guard !host.isOfflineSimulation else {
            notice = String(localized: "Modo offline: os registros continuam neste aparelho.")
            return nil
        }
        guard let client = makeClient(for: scope) else {
            notice = String(localized: "Ambiente de rede inválido para a conta atual.")
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
                    ? String(localized: "Sincronização ativada. Os registros pendentes serão enviados.")
                    : String(localized: "Sincronização desativada. Os registros continuam neste aparelho.")
                if enabled { self.dispatchPending() }
            } catch {
                guard let self else { return }
                self.isWorking = false
                self.notice = enabled
                    ? Self.describe(error)
                    : String(localized: "Envios pausados neste aparelho; o consentimento no servidor não pôde ser revogado agora.")
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
                    notice = String(localized: "O servidor repetiu a página; tente novamente.")
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
                self.notice = String(localized: "Não foi possível ler a versão do servidor agora.")
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
                self.notice = String(localized: "Diário apagado da nuvem. Os registros continuam neste aparelho.")
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
            return String(localized: "Premium (professional.write) ou consentimento de envio necessário; os registros continuam neste aparelho")
        case "ledger.invalid", "http-400":
            return String(localized: "dados recusados pelo servidor: o veículo precisa existir na nuvem e o hodômetro não pode ser menor que um já enviado")
        case "ledger.auth-required", "http-401":
            return String(localized: "entre novamente na conta")
        case "ledger.not-found", "http-404":
            return String(localized: "registro indisponível no servidor")
        case "ledger.conflict":
            return String(localized: "conflito de versões")
        case "ledger.offline":
            return String(localized: "sem conexão")
        case "ledger.sign-in-required":
            return String(localized: "entre na conta gratuita")
        case "ledger.current-unknown":
            return String(localized: "Atualize a versão do servidor antes de decidir.")
        case "ledger.current-missing":
            return String(localized: "O servidor não tem mais este registro; use a versão do servidor para removê-lo aqui.")
        case "ledger.current-undecodable":
            return String(localized: "A versão do servidor não pôde ser lida neste aparelho.")
        case "ledger.local-invalid":
            return String(localized: "Revise o registro local antes de mantê-lo.")
        case "":
            return String(localized: "sem detalhe")
        default:
            return code ?? ""
        }
    }

    public static func describe(_ error: Error) -> String {
        if let sync = error as? LedgerSyncError {
            switch sync {
            case .signInRequired: return String(localized: "Entre na conta gratuita para continuar.")
            case .ownerMismatch: return String(localized: "A sessão mudou de conta; nada foi enviado.")
            case .invalidCommand: return String(localized: "O registro não pôde ser preparado para envio.")
            case .invalidReceipt, .invalidPage: return String(localized: "Resposta inválida do servidor.")
            }
        }
        if let transport = error as? SignedTransportError, case .refused(_, let code, _) = transport, let code,
           code.hasPrefix("ledger.") {
            return String(localized: "Servidor recusou (\(codeLabel(code))).")
        }
        return IdentityCoordinator.describe(error)
    }

    public static func statusLabel(_ status: LedgerRecordSyncStatus) -> String {
        switch status {
        case .localOnly: return String(localized: "Somente neste aparelho")
        case .reviewRequired: return String(localized: "Registro antigo: revise antes de sincronizar")
        case .pending: return String(localized: "Envio pendente")
        case .synced(let revision): return String(localized: "Sincronizado (revisão \(String(revision)))")
        case .conflict: return String(localized: "Conflito: escolha a versão")
        case .refused(let code): return String(localized: "Recusado · \(codeLabel(code))")
        case .paused: return String(localized: "Envio pausado: Premium necessário")
        case .erasedFromCloud: return String(localized: "Apagado da nuvem; mantido neste aparelho")
        }
    }

    public static func reasonLabel(_ reason: String) -> String {
        switch reason {
        case "no-records": return String(localized: "sem registros no mês")
        case "odometer-missing": return String(localized: "falta hodômetro em algum registro")
        case "odometer-unit-unverified": return String(localized: "há registros antigos com hodômetro a revisar")
        case "declared-fill-gap": return String(localized: "há abastecimento não registrado declarado")
        case "odometer-segment-boundary": return String(localized: "o hodômetro foi zerado ou trocado no período")
        case "distance-nonpositive": return String(localized: "distância insuficiente entre leituras")
        case "full-anchors-required": return String(localized: "são necessários dois abastecimentos de tanque cheio")
        case "unsupported-mixed-method": return String(localized: "combustíveis diferentes no mesmo intervalo")
        case "unsupported-gnv-method": return String(localized: "consumo de GNV ainda não tem método aprovado")
        case "incompatible-unit": return String(localized: "unidades incompatíveis")
        case "phev-shared-distance": return String(localized: "híbrido plug-in: distância compartilhada entre combustível e energia")
        case "energy-unmeasured": return String(localized: "energia das recargas não informada")
        case "partial-measurement": return String(localized: "nem todas as recargas têm energia informada")
        case "soc-inconsistent": return String(localized: "carga final menor que a inicial")
        case "comparison-baseline-zero": return String(localized: "mês anterior sem custo para comparar")
        case "input-out-of-bounds": return String(localized: "valores fora dos limites")
        case "unmatched-period-window": return String(localized: "janela de distância não corresponde ao período")
        default:
            if reason.hasPrefix("baseline-incomplete: ") {
                return String(localized: "mês anterior incompleto: \(reasonLabel(String(reason.dropFirst("baseline-incomplete: ".count))))")
            }
            if reason.hasPrefix("current-incomplete: ") {
                return String(localized: "mês atual incompleto: \(reasonLabel(String(reason.dropFirst("current-incomplete: ".count))))")
            }
            return reason
        }
    }
}
