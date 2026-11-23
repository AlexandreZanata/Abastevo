import Foundation
import XCTest
@testable import AnpFuelCore

/// Records gateway calls and answers with scripted outcomes.
final class FakeCommunityGateway: CommunityWriteGateway, @unchecked Sendable {
    private let lock = NSLock()
    private var calls: [String] = []
    private var submitted: [CommunityOutboxItem] = []
    var evidence: Result<EvidenceProgress, Error> = .success(EvidenceProgress(uploadSessionId: "u-1", evidenceId: "e0000000-0000-4000-8000-000000000002"))
    var receipt: Result<CommunityWriteReceipt, Error> = .success(CommunityWriteReceipt(id: "r-1", state: "RECEIVED"))

    var callLog: [String] {
        lock.withLock { calls }
    }

    var submittedItems: [CommunityOutboxItem] {
        lock.withLock { submitted }
    }

    private func record(_ name: String, _ item: CommunityOutboxItem) {
        lock.withLock {
            calls.append(name)
            submitted.append(item)
        }
    }

    func uploadEvidence(_ item: CommunityOutboxItem) async throws -> EvidenceProgress {
        record("upload", item)
        return try evidence.get()
    }

    func submitObservation(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        record("observation", item)
        return try receipt.get()
    }

    func submitStationComment(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        record("station-comment", item)
        return try receipt.get()
    }

    func submitForumTopic(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        record("forum-topic", item)
        return try receipt.get()
    }

    func submitForumComment(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        record("forum-comment", item)
        return try receipt.get()
    }
}

final class CommunityOutboxTests: XCTestCase {
    private let origin = "https://abastevo.com.br"
    private let scope = "anonymous:fp:abc"
    private let now: Int64 = 1_764_000_000_000

    private func priceItem(id: String = "c0000000-0000-4000-8000-000000000001", photo: CommunityPhotoEvidence? = nil, createdAt: Int64 = 0) -> CommunityOutboxItem {
        CommunityOutboxItem(
            id: id,
            kind: .priceObservation,
            ownerScope: scope,
            origin: origin,
            createdAtMillis: createdAt,
            price: CommunityPriceDraft(
                stationId: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
                stationName: "Posto",
                product: .ethanol,
                amountMilli: 3890,
                condition: .standard,
                capturedAtMillis: now - 60_000,
                photo: photo
            )
        )
    }

    private func context(owner: String? = nil, hasAccount: Bool = false) -> CommunityDispatchContext {
        CommunityDispatchContext(nowMillis: now, ownerScope: owner ?? scope, origin: origin, hasAccount: hasAccount)
    }

    private func photo(evidence: String? = nil, session: String? = nil, expiresAt: Int64? = nil) -> CommunityPhotoEvidence {
        CommunityPhotoEvidence(
            photoId: "photo-1",
            captureId: "e0000000-0000-4000-8000-000000000003",
            capturedAtMillis: now - 60_000,
            captureExpiresAtMillis: expiresAt ?? now + 3_600_000,
            uploadSessionId: session,
            evidenceId: evidence
        )
    }

    func testEnqueueKeepsNewestFirstAndDispatchesTheOldestEligible() {
        let first = priceItem(id: "a", createdAt: 1)
        let second = priceItem(id: "b", createdAt: 2)
        var items = CommunityOutbox.enqueue(first, into: [])
        items = CommunityOutbox.enqueue(second, into: items)
        XCTAssertEqual(items.map(\.id), ["b", "a"])
        XCTAssertEqual(CommunityOutbox.nextEligible(items, nowMillis: now)?.id, "a")

        items[1] = CommunityOutbox.retryLater(items[1], code: "community.offline", nowMillis: now)
        XCTAssertEqual(CommunityOutbox.nextEligible(items, nowMillis: now)?.id, "b")
        XCTAssertEqual(CommunityOutbox.nextWakeMillis(items, nowMillis: now), items[1].nextEligibleMillis)
        XCTAssertEqual(CommunityOutbox.retryNow(items, id: "a").last?.nextEligibleMillis, 0)
    }

    func testBackoffHonoursRetryAfterAndParksAfterTheAttemptCap() {
        var item = priceItem()
        item = CommunityOutbox.retryLater(item, code: "community.quota-exceeded", nowMillis: now, notBeforeMillis: now + 600_000)
        XCTAssertEqual(item.status, .pending)
        XCTAssertEqual(item.attempts, 1)
        XCTAssertEqual(item.nextEligibleMillis, now + 600_000)

        for step in 0..<40 {
            item = CommunityOutbox.retryLater(item, code: "community.offline", nowMillis: now + Int64(step))
        }
        XCTAssertEqual(item.status, .expired)
        XCTAssertEqual(item.attempts, PortableOutbox.maxDispatchAttempts)
        XCTAssertEqual(item.lastCode, CommunityOutbox.attemptsExhaustedCode)
        XCTAssertNil(CommunityOutbox.nextEligible([item], nowMillis: Int64.max - 1))
    }

