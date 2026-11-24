import SwiftUI
import AnpFuelCore

struct QualityReportTarget {
    let stationId: String
    let fuel: FuelProduct
    var revision: String? = nil
    var sample: String? = nil
}

@MainActor
struct QualityReportHistoryView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    var body: some View {
        NavigationStack {
            QualityReportScreen(viewModel: viewModel, target: nil)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
                    }
                }
        }
    }
}

@MainActor
struct QualityReportScreen: View {
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.openURL) private var openURL
    @ObservedObject var viewModel: AppStateViewModel
    let target: QualityReportTarget?
    @State private var reports: [QualityReport] = []
    @State private var cursor: String?
    @State private var selected: QualityReport?
    @State private var openedDay = QualityReportCommand.dayUTC(Date())
    @State private var symptom: QualityReportSymptom = .other
    @State private var reportText = ""
    @State private var confirmation: QualityReportCommand?
    @State private var showConfirmation = false
    @State private var showAnp = false
    @State private var pending = false
    @State private var retryAt: Date?
    @State private var busy = false
    @State private var hasLoaded = false
    @State private var received = false
    @State private var error: QualityReportError?
    @State private var browserFailed = false
    @State private var ticket = UUID()
    @State private var stateScope: QualityReportScope?

    private struct Input: Hashable {
        let scope: QualityReportScope?
        let active: Bool
        let offline: Bool
    }

    private var input: Input {
        Input(scope: viewModel.qualityReportScope, active: scenePhase == .active, offline: viewModel.isOfflineSimulation)
    }

    var body: some View {
        List {
            Section {
                Text(String(localized: "android.quality_reports_note"))
                    .foregroundStyle(.secondary)
            }
            if input.active {
                if let scope = input.scope {
                    if stateScope == scope {
                        ownerSections(scope: scope)
                    } else {
                        ProgressView()
                    }
                } else {
                    Button(String(localized: "android.quality_reports_login")) { viewModel.activeSheet = .auth }
                }
            }
            Section {
                Button(String(localized: "android.quality_reports_anp")) { showAnp = true }
                if browserFailed {
                    Text(String(localized: "android.quality_reports_browser_failed"))
                }
            }
        }
        .privacySensitive()
        .navigationTitle(target == nil ? String(localized: "quality.reports.history") : String(localized: "android.quality_reports_title"))
        .task(id: input) { await resume() }
        .confirmationDialog(String(localized: "android.quality_reports_confirm_title"), isPresented: $showConfirmation) {
            if let command = confirmation, let scope = input.scope, input.active, stateScope == scope {
                Button(String(localized: "android.quality_reports_confirm"), role: command.action == .withdraw ? .destructive : nil) {
                    Task { await confirm(command, scope: scope) }
                }
            }
            Button(String(localized: "android.action_cancel"), role: .cancel) { confirmation = nil }
        } message: {
            if let confirmation { Text(confirmationMessage(confirmation.action)) }
        }
        .confirmationDialog(String(localized: "android.quality_reports_anp"), isPresented: $showAnp) {
            Button(String(localized: "android.quality_reports_open")) { openAnp() }
            Button(String(localized: "android.action_cancel"), role: .cancel) {}
        } message: {
            Text(String(localized: "android.quality_reports_anp_handoff"))
        }
    }

    @ViewBuilder
    private func ownerSections(scope: QualityReportScope) -> some View {
        if busy { ProgressView() }
        if received { Text(String(localized: "android.quality_reports_received")) }
        if let error { Text(errorLabel(error)).foregroundStyle(.secondary) }
        if pending {
            Section {
                Text(String(localized: "android.quality_reports_pending"))
                if let retryAt {
                    TimelineView(.periodic(from: .now, by: 1)) { context in
                        Button(String(localized: "android.action_retry")) { Task { await sendPending(scope: scope) } }
                            .disabled(busy || input.offline || retryAt > context.date)
                    }
                }
            }
        }
        if target != nil || selected != nil {
            formSection.disabled(busy || pending || input.offline)
        }
        Section(String(localized: "quality.reports.history")) {
            Button(String(localized: "android.quality_reports_refresh")) { Task { await readPage(scope: scope, after: nil) } }
                .disabled(busy || input.offline)
            if reports.isEmpty && hasLoaded {
                Text(String(localized: "android.quality_reports_empty"))
                if target == nil {
                    Text(String(localized: "android.quality_reports_empty_how")).foregroundStyle(.secondary)
                    NavigationLink {
                        QualityReportStationPicker(viewModel: viewModel)
                    } label: {
                        Text(String(localized: "android.quality_reports_choose_station"))
                    }
                } else {
                    Text(String(localized: "android.quality_reports_empty_here")).foregroundStyle(.secondary)
                }
            }
            ForEach(reports) { report in
                Button {
                    Task { await select(report.id, scope: scope) }
                } label: {
                    VStack(alignment: .leading, spacing: 5) {
                        Text(statusLabel(report.status)).font(.headline)
                        if let product = FuelProduct.fromWireCode(report.fuel) { Text(product.localizedName) }
                        Text(QualityDisplay.date(report.occurredOn)).font(.caption)
                        if !report.text.isEmpty { Text(report.text).lineLimit(3) }
                    }
                }
                .disabled(busy || pending || input.offline)
            }
            if let cursor {
                Button(String(localized: "android.quality_reports_more")) { Task { await readPage(scope: scope, after: cursor) } }
                    .disabled(busy || input.offline)
            }
        }
    }

    private var formSection: some View {
        let editable = selected.map { Self.editableStatuses.contains($0.status) } ?? true
        let canAppeal = selected.map { Self.appealableStatuses.contains($0.status) } ?? false
        let canWithdraw = selected.map { Self.withdrawableStatuses.contains($0.status) } ?? false
        let kind: QualityReportKind? = selected?.kind ?? target.map { $0.sample == nil ? .experience : .linkReview }
        return Section {
            if let kind {
                Text(kind == .experience ? String(localized: "android.quality_reports_experience") : String(localized: "android.quality_reports_link"))
                    .font(.headline)
            }
            if let selected {
                Text(statusLabel(selected.status))
                Text(String(localized: "android.quality_reports_claim")).foregroundStyle(.secondary)
                if let reason = selected.reviewReason { Text(reasonLabel(reason)).foregroundStyle(.secondary) }
            }
            if editable {
                TextField(String(localized: "android.quality_reports_day"), text: $openedDay)
                    .keyboardType(.numbersAndPunctuation)
                Picker(String(localized: "android.quality_reports_experience"), selection: $symptom) {
                    ForEach(QualityReportSymptom.allCases) { value in Text(symptomLabel(value)).tag(value) }
                }
            }
            if editable || canAppeal {
                TextField(String(localized: "android.quality_reports_text"), text: $reportText, axis: .vertical)
                    .lineLimit(3...8)
                Text(String(localized: "android.quality_reports_text_limit")).font(.caption).foregroundStyle(.secondary)
            }
            if editable {
                Button(selected == nil ? String(localized: "android.quality_reports_submit") : String(localized: "android.quality_reports_edit")) {
                    request(selected == nil ? .submit : .edit)
                }
            }
            if canAppeal {
                Button(String(localized: "android.quality_reports_appeal")) { request(.appeal) }
            }
            if canWithdraw {
                Button(String(localized: "android.quality_reports_withdraw"), role: .destructive) { request(.withdraw) }
            }
            if selected != nil && target != nil {
                Button(String(localized: "android.quality_reports_new")) { resetForm() }
            }
        }
    }

    private static let editableStatuses = Set(["PENDING_REVIEW", "REVIEWED"])
    private static let appealableStatuses = Set(["HIDDEN", "REVIEWED"])
    private static let withdrawableStatuses = editableStatuses.union(appealableStatuses)

    private func resetForm() {
        selected = nil
        openedDay = QualityReportCommand.dayUTC(Date())
        symptom = .other
        reportText = ""
        confirmation = nil
        showConfirmation = false
    }

    private func resume() async {
        ticket = UUID()
        stateScope = nil
        busy = false
        reports = []
        cursor = nil
        hasLoaded = false
        error = nil
        pending = false
        received = false
        retryAt = nil
        resetForm()
        guard let scope = input.scope, input.active else { return }
        stateScope = scope
        updatePending(scope)
        if input.offline {
            error = .unavailable(retryAfterSeconds: nil)
        } else {
            await readPage(scope: scope, after: nil)
        }
    }

    private func updatePending(_ scope: QualityReportScope) {
        do {
            pending = try viewModel.qualityReportJournal.pending(scope: scope) != nil
            retryAt = try viewModel.qualityReportJournal.retryAvailableAt(scope: scope)
        } catch {
            pending = true
            retryAt = nil
            self.error = .unavailable(retryAfterSeconds: nil)
        }
    }

    private func start(_ scope: QualityReportScope) -> UUID? {
        guard !busy, isCurrent(scope: scope, operation: ticket) else { return nil }
        ticket = UUID()
        busy = true
        error = nil
        return ticket
    }

    private func isCurrent(scope: QualityReportScope, operation: UUID) -> Bool {
        !Task.isCancelled && ticket == operation && stateScope == scope && input.scope == scope && input.active && !input.offline
    }

    private func readPage(scope: QualityReportScope, after: String?, preserveReceipt: Bool = false) async {
        guard let operation = start(scope) else { return }
        do {
            let page = try await viewModel.communityGateway.qualityReports(after: after, scope: scope)
            guard isCurrent(scope: scope, operation: operation) else { return }
            let incoming = page.items.filter { target == nil || $0.stationId == target?.stationId }
            if after == nil {
                reports = incoming
                if !preserveReceipt { received = false }
            } else {
                let ids = Set(reports.map(\.id))
                reports.append(contentsOf: incoming.filter { !ids.contains($0.id) })
            }
            cursor = page.nextAfterId
            hasLoaded = true
        } catch {
            guard isCurrent(scope: scope, operation: operation) else { return }
            self.error = error as? QualityReportError ?? .unavailable(retryAfterSeconds: nil)
        }
        if isCurrent(scope: scope, operation: operation) { busy = false }
    }

    private func select(_ id: String, scope: QualityReportScope) async {
        guard !pending, let operation = start(scope) else { return }
        do {
            let report = try await viewModel.communityGateway.qualityReport(id: id, scope: scope)
            guard isCurrent(scope: scope, operation: operation) else { return }
            selected = report
            openedDay = report.occurredOn
            symptom = report.symptom
            reportText = report.text
        } catch {
            guard isCurrent(scope: scope, operation: operation) else { return }
            self.error = error as? QualityReportError ?? .unavailable(retryAfterSeconds: nil)
        }
        if isCurrent(scope: scope, operation: operation) { busy = false }
    }

    private func request(_ action: QualityReportAction) {
        let id = selected?.id ?? UUID().uuidString.lowercased()
        let revision = selected?.revision ?? 0
        let command: QualityReportCommand
        switch action {
        case .withdraw, .appeal:
            command = QualityReportCommand(reportId: id, action: action, expectedRevision: revision,
                                           text: action == .appeal ? reportText : nil)
        case .submit, .edit:
            let kind: QualityReportKind = selected?.kind ?? (target?.sample == nil ? .experience : .linkReview)
            command = QualityReportCommand(
                reportId: id, action: action, expectedRevision: revision, kind: kind,
                stationId: selected?.stationId ?? target?.stationId, fuel: selected?.fuel ?? target?.fuel.wireCode,
                occurredOn: openedDay, symptom: symptom, text: reportText,
                officialRevisionId: selected == nil ? target?.revision : selected?.officialRevisionId,
                sampleRef: selected == nil ? target?.sample : selected?.sampleRef
            )
        }
        do {
            try command.validate()
            confirmation = command
            showConfirmation = true
            error = nil
        } catch {
            self.error = .invalid
        }
    }

    private func confirm(_ command: QualityReportCommand, scope: QualityReportScope) async {
        guard !busy, !pending, isCurrent(scope: scope, operation: ticket) else { return }
        do {
            try viewModel.qualityReportJournal.freeze(command, scope: scope)
        } catch {
            self.error = error as? QualityReportError ?? .unavailable(retryAfterSeconds: nil)
            return
        }
        resetForm()
        await sendPending(scope: scope)
    }

    private func sendPending(scope: QualityReportScope) async {
        guard let operation = start(scope) else { return }
        do {
            let receipt = try await viewModel.qualityReportJournal.dispatch(scope: scope, client: viewModel.communityGateway)
            guard isCurrent(scope: scope, operation: operation) else { return }
            received = receipt != nil
        } catch {
            guard isCurrent(scope: scope, operation: operation) else { return }
            self.error = error as? QualityReportError ?? .unavailable(retryAfterSeconds: nil)
        }
        updatePending(scope)
        busy = false
        if received { await readPage(scope: scope, after: nil, preserveReceipt: true) }
    }

    private func openAnp() {
        guard let url = URL(string: "https://www.gov.br/anp/pt-br/canais_atendimento/ouvidoria") else { return }
        openURL(url) { accepted in browserFailed = !accepted }
    }

    private func confirmationMessage(_ action: QualityReportAction) -> String {
        switch action {
        case .withdraw: return String(localized: "android.quality_reports_confirm_withdraw")
        case .appeal: return String(localized: "android.quality_reports_confirm_appeal")
        case .submit, .edit: return String(localized: "android.quality_reports_confirm_submit")
        }
    }

    private func symptomLabel(_ symptom: QualityReportSymptom) -> String {
        switch symptom {
        case .starting: return String(localized: "android.quality_reports_starting")
        case .performance: return String(localized: "android.quality_reports_performance")
        case .consumption: return String(localized: "android.quality_reports_consumption")
        case .other: return String(localized: "android.quality_reports_other")
        }
    }

    private func statusLabel(_ status: String) -> String {
        switch status {
        case "PENDING_REVIEW": return String(localized: "android.quality_reports_status_pending")
        case "REVIEWED": return String(localized: "android.quality_reports_status_reviewed")
        case "HIDDEN": return String(localized: "android.quality_reports_status_hidden")
        case "WITHDRAWN": return String(localized: "android.quality_reports_status_withdrawn")
        default: return String(localized: "android.quality_reports_status_unknown")
        }
    }

    private func reasonLabel(_ reason: String) -> String {
        switch reason {
        case "TRIAGE": return String(localized: "android.quality_reports_reason_triage")
        case "CLAIM_RECORDED": return String(localized: "android.quality_reports_reason_recorded")
        case "DUPLICATE": return String(localized: "android.quality_reports_reason_duplicate")
        case "INSUFFICIENT_EVIDENCE": return String(localized: "android.quality_reports_reason_insufficient")
        case "UNSUPPORTED_LINK": return String(localized: "android.quality_reports_reason_link")
        case "ABUSE": return String(localized: "android.quality_reports_reason_abuse")
        case "PRIVACY": return String(localized: "android.quality_reports_reason_privacy")
        default: return String(localized: "android.quality_reports_reason_unknown")
        }
    }

    private func errorLabel(_ error: QualityReportError) -> String {
        switch error {
        case .invalid: return String(localized: "android.quality_reports_invalid")
        case .authRequired, .ownerChanged: return String(localized: "android.quality_reports_account")
        case .conflict: return String(localized: "android.quality_reports_conflict")
        case .notFound: return String(localized: "android.quality_reports_missing")
        case .quota: return String(localized: "android.quality_reports_quota")
        case .unavailable: return String(localized: "android.quality_reports_failed")
        }
    }
}

@MainActor
private struct QualityReportStationPicker: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var selected: StationRecord?

    var body: some View {
        List(viewModel.stations) { station in
            Button(station.name) { selected = station }
        }
        .navigationTitle(String(localized: "android.quality_reports_choose_station"))
        .sheet(item: $selected) { station in StationDetailSheetView(viewModel: viewModel, station: station) }
    }
}
