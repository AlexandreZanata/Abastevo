import Foundation
import XCTest
@testable import AnpFuelCore

final class OfficialQualityTests: XCTestCase {
    private let stationId = "00000000-0000-4000-8000-000000000001"
    private let revisionId = "00000000-0000-4000-8000-000000000002"
    private let assayId = "00000000-0000-4000-8000-000000000003"

    private var selection: QualitySelection {
        QualitySelection(stationId: stationId, fuel: .ethanol, revisionId: revisionId)
    }

    private func lease(source: String = "pmqc", stream: String = "pmqc-assay-v1") -> String {
        """
        {"source":"\(source)","stream":"\(stream)","edition":"2026-09","revision_id":"\(revisionId)",
         "published_at":"2026-10-01T12:00:00Z","available":true,"stale":false,
         "revision_status":"published","provenance_state":"VERIFIED","is_current_stream":true}
        """
    }

    private var summaryBody: String {
        """
        {"station_id":"\(stationId)","lease":\(lease()),
         "fuel_cells":[{"fuel_product":"ETHANOL","conformity":"CONFORMING","count":6,"last_observed":"2026-09-04"}],
         "sample_dimensions":{"available":true,"items":[{"fuel_product":"ETHANOL","samples":3,"visits":2,
          "conforming":1,"nonconforming":1,"unresolved":1,"oldest_observed_on":"2026-09-01",
          "newest_observed_on":"2026-09-04","score_state":"UNSCORED","score":null}]}}
        """
    }

