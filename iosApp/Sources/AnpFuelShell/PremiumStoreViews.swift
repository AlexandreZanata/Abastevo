import Foundation
import StoreKit
import SwiftUI
import AnpFuelCore

/// StoreKit 2 Premium purchase, restore and management (B-BR-STORE-01..10).
/// The server verifies every transaction; the app never grants access from a
/// local StoreKit result.
@MainActor
public final class PremiumStoreModel: ObservableObject {
    public enum Phase: Equatable {
        case idle
        case loading
        case purchasing
        case verifying
        case pending
        case message(String)
    }

    @Published public private(set) var status: PremiumStatusWire?
    @Published public private(set) var products: [Product] = []
    @Published public private(set) var phase: Phase = .idle
    @Published public var showManageSheet = false

    private let client: AbastevoHttpClient
    private let session: () -> PortableAuth.Session?
    private let signer: () -> PremiumRequestSigner?
    private var updates: Task<Void, Never>?
    /// One operation ID per transaction, reused across retries.
    private var operations: [UInt64: String] = [:]

    public init(
        client: AbastevoHttpClient,
        session: @escaping () -> PortableAuth.Session?,
        signer: @escaping () -> PremiumRequestSigner?
    ) {
        self.client = client
        self.session = session
        self.signer = signer
    }

    public var availability: PremiumPurchaseAvailability {
        guard let status else { return .unavailable }
        return PremiumStoreBilling.availability(for: status)
    }

    /// Starts the transaction listener once; it delivers unfinished
    /// transactions, renewals, Ask to Buy approvals and other-device purchases.
    public func start() {
        guard updates == nil else { return }
        updates = Task { [weak self] in
            for await result in StoreKit.Transaction.updates {
                await self?.handle(result, kind: .verify)
            }
        }
        Task { [weak self] in
            for await result in StoreKit.Transaction.unfinished {
                await self?.handle(result, kind: .verify)
            }
        }
    }

    public func refresh() async {
        guard let current = session() else {
            status = nil
            products = []
            phase = .message(String(localized: "Entre na sua conta gratuita para ver o Premium."))
            return
        }
        phase = .loading
        do {
            let fresh = try await client.fetchPremiumStatus(familyId: current.familyId, accessToken: current.accessToken)
            guard session()?.accountId == current.accountId else { return }
            status = fresh
            await loadProducts()
            phase = .idle
        } catch {
            phase = .message(String(localized: "Não foi possível consultar o Premium agora. Tente novamente."))
        }
    }

    private func loadProducts() async {
        guard case .available(let ids, _) = availability else {
            products = []
            return
        }
        do {
            let loaded = try await Product.products(for: ids)
            products = loaded.sorted { $0.price < $1.price }
        } catch {
            products = []
            phase = .message(String(localized: "A App Store não respondeu. Tente novamente."))
        }
    }

    public func purchase(_ product: Product) async {
        guard case .available(_, let token) = availability else { return }
        phase = .purchasing
        do {
            let result = try await product.purchase(options: [.appAccountToken(token)])
            switch result {
            case .success(let verification):
                await handle(verification, kind: .verify)
            case .pending:
                phase = .pending
            case .userCancelled:
                phase = .idle
            @unknown default:
                phase = .idle
            }
        } catch {
            phase = .message(String(localized: "A compra não foi concluída. Nenhuma cobrança foi confirmada."))
        }
    }

    /// Restore: syncs with the App Store, then sends each current
    /// subscription entitlement to the server.
    public func restore() async {
        phase = .loading
        do {
            try await AppStore.sync()
        } catch {
            phase = .message(String(localized: "Não foi possível falar com a App Store para restaurar."))
            return
        }
        var found = false
        for await result in StoreKit.Transaction.currentEntitlements {
            if case .verified(let transaction) = result, transaction.productType == .autoRenewable {
                found = true
                await handle(result, kind: .restore)
            }
        }
        if !found {
            phase = .message(String(localized: "Nenhuma assinatura da App Store encontrada para este Apple ID."))
        }
    }

