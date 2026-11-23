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
        switch outcome.verdict {
        case .erase:
            store.erase(snapshot.partition)
            notice = String(localized: "O servidor informou que este aparelho foi perdido ou comprometido. Os dados desta empresa foram apagados deste aparelho.")
            return
        case .lock:
            snapshot.locked = true
            snapshot.enrollment = outcome.enrollment
            notice = String(localized: "Seu acesso a esta empresa terminou. Registros ainda não enviados continuam guardados e serão oferecidos ao servidor; nenhum registro novo é aceito.")
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
        guard let current = active else { throw AppValidationError(String(localized: "Escolha uma empresa antes de registrar.")) }
        if current.locked {
            throw AppValidationError(String(localized: "Seu acesso a esta empresa terminou. Nenhum registro novo é aceito."))
        }
        return current
    }

    /// `trip.start` for an assigned asset with a reading or a written reason.
    public func startTrip(assetId: String, evidence: EnterpriseMeterEvidence, departureRunId: String?) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard let employee = current.employeeId else {
                throw AppValidationError(String(localized: "O servidor não informou seu cadastro de motorista nesta empresa. Atualize o acesso."))
            }
            guard let asset = findAsset(assetId) else { throw AppValidationError(String(localized: "Escolha um ativo da sua lista.")) }
            if snapshot?.openTrip != nil {
                throw AppValidationError(String(localized: "Encerre a viagem em andamento antes de iniciar outra."))
            }
            let now = Self.nowMillis()
            let tripId = EnterpriseValues.newId()
            let operation = try EnterpriseOperationBuilder.startTrip(
                envelope: makeEnvelope(current, now: now), tripId: tripId, assetId: asset.id, driverEmployeeId: employee,
                evidence: evidence, departureRunId: departureRunId
            )
            let label = Self.assetLabel(asset)
            var meterId: String?
            if case .reading(let id, _, _, _, _, _) = evidence { meterId = id }
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .tripStart, summary: String(localized: "Início de viagem · \(label)"),
                                            createdAtMillis: now), partition: current.partition) { snapshot in
                snapshot.trips.append(EnterpriseTripRecord(id: tripId, assetId: asset.id, assetLabel: label,
                                                           startOperationId: operation.operationId, startedAtMillis: now,
                                                           startMeterId: meterId))
            }
            return .success(())
        } catch {
            return .failure(Self.validation(error))
        }
    }

    /// `trip.finish` of the open trip; it depends on the start until the server received it.
    public func finishTrip(evidence: EnterpriseMeterEvidence) -> Result<Void, AppValidationError> {
        do {
            let current = try writableCompany()
            guard let trip = snapshot?.openTrip else { throw AppValidationError(String(localized: "Nenhuma viagem em andamento.")) }
            let startItem = snapshot?.queue.first { $0.id == trip.startOperationId }
            if startItem?.status == .rejected {
                throw AppValidationError(String(localized: "O servidor recusou o início desta viagem. Remova-a da lista."))
            }
            let dependency: String? = startItem?.status.isReceived == true ? nil : trip.startOperationId
            let now = Self.nowMillis()
            let operation = try EnterpriseOperationBuilder.finishTrip(
                envelope: makeEnvelope(current, now: now), tripId: trip.id, evidence: evidence, startOperationId: dependency
            )
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .tripFinish,
                                            summary: String(localized: "Fim de viagem · \(trip.assetLabel)"), createdAtMillis: now),
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
            guard let asset = findAsset(assetId) else { throw AppValidationError(String(localized: "Escolha um ativo da sua lista.")) }
            guard let paid = try? PortableMoney.parseBrlCentavos(paidText) else {
                throw AppValidationError(String(localized: "Informe o valor pago, por exemplo 12,50."))
            }
            let trimmedCompany = companyText.trimmingCharacters(in: .whitespacesAndNewlines)
            let companyAmount: Int64
            if trimmedCompany.isEmpty {
                companyAmount = paid
            } else if let parsed = try? PortableMoney.parseBrlCentavos(trimmedCompany) {
                companyAmount = parsed
            } else {
                throw AppValidationError(String(localized: "Informe o valor da empresa, por exemplo 12,50."))
            }
            if companyAmount > paid {
                throw AppValidationError(String(localized: "O valor da empresa não pode ser maior que o valor pago."))
            }
            if let receipt, receipt.isEmpty || receipt.count > EnterpriseAttachmentWire.maxBytes {
                throw AppValidationError(String(localized: "A foto do comprovante precisa ter até 5 MB."))
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
                    throw AppValidationError(String(localized: "Não foi possível guardar a foto neste aparelho."))
                }
                attachment = EnterpriseAttachmentItem(
                    documentId: documentId, declarationId: EnterpriseValues.newId(), linkId: EnterpriseValues.newId(),
                    sha256: EnterpriseCanonicalJson.sha256Hex(receipt), bytes: Int64(receipt.count),
                    targetKind: "EXPENSE", targetId: expenseId, ownerOperationId: operation.operationId
                )
            }
            let label = "\(Self.categoryLabel(category)) · \(AbastevoTheme.formatCentavos(paid))"
            do {
                try enqueue(EnterpriseQueueItem(operation: operation, kind: .expense, summary: String(localized: "Despesa · \(label)"),
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

    /// Reads the applicable departure checklist of an asset on the company channel.
    public func loadChecklist(assetId: String, context: String) async -> Result<EnterpriseChecklistApplicable, AppValidationError> {
        guard let current = active, !isOfflineProbe() else {
            return .failure(AppValidationError(String(localized: "Conecte-se para consultar o checklist.")))
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
                throw AppValidationError(String(localized: "Não há checklist obrigatório para este ativo."))
            }
            let evaluation = try EnterpriseChecklistRules.evaluate(items: applicable.items, policyMode: mode, answers: answers)
            if evaluation.acknowledgementRequired && !acknowledged {
                throw AppValidationError(String(localized: "Confirme que está ciente dos itens críticos com problema."))
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
            try enqueue(EnterpriseQueueItem(operation: operation, kind: .checklist, summary: String(localized: "Checklist de saída · \(label)"),
                                            createdAtMillis: now), partition: current.partition) { _ in }
            return .success((runId, evaluation.outcome))
        } catch let failure as EnterpriseChecklistFailure {
            switch failure {
            case .photoRequired:
                return .failure(AppValidationError(String(localized: "Um item com problema exige foto, que ainda não pode ser enviada pelo iOS. Fale com o gestor da frota.")))
            case .answerNotAllowed:
                return .failure(AppValidationError(String(localized: "Somente itens opcionais podem ser marcados como não se aplica.")))
            case .incomplete:
                return .failure(AppValidationError(String(localized: "Responda todos os itens do checklist.")))
            case .invalid:
                return .failure(AppValidationError(String(localized: "Checklist inválido. Consulte novamente.")))
            }
        } catch {
            return .failure(Self.validation(error))
        }
    }

    private func enqueue(_ item: EnterpriseQueueItem, partition: EnterprisePartition,
                         update: (inout EnterprisePartitionSnapshot) -> Void) throws {
        guard var current = snapshot, current.partition == partition else {
            throw AppValidationError(String(localized: "Escolha uma empresa antes de registrar."))
        }
        do {
            try EnterpriseQueue.enqueue(item, into: &current)
        } catch EnterpriseQueueError.full {
            throw AppValidationError(String(localized: "Muitos registros aguardando envio. Conecte-se para enviá-los antes de criar outros."))
        } catch EnterpriseQueueError.locked {
            throw AppValidationError(String(localized: "Seu acesso a esta empresa terminou. Nenhum registro novo é aceito."))
        }
        update(&current)
        store.save(current)
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
                mutate(current.partition) { EnterpriseQueue.apply(acks, to: &$0, nowMillis: Self.nowMillis()) }
            } catch {
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
                    notice = String(localized: "Entre novamente na sua conta para enviar os registros da empresa.")
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
            guard deliveryToken == token, !Task.isCancelled, !isOfflineProbe(), !current.locked,
                  let held = store.load(current.partition),
                  let item = EnterpriseQueue.dueAttachment(held, nowMillis: Self.nowMillis()) else { return }
            guard let content = store.loadReceipt(partition: current.partition, documentId: item.documentId),
                  EnterpriseCanonicalJson.sha256Hex(content) == item.sha256 else {
                finishAttachment(item, partition: current.partition, storage: nil, refused: "LOCAL_COPY_MISSING", failure: nil, retryAfter: nil)
                continue
            }
            do {
                let session = try await sync.openAttachment(
                    partition: current.partition, enrollment: current.enrollment, declarationId: item.declarationId,
                    documentId: item.documentId, linkId: item.linkId, sha256: item.sha256, bytes: item.bytes,
                    targetKind: item.targetKind, targetId: item.targetId
                )
                var storage = session.storage
                if storage == .uploading {
                    guard let capability = session.capability else {
                        finishAttachment(item, partition: current.partition, storage: nil, refused: nil, failure: "UPLOAD_WINDOW_CLOSED", retryAfter: nil)
                        continue
                    }
                    storage = try await sync.upload(partition: current.partition, documentId: item.documentId,
                                                    capability: capability, content: content)
                }
                finishAttachment(item, partition: current.partition, storage: storage, refused: nil, failure: nil, retryAfter: nil)
            } catch {
                let failure = EnterpriseSyncError(errorValue: error)
                switch failure {
                case .offline, .signInRequired:
                    return
                case .refused(let status, let code, let retryAfter):
                    let isFinalRefusal = EnterpriseAttachmentWire.isFinalRefusal(status: status, code: code)
                    finishAttachment(item, partition: current.partition, storage: nil, refused: isFinalRefusal ? (code ?? "REFUSED") : nil,
                                     failure: code ?? "HTTP_\(status)", retryAfter: retryAfter.map { $0 * 1000 })
                case .unavailable, .invalidResponse:
                    finishAttachment(item, partition: current.partition, storage: nil, refused: nil, failure: "UNAVAILABLE", retryAfter: nil)
                }
            }
        }
    }

    private func finishAttachment(_ item: EnterpriseAttachmentItem, partition: EnterprisePartition, storage: EnterpriseAttachmentStorage?,
                                  refused: String?, failure: String?, retryAfter: Int64?) {
        mutate(partition) {
            EnterpriseQueue.applyAttachment(id: item.id, storage: storage, refusedCode: refused, failureCode: failure, to: &$0,
                                            nowMillis: Self.nowMillis(), retryAfterMillis: retryAfter)
        }
        let holder = snapshot?.partition == partition ? snapshot : store.load(partition)
        let updated = holder?.attachments.first(where: { $0.id == item.id })
        if let status = updated?.status, status == .delivered || status == .refused {
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
                mutate(current.partition) { $0.locked = true }
                active = EnterpriseActiveCompany(partition: current.partition, summary: current.summary, employeeId: current.employeeId,
                                                 enrollment: outcome.enrollment, confirmedAtMillis: Self.nowMillis(), locked: true)
                notice = String(localized: "Seu acesso a esta empresa terminou. Registros ainda não enviados continuam guardados; nenhum registro novo é aceito.")
            case .erase:
                stopDelivery()
                store.erase(current.partition)
                active = nil
                snapshot = nil
                notice = String(localized: "O servidor informou que este aparelho foi perdido ou comprometido. Os dados desta empresa foram apagados deste aparelho.")
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
            case .malformed: return AppValidationError(String(localized: "Confira os dados: valores, leitura ou motivo inválidos."))
            case .unsupported: return AppValidationError(String(localized: "Este tipo de registro ainda não é aceito pelo servidor."))
            }
        }
        return AppValidationError(String(localized: "Não foi possível salvar o registro neste aparelho."))
    }

    public static func accessMessage(_ error: EnterpriseAccessError) -> String {
        switch error {
        case .signInRequired: return String(localized: "Entre na sua conta pessoal do Abastevo para continuar.")
        case .freshAuthRequired: return String(localized: "Confirme seu acesso entrando novamente na sua conta.")
        case .invalidCode: return String(localized: "Código inválido ou expirado. Peça um novo ao responsável.")
        case .unavailable: return String(localized: "Não foi possível confirmar a operação. Confira sua conexão e tente novamente.")
        case .conflict: return String(localized: "O servidor recusou por conflito com o estado atual. Consulte novamente.")
        case .alreadyMember: return String(localized: "Sua conta já tem um vínculo com esta empresa. Consulte seu acesso.")
        case .managedRestricted: return String(localized: "Esta conta gerenciada não pode realizar esta ação.")
        case .rateLimited: return String(localized: "Muitas tentativas. Aguarde antes de tentar novamente.")
        case .stale: return String(localized: "Esta consulta deixou de ser válida. Consulte novamente.")
        case .accessDenied: return String(localized: "Este acesso não está disponível. Atualize suas empresas.")
        case .offline: return String(localized: "Sem conexão. A seleção de empresa e os convites exigem confirmação online.")
        case .entitlementRequired: return String(localized: "O plano desta empresa não inclui este recurso.")
        case .membershipSuspended: return String(localized: "Seu vínculo com esta empresa está suspenso ou encerrado.")
        case .organizationSuspended: return String(localized: "Esta empresa está suspensa ou arquivada.")
        case .deviceRevoked: return String(localized: "Este aparelho foi desvinculado da empresa.")
        }
    }

    static func syncMessage(_ error: Error) -> String {
        accessMessage(accessError(error))
    }

    public static func statusLabel(_ item: EnterpriseQueueItem) -> String {
        switch item.status {
        case .pending:
            if item.attempts == 0 { return String(localized: "Guardado neste aparelho · aguardando envio") }
            return String(localized: "Aguardando nova tentativa (\(item.attempts)) · \(codeLabel(item.lastCode))")
        case .accepted:
            return String(localized: "Recebido e aceito pelo servidor")
        case .inReview:
            return String(localized: "Recebido · em revisão pela empresa (\(codeLabel(item.lastCode)))")
        case .rejected:
            return String(localized: "Recusado pelo servidor · \(codeLabel(item.lastCode))")
        case .exhausted:
            return String(localized: "Envio pausado após \(item.attempts) tentativas · \(codeLabel(item.lastCode))")
        }
    }

    public static func attachmentLabel(_ item: EnterpriseAttachmentItem) -> String {
        switch item.status {
        case .waitingOwner: return String(localized: "Foto aguardando o recebimento da despesa")
        case .pending: return item.attempts == 0 ? String(localized: "Foto aguardando envio") : String(localized: "Foto aguardando nova tentativa (\(item.attempts))")
        case .delivered: return String(localized: "Foto recebida pelo servidor")
        case .refused: return String(localized: "Foto recusada · \(codeLabel(item.lastCode))")
        case .exhausted: return String(localized: "Envio da foto pausado após \(item.attempts) tentativas")
        }
    }

    public static func codeLabel(_ code: String?) -> String {
        switch code ?? "" {
        case "OFFLINE": return String(localized: "sem conexão")
        case "UNAVAILABLE": return String(localized: "serviço indisponível")
        case "DEPENDENCY_PENDING": return String(localized: "aguardando o registro anterior")
        case "MALFORMED": return String(localized: "dados inválidos")
        case "FORBIDDEN": return String(localized: "sem permissão")
        case "DEVICE_REVOKED": return String(localized: "aparelho desvinculado")
        case "MEMBERSHIP_ENDED": return String(localized: "vínculo encerrado")
        case "INTAKE_WINDOW_CLOSED": return String(localized: "prazo de envio encerrado")
        case "ORG_SUSPENDED": return String(localized: "empresa suspensa")
        case "OPERATION_ID_REUSED": return String(localized: "registro duplicado")
        case "ENTITLEMENT_REQUIRED": return String(localized: "recurso fora do plano")
        case "ASSET_UNAVAILABLE": return String(localized: "ativo indisponível")
        case "ASSET_BUSY": return String(localized: "ativo em uso")
        case "TRIP_ALREADY_CLOSED": return String(localized: "viagem já encerrada")
        case "PERMISSION_CHANGED": return String(localized: "permissão alterada")
        case "PERIOD_LOCKED": return String(localized: "período fechado")
        case "STALE_REVISION": return String(localized: "versão desatualizada")
        case "ORPHAN_REFERENCE": return String(localized: "referência não encontrada")
        case "OWNER_REJECTED": return String(localized: "a despesa foi recusada")
        case "LOCAL_COPY_MISSING": return String(localized: "cópia local indisponível")
        case "UPLOAD_WINDOW_CLOSED": return String(localized: "janela de envio encerrada")
        case "": return String(localized: "sem detalhe")
        default: return code ?? ""
        }
    }

    public static func categoryLabel(_ category: String) -> String {
        switch category {
        case "FINE": return String(localized: "Multa")
        case "INSURANCE": return String(localized: "Seguro")
        case "LEASE": return String(localized: "Locação")
        case "MAINTENANCE": return String(localized: "Manutenção")
        case "OTHER": return String(localized: "Outros")
        case "PARKING": return String(localized: "Estacionamento")
        case "TOLL": return String(localized: "Pedágio")
        default: return category
        }
    }

    public static func roleLabel(_ role: String) -> String {
        switch role {
        case "OWNER": return String(localized: "Proprietário")
        case "ADMIN": return String(localized: "Administrador")
        case "FLEET_MANAGER": return String(localized: "Gestor de frota")
        case "SUPERVISOR": return String(localized: "Supervisor")
        case "FINANCE": return String(localized: "Financeiro")
        case "APPROVER": return String(localized: "Aprovador")
        case "DRIVER": return String(localized: "Motorista")
        case "OPERATIONS_STAFF": return String(localized: "Equipe de operação")
        case "AUDITOR": return String(localized: "Auditor")
        case "DATA_MANAGER": return String(localized: "Gestor de dados")
        default: return String(localized: "Papel não identificado")
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
