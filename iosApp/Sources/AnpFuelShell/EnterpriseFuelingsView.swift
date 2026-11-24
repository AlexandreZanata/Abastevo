import AnpFuelCore
import Foundation
import PhotosUI
import SwiftUI

@MainActor
struct EnterpriseFuelingsView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let partition: EnterprisePartition
    let membershipId: String

    var body: some View {
        let held = currentSnapshot
        List {
            Section {
                if held?.locked == false {
                    NavigationLink(String(localized: "android.enterprise_fuelings_new")) {
                        EnterpriseFuelingCapture(enterprise: enterprise, partition: partition, membershipId: membershipId)
                    }
                }
                if let held {
                    let rows = held.queue.reversed().filter { $0.kind == .fueling }
                    if rows.isEmpty {
                        Text(String(localized: "android.enterprise_fuelings_empty")).foregroundStyle(.secondary)
                    }
                    ForEach(rows) { row in
                        if let fact = EnterpriseFuelingOperations.fact(row.operation, partition: partition) {
                            NavigationLink {
                                EnterpriseFuelingDetail(enterprise: enterprise, partition: partition,
                                                        membershipId: membershipId, operationId: row.id)
                            } label: {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(fact.product.fieldTitle).font(.headline)
                                    Text(fact.fieldQuantity).font(.subheadline)
                                    Text(EnterpriseCoordinator.statusLabel(row)).font(.caption).foregroundStyle(.secondary)
                                }
                            }
                        } else {
                            Text(String(localized: "android.enterprise_fuelings_unavailable")).foregroundStyle(.secondary)
                        }
                    }
                } else {
                    Text(String(localized: "android.enterprise_fuelings_unavailable")).foregroundStyle(.secondary)
                }
            }
            Section(String(localized: "enterprise.fueling.authorization_title")) {
                Text(String(localized: "enterprise.fueling.authorization_unavailable")).foregroundStyle(.secondary)
                Text(String(localized: "enterprise.fueling.redemption_at_station")).foregroundStyle(.secondary)
            }
        }
        .navigationTitle(String(localized: "android.enterprise_fuelings_title"))
    }

    private var currentSnapshot: EnterprisePartitionSnapshot? {
        guard enterprise.active?.partition == partition, enterprise.active?.summary.membershipId == membershipId,
              let snapshot = enterprise.snapshot, snapshot.partition == partition else { return nil }
        return snapshot
    }
}

