import Foundation

/// A bounded XML subset used by GPX exchange. Parsing never delegates resolution
/// to an XML engine and never evaluates declarations other than the UTF-8 header.
enum StrictSavedXML {
    final class Node {
        let name: String
        let attributes: [String: String]
        var children: [Node] = []
        var text = ""

        init(_ name: String, attributes: [String: String]) {
            self.name = name
            self.attributes = attributes
        }

        var localName: String {
            String(name.split(separator: ":", maxSplits: 1).last ?? "")
        }

        func first(_ name: String) -> Node? { children.first { $0.localName == name } }

        func all(_ name: String) -> [Node] { children.filter { $0.localName == name } }

        func text(_ name: String) -> String? {
            guard let value = first(name)?.text.trimmingCharacters(in: .whitespacesAndNewlines),
                  !value.isEmpty else { return nil }
            return value
        }
    }

    static func parse(_ data: Data) throws -> Node {
        guard data.count <= SavedMapBounds.fileBytes else { throw SavedMapError.fileTooLarge }
        let textData = data.starts(with: [0xef, 0xbb, 0xbf]) ? data.dropFirst(3) : data[...]
        guard let text = String(data: textData, encoding: .utf8) else {
            throw SavedMapError.xmlEncodingUnsupported
        }
        guard text.unicodeScalars.allSatisfy(validXMLScalar) else { throw SavedMapError.xmlMalformed }
        var parser = Reader(bytes: Array(text.utf8))
        return try parser.document()
    }

    private static func validXMLScalar(_ scalar: Unicode.Scalar) -> Bool {
        let value = scalar.value
        return value == 9 || value == 10 || value == 13 || (0x20...0xd7ff).contains(value)
            || (0xe000...0xfffd).contains(value) || (0x10000...0x10ffff).contains(value)
    }

    private struct Reader {
        let bytes: [UInt8]
        var position = 0
        var elements = 0

        var current: UInt8? { position < bytes.count ? bytes[position] : nil }

        mutating func document() throws -> Node {
            try declaration()
            try outsideRoot()
            guard current == 60 else { throw SavedMapError.xmlMalformed }
            let root = try element(depth: 1)
            try outsideRoot()
            guard current == nil else { throw SavedMapError.xmlMalformed }
            return root
        }

        mutating func declaration() throws {
            guard token("<?xml") else { return }
            position += 5
            guard let next = current, Self.space(next) else { throw SavedMapError.xmlPIRefused }
            let raw = try readUntil("?>")
            let pattern = "^\\s+version\\s*=\\s*(['\"])(1\\.0|1\\.1)\\1(?:\\s+encoding\\s*=\\s*(['\"])([A-Za-z][A-Za-z0-9._-]*)\\3)?(?:\\s+standalone\\s*=\\s*(['\"])(yes|no)\\5)?\\s*$"
            let regex = try NSRegularExpression(pattern: pattern)
            let range = NSRange(raw.startIndex..<raw.endIndex, in: raw)
            guard let matched = regex.firstMatch(in: raw, range: range) else {
                throw SavedMapError.xmlMalformed
            }
            if let encoding = Range(matched.range(at: 4), in: raw),
               raw[encoding].caseInsensitiveCompare("UTF-8") != .orderedSame {
                throw SavedMapError.xmlEncodingUnsupported
            }
        }

        mutating func outsideRoot() throws {
            while true {
                skipSpace()
                if token("<!--") {
                    try comment()
                } else {
                    if token("<?") { throw SavedMapError.xmlPIRefused }
                    if token("<!") { throw SavedMapError.xmlDoctypeRefused }
                    return
                }
            }
        }

