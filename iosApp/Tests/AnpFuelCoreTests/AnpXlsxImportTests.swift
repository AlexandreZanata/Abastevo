import XCTest
#if canImport(Compression)
import Compression
#endif
@testable import AnpFuelCore

/// Minimal ZIP writer for synthetic XLSX inputs (no production data).
enum TestZip {
    struct File {
        let name: String
        let content: Data
        let deflate: Bool
    }

    static func make(_ files: [File], corruptCrcOf: String? = nil, lieAboutSizeOf: String? = nil) -> Data {
        var archive = Data()
        var central = Data()
        for file in files {
            let payload = file.deflate ? deflate(file.content) : file.content
            let method: UInt16 = file.deflate ? 8 : 0
            var crc = AnpCrc32.checksum(file.content)
            if file.name == corruptCrcOf { crc ^= 1 }
            let declared = UInt32(file.content.count) + (file.name == lieAboutSizeOf ? 1 : 0)
            let offset = UInt32(archive.count)
            let name = Data(file.name.utf8)
            archive.append(le32(0x0403_4B50))
            archive.append(le16(20)); archive.append(le16(0)); archive.append(le16(method))
            archive.append(le16(0)); archive.append(le16(0)); archive.append(le32(crc))
            archive.append(le32(UInt32(payload.count))); archive.append(le32(declared))
            archive.append(le16(UInt16(name.count))); archive.append(le16(0))
            archive.append(name)
            archive.append(payload)
            central.append(le32(0x0201_4B50))
            central.append(le16(20)); central.append(le16(20)); central.append(le16(0)); central.append(le16(method))
            central.append(le16(0)); central.append(le16(0)); central.append(le32(crc))
            central.append(le32(UInt32(payload.count))); central.append(le32(declared))
            central.append(le16(UInt16(name.count))); central.append(le16(0)); central.append(le16(0))
            central.append(le16(0)); central.append(le16(0)); central.append(le32(0)); central.append(le32(offset))
            central.append(name)
        }
        let directoryOffset = UInt32(archive.count)
        archive.append(central)
        archive.append(le32(0x0605_4B50))
        archive.append(le16(0)); archive.append(le16(0))
        archive.append(le16(UInt16(files.count))); archive.append(le16(UInt16(files.count)))
        archive.append(le32(UInt32(central.count))); archive.append(le32(directoryOffset)); archive.append(le16(0))
        return archive
    }

    static func deflate(_ data: Data) -> Data {
        #if canImport(Compression)
        let capacity = data.count + 1_024
        var output = Data(count: capacity)
        let written = output.withUnsafeMutableBytes { (destination: UnsafeMutableRawBufferPointer) -> Int in
            data.withUnsafeBytes { (source: UnsafeRawBufferPointer) -> Int in
                guard let target = destination.baseAddress?.assumingMemoryBound(to: UInt8.self),
                      let input = source.baseAddress?.assumingMemoryBound(to: UInt8.self) else { return 0 }
                return compression_encode_buffer(target, capacity, input, data.count, nil, COMPRESSION_ZLIB)
            }
        }
        return Data(output.prefix(written))
        #else
        return data
        #endif
    }

    private static func le16(_ value: UInt16) -> Data {
        Data([UInt8(value & 0xFF), UInt8(value >> 8)])
    }

    private static func le32(_ value: UInt32) -> Data {
        Data([UInt8(value & 0xFF), UInt8((value >> 8) & 0xFF), UInt8((value >> 16) & 0xFF), UInt8(value >> 24)])
    }
}

/// Synthetic ANP-shaped workbooks: same sheet names, header row and columns as gov.br files.
enum TestWorkbook {
    static let main = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

