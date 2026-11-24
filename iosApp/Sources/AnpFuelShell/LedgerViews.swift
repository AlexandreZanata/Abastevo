import SwiftUI
import AnpFuelCore

/// Vehicles, private ledger, monthly indicators and ledger cloud sync (B-BR-IOS-VEHICLE).
/// Android references: `VehicleViewModel`, `LedgerEntriesViewModel`,
/// `MonthlyIndicatorsViewModel` and `LedgerSyncViewModel`.
public struct VehiclesAndLedgerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        LedgerHomeView(viewModel: viewModel, ledger: viewModel.ledger) {
            dismiss()
        }
    }
}

enum LedgerFormat {
    static func money(_ centavos: Int64?) -> String {
        guard let centavos else { return "—" }
        return AbastevoTheme.formatCentavos(centavos)
    }

    static func quantity(_ milli: Int64, unit: LedgerUnit?) -> String {
        let value = LedgerUnits.formatScaled(milli, fractionDigits: 3, locale: .current)
        guard let unit else { return value }
        return String(localized: "ledger.quantity_value", defaultValue: "\(value) \(unit.shortLabel)")
    }

    static func kilometres(_ metres: Int64?) -> String {
        guard let metres else { return String(localized: "ledger.odometer_absent") }
        let value = LedgerUnits.formatScaled(metres, fractionDigits: 3, locale: .current)
        return String(localized: "ledger.distance_kilometres", defaultValue: "\(value) km")
    }

    static func kwh(_ wh: Int64?) -> String {
        guard let wh else { return String(localized: "ledger.charge_energy_absent") }
        let value = LedgerUnits.formatScaled(wh, fractionDigits: 3, locale: .current)
        return String(localized: "ledger.energy_kwh", defaultValue: "\(value) kWh")
    }

    static func monthLabel(year: Int, month: Int) -> String {
        let calendar = Calendar(identifier: .gregorian)
        guard let date = calendar.date(from: DateComponents(year: year, month: month, day: 1)) else { return "—" }
        var style = Date.FormatStyle().month(.wide).year()
        style.calendar = calendar
        return date.formatted(style)
    }

    static func dateTime(millis: Int64, timeZone: TimeZone) -> String {
        let date = Date(timeIntervalSince1970: TimeInterval(millis) / 1_000)
        var style = Date.FormatStyle(date: .abbreviated, time: .shortened)
        style.timeZone = timeZone
        return date.formatted(style)
    }

    /// One-line summary of a record (also used for conflict versions).
    static func summary(_ record: LedgerEntryRecord, vehicles: [VehicleRecord]) -> String {
        let vehicle = vehicles.first(where: { $0.id == record.vehicleId })?.name
            ?? String(localized: "ledger.vehicle_absent")
        let timeZone = TimeZone(identifier: record.timezone) ?? .gmt
        var parts = [vehicle, dateTime(millis: record.occurredAtMillis, timeZone: timeZone)]
        switch record.kind {
        case .refuel:
            parts.append(record.fuel?.localizedLabel ?? String(localized: "ledger.review_fuel"))
            parts.append(quantity(record.quantityMilli, unit: record.fuel?.unit))
            parts.append(record.fill.localizedLabel)
            parts.append(money(record.totalCentavos))
        case .expense:
            parts.append(record.category?.localizedLabel ?? "")
            parts.append(money(record.totalCentavos))
        case .charge:
            parts.append(kwh(record.energyWh))
            parts.append(money(record.totalCentavos))
            if let start = record.socStartPercent, let end = record.socEndPercent {
                parts.append(String(localized: "ledger.charge_range", defaultValue: "\((Double(start) / 100).formatted(.percent)) → \((Double(end) / 100).formatted(.percent))"))
            }
        case .odometer:
            break
        }
        parts.append(kilometres(record.odometerMetres))
        if !record.note.isEmpty {
            parts.append(record.note)
        }
        return parts.joined(separator: " · ")
    }

    static func fuelEfficiency(_ rational: String) -> String? {
        guard let value = LedgerRational.displayText(rational, fractionDigits: 2, locale: .current) else { return nil }
        return String(localized: "ledger.efficiency_km_per_litre", defaultValue: "\(value) km/L")
    }

    static func costPerKilometre(_ centavos: String) -> String? {
        guard let cost = LedgerRational.decimal(centavos, fractionDigits: 6) else { return nil }
        var reais = cost / 100
        var rounded = Decimal()
        NSDecimalRound(&rounded, &reais, 3, .plain)
        let value = rounded.formatted(.currency(code: "BRL").precision(.fractionLength(0...3)))
        return String(localized: "ledger.cost_per_kilometre", defaultValue: "\(value)/km")
    }

