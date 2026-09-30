import SwiftUI
import AnpFuelCore

/// Thin iPhone host for the shared-fixture use case (P12-T04).
///
/// Runs `TankFillUseCase.fillCheapest` against the frozen golden vectors and
/// presents the source-separated result. Live reads, accounts and persistence
/// arrive in P10/P13/P17; this shell proves native wiring only.
///
/// NOT COMPILED: no Xcode toolchain on this Linux host. First real
/// simulator/device run happens on macOS (Xcode 26.4); see iosApp/README.md.
@main
struct AnpFuelApp: App {
    var body: some Scene {
        WindowGroup {
            TankFillView(
                viewModel: TankFillViewModel(
                    prices: [
                        StationPrice(station: "Expensive", unitMilli: 5990),
                        StationPrice(station: "Cheap", unitMilli: 5499),
                    ],
                    capacityMilliLiters: 50_000
                )
            )
        }
    }
}

struct TankFillViewModel {
    let prices: [StationPrice]
    let capacityMilliLiters: Int64

    var result: TankFillResult? {
        try? TankFillUseCase.fillCheapest(
            prices: prices,
            capacityMilliLiters: capacityMilliLiters
        )
    }

    var totalText: String {
        guard let result = result,
              let text = try? PortableMoney.format(result.totalMilli) else {
            return "—"
        }
        return "R$ \(text)"
    }
}

struct TankFillView: View {
    let viewModel: TankFillViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Cheapest tank fill").font(.headline)
            if let result = viewModel.result {
                Text(result.station)
                Text(viewModel.totalText).font(.title)
            } else {
                Text("No prices").foregroundStyle(.secondary)
            }
        }
        .padding()
    }
}
