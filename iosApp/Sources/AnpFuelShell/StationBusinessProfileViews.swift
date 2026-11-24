import SwiftUI
import UniformTypeIdentifiers
import AnpFuelCore

struct StationBusinessProfileCard: View {
    @ObservedObject var viewModel: AppStateViewModel
    let canonicalStationId: String?
    let fuel: FuelProduct

    @State private var loadedKey: ReadKey?
    @State private var profile: StationBusinessProfile?
    @State private var pending = true
    @State private var stale = false
    @State private var showOwner = false

    private struct ReadKey: Equatable {
        let stationId: String
        let origin: String
        let offline: Bool

        func sameStation(as other: ReadKey?) -> Bool {
            stationId == other?.stationId && origin == other?.origin
        }
    }

    private var key: ReadKey? {
        canonicalStationId.map {
            ReadKey(stationId: $0, origin: viewModel.snapshot.apiEnvironment.origin, offline: viewModel.isOfflineSimulation)
        }
    }

    var body: some View {
        if let key {
            EnergiaCard {
                Text(String(localized: "android.station_profile_title"))
                    .font(.headline)
                    .accessibilityAddTraits(.isHeader)
                if pending { ProgressView() }
                if let profile, key.sameStation(as: loadedKey) {
                    let currentBadge = profile.verifiedRepresentation && !stale && !pending && !key.offline
                    Text(stale || key.offline
                         ? String(localized: "android.station_profile_stale")
                         : currentBadge
                            ? String(localized: "android.station_profile_verified")
                            : String(localized: "android.station_profile_unclaimed"))
                        .font(.subheadline.bold())
                    if currentBadge, let source = profile.operatorSource {
                        Text(sourceLabel(source)).font(.caption).foregroundStyle(.secondary)
                    }
                    Text(String(localized: "android.station_profile_representation_note"))
                        .font(.caption).foregroundStyle(.secondary)
                    if profile.business.isEmpty {
                        Text(String(localized: "android.station_profile_empty"))
                            .foregroundStyle(.secondary)
                    }
                    ForEach(StationBusinessField.allCases) { field in
                        if let value = profile.business[field], !value.isEmpty {
                            LabeledContent(field.title, value: value)
                        }
                    }
                    Button(String(localized: "android.station_profile_claim")) { showOwner = true }
                        .disabled(pending || stale || key.offline)
                } else if !pending {
                    Text(String(localized: "android.station_profile_unavailable"))
                        .foregroundStyle(.secondary)
                }
                if stale || key.offline || (!pending && profile == nil) {
                    Button(String(localized: "android.action_retry")) { Task { await load(key) } }
                        .disabled(pending || key.offline)
                }
            }
            .task(id: key) { await load(key) }
            .onChange(of: key) { old, next in
                if !next.sameStation(as: old) || next.offline { showOwner = false }
            }
            .sheet(isPresented: $showOwner) {
                StationRepresentationSheet(viewModel: viewModel, stationId: key.stationId, fuel: fuel)
            }
        }
    }

    private func load(_ request: ReadKey) async {
        if !request.sameStation(as: loadedKey) { profile = nil }
        loadedKey = request
        pending = true
        do {
            let result = try await viewModel.socialReadClient().fetchStationBusinessProfile(stationId: request.stationId)
            guard !Task.isCancelled, key == request else { return }
            profile = result
            stale = false
        } catch {
            guard !Task.isCancelled, key == request else { return }
            stale = profile != nil
        }
        pending = false
    }

    private func sourceLabel(_ source: String) -> String {
        switch source {
        case "registry": return String(localized: "station.owner.source_registry")
        case "dou": return String(localized: "station.owner.source_dou")
        default: return String(localized: "station.owner.source_review")
        }
    }
}

@MainActor
private final class StationRepresentationModel: ObservableObject {
    private struct Request {
        let client: CommunityWriteClient
        let scope: String
        let origin: String
        let generation: UUID
    }

    private struct PreparedProof {
        let claim: StationRepresentationClaim
        let bytes: Data
        let operationId: String
    }

    let viewModel: AppStateViewModel
    let stationId: String

