import Foundation
#if canImport(Compression)
import Compression
#endif

/// Bounded XLSX worksheet reader for untrusted ANP spreadsheets.
/// Swift port of Android `StreamingXlsxParser`, `XlsxLimits`, `XlsxCellReference`
/// and `CappedInputStream` (SEC06-B): every bound refuses before the matching
/// allocation, inflated bytes are capped while they are produced, and each
/// ZIP entry must match its declared size and CRC-32.
/// Zero postfix unwrap operators anywhere in this file.

public enum AnpXlsxError: Error, Equatable {
    case notZip
    case unsupportedArchive
    case corruptEntry(String)
    case entryTooLarge(String)
    case sheetMissing(String)
    case invalidWorkbook(String)
    case limitExceeded(String)
    case inflateUnavailable
    case weekMismatch
    case noDataRows
}

public enum AnpXlsxLimits {
    public static let maxEntryBytes = 256 * 1024 * 1024
    public static let maxSharedStrings = 1_000_000
    public static let maxTextChars = 32_768
    public static let maxWorksheets = 1_024
    public static let maxColumnIndex = 16_383
    public static let maxRowNumber = 1_048_576
    public static let maxArchiveEntries = 10_000
}

/// One worksheet row; `rowNumber` is the 1-based Excel row.
public struct AnpSheetRow: Equatable, Sendable {
    public let rowNumber: Int
    public let cells: [String?]

    public init(rowNumber: Int, cells: [String?]) {
        self.rowNumber = rowNumber
        self.cells = cells
    }

    public func cell(_ index: Int) -> String? {
        index >= 0 && index < cells.count ? cells[index] : nil
    }
}

/// Central-directory ZIP reader (stored and deflate entries, no ZIP64, no encryption).
struct AnpZipArchive {
    struct Entry {
        let name: String
        let method: UInt16
        let flags: UInt16
        let crc32: UInt32
        let compressedSize: Int
        let uncompressedSize: Int
        let localHeaderOffset: Int
    }

    private let data: Data
    private let entries: [String: Entry]

    init(data: Data) throws {
        guard data.count >= 22 else { throw AnpXlsxError.notZip }
        var endRecord: Int?
        let lowest = max(0, data.count - 22 - 65_535)
        var offset = data.count - 22
        while offset >= lowest {
            if Self.u32(data, offset) == 0x0605_4B50 {
                endRecord = offset
                break
            }
            offset -= 1
        }
        guard let end = endRecord,
              let total = Self.u16(data, end + 10),
              let directorySize = Self.u32(data, end + 12),
              let directoryOffset = Self.u32(data, end + 16) else {
            throw AnpXlsxError.notZip
        }
        guard total != 0xFFFF, directoryOffset != 0xFFFF_FFFF, Int(total) <= AnpXlsxLimits.maxArchiveEntries else {
            throw AnpXlsxError.unsupportedArchive
        }
        let directoryStart = Int(directoryOffset)
        let directoryEnd = directoryStart + Int(directorySize)
        guard directoryEnd <= end else { throw AnpXlsxError.notZip }
        var parsed: [String: Entry] = [:]
        var cursor = directoryStart
        for _ in 0..<Int(total) {
            guard Self.u32(data, cursor) == 0x0201_4B50,
                  let flags = Self.u16(data, cursor + 8),
                  let method = Self.u16(data, cursor + 10),
                  let crc = Self.u32(data, cursor + 16),
                  let compressed = Self.u32(data, cursor + 20),
                  let uncompressed = Self.u32(data, cursor + 24),
                  let nameLength = Self.u16(data, cursor + 28),
                  let extraLength = Self.u16(data, cursor + 30),
                  let commentLength = Self.u16(data, cursor + 32),
                  let localOffset = Self.u32(data, cursor + 42) else {
                throw AnpXlsxError.notZip
            }
            guard compressed != 0xFFFF_FFFF, uncompressed != 0xFFFF_FFFF, localOffset != 0xFFFF_FFFF else {
                throw AnpXlsxError.unsupportedArchive
            }
            let nameStart = cursor + 46
            let nameEnd = nameStart + Int(nameLength)
            guard nameEnd <= directoryEnd else { throw AnpXlsxError.notZip }
            let nameBytes = data.subdata(in: (data.startIndex + nameStart)..<(data.startIndex + nameEnd))
            let name = String(decoding: nameBytes, as: UTF8.self)
            if parsed[name] == nil {
                parsed[name] = Entry(
                    name: name,
                    method: method,
                    flags: flags,
                    crc32: crc,
                    compressedSize: Int(compressed),
                    uncompressedSize: Int(uncompressed),
                    localHeaderOffset: Int(localOffset)
                )
            }
            cursor = nameEnd + Int(extraLength) + Int(commentLength)
        }
        self.data = data
        self.entries = parsed
    }

