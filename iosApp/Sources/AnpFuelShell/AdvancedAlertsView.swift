import SwiftUI
import AnpFuelCore

@MainActor
struct AdvancedAlertsView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject private var model: AdvancedAlertsModel
    @State private var pane = Pane.rules
    @State private var editor: Editor?
    @State private var confirmation: OwnerConfirmation?
    @State private var confirming = false

    private enum Pane: CaseIterable, Hashable { case rules, inbox }
    private struct Input: Equatable {
        let scope: AccountDataExportScope?
        let offline: Bool
    }
    private struct Editor: Identifiable {
        let id = UUID()
        let existing: AdvancedAlertRuleRecord?
        let scope: AccountDataExportScope
    }
    private struct OwnerConfirmation {
        let action: Confirmation
        let scope: AccountDataExportScope
    }
    private enum Confirmation {
        case delete(AdvancedAlertRuleRecord)
        case erase
        case discard
    }

    init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
        _model = ObservedObject(wrappedValue: viewModel.advancedAlerts)
    }

    private var input: Input {
        Input(scope: viewModel.advancedAlertScope, offline: viewModel.isOfflineSimulation)
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(String(localized: "alerts.service_boundary")).foregroundStyle(.secondary)
                }
                if let owner = input.scope, owner == model.scope {
                    ownerSections(scope: owner)
                } else if input.scope == nil {
                    Button(String(localized: "android.profile_action_sign_in")) { viewModel.activeSheet = .auth }
                } else {
                    ProgressView()
                }
            }
            .privacySensitive()
            .navigationTitle(String(localized: "alerts.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
            .refreshable { await model.refresh(on: viewModel) }
            .task(id: input) {
                editor = nil
                confirmation = nil
                confirming = false
                await model.activate(on: viewModel)
            }
            .sheet(item: $editor) { editor in
                AdvancedAlertEditor(existing: editor.existing, stations: viewModel.stations) { command in
                    self.editor = nil
                    Task { await model.confirm(command, scope: editor.scope, on: viewModel) }
                }
            }
            .confirmationDialog(String(localized: "alerts.confirm"), isPresented: $confirming, presenting: confirmation) { value in
                Button(confirmationAction(value.action), role: .destructive) { execute(value) }
                Button(String(localized: "android.action_cancel"), role: .cancel) {}
            } message: { value in
                Text(confirmationMessage(value.action))
            }
        }
    }

    @ViewBuilder
    private func ownerSections(scope: AccountDataExportScope) -> some View {
        Section {
            Picker(String(localized: "alerts.title"), selection: $pane) {
                Text(String(localized: "alerts.rules")).tag(Pane.rules)
                Text(String(localized: "alerts.inbox")).tag(Pane.inbox)
            }
            .pickerStyle(.segmented)
            Button(String(localized: "alerts.refresh")) { Task { await model.refresh(on: viewModel) } }
                .disabled(model.busy || input.offline)
            if model.busy { ProgressView() }
            if input.offline { Text(String(localized: "alerts.offline")).foregroundStyle(.secondary) }
            if model.acknowledged { Text(String(localized: "alerts.acknowledged")) }
            if let error = model.commandError { Text(errorText(error)).foregroundStyle(.secondary) }
            if model.pending {
                Text(String(localized: "alerts.pending")).foregroundStyle(.secondary)
                if let retryAt = model.retryAt {
                    TimelineView(.periodic(from: .now, by: 1)) { context in
                        Button(String(localized: "android.action_retry")) { Task { await model.dispatch(scope: scope, on: viewModel) } }
                            .disabled(model.busy || input.offline || retryAt > context.date)
                    }
                }
                Button(String(localized: "alerts.discard"), role: .destructive) { request(.discard, scope: scope) }
                    .disabled(model.busy)
            }
        }
        if pane == .rules { ruleSection(scope: scope) } else { inboxSection }
        Section {
            Toggle(String(localized: "alerts.local_notifications"), isOn: Binding(
                get: { model.notificationsEnabled }, set: { value in Task { await model.setNotifications(value, scope: scope, on: viewModel) } }
            ))
            .disabled(model.busy)
            Text(String(localized: "alerts.local_notifications_note")).font(.caption).foregroundStyle(.secondary)
            if model.notificationPermissionDenied {
                Text(String(localized: "alerts.notification_permission")).foregroundStyle(.secondary)
            }
            Button(String(localized: "alerts.erase"), role: .destructive) { request(.erase, scope: scope) }
                .disabled(model.busy || input.offline)
        }
    }

    private func ruleSection(scope: AccountDataExportScope) -> some View {
        Section(String(localized: "alerts.rules")) {
            if let error = model.rulesError { Text(errorText(error)).foregroundStyle(.secondary) }
            if let rules = model.rules {
                if rules.isEmpty { Text(String(localized: "alerts.rules_empty")) }
                ForEach(rules) { row in ruleRow(row, scope: scope) }
            }
            Button(String(localized: "alerts.create")) { editor = Editor(existing: nil, scope: scope) }
                .disabled(model.busy || model.pending || (model.rules?.count ?? 0) >= 20)
            Text(String(localized: "alerts.capability_note")).font(.caption).foregroundStyle(.secondary)
        }
    }

    private func ruleRow(_ row: AdvancedAlertRuleRecord, scope: AccountDataExportScope) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            if row.payload.target.isStationTarget {
                let stations = row.payload.target.stationIds ?? []
                Text(String(localized: "alerts.station_count", defaultValue: "Selected stations: \(stations.count)"))
                    .font(.headline)
            } else {
                if let city = row.payload.target.city { Text(city).font(.headline) }
                Text(String(localized: "alerts.city_unavailable")).foregroundStyle(.secondary)
            }
            Text(row.enabled ? String(localized: "alerts.enabled") : String(localized: "alerts.disabled"))
            ForEach(Array(row.payload.predicates.enumerated()), id: \.offset) { _, predicate in
                if let product = FuelProduct.fromWireCode(predicate.fuel) {
                    Text(product.localizedName).font(.subheadline)
                }
            }
            if row.payload.target.isStationTarget {
                Button(String(localized: "alerts.edit")) { editor = Editor(existing: row, scope: scope) }
                    .disabled(model.busy || model.pending)
                Button(row.enabled ? String(localized: "alerts.disable") : String(localized: "alerts.enable")) {
                    let command = AdvancedAlertCommand(ruleId: row.ruleId, expectedRevision: row.revision, enabled: !row.enabled)
                    Task { await model.confirm(command, scope: scope, on: viewModel) }
                }
                .disabled(model.busy || model.pending)
            }
            Button(String(localized: "alerts.delete"), role: .destructive) { request(.delete(row), scope: scope) }
                .disabled(model.busy || model.pending)
        }
        .buttonStyle(.borderless)
    }

    private var inboxSection: some View {
        Section(String(localized: "alerts.inbox")) {
            if let error = model.inboxError { Text(errorText(error)).foregroundStyle(.secondary) }
            if let inbox = model.inbox {
                if inbox.items.isEmpty { Text(String(localized: "alerts.inbox_empty")) }
                ForEach(inbox.items) { item in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(inboxStatus(item.status)).font(.headline)
                        if let stationId = item.stationId,
                           let station = viewModel.stations.first(where: { $0.id == stationId }) {
                            Text(station.name)
                        } else {
                            Text(String(localized: "alerts.station_unavailable")).foregroundStyle(.secondary)
                        }
                        if let source = item.source { Text(sourceLabel(source)) }
                        if let observedAt = item.observedAt {
                            Text(observedAt.formatted(.dateTime.year().month().day().hour().minute())).font(.caption)
                        }
                    }
                }
                if inbox.limitReached { Text(String(localized: "alerts.inbox_limit")).foregroundStyle(.secondary) }
            }
        }
    }

    private func request(_ confirmation: Confirmation, scope: AccountDataExportScope) {
        self.confirmation = OwnerConfirmation(action: confirmation, scope: scope)
        confirming = true
    }

    private func execute(_ confirmation: OwnerConfirmation) {
        switch confirmation.action {
        case .delete(let row):
            let command = AdvancedAlertCommand(ruleId: row.ruleId, expectedRevision: row.revision, delete: true)
            Task { await model.confirm(command, scope: confirmation.scope, on: viewModel) }
        case .erase: Task { await model.erase(scope: confirmation.scope, on: viewModel) }
        case .discard: model.discardPending(scope: confirmation.scope, on: viewModel)
        }
    }

    private func confirmationAction(_ confirmation: Confirmation) -> String {
        switch confirmation {
        case .delete: return String(localized: "alerts.delete")
        case .erase: return String(localized: "alerts.erase")
        case .discard: return String(localized: "alerts.discard")
        }
    }

    private func confirmationMessage(_ confirmation: Confirmation) -> String {
        switch confirmation {
        case .delete: return String(localized: "alerts.delete_message")
        case .erase: return String(localized: "alerts.erase_message")
        case .discard: return String(localized: "alerts.discard_message")
        }
    }

    private func errorText(_ error: AdvancedAlertError) -> String {
        switch error {
        case .invalid: return String(localized: "alerts.invalid")
        case .authRequired, .ownerChanged: return String(localized: "alerts.auth")
        case .capabilityRequired: return String(localized: "alerts.capability_note")
        case .notFound: return String(localized: "alerts.not_found")
        case .conflict: return String(localized: "alerts.conflict")
        case .rateLimited: return String(localized: "alerts.rate_limited")
        case .unavailable: return String(localized: "alerts.unavailable")
        case .requestInProgress: return String(localized: "alerts.in_progress")
        }
    }

    private func inboxStatus(_ status: AdvancedAlertInboxStatus) -> String {
        switch status {
        case .evaluated: return String(localized: "alerts.inbox_evaluated")
        case .dispatched: return String(localized: "alerts.inbox_dispatched")
        case .sent: return String(localized: "alerts.inbox_sent")
        case .dropped: return String(localized: "alerts.inbox_dropped")
        case .unknown: return String(localized: "alerts.inbox_unknown")
        }
    }
}

