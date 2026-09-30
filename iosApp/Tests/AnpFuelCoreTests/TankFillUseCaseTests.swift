import XCTest
@testable import AnpFuelCore

/// Swift unit tests for the shared-fixture use case the shell executes.
///
/// NOT RUN: no Swift/Xcode toolchain on this Linux host. First real run is
/// `swift test` / `xcodebuild test` on macOS (Xcode 26.4); see iosApp/README.md.
final class TankFillUseCaseTests: XCTestCase {
    func testFillCheapestUsesMinimumUnitPrice() throws {
        let result = try TankFillUseCase.fillCheapest(
            prices: [
                StationPrice(station: "Expensive", unitMilli: 5990),
                StationPrice(station: "Cheap", unitMilli: 5499),
            ],
            capacityMilliLiters: 50_000
        )
        XCTAssertEqual(result.station, "Cheap")
        XCTAssertEqual(result.unitMilli, 5499)
        XCTAssertEqual(result.totalMilli, 274950)
    }

    func testCheapestKeepsFirstOnTie() throws {
        let best = try TankFillUseCase.cheapest(prices: [
            StationPrice(station: "A", unitMilli: 5499),
            StationPrice(station: "B", unitMilli: 5499),
        ])
        XCTAssertEqual(best.station, "A")
    }

    func testEmptyPricesThrow() {
        XCTAssertThrowsError(try TankFillUseCase.fillCheapest(prices: [], capacityMilliLiters: 50_000))
    }
}
