import Foundation
import XCTest
@testable import AnpFuelCore

/// Replays `contracts/testdata/account/public-names-v1.tsv`, like the Go account domain
/// and Android `PublicNameTest`.
final class PublicNameTests: XCTestCase {
    func testSharedPublicNameVectorsMatchTheAccountApiRule() throws {
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/account/public-names-v1.tsv")
        let text = try String(contentsOf: url, encoding: .utf8)
        var cases = 0
        for (index, line) in text.components(separatedBy: "\n").enumerated() {
            if line.isEmpty || line.hasPrefix("#") { continue }
            let fields = line.components(separatedBy: "\t")
            XCTAssertEqual(fields.count, 3, "line \(index + 1): want 3 tab-separated fields")
            guard fields.count == 3 else { continue }
            let input = try Self.unescape(fields[1])
            cases += 1
            switch fields[0] {
            case "normalize":
                // Scalars, not String equality: canonical equivalence would hide a missing NFC step.
                XCTAssertEqual(Array(PublicName.normalize(input).unicodeScalars), Array(fields[2].unicodeScalars),
                               "line \(index + 1): normalize")
            case "valid":
                XCTAssertEqual(PublicName.isValid(input), fields[2] == "true", "line \(index + 1): isValid(\(fields[1]))")
                XCTAssertTrue(fields[2] == "true" || fields[2] == "false", "line \(index + 1): want true or false")
            default:
                XCTFail("line \(index + 1): unknown kind \(fields[0])")
            }
        }
        XCTAssertGreaterThanOrEqual(cases, 30)
    }

    /// Decodes the vector escapes shared with Go's strconv.Unquote: \t \n \r \\ \uXXXX \UXXXXXXXX.
    private static func unescape(_ field: String) throws -> String {
        var out = String.UnicodeScalarView()
        let scalars = Array(field.unicodeScalars)
        var index = 0
        while index < scalars.count {
            let scalar = scalars[index]
            guard scalar == "\\", index + 1 < scalars.count else {
                out.append(scalar)
                index += 1
                continue
            }
            switch scalars[index + 1] {
            case "t": out.append("\t"); index += 2
            case "n": out.append("\n"); index += 2
            case "r": out.append("\r"); index += 2
            case "\\": out.append("\\"); index += 2
            case "u", "U":
                let width = scalars[index + 1] == "u" ? 4 : 8
                let digits = String(String.UnicodeScalarView(scalars[(index + 2)..<(index + 2 + width)]))
                guard let value = UInt32(digits, radix: 16), let decoded = Unicode.Scalar(value) else {
                    throw NSError(domain: "PublicNameTests", code: 2)
                }
                out.append(decoded)
                index += 2 + width
            default:
                throw NSError(domain: "PublicNameTests", code: 3)
            }
        }
        return String(out)
    }

    func testNamesAreCheckedAsWrittenBeforeNormalizing() {
        XCTAssertTrue(PublicName.isValid("  Ana   Lima  "))
        XCTAssertEqual(PublicName.normalize("  Ana   Lima  "), "Ana Lima")
        XCTAssertFalse(PublicName.isValid("Ana\u{0007}Lima"))
        XCTAssertFalse(PublicName.isValid("Moderação Lima"))
        XCTAssertTrue(PublicName.isValid("Administradora Lima"))
    }
}
