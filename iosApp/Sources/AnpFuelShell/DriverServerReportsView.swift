import SwiftUI
import AnpFuelCore

@MainActor
struct DriverServerReportsView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject private var ledger: LedgerCoordinator
    @State private var tab = Pane.earnings
    @State private var activeScope: AccountDataExportScope?
    @State private var earnings: [DriverServerEarning] = []
    @State private var cursor: String?
    @State private var usedCursors: Set<String> = []
    @State private var earningsLoaded = false
    @State private var earningsBusy = false
    @State private var earningsError: DriverServerError?
    @State private var earningsTicket = UUID()
    @State private var report: DriverServerReport?
    @State private var loadedRequest: DriverReportRequest?
    @State private var reportBusy = false
    @State private var reportError: DriverServerError?
    @State private var reportTicket = UUID()
    @State private var buildAttempt: BuildAttempt?
    @State private var buildRetryAt: Date?
    @State private var checkSaved = false
    @State private var vehicleId: String?
    @State private var period: DriverReportPeriod = .month
    @State private var basis: DriverReportDistanceBasis = .odometer
    @State private var platform: DriverPlatform?
    @State private var timezone = TimeZone.current.identifier
    @State private var anchor = Date()
    @State private var from = Date()
    @State private var to = Calendar.current.date(byAdding: .day, value: 1, to: Date()) ?? Date()

    private enum Pane: Hashable { case earnings, reports }
    private enum ReportAction { case get, build }
    private struct BuildAttempt {
        let request: DriverReportRequest
        let operationId: String
    }
    private static let zones = Array(Set(TimeZone.knownTimeZoneIdentifiers + ["UTC", TimeZone.current.identifier])).sorted()
    private static let platforms: [DriverPlatform] = ["UBER", "99", "INDRIVE", "OTHER"].compactMap(DriverPlatform.init(rawValue:))

    init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
        _ledger = ObservedObject(wrappedValue: viewModel.ledger)
    }

    private var selectedRequest: DriverReportRequest? {
        let zone = timezone
        guard let vehicleId, vehicles.contains(vehicleId) else { return nil }
        let request: DriverReportRequest
        if period == .custom {
            request = DriverReportRequest(vehicleId: vehicleId, timezone: zone, period: period,
                                          from: DriverReportRequest.day(from, timezone: zone),
                                          to: DriverReportRequest.day(to, timezone: zone), platform: platform, basis: basis)
        } else {
            request = DriverReportRequest(vehicleId: vehicleId, timezone: zone, period: period,
                                          anchor: DriverReportRequest.day(anchor, timezone: zone), platform: platform, basis: basis)
        }
        return (try? request.validate()) != nil ? request : nil
    }

    private var vehicles: [String] {
        let local = viewModel.snapshot.vehicles.map(\.id).filter { CommunityWire.isUuid($0) && $0 == $0.lowercased() }
        let server = earnings.map(\.vehicleId)
        var seen = Set<String>()
        return (local + server).filter { seen.insert($0).inserted }
    }

    private var canBuild: Bool {
        guard viewModel.accountDataExportScope != nil, let scope = ledger.currentScope,
              let observation = ledger.syncState?.premiumObservation,
              observation.accountId == scope.accountId, observation.origin == scope.origin,
              observation.grants("professional.report") else { return false }
        let now = LedgerCoordinator.nowMillis()
        let age = now - observation.observedAtMillis
        return (0...900_000).contains(age) && (observation.validUntilMillis.map { now < $0 } ?? true)
    }

    var body: some View {
        List {
            if let scope = viewModel.accountDataExportScope, activeScope == scope {
                Section {
                    Picker(String(localized: "driver.server.title"), selection: $tab) {
                        Text(String(localized: "driver.server.earnings")).tag(Pane.earnings)
                        Text(String(localized: "ledger.server.reports")).tag(Pane.reports)
                    }
                    .pickerStyle(.segmented)
                }
                if tab == .earnings { earningSections(scope: scope) } else { reportSections(scope: scope) }
            } else if viewModel.isOfflineSimulation {
                Text(String(localized: "driver.server.offline"))
            } else if viewModel.accountDataExportScope == nil {
                Text(String(localized: "driver.server.auth"))
            } else {
                ProgressView()
            }
        }
        .privacySensitive()
        .navigationTitle(String(localized: "driver.server.title"))
        .task(id: viewModel.accountDataExportScope) {
            resetOwner()
            guard let scope = viewModel.accountDataExportScope else { return }
            activeScope = scope
            async let premium: Void = ledger.refreshPremiumProof()
            await readEarnings(scope: scope, after: nil)
            _ = await premium
        }
        .onChange(of: selectedRequest) { _, _ in resetReport() }
    }

    @ViewBuilder
    private func earningSections(scope: AccountDataExportScope) -> some View {
        Section {
            Button(String(localized: "driver.server.refresh_earnings")) { Task { await readEarnings(scope: scope, after: nil) } }
                .disabled(earningsBusy)
            if earningsBusy { ProgressView() }
            if let earningsError { Text(errorLabel(earningsError)).foregroundStyle(.secondary) }
            if earningsLoaded && earnings.isEmpty { Text(String(localized: "driver.server.earnings_empty")) }
            ForEach(earnings) { row in
                VStack(alignment: .leading, spacing: 6) {
                    Text(vehicleLabel(row.vehicleId)).font(.headline)
                    Text(platformLabel(row.platform))
                    if let start = row.fromDate, let end = row.toDate {
                        Text(intervalLabel(start, end, timezone: row.timezone)).font(.caption)
                    }
                    LabeledContent(String(localized: "driver.server.earned"), value: amount(row.money.earnedCentavos))
                    if row.needsReconciliation {
                        Text(String(localized: "driver.server.reconciliation")).foregroundStyle(.secondary)
                    }
                }
            }
            if let cursor {
                Button(String(localized: "driver.server.more_earnings")) { Task { await readEarnings(scope: scope, after: cursor) } }
                    .disabled(earningsBusy)
            }
        }
    }

    @ViewBuilder
    private func reportSections(scope: AccountDataExportScope) -> some View {
        Section {
            Picker(String(localized: "driver.vehicle"), selection: $vehicleId) {
                Text(String(localized: "driver.select_vehicle")).tag(nil as String?)
                ForEach(vehicles, id: \.self) { id in Text(vehicleLabel(id)).tag(Optional(id)) }
            }
            Picker(String(localized: "driver.server.period"), selection: $period) {
                ForEach(DriverReportPeriod.allCases) { item in Text(periodLabel(item)).tag(item) }
            }
            if period == .custom {
                DatePicker(String(localized: "driver.server.from"), selection: $from, displayedComponents: .date)
                    .environment(\.timeZone, TimeZone(identifier: timezone) ?? .gmt)
                DatePicker(String(localized: "driver.server.to_exclusive"), selection: $to, displayedComponents: .date)
                    .environment(\.timeZone, TimeZone(identifier: timezone) ?? .gmt)
            } else {
                DatePicker(String(localized: "driver.server.anchor"), selection: $anchor, displayedComponents: .date)
                    .environment(\.timeZone, TimeZone(identifier: timezone) ?? .gmt)
            }
            Picker(String(localized: "alerts.timezone"), selection: $timezone) {
                ForEach(Self.zones, id: \.self) { zone in Text(zone).tag(zone) }
            }
            .pickerStyle(.navigationLink)
            Picker(String(localized: "driver.server.platform"), selection: $platform) {
                Text(String(localized: "driver.server.all_platforms")).tag(nil as DriverPlatform?)
                ForEach(Self.platforms, id: \.rawValue) { value in Text(platformLabel(value)).tag(Optional(value)) }
            }
            Picker(String(localized: "driver.server.distance_basis"), selection: $basis) {
                ForEach(DriverReportDistanceBasis.allCases) { value in Text(basisLabel(value)).tag(value) }
            }
            Button(String(localized: "ledger.server.read")) { runReport(.get, scope: scope) }
                .disabled(reportBusy || selectedRequest == nil)
            TimelineView(.periodic(from: .now, by: 1)) { context in
                Button(String(localized: "ledger.server.build")) { runReport(.build, scope: scope) }
                    .disabled(reportBusy || selectedRequest == nil || !canBuild || checkSaved || (buildRetryAt.map { $0 > context.date } ?? false))
            }
            if !canBuild {
                Text(String(localized: "driver.server.premium_build")).foregroundStyle(.secondary)
                Button(String(localized: "android.action_retry")) { Task { await ledger.refreshPremiumProof() } }
            }
            if checkSaved { Text(String(localized: "driver.server.check_saved")).foregroundStyle(.secondary) }
            if reportBusy { ProgressView() }
            if let reportError { Text(errorLabel(reportError)).foregroundStyle(.secondary) }
        }
        if let report, loadedRequest == selectedRequest { reportResults(report) }
    }

    @ViewBuilder
    private func reportResults(_ report: DriverServerReport) -> some View {
        Section(String(localized: "driver.server.report")) {
            if let (start, end) = report.dayRange {
                Text(intervalLabel(start, end, timezone: report.timezone)).font(.headline)
            }
            if let generated = report.generatedDate {
                LabeledContent(String(localized: "driver.server.generated"), value: generated.formatted(.dateTime.year().month().day().hour().minute()))
            }
            LabeledContent(String(localized: "driver.server.snapshot_version"), value: report.snapshotVersion.formatted())
            LabeledContent(String(localized: "driver.server.earned"), value: amount(report.earnedCentavos))
            LabeledContent(String(localized: "driver.server.adjustments"), value: amount(report.adjustmentsCentavos))
            LabeledContent(String(localized: "driver.server.recorded_costs"), value: amount(report.costsCentavos))
            LabeledContent(String(localized: "driver.server.modeled_costs"), value: amount(report.modeledCostsCentavos))
            LabeledContent(String(localized: "driver.server.recorded_result"), value: amount(report.recordedResultCentavos))
            LabeledContent(String(localized: "driver.server.modeled_result"), value: amount(report.modeledResultCentavos))
            LabeledContent(String(localized: "driver.server.distance"), value: distance(report.distanceMetres))
        }
        Section {
            LabeledContent(String(localized: "driver.server.earnings_per_km"), value: ratio(report.earningsPerKm))
            LabeledContent(String(localized: "driver.server.recorded_per_km"), value: ratio(report.recordedResultPerKm))
            LabeledContent(String(localized: "driver.server.modeled_per_km"), value: ratio(report.modeledResultPerKm))
            LabeledContent(String(localized: "driver.server.recorded_per_hour"), value: ratio(report.recordedResultPerHour))
            LabeledContent(String(localized: "driver.server.modeled_per_hour"), value: ratio(report.modeledResultPerHour))
        }
        Section(String(localized: "driver.server.coverage")) {
            LabeledContent(String(localized: "driver.server.earnings"), value: coverageLabel(report.coverage["earnings"]))
            LabeledContent(String(localized: "driver.server.recorded_costs"), value: coverageLabel(report.coverage["costs"]))
            LabeledContent(String(localized: "driver.server.distance"), value: coverageLabel(report.coverage["distance"]))
            LabeledContent(String(localized: "driver.server.report_time"), value: coverageLabel(report.coverage["time"]))
            LabeledContent(String(localized: "driver.server.report_result"), value: coverageLabel(report.coverage["result"]))
        }
    }

    private func readEarnings(scope: AccountDataExportScope, after: String?) async {
        guard viewModel.accountDataExportScope == scope, scope == activeScope, !earningsBusy,
              after.map({ $0 == cursor && !usedCursors.contains($0) }) ?? true else { return }
        earningsTicket = UUID()
        let ticket = earningsTicket
        earningsBusy = true
        earningsError = nil
        defer { if earningsTicket == ticket { earningsBusy = false } }
        do {
            let page = try await viewModel.communityGateway.driverEarnings(cursor: after, scope: scope)
            guard ownerMatches(scope), earningsTicket == ticket else { return }
            if let after {
                guard !usedCursors.contains(page.nextCursor ?? ""),
                      earnings.last.map({ last in page.items.allSatisfy { $0.id > last.id } }) ?? true else {
                    throw DriverServerError.unavailable(retryAfterSeconds: nil)
                }
                usedCursors.insert(after)
                earnings.append(contentsOf: page.items)
            } else {
                usedCursors = []
                earnings = page.items
            }
            earningsLoaded = true
            cursor = page.nextCursor
        } catch {
            guard ownerMatches(scope), earningsTicket == ticket else { return }
            earningsError = (error as? DriverServerError) ?? .unavailable(retryAfterSeconds: nil)
        }
    }

    private func runReport(_ action: ReportAction, scope: AccountDataExportScope) {
        guard ownerMatches(scope), let selection = selectedRequest, !reportBusy,
              action == .get || (canBuild && !checkSaved && (buildRetryAt.map { $0 <= Date() } ?? true)) else { return }
        let request: DriverReportRequest
        let operation: String?
        if action == .build {
            if let previous = buildAttempt, previous.request.expecting(nil) == selection {
                request = previous.request
                operation = previous.operationId
            } else {
                request = selection.expecting(loadedRequest == selection ? report?.revision : nil)
                let stable = UUID().uuidString.lowercased()
                buildAttempt = BuildAttempt(request: request, operationId: stable)
                operation = stable
            }
        } else {
            request = selection
            operation = nil
        }
        let ticket = UUID()
        reportTicket = ticket
        reportBusy = true
        reportError = nil
        Task { await loadReport(request, selection: selection, scope: scope, ticket: ticket, operation: operation) }
    }

    private func loadReport(_ request: DriverReportRequest, selection: DriverReportRequest, scope: AccountDataExportScope,
                            ticket: UUID, operation: String?) async {
        defer { if reportTicket == ticket { reportBusy = false } }
        guard ownerMatches(scope), reportTicket == ticket, selectedRequest == selection else { return }
        do {
            let client = viewModel.communityGateway
            let received: DriverServerReport
            if let operation {
                received = try await client.buildDriverReport(request, scope: scope, operationId: operation)
            } else {
                received = try await client.driverSavedReport(request, scope: scope)
            }
            guard ownerMatches(scope), reportTicket == ticket, selectedRequest == selection else { return }
            report = received
            loadedRequest = selection
            buildAttempt = nil
            buildRetryAt = nil
            checkSaved = false
        } catch {
            guard ownerMatches(scope), reportTicket == ticket, selectedRequest == selection else { return }
            let issue = (error as? DriverServerError) ?? .unavailable(retryAfterSeconds: nil)
            report = nil
            loadedRequest = nil
            reportError = issue
            switch issue {
            case .unavailable(let seconds) where operation != nil:
                checkSaved = true
                if let seconds, seconds > 0 { buildRetryAt = Date().addingTimeInterval(Double(seconds)) }
            case .notFound, .stale:
                checkSaved = false
                if issue == .stale { buildAttempt = nil }
            default:
                if operation != nil {
                    checkSaved = false
                    buildAttempt = nil
                    buildRetryAt = nil
                }
            }
        }
    }

    private func resetOwner() {
        activeScope = nil
        earnings = []
        cursor = nil
        usedCursors = []
        earningsTicket = UUID()
        earningsLoaded = false
        earningsBusy = false
        earningsError = nil
        vehicleId = nil
        resetReport()
    }

    private func resetReport() {
        reportTicket = UUID()
        report = nil
        loadedRequest = nil
        reportBusy = false
        reportError = nil
        buildRetryAt = nil
        buildAttempt = nil
        checkSaved = false
    }

    private func ownerMatches(_ scope: AccountDataExportScope) -> Bool {
        !Task.isCancelled && viewModel.accountDataExportScope == scope && activeScope == scope
    }

    private func vehicleLabel(_ id: String) -> String {
        viewModel.snapshot.vehicles.first(where: { $0.id == id })?.name
            ?? String(localized: "ledger.server.vehicle_reference", defaultValue: "Vehicle \(String(id.prefix(8)))")
    }

    private func intervalLabel(_ from: Date, _ to: Date, timezone: String) -> String {
        var style = Date.FormatStyle(date: .abbreviated, time: .omitted)
        style.timeZone = TimeZone(identifier: timezone) ?? .gmt
        let start = from.formatted(style)
        let end = to.formatted(style)
        return String(localized: "driver.server.interval", defaultValue: "\(start) to \(end) (end excluded)")
    }

    private func amount(_ cents: Int64?) -> String {
        guard let cents else { return String(localized: "driver.server.unknown") }
        return (Decimal(cents) / 100).formatted(.currency(code: "BRL"))
    }

    private func distance(_ metres: Int64?) -> String {
        guard let metres else { return String(localized: "driver.server.unknown") }
        return Measurement(value: Double(metres) / 1_000, unit: UnitLength.kilometers)
            .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(0...3))))
    }

    private func ratio(_ value: String) -> String {
        guard let ratio = LedgerReportedRatio(value) else { return String(localized: "driver.server.unknown") }
        return (ratio.decimal / 100).formatted(.currency(code: "BRL").precision(.fractionLength(2...4)))
    }

    private func periodLabel(_ period: DriverReportPeriod) -> String {
        switch period {
        case .day: return String(localized: "driver.server.day")
        case .week: return String(localized: "driver.server.week")
        case .month: return String(localized: "ledger.server.month")
        case .custom: return String(localized: "driver.server.custom")
        }
    }

    private func platformLabel(_ platform: DriverPlatform) -> String {
        switch platform.rawValue {
        case "UBER": return "Uber"
        case "99": return "99"
        case "INDRIVE": return "inDrive"
        default: return String(localized: "driver.server.other_platform")
        }
    }

    private func basisLabel(_ basis: DriverReportDistanceBasis) -> String {
        switch basis {
        case .odometer: return String(localized: "driver.server.odometer")
        case .gpsEstimate: return String(localized: "driver.server.gps_estimate")
        case .paidReported: return String(localized: "driver.server.paid_distance")
        case .manualTrip: return String(localized: "driver.server.manual_distance")
        }
    }

    private func coverageLabel(_ value: String?) -> String {
        switch value {
        case "complete": return String(localized: "driver.server.coverage_complete")
        case "no-records": return String(localized: "ledger.reason.no_records")
        case "partial-window", "profile-window-partial", "cost-item-window-partial", "unmatched-period-window":
            return String(localized: "ledger.reason.period_mismatch")
        case "adjustment-revision-obsolete": return String(localized: "driver.server.reconciliation")
        case "profile-missing": return String(localized: "driver.server.profile_missing")
        case "cost-amount-unknown", "cost-coverage-incomplete": return String(localized: "driver.server.costs_incomplete")
        case "distance-source-ambiguous-or-missing", "distance-coverage-incomplete", "odometer-segment-boundary",
             "reversed-odometer", "prediction-not-measured", "distance-classification-incomplete", "work-distance-nonpositive":
            return String(localized: "driver.server.distance_incomplete")
        case "model-assumptions-missing": return String(localized: "driver.server.model_missing")
        case "time-not-projected": return String(localized: "driver.server.time_unknown")
        default: return String(localized: "driver.server.unknown")
        }
    }

    private func errorLabel(_ issue: DriverServerError) -> String {
        switch issue {
        case .invalid: return String(localized: "driver.server.invalid")
        case .authRequired, .ownerChanged: return String(localized: "driver.server.auth")
        case .capabilityRequired: return String(localized: "driver.server.premium_build")
        case .disabled: return String(localized: "driver.server.disabled")
        case .notFound: return String(localized: "driver.server.not_found")
        case .stale: return String(localized: "driver.server.stale")
        case .reportTooLarge: return String(localized: "driver.server.too_large")
        case .conflict: return String(localized: "driver.server.conflict")
        case .unavailable: return String(localized: "driver.server.unavailable")
        }
    }
}
