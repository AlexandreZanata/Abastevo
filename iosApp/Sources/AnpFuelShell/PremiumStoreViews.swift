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
            phase = .message(String(localized: "android.premium_signed_out_title"))
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
            phase = .message(String(localized: "android.premium_offline"))
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
            phase = .message(String(localized: "premium.app_store_offline"))
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
            phase = .message(String(localized: "premium.purchase_incomplete"))
        }
    }

    /// Restore: syncs with the App Store, then sends each current
    /// subscription entitlement to the server.
    public func restore() async {
        phase = .loading
        do {
            try await AppStore.sync()
        } catch {
            phase = .message(String(localized: "premium.restore_offline"))
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
            phase = .message(String(localized: "premium.no_subscription"))
        }
    }

    private func handle(_ result: VerificationResult<StoreKit.Transaction>, kind: PremiumPurchaseKind) async {
        guard case .verified(let transaction) = result else {
            // An unverified transaction is never sent or finished.
            phase = .message(String(localized: "premium.unverified_transaction"))
            return
        }
        guard transaction.productType == .autoRenewable else { return }
        guard let current = session(), let signer = signer() else {
            phase = .message(String(localized: "premium.purchase_sign_in"))
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
            phase = .message(String(localized: "android.premium_status_active"))
        case .failure(.denied):
            phase = .message(String(localized: "premium.other_account"))
        case .failure(.authRequired):
            phase = .message(String(localized: "premium.session_expired"))
        case .failure:
            phase = .message(String(localized: "premium.verify_retry"))
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
                    Button(String(localized: "android.premium_restore")) {
                        Task { await model.restore() }
                    }
                    Button(String(localized: "premium.manage_app_store")) {
                        model.showManageSheet = true
                    }
                } footer: {
                    Text(String(localized: "premium.cancel_and_delete"))
                }
                legalSection
            }
            .navigationTitle(String(localized: "premium.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
        Section(String(localized: "android.premium_status_section_title")) {
            if let status = model.status {
                LabeledContent(String(localized: "premium.status"), value: Self.stateLabel(status))
                if let until = status.validUntil, status.isPremiumActive {
                    LabeledContent(String(localized: "premium.valid_until"), value: Self.dateLabel(until))
                }
                ForEach(Array(status.tiers.enumerated()), id: \.offset) { _, tier in
                    LabeledContent(String(localized: "premium.source"), value: Self.sourceLabel(tier.source))
                }
            } else {
                Text(String(localized: "android.premium_signed_out_title"))
                    .foregroundStyle(.secondary)
            }
            switch model.phase {
            case .loading, .verifying, .purchasing:
                ProgressView()
            case .pending:
                Text(String(localized: "premium.ask_to_buy"))
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
                Text(String(localized: "premium.active_platforms"))
            }
        case .unavailable:
            Section {
                Text(String(localized: "premium.iphone_subscription_unavailable"))
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
                Text(String(localized: "premium.subscribe_app_store"))
            } footer: {
                Text(String(localized: "premium.store_disclosure"))
            }
        }
    }

    private var legalSection: some View {
        Section {
            if let terms = Self.termsUrl {
                Link(String(localized: "premium.terms_eula"), destination: terms)
            }
            if let privacy = Self.privacyUrl {
                Link(String(localized: "premium.privacy_policy"), destination: privacy)
            }
        }
    }

    static func priceLine(_ product: Product) -> String {
        guard let period = product.subscription?.subscriptionPeriod else {
            return product.displayPrice
        }
        let components: DateComponents
        let unit: NSCalendar.Unit
        switch period.unit {
        case .day:
            components = DateComponents(day: period.value)
            unit = .day
        case .week:
            components = DateComponents(weekOfMonth: period.value)
            unit = .weekOfMonth
        case .month:
            components = DateComponents(month: period.value)
            unit = .month
        case .year:
            components = DateComponents(year: period.value)
            unit = .year
        @unknown default:
            return product.displayPrice
        }
        let formatter = DateComponentsFormatter()
        formatter.allowedUnits = unit
        formatter.unitsStyle = .full
        guard let interval = formatter.string(from: components) else { return product.displayPrice }
        return String(localized: "premium.price_period", defaultValue: "\(product.displayPrice) / \(interval)")
    }

    static func stateLabel(_ status: PremiumStatusWire) -> String {
        switch status.state {
        case "ACTIVE": return String(localized: "android.premium_status_active")
        case "CANCELED": return String(localized: "android.premium_status_canceled")
        case "GRACE": return String(localized: "android.premium_status_grace")
        case "HOLD": return String(localized: "android.premium_status_hold")
        case "PAUSED": return String(localized: "android.premium_status_paused")
        case "PENDING": return String(localized: "premium.payment_pending")
        case "EXPIRED": return String(localized: "android.premium_status_expired")
        case "REVOKED": return String(localized: "android.premium_status_revoked")
        default: return String(localized: "android.premium_status_none")
        }
    }

    static func sourceLabel(_ source: String) -> String {
        switch source {
        case "app_store": return String(localized: "premium.source_app_store")
        case "play": return String(localized: "premium.source_play")
        case "web": return String(localized: "premium.source_web")
        case "admin": return String(localized: "premium.source_admin")
        default: return String(localized: "premium.source_unknown")
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
