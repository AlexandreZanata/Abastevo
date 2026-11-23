import Foundation
import CryptoKit

/// Exact JSON values of the enterprise canonical profile.
///
/// Swift port of Kotlin `SyncJson`/`CanonicalJson` (Android domain) and the Go
/// `orgsync/protocol` parser. Numbers are integer lexemes only and never pass
/// through floating point. Object keys are lowercase segmented names. This is
/// the small enterprise profile, deliberately not a claim of RFC 8785.
/// Zero postfix unwrap operators anywhere in this file.
public enum EnterpriseJson: Equatable, Sendable {
    case object([String: EnterpriseJson])
    case array([EnterpriseJson])
    case text(String)
    case integer(String)
    case bool(Bool)
    case null

    public static func int(_ value: Int64) -> EnterpriseJson {
        return .integer(String(value))
    }

    public static func optionalText(_ value: String?) -> EnterpriseJson {
        guard let value else { return .null }
        return .text(value)
    }

    public var objectValue: [String: EnterpriseJson]? {
        if case .object(let fields) = self { return fields }
        return nil
    }

    public var arrayValue: [EnterpriseJson]? {
        if case .array(let items) = self { return items }
        return nil
    }

    public var textValue: String? {
        if case .text(let value) = self { return value }
        return nil
    }

    public var integerLexeme: String? {
        if case .integer(let lexeme) = self { return lexeme }
        return nil
    }

    public var boolValue: Bool? {
        if case .bool(let value) = self { return value }
        return nil
    }

    public var isNull: Bool {
        if case .null = self { return true }
        return false
    }

    public subscript(key: String) -> EnterpriseJson? {
        return objectValue?[key]
    }
}

/// Failure classes shared with Kotlin `SyncFailure` and the Go protocol errors.
public enum EnterpriseProtocolFailure: Error, Equatable, Sendable {
    case invalid
    case limit
    case binding
    case digest
}

public enum EnterpriseCanonicalJson {
    public static let maxBatchBytes = 65_536
    public static let maxOperationBytes = 8_192
    public static let maxTransportDepth = 64
    public static let maxOperationDepth = 62

    // MARK: - Encoding

    /// Compact encoding with keys sorted by their bytes; only `"`, `\` and C0 controls are escaped.
    public static func encode(_ value: EnterpriseJson, maxDepth: Int = maxTransportDepth) throws -> Data {
        guard maxDepth >= 1, maxDepth <= maxTransportDepth else { throw EnterpriseProtocolFailure.invalid }
        var output: [UInt8] = []
        output.reserveCapacity(256)
        try append(value, containers: 0, limit: maxDepth, into: &output)
        return Data(output)
    }

    /// Canonical bytes as a UTF-8 string (always valid UTF-8 by construction).
    public static func encodeString(_ value: EnterpriseJson, maxDepth: Int = maxTransportDepth) throws -> String {
        let data = try encode(value, maxDepth: maxDepth)
        return String(decoding: data, as: UTF8.self)
    }

    private static func append(_ value: EnterpriseJson, containers: Int, limit: Int, into output: inout [UInt8]) throws {
        switch value {
        case .object(let fields):
            if containers >= limit { throw EnterpriseProtocolFailure.limit }
            output.append(UInt8(ascii: "{"))
            let keys = fields.keys.sorted { lhs, rhs in
                Array(lhs.utf8).lexicographicallyPrecedes(Array(rhs.utf8))
            }
            var first = true
            for key in keys {
                guard isSegmented(key), let item = fields[key] else { throw EnterpriseProtocolFailure.invalid }
                if !first { output.append(UInt8(ascii: ",")) }
                first = false
                appendText(key, into: &output)
                output.append(UInt8(ascii: ":"))
                try append(item, containers: containers + 1, limit: limit, into: &output)
            }
            output.append(UInt8(ascii: "}"))
        case .array(let items):
            if containers >= limit { throw EnterpriseProtocolFailure.limit }
            output.append(UInt8(ascii: "["))
            for (index, item) in items.enumerated() {
                if index > 0 { output.append(UInt8(ascii: ",")) }
                try append(item, containers: containers + 1, limit: limit, into: &output)
            }
            output.append(UInt8(ascii: "]"))
        case .text(let text):
            appendText(text, into: &output)
        case .integer(let lexeme):
            guard isIntegerLexeme(lexeme) else { throw EnterpriseProtocolFailure.invalid }
            output.append(contentsOf: Array(lexeme.utf8))
        case .bool(let flag):
            output.append(contentsOf: Array((flag ? "true" : "false").utf8))
        case .null:
            output.append(contentsOf: Array("null".utf8))
        }
    }