    static func xlsx(sheetName: String, rows: [[String?]], sharedStrings: [String] = [], deflate: Bool = true) -> Data {
        let relationshipsNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
        let packageNamespace = "http://schemas.openxmlformats.org/package/2006/relationships"
        let workbook = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>"
            + "<workbook xmlns=\"\(main)\" xmlns:r=\"\(relationshipsNamespace)\"><sheets>"
            + "<sheet name=\"CAPA\" sheetId=\"1\" r:id=\"rId1\"/>"
            + "<sheet name=\"\(sheetName)\" sheetId=\"2\" r:id=\"rId2\"/></sheets></workbook>"
        let rels = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>"
            + "<Relationships xmlns=\"\(packageNamespace)\">"
            + "<Relationship Id=\"rId1\" Type=\"worksheet\" Target=\"worksheets/sheet1.xml\"/>"
            + "<Relationship Id=\"rId2\" Type=\"worksheet\" Target=\"worksheets/sheet2.xml\"/></Relationships>"
        let strings = "<sst xmlns=\"\(main)\">" + sharedStrings.map { "<si><t>\(escape($0))</t></si>" }.joined() + "</sst>"
        var sheet = "<worksheet xmlns=\"\(main)\"><sheetData><row r=\"10\"><c r=\"A10\" t=\"inlineStr\"><is><t>CABEÇALHO</t></is></c></row>"
        for (index, row) in rows.enumerated() {
            let number = 11 + index
            sheet += "<row r=\"\(number)\">"
            for (column, value) in row.enumerated() {
                guard let value else { continue }
                let reference = "\(columnName(column))\(number)"
                if value.hasPrefix("#s") {
                    sheet += "<c r=\"\(reference)\" t=\"s\"><v>\(value.dropFirst(2))</v></c>"
                } else if Double(value) != nil {
                    sheet += "<c r=\"\(reference)\"><v>\(value)</v></c>"
                } else {
                    sheet += "<c r=\"\(reference)\" t=\"inlineStr\"><is><t>\(escape(value))</t></is></c>"
                }
            }
            sheet += "</row>"
        }
        sheet += "</sheetData></worksheet>"
        let empty = "<worksheet xmlns=\"\(main)\"><sheetData/></worksheet>"
        return TestZip.make([
            TestZip.File(name: "xl/workbook.xml", content: Data(workbook.utf8), deflate: deflate),
            TestZip.File(name: "xl/_rels/workbook.xml.rels", content: Data(rels.utf8), deflate: false),
            TestZip.File(name: "xl/sharedStrings.xml", content: Data(strings.utf8), deflate: deflate),
            TestZip.File(name: "xl/worksheets/sheet1.xml", content: Data(empty.utf8), deflate: false),
            TestZip.File(name: "xl/worksheets/sheet2.xml", content: Data(sheet.utf8), deflate: deflate),
        ])
    }

    static func columnName(_ index: Int) -> String {
        let letters = Array("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
        return index < 26 ? String(letters[index]) : String(letters[index / 26 - 1]) + String(letters[index % 26])
    }

    static func escape(_ text: String) -> String {
        text.replacingOccurrences(of: "&", with: "&amp;").replacingOccurrences(of: "<", with: "&lt;")
    }
}

final class AnpXlsxImportTests: XCTestCase {
    private let week = AnpSurveyWeekRef(startIso: "2026-11-09", endIso: "2026-11-15")

    /// Columns: start, end, state, municipality, product, stations, unit, average, sd, min, max, cv.
    private func summaryRow(_ state: String, _ city: String, _ product: String, average: String, start: String = "46335", end: String = "46341") -> [String?] {
        [start, end, state, city, product, "12", "R$/l", average, "0.1", "5.99", "6.49", "0.02"]
    }

