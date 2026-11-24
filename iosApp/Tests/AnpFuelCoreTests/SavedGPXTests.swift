import Foundation
import XCTest
@testable import AnpFuelCore

final class SavedGPXTests: XCTestCase {
    private let placeId = "11111111-1111-4111-8111-111111111111"
    private let routeId = "22222222-2222-4222-8222-222222222222"
    private let stationId = "33333333-3333-4333-8333-333333333333"
    private let trackId = "44444444-4444-4444-8444-444444444444"

    func testRecognizesEntitiesCDATACommentsAndABoundedUTF8Declaration() throws {
        let xml = #"""
        <?xml version="1.0" encoding="UTF-8"?><!-- c --><a x="1 &amp; 2"><b>a&lt;b&#x41;&#66;</b><c><![CDATA[<raw & text>]]></c><d/></a>
        """#
        let root = try StrictSavedXML.parse(Data([0xef, 0xbb, 0xbf]) + Data(xml.utf8))
        XCTAssertEqual(root.attributes["x"], "1 & 2")
        XCTAssertEqual(root.text("b"), "a<bAB")
        XCTAssertEqual(root.text("c"), "<raw & text>")
        XCTAssertNotNil(root.first("d"))
    }

    func testForbidsXmlDeclarationsEntitiesProcessingInstructionsAndForeignEncodings() {
        assertXmlError(#"<!DOCTYPE root><root/>"#, .xmlDoctypeRefused)
        assertXmlError(#"<root><!ENTITY forbidden "content"></root>"#, .xmlDoctypeRefused)
        assertXmlError(#"<root>&unrecognized;</root>"#, .xmlEntityRefused)
        assertXmlError(#"<root>&amp</root>"#, .xmlEntityRefused)
        assertXmlError(#"<root>&#xD800;</root>"#, .xmlEntityRefused)
        assertXmlError(#"<root>&#0;</root>"#, .xmlEntityRefused)
        assertXmlError(#"<root value="&external;"/>"#, .xmlEntityRefused)
        assertXmlError(#"<?xml version="1.0"?><?stylesheet href="style.xsl"?><root/>"#, .xmlPIRefused)
        assertXmlError(#"<root><?nonleading forbidden?></root>"#, .xmlPIRefused)
        assertXmlError(#"<?xml version="1.0" encoding="ISO-8859-1"?><root/>"#, .xmlEncodingUnsupported)
        for data in [Data([0xff, 0xfe, 0x3c, 0x00]), Data([0x3c, 0x61, 0x3e, 0xc3, 0x28])] {
            XCTAssertThrowsError(try StrictSavedXML.parse(data)) {
                XCTAssertEqual($0 as? SavedMapError, .xmlEncodingUnsupported)
            }
        }
    }

    func testStructureAndParserLimits() throws {
        for malformed in ["", "text", "<a>", "<a></b>", "<a><b></a></b>", "<a/><b/>", "<a x=1/>",
                          "<a x='1' x='2'/>", "<a/>tail", "<1a/>", "<a x='<'/>", "<a><!-- unterminated</a>",
                          "<a><![CDATA[x</a>", "<a x='1'y='2'/>"] {
            assertXmlError(malformed, .xmlMalformed)
        }
        let fullDepth = String(repeating: "<a>", count: 64) + String(repeating: "</a>", count: 64)
        XCTAssertNoThrow(try StrictSavedXML.parse(Data(fullDepth.utf8)))
        assertXmlError("<a>" + fullDepth + "</a>", .xmlTooDeep)
        assertXmlError("<r>" + String(repeating: "<e/>", count: SavedMapBounds.xmlElements) + "</r>", .xmlTooLarge)
        assertXmlError("<a " + (1...33).map { "k\($0)='v'" }.joined(separator: " ") + "/>", .xmlMalformed)
        assertXmlError("<a k='" + String(repeating: "x", count: 4_097) + "'/>", .xmlMalformed)
        XCTAssertThrowsError(try StrictSavedXML.parse(Data(repeating: 0, count: SavedMapBounds.fileBytes + 1))) {
            XCTAssertEqual($0 as? SavedMapError, .fileTooLarge)
        }
    }

    func testCoordinatesAreExactMicrodegreesAndNeverClamped() throws {
        let point = try SavedGeoPoint.parse(latitude: "-25.4295", longitude: "-49.2719")
        XCTAssertEqual(point.latitudeMicro, -25_429_500)
        XCTAssertEqual(point.longitudeMicro, -49_271_900)
        XCTAssertEqual(try SavedGeoPoint.parse(latitude: "-25.4295675", longitude: "0").latitudeMicro, -25_429_568)
        XCTAssertEqual(try SavedGeoPoint.parse(latitude: "90", longitude: "-180.000000000").longitudeMicro, -180_000_000)
        XCTAssertEqual(try SavedGeoPoint.parse(latitude: "0.0000004", longitude: "0").latitudeMicro, 0)
        XCTAssertEqual(try SavedGeoPoint.parse(latitude: "0.0000005", longitude: "0").latitudeMicro, 1)
        for bad in ["", "abc", "1e3", "NaN", "Infinity", "-", "+", "1,5", "91", "-90.0000005",
                    "1.0000000000000000001", " 12 3", "0x10"] {
            XCTAssertThrowsError(try SavedGeoPoint.parse(latitude: bad, longitude: "0"), bad)
        }
        XCTAssertThrowsError(try SavedGeoPoint.parse(latitude: "0", longitude: "180.0000005"))
        XCTAssertThrowsError(try SavedGeoPoint(latitude: .nan, longitude: 0))
        XCTAssertThrowsError(try SavedGeoPoint(latitude: 0, longitude: .infinity))
    }

    func testPlaceTripAndScopeValidation() throws {
        let point = try SavedGeoPoint(latitudeMicro: 0, longitudeMicro: 0)
        let otherPoint = try SavedGeoPoint(latitudeMicro: 1, longitudeMicro: 1)
        let waypoint = try SavedMapTripPoint(point: point)
        let other = try SavedMapTripPoint(point: otherPoint)
        XCTAssertNil(SavedMapScope(owner: "", origin: "https://local.example"))
        XCTAssertNil(SavedMapScope(owner: "guest", origin: ""))
        XCTAssertThrowsError(try SavedMapPlace(
            id: placeId, name: String(repeating: "x", count: 121), point: point, stationId: nil,
            source: .manual, sourceId: nil, note: nil, createdAtMillis: 0
        ))
        XCTAssertThrowsError(try SavedMapPlace(
            id: placeId, name: "Place", point: point, stationId: "not-a-station-id",
            source: .anp, sourceId: nil, note: nil, createdAtMillis: 0
        ))
        XCTAssertNoThrow(try SavedMapTrip(
            id: routeId, kind: .routePlan, name: "Plan", points: [waypoint, other], avoid: ["TOLL"], createdAtMillis: 0
        ))
        XCTAssertThrowsError(try SavedMapTrip(
            id: routeId, kind: .routePlan, name: "Plan", points: [waypoint, waypoint], createdAtMillis: 0
        ))
        XCTAssertThrowsError(try SavedMapTrip(
            id: trackId, kind: .recordedTrack, name: "Track", points: [waypoint], avoid: ["TOLL"], createdAtMillis: 0
        ))
        XCTAssertThrowsError(try SavedMapTripPoint(point: point, timeMillis: -1))
    }

    func testGPXRoundTripKeepsIDsSourcesNotesCoordinatesAndAttribution() throws {
        let place = try SavedMapPlace(
            id: placeId, name: "Posto & Cia <A>", point: .parse(latitude: "-25.4295", longitude: "-49.2719"),
            stationId: stationId, source: .anp, sourceId: "ANP-123", note: "Aberto 24h", createdAtMillis: 1000
        )
        let origin = try SavedMapTripPoint(point: place.point)
        let stop = try SavedMapTripPoint(point: .parse(latitude: "-25.43", longitude: "-49.27"))
        let plan = try SavedMapTrip(
            id: routeId, kind: .routePlan, name: "Casa → Trabalho", points: [origin, stop], avoid: ["TOLL", "FERRY"],
            sourceId: "plan-1", note: "nota", createdAtMillis: 1000
        )
        let timedPoint = try SavedMapTripPoint(point: place.point, elevationCm: -50, timeMillis: 1_700_000_000_421)
        let track = try SavedMapTrip(id: trackId, kind: .recordedTrack, name: "Ride", points: [timedPoint],
                                    createdAtMillis: 1000)
        let data = try SavedGPX.write(places: [place], trips: [plan, track])
        let exported = String(decoding: data, as: UTF8.self)
        XCTAssertTrue(exported.contains(SavedGPX.licenseNotice))
        XCTAssertTrue(exported.contains(#"lat="-25.429500""#))
        XCTAssertTrue(exported.contains("Posto &amp; Cia &lt;A&gt;"))
        let restored = try parse(data)
        XCTAssertEqual(restored.places, [place])
        XCTAssertEqual(restored.trips.sorted { $0.id < $1.id }, [plan, track])
        XCTAssertEqual(restored.skippedUnsupported, 0)
        XCTAssertEqual(restored.contentSha256, try parse(Data(data)).contentSha256)
    }

    func testForeignGPXPreservesSegmentsAndValidOptionalFieldsAndTruncatesText() throws {
        let input = """
        <?xml version="1.0"?><gpx version="1.0" creator="Other">
          <wpt lat="1" lon="2"><name>\(String(repeating: "n", count: 200))</name><cmt>\(String(repeating: "d", count: 900))</cmt></wpt>
          <trk><name>Morning</name><trkseg><trkpt lat="1" lon="2"><ele>900.5</ele><time>2026-10-01T10:00:00Z</time></trkpt>
          <trkpt lat="3" lon="4"><ele>bad</ele><time>garbage</time></trkpt></trkseg><trkseg><trkpt lat="5" lon="6"/></trkseg></trk>
          <rte><name>Plan</name><rtept lat="1" lon="1"/><rtept lat="2" lon="2"/></rte><unknownThing/>
        </gpx>
        """
        let content = try parse(Data(input.utf8))
        let place = try XCTUnwrap(content.places.first)
        XCTAssertEqual(place.source, .imported)
        XCTAssertEqual(place.name.count, 120)
        XCTAssertEqual(place.note?.count, 500)
        XCTAssertEqual(content.truncatedFields, 2)
        XCTAssertEqual(content.skippedUnsupported, 1)
        let morning = try XCTUnwrap(content.trips.first { $0.name == "Morning" })
        XCTAssertEqual(morning.points.count, 3)
        XCTAssertEqual(morning.points[0].elevationCm, 90_050)
        XCTAssertEqual(morning.points[0].timeMillis, 1_790_848_800_000)
        XCTAssertNil(morning.points[1].elevationCm)
        XCTAssertNil(morning.points[1].timeMillis)
        XCTAssertEqual(content.trips.first { $0.name == "Plan" }?.kind, .importedTrack)
    }

    func testMalformedFileDuplicateIDsAndRecordLimitsAreRejectedAsAWhole() throws {
        let valid = #"<wpt lat="1" lon="1"/>"#
        for bad in [#"<wpt lat="abc" lon="1"/>"#, #"<wpt lat="1"/>"#, #"<wpt lat="91" lon="0"/>"#,
                    #"<wpt lat="1e2" lon="0"/>"#, #"<wpt lat="0" lon="181"/>"#] {
            XCTAssertThrowsError(try parse(Data("<gpx>\(valid)\(bad)</gpx>".utf8)))
        }
        let identified = #"<wpt lat="1" lon="1"><extensions><abv:id>\#(placeId)</abv:id></extensions></wpt>"#
        assertGPXError("<gpx>\(identified)\(identified)</gpx>", .duplicateRecordID)
        assertGPXError("<kml/>", .unsupportedFormat)
        assertGPXError("", .fileEmpty)
        assertGPXError("<gpx/>", .noRecords)
        assertGPXError("<gpx>" + String(repeating: valid, count: 5_001) + "</gpx>", .tooManyRecords)
        let point = #"<trkpt lat="1" lon="1"/>"#
        let exact = "<gpx><trk><trkseg>" + String(repeating: point, count: 10_000) + "</trkseg></trk></gpx>"
        XCTAssertEqual(try parse(Data(exact.utf8)).trips.first?.points.count, 10_000)
        let oversized = "<gpx><trk><trkseg>" + String(repeating: point, count: 10_001) + "</trkseg></trk></gpx>"
        assertGPXError(oversized, .tooManyPoints)
    }

    private func parse(_ data: Data) throws -> SavedGPXContent {
        try SavedGPX.parse(data, nowMillis: 1000, unnamedPlace: "Imported place", unnamedTrip: "Imported trip")
    }

    private func assertGPXError(_ value: String, _ error: SavedMapError, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertThrowsError(try parse(Data(value.utf8)), file: file, line: line) {
            XCTAssertEqual($0 as? SavedMapError, error, file: file, line: line)
        }
    }

    private func assertXmlError(_ value: String, _ error: SavedMapError, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertThrowsError(try StrictSavedXML.parse(Data(value.utf8)), file: file, line: line) {
            XCTAssertEqual($0 as? SavedMapError, error, file: file, line: line)
        }
    }
}