    static func exactCostPerKilometre(_ centavos: String) -> String {
        guard !centavos.isEmpty else { return "" }
        return String(localized: "ledger.centavos_per_kilometre", defaultValue: "\(centavos) centavos/km")
    }

    static func energyPerKilometre(_ rational: String) -> String? {
        guard let value = LedgerRational.displayText(rational, fractionDigits: 1, locale: .current) else { return nil }
        return String(localized: "ledger.energy_wh_per_kilometre", defaultValue: "\(value) Wh/km")
    }

    static func percent(_ rational: String) -> String? {
        guard var value = LedgerRational.decimal(rational, fractionDigits: 6) else { return nil }
        var rounded = Decimal()
        NSDecimalRound(&rounded, &value, 1, .plain)
        return (rounded / 100).formatted(.percent.precision(.fractionLength(0...1)))
    }
}

struct LedgerHomeView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator
    let close: () -> Void

    @State private var pendingDelete: LedgerEntryRecord?

    var body: some View {
        let vehicles = viewModel.snapshot.vehicles
        let entries = viewModel.snapshot.ledgerEntries.sorted { lhs, rhs in
            lhs.occurredAtMillis != rhs.occurredAtMillis ? lhs.occurredAtMillis > rhs.occurredAtMillis : lhs.id > rhs.id
        }
        NavigationStack {
            List {
                Section(String(localized: "android.profile_tool_vehicles_title")) {
                    if vehicles.isEmpty {
                        Text(String(localized: "android.vehicle_empty_message")).foregroundStyle(.secondary)
                    }
                    ForEach(vehicles) { vehicle in
                        NavigationLink {
                            VehicleEditorView(viewModel: viewModel, existing: vehicle)
                        } label: {
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(vehicle.name).font(.headline)
                                    Spacer()
                                    Text(vehicle.propulsion.localizedLabel).font(.caption).foregroundStyle(.secondary)
                                }
                                Text(vehicleDetail(vehicle)).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                        .accessibilityHint(String(localized: "android.vehicle_edit"))
                    }
                    .onDelete { offsets in
                        let ids = Set(offsets.map { vehicles[$0].id })
                        viewModel.removeVehicles(ids)
                    }
                    Text(String(localized: "ledger.vehicle_delete_retention"))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }

                addVehicleSection

                Section(String(localized: "ledger.section")) {
                    NavigationLink {
                        LedgerEntryEditorView(viewModel: viewModel, ledger: ledger, existing: nil)
                    } label: {
                        Label(String(localized: "android.ledger_entry_add_title"), systemImage: "plus.circle")
                    }
                    .disabled(vehicles.isEmpty)
                    if vehicles.isEmpty {
                        Text(String(localized: "ledger.no_vehicle_for_entries")).foregroundStyle(.secondary)
                    }
                }

                LedgerIndicatorsSection(viewModel: viewModel, ledger: ledger)

                Section(String(localized: "ledger.cloud_export")) {
                    NavigationLink {
                        LedgerServerAnalysisView(viewModel: viewModel, ledger: ledger)
                    } label: {
                        Label(String(localized: "ledger.server.analysis"), systemImage: "chart.bar.xaxis")
                    }
                    NavigationLink {
                        LedgerSyncView(viewModel: viewModel, ledger: ledger)
                    } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(String(localized: "android.ledger_sync_title")).font(.subheadline.weight(.semibold))
                            Text(syncSubtitle).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    ShareLink(item: ledger.exportCsv()) {
                        Label(String(localized: "android.monthly_export"), systemImage: "square.and.arrow.up")
                    }
                }

                Section(String(localized: "ledger.entries_count", defaultValue: "Entries (\(entries.count.formatted()))")) {
                    if entries.isEmpty {
                        Text(String(localized: "ledger.no_entries")).foregroundStyle(.secondary)
                    }
                    ForEach(entries) { entry in
                        if entry.kind == .odometer {
                            LedgerEntryRow(entry: entry, vehicles: vehicles, status: ledger.status(of: entry))
                        } else {
                            NavigationLink {
                                LedgerEntryEditorView(viewModel: viewModel, ledger: ledger, existing: entry)
                            } label: {
                                LedgerEntryRow(entry: entry, vehicles: vehicles, status: ledger.status(of: entry))
                            }
                        }
                    }
                    .onDelete { offsets in
                        pendingDelete = offsets.first.map { entries[$0] }
                    }
                }
            }
            .navigationTitle(String(localized: "ledger.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close"), action: close)
                }
            }
            .onAppear { ledger.refresh() }
            .task { await ledger.refreshPremiumProof() }
            .alert(
                String(localized: "android.ledger_entry_delete_confirm"),
                isPresented: Binding(
                    get: { pendingDelete != nil },
                    set: { if !$0 { pendingDelete = nil } }
                )
            ) {
                Button(String(localized: "android.vehicle_delete"), role: .destructive) {
                    if let record = pendingDelete {
                        ledger.delete(record)
                    }
                    pendingDelete = nil
                }
                Button(String(localized: "android.action_cancel"), role: .cancel) {
                    pendingDelete = nil
                }
            } message: {
                Text(String(localized: "ledger.delete_sync_message"))
            }
        }
    }

    private var syncSubtitle: String {
        guard viewModel.currentAccountIdOrNil != nil else {
            return String(localized: "ledger.status.local_sign_in")
        }
        guard let state = ledger.syncState else { return String(localized: "ledger.status.local_only") }
        if !state.enabled { return String(localized: "ledger.status.sync_disabled") }
        if !state.conflicts.isEmpty { return String(localized: "ledger.status.conflicts_awaiting") }
        if state.paidUploadsPaused { return String(localized: "ledger.status.uploads_paused") }
        if state.pendingCount > 0 { return String(localized: "ledger.status.pending_count", defaultValue: "Pending uploads: \(state.pendingCount.formatted())") }
        return String(localized: "ledger.status.sync_enabled")
    }

    private func vehicleDetail(_ vehicle: VehicleRecord) -> String {
        var parts: [String] = []
        if vehicle.propulsion.supportsFuel {
            parts.append(vehicle.primaryFuel.localizedName)
            parts.append(String(localized: "ledger.tank", defaultValue: "Tank: \(LedgerUnits.formatScaled(vehicle.tankCapacityMilliLiters, fractionDigits: 3, locale: .current)) L"))
        }
        if vehicle.propulsion.supportsElectric {
            parts.append(String(localized: "ledger.battery", defaultValue: "Battery: \(LedgerUnits.formatScaled(vehicle.batteryCapacityWh, fractionDigits: 3, locale: .current)) kWh"))
        }
        return parts.joined(separator: " · ")
    }

    @ViewBuilder
    private var addVehicleSection: some View {
        let admission = ledger.admissionForNewVehicle()
        Section(String(localized: "android.vehicle_add")) {
            switch admission {
            case .accept:
                NavigationLink {
                    VehicleEditorView(viewModel: viewModel, existing: nil)
                } label: {
                    Label(String(localized: "android.vehicle_add"), systemImage: "plus.circle")
                }
            case .premiumRequired, .hostedAuthorityRequired:
                Text(LedgerCoordinator.admissionMessage(admission)).font(.caption)
                if viewModel.currentAccountIdOrNil == nil {
                    Button(String(localized: "ledger.sign_in_free")) { viewModel.activeSheet = .auth }
                } else {
                    Button(String(localized: "ledger.explore_premium")) { viewModel.activeSheet = .premium }
                    Button(String(localized: "android.premium_refresh_status")) {
                        Task { await ledger.refreshPremiumProof() }
                    }
                }
            case .capacityExceeded:
                Text(LedgerCoordinator.admissionMessage(admission)).font(.caption)
            }
        }
    }
}