    func contains(_ name: String) -> Bool {
        entries[name] != nil
    }

    /// Inflated entry bytes, or nil when the archive has no such entry.
    func extract(_ name: String, maxBytes: Int) throws -> Data? {
        guard let entry = entries[name] else { return nil }
        guard (entry.flags & 0x0001) == 0 else { throw AnpXlsxError.unsupportedArchive }
        guard entry.uncompressedSize <= maxBytes else { throw AnpXlsxError.entryTooLarge(name) }
        let local = entry.localHeaderOffset
        guard Self.u32(data, local) == 0x0403_4B50,
              let nameLength = Self.u16(data, local + 26),
              let extraLength = Self.u16(data, local + 28) else {
            throw AnpXlsxError.corruptEntry(name)
        }
        let start = local + 30 + Int(nameLength) + Int(extraLength)
        let end = start + entry.compressedSize
        guard start >= 0, start <= end, end <= data.count else { throw AnpXlsxError.corruptEntry(name) }
        let payload = data.subdata(in: (data.startIndex + start)..<(data.startIndex + end))
        let output: Data
        switch entry.method {
        case 0:
            output = payload
        case 8:
            output = try AnpInflate.raw(payload, maxBytes: maxBytes)
        default:
            throw AnpXlsxError.unsupportedArchive
        }
        guard output.count == entry.uncompressedSize, AnpCrc32.checksum(output) == entry.crc32 else {
            throw AnpXlsxError.corruptEntry(name)
        }
        return output
    }

    static func u16(_ data: Data, _ offset: Int) -> UInt16? {
        guard offset >= 0, offset + 2 <= data.count else { return nil }
        let base = data.startIndex + offset
        return UInt16(data[base]) | (UInt16(data[base + 1]) << 8)
    }

    static func u32(_ data: Data, _ offset: Int) -> UInt32? {
        guard offset >= 0, offset + 4 <= data.count else { return nil }
        let base = data.startIndex + offset
        return UInt32(data[base])
            | (UInt32(data[base + 1]) << 8)
            | (UInt32(data[base + 2]) << 16)
            | (UInt32(data[base + 3]) << 24)
    }
}

/// Raw DEFLATE (RFC 1951) decoding with the output cap enforced chunk by chunk.
enum AnpInflate {
    static func raw(_ input: Data, maxBytes: Int) throws -> Data {
        #if canImport(Compression)
        if input.isEmpty { return Data() }
        let stream = UnsafeMutablePointer<compression_stream>.allocate(capacity: 1)
        defer { stream.deallocate() }
        guard compression_stream_init(stream, COMPRESSION_STREAM_DECODE, COMPRESSION_ZLIB) == COMPRESSION_STATUS_OK else {
            throw AnpXlsxError.inflateUnavailable
        }
        defer { compression_stream_destroy(stream) }
        let chunk = 64 * 1024
        let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: chunk)
        defer { buffer.deallocate() }
        var output = Data()
        try input.withUnsafeBytes { (raw: UnsafeRawBufferPointer) in
            guard let source = raw.baseAddress?.assumingMemoryBound(to: UInt8.self) else {
                throw AnpXlsxError.corruptEntry("deflate")
            }
            stream.pointee.src_ptr = source
            stream.pointee.src_size = raw.count
            var status = COMPRESSION_STATUS_OK
            repeat {
                stream.pointee.dst_ptr = buffer
                stream.pointee.dst_size = chunk
                let pendingInput = stream.pointee.src_size
                status = compression_stream_process(stream, Int32(COMPRESSION_STREAM_FINALIZE.rawValue))
                guard status == COMPRESSION_STATUS_OK || status == COMPRESSION_STATUS_END else {
                    throw AnpXlsxError.corruptEntry("deflate")
                }
                let produced = chunk - stream.pointee.dst_size
                guard output.count + produced <= maxBytes else {
                    throw AnpXlsxError.limitExceeded("inflated entry")
                }
                if produced > 0 {
                    output.append(buffer, count: produced)
                } else if status == COMPRESSION_STATUS_OK && stream.pointee.src_size == pendingInput {
                    throw AnpXlsxError.corruptEntry("truncated deflate stream")
                }
            } while status == COMPRESSION_STATUS_OK
        }
        return output
        #else
        throw AnpXlsxError.inflateUnavailable
        #endif
    }
}