    func testClassificationSeparatesRetriesRefusalsAndSignIn() {
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.offline, nowMillis: now), .retry(code: "community.offline", notBeforeMillis: nil))
        XCTAssertEqual(
            CommunityOutbox.classify(SignedTransportError.refused(status: 429, code: "community.quota-exceeded", retryAfterSeconds: 30), nowMillis: now),
            .retry(code: "community.quota-exceeded", notBeforeMillis: now + 30_000)
        )
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.refused(status: 503, code: nil, retryAfterSeconds: nil), nowMillis: now), .retry(code: "http-503", notBeforeMillis: nil))
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.refused(status: 401, code: "community.auth-required", retryAfterSeconds: nil), nowMillis: now), .retry(code: "community.auth-required", notBeforeMillis: nil))
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.refused(status: 400, code: "community.invalid", retryAfterSeconds: nil), nowMillis: now), .refused(code: "community.invalid"))
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.refused(status: 403, code: "community.account-blocked", retryAfterSeconds: nil), nowMillis: now), .refused(code: "community.account-blocked"))
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.refused(status: 409, code: nil, retryAfterSeconds: nil), nowMillis: now), .refused(code: "http-409"))
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.refused(status: 404, code: "forum.disabled", retryAfterSeconds: nil), nowMillis: now), .retry(code: "forum.disabled", notBeforeMillis: nil))
        XCTAssertEqual(CommunityOutbox.classify(CommunityWriteError.signInRequired, nowMillis: now), .needsSignIn)
        XCTAssertEqual(CommunityOutbox.classify(SignedTransportError.sessionRequired, nowMillis: now), .needsSignIn)
        XCTAssertEqual(CommunityOutbox.classify(CommunityWriteError.photoExpired, nowMillis: now), .expired(code: "contribution.photo-expired"))
        XCTAssertEqual(CommunityOutbox.classify(CommunityWriteError.invalid("feedback.text-invalid"), nowMillis: now), .refused(code: "feedback.text-invalid"))
        XCTAssertEqual(CommunityOutbox.classify(URLError(.timedOut), nowMillis: now), .retry(code: "community.unavailable", notBeforeMillis: nil))
    }

    func testSignInResumesDraftsUnderTheNewOwner() {
        let draft = CommunityOutbox.waitForSignIn(CommunityOutboxItem(id: "t", kind: .forumTopic, ownerScope: scope, origin: origin, createdAtMillis: 0))
        XCTAssertEqual(draft.status, .draft)
        let foreign = CommunityOutbox.waitForSignIn(CommunityOutboxItem(id: "f", kind: .forumTopic, ownerScope: scope, origin: "https://other.example", createdAtMillis: 0))
        let resumed = CommunityOutbox.resumeAfterSignIn([draft, foreign], ownerScope: "account:acc:fp:abc", origin: origin)
        XCTAssertEqual(resumed[0].status, .pending)
        XCTAssertEqual(resumed[0].ownerScope, "account:acc:fp:abc")
        XCTAssertNil(resumed[0].lastCode)
        XCTAssertEqual(resumed[1].status, .draft, "an item from another origin never moves")
    }

    func testDispatcherAcceptsAPriceWithItsReceipt() async {
        let gateway = FakeCommunityGateway()
        let result = await CommunityOutboxDispatcher.run(priceItem(), context: context(), gateway: gateway, clock: { 0 })
        XCTAssertEqual(result.status, .accepted)
        XCTAssertEqual(result.receiptId, "r-1")
        XCTAssertEqual(result.serverState, "RECEIVED")
        XCTAssertEqual(gateway.callLog, ["observation"])
    }

    func testDispatcherRefusesAnItemFromAnotherOwnerOrOriginWithoutNetwork() async {
        let gateway = FakeCommunityGateway()
        let owner = await CommunityOutboxDispatcher.run(priceItem(), context: context(owner: "account:other:fp:abc"), gateway: gateway, clock: { 0 })
        XCTAssertEqual(owner.status, .refused)
        XCTAssertEqual(owner.lastCode, CommunityOutbox.ownerChangedCode)
        let wrongOrigin = await CommunityOutboxDispatcher.run(
            priceItem(),
            context: CommunityDispatchContext(nowMillis: now, ownerScope: scope, origin: "https://api.anpfuel.example.invalid", hasAccount: false),
            gateway: gateway,
            clock: { 0 }
        )
        XCTAssertEqual(wrongOrigin.status, .refused)
        XCTAssertTrue(gateway.callLog.isEmpty)
    }

    func testAccountWritesWaitForSignInWithoutNetwork() async {
        let gateway = FakeCommunityGateway()
        let topic = CommunityOutboxItem(
            id: "t",
            kind: .forumTopic,
            ownerScope: scope,
            origin: origin,
            createdAtMillis: 0,
            text: CommunityTextDraft(text: "body", categoryCode: "FUELS", title: "title")
        )
        let result = await CommunityOutboxDispatcher.run(topic, context: context(hasAccount: false), gateway: gateway, clock: { 0 })
        XCTAssertEqual(result.status, .draft)
        XCTAssertEqual(result.attempts, 0)
        XCTAssertTrue(gateway.callLog.isEmpty)

        let signedIn = await CommunityOutboxDispatcher.run(topic, context: context(hasAccount: true), gateway: gateway, clock: { 0 })
        XCTAssertEqual(signedIn.status, .accepted)
        XCTAssertEqual(gateway.callLog, ["forum-topic"])
    }

    func testEvidenceUploadPrecedesTheObservationAndKeepsItsReceipt() async {
        let gateway = FakeCommunityGateway()
        let result = await CommunityOutboxDispatcher.run(priceItem(photo: photo()), context: context(), gateway: gateway, clock: { 0 })
        XCTAssertEqual(gateway.callLog, ["upload", "observation"])
        XCTAssertEqual(gateway.submittedItems.last?.price?.photo?.evidenceId, "e0000000-0000-4000-8000-000000000002")
        XCTAssertEqual(result.status, .accepted)
        XCTAssertEqual(result.price?.photo?.uploadSessionId, "u-1")

        let ready = FakeCommunityGateway()
        _ = await CommunityOutboxDispatcher.run(priceItem(photo: photo(evidence: "e0000000-0000-4000-8000-000000000002", session: "u-1")), context: context(), gateway: ready, clock: { 0 })
        XCTAssertEqual(ready.callLog, ["observation"], "an evidence receipt is never uploaded twice")
    }

    func testPendingMediaKeepsTheUploadSessionForTheNextAttempt() async {
        let gateway = FakeCommunityGateway()
        gateway.evidence = .success(EvidenceProgress(uploadSessionId: "u-9", evidenceId: nil))
        let result = await CommunityOutboxDispatcher.run(priceItem(photo: photo()), context: context(), gateway: gateway, clock: { 1_000 })
        XCTAssertEqual(result.status, .pending)
        XCTAssertEqual(result.attempts, 1)
        XCTAssertEqual(result.lastCode, "contribution.media-pending")
        XCTAssertEqual(result.price?.photo?.uploadSessionId, "u-9")
        XCTAssertGreaterThan(result.nextEligibleMillis, 1_000)
        XCTAssertEqual(gateway.callLog, ["upload"])
    }

    func testLapsedPhotoExpiresWithoutNetwork() async {
        let gateway = FakeCommunityGateway()
        let result = await CommunityOutboxDispatcher.run(priceItem(photo: photo(expiresAt: now - 1)), context: context(), gateway: gateway, clock: { 0 })
        XCTAssertEqual(result.status, .expired)
        XCTAssertEqual(result.lastCode, "contribution.photo-expired")
        XCTAssertTrue(gateway.callLog.isEmpty)
    }

    func testServerRefusalAndOutageMapToHonestStates() async {
        let refusing = FakeCommunityGateway()
        refusing.receipt = .failure(SignedTransportError.refused(status: 400, code: "community.invalid", retryAfterSeconds: nil))
        let refused = await CommunityOutboxDispatcher.run(priceItem(), context: context(), gateway: refusing, clock: { 0 })
        XCTAssertEqual(refused.status, .refused)
        XCTAssertEqual(refused.lastCode, "community.invalid")

        let offline = FakeCommunityGateway()
        offline.receipt = .failure(SignedTransportError.offline)
        let pending = await CommunityOutboxDispatcher.run(priceItem(), context: context(), gateway: offline, clock: { 5_000 })
        XCTAssertEqual(pending.status, .pending)
        XCTAssertEqual(pending.attempts, 1)
        XCTAssertGreaterThan(pending.nextEligibleMillis, 5_000)

        let expiredSession = FakeCommunityGateway()
        expiredSession.receipt = .failure(CommunityWriteError.signInRequired)
        let comment = CommunityOutboxItem(id: "c", kind: .stationComment, ownerScope: scope, origin: origin, createdAtMillis: 0, text: CommunityTextDraft(text: "ok"))
        let waiting = await CommunityOutboxDispatcher.run(comment, context: context(hasAccount: true), gateway: expiredSession, clock: { 0 })
        XCTAssertEqual(waiting.status, .draft)
    }

    func testStoreRoundTripsAndPurges() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = CommunityOutboxStore(directory: directory)
        XCTAssertTrue(store.load().isEmpty)
        let items = [priceItem(photo: photo(evidence: "e0000000-0000-4000-8000-000000000002", session: "u-1"))]
        store.save(items)
        XCTAssertEqual(store.load(), items)
        store.purge()
        XCTAssertTrue(store.load().isEmpty)
    }

    func testPruneKeepsOpenItemsAndRecentTerminalOnes() {
        var items: [CommunityOutboxItem] = []
        for index in 0..<(CommunityOutbox.maxRetainedItems + 20) {
            var item = priceItem(id: "id-\(index)")
            if index % 2 == 0 {
                item = CommunityOutbox.accept(item, receipt: CommunityWriteReceipt(id: "r", state: "RECEIVED"))
            }
            items.append(item)
        }
        let pruned = CommunityOutbox.prune(items)
        XCTAssertEqual(pruned.count, CommunityOutbox.maxRetainedItems)
        XCTAssertEqual(pruned.filter { $0.status == .pending }.count, items.filter { $0.status == .pending }.count)
    }
}
