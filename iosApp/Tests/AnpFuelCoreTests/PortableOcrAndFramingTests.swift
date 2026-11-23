import XCTest
@testable import AnpFuelCore

final class PortableOcrAndFramingTests: XCTestCase {
    func testPriceOcrPreservesOrderWithoutAutoMinimum() {
        let text = "GASOLINA R$ 6,199\nETANOL 4,090\nDIESEL S10 R$ 6,299"
        let candidates = PortablePriceOcr.parseCandidates(text)
        XCTAssertEqual(candidates.count, 3)
        XCTAssertEqual(candidates.map { $0.priceMilli }, [6199, 4090, 6299])
        XCTAssertFalse(PortablePriceOcr.isLowConfidence(candidates[0]))
        XCTAssertTrue(PortablePriceOcr.isLowConfidence(candidates[1]))
    }

    func testFuelBoardAssociatesRowsAndDetectsConditionalBoard() {
        let tokens = [
            FuelBoardOcr.Token(text: "GASOLINA COMUM", left: 10, top: 10, right: 200, bottom: 50),
            FuelBoardOcr.Token(text: "5,899", left: 240, top: 12, right: 340, bottom: 48),
            FuelBoardOcr.Token(text: "ETANOL", left: 10, top: 70, right: 160, bottom: 110),
            FuelBoardOcr.Token(text: "3,990", left: 240, top: 72, right: 340, bottom: 108),
            FuelBoardOcr.Token(text: "PRECO NO PIX", left: 10, top: 130, right: 220, bottom: 160),
        ]
        let result = FuelBoardOcr.associate(tokens: tokens)
        XCTAssertTrue(result.conditional)
        XCTAssertFalse(result.unresolved)
        XCTAssertEqual(result.rows.count, 2)
        XCTAssertTrue(result.rows.contains(FuelBoardOcr.Row(product: .gasolineRegular, amountMilli: 5899)))
        XCTAssertTrue(result.rows.contains(FuelBoardOcr.Row(product: .ethanol, amountMilli: 3990)))
    }

    func testBoardFramingAndLiveZoom() {
        let tokens = [
            FuelBoardOcr.Token(text: "GASOLINA", left: 100, top: 100, right: 260, bottom: 140),
            FuelBoardOcr.Token(text: "5,999", left: 300, top: 100, right: 420, bottom: 140),
        ]
        let frame = FuelBoardFraming.locate(tokens: tokens, width: 1000, height: 1000)
        XCTAssertNotNil(frame)
        if let f = frame {
            XCTAssertGreaterThan(FuelBoardFraming.zoomFor(frame: f), 1.0)
        }
        if let board = LiveBoardZoom.Board(left: 0.2, top: 0.2, right: 0.6, bottom: 0.6) {
            let s1 = LiveBoardZoom.next(state: LiveBoardZoom.State(ratio: 1.0), board: board, minRatio: 1.0, maxRatio: 5.0)
            let s2 = LiveBoardZoom.next(state: s1, board: board, minRatio: 1.0, maxRatio: 5.0)
            XCTAssertGreaterThan(s2.ratio, 1.0)
        }
    }
}
