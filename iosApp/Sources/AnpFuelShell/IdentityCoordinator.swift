import Foundation
import Combine
import SwiftUI
import AnpFuelCore
#if canImport(UIKit)
import UIKit
#endif

/// Device contributor identity and the signed community write queue for the SwiftUI shell.
///
/// Network work runs off the main actor: the transport is an actor, the
/// gateway is nonisolated, and account refreshes go through
/// `AuthBlockingWork`. Only published state changes on the main actor.
@MainActor
public final class IdentityCoordinator: ObservableObject {
    @Published public private(set) var outboxItems: [CommunityOutboxItem] = []
    @Published public private(set) var ownObservations: [OwnObservation] = []
    @Published public private(set) var ownHistoryState: PublicReadState = .unavailable
    @Published public private(set) var identityNotice: String?
    @Published public private(set) var isDispatching = false
    @Published public private(set) var contributorId: String?

    /// Set by the owning view model; an offline simulation pauses dispatch.
    public var isOfflineProbe: @MainActor () -> Bool = { false }

    private let signer: IosDeviceProofSigner
    private let identityStore: IosDeviceIdentityKeychainStore
    private let sessions: AuthFlowSessionSource
    private let exchange: UrlSessionSignedExchange
    private let outboxStore: CommunityOutboxStore
    private let photoCache: IosTransientPhotoCache
    private var origin: String
    private var transport: SignedRequestTransport
    private var client: CommunityWriteClient
    private var dispatchTask: Task<Void, Never>?
    private var dispatchToken: UUID?
    private var wakeTask: Task<Void, Never>?
    private var activationObserver: AnyCancellable?

    init(authFlow: AuthFlow, signer: IosDeviceProofSigner, photoCache: IosTransientPhotoCache, environment: ApiEnvironment) {
        let store = IosDeviceIdentityKeychainStore()
        let source = AuthFlowSessionSource(flow: authFlow)
        let exchange = UrlSessionSignedExchange()
        let pair = IdentityCoordinator.makeClient(
            origin: environment.origin,
            signer: signer,
            identityStore: store,
            exchange: exchange,
            sessions: source,
            photoCache: photoCache
        )
        self.signer = signer
        self.identityStore = store
        self.sessions = source
        self.exchange = exchange
        self.outboxStore = CommunityOutboxStore()
        self.photoCache = photoCache
        self.origin = environment.origin
        self.transport = pair.0
        self.client = pair.1
    }

    private static func makeClient(
        origin: String,
        signer: IosDeviceProofSigner,
        identityStore: IosDeviceIdentityKeychainStore,
        exchange: UrlSessionSignedExchange,
        sessions: AuthFlowSessionSource,
        photoCache: IosTransientPhotoCache
    ) -> (SignedRequestTransport, CommunityWriteClient) {
        let transport = SignedRequestTransport(
            origin: origin,
            signer: signer,
            identityStore: identityStore,
            exchange: exchange,
            sessions: sessions
        )
        let client = CommunityWriteClient(
            transport: transport,
            exchange: exchange,
            sessions: sessions,
            photoBytes: { id in photoCache.get(id: id) }
        )
        return (transport, client)
    }

    /// The signed transport of the current origin, for other owner-signed
    /// commands such as Premium purchase verification.
    public var signedTransport: SignedRequestTransport { transport }

    /// Loads the queue, observes app activation and resumes pending writes.
    public func start() {
        outboxItems = outboxStore.load()
        contributorId = transport.storedIdentity()?.contributorId
        #if canImport(UIKit)
        activationObserver = NotificationCenter.default
            .publisher(for: UIApplication.didBecomeActiveNotification)
            .receive(on: RunLoop.main)
            .sink { [weak self] _ in
                self?.dispatchPending()
            }
        #endif
        dispatchPending()
    }