@MainActor
private struct AdvancedAlertEditor: View {
    @Environment(\.dismiss) private var dismiss
    let existing: AdvancedAlertRuleRecord?
    let stations: [StationRecord]
    let save: (AdvancedAlertCommand) -> Void
    @State private var predicates: [PredicateDraft]
    @State private var selectedStations: Set<String>
    @State private var op: AdvancedAlertOperator
    @State private var days: Set<Int>
    @State private var timezone: String
    @State private var windowStart: Date
    @State private var windowEnd: Date
    @State private var quietStart: Date
    @State private var quietEnd: Date
    @State private var cooldown: Int64
    @State private var invalid = false

    private static let defaultZone = "America/Sao_Paulo"
    private static let timezoneIdentifiers = Array(Set(TimeZone.knownTimeZoneIdentifiers + [defaultZone, "UTC"])).sorted()

    init(existing: AdvancedAlertRuleRecord?, stations: [StationRecord], save: @escaping (AdvancedAlertCommand) -> Void) {
        self.existing = existing
        self.stations = stations
        self.save = save
        _predicates = State(initialValue: existing?.payload.predicates.map(PredicateDraft.init) ?? [PredicateDraft()])
        _selectedStations = State(initialValue: Set(existing?.payload.target.stationIds ?? []))
        _op = State(initialValue: existing?.payload.op ?? .all)
        let schedule = existing?.payload.schedule
        _days = State(initialValue: Set(schedule?.days ?? Array(0...6)))
        _timezone = State(initialValue: schedule?.timezone ?? (TimeZone.knownTimeZoneIdentifiers.contains(TimeZone.current.identifier) ? TimeZone.current.identifier : Self.defaultZone))
        _windowStart = State(initialValue: Self.date(for: schedule?.windowStart))
        _windowEnd = State(initialValue: Self.date(for: schedule?.windowEnd))
        _quietStart = State(initialValue: Self.date(for: schedule?.quietStart))
        _quietEnd = State(initialValue: Self.date(for: schedule?.quietEnd))
        _cooldown = State(initialValue: schedule?.cooldownHours ?? 0)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section(String(localized: "alerts.rule_predicates")) {
                    Picker(String(localized: "alerts.rule_match"), selection: $op) {
                        Text(String(localized: "alerts.match_all")).tag(AdvancedAlertOperator.all)
                        Text(String(localized: "alerts.match_any")).tag(AdvancedAlertOperator.any)
                    }
                    ForEach($predicates) { $predicate in
                        predicateEditor($predicate)
                        if predicates.count > 1 {
                            Button(String(localized: "alerts.remove_predicate"), role: .destructive) {
                                predicates.removeAll { $0.id == predicate.id }
                            }
                        }
                    }
                    if predicates.count < 3 {
                        Button(String(localized: "alerts.add_predicate")) { predicates.append(PredicateDraft()) }
                    }
                }
                Section(String(localized: "alerts.rule_target")) {
                    NavigationLink {
                        AdvancedAlertStationPicker(stations: stations, selected: $selectedStations)
                    } label: {
                        Text(String(localized: "alerts.station_count", defaultValue: "Selected stations: \(selectedStations.count)"))
                    }
                    Text(String(localized: "alerts.city_unavailable")).font(.caption).foregroundStyle(.secondary)
                }
                scheduleEditor
                if invalid { Text(String(localized: "alerts.invalid")).foregroundStyle(.secondary) }
            }
            .navigationTitle(existing == nil ? String(localized: "alerts.create") : String(localized: "alerts.edit"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.action_cancel")) { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "alerts.save")) { commit() }
                }
            }
        }
    }

    @ViewBuilder
    private func predicateEditor(_ draft: Binding<PredicateDraft>) -> some View {
        Picker(String(localized: "alerts.predicate_kind"), selection: draft.kind) {
            Text(String(localized: "alerts.threshold")).tag(AdvancedAlertPredicateKind.threshold)
            Text(String(localized: "alerts.change")).tag(AdvancedAlertPredicateKind.change)
        }
        Picker(String(localized: "android.vehicle_fuel_product_label"), selection: draft.fuel) {
            ForEach(FuelProduct.allCases.filter { $0 != .lpgP13 }) { fuel in
                Text(fuel.localizedName).tag(fuel)
            }
        }
        LabeledContent(String(localized: "alerts.unit"), value: draft.wrappedValue.fuel == .cng
                       ? String(localized: "alerts.unit_cubic_metre") : String(localized: "alerts.unit_litre"))
        Picker(String(localized: "alerts.source"), selection: draft.source) {
            ForEach(AdvancedAlertSource.allCases) { source in Text(sourceLabel(source)).tag(source) }
        }
        Picker(String(localized: "alerts.condition"), selection: draft.condition) {
            ForEach(AdvancedAlertCondition.allCases) { condition in Text(conditionLabel(condition)).tag(condition) }
        }
        if draft.wrappedValue.kind == .threshold {
            TextField(String(localized: "alerts.threshold_amount"), text: draft.threshold).keyboardType(.decimalPad)
        } else {
            TextField(String(localized: "alerts.drop_amount"), text: draft.drop).keyboardType(.decimalPad)
            TextField(String(localized: "alerts.drop_percent"), text: draft.percent).keyboardType(.decimalPad)
            TextField(String(localized: "alerts.baseline"), text: draft.baseline).keyboardType(.numberPad)
        }
    }

    private var scheduleEditor: some View {
        let weekdays = Calendar.current.weekdaySymbols
        return Section(String(localized: "alerts.schedule")) {
            ForEach(weekdays.indices, id: \.self) { day in
                Toggle(weekdays[day], isOn: Binding(
                    get: { days.contains(day) }, set: { enabled in
                        if enabled { days.insert(day) } else { days.remove(day) }
                    }
                ))
            }
            Picker(String(localized: "alerts.timezone"), selection: $timezone) {
                ForEach(Self.timezoneIdentifiers, id: \.self) { id in Text(id).tag(id) }
            }
            .pickerStyle(.navigationLink)
            DatePicker(String(localized: "alerts.window_start"), selection: $windowStart, displayedComponents: .hourAndMinute)
                .environment(\.timeZone, .gmt)
            DatePicker(String(localized: "alerts.window_end"), selection: $windowEnd, displayedComponents: .hourAndMinute)
                .environment(\.timeZone, .gmt)
            DatePicker(String(localized: "alerts.quiet_start"), selection: $quietStart, displayedComponents: .hourAndMinute)
                .environment(\.timeZone, .gmt)
            DatePicker(String(localized: "alerts.quiet_end"), selection: $quietEnd, displayedComponents: .hourAndMinute)
                .environment(\.timeZone, .gmt)
            Text(String(localized: "alerts.schedule_note")).font(.caption).foregroundStyle(.secondary)
            Stepper(value: $cooldown, in: 0...168) {
                Text(String(localized: "alerts.cooldown", defaultValue: "Cooldown hours: \(cooldown)"))
            }
        }
    }

    private func commit() {
        do {
            let schedule = AdvancedAlertSchedule(days: days.sorted(), windowStart: Self.clock(for: windowStart),
                                                  windowEnd: Self.clock(for: windowEnd), timezone: timezone,
                                                  quietStart: Self.clock(for: quietStart), quietEnd: Self.clock(for: quietEnd),
                                                  cooldownHours: cooldown)
            let rule = AdvancedAlertRule(predicates: try predicates.map { try $0.value() }, op: op,
                                         target: AdvancedAlertTarget(stationIds: selectedStations.sorted()), schedule: schedule)
            let command = AdvancedAlertCommand(ruleId: existing?.ruleId ?? UUID().uuidString.lowercased(),
                                               expectedRevision: existing?.revision ?? 0, payload: rule)
            try command.validate()
            save(command)
        } catch {
            invalid = true
        }
    }

    private static func date(for clock: String?) -> Date {
        let parts = (clock ?? "00:00").split(separator: ":").compactMap { Int($0) }
        guard parts.count == 2 else { return Date(timeIntervalSince1970: 0) }
        return Date(timeIntervalSince1970: Double((parts[0] * 60 + parts[1]) * 60))
    }

    private static func clock(for date: Date) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = .gmt
        let fields = calendar.dateComponents([.hour, .minute], from: date)
        return String(format: "%02d:%02d", fields.hour ?? 0, fields.minute ?? 0)
    }
}