    func testWeeklySummaryImportMapsRowsAndQuarantinesUnknownLabels() throws {
        let expected = try XCTUnwrap(week)
        let xlsx = TestWorkbook.xlsx(
            sheetName: AnpWeekImporter.summarySheet,
            rows: [
                summaryRow("SAO PAULO", "#s0", "GASOLINA COMUM", average: "6.385"),
                summaryRow("SP", "SAO PAULO", "ETANOL HIDRATADO", average: "4.1899999999999995"),
                summaryRow("MATO GROSSO", "SORRISO", "OLEO DIESEL S10", average: "6.29"),
                summaryRow("SAO PAULO", "SAO PAULO", "GASOLINA PODIUM", average: "7.00"),
                summaryRow("ATLANTIDA", "X", "GNV", average: "4.00"),
                [nil, nil, "SP", "SAO PAULO", "", nil, nil, nil, nil, nil, nil, nil],
            ],
            sharedStrings: ["SÃO PAULO"]
        )
        let imported = try AnpWeekImporter.importSummary(xlsx: xlsx, expectedWeek: expected)
        XCTAssertEqual(imported.week, expected)
        XCTAssertEqual(imported.averages.count, 3)
        XCTAssertEqual(imported.quarantinedRows, 2)
        let city = Municipality(ibgeCode: "3550308", name: "SÃO PAULO", state: .SP)
        let saoPaulo = AnpOfficialRules.cityAverages(imported.averages, city: city)
        XCTAssertEqual(saoPaulo.map(\.product), [.ethanol, .gasolineRegular])
        XCTAssertEqual(saoPaulo.first?.averageMilli, 4_190)
        XCTAssertEqual(saoPaulo.last?.averageMilli, 6_390)
        XCTAssertEqual(saoPaulo.last?.minimumMilli, 5_990)
        XCTAssertEqual(saoPaulo.last?.stationCount, 12)
    }

    func testSummaryForAnotherWeekIsRefused() throws {
        let other = try XCTUnwrap(AnpSurveyWeekRef(startIso: "2026-11-02", endIso: "2026-11-08"))
        let xlsx = TestWorkbook.xlsx(sheetName: AnpWeekImporter.summarySheet, rows: [summaryRow("SP", "SAO PAULO", "GASOLINA COMUM", average: "6.1")])
        XCTAssertThrowsError(try AnpWeekImporter.importSummary(xlsx: xlsx, expectedWeek: other)) { error in
            XCTAssertEqual(error as? AnpXlsxError, .weekMismatch)
        }
        let empty = TestWorkbook.xlsx(sheetName: AnpWeekImporter.summarySheet, rows: [])
        XCTAssertThrowsError(try AnpWeekImporter.importSummary(xlsx: empty, expectedWeek: other)) { error in
            XCTAssertEqual(error as? AnpXlsxError, .noDataRows)
        }
        let wrongSheet = TestWorkbook.xlsx(sheetName: "RESUMO", rows: [summaryRow("SP", "SAO PAULO", "GASOLINA COMUM", average: "6.1")])
        XCTAssertThrowsError(try AnpWeekImporter.importSummary(xlsx: wrongSheet, expectedWeek: other)) { error in
            XCTAssertEqual(error as? AnpXlsxError, .sheetMissing(AnpWeekImporter.summarySheet))
        }
    }

    func testStationDetailImportKeepsOnlyTheScopedState() throws {
        func row(_ cnpj: String, _ state: String, _ city: String, _ product: String, _ price: String) -> [String?] {
            [cnpj, "RAZAO", "POSTO \(cnpj.suffix(2))", "AV BRASIL", "100", nil, "CENTRO", "78890000", city, state, "BANDEIRA", product, "R$ / litro", price, "46336"]
        }
        let xlsx = TestWorkbook.xlsx(sheetName: AnpWeekImporter.stationSheet, rows: [
            row("4218406000104", "MATO GROSSO", "SORRISO", "GASOLINA COMUM", "6.19"),
            row("04.218.406/0001-04", "MATO GROSSO", "SORRISO", "GASOLINA COMUM", "6.29"),
            row("11222333000181", "MATO GROSSO", "SORRISO", "ETANOL", "4.49"),
            row("11222333000181", "SAO PAULO", "SAO PAULO", "ETANOL", "3.99"),
            row("11222333000181", "MATO GROSSO", "SORRISO", "GASOLINA PODIUM", "7.00"),
        ])
        let imported = try AnpWeekImporter.importStations(xlsx: xlsx, scope: .MT)
        XCTAssertEqual(imported.stations.count, 2, "duplicate station and product keeps the last row")
        XCTAssertEqual(imported.quarantinedRows, 1)
        let gasoline = try XCTUnwrap(imported.stations.first { $0.product == .gasolineRegular })
        XCTAssertEqual(gasoline.cnpj, "04218406000104")
        XCTAssertEqual(gasoline.priceMilli, 6_290)
        XCTAssertEqual(gasoline.collectedOnIso, "2026-11-10")
        XCTAssertEqual(gasoline.address, "AV BRASIL, nº 100, CENTRO")
        XCTAssertTrue(imported.stations.allSatisfy { $0.state == .MT })
    }