        mutating func element(depth: Int) throws -> Node {
            guard depth <= SavedMapBounds.xmlDepth else { throw SavedMapError.xmlTooDeep }
            elements += 1
            guard elements <= SavedMapBounds.xmlElements else { throw SavedMapError.xmlTooLarge }
            position += 1
            let tag = try readName()
            var attributes: [String: String] = [:]
            while true {
                let hasSpace = skipSpace()
                if token("/>") {
                    position += 2
                    return Node(tag, attributes: attributes)
                }
                if current == 62 {
                    position += 1
                    break
                }
                guard hasSpace, attributes.count < 32 else { throw SavedMapError.xmlMalformed }
                let key = try readName()
                guard attributes[key] == nil else { throw SavedMapError.xmlMalformed }
                skipSpace()
                guard current == 61 else { throw SavedMapError.xmlMalformed }
                position += 1
                skipSpace()
                guard let quote = current, quote == 34 || quote == 39 else { throw SavedMapError.xmlMalformed }
                position += 1
                let start = position
                while let byte = current, byte != quote {
                    guard byte != 60, position - start < 4_096 else { throw SavedMapError.xmlMalformed }
                    position += 1
                }
                guard current == quote else { throw SavedMapError.xmlMalformed }
                attributes[key] = try Self.unescape(bytes[start..<position])
                position += 1
            }
            let node = Node(tag, attributes: attributes)
            while let byte = current {
                if byte != 60 {
                    let start = position
                    while let next = current, next != 60 { position += 1 }
                    node.text += try Self.unescape(bytes[start..<position])
                } else if token("<!--") {
                    try comment()
                } else if token("<![CDATA[") {
                    position += 9
                    node.text += try readUntil("]]>")
                } else if token("</") {
                    position += 2
                    let close = try readName()
                    skipSpace()
                    guard close == tag, current == 62 else { throw SavedMapError.xmlMalformed }
                    position += 1
                    return node
                } else if token("<!") {
                    throw SavedMapError.xmlDoctypeRefused
                } else if token("<?") {
                    throw SavedMapError.xmlPIRefused
                } else {
                    node.children.append(try element(depth: depth + 1))
                }
            }
            throw SavedMapError.xmlMalformed
        }

        mutating func comment() throws {
            position += 4
            let text = try readUntil("-->")
            guard !text.contains("--"), !text.hasSuffix("-") else { throw SavedMapError.xmlMalformed }
        }

        mutating func readUntil(_ delimiter: String) throws -> String {
            let start = position
            let match = Array(delimiter.utf8)
            while position < bytes.count {
                if bytes[position...].starts(with: match) {
                    let value = String(decoding: bytes[start..<position], as: UTF8.self)
                    position += match.count
                    return value
                }
                position += 1
            }
            throw SavedMapError.xmlMalformed
        }

        mutating func readName() throws -> String {
            guard let first = current, Self.nameStart(first) else { throw SavedMapError.xmlMalformed }
            let start = position
            position += 1
            while let byte = current, Self.nameStart(byte) || (48...57).contains(byte) || byte == 45 || byte == 46 {
                position += 1
            }
            guard position - start <= 128 else { throw SavedMapError.xmlMalformed }
            return String(decoding: bytes[start..<position], as: UTF8.self)
        }

        private static func nameStart(_ byte: UInt8) -> Bool {
            (65...90).contains(byte) || (97...122).contains(byte) || byte == 58 || byte == 95 || byte >= 128
        }

        @discardableResult
        mutating func skipSpace() -> Bool {
            let start = position
            while let byte = current, Self.space(byte) { position += 1 }
            return position > start
        }

        static func space(_ byte: UInt8) -> Bool { byte == 0x20 || byte == 9 || byte == 10 || byte == 13 }

        func token(_ value: String) -> Bool { bytes[position...].starts(with: value.utf8) }

        static func unescape(_ source: ArraySlice<UInt8>) throws -> String {
            guard source.contains(38) else { return String(decoding: source, as: UTF8.self) }
            var decoded: [UInt8] = []
            var index = source.startIndex
            while index < source.endIndex {
                guard source[index] == 38 else {
                    decoded.append(source[index])
                    index += 1
                    continue
                }
                let limit = min(source.endIndex, index + 11)
                guard let end = source[(index + 1)..<limit].firstIndex(of: 59) else {
                    throw SavedMapError.xmlEntityRefused
                }
                let entity = String(decoding: source[(index + 1)..<end], as: UTF8.self)
                let scalar: Unicode.Scalar
                switch entity {
                case "amp": scalar = "&"
                case "lt": scalar = "<"
                case "gt": scalar = ">"
                case "quot": scalar = "\""
                case "apos": scalar = "'"
                default:
                    let hex = entity.hasPrefix("#x") || entity.hasPrefix("#X")
                    let digits = entity.dropFirst(hex ? 2 : 1)
                    guard entity.hasPrefix("#"), !digits.isEmpty, digits.utf8.allSatisfy({
                        (48...57).contains($0) || (hex && ((65...70).contains($0) || (97...102).contains($0)))
                    }), let code = UInt32(digits, radix: hex ? 16 : 10),
                          let numeric = Unicode.Scalar(code), validXMLScalar(numeric) else {
                        throw SavedMapError.xmlEntityRefused
                    }
                    scalar = numeric
                }
                decoded.append(contentsOf: String(scalar).utf8)
                index = end + 1
            }
            return String(decoding: decoded, as: UTF8.self)
        }
    }
}