    func testPMQCSeparatesAssaysFromSampleDimensionsAndRejectsScores() throws {
        let result = try QualityWire.summary(Data(summaryBody.utf8), selection: selection, stale: false)
        XCTAssertEqual(result.cells.map(\.count), [6])
        XCTAssertEqual(result.sampleDimensions?.map(\.samples), [3])
        XCTAssertEqual(result.sampleDimensions?.map(\.visits), [2])
        XCTAssertFalse(result.stale)
        let cached = try QualityWire.summary(Data(summaryBody.utf8), selection: selection, stale: true)
        XCTAssertTrue(cached.stale)

        for (field, invalid) in [
            (#""score":null"#, #""score":0"#),
            (#""unresolved":1"#, #""unresolved":0"#),
            (#""fuel_product":"ETHANOL""#, #""fuel_product":"DIESEL_S10""#),
            (#""provenance_state":"VERIFIED""#, #""provenance_state":"PENDING""#),
            (revisionId, assayId),
        ] {
            let wrong = summaryBody.replacingOccurrences(of: field, with: invalid)
            XCTAssertThrowsError(try QualityWire.summary(Data(wrong.utf8), selection: selection, stale: false), invalid)
        }

        let unavailable = """
        {"station_id":"\(stationId)","lease":{"source":"pmqc","stream":"pmqc-assay-v1","edition":"",
        "available":false,"stale":false,"revision_status":"unavailable"},"fuel_cells":[],
        "sample_dimensions":{"available":false,"items":[]}}
        """
        let lack = try QualityWire.summary(Data(unavailable.utf8),
                                          selection: QualitySelection(stationId: stationId, fuel: .ethanol), stale: false)
        XCTAssertFalse(lack.lease.available)
        XCTAssertNil(lack.sampleDimensions)
    }

    func testPMQCCursorAndSampleProvenanceAreBoundToRevisionAndFuel() throws {
        let assay = """
        {"id":"\(assayId)","sample_ref":"S-1","assay_ref":"Appearance","observed_on":"2026-09-04",
        "fuel_product":"ETHANOL","result_text":"Synthetic","unit":"","conformity":"CONFORMING"}
        """
        let page = Data("""
        {"station_id":"\(stationId)","lease":\(lease()),"items":[\(assay)],"next_cursor":"next"}
        """.utf8)
        let first = try QualityWire.assays(page, selection: selection, cursor: nil, stale: false)
        XCTAssertEqual(first.items.map(\.sampleRef), ["S-1"])
        XCTAssertEqual(first.nextCursor, "next")
        XCTAssertThrowsError(try QualityWire.assays(page, selection: selection, cursor: "next", stale: false))
        let sample = Data("""
        {"sample_ref":"S-1","lease":\(lease()),"assays":[\(assay)]}
        """.utf8)
        let result = try QualityWire.sample(sample, ref: "S-1", revision: revisionId, fuel: .ethanol, stale: false)
        XCTAssertEqual(result.assays.map(\.id), [assayId])
        XCTAssertThrowsError(try QualityWire.sample(sample, ref: "S-2", revision: revisionId, fuel: .ethanol, stale: false))
        XCTAssertThrowsError(try QualityWire.sample(sample, ref: "S-1", revision: assayId, fuel: .ethanol, stale: false))

        let digest = String(repeating: "a", count: 64)
        let url = "https://www.gov.br/anp/sample.zip"
        let provenance = """
        {"lease":\(lease()),"resources":[{"url":"\(url)","sha256":"\(digest)"}]}
        """
        let source = try QualityWire.provenance(Data(provenance.utf8), revision: revisionId, stale: false)
        XCTAssertEqual(source.resources.map(\.sha256), [digest])
        let foreignSource = provenance.replacingOccurrences(of: "www.gov.br", with: "example.invalid")
        XCTAssertThrowsError(try QualityWire.provenance(Data(foreignSource.utf8), revision: revisionId, stale: false))
    }

    func testVerbatimEnforcementRecordAndPrivacyRefusal() throws {
        #if SWIFT_PACKAGE
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { url.deleteLastPathComponent() }
        url = url.appendingPathComponent("contracts/testdata/quality/enforcement-source-record-valid.json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "enforcement-source-record-valid", withExtension: "json"))
        #endif
        let data = try Data(contentsOf: url)
        let record = try OccurrenceWire.record(data)
        XCTAssertEqual(record.documentDay, "2026-10-04")
        XCTAssertEqual(record.dateBasis, "document_day")
        XCTAssertEqual(record.documentNumber, "712508")
        XCTAssertTrue(record.isCompanyShaped)

        let payload = try XCTUnwrap(String(data: data, encoding: .utf8))
        let injected = payload.replacingOccurrences(of: "\"native_key\":", with: "\"person_name\":\"synthetic\",\"native_key\":")
        XCTAssertThrowsError(try OccurrenceWire.record(Data(injected.utf8)))
        let atRevision = lease(source: "enforcement", stream: "enforcement-v1")
        let page = Data("{\"lease\":\(atRevision),\"items\":[\(payload)],\"next_cursor\":\"next\"}".utf8)
        XCTAssertEqual(try OccurrenceWire.actions(page, revision: revisionId, cursor: nil, stale: false).items, [record])
        XCTAssertThrowsError(try OccurrenceWire.actions(page, revision: assayId, cursor: nil, stale: false))
        XCTAssertThrowsError(try OccurrenceWire.actions(page, revision: revisionId, cursor: "next", stale: false))
        XCTAssertThrowsError(try OccurrenceFilter(fromDay: "2026-10-10", toDay: "2026-10-01").query(stationList: false))
        XCTAssertThrowsError(try OccurrenceFilter(subject: .company).query(stationList: true))
    }

    func testCachedOfflineSummarySurvivesRestartAndPurgeInvalidatesFlight() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let cache = QualityPublicCache(directory: directory)
        let scope = QualityReadScope(origin: ApiEnvironment.staging.origin, accountId: "account-a")
        let secondOwner = QualityReadScope(origin: scope.origin, accountId: "account-b")
        let otherOrigin = QualityReadScope(origin: ApiEnvironment.preview.origin, accountId: scope.accountId)
        var request = try XCTUnwrap(URLComponents(string: scope.origin))
        request.path = "/v1/quality/summary"
        request.queryItems = try selection.query()
        let url = try XCTUnwrap(request.url)
        let body = Data(summaryBody.utf8)
        let ticket = cache.ticket()
        cache.save(scope: scope, url: url, body: body, etag: "\"fixture\"", ticket: ticket)
        XCTAssertNil(cache.load(scope: secondOwner, url: url, ticket: ticket))
        XCTAssertNil(cache.load(scope: otherOrigin, url: url, ticket: ticket))

        let reopened = QualityPublicCache(directory: directory)
        let reader = QualityPublicReader(client: AbastevoHttpClient(environment: .staging),
                                         cache: reopened, scope: scope, offline: true)
        let cached = try await reader.summary(selection)
        XCTAssertTrue(cached.stale)
        XCTAssertEqual(cached.cells.map(\.count), [6])
        let reopenedTicket = reopened.ticket()
        reopened.purge()
        reopened.save(scope: scope, url: url, body: body, etag: nil, ticket: reopenedTicket)
        XCTAssertNil(reopened.load(scope: scope, url: url, ticket: reopened.ticket()))
        do {
            _ = try await reader.summary(selection)
            XCTFail("Expected unavailable offline after purge")
        } catch {
            XCTAssertEqual(error as? QualityReadError, .unavailable)
        }
    }
}