    @Published private(set) var claims: [StationRepresentationClaim] = []
    @Published private(set) var selected: StationRepresentationClaim?
    @Published private(set) var ready = false
    @Published private(set) var busy = false
    @Published private(set) var signInRequired = false
    @Published private(set) var proofUnavailable = false
    @Published private(set) var notice: String?
    @Published private(set) var preparedBytes: Int?
    @Published private(set) var managementScopes: Set<String> = []
    @Published private(set) var revision: Int?
    @Published var role = StationClaimRole.manager {
        didSet { if role != oldValue { claimKey = Self.newId() } }
    }
    @Published var fields: [StationBusinessField: String] = [:]
    @Published var reply = ""
    @Published var fuel: FuelProduct

    private var original: [StationBusinessField: String] = [:]
    private var claimKey = StationRepresentationModel.newId()
    private var generation = UUID()
    private var prepared: PreparedProof?
    private var editAttempt: (revision: Int, fields: [StationBusinessField: String], id: String)?
    private var replyAttempt: (fuel: FuelProduct, text: String, id: String)?
    private var nextClaimOperation: (action: String, declaration: String, id: String)?
    private var refreshRequired = false

    init(viewModel: AppStateViewModel, stationId: String, fuel: FuelProduct) {
        self.viewModel = viewModel
        self.stationId = stationId
        self.fuel = fuel
    }

    var identityKey: String {
        [viewModel.currentAccountIdOrNil ?? "", viewModel.snapshot.apiEnvironment.origin,
         viewModel.isOfflineSimulation ? "offline" : "online"].joined(separator: "|")
    }

    var changed: [StationBusinessField: String] { fields.filter { original[$0.key] != $0.value } }
    var canEdit: Bool { !busy && (revision ?? 0) > 0 && managementScopes.contains("profile.edit") }
    var canReply: Bool { !busy && managementScopes.contains("reply.official") }

    func clearPrivateState() {
        generation = UUID()
        claims = []
        selected = nil
        ready = false
        busy = false
        signInRequired = false
        proofUnavailable = false
        notice = nil
        claimKey = Self.newId()
        nextClaimOperation = nil
        clearProof()
        clearManagement()
    }

    private func clearManagement() {
        managementScopes = []
        revision = nil
        fields = [:]
        original = [:]
        reply = ""
        editAttempt = nil
        replyAttempt = nil
        refreshRequired = false
    }

    private func clearProof() {
        prepared = nil
        preparedBytes = nil
    }

    private static func newId() -> String { UUID().uuidString.lowercased() }

    func select(_ claim: StationRepresentationClaim) {
        guard !busy, claims.contains(claim) else { return }
        clearProof()
        selected = claim
        notice = nil
    }

    func refresh() async {
        await perform { request in
            let records = try await request.client.stationClaims(stationId: stationId, expectedScope: request.scope)
            try check(request)
            let old = selected
            claims = records
            selected = records.first { $0.id == old?.id }
            if selected?.declarationId != old?.declarationId { clearProof() }
            ready = true
        }
    }

    func openClaim() async {
        guard ready else { return }
        await perform { request in
            let record = try await request.client.openStationClaim(
                stationId: stationId, role: role, scopes: ["profile.edit", "reply.official"], clientKey: claimKey, expectedScope: request.scope
            )
            try check(request)
            clearProof()
            accept(record)
        }
    }

    func reissue() async {
        guard let selected, selected.canSupplyProof else { return }
        let operation = claimOperation("reissue", selected)
        await perform { request in
            let latest = try await request.client.stationClaim(id: selected.id, expectedScope: request.scope)
            try check(request)
            clearProof()
            if latest.declarationId != selected.declarationId {
                accept(latest)
                nextClaimOperation = nil
                return
            }
            guard latest.canSupplyProof else { throw StationRepresentationError.invalidInput }
            let record = try await request.client.reissueStationClaim(id: latest.id, operationId: operation, expectedScope: request.scope)
            try check(request)
            accept(record)
            nextClaimOperation = nil
        }
    }

    func cancel() async {
        guard let selected, selected.state?.mayCancel == true else { return }
        let operation = claimOperation("cancel", selected)
        await perform { request in
            try await request.client.cancelStationClaim(id: selected.id, operationId: operation, expectedScope: request.scope)
            try check(request)
            claims.removeAll { $0.id == selected.id }
            self.selected = nil
            clearProof()
            claimKey = Self.newId()
            nextClaimOperation = nil
            notice = String(localized: "android.station_claim_cancelled")
        }
    }

