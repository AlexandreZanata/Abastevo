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
        return LedgerUnits.formatScaled(milli, fractionDigits: 3) + " " + (unit?.shortLabel ?? "")
    }

    static func kilometres(_ metres: Int64?) -> String {
        guard let metres else { return "sem hodômetro" }
        return LedgerUnits.formatScaled(metres, fractionDigits: 3) + " km"
    }

    static func kwh(_ wh: Int64?) -> String {
        guard let wh else { return "energia não informada" }
        return LedgerUnits.formatScaled(wh, fractionDigits: 3) + " kWh"
    }

    static func monthLabel(year: Int, month: Int) -> String {
        let names = ["janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto",
                     "setembro", "outubro", "novembro", "dezembro"]
        let name = (1...12).contains(month) ? names[month - 1] : String(month)
        return "\(name) de \(year)"
    }

    /// One-line summary of a record (also used for conflict versions).
    static func summary(_ record: LedgerEntryRecord, vehicles: [VehicleRecord]) -> String {
        let vehicle = vehicles.first(where: { $0.id == record.vehicleId })?.name
            ?? String(localized: "veículo não cadastrado neste aparelho")
        var parts: [String] = [vehicle, record.dateLabel]
        switch record.kind {
        case .refuel:
            parts.append(record.fuel?.labelPtBr ?? String(localized: "combustível a revisar"))
            parts.append(quantity(record.quantityMilli, unit: record.fuel?.unit))
            parts.append(record.fill.labelPtBr)
            parts.append(money(record.totalCentavos))
        case .expense:
            parts.append(record.category?.labelPtBr ?? "")
            parts.append(money(record.totalCentavos))
        case .charge:
            parts.append(kwh(record.energyWh))
            parts.append(money(record.totalCentavos))
            if let start = record.socStartPercent, let end = record.socEndPercent {
                parts.append("\(start)% → \(end)%")
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

    static func rational(_ value: String, fractionDigits: Int, divideBy: Decimal = 1) -> String {
        return LedgerRational.displayText(value, fractionDigits: fractionDigits, divideBy: divideBy) ?? value
    }
}

struct LedgerHomeView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var ledger: LedgerCoordinator
    let close: () -> Void

    @State private var name = ""
    @State private var propulsion: Propulsion = .flex
    @State private var primaryFuel: FuelProduct = .gasolineRegular
    @State private var tankLiters = "50"
    @State private var batteryKwh = "52"
    @State private var vehicleError: String?
    @State private var pendingDelete: LedgerEntryRecord?

    var body: some View {
        let vehicles = viewModel.snapshot.vehicles
        let entries = viewModel.snapshot.ledgerEntries.sorted { lhs, rhs in
            lhs.occurredAtMillis != rhs.occurredAtMillis ? lhs.occurredAtMillis > rhs.occurredAtMillis : lhs.id > rhs.id
        }
        NavigationStack {
            List {
                Section("Meus veículos") {
                    if vehicles.isEmpty {
                        Text("Nenhum veículo cadastrado.").foregroundStyle(.secondary)
                    }
                    ForEach(vehicles) { vehicle in
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(vehicle.name).font(.headline)
                                Spacer()
                                Text(vehicle.propulsion.labelPtBr).font(.caption).foregroundStyle(.secondary)
                            }
                            Text(vehicleDetail(vehicle)).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    .onDelete { offsets in
                        let ids = Set(offsets.map { vehicles[$0].id })
                        viewModel.removeVehicles(ids)
                    }
                    Text("Remover um veículo mantém os lançamentos dele neste aparelho.")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }

                addVehicleSection

                Section("Diário") {
                    NavigationLink {
                        LedgerEntryEditorView(viewModel: viewModel, ledger: ledger, existing: nil)
                    } label: {
                        Label("Novo lançamento", systemImage: "plus.circle")
                    }
                    .disabled(vehicles.isEmpty)
                    if vehicles.isEmpty {
                        Text("Cadastre um veículo para registrar lançamentos.").foregroundStyle(.secondary)
                    }
                }

                LedgerIndicatorsSection(viewModel: viewModel, ledger: ledger)

                Section("Nuvem e exportação") {
                    NavigationLink {
                        LedgerSyncView(viewModel: viewModel, ledger: ledger)
                    } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Sincronização do diário").font(.subheadline.weight(.semibold))
                            Text(syncSubtitle).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    ShareLink(item: ledger.exportCsv()) {
                        Label("Exportar diário (CSV)", systemImage: "square.and.arrow.up")
                    }
                }

                Section("Lançamentos (\(entries.count))") {
                    if entries.isEmpty {
                        Text("Nenhum lançamento registrado.").foregroundStyle(.secondary)
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
            .navigationTitle("Veículos & Diário")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar", action: close)
                }
            }
            .onAppear { ledger.refresh() }
            .task { await ledger.refreshPremiumProof() }
            .alert(
                "Excluir lançamento?",
                isPresented: Binding(
                    get: { pendingDelete != nil },
                    set: { if !$0 { pendingDelete = nil } }
                )
            ) {
                Button("Excluir", role: .destructive) {
                    if let record = pendingDelete {
                        ledger.delete(record)
                    }
                    pendingDelete = nil
                }
                Button("Cancelar", role: .cancel) {
                    pendingDelete = nil
                }
            } message: {
                Text("O lançamento sai deste aparelho. Se já foi enviado, a exclusão gratuita também é enviada à nuvem.")
            }
        }
    }

    private var syncSubtitle: String {
        guard viewModel.currentAccountIdOrNil != nil else {
            return String(localized: "Somente neste aparelho · entre para sincronizar")
        }
        guard let state = ledger.syncState else { return String(localized: "Somente neste aparelho") }
        if !state.enabled { return String(localized: "Desativada · registros somente neste aparelho") }
        if !state.conflicts.isEmpty { return String(localized: "Conflitos aguardando sua decisão") }
        if state.paidUploadsPaused { return String(localized: "Envios pausados: Premium necessário") }
        if state.pendingCount > 0 { return String(localized: "Envios pendentes: \(state.pendingCount)") }
        return String(localized: "Ativada")
    }

    private func vehicleDetail(_ vehicle: VehicleRecord) -> String {
        var parts: [String] = []
        if vehicle.propulsion.supportsFuel {
            parts.append(vehicle.primaryFuel.displayNamePtBr)
            parts.append(String(localized: "Tanque: \(LedgerUnits.formatScaled(vehicle.tankCapacityMilliLiters, fractionDigits: 3)) L"))
        }
        if vehicle.propulsion.supportsElectric {
            parts.append(String(localized: "Bateria: \(LedgerUnits.formatScaled(vehicle.batteryCapacityWh, fractionDigits: 3)) kWh"))
        }
        return parts.joined(separator: " · ")
    }

    @ViewBuilder
    private var addVehicleSection: some View {
        let admission = ledger.admissionForNewVehicle()
        Section("Adicionar veículo") {
            switch admission {
            case .accept:
                TextField("Apelido do carro / moto", text: $name)
                Picker("Propulsão", selection: $propulsion) {
                    ForEach(Propulsion.allCases) { option in
                        Text(option.labelPtBr).tag(option)
                    }
                }
                if propulsion.supportsFuel {
                    Picker("Combustível principal", selection: $primaryFuel) {
                        ForEach(FuelProduct.allCases) { fuel in
                            Text(fuel.displayNamePtBr).tag(fuel)
                        }
                    }
                    TextField("Tanque (litros, ex.: 50)", text: $tankLiters)
                        #if os(iOS)
                        .keyboardType(.decimalPad)
                        #endif
                }
                if propulsion.supportsElectric {
                    TextField("Bateria (kWh, ex.: 52)", text: $batteryKwh)
                        #if os(iOS)
                        .keyboardType(.decimalPad)
                        #endif
                }
                if let vehicleError {
                    Text(vehicleError).font(.caption).foregroundStyle(.red)
                }
                Button("Salvar veículo") {
                    switch viewModel.addVehicle(
                        name: name,
                        propulsion: propulsion,
                        primaryFuel: primaryFuel,
                        tankLitersText: tankLiters,
                        batteryKwhText: batteryKwh
                    ) {
                    case .success:
                        name = ""
                        vehicleError = nil
                    case .failure(let error):
                        vehicleError = error.localizedDescription
                    }
                }
            case .premiumRequired, .hostedAuthorityRequired:
                Text(LedgerCoordinator.admissionMessage(admission)).font(.caption)
                if viewModel.currentAccountIdOrNil == nil {
                    Button("Entrar na conta gratuita") { viewModel.activeSheet = .auth }
                } else {
                    Button("Conhecer o Abastevo Premium") { viewModel.activeSheet = .premium }
                    Button("Verificar assinatura novamente") {
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
                Text(entry.kind.labelPtBr).font(.subheadline.bold())
                Spacer()
                if entry.kind != .odometer {
                    Text(LedgerFormat.money(entry.totalCentavos)).font(.subheadline.bold())
                }
            }
            Text(LedgerFormat.summary(entry, vehicles: vehicles))
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(entry.kind == .odometer ? String(localized: "Leitura antiga de hodômetro · somente neste aparelho") : LedgerCoordinator.statusLabel(status))
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
                    Text("Registro migrado da versão anterior: confira data e hora, combustível, tipo de enchimento e hodômetro, e salve para concluir a revisão. Até lá ele não é sincronizado.")
                        .font(.caption)
                }
            }
            Section("Veículo e tipo") {
                Picker("Veículo", selection: $ledgerVehicleId) {
                    Text("Selecione um veículo").tag(nil as String?)
                    ForEach(viewModel.snapshot.vehicles) { vehicle in
                        Text(vehicle.name).tag(Optional(vehicle.id))
                    }
                }
                if existing == nil {
                    Picker("Tipo", selection: $form.kind) {
                        ForEach(LedgerEntryKind.recordable, id: \.self) { kind in
                            Text(kind.labelPtBr).tag(kind)
                        }
                    }
                } else {
                    LabeledContent("Tipo", value: form.kind.labelPtBr)
                }
                DatePicker("Data e hora", selection: $occurredAt, displayedComponents: [.date, .hourAndMinute])
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
                    Text("Leituras antigas de hodômetro não podem ser editadas.").foregroundStyle(.secondary)
                }
            }

            Section("Hodômetro e intervalos") {
                decimalField("Hodômetro (km, opcional)", text: $form.odometerKmText)
                Toggle("Hodômetro zerado ou trocado antes desta leitura", isOn: $form.opensOdometerSegment)
                Toggle("Houve abastecimento ou recarga não registrado antes deste", isOn: $form.missedFillBefore)
                Text("Essas marcações ficam neste aparelho e tornam indisponível o consumo do período afetado.")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }

            Section("Observação") {
                TextField("Observação (até 280 caracteres)", text: $form.note)
            }

            if let errorText {
                Section {
                    Text(errorText).foregroundStyle(.red)
                }
            }

            Section {
                Button(existing == nil ? "Registrar lançamento" : "Salvar alterações", action: save)
                    .disabled(ledgerVehicleId == nil)
                if existing != nil {
                    Button("Excluir lançamento", role: .destructive) {
                        confirmDelete = true
                    }
                }
            }
        }
        .navigationTitle(existing == nil ? "Novo lançamento" : "Editar lançamento")
        .alert("Excluir lançamento?", isPresented: $confirmDelete) {
            Button("Excluir", role: .destructive) {
                if let existing {
                    ledger.delete(existing)
                }
                dismiss()
            }
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text("A exclusão é gratuita e não pode ser desfeita neste aparelho.")
        }
    }

    private var refuelSection: some View {
        Section("Abastecimento") {
            Picker("Combustível", selection: $form.fuel) {
                Text(defaultFuelLabel).tag(nil as LedgerFuel?)
                ForEach(LedgerFuel.allCases, id: \.self) { fuel in
                    Text(fuel.labelPtBr).tag(Optional(fuel))
                }
            }
            decimalField("Quantidade (\(unitLabel), ex.: 40,5)", text: $form.quantityText)
            decimalField("Valor pago (R$)", text: $form.totalText)
            decimalField("Preço por \(unitLabel) (opcional, ex.: 5,899)", text: $form.unitPriceText)
            Picker("Enchimento", selection: $form.fill) {
                ForEach(LedgerFill.allCases, id: \.self) { fill in
                    Text(fill.labelPtBr).tag(fill)
                }
            }
            Text("O consumo usa dois abastecimentos de tanque cheio consecutivos; o valor pago é o total real, sem recalcular pelo preço.")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var expenseSection: some View {
        Section("Despesa") {
            decimalField("Valor (R$)", text: $form.totalText)
            Picker("Categoria", selection: $form.category) {
                ForEach(LedgerExpenseCategory.allCases, id: \.self) { category in
                    Text(category.labelPtBr).tag(category)
                }
            }
        }
    }

    private var chargeSection: some View {
        Section("Recarga elétrica") {
            decimalField("Energia (kWh, opcional)", text: $form.energyKwhText)
            decimalField("Valor pago (R$, opcional)", text: $form.totalText)
            decimalField("Carga inicial (%, opcional)", text: $form.socStartText)
            decimalField("Carga final (%, opcional)", text: $form.socEndText)
            decimalField("Taxas (R$, opcional)", text: $form.feesText)
            Text("Taxas são mostradas à parte e nunca somadas automaticamente ao valor da recarga.")
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
            return String(localized: "Combustível do veículo")
        }
        return String(localized: "Do veículo (\(fuel.labelPtBr))")
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

    @State private var indicatorVehicleId: String?
    @State private var year = Calendar.current.component(.year, from: Date())
    @State private var month = Calendar.current.component(.month, from: Date())

    var body: some View {
        Section("Indicadores do mês") {
            if viewModel.snapshot.vehicles.isEmpty {
                Text("Cadastre um veículo para ver indicadores.").foregroundStyle(.secondary)
            } else {
                Picker("Veículo", selection: $indicatorVehicleId) {
                    Text("Selecione um veículo").tag(nil as String?)
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
                    .accessibilityLabel("Mês anterior")
                    Spacer()
                    Text(LedgerFormat.monthLabel(year: year, month: month)).font(.subheadline.weight(.semibold))
                    Spacer()
                    Button {
                        shiftMonth(1)
                    } label: {
                        Image(systemName: "chevron.right")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel("Próximo mês")
                }
                if let vehicle = viewModel.snapshot.vehicles.first(where: { $0.id == indicatorVehicleId }) {
                    switch ledger.indicators(vehicle: vehicle, year: year, month: month) {
                    case .success(let result):
                        LedgerIndicatorsContent(result: result)
                    case .failure(let error):
                        Text(LedgerCoordinator.reasonLabel(error.rawValue)).foregroundStyle(.secondary)
                    }
                }
            }
        }
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
            Text("Registros no mês: \(result.recordCount)").font(.caption).foregroundStyle(.secondary)
            LabeledContent("Combustível (compras)", value: LedgerFormat.money(result.purchaseCentavos))
            LabeledContent("Despesas", value: LedgerFormat.money(result.expenseCentavos))
            ForEach(LedgerExpenseCategory.allCases.filter { result.byCategory[$0] != nil }, id: \.self) { category in
                LabeledContent("  \(category.labelPtBr)", value: LedgerFormat.money(result.byCategory[category]))
                    .font(.caption)
            }
            if result.chargeCostCentavos > 0 || result.chargeEnergyWh > 0 || result.chargeFeesCentavos > 0 {
                LabeledContent("Recargas", value: LedgerFormat.money(result.chargeCostCentavos))
                LabeledContent("Energia comprada", value: LedgerFormat.kwh(result.chargeEnergyWh))
                LabeledContent("Taxas de recarga (à parte)", value: LedgerFormat.money(result.chargeFeesCentavos))
            }
            Divider()
            metric(
                title: String(localized: "Consumo medido"),
                value: result.measured.unknown.isEmpty
                    ? LedgerFormat.rational(result.measured.kmPerUnit, fractionDigits: 2) + " km/L"
                    : nil,
                exact: result.measured.kmPerUnit,
                reason: result.measured.unknown
            )
            metric(
                title: String(localized: "Custo de combustível por km (intervalo de compras)"),
                value: result.cost.unknown.isEmpty
                    ? "R$ " + LedgerFormat.rational(result.cost.centavosPerKm, fractionDigits: 3, divideBy: 100) + "/km"
                    : nil,
                exact: result.cost.centavosPerKm.isEmpty ? "" : result.cost.centavosPerKm + " centavos/km",
                reason: result.cost.unknown
            )
            metric(
                title: String(localized: "Custo combinado por km (combustível + recarga)"),
                value: result.combined.unknown.isEmpty
                    ? "R$ " + LedgerFormat.rational(result.combined.centavosPerKm, fractionDigits: 3, divideBy: 100) + "/km"
                    : nil,
                exact: result.combined.centavosPerKm.isEmpty ? "" : result.combined.centavosPerKm + " centavos/km",
                reason: result.combined.unknown
            )
            metric(
                title: String(localized: "Energia medida"),
                value: result.energyMeasured.unknown.isEmpty
                    ? LedgerFormat.rational(result.energyMeasured.whPerKm, fractionDigits: 1) + " Wh/km"
                    : nil,
                exact: result.energyMeasured.whPerKm,
                reason: result.energyMeasured.unknown
            )
            metric(
                title: String(localized: "Referência de energia (histórico)"),
                value: result.energyReference.unknown.isEmpty
                    ? LedgerFormat.rational(result.energyReference.whPerKm, fractionDigits: 1) + " Wh/km"
                    : nil,
                exact: result.energyReference.whPerKm,
                reason: result.energyReference.unknown
            )
            metric(
                title: String(localized: "Variação de eficiência vs. mês anterior"),
                value: result.comparison.unknown.isEmpty
                    ? LedgerFormat.rational(result.comparison.changePercent, fractionDigits: 1) + "%"
                    : nil,
                exact: result.comparison.changePercent,
                reason: result.comparison.unknown
            )
            Text("Distância elegível: \(LedgerFormat.kilometres(result.windowNote.distanceMetres)) · cálculo v\(LedgerBounds.calculationVersion). Estimativas de compra, não medição de estoque; lançamentos esquecidos alteram o resultado.")
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
                    Text("Exato: \(exact)").font(.caption2.monospaced()).foregroundStyle(.secondary)
                }
            } else {
                Text("Indisponível: \(LedgerCoordinator.reasonLabel(reason))")
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
                    Text("Os registros ficam somente neste aparelho. Entre na conta gratuita para ativar a sincronização Premium.")
                    Button("Entrar ou criar conta gratuita") { viewModel.activeSheet = .auth }
                }
            } else if let state = ledger.syncState {
                statusSection(state)
                pullSection
                if !state.conflicts.isEmpty {
                    Section("Conflitos (\(state.conflicts.count))") {
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
                    Section("Recusados pelo servidor (\(refused.count))") {
                        ForEach(refused) { command in
                            VStack(alignment: .leading, spacing: 2) {
                                Text(command.isDelete ? String(localized: "Exclusão") : command.dataset.labelPtBr)
                                    .font(.subheadline)
                                Text(LedgerCoordinator.codeLabel(command.lastCode)).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                        Text("Corrija o registro e salve de novo, ou tente novamente após resolver a causa.")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                exitSection
            }
            if let notice = ledger.notice {
                Section {
                    Text(notice)
                    Button("Ok") { ledger.dismissNotice() }
                }
            }
        }
        .navigationTitle("Nuvem do diário")
        .onAppear { ledger.refresh() }
        .alert("Ativar a sincronização?", isPresented: $confirmEnable) {
            Button("Ativar") { ledger.setSyncEnabled(true) }
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text("Os \(viewModel.snapshot.ledgerEntries.count) lançamentos deste aparelho passam a ser enviados para esta conta, sem fotos nem localização. Exige o Premium (professional.write); exclusão e exportação continuam gratuitas.")
        }
        .alert("Apagar o diário da nuvem?", isPresented: $confirmErase) {
            Button("Apagar da nuvem", role: .destructive) { ledger.eraseCloudLedger() }
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text("Apaga gratuitamente os lançamentos desta conta no servidor e revoga o consentimento. Os registros continuam neste aparelho.")
        }
    }

    private func statusSection(_ state: LedgerSyncState) -> some View {
        Section("Sincronização") {
            Toggle(
                "Enviar lançamentos para a nuvem",
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
                Text("Conjuntos autorizados: \(state.consentedDatasets.map { $0.labelPtBr }.joined(separator: ", "))")
                    .font(.caption)
            }
            Text("Pendentes: \(state.pendingCount) · Conflitos: \(state.conflicts.count) · Recusados: \(state.refusedCount)")
                .font(.caption)
            if state.paidUploadsPaused {
                Text("Envios pausados: o servidor exige o Premium (professional.write) ou o consentimento. Os registros continuam aqui, e exclusões seguem gratuitas.")
                    .font(.caption)
                    .foregroundStyle(AbastevoTheme.warningLight)
                Button("Conhecer o Abastevo Premium") { viewModel.activeSheet = .premium }
            }
            if let last = state.lastSuccessAtMillis {
                Text("Último envio confirmado: \(LedgerTime.label(millis: last, timezone: TimeZone.current.identifier))")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            Button(ledger.isSyncing ? "Sincronizando..." : "Sincronizar agora") { ledger.syncNow() }
                .disabled(!state.enabled || ledger.isSyncing)
            Text("Hodômetro zerado e abastecimentos esquecidos ficam neste aparelho: o servidor ainda não recebe essas marcações.")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var pullSection: some View {
        Section("Baixar da conta") {
            Text("Traz lançamentos da conta que faltam neste aparelho e versões mais novas de registros sem edição pendente. Nada local é apagado ou sobrescrito com edições pendentes.")
                .font(.caption)
            Button(ledger.isPulling ? "Baixando..." : "Baixar lançamentos da conta") { ledger.pull() }
                .disabled(ledger.isPulling)
            if let report = ledger.lastPullReport {
                Text("Novos: \(report.added) · Atualizados: \(report.updated) · Iguais: \(report.unchanged) · Mantidos com edição local: \(report.skippedHeld) · Excluídos aqui: \(report.skippedDeleted) · Anulados: \(report.skippedVoided) · Ilegíveis: \(report.undecodable)")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                if report.truncated {
                    Text("Ainda há páginas; toque de novo para continuar de onde parou.")
                        .font(.caption2)
                }
            }
        }
    }

    private var exitSection: some View {
        Section("Seus dados (gratuito)") {
            ShareLink(item: ledger.exportCsv()) {
                Label("Exportar diário (CSV)", systemImage: "square.and.arrow.up")
            }
            Button("Apagar diário da nuvem", role: .destructive) { confirmErase = true }
                .disabled(ledger.isWorking)
        }
    }

    private func conflictTitle(_ conflict: LedgerConflict) -> String {
        let record = LedgerWire.decodePayload(json: conflict.proposedPayloadJson, dataset: conflict.dataset)
        let date = record?.dateLabel ?? ""
        return conflict.dataset.labelPtBr + (date.isEmpty ? "" : " · " + date)
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
                Section("Base: última versão confirmada neste aparelho") {
                    versionText(conflict.basePayloadJson, dataset: conflict.dataset, empty: String(localized: "Sem envio confirmado anterior."))
                }
                Section("Atual: versão no servidor") {
                    switch conflict.current.state {
                    case .unknown:
                        Text("Ainda não lida. Atualize para comparar.").foregroundStyle(.secondary)
                    case .missing:
                        Text("O servidor não tem mais este registro (excluído em outro aparelho).")
                    case .present:
                        if conflict.current.voided {
                            Text("Anulado no servidor.")
                        }
                        versionText(conflict.current.payloadJson, dataset: conflict.dataset, empty: String(localized: "Versão do servidor ilegível neste aparelho."))
                        Text("Revisão \(String(conflict.current.revision ?? 0))").font(.caption2).foregroundStyle(.secondary)
                    }
                }
                Section("Proposta: sua versão neste aparelho") {
                    versionText(conflict.proposedPayloadJson, dataset: conflict.dataset, empty: "")
                }
                Section("Decisão") {
                    Button("Manter a minha versão") {
                        ledger.resolveConflict(conflict, choice: .keepMine)
                        dismiss()
                    }
                    .disabled(conflict.current.state != .present || conflict.current.voided)
                    Button("Usar a versão do servidor") {
                        ledger.resolveConflict(conflict, choice: .takeServer)
                        dismiss()
                    }
                    .disabled(conflict.current.state == .unknown)
                    Button("Atualizar a versão do servidor") { ledger.refreshConflictCurrent(conflict) }
                        .disabled(ledger.isWorking)
                    Button("Decidir depois") { dismiss() }
                    Text("Manter a sua versão envia uma nova alteração sobre a revisão atual. Nada é decidido pela ordem de chegada.")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            } else {
                Text("Conflito já resolvido.").foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Conflito")
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
