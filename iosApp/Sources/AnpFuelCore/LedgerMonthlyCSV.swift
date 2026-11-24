import Foundation

/// Android's MonthlyIndicatorsCsv column and metric contract over the selected
/// local vehicle and period. Local estimates never enter measured or money rows.
public enum LedgerMonthlyCSV {
    public static func build(_ report: LedgerMonthlyIndicators) -> String {
        var csv = "vehicle,metric,value\n"
        let vehicle = LedgerCsvExport.escape(report.vehicleName)
        func line(_ metric: String, _ value: String) {
            csv += vehicle + "," + metric + "," + value + "\n"
        }

        line("purchase_centavos", String(report.purchaseCentavos))
        line("expense_centavos", String(report.expenseCentavos))
        line("charge_energy_wh", String(report.chargeEnergyWh))
        line("charge_cost_centavos", String(report.chargeCostCentavos))
        for category in LedgerExpenseCategory.allCases {
            guard let expense = report.byCategory[category] else { continue }
            line("expense:" + category.rawValue, String(expense))
        }
        line("distance_metres", String(report.windowNote.distanceMetres))
        if report.measured.unknown.isEmpty {
            line("km_per_unit", report.measured.kmPerUnit)
            line("centavos_per_km", report.measured.centavosPerKm)
        } else {
            line("unknown", LedgerCsvExport.escape(report.measured.unknown))
        }
        if report.combined.unknown.isEmpty {
            line("combined_centavos_per_km", report.combined.centavosPerKm)
        } else {
            line("combined_unknown", LedgerCsvExport.escape(report.combined.unknown))
        }
        if report.energyMeasured.unknown.isEmpty {
            line("energy_purchased_wh", String(report.energyMeasured.purchasedWh))
            line("energy_measured_wh_per_km", report.energyMeasured.whPerKm)
        } else {
            line("energy_unknown", LedgerCsvExport.escape(report.energyMeasured.unknown))
        }
        if report.energyReference.unknown.isEmpty {
            line("energy_reference_wh_per_km", report.energyReference.whPerKm)
        } else {
            line("energy_reference_unknown", LedgerCsvExport.escape(report.energyReference.unknown))
        }
        if report.energyComparison.unknown.isEmpty {
            line("energy_comparison_change_percent", report.energyComparison.changePercent)
        } else {
            line("energy_comparison_unknown", LedgerCsvExport.escape(report.energyComparison.unknown))
        }
        line("charge_fees_centavos", String(report.chargeFeesCentavos))
        return csv
    }
}
