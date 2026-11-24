import AnpFuelCore
import SwiftUI

@MainActor
struct DriverAccountingView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject private var model: DriverAccountingModel
    @State private var shiftId: String?
    @State private var platform: DriverPlatform?
    @State private var detailed = false
    @State private var net = ""
    @State private var gross = ""
    @State private var fees = ""
    @State private var tips = ""
    @State private var bonus = ""
    @State private var refund = ""
    @State private var registration: VehicleRecord?
    @State private var conflict: DriverOutboxEntry?
    @State private var registering = false

    init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
        _model = ObservedObject(wrappedValue: viewModel.driverAccounting)
    }

    private var shifts: [DriverShiftRecord] { viewModel.driverVisibleShifts.filter { !$0.isActive } }
    private var selectedShift: DriverShiftRecord? { shifts.first { $0.id == shiftId } }
    private var selectedVehicle: VehicleRecord? {
        guard let shift = selectedShift else { return nil }
        return viewModel.snapshot.vehicles.first { $0.id == shift.vehicleId }
    }

    private var amount: DriverEarningsMoney? {
        if detailed {
            guard let gross = parse(gross), let fees = parse(fees), let tips = parse(tips),
                  let bonus = parse(bonus), let refund = parse(refund) else { return nil }
            return DriverEarningsMoney.detailed(gross: gross, fees: fees, tips: tips, bonus: bonus, refund: refund)
        }
        return parse(net).flatMap(DriverEarningsMoney.inclusiveNet)
    }

    var body: some View {
        List {
            if viewModel.currentAccountIdOrNil == nil {
                Text(String(localized: "driver.sync.sign_in")).foregroundStyle(.secondary)
            } else {
                earningSection
                queueSection
            }
            if let notice = model.notice {
                Text(notice).foregroundStyle(.secondary)
            }
        }
        .navigationTitle(String(localized: "driver.sync.title"))
        .task(id: viewModel.currentAccountIdOrNil) { model.refresh() }
        .onChange(of: viewModel.currentAccountIdOrNil) { _, _ in
            shiftId = nil
            clearAmounts()
            registration = nil
            conflict = nil
        }
        .onChange(of: shiftId) { _, _ in clearAmounts() }
        .confirmationDialog(
            String(localized: "driver.sync.register_vehicle"), isPresented: registrationPresented, presenting: registration
        ) { vehicle in
            Button(String(localized: "driver.sync.register_vehicle")) {
                Task {
                    registering = true
                    defer { registering = false }
                    await model.prepareVehicle(vehicle)
                }
            }
        } message: { vehicle in
            Text(String(localized: "driver.sync.register_vehicle_explanation",
                        defaultValue: "Save \(vehicle.name) to your account’s cloud vehicles so the server can verify ownership."))
        }
        .confirmationDialog(
            String(localized: "driver.sync.dismiss_conflict"), isPresented: conflictPresented, presenting: conflict
        ) { entry in
            Button(String(localized: "driver.sync.dismiss_conflict"), role: .destructive) { model.discard(entry) }
        } message: { _ in
            Text(String(localized: "driver.sync.dismiss_warning"))
        }
    }

    private var earningSection: some View {
        Section(String(localized: "driver.sync.earning")) {
            if shifts.isEmpty {
                Text(String(localized: "driver.sync.finish_first")).foregroundStyle(.secondary)
            } else {
                Picker(String(localized: "driver.finished_shifts"), selection: $shiftId) {
                    Text(String(localized: "driver.sync.select_shift")).tag(nil as String?)
                    ForEach(shifts) { shift in
                        Text(shiftLabel(shift)).tag(Optional(shift.id))
                    }
                }
                if let vehicle = selectedVehicle {
                    Button(String(localized: "driver.sync.register_vehicle")) { registration = vehicle }
                        .disabled(registering || viewModel.accountDataExportScope == nil)
                }
                if selectedShift != nil {
                    Picker(String(localized: "driver.sync.platform"), selection: $platform) {
                        Text(String(localized: "driver.sync.select_platform")).tag(nil as DriverPlatform?)
                        ForEach(DriverPlatform.allCases, id: \.rawValue) { platform in
                            Text(platformLabel(platform)).tag(Optional(platform))
                        }
                    }
                    Toggle(String(localized: "driver.sync.itemized"), isOn: $detailed)
                    if detailed {
                        moneyField(String(localized: "driver.gross_earnings"), text: $gross)
                        moneyField(String(localized: "driver.sync.fees"), text: $fees)
                        moneyField(String(localized: "driver.sync.tips"), text: $tips)
                        moneyField(String(localized: "driver.sync.bonus"), text: $bonus)
                        moneyField(String(localized: "driver.sync.refund"), text: $refund)
                    } else {
                        moneyField(String(localized: "driver.sync.inclusive_net"), text: $net)
                    }
                    Button(String(localized: "driver.sync.queue")) { queue() }
                        .disabled(platform == nil || amount == nil || viewModel.isAuthWorking)
                }
            }
        }
    }

    private var queueSection: some View {
        Section(String(localized: "driver.sync.queue_title")) {
            if model.isDispatching { ProgressView() }
            if !model.entries.isEmpty {
                Button(String(localized: "driver.sync.resume")) { Task { await model.resumeWaiting() } }
                    .disabled(viewModel.accountDataExportScope == nil || model.isDispatching)
            }
            ForEach(model.entries) { entry in
                VStack(alignment: .leading, spacing: 4) {
                    let date = Date(timeIntervalSince1970: TimeInterval(entry.createdAtMillis) / 1000)
                    Text(date.formatted(date: .abbreviated, time: .shortened))
                        .font(.subheadline)
                    Text(stateLabel(entry.state)).font(.headline)
                    if let issue = entry.issue {
                        Text(issueLabel(issue)).font(.footnote).foregroundStyle(.secondary)
                    }
                    if entry.state == .conflict {
                        Button(String(localized: "driver.sync.dismiss_conflict"), role: .destructive) {
                            conflict = entry
                        }
                    }
                }
            }
        }
    }

    private var registrationPresented: Binding<Bool> {
        Binding(get: { registration != nil }, set: { if !$0 { registration = nil } })
    }

    private var conflictPresented: Binding<Bool> {
        Binding(get: { conflict != nil }, set: { if !$0 { conflict = nil } })
    }

    private func moneyField(_ label: String, text: Binding<String>) -> some View {
        TextField(label, text: text).keyboardType(.decimalPad)
    }

    private func parse(_ text: String) -> Int64? {
        try? PortableMoney.parseBrlCentavos(text)
    }

    private func shiftLabel(_ shift: DriverShiftRecord) -> String {
        let name = viewModel.snapshot.vehicles.first { $0.id == shift.vehicleId }?.name
            ?? String(localized: "driver.vehicle")
        guard let timestamp = shift.endedAtLabel, let date = ISO8601DateFormatter().date(from: timestamp) else { return name }
        let closed = date.formatted(date: .abbreviated, time: .shortened)
        return String(localized: "driver.sync.shift_choice", defaultValue: "\(name) · \(closed)")
    }

    private func platformLabel(_ platform: DriverPlatform) -> String {
        switch platform {
        case .uber: return "Uber"
        case .ninetyNine: return "99"
        case .inDrive: return "inDrive"
        case .other: return String(localized: "driver.sync.other_platform")
        }
    }

    private func stateLabel(_ state: DriverOutboxState) -> String {
        switch state {
        case .pending: return String(localized: "driver.sync.pending")
        case .synced: return String(localized: "driver.sync.synced")
        case .conflict: return String(localized: "driver.sync.conflict")
        }
    }

    private func issueLabel(_ issue: DriverOutboxIssue) -> String {
        switch issue {
        case .offline: return String(localized: "ledger.refusal.offline")
        case .timeout, .unavailable, .invalidReceipt: return String(localized: "driver.sync.unavailable")
        case .rateLimited: return String(localized: "ledger.server.rate_limit")
        case .signInRequired: return String(localized: "ledger.refusal.sign_in_again")
        case .capabilityRequired: return String(localized: "ledger.refusal.capability")
        case .disabled: return String(localized: "driver.sync.disabled")
        case .conflict: return String(localized: "driver.sync.conflict_reason")
        case .invalid: return String(localized: "driver.sync.rejected")
        case .retryLimit: return String(localized: "driver.sync.retry_limit")
        }
    }

    private func queue() {
        guard let selectedShift, let platform, let amount else { return }
        if model.queue(selectedShift, platform: platform, money: amount) {
            shiftId = nil
            clearAmounts()
        }
    }

    private func clearAmounts() {
        platform = nil
        net = ""
        gross = ""
        fees = ""
        tips = ""
        bonus = ""
        refund = ""
    }
}