    private static let hexDigits: [UInt8] = Array("0123456789abcdef".utf8)

    private static func appendText(_ text: String, into output: inout [UInt8]) {
        output.append(UInt8(ascii: "\""))
        for scalar in text.unicodeScalars {
            switch scalar.value {
            case 0x22:
                output.append(UInt8(ascii: "\\"))
                output.append(UInt8(ascii: "\""))
            case 0x5C:
                output.append(UInt8(ascii: "\\"))
                output.append(UInt8(ascii: "\\"))
            case 0x00..<0x20:
                output.append(contentsOf: Array("\\u00".utf8))
                output.append(hexDigits[Int(scalar.value >> 4)])
                output.append(hexDigits[Int(scalar.value & 0x0F)])
            default:
                output.append(contentsOf: Array(String(scalar).utf8))
            }
        }
        output.append(UInt8(ascii: "\""))
    }

    // MARK: - Shapes

    /// `^[a-z](?:_?[a-z0-9])*$`: lowercase segments joined by single underscores.
    public static func isSegmented(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        guard let first = bytes.first, first >= 0x61, first <= 0x7A else { return false }
        var afterUnderscore = false
        for byte in bytes {
            if byte == 0x5F {
                if afterUnderscore { return false }
                afterUnderscore = true
            } else {
                let isLower = byte >= 0x61 && byte <= 0x7A
                let isDigit = byte >= 0x30 && byte <= 0x39
                if !isLower && !isDigit { return false }
                afterUnderscore = false
            }
        }
        return !afterUnderscore
    }

    /// `0|-?[1-9][0-9]*`.
    public static func isIntegerLexeme(_ value: String) -> Bool {
        var bytes = Array(value.utf8)
        if bytes == [0x30] { return true }
        if bytes.first == 0x2D { bytes.removeFirst() }
        guard let first = bytes.first, first >= 0x31, first <= 0x39 else { return false }
        return bytes.allSatisfy { $0 >= 0x30 && $0 <= 0x39 }
    }

    public static func sha256Hex(_ data: Data) -> String {
        return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
    }

    // MARK: - Parsing

    /// Strict parser: UTF-8 only, no duplicate or non-segmented keys, integers only, bounded size and depth.
    public static func parse(_ data: Data, maxBytes: Int = maxBatchBytes, maxDepth: Int = maxTransportDepth) throws -> EnterpriseJson {
        guard maxBytes >= 1, maxDepth >= 1, maxDepth <= maxTransportDepth else { throw EnterpriseProtocolFailure.invalid }
        if data.count > maxBytes { throw EnterpriseProtocolFailure.limit }
        if data.starts(with: [0xEF, 0xBB, 0xBF]) { throw EnterpriseProtocolFailure.invalid }
        // Swift's UTF-8 decoder repairs ill-formed input (overlong forms, encoded surrogates,
        // truncated sequences); a repaired text no longer round-trips, so it is refused.
        let text = String(decoding: data, as: UTF8.self)
        guard Data(text.utf8) == data else { throw EnterpriseProtocolFailure.invalid }
        var parser = Parser(units: Array(text.utf16), maxDepth: maxDepth)
        return try parser.parseDocument()
    }

    private struct Parser {
        let units: [UInt16]
        let maxDepth: Int
        var index = 0

        init(units: [UInt16], maxDepth: Int) {
            self.units = units
            self.maxDepth = maxDepth
        }

        mutating func parseDocument() throws -> EnterpriseJson {
            skipWhitespace()
            let value = try parseValue(containers: 0)
            skipWhitespace()
            if index != units.count { throw EnterpriseProtocolFailure.invalid }
            return value
        }

