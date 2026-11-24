import AnpFuelCore
import CoreTransferable
import SwiftUI
import UniformTypeIdentifiers

private struct ServerLedgerCsvFile: Transferable {
    let document: Data

    static var transferRepresentation: some TransferRepresentation {
        DataRepresentation(exportedContentType: .commaSeparatedText) { (file: Self) in file.document }
            .suggestedFileName("abastevo-server-ledger.csv")
    }
}

@MainActor
struct LedgerServerAnalysisView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator
    @State private var isComparing = false
    @State private var selectedDay = Date()
    @State private var baselineDay = Calendar.current.date(byAdding: .month, value: -1, to: Date()) ?? Date()
    @State private var currentVehicle: String?
    @State private var baselineVehicle: String?
    @State private var report: LedgerServerReport?
    @State private var comparison: LedgerServerComparison?
    @State private var csv: Data?
    @State private var loadedQuery: Query?
    @State private var task: Task<Void, Never>?
    @State private var generation = UUID()
    @State private var notice: String?
    @State private var retryAt: Date?
    @State private var pending: PendingAction?

    private enum Action: Equatable {
        case get, build, csv, compare

        var paid: Bool { self == .build || self == .csv }
    }

    private struct Query: Equatable {
        let scope: AccountDataExportScope?
        let period: LedgerReportPeriod?
        let baseline: LedgerComparisonSide?
        let current: LedgerComparisonSide?
        let comparing: Bool
    }

    private struct PendingAction {
        let action: Action
        let query: Query
        let operationId: String
    }

    init(viewModel: AppStateViewModel, ledger: LedgerCoordinator) {
        self.viewModel = viewModel
        self.ledger = ledger
    }

    private var query: Query {
        let available = Set(viewModel.snapshot.vehicles.map(\.id))
        let selectedPeriod = period(selectedDay)
        let baseline = baselineVehicle.flatMap { id in
            period(baselineDay).flatMap { window in
                available.contains(id) ? LedgerComparisonSide(vehicleId: id, period: window) : nil
            }
        }
        let current = currentVehicle.flatMap { id in
            selectedPeriod.flatMap { window in
                available.contains(id) ? LedgerComparisonSide(vehicleId: id, period: window) : nil
            }
        }
        return Query(scope: viewModel.accountDataExportScope, period: selectedPeriod,
                     baseline: baseline, current: current, comparing: isComparing)
    }

    private var paid: Bool {
        guard query.scope != nil, let scope = ledger.currentScope,
              let observation = ledger.syncState?.premiumObservation,
              observation.accountId == scope.accountId, observation.origin == scope.origin,
              observation.grants("professional.write") else { return false }
        let now = LedgerCoordinator.nowMillis()
        let age = now - observation.observedAtMillis
        return (0...900_000).contains(age) && (observation.validUntilMillis.map { now < $0 } ?? true)
    }

    var body: some View {
        List {
            if query.scope == nil {
                Text(String(localized: "ledger.sync.sign_in"))
            } else {
                Picker(String(localized: "ledger.server.analysis"), selection: $isComparing) {
                    Text(String(localized: "ledger.server.reports")).tag(false)
                    Text(String(localized: "ledger.server.compare")).tag(true)
                }
                .pickerStyle(.segmented)

                if isComparing { comparisonForm } else { reportForm }
                if task != nil { ProgressView() }
                if let notice { Text(notice).foregroundStyle(.secondary) }
                if loadedQuery == query, isComparing, let comparison { comparisonResults(comparison) }
                if loadedQuery == query, !isComparing, let report { reportResults(report) }
                if loadedQuery == query, !isComparing, let csv {
                    ShareLink(item: ServerLedgerCsvFile(document: csv), preview: SharePreview(String(localized: "ledger.server.csv"))) {
                        Label(String(localized: "ledger.server.csv"), systemImage: "square.and.arrow.up")
                    }
                }
            }
        }
        .navigationTitle(String(localized: "ledger.server.analysis"))
        .task(id: viewModel.accountDataExportScope?.ownerScope) {
            if viewModel.accountDataExportScope != nil { await ledger.refreshPremiumProof() }
        }
        .onChange(of: query) { _, _ in clear() }
        .onDisappear { clear() }
    }

    private var reportForm: some View {
        Section {
            DatePicker(String(localized: "ledger.server.month"), selection: $selectedDay, displayedComponents: .date)
            if let period = query.period, let date = period.interval?.start {
                Text(date.formatted(.dateTime.year().month(.wide)))
                    .foregroundStyle(.secondary)
            }
            actionButton(String(localized: "ledger.server.read"), action: .get, valid: query.period != nil)
            actionButton(String(localized: "ledger.server.build"), action: .build, valid: query.period != nil && paid)
            actionButton(String(localized: "ledger.server.download_csv"), action: .csv, valid: query.period != nil && paid)
            if !paid {
                Text(String(localized: "ledger.refusal.capability")).foregroundStyle(.secondary)
                Button(String(localized: "ledger.verify_subscription")) {
                    Task { await ledger.refreshPremiumProof() }
                }
            }
        }
    }

    private var comparisonForm: some View {
        Group {
            Section(String(localized: "ledger.server.baseline")) {
                vehiclePicker($baselineVehicle)
                DatePicker(String(localized: "ledger.server.month"), selection: $baselineDay, displayedComponents: .date)
            }
            Section(String(localized: "ledger.server.current")) {
                vehiclePicker($currentVehicle)
                DatePicker(String(localized: "ledger.server.month"), selection: $selectedDay, displayedComponents: .date)
            }
            actionButton(String(localized: "ledger.server.compare"), action: .compare,
                         valid: query.baseline != nil && query.current != nil)
        }
    }

    private func vehiclePicker(_ selection: Binding<String?>) -> some View {
        Picker(String(localized: "ledger.select_vehicle"), selection: selection) {
            Text(String(localized: "ledger.select_vehicle")).tag(nil as String?)
            ForEach(viewModel.snapshot.vehicles) { vehicle in
                Text(vehicle.name).tag(Optional(vehicle.id))
            }
        }
    }

    private func actionButton(_ title: String, action: Action, valid: Bool) -> some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            Button(title) { request(action) }
                .disabled(!valid || task != nil || retryAt.map { $0 > context.date } == true)
        }
    }

    private func reportResults(_ result: LedgerServerReport) -> some View {
        Section(String(localized: "ledger.server.reports")) {
            if let generated = result.generatedDate {
                Text(generated.formatted(date: .abbreviated, time: .shortened))
                    .font(.footnote).foregroundStyle(.secondary)
            }
            LabeledContent(String(localized: "ledger.server.record_count"), value: result.totals.recordCount.formatted())
            LabeledContent(String(localized: "ledger.server.purchase"), value: AbastevoTheme.formatCentavos(result.totals.purchaseCentavos))
            LabeledContent(String(localized: "android.ledger_sync_expense"), value: AbastevoTheme.formatCentavos(result.totals.expenseCentavos))
            if let charge = result.totals.chargeCostCentavos {
                LabeledContent(String(localized: "ledger.server.charge_cost"), value: AbastevoTheme.formatCentavos(charge))
            }
            if let fees = result.totals.chargeFeesCentavos {
                LabeledContent(String(localized: "ledger.server.charge_fees"), value: AbastevoTheme.formatCentavos(fees))
            }
            if let energy = result.totals.chargeEnergyWh {
                let kwh = (Decimal(energy) / 1000).formatted(.number.precision(.fractionLength(0...3)))
                LabeledContent(String(localized: "ledger.server.charge_energy"), value: kwh)
            }
            ForEach(result.totals.byVehicle.keys.sorted(), id: \.self) { id in
                if let cost = result.totals.byVehicle[id] {
                    LabeledContent(vehicleLabel(id), value: AbastevoTheme.formatCentavos(cost))
                }
            }
            coverage(result.totals.coverage, title: String(localized: "ledger.server.consumption"))
            coverage(result.totals.energyCoverage ?? [:], title: String(localized: "ledger.server.efficiency"))
            coverage(result.totals.combinedCostPerKm ?? [:], title: String(localized: "ledger.server.combined_cost"))
        }
    }

    @ViewBuilder private func coverage(_ values: [String: String], title: String) -> some View {
        if !values.isEmpty {
            Text(title).font(.subheadline.bold())
            ForEach(values.keys.sorted(), id: \.self) { id in
                if let value = values[id] {
                    LabeledContent(vehicleLabel(id), value: LedgerReportedRatio(value).map(ratioText) ?? LedgerCoordinator.reasonLabel(value))
                }
            }
        }
    }

    private func comparisonResults(_ result: LedgerServerComparison) -> some View {
        Section(String(localized: "ledger.server.compare")) {
            if let issue = result.issue {
                Text(issueLabel(issue)).foregroundStyle(.secondary)
            }
            if result.compatible, let baseline = result.baseline, let current = result.current {
                Text(String(localized: "ledger.server.consumption")).font(.subheadline.bold())
                LabeledContent(String(localized: "ledger.server.baseline"), value: ratioText(baseline))
                LabeledContent(String(localized: "ledger.server.current"), value: ratioText(current))
                if let change = result.changePercent {
                    let value = (change.decimal / 100).formatted(.percent.precision(.fractionLength(0...2)))
                    LabeledContent(String(localized: "ledger.server.change"), value: value)
                }
            }
        }
    }

    private func vehicleLabel(_ id: String) -> String {
        viewModel.snapshot.vehicles.first { $0.id == id }?.name
            ?? String(localized: "ledger.server.vehicle_reference", defaultValue: "Vehicle \(String(id.prefix(8)))")
    }

    private func issueLabel(_ code: String) -> String {
        let before = "baseline-incomplete: "
        let after = "current-incomplete: "
        if code.hasPrefix(before) {
            let reason = LedgerCoordinator.reasonLabel(String(code.dropFirst(before.count)))
            return String(localized: "ledger.server.baseline_incomplete", defaultValue: "Baseline incomplete · \(reason)")
        }
        if code.hasPrefix(after) {
            let reason = LedgerCoordinator.reasonLabel(String(code.dropFirst(after.count)))
            return String(localized: "ledger.server.current_incomplete", defaultValue: "Current period incomplete · \(reason)")
        }
        switch code {
        case "missing-baseline": return String(localized: "ledger.server.no_baseline")
        case "missing-current": return String(localized: "ledger.server.no_current")
        case "mismatched-method": return String(localized: "ledger.server.method_mismatch")
        case "mismatched-timezone": return String(localized: "ledger.server.zone_mismatch")
        case "incompatible-unit": return String(localized: "ledger.server.unit_mismatch")
        default: return LedgerCoordinator.reasonLabel(code)
        }
    }

    private func ratioText(_ ratio: LedgerReportedRatio) -> String {
        ratio.decimal.formatted(.number.precision(.fractionLength(0...4)))
    }

    private func period(_ date: Date) -> LedgerReportPeriod? {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .current
        let parts = calendar.dateComponents([.year, .month], from: date)
        guard let year = parts.year, let month = parts.month else { return nil }
        return LedgerReportPeriod(year: year, month: month, timezone: calendar.timeZone.identifier)
    }

    private func request(_ action: Action) {
        let requested = query
        guard task == nil, retryAt.map({ $0 > Date() }) != true,
              !action.paid || paid, let scope = requested.scope,
              let (account, client) = ledger.analysisClient(for: scope) else { return }
        let operationId = pending.flatMap { $0.action == action && $0.query == requested ? $0.operationId : nil }
            ?? UUID().uuidString.lowercased()
        pending = PendingAction(action: action, query: requested, operationId: operationId)
        let started = generation
        notice = nil
        task = Task {
            defer { if generation == started { task = nil } }
            do {
                try await load(action, requested: requested, owner: account, client: client,
                               scope: scope, operationId: operationId)
                guard requested == query, generation == started, !Task.isCancelled else { return }
                loadedQuery = requested
                pending = nil
                retryAt = nil
            } catch {
                guard requested == query, generation == started, !Task.isCancelled else { return }
                showFailure(error)
            }
        }
    }

    private func load(
        _ action: Action, requested: Query, owner: String, client: LedgerSyncClient,
        scope: AccountDataExportScope, operationId: String
    ) async throws {
        switch action {
        case .compare:
            guard let baseline = requested.baseline, let current = requested.current else { return }
            let value = try await client.compare(baseline: baseline, current: current, accountId: owner, scope: scope)
            guard requested == query, !Task.isCancelled else { return }
            comparison = value
        case .get, .build, .csv:
            guard let period = requested.period else { return }
            if action == .csv {
                let value = try await client.monthlyReportCsv(period, accountId: owner, scope: scope, operationId: operationId)
                guard requested == query, !Task.isCancelled else { return }
                csv = value
            } else {
                let value: LedgerServerReport
                if action == .build {
                    value = try await client.buildMonthlyReport(period, accountId: owner, scope: scope, operationId: operationId)
                } else {
                    value = try await client.monthlyReport(period, accountId: owner, scope: scope)
                }
                guard requested == query, !Task.isCancelled else { return }
                report = value
            }
        }
    }

    private func showFailure(_ error: Error) {
        if let problem = error as? SignedTransportError {
            switch problem {
            case .refused(let status, let code, let retry):
                if status == 408 || status == 425 || status == 429 || status >= 500 {
                    if let retry { retryAt = Date().addingTimeInterval(TimeInterval(retry)) }
                } else {
                    pending = nil
                }
                if status == 429 {
                    notice = String(localized: "ledger.server.rate_limit")
                } else if status >= 500 {
                    notice = String(localized: "ledger.unavailable")
                } else if code == "metrics.invalid" {
                    notice = String(localized: "ledger.refusal.invalid")
                } else {
                    notice = LedgerCoordinator.codeLabel(code ?? "http-\(status)")
                }
            case .offline: notice = String(localized: "ledger.refusal.offline")
            case .sessionRequired: notice = String(localized: "ledger.refusal.sign_in_again")
            default: notice = String(localized: "ledger.unavailable")
            }
        } else {
            notice = String(localized: "ledger.unavailable")
        }
    }

    private func clear() {
        generation = UUID()
        task?.cancel()
        task = nil
        report = nil
        comparison = nil
        csv = nil
        loadedQuery = nil
        notice = nil
        retryAt = nil
        pending = nil
    }
}
