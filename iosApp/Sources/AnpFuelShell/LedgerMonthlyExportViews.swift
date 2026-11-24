import AnpFuelCore
import CoreTransferable
import Foundation
import SwiftUI
import UniformTypeIdentifiers

@MainActor
struct LedgerEstimateEditor: View {
    @ObservedObject var ledger: LedgerCoordinator
    let vehicleId: String
    let scope: LedgerEstimateScope

    @State private var input = ""
    @State private var errorText: String?
    @State private var configured: LedgerEstimatedEfficiency?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(String(localized: "android.ledger_entry_estimate_label")).font(.subheadline.weight(.semibold))
            if let configured {
                let value = configured.formatted(locale: .current)
                Text(String(localized: "android.ledger_entry_estimate_saved", defaultValue: "Using \(value) km/L for estimates"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            HStack {
                TextField(
                    String(localized: "android.ledger_entry_estimate_label"),
                    text: $input,
                    prompt: Text(String(localized: "android.ledger_entry_estimate_hint"))
                )
                #if os(iOS)
                .keyboardType(.decimalPad)
                #endif
                .onChange(of: input) { _, _ in errorText = nil }
                Button(String(localized: "android.vehicle_save"), action: save)
                    .buttonStyle(.borderless)
                    .disabled(input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                if configured != nil {
                    Button(String(localized: "android.ledger_entry_estimate_clear"), action: clear)
                        .buttonStyle(.borderless)
                }
            }
            if let errorText {
                Text(errorText).font(.caption).foregroundStyle(.secondary)
            }
        }
        .onAppear {
            setControls(ledger.estimatedEfficiency(for: vehicleId, in: scope))
        }
    }

    private func save() {
        guard let estimate = LedgerEstimatedEfficiency(input) else {
            errorText = String(localized: "android.ledger_entry_estimate_invalid")
            return
        }
        update(estimate)
    }

    private func clear() { update(nil) }

    private func update(_ estimate: LedgerEstimatedEfficiency?) {
        do {
            try ledger.setEstimatedEfficiency(estimate, for: vehicleId, in: scope)
            setControls(estimate)
        } catch {
            errorText = String(localized: "ledger.estimate.unavailable")
        }
    }

    private func setControls(_ estimate: LedgerEstimatedEfficiency?) {
        configured = estimate
        input = estimate.map {
            LedgerUnits.formatScaled($0.hundredths, fractionDigits: 2, separator: Locale.current.decimalSeparator ?? ".")
        } ?? ""
        errorText = nil
    }
}

struct LedgerMonthlyExportLink: View {
    let report: LedgerMonthlyIndicators

    var body: some View {
        let file = LedgerMonthlyCSVFile(
            bytes: Data(LedgerMonthlyCSV.build(report).utf8),
            filename: String(format: "abastevo-monthly-%04d-%02d.csv", report.year, report.month)
        )
        ShareLink(item: file, preview: SharePreview(String(localized: "android.monthly_export_subject"))) {
            Label(String(localized: "android.monthly_export"), systemImage: "square.and.arrow.up")
        }
    }
}

private struct LedgerMonthlyCSVFile: Transferable {
    let bytes: Data
    let filename: String

    static var transferRepresentation: some TransferRepresentation {
        DataRepresentation(exportedContentType: .commaSeparatedText) { (file: LedgerMonthlyCSVFile) in
            file.bytes
        }
        .suggestedFileName { $0.filename }
    }
}