    /// Switching origin happens only while signed out; queued items keep their own origin and refuse elsewhere.
    public func updateEnvironment(_ environment: ApiEnvironment) {
        guard environment.origin != origin else { return }
        stopDispatch()
        origin = environment.origin
        let pair = IdentityCoordinator.makeClient(
            origin: environment.origin,
            signer: signer,
            identityStore: identityStore,
            exchange: exchange,
            sessions: sessions,
            photoCache: photoCache
        )
        transport = pair.0
        client = pair.1
        contributorId = transport.storedIdentity()?.contributorId
        ownObservations = []
        ownHistoryState = .unavailable
    }

    public var hasAccount: Bool {
        sessions.currentAccountId() != nil
    }

    // MARK: - Identity

    /// After any login: register the device key when needed and bind it to the account, then resume the queue.
    public func afterSignIn() {
        let transport = self.transport
        Task { [weak self] in
            do {
                let record = try await transport.ensureIdentity()
                self?.contributorId = record.contributorId
                self?.identityNotice = nil
            } catch {
                self?.identityNotice = IdentityCoordinator.describe(error)
            }
            self?.dispatchPending()
        }
    }

    /// Logout: owner data leaves the device and the next request proves identity again.
    public func purgeOwnedData() {
        stopDispatch()
        wakeTask?.cancel()
        wakeTask = nil
        for item in outboxItems {
            cleanupPhoto(of: item)
        }
        outboxItems = []
        outboxStore.purge()
        ownObservations = []
        ownHistoryState = .unavailable
        identityNotice = nil
        let transport = self.transport
        Task {
            await transport.forgetProofs()
        }
    }

    /// Account deletion also forgets the registered contributor; the view model resets the device key.
    public func forgetDeviceIdentity() {
        purgeOwnedData()
        identityStore.clear()
        contributorId = nil
    }

    // MARK: - Queue

    @discardableResult
    public func enqueue(kind: CommunityWriteKind, price: CommunityPriceDraft?, text: CommunityTextDraft?) -> CommunityOutboxItem? {
        guard let scope = transport.ownerScope() else { return nil }
        let waitsForAccount = kind.requiresAccount && !hasAccount
        let item = CommunityOutboxItem(
            id: UUID().uuidString.lowercased(),
            kind: kind,
            ownerScope: scope,
            origin: origin,
            createdAtMillis: Self.nowMillis(),
            price: price,
            text: text,
            status: waitsForAccount ? .draft : .pending,
            lastCode: waitsForAccount ? CommunityOutbox.signInRequiredCode : nil
        )
        outboxItems = CommunityOutbox.enqueue(item, into: outboxItems)
        persist()
        dispatchPending()
        return item
    }

    public func retryNow(_ id: String) {
        outboxItems = CommunityOutbox.retryNow(outboxItems, id: id)
        persist()
        dispatchPending()
    }

    public func discard(_ id: String) {
        if let item = outboxItems.first(where: { $0.id == id }) {
            cleanupPhoto(of: item)
        }
        outboxItems.removeAll { $0.id == id }
        persist()
    }

    public func items(of kind: CommunityWriteKind) -> [CommunityOutboxItem] {
        outboxItems.filter { $0.kind == kind }
    }

    public func dispatchPending() {
        guard dispatchTask == nil, !isOfflineProbe() else { return }
        let client = self.client
        let token = UUID()
        dispatchToken = token
        isDispatching = true
        dispatchTask = Task { [weak self] in
            await self?.drain(client: client, token: token)
        }
    }

    private func stopDispatch() {
        dispatchTask?.cancel()
        dispatchTask = nil
        dispatchToken = nil
        isDispatching = false
    }