private struct PredicateDraft: Identifiable {
    let id = UUID()
    var kind: AdvancedAlertPredicateKind = .threshold
    var fuel: FuelProduct = .gasolineRegular
    var condition: AdvancedAlertCondition = .standard
    var source: AdvancedAlertSource = .official
    var threshold = ""
    var drop = ""
    var percent = ""
    var baseline = "24"

    init() {}

    init(_ prior: AdvancedAlertPredicate) {
        kind = prior.kind
        fuel = FuelProduct.fromWireCode(prior.fuel) ?? .gasolineRegular
        condition = prior.condition
        source = prior.source
        threshold = Self.display(prior.thresholdMilli, divisor: 1_000)
        drop = Self.display(prior.dropMilli, divisor: 1_000)
        percent = Self.display(prior.dropPercentBp, divisor: 100)
        baseline = (prior.baselineHours ?? 24).formatted(.number.grouping(.never))
    }

    func value() throws -> AdvancedAlertPredicate {
        if kind == .threshold {
            let amount = try PortableMoney.parse(PortableMoney.canonicalUserInput(threshold))
            return AdvancedAlertPredicate(kind: kind, fuel: fuel, condition: condition, source: source, thresholdMilli: amount)
        }
        let change = try Self.amount(drop)
        let percentage = try Self.amount(percent)
        guard (percentage ?? 0) % 10 == 0, let hours = Int64(baseline) else { throw AdvancedAlertError.invalid }
        return AdvancedAlertPredicate(kind: kind, fuel: fuel, condition: condition, source: source,
                                      dropMilli: change, dropPercentBp: percentage.map { $0 / 10 }, baselineHours: hours)
    }