enum AnpCrc32 {
    static let table: [UInt32] = (0..<256).map { index -> UInt32 in
        var value = UInt32(index)
        for _ in 0..<8 {
            value = (value & 1) != 0 ? (0xEDB8_8320 ^ (value >> 1)) : (value >> 1)
        }
        return value
    }

    static func checksum(_ data: Data) -> UInt32 {
        var crc: UInt32 = 0xFFFF_FFFF
        let lookup = table
        data.withUnsafeBytes { (raw: UnsafeRawBufferPointer) in
            for byte in raw {
                crc = lookup[Int((crc ^ UInt32(byte)) & 0xFF)] ^ (crc >> 8)
            }
        }
        return crc ^ 0xFFFF_FFFF
    }
}

private let anpSpreadsheetNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

private func anpIsSheetTag(_ elementName: String, _ namespaceURI: String?, _ localName: String) -> Bool {
    guard elementName == localName else { return false }
    guard let namespace = namespaceURI, !namespace.isEmpty else { return true }
    return namespace == anpSpreadsheetNamespace
}

/// Zero-based column of an A1 reference, refused outside the Excel grid (XFD).
func anpColumnIndex(_ reference: String) -> Int? {
    let letters = Array(reference.unicodeScalars.prefix(while: { scalar in
        (scalar.value >= 65 && scalar.value <= 90) || (scalar.value >= 97 && scalar.value <= 122)
    }))
    guard !letters.isEmpty, letters.count <= 3 else { return nil }
    var index = 0
    for scalar in letters {
        let upper = scalar.value >= 97 ? scalar.value - 32 : scalar.value
        index = index * 26 + Int(upper - 64)
    }
    guard index >= 1, index <= AnpXlsxLimits.maxColumnIndex + 1 else { return nil }
    return index - 1
}

protocol AnpXmlFailing: AnyObject {
    var failure: AnpXlsxError? { get }
}

final class AnpSharedStringsHandler: NSObject, XMLParserDelegate, AnpXmlFailing {
    private(set) var strings: [String] = []
    var failure: AnpXlsxError?
    private var inItem = false
    private var inText = false
    private var item = ""
    private var itemLength = 0

    func parser(_ parser: XMLParser, didStartElement elementName: String, namespaceURI: String?, qualifiedName qName: String?, attributes attributeDict: [String: String] = [:]) {
        if anpIsSheetTag(elementName, namespaceURI, "si") {
            guard strings.count < AnpXlsxLimits.maxSharedStrings else {
                stop(parser, .limitExceeded("shared strings"))
                return
            }
            inItem = true
            item = ""
            itemLength = 0
        } else if inItem && anpIsSheetTag(elementName, namespaceURI, "t") {
            inText = true
        }
    }

    func parser(_ parser: XMLParser, didEndElement elementName: String, namespaceURI: String?, qualifiedName qName: String?) {
        if anpIsSheetTag(elementName, namespaceURI, "t") {
            inText = false
        } else if inItem && anpIsSheetTag(elementName, namespaceURI, "si") {
            strings.append(item)
            inItem = false
        }
    }

    func parser(_ parser: XMLParser, foundCharacters string: String) {
        appendText(string, parser)
    }

    func parser(_ parser: XMLParser, foundCDATA CDATABlock: Data) {
        appendText(String(decoding: CDATABlock, as: UTF8.self), parser)
    }

    private func appendText(_ text: String, _ parser: XMLParser) {
        guard inText, failure == nil else { return }
        itemLength += text.utf16.count
        guard itemLength <= AnpXlsxLimits.maxTextChars else {
            stop(parser, .limitExceeded("shared string"))
            return
        }
        item += text
    }

    private func stop(_ parser: XMLParser, _ error: AnpXlsxError) {
        if failure == nil { failure = error }
        parser.abortParsing()
    }
}

final class AnpWorkbookHandler: NSObject, XMLParserDelegate, AnpXmlFailing {
    private(set) var sheets: [(name: String, relationshipId: String)] = []
    var failure: AnpXlsxError?

    func parser(_ parser: XMLParser, didStartElement elementName: String, namespaceURI: String?, qualifiedName qName: String?, attributes attributeDict: [String: String] = [:]) {
        guard anpIsSheetTag(elementName, namespaceURI, "sheet") else { return }
        let relationship = attributeDict.first { key, _ in key == "id" || key.hasSuffix(":id") }?.value ?? ""
        sheets.append((name: attributeDict["name"] ?? "", relationshipId: relationship))
        if sheets.count > AnpXlsxLimits.maxWorksheets {
            if failure == nil { failure = .limitExceeded("worksheets") }
            parser.abortParsing()
        }
    }
}

