import SwiftUI
import AnpFuelCore

@MainActor
public struct SuggestStationSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var isChargingSite: Bool
    @State private var displayName = ""
    @State private var cnpj = ""
    @State private var address = ""
    @State private var openingCityPicker = false
    @State private var proposalQueued = false
    @State private var formWarning: String?
    @State private var requestNotice: String?
    @State private var items: [StationIntakeReceipt] = []
    @State private var selecting: StationIntakeReceipt?
    @State private var cancellation: StationIntakeReceipt?
    @State private var requestGeneration = UUID()
    @State private var isLoading = false
    @State private var isCancelling = false
    @State private var formScope: Scope?
    @State private var cancelIntent: (id: String, operationId: String)?

    private struct Scope: Equatable {
        let accountId: String?
        let origin: String
        let offline: Bool
    }

    public init(viewModel: AppStateViewModel, chargingSite: Bool = false) {
        self.viewModel = viewModel
        self._isChargingSite = State(initialValue: chargingSite)
    }

    private var scope: Scope {
        Scope(accountId: viewModel.currentAccountIdOrNil,
              origin: viewModel.snapshot.apiEnvironment.origin, offline: viewModel.isOfflineSimulation)
    }

    private var queued: [CommunityOutboxItem] { viewModel.identity.stationSuggestionItems }
    private var completed: Set<String> { Set(queued.compactMap(\.receiptId)) }

    private var duplicate: StationRecord? {
        guard !isChargingSite, let clean = BrazilianCNPJ.normalized(cnpj) else { return nil }
        return viewModel.stations.first { BrazilianCNPJ.normalized($0.cnpj) == clean }
    }

    public var body: some View {
        NavigationStack {
            Form {
                if scope.accountId == nil {
                    Section {
                        Text(String(localized: "android.suggest_signin"))
                        Button(String(localized: "android.auth_title")) { viewModel.activeSheet = .auth }
                    }
                } else if formScope?.accountId == scope.accountId && formScope?.origin == scope.origin {
                    proposalForm
                    requests
                }
            }
            .navigationTitle(isChargingSite ? String(localized: "android.charging_add_site") : String(localized: "android.suggest_title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
            .task(id: scope) {
                if formScope?.accountId != scope.accountId || formScope?.origin != scope.origin {
                    resetForm()
                    cancelIntent = nil
                }
                formScope = scope
                items = []
                selecting = nil
                cancellation = nil
                formWarning = nil
                requestNotice = nil
                await refresh()
            }
            .onChange(of: completed) { _, _ in Task { await refresh() } }
            .refreshable { await refresh() }
            .sheet(isPresented: $openingCityPicker) {
                LocationPickerSheetView(viewModel: viewModel)
            }
            .confirmationDialog(
                String(localized: "android.suggest_cancel"), isPresented: cancellationPresented, presenting: cancellation
            ) { item in
                Button(String(localized: "android.suggest_cancel"), role: .destructive) {
                    Task { await cancel(item) }
                }
            }
        }
    }

    private var proposalForm: some View {
        Section(String(localized: "station.suggestion.fields")) {
            if proposalQueued {
                Button(String(localized: "android.suggest_new_form")) { resetForm() }
            } else {
                Toggle(String(localized: "station.suggestion.charging"), isOn: $isChargingSite)
                TextField(
                    isChargingSite ? String(localized: "android.charging_suggestion_name_optional") : String(localized: "android.suggest_display"),
                    text: $displayName
                )
                Button(viewModel.municipalityLabel) { openingCityPicker = true }
                if isChargingSite {
                    TextField(String(localized: "android.charging_suggestion_address"), text: $address)
                    Text(String(localized: "android.charging_suggestion_help"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    TextField(String(localized: "android.suggest_cnpj"), text: $cnpj)
                        #if os(iOS)
                        .textInputAutocapitalization(.characters)
                        #endif
                        .autocorrectionDisabled()
                }
                if let duplicate {
                    Text(String(localized: "android.suggest_duplicate_hint", defaultValue: "Already listed as \(duplicate.name) — review will link duplicates."))
                        .font(.caption)
                }
                if let formWarning {
                    Text(formWarning).foregroundStyle(.secondary)
                }
                Button(String(localized: "android.suggest_submit")) { enqueue() }
                    .disabled(viewModel.isAuthWorking)
            }
        }
    }

    private var requests: some View {
        Section(String(localized: "android.suggest_mine_title")) {
            if let requestNotice { Text(requestNotice).font(.caption) }
            if isLoading { ProgressView() }
            if scope.offline {
                Text(String(localized: "station.suggestion.status_offline"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                Button(String(localized: "android.suggest_refresh_status")) { Task { await refresh() } }
                    .disabled(isLoading || isCancelling)
            }
            if let selecting { statusRow(selecting, showsActions: true) }
            ForEach(items) { item in
                if item.id != selecting?.id {
                    Button { Task { await load(item.id) } } label: { statusRow(item, showsActions: false) }
                        .disabled(isCancelling || scope.offline)
                }
            }
            ForEach(queued.filter { item in
                item.receiptId.map { receiptId in !items.contains { $0.id == receiptId } } ?? true
            }) { item in
                CommunityOutboxRow(viewModel: viewModel, item: item)
                if item.status == .accepted {
                    Text(String(localized: "android.suggest_sent")).font(.caption).foregroundStyle(.secondary)
                }
            }
            if items.isEmpty && queued.isEmpty && !isLoading && requestNotice == nil && !scope.offline {
                Text(String(localized: "station.suggestion.empty"))
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var cancellationPresented: Binding<Bool> {
        Binding(get: { cancellation != nil }, set: { if !$0 { cancellation = nil } })
    }

    private func statusRow(_ item: StationIntakeReceipt, showsActions: Bool) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            let title = queued.first(where: { $0.receiptId == item.id })?.stationProposal?.label
            Text(title ?? String(localized: "station.suggestion.reference", defaultValue: "Suggestion \(String(item.id.prefix(8)))"))
                .font(.subheadline.bold())
            HStack {
                Text(stateLabel(item.state))
                if let date = item.createdDate { Text(date.formatted(date: .abbreviated, time: .shortened)) }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            if showsActions && item.state == .pending {
                Button(String(localized: "android.suggest_cancel"), role: .destructive) { cancellation = item }
                    .disabled(isCancelling || scope.offline)
            }
        }
    }

    private func enqueue() {
        guard !proposalQueued, let municipality = viewModel.selectedMunicipality else {
            formWarning = String(localized: "android.feed_choose_city")
            return
        }
        do {
            let proposal = try StationIntakeProposal(
                displayName: displayName, municipalityCode: municipality.ibgeCode, state: municipality.state,
                cnpj: isChargingSite ? "" : cnpj, publicAddress: isChargingSite ? address : "",
                chargingSite: isChargingSite
            )
            guard viewModel.identity.enqueueStationSuggestion(proposal) != nil else {
                formWarning = String(localized: "android.suggest_signin")
                return
            }
            proposalQueued = true
            formWarning = nil
        } catch {
            formWarning = String(localized: "android.suggest_invalid")
        }
    }

    private func resetForm() {
        proposalQueued = false
        displayName = ""
        cnpj = ""
        address = ""
        formWarning = nil
    }

    private func refresh() async {
        guard !scope.offline, scope.accountId != nil else { return }
        let original = scope
        let client = viewModel.communityGateway
        guard let owner = client.transport.ownerScope() else { return }
        let token = UUID()
        requestGeneration = token
        isLoading = true
        requestNotice = nil
        defer { if requestGeneration == token { isLoading = false } }
        do {
            let rows = try await client.ownedStationSuggestions(expectedScope: owner)
            guard original == scope, requestGeneration == token, !Task.isCancelled else { return }
            items = rows
            if let selected = selecting { selecting = rows.first { $0.id == selected.id } }
        } catch {
            guard original == scope, requestGeneration == token, !Task.isCancelled else { return }
            requestNotice = Self.describe(error)
        }
    }

    private func load(_ id: String) async {
        guard !scope.offline, scope.accountId != nil else { return }
        let original = scope
        let client = viewModel.communityGateway
        guard let owner = client.transport.ownerScope() else { return }
        let token = UUID()
        requestGeneration = token
        isLoading = true
        requestNotice = nil
        defer { if requestGeneration == token { isLoading = false } }
        do {
            let row = try await client.stationSuggestionStatus(id: id, expectedScope: owner)
            guard original == scope, requestGeneration == token, !Task.isCancelled else { return }
            selecting = row
        } catch {
            guard original == scope, requestGeneration == token, !Task.isCancelled else { return }
            selecting = nil
            requestNotice = Self.describe(error)
        }
    }

    private func cancel(_ item: StationIntakeReceipt) async {
        guard !isCancelling, item.state == .pending, !scope.offline, scope.accountId != nil else { return }
        let original = scope
        let client = viewModel.communityGateway
        guard let owner = client.transport.ownerScope() else { return }
        let operation = cancelIntent.flatMap { $0.id == item.id ? $0.operationId : nil }
            ?? UUID().uuidString.lowercased()
        cancelIntent = (item.id, operation)
        isCancelling = true
        defer { isCancelling = false }
        do {
            try await client.cancelStationSuggestion(id: item.id, operationId: operation, expectedScope: owner)
            guard original == scope, !Task.isCancelled else { return }
            cancelIntent = nil
            selecting = nil
            await refresh()
            guard original == scope, !Task.isCancelled else { return }
            requestNotice = String(localized: "station.suggestion.cancelled")
        } catch {
            guard original == scope, !Task.isCancelled else { return }
            if let refusal = error as? SignedTransportError, case .refused(let status, _, _) = refusal,
               status == 404 || status == 409 {
                selecting = nil
                if status == 409,
                   let latest = try? await client.stationSuggestionStatus(id: item.id, expectedScope: owner),
                   latest.state == .cancelled, original == scope, !Task.isCancelled {
                    cancelIntent = nil
                    await refresh()
                    guard original == scope, !Task.isCancelled else { return }
                    requestNotice = String(localized: "station.suggestion.cancelled")
                    return
                }
                cancelIntent = nil
                await refresh()
            }
            guard original == scope, !Task.isCancelled else { return }
            requestNotice = Self.describe(error)
        }
    }

    private func stateLabel(_ state: StationIntakeReceipt.State) -> String {
        switch state {
        case .pending: return String(localized: "station.suggestion.pending")
        case .cancelled: return String(localized: "station.suggestion.cancelled")
        case .approved: return String(localized: "station.suggestion.approved")
        case .rejected: return String(localized: "station.suggestion.rejected")
        }
    }

    static func describe(_ error: Error) -> String {
        if let transport = error as? SignedTransportError {
            switch transport {
            case .sessionRequired, .refused(401, _, _):
                return String(localized: "android.suggest_signin")
            case .refused(403, _, _):
                return String(localized: "station.suggestion.author_forbidden")
            case .refused(404, _, _):
                return String(localized: "station.suggestion.not_found")
            case .refused(409, .some("intake.closed"), _):
                return String(localized: "station.suggestion.closed")
            case .refused(409, _, _):
                return String(localized: "android.suggest_conflict")
            case .refused(429, _, _):
                return String(localized: "android.suggest_quota")
            case .refused(400, _, _):
                return String(localized: "android.suggest_invalid")
            default: break
            }
        }
        if let failure = error as? CommunityWriteError, failure == .signInRequired {
            return String(localized: "android.suggest_signin")
        }
        return String(localized: "station.suggestion.read_failed")
    }
}