@MainActor
private struct EnterpriseFuelingCapture: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var enterprise: EnterpriseCoordinator
    let partition: EnterprisePartition
    let membershipId: String

    @State private var form = EnterpriseFuelingFormInput()
    @State private var assetId: String?
    @State private var meterId: String?
    @State private var meterText = ""
    @State private var withoutReading = false
    @State private var reason = ""
    @State private var receipt: Data?
    @State private var preparing = false
    @State private var errorText: String?

    private var current: Bool {
        enterprise.active?.partition == partition && enterprise.active?.summary.membershipId == membershipId
    }

    private var asset: EnterpriseReferenceAsset? {
        guard current, let assetId else { return nil }
        return enterprise.findAsset(assetId)
    }

    private var meter: EnterpriseReferenceMeter? {
        guard let meterId else { return nil }
        return asset?.meters.first { $0.id == meterId }
    }

    var body: some View {
        Form {
            Section {
                Picker(String(localized: "enterprise.assets.asset"), selection: $assetId) {
                    Text(String(localized: "android.enterprise_assets_pick_title")).tag(nil as String?)
                    if current {
                        ForEach(enterprise.assets) { asset in
                            Text(EnterpriseCoordinator.assetLabel(asset)).tag(Optional(asset.id))
                        }
                    }
                }
                Picker(String(localized: "android.enterprise_fuelings_product"), selection: $form.product) {
                    Text(String(localized: "enterprise.fueling.select_product")).tag(nil as EnterpriseFuelingProduct?)
                    ForEach(EnterpriseFuelingProduct.allCases, id: \.rawValue) { product in
                        Text(product.fieldTitle).tag(Optional(product))
                    }
                }
                if let product = form.product {
                    let unit = product.fieldUnit
                    decimal(String(localized: "android.enterprise_fuelings_quantity", defaultValue: "Quantity (\(unit))"),
                            text: $form.quantity)
                    decimal(String(localized: "android.enterprise_fuelings_unit_price", defaultValue: "Unit price (R$ per \(unit), optional)"),
                            text: $form.unitPrice)
                }
                decimal(String(localized: "android.enterprise_fuelings_paid"), text: $form.paid)
                decimal(String(localized: "android.enterprise_fuelings_discount"), text: $form.discount)
                Picker(String(localized: "android.enterprise_fuelings_fill"), selection: $form.fill) {
                    ForEach(EnterpriseFuelingFill.allCases, id: \.rawValue) { fill in
                        Text(fill.fieldTitle).tag(fill)
                    }
                }
                if form.hasArithmeticWarning {
                    Text(String(localized: "android.enterprise_fuelings_warning_arithmetic"))
                        .font(.footnote).foregroundStyle(.secondary)
                }
            }
            Section {
                Picker(String(localized: "android.enterprise_fuelings_payer"), selection: $form.payer) {
                    Text(String(localized: "android.enterprise_fuelings_payer_company")).tag(EnterpriseExpensePayer.company)
                    Text(String(localized: "android.enterprise_fuelings_payer_employee")).tag(EnterpriseExpensePayer.employee)
                }
                Picker(String(localized: "android.enterprise_fuelings_payment"), selection: $form.payment) {
                    ForEach(EnterpriseFuelingPayment.allCases, id: \.rawValue) { payment in
                        Text(payment.fieldTitle).tag(payment)
                    }
                }
                TextField(String(localized: "android.enterprise_fuelings_place"), text: $form.place)
            }
            if let asset {
                Section(String(localized: "android.enterprise_fuelings_reading")) {
                    if asset.meters.isEmpty {
                        Text(String(localized: "android.enterprise_fuelings_no_meter")).foregroundStyle(.secondary)
                    } else {
                        Toggle(String(localized: "android.enterprise_fuelings_reading_absent"), isOn: $withoutReading)
                    }
                    if withoutReading || asset.meters.isEmpty {
                        TextField(String(localized: "android.enterprise_fuelings_absence_reason"), text: $reason)
                    } else {
                        Picker(String(localized: "android.enterprise_fuelings_reading"), selection: $meterId) {
                            Text(String(localized: "enterprise.fueling.select_meter")).tag(nil as String?)
                            ForEach(asset.meters) { meter in
                                Text(meter.fieldTitle).tag(Optional(meter.id))
                            }
                        }
                        if let meter {
                            decimal(meter.fieldTitle, text: $meterText)
                            if let warning = readingWarning {
                                Text(warning.fieldTitle).font(.footnote).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            Section(String(localized: "android.enterprise_fuelings_receipt_optional")) {
                EnterpriseFuelingPhotoInput(bytes: $receipt, preparing: $preparing) { current }
            }
            Section {
                Button(String(localized: "android.enterprise_fuelings_save"), action: save)
                    .buttonStyle(.borderedProminent)
                    .disabled(!current || assetId == nil || form.product == nil || preparing)
                if let errorText { Text(errorText).font(.footnote).foregroundStyle(.secondary) }
            }
        }
        .navigationTitle(String(localized: "android.enterprise_fuelings_form_title"))
        .onChange(of: assetId) { _, _ in
            meterId = asset?.meters.first?.id
            meterText = ""
            withoutReading = false
            reason = ""
        }
        .onChange(of: current) { _, stillCurrent in
            if !stillCurrent {
                receipt = nil
                dismiss()
            }
        }
    }

    private var readingEvidence: EnterpriseMeterEvidence? {
        if withoutReading || asset?.meters.isEmpty == true {
            return .missing(reason: reason.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        guard let meter else { return nil }
        let value: Int64?
        switch meter.kind {
        case .odometer: value = EnterpriseValues.parseKilometres(meterText)
        case .hourMeter: value = EnterpriseValues.parseHours(meterText)
        }
        guard let value else { return nil }
        return .reading(meterId: meter.id, segmentId: meter.segmentId, kind: meter.kind,
                        readingId: EnterpriseValues.newId(), value: value, modulus: meter.modulus)
    }

    private var readingWarning: EnterpriseFuelingReadingWarning? {
        guard let currentReading = readingEvidence, let meter, let assetId else { return nil }
        let now = Int64(Date().timeIntervalSince1970 * 1_000)
        let past = (enterprise.snapshot?.queue ?? []).reversed()
        for item in past where item.kind == .fueling {
            guard let fact = EnterpriseFuelingOperations.fact(item.operation, partition: partition), fact.assetId == assetId,
                  case .reading(let id, let segment, let kind, _, _, _) = fact.reading,
                  id == meter.id, segment == meter.segmentId, kind == meter.kind else { continue }
            guard item.createdAtMillis < now else { return nil }
            return EnterpriseFuelingReadingWarning.compare(
                current: currentReading, previous: fact.reading, elapsedMillis: now - item.createdAtMillis
            )
        }
        return nil
    }

    private func decimal(_ title: String, text: Binding<String>) -> some View {
        TextField(title, text: text)
            #if os(iOS)
            .keyboardType(.decimalPad)
            #endif
    }

    private func save() {
        guard let assetId, let readingEvidence else {
            errorText = String(localized: "android.enterprise_fuelings_capture_invalid")
            return
        }
        switch enterprise.submitFueling(partition: partition, membershipId: membershipId, assetId: assetId,
                                        form: form, evidence: readingEvidence, receipt: receipt) {
        case .success:
            receipt = nil
            dismiss()
        case .failure(let failure):
            errorText = failure.message
        }
    }
}

@MainActor
private struct EnterpriseFuelingDetail: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let partition: EnterprisePartition
    let membershipId: String
    let operationId: String

    @State private var receipt: Data?
    @State private var preparing = false
    @State private var notice: String?

    var body: some View {
        let writable = enterprise.active?.partition == partition && enterprise.active?.summary.membershipId == membershipId
        let snapshot = writable ? enterprise.snapshot : nil
        let entry = snapshot?.queue.first { $0.id == operationId && $0.kind == .fueling }
        List {
            if let entry, let fact = EnterpriseFuelingOperations.fact(entry.operation, partition: partition) {
                Section {
                    LabeledContent(String(localized: "android.enterprise_fuelings_product"), value: fact.product.fieldTitle)
                    LabeledContent(String(localized: "android.enterprise_fuelings_fill"), value: fact.fill.fieldTitle)
                    Text(fact.fieldQuantity)
                    LabeledContent(String(localized: "android.enterprise_fuelings_paid"), value: AbastevoTheme.formatCentavos(fact.paidCentavos))
                    if let discount = fact.discountCentavos {
                        LabeledContent(String(localized: "android.enterprise_fuelings_discount"), value: AbastevoTheme.formatCentavos(discount))
                    }
                    if let price = fact.unitPriceMilli {
                        let value = LedgerUnits.formatScaled(price, fractionDigits: 3, locale: .current)
                        let unit = fact.product.fieldUnit
                        Text(String(localized: "android.enterprise_fuelings_price_value", defaultValue: "Unit price: R$ \(value) per \(unit)"))
                    }
                    if let place = fact.placeName {
                        Text(String(localized: "android.enterprise_fuelings_place_value", defaultValue: "Place: \(place)"))
                    }
                    Text(EnterpriseCoordinator.statusLabel(entry))
                }
                Section(String(localized: "android.enterprise_fuelings_receipt_title")) {
                    ForEach(snapshot?.attachments.filter { $0.targetKind == "FUELING" && $0.targetId == fact.id } ?? []) {
                        Text(EnterpriseCoordinator.attachmentLabel($0))
                    }
                    if snapshot?.locked == false, entry.status != .rejected {
                        EnterpriseFuelingPhotoInput(bytes: $receipt, preparing: $preparing) { writable }
                        if let receipt, !preparing {
                            Button(String(localized: "android.enterprise_fuelings_add_receipt")) {
                                switch enterprise.attachFuelingReceipt(receipt, operationId: operationId,
                                                                        partition: partition, membershipId: membershipId) {
                                case .success:
                                    self.receipt = nil
                                    notice = String(localized: "android.enterprise_receipts_label_saved")
                                case .failure(let failure):
                                    notice = failure.message
                                }
                            }
                        }
                    }
                    if let notice { Text(notice).font(.caption).foregroundStyle(.secondary) }
                }
            } else {
                Text(String(localized: "android.enterprise_fuelings_unavailable")).foregroundStyle(.secondary)
            }
        }
        .navigationTitle(String(localized: "android.enterprise_fuelings_open"))
        .onChange(of: writable) { _, value in
            if !value { receipt = nil }
        }
    }
}

@MainActor
private struct EnterpriseFuelingPhotoInput: View {
    @Binding var bytes: Data?
    @Binding var preparing: Bool
    let isCurrent: () -> Bool

    @State private var item: PhotosPickerItem?
    @State private var showCamera = false
    @State private var errorText: String?
    @State private var request: Task<Void, Never>?

    var body: some View {
        Group {
            if preparing {
                ProgressView(String(localized: "enterprise.receipt.preparing"))
            } else if let bytes {
                let size = ByteCountFormatStyle(style: .file).format(Int64(bytes.count))
                Text(String(localized: "enterprise.receipt.ready", defaultValue: "Photo ready (\(size)). Location and metadata removed."))
                    .font(.caption).foregroundStyle(.secondary)
                Button(String(localized: "android.vehicle_remove_photo"), role: .destructive) { self.bytes = nil }
            } else {
                PhotosPicker(selection: $item, matching: .images) {
                    Label(String(localized: "enterprise.receipt.choose_photo"), systemImage: "photo")
                }
                #if canImport(UIKit)
                Button { showCamera = true } label: {
                    Label(String(localized: "enterprise.receipt.take_photo"), systemImage: "camera")
                }
                #endif
            }
            if let errorText { Text(errorText).font(.caption).foregroundStyle(.secondary) }
        }
        .onChange(of: item) { _, selection in
            guard let selection else { return }
            begin { try? await selection.loadTransferable(type: Data.self) }
            item = nil
        }
        #if canImport(UIKit)
        .sheet(isPresented: $showCamera) {
            IosCameraCaptureView { data in begin { data } }
        }
        #endif
        .onDisappear {
            request?.cancel()
            preparing = false
        }
    }

    private func begin(_ load: @escaping () async -> Data?) {
        request?.cancel()
        preparing = true
        errorText = nil
        request = Task {
            guard let raw = await load() else {
                if !Task.isCancelled, isCurrent() {
                    errorText = String(localized: "enterprise.receipt.read_failed")
                    preparing = false
                }
                return
            }
            let encoded = await EnterpriseReceiptEncoder.prepare(raw)
            guard !Task.isCancelled, isCurrent() else { return }
            preparing = false
            if let encoded {
                bytes = encoded
            } else {
                errorText = String(localized: "enterprise.receipt.prepare_failed")
            }
        }
    }
}

private extension EnterpriseFuelingFact {
    var fieldQuantity: String {
        let value = LedgerUnits.formatScaled(quantityMilli, fractionDigits: 3, locale: .current)
        let unit = product.fieldUnit
        return String(localized: "android.enterprise_fuelings_quantity_value", defaultValue: "\(value) \(unit)")
    }
}

private extension EnterpriseReferenceMeter {
    var fieldTitle: String {
        switch kind {
        case .odometer: return String(localized: "android.enterprise_fuelings_meter_odometer")
        case .hourMeter: return String(localized: "android.enterprise_fuelings_meter_hours")
        }
    }
}

private extension EnterpriseFuelingReadingWarning {
    var fieldTitle: String {
        switch self {
        case .belowPrevious: return String(localized: "android.enterprise_fuelings_warning_below")
        case .unusualRate: return String(localized: "android.enterprise_fuelings_warning_rate")
        }
    }
}

private extension EnterpriseFuelingProduct {
    var fieldTitle: String {
        switch self {
        case .ethanol: return String(localized: "android.enterprise_fuelings_product_ethanol")
        case .gasolineRegular: return String(localized: "android.enterprise_fuelings_product_gasoline_regular")
        case .gasolineAdditived: return String(localized: "android.enterprise_fuelings_product_gasoline_additived")
        case .gasolinePremiumGrade: return String(localized: "android.enterprise_fuelings_product_gasoline_premium")
        case .dieselS10: return String(localized: "android.enterprise_fuelings_product_diesel_s10")
        case .dieselS500: return String(localized: "android.enterprise_fuelings_product_diesel_s500")
        case .cng: return String(localized: "android.enterprise_fuelings_product_cng")
        case .electricity: return String(localized: "android.enterprise_fuelings_product_electricity")
        case .arla: return String(localized: "android.enterprise_fuelings_product_arla")
        }
    }

    var fieldUnit: String {
        switch self {
        case .cng: return String(localized: "android.enterprise_fuelings_unit_m3")
        case .electricity: return String(localized: "android.enterprise_fuelings_unit_kwh")
        default: return String(localized: "android.enterprise_fuelings_unit_l")
        }
    }
}

private extension EnterpriseFuelingFill {
    var fieldTitle: String {
        switch self {
        case .full: return String(localized: "android.enterprise_fuelings_fill_full")
        case .partial: return String(localized: "android.enterprise_fuelings_fill_partial")
        case .unknown: return String(localized: "android.enterprise_fuelings_fill_unknown")
        }
    }
}

private extension EnterpriseFuelingPayment {
    var fieldTitle: String {
        switch self {
        case .cash: return String(localized: "android.enterprise_fuelings_payment_cash")
        case .pix: return String(localized: "android.enterprise_fuelings_payment_pix")
        case .debitCard: return String(localized: "android.enterprise_fuelings_payment_debit")
        case .creditCard: return String(localized: "android.enterprise_fuelings_payment_credit")
        case .fuelCard: return String(localized: "android.enterprise_fuelings_payment_fuel_card")
        case .supplierAccount: return String(localized: "android.enterprise_fuelings_payment_supplier")
        case .agreementVoucher: return String(localized: "enterprise.fueling.payment_agreement")
        case .internalStock: return String(localized: "enterprise.fueling.payment_stock")
        case .other: return String(localized: "android.enterprise_fuelings_payment_other")
        }
    }
}