    func declarationFile() async -> StationDeclarationFile? {
        guard let selected else { return nil }
        var file: StationDeclarationFile?
        await perform { request in
            let confirmed = try await fresh(selected, request: request)
            file = StationDeclarationFile(bytes: try StationRepresentationRules.declarationData(confirmed.declaration))
        }
        return file
    }

    func importTarget() async -> StationRepresentationClaim? {
        guard !proofUnavailable, let selected else { return nil }
        var result: StationRepresentationClaim?
        await perform { request in
            result = try await fresh(selected, request: request)
        }
        return result
    }

    func readProof(_ url: URL, target: StationRepresentationClaim) async {
        guard !proofUnavailable else { return }
        clearProof()
        await perform { request in
            let confirmed = try await fresh(target, request: request)
            let bytes: Data
            do {
                bytes = try await Task.detached(priority: .userInitiated) {
                    let accessed = url.startAccessingSecurityScopedResource()
                    defer { if accessed { url.stopAccessingSecurityScopedResource() } }
                    guard try url.resourceValues(forKeys: [.contentTypeKey]).contentType?.conforms(to: .pdf) == true else {
                        throw StationRepresentationError.invalidInput
                    }
                    return try StationRepresentationRules.readPdf(from: url)
                }.value
            } catch {
                try check(request)
                notice = String(localized: "android.station_claim_file_unavailable")
                return
            }
            try check(request)
            prepared = PreparedProof(claim: confirmed, bytes: bytes, operationId: Self.newId())
            preparedBytes = bytes.count
            notice = String(localized: "android.station_claim_file_ready")
        }
    }

    func submitProof() async {
        guard !proofUnavailable, let prepared else { return }
        await perform { request in
            _ = try await request.client.submitStationClaimProof(
                claim: prepared.claim, pdf: prepared.bytes, operationId: prepared.operationId, expectedScope: request.scope
            )
            try check(request)
            clearProof()
            notice = String(localized: "android.station_claim_received")
        }
    }

    func documentExported(_ result: Result<URL, Error>) {
        guard ready, viewModel.currentAccountIdOrNil != nil else { return }
        switch result {
        case .success:
            notice = String(localized: "android.station_claim_file_saved")
        case .failure:
            notice = String(localized: "android.station_claim_file_unavailable")
        }
    }

    func refreshManagement() async {
        refreshRequired = false
        await perform { request in
            let owned = try await request.client.stationClaims(stationId: stationId, expectedScope: request.scope)
            let profile = try await viewModel.socialReadClient().fetchStationBusinessProfile(stationId: stationId)
            try check(request)
            guard let profile else { throw StationRepresentationError.invalidResponse }
            managementScopes = Set(owned.filter { $0.state == .approved }.flatMap(\.scopes))
            updateBusinessProfile(profile)
        }
    }

    func save() async {
        guard canEdit, let revision, StationRepresentationRules.validateBusinessFields(changed) else { return }
        let updates = changed
        let previous = editAttempt
        let id = previous?.revision == revision && previous?.fields == updates ? previous?.id ?? Self.newId() : Self.newId()
        editAttempt = (revision, updates, id)
        await perform { request in
            _ = try await request.client.editStationBusinessProfile(
                stationId: stationId, revision: revision, fields: updates, operationId: id, expectedScope: request.scope
            )
            let profile = try await viewModel.socialReadClient().fetchStationBusinessProfile(stationId: stationId)
            try check(request)
            if let profile {
                updateBusinessProfile(profile)
                notice = String(localized: "android.station_management_saved")
            } else {
                managementScopes = []
                notice = String(localized: "android.station_management_changed")
            }
            editAttempt = nil
        }
    }

    func sendReply() async {
        guard canReply, !refreshRequired, PortableText.isValidComment(reply) else { return }
        let draft = PortableText.normalize(reply)
        let previous = replyAttempt
        let id = previous?.fuel == fuel && previous?.text == draft ? previous?.id ?? Self.newId() : Self.newId()
        replyAttempt = (fuel, draft, id)
        await perform { request in
            do {
                _ = try await request.client.replyAsStation(
                    stationId: stationId, product: fuel, text: draft, operationId: id, expectedScope: request.scope
                )
            } catch {
                let failure = StationRepresentationError.from(error)
                switch failure {
                case .offline, .unavailable, .invalidResponse, .refused(status: 503, code: _, retryAfterSeconds: _):
                    refreshRequired = true
                default: break
                }
                throw failure
            }
            try check(request)
            reply = ""
            replyAttempt = nil
            notice = String(localized: "android.station_management_reply_sent")
        }
    }