final class AnpRelationshipsHandler: NSObject, XMLParserDelegate, AnpXmlFailing {
    private(set) var targets: [String: String] = [:]
    var failure: AnpXlsxError?

    func parser(_ parser: XMLParser, didStartElement elementName: String, namespaceURI: String?, qualifiedName qName: String?, attributes attributeDict: [String: String] = [:]) {
        guard elementName == "Relationship", let id = attributeDict["Id"], let target = attributeDict["Target"] else { return }
        if targets[id] == nil { targets[id] = target }
        if targets.count > AnpXlsxLimits.maxWorksheets * 4 {
            if failure == nil { failure = .limitExceeded("relationships") }
            parser.abortParsing()
        }
    }
}

final class AnpSheetRowsHandler: NSObject, XMLParserDelegate, AnpXmlFailing {
    private enum Capture {
        case idle
        case value
        case text
    }

    var failure: AnpXlsxError?
    private let sharedStrings: [String]
    private let firstDataRow: Int
    private let headerRow: Int?
    private let onRow: (AnpSheetRow) -> Bool
    private var collecting = false
    private var rowNumber = 0
    private var cells: [Int: String] = [:]
    private var cellColumn: Int?
    private var cellType: String?
    private var value: String?
    private var inline: String?
    private var inInlineString = false
    private var capture = Capture.idle
    private var buffer = ""
    private var bufferLength = 0

    init(sharedStrings: [String], firstDataRow: Int, headerRow: Int?, onRow: @escaping (AnpSheetRow) -> Bool) {
        self.sharedStrings = sharedStrings
        self.firstDataRow = firstDataRow
        self.headerRow = headerRow
        self.onRow = onRow
    }

    func parser(_ parser: XMLParser, didStartElement elementName: String, namespaceURI: String?, qualifiedName qName: String?, attributes attributeDict: [String: String] = [:]) {
        guard failure == nil else { return }
        if anpIsSheetTag(elementName, namespaceURI, "row") {
            guard let raw = attributeDict["r"], let number = Int(raw), (1...AnpXlsxLimits.maxRowNumber).contains(number) else {
                stop(parser, .invalidWorkbook("row reference"))
                return
            }
            rowNumber = number
            collecting = number >= firstDataRow || number == headerRow
            cells = [:]
            return
        }
        guard collecting else { return }
        if anpIsSheetTag(elementName, namespaceURI, "c") {
            guard let reference = attributeDict["r"], let column = anpColumnIndex(reference) else {
                stop(parser, .invalidWorkbook("cell reference"))
                return
            }
            guard cells.count <= AnpXlsxLimits.maxColumnIndex else {
                stop(parser, .limitExceeded("row width"))
                return
            }
            cellColumn = column
            cellType = attributeDict["t"]
            value = nil
            inline = nil
            inInlineString = false
            capture = .idle
        } else if cellColumn != nil && anpIsSheetTag(elementName, namespaceURI, "v") {
            beginCapture(.value)
        } else if cellColumn != nil && anpIsSheetTag(elementName, namespaceURI, "is") {
            inInlineString = true
            inline = ""
        } else if cellColumn != nil && anpIsSheetTag(elementName, namespaceURI, "t") {
            beginCapture(.text)
        }
    }

    func parser(_ parser: XMLParser, didEndElement elementName: String, namespaceURI: String?, qualifiedName qName: String?) {
        guard failure == nil else { return }
        if anpIsSheetTag(elementName, namespaceURI, "row") {
            guard collecting else { return }
            collecting = false
            let width = (cells.keys.max() ?? -1) + 1
            let row = AnpSheetRow(rowNumber: rowNumber, cells: (0..<width).map { cells[$0] })
            if !onRow(row) { stop(parser, .limitExceeded("rows")) }
            return
        }
        guard collecting, let column = cellColumn else { return }
        if anpIsSheetTag(elementName, namespaceURI, "v") {
            value = buffer
            capture = .idle
        } else if anpIsSheetTag(elementName, namespaceURI, "t") {
            if inInlineString {
                let combined = (inline ?? "") + buffer
                guard combined.utf16.count <= AnpXlsxLimits.maxTextChars else {
                    stop(parser, .limitExceeded("inline string"))
                    return
                }
                inline = combined
            } else {
                inline = buffer
            }
            capture = .idle
        } else if anpIsSheetTag(elementName, namespaceURI, "is") {
            inInlineString = false
        } else if anpIsSheetTag(elementName, namespaceURI, "c") {
            switch resolveCell() {
            case .success(let resolved):
                if let resolved { cells[column] = resolved }
            case .failure(let error):
                stop(parser, error)
                return
            }
            cellColumn = nil
        }
    }