    func testZipIntegrityRefusesCorruptOrOversizedEntries() throws {
        let body = Data(String(repeating: "abc", count: 400).utf8)
        let good = TestZip.make([TestZip.File(name: "a.xml", content: body, deflate: true)])
        let archive = try AnpZipArchive(data: good)
        XCTAssertEqual(try archive.extract("a.xml", maxBytes: 10_000), body)
        XCTAssertNil(try archive.extract("missing.xml", maxBytes: 10_000))
        XCTAssertThrowsError(try archive.extract("a.xml", maxBytes: 100)) { error in
            XCTAssertEqual(error as? AnpXlsxError, .entryTooLarge("a.xml"))
        }
        let corrupt = try AnpZipArchive(data: TestZip.make([TestZip.File(name: "a.xml", content: body, deflate: true)], corruptCrcOf: "a.xml"))
        XCTAssertThrowsError(try corrupt.extract("a.xml", maxBytes: 10_000)) { error in
            XCTAssertEqual(error as? AnpXlsxError, .corruptEntry("a.xml"))
        }
        let lying = try AnpZipArchive(data: TestZip.make([TestZip.File(name: "a.xml", content: body, deflate: false)], lieAboutSizeOf: "a.xml"))
        XCTAssertThrowsError(try lying.extract("a.xml", maxBytes: 10_000))
        XCTAssertThrowsError(try AnpZipArchive(data: Data("not a zip".utf8)))
        XCTAssertThrowsError(try AnpInflate.raw(TestZip.deflate(body), maxBytes: 100)) { error in
            XCTAssertEqual(error as? AnpXlsxError, .limitExceeded("inflated entry"))
        }
        XCTAssertFalse(AnpSurveyClient.looksLikeSpreadsheet(Data("<html>".utf8)))
        XCTAssertTrue(AnpSurveyClient.looksLikeSpreadsheet(good))
        XCTAssertTrue(AnpSurveyClient.isAcceptedContentType(nil))
        XCTAssertTrue(AnpSurveyClient.isAcceptedContentType("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet; charset=binary"))
        XCTAssertFalse(AnpSurveyClient.isAcceptedContentType("text/html"))
    }

    func testCellReferencesStayInsideTheExcelGrid() {
        XCTAssertEqual(anpColumnIndex("A11"), 0)
        XCTAssertEqual(anpColumnIndex("N11"), 13)
        XCTAssertEqual(anpColumnIndex("AA1"), 26)
        XCTAssertEqual(anpColumnIndex("XFD1"), 16_383)
        XCTAssertNil(anpColumnIndex("XFE1"))
        XCTAssertNil(anpColumnIndex("ZZZZ1"))
        XCTAssertNil(anpColumnIndex("11"))
    }

