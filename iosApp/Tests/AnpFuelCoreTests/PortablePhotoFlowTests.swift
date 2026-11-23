import XCTest
@testable import AnpFuelCore

final class PortablePhotoFlowTests: XCTestCase {
    private final class StubDecoder: PhotoDecoder {
        var dims: PhotoDims? = PhotoDims(width: 4000, height: 3000)
        func probeDims(bytes: Data) -> PhotoDims? { dims }
    }

    private final class StubEncoder: PhotoEncoder {
        var encodedSize = 140_000
        func encode(source: Data, request: PhotoEncodeRequest) -> Data? {
            return Data(repeating: 0xAB, count: encodedSize)
        }
    }

    private final class StubCache: PhotoCache {
        var map: [String: (Data, Int64)] = [:]
        func put(id: String, bytes: Data, capturedAtMillis: Int64) { map[id] = (bytes, capturedAtMillis) }
        func get(id: String) -> Data? { map[id]?.0 }
        func delete(id: String) { map.removeValue(forKey: id) }
        func sweepExpired(nowMillis: Int64) -> Int {
            let before = map.count
            map = map.filter { !PortablePhoto.isTransientExpired(capturedAtMillis: $0.value.1, nowMillis: nowMillis) }
            return before - map.count
        }
    }

    private final class StubClock: PhotoClock {
        var now: Int64 = 1_700_000_000_000
        func nowMillis() -> Int64 { now }
    }

    private final class StubIds: PhotoIdSource {
        func nextId() -> String { "photo-1" }
    }

    func testPhotoFlowPrepareAnd24hExpirySweep() {
        let cache = StubCache()
        let clock = StubClock()
        let encoder = StubEncoder()
        let flow = PhotoFlow(ports: PhotoPorts(decoder: StubDecoder(), encoder: encoder, cache: cache, clock: clock, ids: StubIds()))

        let result = flow.prepare(intentBytes: Data([1, 2, 3]), mime: "image/heic")
        XCTAssertEqual(result, .ready(id: "photo-1", bytes: 140_000, attempts: 1))
        XCTAssertNotNil(cache.get(id: "photo-1"))

        clock.now += PortablePhoto.transientTTLMillis
        XCTAssertEqual(flow.sweepExpired(), 1)
        XCTAssertNil(cache.get(id: "photo-1"))
    }

    func testUnsupportedMimeAndOversizeRefuseBeforePublish() {
        let cache = StubCache()
        let clock = StubClock()
        let encoder = StubEncoder()
        encoder.encodedSize = Int(PortablePhoto.capBytes) + 1
        let flow = PhotoFlow(ports: PhotoPorts(decoder: StubDecoder(), encoder: encoder, cache: cache, clock: clock, ids: StubIds()))

        XCTAssertEqual(flow.prepare(intentBytes: Data([1]), mime: "image/gif"), .refused(code: PortablePhoto.unsupportedFormat))
        XCTAssertEqual(flow.prepare(intentBytes: Data([1]), mime: "image/jpeg"), .refused(code: PortablePhoto.overBudget))
        XCTAssertTrue(cache.map.isEmpty)
    }
}
