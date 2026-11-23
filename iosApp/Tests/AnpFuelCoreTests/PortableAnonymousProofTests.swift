import XCTest
@testable import AnpFuelCore

final class PortableAnonymousProofTests: XCTestCase {
    func testBodyCoveredBaseLinesMatchGoldenProfile() {
        let lines = PortableAnonymousProof.buildBaseLines(
            method: "POST",
            authority: "api.example.invalid",
            path: "/v1/contributors",
            query: "",
            contentType: "application/json",
            contentDigest: "\"sha-512=:MEUCIQ==:\"",
            created: 1_735_689_600,
            expires: 1_735_689_900,
            keyId: "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624",
            nonce: "ch-1.client-1"
        )
        XCTAssertEqual(lines.count, 10)
        XCTAssertTrue(PortableAnonymousProof.isLinesShapeValid(lines))
        XCTAssertEqual(lines[0], "\"@method\": POST")
        XCTAssertEqual(lines[5], "\"content-digest\": \"sha-512=:MEUCIQ==:\"")
        XCTAssertEqual(lines[9], "\"nonce\": \"ch-1.client-1\"")
    }

    func testBodylessLinesHave8Elements() {
        let lines = PortableAnonymousProof.buildBaseLines(
            method: "GET",
            authority: "abastevo.com.br",
            path: "/v1/privacy/training-consent",
            query: "",
            contentType: nil,
            contentDigest: nil,
            created: 1_735_689_600,
            expires: 1_735_689_720,
            keyId: "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624",
            nonce: "ch-1.client-1"
        )
        XCTAssertEqual(lines.count, 8)
        XCTAssertTrue(PortableAnonymousProof.isLinesShapeValid(lines))
        XCTAssertFalse(PortableAnonymousProof.isLinesShapeValid(Array(lines.reversed())))
    }

    func testFingerprintAndWindowChecks() {
        XCTAssertTrue(PortableAnonymousProof.isFingerprintShape("fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624"))
        XCTAssertFalse(PortableAnonymousProof.isFingerprintShape("fp:E568C4F52605D14215F6E649000E4FFC2DA4CAA4BDCBBF2B8BCD9937FF9EA624"))
        XCTAssertFalse(PortableAnonymousProof.isFingerprintShape("fp:short"))
        XCTAssertNotNil(PortableAnonymousProof.splitNonce("ch-1.client-1"))
        XCTAssertNil(PortableAnonymousProof.splitNonce("missingdot"))
        XCTAssertTrue(PortableAnonymousProof.isWindowValid(created: 1000, expires: 1120, nowEpochSeconds: 1050))
        XCTAssertFalse(PortableAnonymousProof.isWindowValid(created: 1000, expires: 1500, nowEpochSeconds: 1050))
    }

    func testAccountBindingStatementAndProofShape() {
        let stmt = PortableAccountBinding.statement(accountId: "acc-1", contributorId: "ctb-1", expiry: 1_735_689_720)
        XCTAssertEqual(stmt, "anpfuel-bind.v1\nacc-1\nctb-1\n1735689720")
        let fp = "fp:e568c4f52605d14215f6e649000e4ffc2da4caa4bdcbbf2b8bcd9937ff9ea624"
        XCTAssertNotNil(PortableAccountBinding.proof(fingerprint: fp, expiry: 1_735_689_720, signatureB64Url: "AbCdEf_123-"))
        XCTAssertNil(PortableAccountBinding.proof(fingerprint: "bad", expiry: 100, signatureB64Url: "abc"))
    }
}