    func testDownloadStoreVerifiesDigestsAndKeepsOlderStateReadable() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("anp-store-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = AnpDownloadStore(directory: directory)
        let entry = AnpSurveyCatalogEntry(
            week: try XCTUnwrap(week),
            summaryUrl: "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/resumo_semanal_lpc_2026-11-09_2026-11-15.xlsx",
            stationUrl: "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas_lpc_2026-11-09_2026-11-15.xlsx",
            publishedOnIso: "2026-11-18"
        )
        let average = AnpMunicipalityAverage(state: .MT, municipality: "SORRISO", product: .gasolineRegular, stationCount: 9, unit: "R$/l", averageMilli: 6_290, minimumMilli: 6_190, maximumMilli: 6_490)
        let station = AnpStationWeekPrice(cnpj: "04218406000104", tradeName: "POSTO", legalName: nil, brand: nil, address: "AV", municipality: "SORRISO", state: .MT, product: .gasolineRegular, priceMilli: 6_190, collectedOnIso: "2026-11-10")
        let manifest = try store.save(
            entry: entry,
            summary: AnpSummaryImport(week: entry.week, averages: [average], quarantinedRows: 1),
            summarySource: AnpSourceFileDigest(url: entry.summaryUrl, sha256: String(repeating: "a", count: 64), bytes: 1_000),
            stations: AnpStationImport(stations: [station], quarantinedRows: 0),
            stationSource: AnpSourceFileDigest(url: entry.stationUrl, sha256: String(repeating: "b", count: 64), bytes: 5_000),
            scope: .MT,
            now: Date(timeIntervalSince1970: 1_800_000_000)
        )
        XCTAssertTrue(manifest.hasStationData)
        XCTAssertEqual(manifest.downloadedBytes, 6_000)
        XCTAssertEqual(store.manifests().map(\.week), [entry.week])
        XCTAssertEqual(store.averages(weekId: entry.week.id), [average])
        XCTAssertEqual(store.stations(weekId: entry.week.id), [station])

        let averagesFile = directory.appendingPathComponent("\(entry.week.id).averages.json")
        try Data("[]".utf8).write(to: averagesFile)
        XCTAssertNil(store.averages(weekId: entry.week.id), "tampered rows fail closed")
        XCTAssertTrue(store.manifests().isEmpty, "a corrupt week is dropped")

        try Data(#"{"manifests":[]}"#.utf8).write(to: directory.appendingPathComponent("anp-state-v1.json"))
        let state = store.loadState()
        XCTAssertNil(state.lastCatalogCheckEpochSeconds)
        XCTAssertTrue(state.notifiedWeekByAlert.isEmpty)
    }

    func testOfficialStationHistoryDecodesTheBackendShape() throws {
        let body = Data("""
        {"items":[
          {"revision_id":"00000000-0000-4000-8000-000000000010","survey_week":{"start":"2026-11-02","end":"2026-11-08"},
           "collected_on":"2026-11-03","fuel_product":"GASOLINE_REGULAR","unit":"L","amount_milli_brl":6190,"currency":"BRL",
           "source":{"kind":"ANP","url":"gov.br/anp/x.xlsx","checksum":"sha256:a"}},
          {"revision_id":"00000000-0000-4000-8000-000000000011","survey_week":{"start":"2026-11-09","end":"2026-11-15"},
           "collected_on":"2026-11-10","fuel_product":"GASOLINE_REGULAR","unit":"L","amount_milli_brl":6290,"currency":"BRL",
           "source":{"kind":"ANP","url":"gov.br/anp/y.xlsx","checksum":"sha256:b"}}
        ],"next_cursor":null,"generated_at":"2026-11-23T10:00:00Z"}
        """.utf8)
        let entries = try AbastevoHttpClient.decodeOfficialHistory(body, fuel: .gasolineRegular)
        XCTAssertEqual(entries.map(\.amountMilli), [6_290, 6_190])
        XCTAssertThrowsError(try AbastevoHttpClient.decodeOfficialHistory(body, fuel: .ethanol))
        let empty = Data(#"{"items":null,"next_cursor":null,"generated_at":"2026-11-23T10:00:00Z"}"#.utf8)
        XCTAssertTrue(try AbastevoHttpClient.decodeOfficialHistory(empty, fuel: .ethanol).isEmpty)
    }
}