    private func updateBusinessProfile(_ profile: StationBusinessProfile) {
        fields = profile.business
        original = profile.business
        revision = profile.revision
    }

    private func accept(_ record: StationRepresentationClaim) {
        claims.removeAll { $0.id == record.id }
        claims.insert(record, at: 0)
        selected = record
    }

    private func claimOperation(_ action: String, _ record: StationRepresentationClaim) -> String {
        if let last = nextClaimOperation, last.action == action, last.declaration == record.declarationId { return last.id }
        let id = Self.newId()
        nextClaimOperation = (action, record.declarationId, id)
        return id
    }

    private func fresh(_ target: StationRepresentationClaim, request: Request) async throws -> StationRepresentationClaim {
        let record = try await request.client.stationClaim(id: target.id, expectedScope: request.scope)
        try check(request)
        guard selected?.id == target.id, record.stationId == stationId,
              record.declarationId == target.declarationId, record.canSupplyProof(at: Date()) else {
            clearProof()
            throw StationRepresentationError.invalidInput
        }
        return record
    }

    private func perform(_ action: (Request) async throws -> Void) async {
        guard !busy else { return }
        let client = viewModel.communityGateway
        guard viewModel.currentAccountIdOrNil != nil, let scope = client.transport.ownerScope() else {
            clearPrivateState()
            signInRequired = true
            return
        }
        guard !viewModel.isOfflineSimulation else {
            clearPrivateState()
            notice = String(localized: "android.station_profile_unavailable")
            return
        }
        let request = Request(client: client, scope: scope, origin: client.transport.origin, generation: generation)
        busy = true
        notice = nil
        defer { if generation == request.generation { busy = false } }
        do {
            try await action(request)
        } catch {
            guard (try? check(request)) != nil else { return }
            let failure = StationRepresentationError.from(error)
            if failure.integrationPending {
                clearPrivateState()
                notice = String(localized: "station.owner.integration_pending")
                return
            }
            if failure.proofStorageUnavailable {
                clearProof()
                proofUnavailable = true
                notice = String(localized: "station.owner.proof_unavailable")
                return
            }
            switch failure {
            case .signInRequired, .refused(status: 401, code: _, retryAfterSeconds: _):
                clearPrivateState()
                signInRequired = true
                notice = String(localized: "android.station_claim_sign_in")
            case .refused(status: 403, code: _, retryAfterSeconds: _), .refused(status: 404, code: _, retryAfterSeconds: _):
                clearPrivateState()
                notice = String(localized: "android.station_claim_refused")
            case .refused(status: 409, code: "profile.stale", retryAfterSeconds: _):
                managementScopes = []
                notice = String(localized: "android.station_management_changed")
            case .refused(status: 409, code: "claim.busy-retry", retryAfterSeconds: _):
                notice = String(localized: "station.owner.claim_busy")
            case .refused(status: 429, code: _, retryAfterSeconds: _):
                notice = String(localized: "station.owner.claim_quota")
            case .invalidInput, .refused(status: 400, code: _, retryAfterSeconds: _),
                 .refused(status: 409, code: _, retryAfterSeconds: _):
                clearProof()
                notice = String(localized: "android.station_claim_invalid")
            default:
                managementScopes = []
                notice = String(localized: "android.station_management_unavailable")
            }
        }
    }

    private func check(_ request: Request) throws {
        guard !Task.isCancelled, generation == request.generation, !viewModel.isOfflineSimulation,
              viewModel.snapshot.apiEnvironment.origin == request.origin, viewModel.currentAccountIdOrNil != nil,
              viewModel.communityGateway.transport.ownerScope() == request.scope else {
            throw StationRepresentationError.ownerChanged
        }
    }
}