    private func handle(_ result: VerificationResult<StoreKit.Transaction>, kind: PremiumPurchaseKind) async {
        guard case .verified(let transaction) = result else {
            // An unverified transaction is never sent or finished.
            phase = .message(String(localized: "A App Store não confirmou esta transação."))
            return
        }
        guard transaction.productType == .autoRenewable else { return }
        guard let current = session(), let signer = signer() else {
            phase = .message(String(localized: "Entre na conta usada na compra para ativar o Premium."))
            return
        }
        let original = transaction.originalID
        let operation = operations[original] ?? UUID().uuidString.lowercased()
        operations[original] = operation
        phase = .verifying
        let outcome: Result<PremiumReceiptWire, PremiumApiError>
        do {
            let receipt = try await client.submitAppStorePurchase(
                kind: kind,
                originalTransactionId: original,
                operationId: operation,
                familyId: current.familyId,
                accessToken: current.accessToken,
                signer: signer
            )
            outcome = .success(receipt)
        } catch let error as PremiumApiError {
            outcome = .failure(error)
        } catch {
            outcome = .failure(.unavailable)
        }
        if PremiumStoreBilling.finishDecision(for: outcome) == .finish {
            await transaction.finish()
            operations[original] = nil
        }
        switch outcome {
        case .success(let receipt):
            if session()?.accountId == current.accountId {
                status = receipt.status
            }
            phase = .message(String(localized: "Premium ativo. Obrigado por apoiar o Abastevo."))
        case .failure(.denied):
            phase = .message(String(localized: "Esta assinatura pertence a outra conta e não foi ativada aqui."))
        case .failure(.authRequired):
            phase = .message(String(localized: "Sua sessão expirou. Entre novamente para ativar o Premium."))
        case .failure:
            phase = .message(String(localized: "Compra recebida pela App Store. A ativação será tentada de novo automaticamente."))
        }
    }
}

public struct PremiumStoreSheetView: View {
    @ObservedObject var model: PremiumStoreModel
    @Environment(\.dismiss) private var dismiss

    private static let termsUrl = URL(string: "https://abastevo.com.br/termos/")
    private static let privacyUrl = URL(string: "https://abastevo.com.br/privacidade/")

    public init(model: PremiumStoreModel) {
        self.model = model
    }

