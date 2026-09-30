/// Thin shared-fixture use case executed by the iPhone shell (P12-T04).
///
/// Given station unit prices in integer milli-BRL and a tank capacity, this
/// selects the cheapest station and computes the exact tank-fill total via
/// `PortableMoney`. It is the use case the SwiftUI host runs against the
/// frozen golden vectors until P17 wires live reads.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public struct StationPrice: Equatable {
    public let station: String
    public let unitMilli: Int64
    public init(station: String, unitMilli: Int64) {
        self.station = station
        self.unitMilli = unitMilli
    }
}

public struct TankFillResult: Equatable {
    public let station: String
    public let unitMilli: Int64
    public let capacityMilliLiters: Int64
    public let totalMilli: Int64
}

public enum TankFillUseCaseError: Error, Equatable {
    case noPrices
}

public enum TankFillUseCase {
    /// Cheapest station by unit price; ties break toward the first listed.
    public static func cheapest(prices: [StationPrice]) throws -> StationPrice {
        var best: StationPrice?
        for price in prices {
            if let current = best {
                if price.unitMilli < current.unitMilli { best = price }
            } else {
                best = price
            }
        }
        guard let found = best else { throw TankFillUseCaseError.noPrices }
        return found
    }

    /// Exact tank-fill total for the cheapest station.
    public static func fillCheapest(
        prices: [StationPrice],
        capacityMilliLiters: Int64
    ) throws -> TankFillResult {
        let best = try cheapest(prices: prices)
        let total = try PortableMoney.multiplyTankFill(
            unitMilli: best.unitMilli,
            capacityMilliLiters: capacityMilliLiters
        )
        return TankFillResult(
            station: best.station,
            unitMilli: best.unitMilli,
            capacityMilliLiters: capacityMilliLiters,
            totalMilli: total
        )
    }
}