    func parser(_ parser: XMLParser, foundCharacters string: String) {
        appendText(string, parser)
    }

    func parser(_ parser: XMLParser, foundCDATA CDATABlock: Data) {
        appendText(String(decoding: CDATABlock, as: UTF8.self), parser)
    }

    private func beginCapture(_ mode: Capture) {
        capture = mode
        buffer = ""
        bufferLength = 0
    }

    private func appendText(_ text: String, _ parser: XMLParser) {
        guard failure == nil, capture != .idle else { return }
        bufferLength += text.utf16.count
        guard bufferLength <= AnpXlsxLimits.maxTextChars else {
            stop(parser, .limitExceeded("text run"))
            return
        }
        buffer += text
    }

    private func resolveCell() -> Result<String?, AnpXlsxError> {
        switch cellType {
        case "s":
            guard let raw = value?.trimmingCharacters(in: .whitespacesAndNewlines), let index = Int(raw) else {
                return .success(nil)
            }
            guard index >= 0, index < sharedStrings.count else {
                return .failure(.invalidWorkbook("shared string index"))
            }
            return .success(sharedStrings[index])
        case "inlineStr":
            return .success(inline)
        default:
            return .success(value ?? inline)
        }
    }

    private func stop(_ parser: XMLParser, _ error: AnpXlsxError) {
        if failure == nil { failure = error }
        parser.abortParsing()
    }
}

public enum AnpXlsxReader {
    /// Streams the rows of `sheetName` at or after `firstDataRow` (plus `headerRow`).
    /// `onRow` returns false to stop with a row-limit refusal.
    public static func readSheet(
        xlsx: Data,
        sheetName: String,
        firstDataRow: Int,
        headerRow: Int?,
        onRow: @escaping (AnpSheetRow) -> Bool
    ) throws {
        guard firstDataRow > 0, headerRow.map({ $0 > 0 }) ?? true else {
            throw AnpXlsxError.invalidWorkbook("row options")
        }
        let archive = try AnpZipArchive(data: xlsx)
        var sharedStrings: [String] = []
        if let stringsXml = try archive.extract("xl/sharedStrings.xml", maxBytes: AnpXlsxLimits.maxEntryBytes) {
            let handler = AnpSharedStringsHandler()
            try run(stringsXml, handler)
            sharedStrings = handler.strings
        }
        guard let workbookXml = try archive.extract("xl/workbook.xml", maxBytes: AnpXlsxLimits.maxEntryBytes) else {
            throw AnpXlsxError.sheetMissing(sheetName)
        }
        let workbook = AnpWorkbookHandler()
        try run(workbookXml, workbook)
        guard let sheet = workbook.sheets.first(where: { $0.name == sheetName }) else {
            throw AnpXlsxError.sheetMissing(sheetName)
        }
        guard !sheet.relationshipId.isEmpty,
              let relsXml = try archive.extract("xl/_rels/workbook.xml.rels", maxBytes: AnpXlsxLimits.maxEntryBytes) else {
            throw AnpXlsxError.invalidWorkbook("relationship")
        }
        let relationships = AnpRelationshipsHandler()
        try run(relsXml, relationships)
        guard let target = relationships.targets[sheet.relationshipId] else {
            throw AnpXlsxError.invalidWorkbook("relationship target")
        }
        let path = target.hasPrefix("/") ? String(target.dropFirst()) : "xl/\(target)"
        guard let sheetXml = try archive.extract(path, maxBytes: AnpXlsxLimits.maxEntryBytes) else {
            throw AnpXlsxError.sheetMissing(sheetName)
        }
        let rows = AnpSheetRowsHandler(
            sharedStrings: sharedStrings, firstDataRow: firstDataRow, headerRow: headerRow, onRow: onRow
        )
        try run(sheetXml, rows)
    }

    private static func run<Handler: NSObject & XMLParserDelegate & AnpXmlFailing>(_ xml: Data, _ handler: Handler) throws {
        let parser = XMLParser(data: xml)
        parser.shouldProcessNamespaces = true
        parser.shouldResolveExternalEntities = false
        parser.delegate = handler
        let completed = parser.parse()
        parser.delegate = nil
        if let failure = handler.failure { throw failure }
        guard completed else { throw AnpXlsxError.invalidWorkbook("xml") }
    }
}