struct LedgerEntryRow: View {
    let entry: LedgerEntryRecord
    let vehicles: [VehicleRecord]
    let status: LedgerRecordSyncStatus

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(entry.kind.localizedLabel).font(.subheadline.bold())
                Spacer()
                if entry.kind != .odometer {
                    Text(LedgerFormat.money(entry.totalCentavos)).font(.subheadline.bold())
                }
            }
            Text(LedgerFormat.summary(entry, vehicles: vehicles))
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(entry.kind == .odometer ? String(localized: "ledger.odometer_legacy_local") : LedgerCoordinator.statusLabel(status))
                .font(.caption2)
                .foregroundStyle(statusColor)
        }
        .accessibilityElement(children: .combine)
    }

    private var statusColor: Color {
        switch status {
        case .conflict, .refused: return AbastevoTheme.errorRed
        case .reviewRequired, .paused: return AbastevoTheme.warningLight
        case .synced: return AbastevoTheme.accentLight
        default: return .secondary
        }
    }
}

struct LedgerEntryEditorView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator
    let existing: LedgerEntryRecord?

    @State private var form: LedgerEntryForm
    @State private var occurredAt: Date
    @State private var ledgerVehicleId: String?
    @State private var errorText: String? = nil
    @State private var confirmDelete = false

    init(viewModel: AppStateViewModel, ledger: LedgerCoordinator, existing: LedgerEntryRecord?) {
        self._viewModel = ObservedObject(wrappedValue: viewModel)
        self._ledger = ObservedObject(wrappedValue: ledger)
        self.existing = existing
        let nowMillis = Int64(Date().timeIntervalSince1970 * 1000)
        let base = existing.map { LedgerEntryForm.editing($0) } ?? LedgerEntryForm(occurredAtMillis: nowMillis)
        self._form = State(initialValue: base)
        self._occurredAt = State(initialValue: Date(timeIntervalSince1970: TimeInterval(base.occurredAtMillis) / 1000))
        self._ledgerVehicleId = State(initialValue: existing?.vehicleId)
    }

    private var selectedVehicle: VehicleRecord? {
        guard let ledgerVehicleId else { return nil }
        return viewModel.snapshot.vehicles.first { $0.id == ledgerVehicleId }
    }

    var body: some View {
        Form {
            if existing?.reviewRequired == true {
                Section {
                    Text(String(localized: "ledger.migrated_review"))
                        .font(.caption)
                }
            }
            Section(String(localized: "ledger.vehicle_and_type")) {
                Picker(String(localized: "ledger.vehicle"), selection: $ledgerVehicleId) {
                    Text(String(localized: "ledger.select_vehicle")).tag(nil as String?)
                    ForEach(viewModel.snapshot.vehicles) { vehicle in
                        Text(vehicle.name).tag(Optional(vehicle.id))
                    }
                }
                if existing == nil {
                    Picker(String(localized: "android.occurrence_filter_type"), selection: $form.kind) {
                        ForEach(LedgerEntryKind.recordable, id: \.self) { kind in
                            Text(kind.localizedLabel).tag(kind)
                        }
                    }
                } else {
                    LabeledContent(String(localized: "android.occurrence_filter_type"), value: form.kind.localizedLabel)
                }
                DatePicker(String(localized: "ledger.date_time"), selection: $occurredAt, displayedComponents: [.date, .hourAndMinute])
            }

            switch form.kind {
            case .refuel:
                refuelSection
            case .expense:
                expenseSection
            case .charge:
                chargeSection
            case .odometer:
                Section {
                    Text(String(localized: "ledger.odometer_legacy_read_only")).foregroundStyle(.secondary)
                }
            }

            Section(String(localized: "ledger.odometer_and_gaps")) {
                decimalField(String(localized: "ledger.odometer_optional"), text: $form.odometerKmText)
                Toggle(String(localized: "ledger.odometer_reset"), isOn: $form.opensOdometerSegment)
                Toggle(String(localized: "ledger.unrecorded_fill"), isOn: $form.missedFillBefore)
                Text(String(localized: "ledger.gaps_effect"))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }

            Section(String(localized: "ledger.note")) {
                TextField(String(localized: "ledger.note_limit"), text: $form.note)
            }

            if let errorText {
                Section {
                    Text(errorText).foregroundStyle(.red)
                }
            }

            Section {
                Button(existing == nil ? String(localized: "ledger.add_entry") : String(localized: "android.forum_edit_save"), action: save)
                    .disabled(ledgerVehicleId == nil)
                if existing != nil {
                    Button(String(localized: "ledger.delete_entry"), role: .destructive) {
                        confirmDelete = true
                    }
                }
            }
        }
        .navigationTitle(existing == nil ? String(localized: "android.ledger_entry_add_title") : String(localized: "android.ledger_entry_edit_title"))
        .alert(String(localized: "android.ledger_entry_delete_confirm"), isPresented: $confirmDelete) {
            Button(String(localized: "android.vehicle_delete"), role: .destructive) {
                if let existing {
                    ledger.delete(existing)
                }
                dismiss()
            }
            Button(String(localized: "android.action_cancel"), role: .cancel) {}
        } message: {
            Text(String(localized: "ledger.delete_irreversible"))
        }
    }

    private var refuelSection: some View {
        Section(String(localized: "android.ledger_entry_add_refuel")) {
            Picker(String(localized: "android.history_fuel_product_label"), selection: $form.fuel) {
                Text(defaultFuelLabel).tag(nil as LedgerFuel?)
                ForEach(LedgerFuel.allCases, id: \.self) { fuel in
                    Text(fuel.localizedLabel).tag(Optional(fuel))
                }
            }
            decimalField(String(localized: "ledger.quantity", defaultValue: "Quantity (\(unitLabel), e.g. \((40.5).formatted(.number)))"), text: $form.quantityText)
            decimalField(String(localized: "ledger.paid_amount"), text: $form.totalText)
            decimalField(String(localized: "ledger.unit_price", defaultValue: "Price per \(unitLabel) (optional, e.g. \((5.899).formatted(.number)))"), text: $form.unitPriceText)
            Picker(String(localized: "ledger.fill"), selection: $form.fill) {
                ForEach(LedgerFill.allCases, id: \.self) { fill in
                    Text(fill.localizedLabel).tag(fill)
                }
            }
            Text(String(localized: "ledger.refuel_calculation"))
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var expenseSection: some View {
        Section(String(localized: "android.ledger_entry_add_expense")) {
            decimalField(String(localized: "ledger.amount"), text: $form.totalText)
            Picker(String(localized: "android.forum_topic_category_label"), selection: $form.category) {
                ForEach(LedgerExpenseCategory.allCases, id: \.self) { category in
                    Text(category.localizedLabel).tag(category)
                }
            }
        }
    }

    private var chargeSection: some View {
        Section(String(localized: "android.ledger_entry_add_charge")) {
            decimalField(String(localized: "ledger.charge_energy_optional"), text: $form.energyKwhText)
            decimalField(String(localized: "ledger.charge_amount_optional"), text: $form.totalText)
            decimalField(String(localized: "ledger.charge_start_optional"), text: $form.socStartText)
            decimalField(String(localized: "ledger.charge_end_optional"), text: $form.socEndText)
            decimalField(String(localized: "ledger.charge_fees_optional"), text: $form.feesText)
            Text(String(localized: "ledger.charge_fees_separate"))
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var unitLabel: String {
        let fuel = form.fuel ?? selectedVehicle.flatMap { LedgerFuel.from(vehicleFuel: $0.primaryFuel) }
        return fuel?.unit.shortLabel ?? "L"
    }

    private var defaultFuelLabel: String {
        guard let vehicle = selectedVehicle, let fuel = LedgerFuel.from(vehicleFuel: vehicle.primaryFuel) else {
            return String(localized: "ledger.vehicle_fuel")
        }
        return String(localized: "ledger.vehicle_fuel_specific", defaultValue: "Vehicle fuel (\(fuel.localizedLabel))")
    }

    @ViewBuilder
    private func decimalField(_ title: String, text: Binding<String>) -> some View {
        TextField(title, text: text)
            #if os(iOS)
            .keyboardType(.decimalPad)
            #endif
    }

    private func save() {
        var draft = form
        draft.vehicleId = ledgerVehicleId
        draft.occurredAtMillis = Int64(occurredAt.timeIntervalSince1970 * 1000)
        switch ledger.save(form: draft, editing: existing) {
        case .success:
            errorText = nil
            dismiss()
        case .failure(let error):
            errorText = error.localizedDescription
        }
    }
}

struct LedgerIndicatorsSection: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator

    private struct EstimateEditorID: Hashable {
        let scope: LedgerEstimateScope
        let vehicleId: String
    }

    @State private var indicatorVehicleId: String?
    @State private var year = Calendar(identifier: .gregorian).component(.year, from: Date())
    @State private var month = Calendar(identifier: .gregorian).component(.month, from: Date())

    var body: some View {
        Section(String(localized: "android.monthly_title")) {
            if viewModel.snapshot.vehicles.isEmpty {
                Text(String(localized: "ledger.no_vehicle_for_metrics")).foregroundStyle(.secondary)
            } else {
                Picker(String(localized: "ledger.vehicle"), selection: $indicatorVehicleId) {
                    Text(String(localized: "ledger.select_vehicle")).tag(nil as String?)
                    ForEach(viewModel.snapshot.vehicles) { vehicle in
                        Text(vehicle.name).tag(Optional(vehicle.id))
                    }
                }
                HStack {
                    Button {
                        shiftMonth(-1)
                    } label: {
                        Image(systemName: "chevron.left")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel(String(localized: "android.monthly_previous"))
                    Spacer()
                    Text(LedgerFormat.monthLabel(year: year, month: month)).font(.subheadline.weight(.semibold))
                    Spacer()
                    Button {
                        shiftMonth(1)
                    } label: {
                        Image(systemName: "chevron.right")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel(String(localized: "android.monthly_next"))
                }
                if let scope = ledger.estimateScope,
                   let vehicle = viewModel.snapshot.vehicles.first(where: { $0.id == indicatorVehicleId }) {
                    LedgerEstimateEditor(ledger: ledger, vehicleId: vehicle.id, scope: scope)
                        .id(EstimateEditorID(scope: scope, vehicleId: vehicle.id))
                    switch ledger.indicators(vehicle: vehicle, year: year, month: month) {
                    case .success(let result):
                        LedgerIndicatorsContent(result: result)
                        if let estimate = ledger.estimatedEfficiency(for: vehicle.id, in: scope)?.fallback(for: result) {
                            let value = estimate.formatted(locale: .current)
                            Text(String(localized: "android.monthly_estimated", defaultValue: "Estimated efficiency: \(value) km/L"))
                                .font(.subheadline)
                        }
                        LedgerMonthlyExportLink(report: result)
                    case .failure(let error):
                        Text(LedgerCoordinator.reasonLabel(error.rawValue)).foregroundStyle(.secondary)
                    }
                }
            }
        }
        .onChange(of: ledger.estimateScope) { _, _ in indicatorVehicleId = nil }
    }

    private func shiftMonth(_ delta: Int) {
        var nextMonth = month + delta
        var nextYear = year
        if nextMonth < 1 {
            nextMonth = 12
            nextYear -= 1
        } else if nextMonth > 12 {
            nextMonth = 1
            nextYear += 1
        }
        guard (2000...2100).contains(nextYear) else { return }
        month = nextMonth
        year = nextYear
    }
}

struct LedgerIndicatorsContent: View {
    let result: LedgerMonthlyIndicators

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(String(localized: "ledger.metrics.records_count", defaultValue: "Entries this month: \(result.recordCount.formatted())")).font(.caption).foregroundStyle(.secondary)
            LabeledContent(String(localized: "ledger.metrics.fuel_purchases"), value: LedgerFormat.money(result.purchaseCentavos))
            LabeledContent(String(localized: "android.ledger_sync_expense"), value: LedgerFormat.money(result.expenseCentavos))
            ForEach(LedgerExpenseCategory.allCases.filter { result.byCategory[$0] != nil }, id: \.self) { category in
                LabeledContent(category.localizedLabel, value: LedgerFormat.money(result.byCategory[category]))
                    .font(.caption)
                    .padding(.leading, 6)
            }
            if result.chargeCostCentavos > 0 || result.chargeEnergyWh > 0 || result.chargeFeesCentavos > 0 {
                LabeledContent(String(localized: "android.ledger_sync_charge"), value: LedgerFormat.money(result.chargeCostCentavos))
                LabeledContent(String(localized: "ledger.metrics.energy_bought"), value: LedgerFormat.kwh(result.chargeEnergyWh))
                LabeledContent(String(localized: "ledger.metrics.charge_fees"), value: LedgerFormat.money(result.chargeFeesCentavos))
            }
            Divider()
            metric(
                title: String(localized: "ledger.metrics.measured"),
                value: result.measured.unknown.isEmpty
                    ? LedgerFormat.fuelEfficiency(result.measured.kmPerUnit)
                    : nil,
                exact: result.measured.kmPerUnit,
                reason: result.measured.unknown
            )
            metric(
                title: String(localized: "ledger.metrics.fuel_cost"),
                value: result.cost.unknown.isEmpty
                    ? LedgerFormat.costPerKilometre(result.cost.centavosPerKm)
                    : nil,
                exact: LedgerFormat.exactCostPerKilometre(result.cost.centavosPerKm),
                reason: result.cost.unknown
            )
            metric(
                title: String(localized: "ledger.metrics.combined_cost"),
                value: result.combined.unknown.isEmpty
                    ? LedgerFormat.costPerKilometre(result.combined.centavosPerKm)
                    : nil,
                exact: LedgerFormat.exactCostPerKilometre(result.combined.centavosPerKm),
                reason: result.combined.unknown
            )
            metric(
                title: String(localized: "ledger.metrics.energy_measured"),
                value: result.energyMeasured.unknown.isEmpty
                    ? LedgerFormat.energyPerKilometre(result.energyMeasured.whPerKm)
                    : nil,
                exact: result.energyMeasured.whPerKm,
                reason: result.energyMeasured.unknown
            )
            metric(
                title: String(localized: "ledger.metrics.energy_reference"),
                value: result.energyReference.unknown.isEmpty
                    ? LedgerFormat.energyPerKilometre(result.energyReference.whPerKm)
                    : nil,
                exact: result.energyReference.whPerKm,
                reason: result.energyReference.unknown
            )
            metric(
                title: String(localized: "ledger.metrics.efficiency_change"),
                value: result.comparison.unknown.isEmpty
                    ? LedgerFormat.percent(result.comparison.changePercent)
                    : nil,
                exact: result.comparison.changePercent,
                reason: result.comparison.unknown
            )
            Text(String(localized: "ledger.metrics.basis", defaultValue: "Eligible distance: \(LedgerFormat.kilometres(result.windowNote.distanceMetres)) · calculation v\(String(LedgerBounds.calculationVersion)). Based on purchases rather than fuel inventory. Missing entries affect the result."))
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private func metric(title: String, value: String?, exact: String, reason: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(.caption.weight(.semibold))
            if let value {
                Text(value).font(.subheadline)
                if !exact.isEmpty {
                    Text(String(localized: "ledger.metrics.exact", defaultValue: "Exact: \(exact)")).font(.caption2.monospaced()).foregroundStyle(.secondary)
                }
            } else {
                Text(String(localized: "ledger.metrics.unavailable", defaultValue: "Unavailable: \(LedgerCoordinator.reasonLabel(reason))"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .accessibilityElement(children: .combine)
    }
}

struct LedgerSyncView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator

    @State private var confirmEnable = false
    @State private var confirmErase = false

    var body: some View {
        List {
            if viewModel.currentAccountIdOrNil == nil {
                Section {
                    Text(String(localized: "ledger.sync.signed_out"))
                    Button(String(localized: "android.profile_action_sign_in")) { viewModel.activeSheet = .auth }
                }
            } else if let state = ledger.syncState {
                statusSection(state)
                pullSection
                if !state.conflicts.isEmpty {
                    Section(String(localized: "ledger.sync.conflicts_count", defaultValue: "Conflicts (\(state.conflicts.count.formatted()))")) {
                        ForEach(state.conflicts) { conflict in
                            NavigationLink {
                                LedgerConflictView(viewModel: viewModel, ledger: ledger, conflictKey: conflict.key)
                            } label: {
                                Text(conflictTitle(conflict))
                            }
                        }
                    }
                }
                let refused = state.commands.filter { $0.status == .refused }
                if !refused.isEmpty {
                    Section(String(localized: "ledger.sync.refused_count", defaultValue: "Refused by server (\(refused.count.formatted()))")) {
                        ForEach(refused) { command in
                            VStack(alignment: .leading, spacing: 2) {
                                Text(command.isDelete ? String(localized: "ledger.sync.deletion") : command.dataset.localizedLabel)
                                    .font(.subheadline)
                                Text(LedgerCoordinator.codeLabel(command.lastCode)).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                        Text(String(localized: "ledger.sync.refused_next_steps"))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                exitSection
            }
            if let notice = ledger.notice {
                Section {
                    Text(notice)
                    Button(String(localized: "android.map_downloads_ok")) { ledger.dismissNotice() }
                }
            }
        }
        .navigationTitle(String(localized: "android.ledger_sync_title"))
        .onAppear { ledger.refresh() }
        .alert(String(localized: "ledger.sync.enable_prompt"), isPresented: $confirmEnable) {
            Button(String(localized: "android.map_activate")) { ledger.setSyncEnabled(true) }
            Button(String(localized: "android.action_cancel"), role: .cancel) {}
        } message: {
            Text(String(localized: "ledger.sync.enable_explanation", defaultValue: "Entries to upload to this account: \(viewModel.snapshot.ledgerEntries.count.formatted()). Photos and location are excluded. Premium (professional.write) is required. Deleting and exporting are still free."))
        }
        .alert(String(localized: "ledger.sync.erase_prompt"), isPresented: $confirmErase) {
            Button(String(localized: "ledger.sync.erase_action"), role: .destructive) { ledger.eraseCloudLedger() }
            Button(String(localized: "android.action_cancel"), role: .cancel) {}
        } message: {
            Text(String(localized: "ledger.sync.erase_explanation"))
        }
    }

    private func statusSection(_ state: LedgerSyncState) -> some View {
        Section(String(localized: "ledger.sync.section")) {
            Toggle(
                String(localized: "ledger.sync.upload_toggle"),
                isOn: Binding(
                    get: { state.enabled },
                    set: { value in
                        if value {
                            confirmEnable = true
                        } else {
                            ledger.setSyncEnabled(false)
                        }
                    }
                )
            )
            .disabled(ledger.isWorking)
            if state.enabled {
                Text(String(localized: "ledger.sync.consented_datasets", defaultValue: "Authorized datasets: \(state.consentedDatasets.map(\.localizedLabel).formatted(.list(type: .and)))"))
                    .font(.caption)
            }
            Text(String(localized: "ledger.sync.counts", defaultValue: "Pending: \(state.pendingCount.formatted()) · Conflicts: \(state.conflicts.count.formatted()) · Refused: \(state.refusedCount.formatted())"))
                .font(.caption)
            if state.paidUploadsPaused {
                Text(String(localized: "ledger.sync.paused_explanation"))
                    .font(.caption)
                    .foregroundStyle(AbastevoTheme.warningLight)
                Button(String(localized: "ledger.explore_premium")) { viewModel.activeSheet = .premium }
            }
            if let last = state.lastSuccessAtMillis {
                Text(String(localized: "ledger.sync.last_upload", defaultValue: "Last confirmed upload: \(LedgerFormat.dateTime(millis: last, timeZone: .current))"))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            Button(ledger.isSyncing ? String(localized: "ledger.sync.syncing") : String(localized: "android.onboarding_action_sync_now")) { ledger.syncNow() }
                .disabled(!state.enabled || ledger.isSyncing)
            Text(String(localized: "ledger.sync.local_markers"))
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var pullSection: some View {
        Section(String(localized: "ledger.sync.pull_section")) {
            Text(String(localized: "ledger.sync.pull_explanation"))
                .font(.caption)
            Button(ledger.isPulling ? String(localized: "ledger.sync.downloading") : String(localized: "ledger.sync.pull_action")) { ledger.pull() }
                .disabled(ledger.isPulling)
            if let report = ledger.lastPullReport {
                Text(String(localized: "ledger.sync.pull_counts", defaultValue: "New: \(report.added.formatted()) · Updated: \(report.updated.formatted()) · Unchanged: \(report.unchanged.formatted()) · Local edits kept: \(report.skippedHeld.formatted()) · Deleted here: \(report.skippedDeleted.formatted()) · Voided: \(report.skippedVoided.formatted()) · Unreadable: \(report.undecodable.formatted())"))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                if report.truncated {
                    Text(String(localized: "ledger.sync.more_pages"))
                        .font(.caption2)
                }
            }
        }
    }

    private var exitSection: some View {
        Section(String(localized: "ledger.sync.your_data")) {
            ShareLink(item: ledger.exportCsv()) {
                Label(String(localized: "android.monthly_export"), systemImage: "square.and.arrow.up")
            }
            Button(String(localized: "ledger.sync.erase_ledger_action"), role: .destructive) { confirmErase = true }
                .disabled(ledger.isWorking)
        }
    }

    private func conflictTitle(_ conflict: LedgerConflict) -> String {
        let dataset = conflict.dataset.localizedLabel
        guard let record = LedgerWire.decodePayload(json: conflict.proposedPayloadJson, dataset: conflict.dataset) else {
            return dataset
        }
        let date = LedgerFormat.dateTime(
            millis: record.occurredAtMillis,
            timeZone: TimeZone(identifier: record.timezone) ?? .gmt
        )
        return dataset + " · " + date
    }
}

struct LedgerConflictView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator
    let conflictKey: String

    var body: some View {
        List {
            if let conflict = ledger.syncState?.conflicts.first(where: { $0.key == conflictKey }) {
                Section(String(localized: "ledger.conflict.base")) {
                    versionText(conflict.basePayloadJson, dataset: conflict.dataset, empty: String(localized: "ledger.conflict.no_base"))
                }
                Section(String(localized: "ledger.conflict.current")) {
                    switch conflict.current.state {
                    case .unknown:
                        Text(String(localized: "ledger.conflict.current_not_loaded")).foregroundStyle(.secondary)
                    case .missing:
                        Text(String(localized: "ledger.conflict.deleted_elsewhere"))
                    case .present:
                        if conflict.current.voided {
                            Text(String(localized: "ledger.conflict.voided"))
                        }
                        versionText(conflict.current.payloadJson, dataset: conflict.dataset, empty: String(localized: "ledger.conflict.unreadable"))
                        if let revision = conflict.current.revision {
                            Text(String(localized: "ledger.conflict.revision", defaultValue: "Revision \(revision.formatted())"))
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                Section(String(localized: "ledger.conflict.proposed")) {
                    versionText(conflict.proposedPayloadJson, dataset: conflict.dataset, empty: "")
                }
                Section(String(localized: "ledger.conflict.decision")) {
                    Button(String(localized: "android.ledger_sync_keep_mine")) {
                        ledger.resolveConflict(conflict, choice: .keepMine)
                        dismiss()
                    }
                    .disabled(conflict.current.state != .present || conflict.current.voided)
                    Button(String(localized: "android.ledger_sync_take_server")) {
                        ledger.resolveConflict(conflict, choice: .takeServer)
                        dismiss()
                    }
                    .disabled(conflict.current.state == .unknown)
                    Button(String(localized: "ledger.conflict.refresh")) { ledger.refreshConflictCurrent(conflict) }
                        .disabled(ledger.isWorking)
                    Button(String(localized: "ledger.conflict.decide_later")) { dismiss() }
                    Text(String(localized: "ledger.conflict.keep_explanation"))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            } else {
                Text(String(localized: "ledger.conflict.resolved")).foregroundStyle(.secondary)
            }
        }
        .navigationTitle(String(localized: "android.ledger_entry_state_conflict"))
    }

    @ViewBuilder
    private func versionText(_ payload: String?, dataset: LedgerDataset, empty: String) -> some View {
        if let payload, let record = LedgerWire.decodePayload(json: payload, dataset: dataset) {
            Text(LedgerFormat.summary(record, vehicles: viewModel.snapshot.vehicles)).font(.caption)
        } else {
            Text(empty).foregroundStyle(.secondary)
        }
    }
}