    private static func display(_ value: Int64?, divisor: Int) -> String {
        guard let value, value > 0 else { return "" }
        return (Decimal(value) / Decimal(divisor)).formatted(.number.grouping(.never).precision(.fractionLength(0...3)))
    }

    private static func amount(_ input: String) throws -> Int64? {
        if input.trimmingCharacters(in: .whitespaces).isEmpty { return nil }
        do {
            return try PortableMoney.parse(PortableMoney.canonicalUserInput(input))
        } catch let error as PortableMoneyError where error.code == "zero-price" {
            return nil
        }
    }
}

private struct AdvancedAlertStationPicker: View {
    let stations: [StationRecord]
    @Binding var selected: Set<String>
    @State private var search = ""

    var body: some View {
        let known = stations.filter { CommunityWire.isUuid($0.id) && $0.id == $0.id.lowercased() }
        let visible = known.filter { search.isEmpty || $0.name.localizedCaseInsensitiveContains(search) }
        let unknown = selected.subtracting(Set(known.map(\.id))).sorted()
        List {
            ForEach(visible) { station in
                Toggle(station.name, isOn: Binding(
                    get: { selected.contains(station.id) }, set: { value in
                        if value { selected.insert(station.id) } else { selected.remove(station.id) }
                    }
                ))
                .disabled(selected.count >= 10 && !selected.contains(station.id))
            }
            ForEach(unknown, id: \.self) { id in
                Button(String(localized: "alerts.remove_unknown_station"), role: .destructive) { selected.remove(id) }
            }
            if known.isEmpty { Text(String(localized: "alerts.stations_empty")) }
        }
        .navigationTitle(String(localized: "alerts.rule_target"))
        .searchable(text: $search, prompt: Text(String(localized: "alerts.station_search")))
    }
}

private func sourceLabel(_ source: AdvancedAlertSource) -> String {
    switch source {
    case .official: return String(localized: "alerts.source_official")
    case .community: return String(localized: "alerts.source_community")
    }
}

private func conditionLabel(_ condition: AdvancedAlertCondition) -> String {
    switch condition {
    case .standard: return PriceConditionKind.standard.localizedLabel
    case .cash: return PriceConditionKind.cash.localizedLabel
    case .debit: return PriceConditionKind.debit.localizedLabel
    case .credit: return PriceConditionKind.credit.localizedLabel
    case .app: return PriceConditionKind.app.localizedLabel
    case .loyalty: return PriceConditionKind.loyalty.localizedLabel
    case .other: return String(localized: "alerts.condition_other")
    }
}