    public var body: some View {
        NavigationStack {
            List {
                statusSection
                purchaseSection
                Section {
                    Button(String(localized: "Restaurar compras")) {
                        Task { await model.restore() }
                    }
                    Button(String(localized: "Gerenciar assinatura na App Store")) {
                        model.showManageSheet = true
                    }
                } footer: {
                    Text("Cancelar o Premium não apaga seus dados. Excluir a conta não cancela a cobrança da App Store: cancele a assinatura aqui.")
                }
                legalSection
            }
            .navigationTitle(String(localized: "Abastevo Premium"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "Fechar")) { dismiss() }
                }
            }
            .manageSubscriptionsSheet(isPresented: $model.showManageSheet)
            .task {
                model.start()
                await model.refresh()
            }
            .refreshable { await model.refresh() }
        }
    }

    @ViewBuilder
    private var statusSection: some View {
        Section(String(localized: "Seu acesso")) {
            if let status = model.status {
                LabeledContent(String(localized: "Situação"), value: Self.stateLabel(status))
                if let until = status.validUntil, status.isPremiumActive {
                    LabeledContent(String(localized: "Acesso até"), value: Self.dateLabel(until))
                }
                ForEach(Array(status.tiers.enumerated()), id: \.offset) { _, tier in
                    LabeledContent(String(localized: "Origem"), value: Self.sourceLabel(tier.source))
                }
            } else {
                Text("Entre na sua conta gratuita para consultar o Premium.")
                    .foregroundStyle(.secondary)
            }
            switch model.phase {
            case .loading, .verifying, .purchasing:
                ProgressView()
            case .pending:
                Text("Compra aguardando aprovação (Pedir para Comprar). O Premium ativa quando a App Store confirmar.")
                    .font(.footnote)
            case .message(let text):
                Text(text).font(.footnote)
            case .idle:
                EmptyView()
            }
        }
    }

    @ViewBuilder
    private var purchaseSection: some View {
        switch model.availability {
        case .active:
            Section {
                Text("Seu Premium já está ativo e vale também no Android e na web.")
            }
        case .unavailable:
            Section {
                Text("A assinatura pelo iPhone ainda não está disponível.")
                    .foregroundStyle(.secondary)
            }
        case .available:
            Section {
                ForEach(model.products, id: \.id) { product in
                    Button {
                        Task { await model.purchase(product) }
                    } label: {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(product.displayName).font(.headline)
                            Text(Self.priceLine(product)).font(.subheadline)
                            if !product.description.isEmpty {
                                Text(product.description).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                    }
                    .disabled(model.phase == .purchasing || model.phase == .verifying)
                }
            } header: {
                Text("Assinar pela App Store")
            } footer: {
                Text("Backup e sincronização na nuvem, alertas avançados, ferramentas profissionais e veículos adicionais. A cobrança é feita na sua conta Apple e renova automaticamente até ser cancelada nos Ajustes, pelo menos 24 horas antes do fim do período.")
            }
        }
    }

    private var legalSection: some View {
        Section {
            if let terms = Self.termsUrl {
                Link(String(localized: "Termos de Uso (EULA)"), destination: terms)
            }
            if let privacy = Self.privacyUrl {
                Link(String(localized: "Política de Privacidade"), destination: privacy)
            }
        }
    }

    static func priceLine(_ product: Product) -> String {
        guard let period = product.subscription?.subscriptionPeriod else {
            return product.displayPrice
        }
        let unit: String
        switch period.unit {
        case .day: unit = period.value == 1 ? String(localized: "dia") : String(localized: "dias")
        case .week: unit = period.value == 1 ? String(localized: "semana") : String(localized: "semanas")
        case .month: unit = period.value == 1 ? String(localized: "mês") : String(localized: "meses")
        case .year: unit = period.value == 1 ? String(localized: "ano") : String(localized: "anos")
        @unknown default: unit = ""
        }
        let count = period.value == 1 ? "" : "\(period.value) "
        return "\(product.displayPrice) / \(count)\(unit)"
    }

    static func stateLabel(_ status: PremiumStatusWire) -> String {
        switch status.state {
        case "ACTIVE": return String(localized: "Ativo")
        case "CANCELED": return String(localized: "Ativo até o fim do período (renovação desligada)")
        case "GRACE": return String(localized: "Ativo em período de carência de cobrança")
        case "HOLD": return String(localized: "Suspenso: problema de cobrança na loja")
        case "PAUSED": return String(localized: "Pausado")
        case "PENDING": return String(localized: "Aguardando pagamento")
        case "EXPIRED": return String(localized: "Expirado")
        case "REVOKED": return String(localized: "Reembolsado ou revogado")
        default: return String(localized: "Gratuito")
        }
    }

    static func sourceLabel(_ source: String) -> String {
        switch source {
        case "app_store": return "App Store"
        case "play": return "Google Play"
        case "web": return String(localized: "Site abastevo.com.br")
        case "admin": return String(localized: "Concedido pela equipe")
        default: return source
        }
    }

    static func dateLabel(_ iso: String) -> String {
        let precise = ISO8601DateFormatter()
        precise.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let plain = ISO8601DateFormatter()
        plain.formatOptions = [.withInternetDateTime]
        guard let date = precise.date(from: iso) ?? plain.date(from: iso) else { return iso }
        return date.formatted(date: .abbreviated, time: .shortened)
    }
}
