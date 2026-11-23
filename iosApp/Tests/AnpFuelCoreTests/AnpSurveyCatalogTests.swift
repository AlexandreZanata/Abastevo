import XCTest
@testable import AnpFuelCore

final class AnpSurveyCatalogTests: XCTestCase {
    private func repoFile(_ relative: String) -> URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent(relative)
    }

    func testPatternsCompile() {
        XCTAssertTrue(AnpPriceTableUrl.patternsCompile)
        XCTAssertTrue(AnpListingParser.patternsCompile)
    }

    func testApprovedOriginsAreHttpsGovBrAnpHierarchyOnly() {
        let base = "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/"
        XCTAssertTrue(AnpEndpoints.isApprovedUrl(AnpEndpoints.listingPageUrl))
        XCTAssertTrue(AnpEndpoints.isApprovedUrl(base + "arquivos-lpc/2026/resumo_semanal_lpc_2026-11-09_2026-11-15.xlsx"))
        XCTAssertFalse(AnpEndpoints.isApprovedUrl("http://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/x.xlsx"))
        XCTAssertFalse(AnpEndpoints.isApprovedUrl("https://evil.example/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/x.xlsx"))
        XCTAssertFalse(AnpEndpoints.isApprovedUrl("https://user:pass@www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/x.xlsx"))
        XCTAssertFalse(AnpEndpoints.isApprovedUrl("https://www.gov.br/outro/arquivos-lpc/2026/x.xlsx"))
        XCTAssertFalse(AnpEndpoints.isApprovedUrl(base + "../../../../../outro/x.xlsx"))
        XCTAssertFalse(AnpEndpoints.isApprovedUrl("https://www.gov.br:8443/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/x.xlsx"))
    }

    func testPriceTableFileNamesCoverCanonicalAndLegacyVariants() throws {
        let root = "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/"
        let canonical = try XCTUnwrap(AnpPriceTableUrl.parse(root + "resumo_semanal_lpc_2026-11-09_2026-11-15.xlsx"))
        XCTAssertEqual(canonical.kind, .weeklySummary)
        XCTAssertEqual(canonical.week.id, "2026-11-09_2026-11-15")
        let revision = try XCTUnwrap(AnpPriceTableUrl.parse(root + "revendas_lpc_2026-11-09_2026-11-15-2.xlsx"))
        XCTAssertEqual(revision.kind, .stationDetail)
        let dotted = try XCTUnwrap(AnpPriceTableUrl.parse(root + "revendas_lpc_2026.11.09_2026.11.15.xlsx"))
        XCTAssertEqual(dotted.week.startIso, "2026-11-09")
        let compact = try XCTUnwrap(AnpPriceTableUrl.parse(root + "resumo_semanal_lpc_09112026-a-15112026.xlsx"))
        XCTAssertEqual(compact.week.endIso, "2026-11-15")
        XCTAssertNil(AnpPriceTableUrl.parse(root + "resumo_semanal_lpc_2026-11-01_2026-11-15.xlsx"), "more than seven days")
        XCTAssertTrue(AnpPriceTableUrl.isPriceTableUrl(root + "planilha-sem-padrao.xlsx"))
        XCTAssertFalse(AnpPriceTableUrl.isPriceTableUrl(root + "planilha.pdf"))
        XCTAssertEqual(AnpPriceTableUrl.inferKind(linkText: "Preços médios semanais: Brasil, regiões"), .weeklySummary)
        XCTAssertEqual(AnpPriceTableUrl.inferKind(linkText: "Preços por posto revendedor"), .stationDetail)
        XCTAssertEqual(AnpPriceTableUrl.safeFileName(root + "revendas.xlsx?download=1"), "revendas.xlsx")
        XCTAssertNil(AnpPriceTableUrl.safeFileName(root))
    }

    func testListingGroupsCompleteWeekBlocksNewestFirst() throws {
        let html = """
        <html><body><h3>Levantamento</h3>
        <p><strong>02/11/2026 a 08/11/2026</strong></p>
        <ul>
          <li><a href="arquivos-lpc/2026/resumo_semanal_lpc_2026-11-02_2026-11-08.xlsx">Preços m&eacute;dios semanais</a> (Atualizado em 10/11/2026)</li>
          <li><a href='arquivos-lpc/2026/revendas_lpc_2026-11-02_2026-11-08.xlsx'>Preços por posto revendedor</a></li>
        </ul>
        <p>09/11/2026 a 15/11/2026</p>
        <ul>
          <li><a href="https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/resumo_semanal_lpc_2026-11-09_2026-11-15.xlsx">Médios semanais</a> (Atualizado em 17/11/2026)</li>
          <li><a href="arquivos-lpc/2026/revendas_lpc_2026-11-09_2026-11-15.xlsx">Posto revendedor</a> (Atualizado em 18/11/2026)</li>
        </ul>
        <p>NOTA: Coleta parcial em um estado.</p>
        <p>16/11/2026 a 22/11/2026</p>
        <ul><li><a href="arquivos-lpc/2026/resumo_semanal_lpc_2026-11-16_2026-11-22.xlsx">Médios semanais</a></li></ul>
        </body></html>
        """
        let catalog = AnpListingParser.parseCatalog(html: html)
        XCTAssertEqual(catalog.map(\.week.id), ["2026-11-09_2026-11-15", "2026-11-02_2026-11-08"])
        let newest = try XCTUnwrap(catalog.first)
        XCTAssertEqual(newest.publishedOnIso, "2026-11-18")
        XCTAssertEqual(newest.operationalNote, "Coleta parcial em um estado.")
        XCTAssertTrue(newest.stationUrl.hasPrefix("https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/2026/revendas"))
        XCTAssertTrue(AnpEndpoints.isApprovedUrl(newest.summaryUrl))
        XCTAssertEqual(catalog.last?.publishedOnIso, "2026-11-10")
        XCTAssertNil(catalog.last?.operationalNote)
        XCTAssertTrue(AnpListingParser.parseCatalog(html: "<html><body>sem semanas</body></html>").isEmpty)
    }

    func testBrazilianDatesAndExcelSerialsAreStrict() throws {
        XCTAssertEqual(AnpCalendar.parseBrazilianDay("7/6/2026"), "2026-06-07")
        XCTAssertNil(AnpCalendar.parseBrazilianDay("31/02/2026"))
        XCTAssertNil(AnpCalendar.parseBrazilianDay("07/06/26"))
        XCTAssertNil(AnpSurveyWeekRef(startIso: "2026-11-15", endIso: "2026-11-09"))
        XCTAssertEqual(AnpSurveyWeekRef(id: "2026-11-09_2026-11-15")?.endYear, 2026)
        XCTAssertEqual(AnpCalendar.daysBetween("2026-11-15", "2026-11-23"), 8)

        let fixtures = try JSONSerialization.jsonObject(with: Data(contentsOf: repoFile("contracts/testdata/anp/anp-016.json"))) as? [String: Any]
        let input = try XCTUnwrap(fixtures?["input"] as? [String: Any])
        let expected = try XCTUnwrap(fixtures?["expected_legacy"] as? [String: Any])
        let serial = try XCTUnwrap(input["excel_serial"] as? Int)
        XCTAssertEqual(AnpCalendar.excelSerialToIso(String(serial)), expected["iso_date"] as? String)
        XCTAssertEqual(AnpCalendar.excelSerialToIso("45351"), "2024-02-29")
        XCTAssertEqual(AnpCalendar.excelSerialToIso("46335.75"), "2026-11-09")
        XCTAssertNil(AnpCalendar.excelSerialToIso("abc"))
    }

    /// Replays the shared ANP label and price fixtures (`contracts/testdata/anp`).
    func testAnpLabelAndPriceFixturesReplay() throws {
        for index in 1...15 {
            let id = String(format: "anp-%03ld", index)
            let data = try Data(contentsOf: repoFile("contracts/testdata/anp/\(id).json"))
            let fixture = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any], id)
            let input = try XCTUnwrap(fixture["input"] as? [String: Any], id)
            let label = try XCTUnwrap(input["label"] as? String, id)
            let priceText = try XCTUnwrap(input["price_text"] as? String, id)
            if let expected = fixture["expected_legacy"] as? [String: Any] {
                let product = AnpTextNormalizer.fuelProduct(anpLabel: label)
                XCTAssertEqual(product?.rawValue, expected["product"] as? String, id)
                XCTAssertEqual(product?.unitWire, expected["unit"] as? String, id)
                XCTAssertEqual(try PortableMoney.parse(priceText), (expected["amount_milli_brl"] as? NSNumber)?.int64Value, id)
            } else {
                let refused = AnpTextNormalizer.fuelProduct(anpLabel: label) == nil || PortableMoney.code(of: priceText) != "ok"
                XCTAssertTrue(refused, "\(id) must stay quarantined")
            }
        }
    }

    func testCentavoRoundingMatchesAndroidPriceAmount() {
        XCTAssertEqual(AnpPriceRounding.milli(fromCell: "6.385"), 6_390)
        XCTAssertEqual(AnpPriceRounding.milli(fromCell: "6.3849999999999998"), 6_390)
        XCTAssertEqual(AnpPriceRounding.milli(fromCell: "6.3849"), 6_380)
        XCTAssertEqual(AnpPriceRounding.milli(fromCell: "6.39"), 6_390)
        XCTAssertEqual(AnpPriceRounding.milli(fromCell: "109.9"), 109_900)
        XCTAssertEqual(AnpPriceRounding.milli(fromCell: "5,999"), 6_000)
        XCTAssertNil(AnpPriceRounding.milli(fromCell: "0"))
        XCTAssertNil(AnpPriceRounding.milli(fromCell: "-1.0"))
        XCTAssertNil(AnpPriceRounding.milli(fromCell: ""))
        XCTAssertNil(AnpPriceRounding.milli(fromCell: "abc"))
        XCTAssertEqual(AnpPriceRounding.formatBrl(milli: 6_390), "R$ 6,39")
        XCTAssertEqual(AnpPriceRounding.formatBrl(milli: 1_109_900), "R$ 1.109,90")
    }

    func testMunicipalityStateAndCnpjNormalizersMatchTheCatalog() throws {
        XCTAssertEqual(AnpTextNormalizer.municipality("  São João  d'Aliança "), "SAO JOAO DALIANCA")
        XCTAssertEqual(AnpTextNormalizer.municipality("Embu-Guaçu"), "EMBU GUACU")
        XCTAssertEqual(AnpTextNormalizer.state(anpLabel: "SÃO PAULO"), .SP)
        XCTAssertEqual(AnpTextNormalizer.state(anpLabel: "mt"), .MT)
        XCTAssertNil(AnpTextNormalizer.state(anpLabel: "Atlântida"))
        XCTAssertEqual(AnpTextNormalizer.cnpjDigits("04.218.406/0001-04"), "04218406000104")
        XCTAssertEqual(AnpTextNormalizer.cnpjDigits("4218406000104"), "04218406000104")
        XCTAssertEqual(AnpTextNormalizer.cnpjDigits("4.218406000104E12"), "04218406000104")

        let catalogData = try Data(contentsOf: repoFile("data/src/main/assets/ibge_municipalities.json"))
        let rows = try XCTUnwrap(try JSONSerialization.jsonObject(with: catalogData) as? [[String: String]])
        for row in rows.prefix(800) {
            XCTAssertEqual(AnpTextNormalizer.municipality(row["municipality"] ?? ""), row["normalizedName"], row["ibgeCode"] ?? "")
        }
    }
}