private struct StationRepresentationSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @StateObject private var model: StationRepresentationModel
    @State private var showSignIn = false
    @State private var showManagement = false
    @State private var importing = false
    @State private var exporting = false
    @State private var importClaim: StationRepresentationClaim?
    @State private var exportFile: StationDeclarationFile?
    @State private var confirmCancel = false

    init(viewModel: AppStateViewModel, stationId: String, fuel: FuelProduct) {
        self.viewModel = viewModel
        self._model = StateObject(wrappedValue: StationRepresentationModel(viewModel: viewModel, stationId: stationId, fuel: fuel))
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(String(localized: "android.station_claim_explanation"))
                    if model.busy { ProgressView() }
                    if let notice = model.notice { Text(notice).foregroundStyle(.secondary) }
                    if model.signInRequired {
                        Button(String(localized: "android.station_claim_login")) { showSignIn = true }
                    } else {
                        Button(String(localized: "android.station_claim_refresh")) { Task { await model.refresh() } }
                            .disabled(model.busy)
                    }
                }
                if model.ready {
                    Section {
                        Picker(String(localized: "android.station_claim_role"), selection: $model.role) {
                            ForEach(StationClaimRole.allCases) { role in
                                Text(role.title).tag(role)
                            }
                        }
                        Text(String(localized: "android.station_claim_scopes")).font(.caption)
                        Button(String(localized: "android.station_claim_open")) { Task { await model.openClaim() } }
                    }
                    .disabled(model.busy)

                    if !model.claims.isEmpty {
                        Section(String(localized: "station.owner.my_claims")) {
                            ForEach(model.claims) { claim in
                                Button {
                                    model.select(claim)
                                } label: {
                                    if model.selected?.id == claim.id {
                                        Label(claim.state.title, systemImage: "checkmark.circle")
                                    } else {
                                        Text(claim.state.title)
                                    }
                                }
                            }
                        }
                        .disabled(model.busy)
                    }
                    if let claim = model.selected {
                        claimDetails(claim)
                    }
                }
            }
            .navigationTitle(String(localized: "android.station_claim_title"))
            .navigationDestination(isPresented: $showManagement) { StationManagementView(model: model) }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
        .task(id: model.identityKey) {
            model.clearPrivateState()
            importClaim = nil
            exportFile = nil
            importing = false
            exporting = false
            showManagement = false
            await model.refresh()
        }
        .onDisappear { model.clearPrivateState() }
        .sheet(isPresented: $showSignIn) { AuthSheetView(viewModel: viewModel) }
        .fileImporter(isPresented: $importing, allowedContentTypes: [.pdf]) { result in
            guard let target = importClaim else { return }
            importClaim = nil
            if case .success(let url) = result { Task { await model.readProof(url, target: target) } }
        }
        .fileExporter(isPresented: $exporting, document: exportFile, contentType: .plainText, defaultFilename: "declaration.txt") { result in
            model.documentExported(result)
            exportFile = nil
        }
        .confirmationDialog(String(localized: "android.station_claim_cancel_question"), isPresented: $confirmCancel, titleVisibility: .visible) {
            Button(String(localized: "android.station_claim_cancel_confirm"), role: .destructive) { Task { await model.cancel() } }
            Button(String(localized: "android.action_back"), role: .cancel) {}
        }
    }

    private func claimDetails(_ claim: StationRepresentationClaim) -> some View {
        Section(claim.state.title) {
            if claim.canSupplyProof(at: Date()) && !model.proofUnavailable {
                Text(String(localized: "android.station_claim_privacy"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(String(localized: "android.station_claim_export_help"))
                Text(verbatim: claim.declaration)
                    .font(.footnote.monospaced())
                    .textSelection(.enabled)
                Button(String(localized: "android.station_claim_export")) {
                    Task {
                        exportFile = await model.declarationFile()
                        exporting = exportFile != nil
                    }
                }
                Button(String(localized: "android.station_claim_pick")) {
                    Task {
                        importClaim = await model.importTarget()
                        importing = importClaim != nil
                    }
                }
                if let bytes = model.preparedBytes {
                    Text(String(localized: "android.station_claim_ready_bytes", defaultValue: "Selected PDF: \(bytes) bytes. Confirm before sending."))
                        .foregroundStyle(.secondary)
                    Button(String(localized: "android.station_claim_send")) { Task { await model.submitProof() } }
                }
            }
            if claim.canSupplyProof {
                Button(String(localized: "android.station_claim_reissue")) { Task { await model.reissue() } }
            }
            if claim.state == .approved {
                Button(String(localized: "android.station_profile_claim")) { showManagement = true }
            }
            if claim.state?.mayCancel == true {
                Button(String(localized: "android.station_claim_cancel"), role: .destructive) { confirmCancel = true }
            }
        }
        .disabled(model.busy)
    }
}

private struct StationManagementView: View {
    @ObservedObject var model: StationRepresentationModel

    var body: some View {
        Form {
            Section {
                Text(String(localized: "android.station_management_server"))
                Text(String(localized: "android.station_management_canonical")).font(.caption)
                if model.busy { ProgressView() }
                if let notice = model.notice { Text(notice).foregroundStyle(.secondary) }
                if !model.busy && model.managementScopes.isEmpty {
                    Text(String(localized: "android.station_management_no_scope")).foregroundStyle(.secondary)
                }
                Button(String(localized: "android.action_retry")) { Task { await model.refreshManagement() } }
                    .disabled(model.busy)
            }
            if model.managementScopes.contains("profile.edit") {
                Section(String(localized: "android.station_profile_title")) {
                    ForEach(StationBusinessField.allCases) { field in
                        TextField(field.title, text: fieldBinding(field), axis: .vertical)
                            .lineLimit(1...5)
                    }
                    if !model.changed.isEmpty && !StationRepresentationRules.validateBusinessFields(model.changed) {
                        Text(String(localized: "android.station_management_invalid")).font(.caption).foregroundStyle(.secondary)
                    }
                    Button(String(localized: "android.station_management_save")) { Task { await model.save() } }
                        .disabled(!StationRepresentationRules.validateBusinessFields(model.changed))
                }
                .disabled(!model.canEdit)
            }
            if model.managementScopes.contains("reply.official") {
                Section(String(localized: "android.station_management_reply")) {
                    Picker(String(localized: "android.map_filter_fuel"), selection: $model.fuel) {
                        ForEach(FuelProduct.allCases) { fuel in Text(fuel.localizedName).tag(fuel) }
                    }
                    TextField(String(localized: "android.station_management_reply_text"), text: $model.reply, axis: .vertical)
                        .lineLimit(2...5)
                    let count = PortableText.countScalars(PortableText.normalize(model.reply))
                    Text(String(localized: "android.station_management_reply_count", defaultValue: "\(count) / 280 characters"))
                        .font(.caption).foregroundStyle(.secondary)
                    Button(String(localized: "android.station_management_publish")) { Task { await model.sendReply() } }
                        .disabled(!PortableText.isValidComment(model.reply))
                }
                .disabled(!model.canReply)
            }
        }
        .navigationTitle(String(localized: "android.station_management_title"))
        .task { await model.refreshManagement() }
    }

    private func fieldBinding(_ field: StationBusinessField) -> Binding<String> {
        Binding(
            get: { model.fields[field] ?? "" },
            set: { if $0.utf16.count <= 1_024 { model.fields[field] = $0 } }
        )
    }
}

private struct StationDeclarationFile: FileDocument {
    static var readableContentTypes: [UTType] { [.plainText] }
    let bytes: Data

    init(bytes: Data) {
        self.bytes = bytes
    }

    init(configuration: ReadConfiguration) throws {
        guard let bytes = configuration.file.regularFileContents, bytes.count <= 8_192 * 4 else {
            throw StationRepresentationError.invalidInput
        }
        self.bytes = bytes
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        FileWrapper(regularFileWithContents: bytes)
    }
}

private extension StationBusinessField {
    var title: String {
        switch self {
        case .openingHours: return String(localized: "android.station_profile_hours")
        case .services: return String(localized: "android.station_profile_services")
        case .phone: return String(localized: "android.station_profile_phone")
        case .website: return String(localized: "android.station_profile_website")
        case .description: return String(localized: "android.station_profile_description")
        }
    }
}

private extension StationClaimRole {
    var title: String {
        switch self {
        case .administrator: return String(localized: "android.station_claim_administrator")
        case .manager: return String(localized: "android.station_claim_manager")
        }
    }
}

private extension Optional where Wrapped == StationClaimState {
    var title: String {
        switch self {
        case .draft?, .awaitingProof?: return String(localized: "android.station_claim_draft")
        case .checking?, .inReview?: return String(localized: "android.station_claim_review")
        case .needsInformation?: return String(localized: "android.station_claim_needs_info")
        case .approved?: return String(localized: "android.station_claim_approved")
        case .denied?: return String(localized: "android.station_claim_denied")
        case .cancelled?: return String(localized: "android.station_claim_cancelled")
        case .expired?: return String(localized: "android.station_claim_expired")
        case nil: return String(localized: "android.station_claim_unknown")
        }
    }
}