        private mutating func parseValue(containers: Int) throws -> EnterpriseJson {
            skipWhitespace()
            guard index < units.count else { throw EnterpriseProtocolFailure.invalid }
            switch units[index] {
            case 0x7B:
                if containers >= maxDepth { throw EnterpriseProtocolFailure.limit }
                index += 1
                skipWhitespace()
                var fields: [String: EnterpriseJson] = [:]
                if take(0x7D) { return .object(fields) }
                while true {
                    skipWhitespace()
                    guard index < units.count, units[index] == 0x22 else { throw EnterpriseProtocolFailure.invalid }
                    let key = try parseText()
                    if !EnterpriseCanonicalJson.isSegmented(key) || fields[key] != nil {
                        throw EnterpriseProtocolFailure.invalid
                    }
                    skipWhitespace()
                    try expect(0x3A)
                    fields[key] = try parseValue(containers: containers + 1)
                    skipWhitespace()
                    if take(0x7D) { break }
                    try expect(0x2C)
                }
                return .object(fields)
            case 0x5B:
                if containers >= maxDepth { throw EnterpriseProtocolFailure.limit }
                index += 1
                skipWhitespace()
                var items: [EnterpriseJson] = []
                if take(0x5D) { return .array(items) }
                while true {
                    items.append(try parseValue(containers: containers + 1))
                    skipWhitespace()
                    if take(0x5D) { break }
                    try expect(0x2C)
                }
                return .array(items)
            case 0x22:
                return .text(try parseText())
            case 0x74:
                try literal("true")
                return .bool(true)
            case 0x66:
                try literal("false")
                return .bool(false)
            case 0x6E:
                try literal("null")
                return .null
            default:
                let start = index
                if units[index] == 0x2D { index += 1 }
                let digits = index
                while index < units.count, units[index] >= 0x30, units[index] <= 0x39 {
                    index += 1
                }
                if index == digits { throw EnterpriseProtocolFailure.invalid }
                let lexeme = String(decoding: units[start..<index], as: UTF16.self)
                guard EnterpriseCanonicalJson.isIntegerLexeme(lexeme) else { throw EnterpriseProtocolFailure.invalid }
                return .integer(lexeme)
            }
        }

        private mutating func parseText() throws -> String {
            try expect(0x22)
            var result: [UInt16] = []
            while index < units.count {
                let unit = units[index]
                index += 1
                if unit == 0x22 {
                    return try Parser.validated(result)
                }
                if unit < 0x20 { throw EnterpriseProtocolFailure.invalid }
                if unit != 0x5C {
                    result.append(unit)
                    continue
                }
                guard index < units.count else { throw EnterpriseProtocolFailure.invalid }
                let escape = units[index]
                index += 1
                switch escape {
                case 0x22: result.append(0x22)
                case 0x5C: result.append(0x5C)
                case 0x2F: result.append(0x2F)
                case 0x62: result.append(0x08)
                case 0x66: result.append(0x0C)
                case 0x6E: result.append(0x0A)
                case 0x72: result.append(0x0D)
                case 0x74: result.append(0x09)
                case 0x75:
                    guard units.count - index >= 4 else { throw EnterpriseProtocolFailure.invalid }
                    var code: UInt16 = 0
                    for offset in 0..<4 {
                        guard let digit = Parser.hexValue(units[index + offset]) else { throw EnterpriseProtocolFailure.invalid }
                        code = code &* 16 &+ digit
                    }
                    index += 4
                    result.append(code)
                default:
                    throw EnterpriseProtocolFailure.invalid
                }
            }
            throw EnterpriseProtocolFailure.invalid
        }

        private static func hexValue(_ unit: UInt16) -> UInt16? {
            switch unit {
            case 0x30...0x39: return unit - 0x30
            case 0x41...0x46: return unit - 0x41 + 10
            case 0x61...0x66: return unit - 0x61 + 10
            default: return nil
            }
        }

        /// Refuses lone surrogates exactly as the Kotlin `requireUnicode` check.
        private static func validated(_ units: [UInt16]) throws -> String {
            var position = 0
            while position < units.count {
                let unit = units[position]
                position += 1
                if unit >= 0xD800 && unit <= 0xDBFF {
                    guard position < units.count, units[position] >= 0xDC00, units[position] <= 0xDFFF else {
                        throw EnterpriseProtocolFailure.invalid
                    }
                    position += 1
                } else if unit >= 0xDC00 && unit <= 0xDFFF {
                    throw EnterpriseProtocolFailure.invalid
                }
            }
            return String(decoding: units, as: UTF16.self)
        }

        private mutating func literal(_ token: String) throws {
            let expected = Array(token.utf16)
            guard units.count - index >= expected.count, Array(units[index..<(index + expected.count)]) == expected else {
                throw EnterpriseProtocolFailure.invalid
            }
            index += expected.count
        }

        private mutating func skipWhitespace() {
            while index < units.count {
                let unit = units[index]
                if unit == 0x20 || unit == 0x09 || unit == 0x0A || unit == 0x0D {
                    index += 1
                } else {
                    return
                }
            }
        }

        private mutating func take(_ unit: UInt16) -> Bool {
            if index < units.count, units[index] == unit {
                index += 1
                return true
            }
            return false
        }

        private mutating func expect(_ unit: UInt16) throws {
            if !take(unit) { throw EnterpriseProtocolFailure.invalid }
        }
    }
}
