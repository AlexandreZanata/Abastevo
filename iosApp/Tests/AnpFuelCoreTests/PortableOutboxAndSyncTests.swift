import XCTest
@testable import AnpFuelCore

final class PortableOutboxAndSyncTests: XCTestCase {
    func testOutboxEnqueueReplacesSameCommandWithBumpedRevision() throws {
        var queue = try PortableOutbox.enqueue(commands: [], commandId: "cmd-1", kind: "price", payload: "{v1}")
        XCTAssertEqual(queue.count, 1)
        XCTAssertEqual(queue[0].revision, 1)

        queue = try PortableOutbox.enqueue(commands: queue, commandId: "cmd-1", kind: "price", payload: "{v2}")
        XCTAssertEqual(queue.count, 1)
        XCTAssertEqual(queue[0].revision, 2)
        XCTAssertEqual(queue[0].payload, "{v2}")

        let staleAck = PortableOutbox.markAcked(commands: queue, commandId: "cmd-1", revision: 1)
        XCTAssertEqual(staleAck.count, 1)

        let freshAck = PortableOutbox.markAcked(commands: queue, commandId: "cmd-1", revision: 2)
        XCTAssertEqual(freshAck.count, 0)
    }

    func testOutboxAttemptBudgetParksAt25Attempts() {
        var cmd = OutboxCommand(commandId: "cmd-park", kind: "price", payload: "{}")
        for step in 1...30 {
            cmd = PortableOutbox.markFailed(command: cmd, tickMillis: Int64(step * 1000))
        }
        XCTAssertEqual(cmd.attempts, PortableOutbox.maxDispatchAttempts)
        XCTAssertEqual(cmd.nextEligibleTick, Int64.max)
        XCTAssertNil(PortableOutbox.selectNext(commands: [cmd], tickMillis: 999_999_999_999, isCancelled: false))
    }

    func testRecordRoundTripPreservesFields() throws {
        let cmd = OutboxCommand(commandId: "c-42", kind: "consent", payload: "{\"ok\":true}", revision: 3, state: .failed, attempts: 2, nextEligibleTick: 5000, nonce: "n-1")
        let rec = PortableOutbox.toRecord(cmd)
        let decoded = try PortableOutbox.fromRecord(rec)
        XCTAssertEqual(decoded, cmd)
    }

    func testSyncStateResolution() {
        let cached = CachedValue(payload: "cached", revision: 4, cachedTick: 1000)
        XCTAssertEqual(
            PortableSyncState.resolve(remotePayload: "fresh", remoteRevision: 5, cache: cached, isOnline: true, tickMillis: 2000, staleAfterMillis: 500).origin,
            .remoteFresh
        )
        XCTAssertEqual(
            PortableSyncState.resolve(remotePayload: "fresh", remoteRevision: 5, cache: cached, isOnline: false, tickMillis: 1200, staleAfterMillis: 500).origin,
            .cache
        )
        XCTAssertEqual(
            PortableSyncState.resolve(remotePayload: "fresh", remoteRevision: 5, cache: cached, isOnline: false, tickMillis: 2000, staleAfterMillis: 500).origin,
            .cacheStale
        )
        XCTAssertEqual(
            PortableSyncState.resolve(remotePayload: nil, remoteRevision: 0, cache: nil, isOnline: false, tickMillis: 2000, staleAfterMillis: 500).origin,
            .empty
        )
    }
}
