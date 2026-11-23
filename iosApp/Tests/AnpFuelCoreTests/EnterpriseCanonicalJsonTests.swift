import XCTest
@testable import AnpFuelCore

/// Rows copied verbatim from `contracts/testdata/enterprise/sync/canonical-v1.psv`
/// (raw hex → canonical hex). `scripts/tests/test_ios_app_integrity.py` checks that
/// every copied row still matches the fixture.
final class EnterpriseCanonicalJsonTests: XCTestCase {
    struct CanonicalVector {
        let id: String
        let rawHex: String
        let canonicalHex: String?
        let failure: EnterpriseProtocolFailure?
    }

    static let vectors: [CanonicalVector] = [
        CanonicalVector(id: "object-empty", rawHex: "7b7d", canonicalHex: "7b7d", failure: nil),
        CanonicalVector(id: "true", rawHex: "207472756520", canonicalHex: "74727565", failure: nil),
        CanonicalVector(id: "large-int", rawHex: "39323233333732303336383534373735383038", canonicalHex: "39323233333732303336383534373735383038", failure: nil),
        CanonicalVector(id: "key-order", rawHex: "207b20227a223a312c202261223a32207d20", canonicalHex: "7b2261223a322c227a223a317d", failure: nil),
        CanonicalVector(id: "array-order", rawHex: "7b2276616c756573223a5b322c315d7d", canonicalHex: "7b2276616c756573223a5b322c315d7d", failure: nil),
        CanonicalVector(id: "string-html", rawHex: "223c3e2622", canonicalHex: "223c3e2622", failure: nil),
        CanonicalVector(id: "unicode-raw", rawHex: "22436166c3a920f09f9a9a22", canonicalHex: "22436166c3a920f09f9a9a22", failure: nil),
        CanonicalVector(id: "unicode-decomposed", rawHex: "2265cc8122", canonicalHex: "2265cc8122", failure: nil),
        CanonicalVector(id: "escaped-surrogate-pair", rawHex: "225c75643833645c756465396122", canonicalHex: "22f09f9a9a22", failure: nil),
        CanonicalVector(id: "escaped-slash", rawHex: "22615c2f6222", canonicalHex: "22612f6222", failure: nil),
        CanonicalVector(id: "escaped-key", rawHex: "7b225c7530303631223a317d", canonicalHex: "7b2261223a317d", failure: nil),
        CanonicalVector(id: "quote-backslash", rawHex: "225c225c5c22", canonicalHex: "225c225c5c22", failure: nil),
        CanonicalVector(id: "c1-raw", rawHex: "22c28522", canonicalHex: "22c28522", failure: nil),
        CanonicalVector(id: "negative-zero", rawHex: "2d30", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "leading-zero", rawHex: "3031", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "fraction", rawHex: "312e30", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "exponent", rawHex: "316533", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "duplicate-key", rawHex: "7b2261223a312c2261223a327d", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "duplicate-alias", rawHex: "7b2261223a312c225c7530303631223a327d", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "upper-key", rawHex: "7b2241223a317d", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "double-snake", rawHex: "7b22615f5f62223a317d", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "unpaired-high", rawHex: "225c756438303022", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "unpaired-low", rawHex: "225c756463303022", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "raw-control", rawHex: "22610a6222", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "trailing-comma", rawHex: "5b312c5d", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "invalid-utf8", rawHex: "22ff22", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "overlong-utf8", rawHex: "22c0af22", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "encoded-surrogate", rawHex: "22eda08022", canonicalHex: nil, failure: .invalid),
        CanonicalVector(id: "unicode-digit", rawHex: "efbc91", canonicalHex: nil, failure: .invalid),
    ]

    func testCanonicalVectorsMatchTheSharedProfile() throws {
        for vector in Self.vectors {
            let raw = try XCTUnwrap(EnterpriseTestHex.data(vector.rawHex), vector.id)
            if let expected = vector.canonicalHex {
                let parsed = try EnterpriseCanonicalJson.parse(raw)
                let canonical = try EnterpriseCanonicalJson.encode(parsed)
                XCTAssertEqual(EnterpriseTestHex.string(canonical), expected, vector.id)
            } else {
                XCTAssertThrowsError(try EnterpriseCanonicalJson.parse(raw), vector.id) { error in
                    XCTAssertEqual(error as? EnterpriseProtocolFailure, vector.failure, vector.id)
                }
            }
        }
    }

    func testControlCharactersAreEscapedWithLowercaseHex() throws {
        let encoded = try EnterpriseCanonicalJson.encodeString(.text("\u{08}\u{0C}\n\r\t\u{00}\u{1F}"))
        XCTAssertEqual(encoded, #""\u0008\u000c\u000a\u000d\u0009\u0000\u001f""#)
    }

    func testDepthAndSizeBoundsAreLimits() throws {
        let deep64 = String(repeating: "[", count: 64) + String(repeating: "]", count: 64)
        XCTAssertNoThrow(try EnterpriseCanonicalJson.parse(Data(deep64.utf8)))
        let deep65 = String(repeating: "[", count: 65) + String(repeating: "]", count: 65)
        XCTAssertThrowsError(try EnterpriseCanonicalJson.parse(Data(deep65.utf8))) { error in
            XCTAssertEqual(error as? EnterpriseProtocolFailure, .limit)
        }
        let oversized = Data(repeating: 0x20, count: EnterpriseCanonicalJson.maxBatchBytes) + Data("0".utf8)
        XCTAssertThrowsError(try EnterpriseCanonicalJson.parse(oversized)) { error in
            XCTAssertEqual(error as? EnterpriseProtocolFailure, .limit)
        }
        XCTAssertThrowsError(try EnterpriseCanonicalJson.parse(Data([0xEF, 0xBB, 0xBF, 0x7B, 0x7D])))
    }

    func testEncoderRefusesKeysOutsideTheSegmentedShape() {
        XCTAssertThrowsError(try EnterpriseCanonicalJson.encode(.object(["camelCase": .null])))
        XCTAssertThrowsError(try EnterpriseCanonicalJson.encode(.integer("01")))
        XCTAssertTrue(EnterpriseCanonicalJson.isSegmented("a_2_b"))
        XCTAssertFalse(EnterpriseCanonicalJson.isSegmented("_a"))
        XCTAssertFalse(EnterpriseCanonicalJson.isSegmented("a_"))
    }
}

enum EnterpriseTestHex {
    static func data(_ hex: String) -> Data? {
        var bytes: [UInt8] = []
        var index = hex.startIndex
        while index < hex.endIndex {
            let next = hex.index(index, offsetBy: 2, limitedBy: hex.endIndex) ?? hex.endIndex
            guard let byte = UInt8(hex[index..<next], radix: 16) else { return nil }
            bytes.append(byte)
            index = next
        }
        return Data(bytes)
    }

    static func string(_ data: Data) -> String {
        return data.map { String(format: "%02x", $0) }.joined()
    }
}