    private func drain(client: CommunityWriteClient, token: UUID) async {
        defer {
            if dispatchToken == token {
                dispatchToken = nil
                dispatchTask = nil
                isDispatching = false
                scheduleWake()
            }
        }
        if hasAccount, let scope = transport.ownerScope() {
            outboxItems = CommunityOutbox.resumeAfterSignIn(outboxItems, ownerScope: scope, origin: origin)
            persist()
        }
        for _ in 0..<12 {
            if Task.isCancelled || dispatchToken != token || isOfflineProbe() { break }
            guard let item = CommunityOutbox.nextEligible(outboxItems, nowMillis: Self.nowMillis()) else { break }
            let context = CommunityDispatchContext(
                nowMillis: Self.nowMillis(),
                ownerScope: transport.ownerScope(),
                origin: origin,
                hasAccount: hasAccount
            )
            let updated = await CommunityOutboxDispatcher.run(item, context: context, gateway: client)
            guard dispatchToken == token else { break }
            replace(updated)
            if updated.status.isTerminal {
                cleanupPhoto(of: updated)
            }
        }
    }

    private func scheduleWake() {
        wakeTask?.cancel()
        wakeTask = nil
        let now = Self.nowMillis()
        guard let wake = CommunityOutbox.nextWakeMillis(outboxItems, nowMillis: now) else { return }
        let delayMillis = min(max(wake - now, 1_000), 300_000)
        wakeTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(delayMillis) * 1_000_000)
            if Task.isCancelled { return }
            self?.dispatchPending()
        }
    }

    private func replace(_ item: CommunityOutboxItem) {
        guard let index = outboxItems.firstIndex(where: { $0.id == item.id }) else { return }
        outboxItems[index] = item
        persist()
    }

    private func cleanupPhoto(of item: CommunityOutboxItem) {
        if let photoId = item.price?.photo?.photoId {
            photoCache.delete(id: photoId)
        }
    }

    private func persist() {
        outboxStore.save(outboxItems)
    }

    // MARK: - Owner reads and direct actions

    /// Private owner history from `GET /v1/contributors/me/observations` (signed by this device key).
    public func refreshOwnHistory() async {
        guard !isOfflineProbe() else {
            ownHistoryState = .unavailable
            return
        }
        ownHistoryState = .loading
        do {
            let page = try await client.ownObservations(limit: 50)
            ownObservations = page.items
            ownHistoryState = .loaded
        } catch {
            ownHistoryState = .unavailable
            identityNotice = Self.describe(error)
        }
    }

    /// Refreshes the validation state of accepted price receipts (RECEIVED and VALIDATING).
    public func refreshAcceptedStatuses() async {
        guard !isOfflineProbe() else { return }
        let open: Set<String> = ["RECEIVED", "VALIDATING"]
        let targets = outboxItems.filter { item in
            item.kind == .priceObservation && item.status == .accepted && open.contains(item.serverState ?? "")
        }
        for item in targets.prefix(10) {
            guard let receiptId = item.receiptId,
                  let observation = try? await client.observationStatus(id: receiptId),
                  var current = outboxItems.first(where: { $0.id == item.id }) else {
                continue
            }
            current.serverState = observation.validationState
            replace(current)
        }
    }

    /// Requests a two-minute camera window for evidence; nil keeps the photo as local OCR input only.
    public func authorizePhotoCapture(stationId: String) async -> PhotoCaptureGrant? {
        guard !isOfflineProbe(), CommunityWire.isUuid(stationId) else { return nil }
        do {
            let grant = try await client.authorizePhotoCapture(
                stationId: stationId,
                clientCaptureId: UUID().uuidString.lowercased()
            )
            identityNotice = nil
            return grant
        } catch {
            identityNotice = Self.describe(error)
            return nil
        }
    }

    public func voteStationComment(commentId: String, valid: Bool) async -> Result<StationCommentTally, AppValidationError> {
        guard !isOfflineProbe() else {
            return .failure(AppValidationError(String(localized: "Conecte-se para votar.")))
        }
        do {
            let tally = try await client.voteStationComment(commentId: commentId, valid: valid)
            return .success(tally)
        } catch {
            return .failure(AppValidationError(Self.describe(error)))
        }
    }

    // MARK: - Labels

    static func nowMillis() -> Int64 {
        Int64(Date().timeIntervalSince1970 * 1000)
    }

    public static func statusLabel(for item: CommunityOutboxItem) -> String {
        switch item.status {
        case .draft:
            return String(localized: "Rascunho: entre na conta gratuita para publicar")
        case .pending:
            if item.attempts == 0 {
                return String(localized: "Pendente de envio")
            }
            return String(localized: "Pendente: nova tentativa automática (\(item.attempts))")
        case .accepted:
            return String(localized: "Aceito pelo servidor · \(serverStateLabel(item.serverState))")
        case .refused:
            return String(localized: "Recusado pelo servidor · \(codeLabel(item.lastCode))")
        case .expired:
            return String(localized: "Expirado · \(codeLabel(item.lastCode))")
        }
    }

    public static func serverStateLabel(_ state: String?) -> String {
        switch state ?? "" {
        case "RECEIVED": return String(localized: "recebido, aguardando validação")
        case "VALIDATING": return String(localized: "em validação")
        case "VALIDATED": return String(localized: "validado")
        case "REJECTED": return String(localized: "rejeitado na validação")
        case "PENDING_REVIEW": return String(localized: "em moderação")
        case "PUBLISHED": return String(localized: "publicado")
        default: return state ?? String(localized: "recibo registrado")
        }
    }

    public static func codeLabel(_ code: String?) -> String {
        switch code ?? "" {
        case CommunityOutbox.ownerChangedCode: return String(localized: "criado em outra conta ou ambiente")
        case CommunityOutbox.attemptsExhaustedCode: return String(localized: "limite de tentativas atingido")
        case "contribution.photo-expired": return String(localized: "a foto passou do prazo de 24 horas")
        case "contribution.photo-rejected": return String(localized: "a foto foi recusada")
        case "community.quota-exceeded", "feedback.quota-exceeded": return String(localized: "limite de envios atingido")
        case "community.capture-ineligible": return String(localized: "foto fora da janela autorizada")
        case "community.account-blocked": return String(localized: "conta suspensa ou excluída")
        case "feedback.text-too-long", "feedback.text-invalid": return String(localized: "texto inválido ou acima de 280 caracteres")
        case "account.binding-cross-account-refused": return String(localized: "este aparelho já está vinculado a outra conta")
        case "": return String(localized: "sem detalhe")
        default: return code ?? ""
        }
    }

    public static func describe(_ error: Error) -> String {
        if let write = error as? CommunityWriteError {
            switch write {
            case .signInRequired: return String(localized: "Entre na conta gratuita para continuar.")
            case .photoExpired: return String(localized: "A foto passou do prazo de 24 horas.")
            case .photoRejected: return String(localized: "A foto foi recusada pelo servidor.")
            case .mediaPending: return String(localized: "A foto ainda está em processamento.")
            case .invalidReceipt: return String(localized: "O servidor respondeu com um recibo inválido.")
            case .invalid(let code): return String(localized: "Dados recusados (\(code)).")
            }
        }
        if let transport = error as? SignedTransportError {
            switch transport {
            case .offline: return String(localized: "Sem conexão com o servidor.")
            case .identityUnavailable, .signingUnavailable: return String(localized: "Chave do dispositivo indisponível.")
            case .sessionRequired: return String(localized: "Entre na conta gratuita para continuar.")
            case .originUnavailable, .invalidRequest: return String(localized: "Ambiente de rede inválido.")
            case .oversizedResponse, .invalidResponse: return String(localized: "Resposta inválida do servidor.")
            case .refused(let status, let code, _):
                let label = codeLabel(code ?? ("http-" + String(status)))
                return String(localized: "Servidor recusou (\(label)).")
            }
        }
        return String(localized: "Serviço indisponível no momento.")
    }
}
