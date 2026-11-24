import Combine
import Foundation
import AnpFuelCore

@MainActor
public final class CommunitySafetyViewModel: ObservableObject {
    public enum BlockListState: Equatable {
        case signedOut
        case idle
        case loading
        case loaded([CommunityBlockedUser])
        case failed(CommunitySafetyError)
    }

    public enum Notice {
        case forumReport
        case stationReport
        case userReport(replayed: Bool)
        case blocked
        case unblocked
    }

    @Published public private(set) var revision = 0
    @Published public private(set) var blockList: BlockListState = .signedOut
    @Published public private(set) var notice: Notice?
    @Published public private(set) var removingBlock = false

    private struct Owner: Equatable, Sendable {
        let scope: String
        let origin: String
        let generation = UUID()
    }

    private var owner: Owner?
    private var client: CommunityWriteClient?
    private var blockCount: Int?
    private var locallyBlocked: [CommunitySafetyReference: String] = [:]
    private var blockListLoad: UUID?
    private let lookupBudget: Duration

    public init(lookupBudget: Duration = CommunitySafetyWire.hiddenLookupBudget) {
        self.lookupBudget = lookupBudget
    }

    public var signedIn: Bool { owner != nil }
    public var ownerGeneration: UUID? { owner?.generation }

    public func configure(client: CommunityWriteClient, ownerScope: String?, origin: String) {
        guard let ownerScope, ownerScope.hasPrefix("account:"), client.transport.origin == origin else {
            clear()
            return
        }
        if owner?.scope == ownerScope, owner?.origin == origin {
            self.client = client
            return
        }
        reset()
        owner = Owner(scope: ownerScope, origin: origin)
        self.client = client
        blockList = .idle
    }

    public func clear() {
        guard owner != nil else { return }
        reset()
        owner = nil
        client = nil
        blockList = .signedOut
    }

    private func reset() {
        blockCount = nil
        locallyBlocked = [:]
        blockListLoad = nil
        notice = nil
        removingBlock = false
        revision += 1
    }

    public func isLocallyHidden(_ reference: CommunitySafetyReference) -> Bool {
        locallyBlocked[reference] != nil
    }

    /// B-BR-SAFE-06: never render while checking; show unchecked content on timeout.
    /// Nil distinguishes a stale or cancelled lookup from a resolved empty set.
    public func filterHidden(_ references: [CommunitySafetyReference]) async -> Set<CommunitySafetyReference>? {
        guard !Task.isCancelled else { return nil }
        guard let owner, let client else { return [] }
        let startingRevision = revision
        let resolution = await client.resolveHiddenCommunityContent(
            references,
            immediatelyHidden: Set(locallyBlocked.keys),
            knownBlockCount: blockCount,
            expectedScope: owner.scope,
            budget: lookupBudget
        )
        guard let resolution, !Task.isCancelled, self.owner == owner, revision == startingRevision else { return nil }
        if let count = resolution.blockCount, blockCount == nil { blockCount = count }
        return resolution.hidden
    }

    public func refreshBlockedUsers() async {
        guard let client, let owner else { return }
        let load = UUID()
        blockListLoad = load
        let startingRevision = revision
        blockList = .loading
        do {
            let users = try await client.listCommunityBlocks(expectedScope: owner.scope)
            guard !Task.isCancelled, self.owner == owner, revision == startingRevision,
                  blockListLoad == load else { return }
            let activeIds = Set(users.map(\.id))
            let confirmed = locallyBlocked.filter { activeIds.contains($0.value) }
            let shouldRefilter = blockCount != users.count || confirmed != locallyBlocked
            locallyBlocked = confirmed
            blockCount = users.count
            blockList = .loaded(users)
            if shouldRefilter { revision += 1 }
        } catch {
            guard !Task.isCancelled, self.owner == owner, revision == startingRevision,
                  blockListLoad == load else { return }
            blockList = .failed(CommunitySafetyError.from(error))
        }
    }

    public func reportUser(
        _ reference: CommunitySafetyReference, reason: CommunityUserReportReason, detail: String?, operationId: String
    ) async throws {
        guard let client, let owner else { throw CommunitySafetyError.signInRequired }
        let receipt = try await client.reportCommunityUser(reference, reason: reason, detail: detail,
                                                           operationId: operationId, expectedScope: owner.scope)
        try requireCurrent(owner)
        notice = .userReport(replayed: receipt.replayed)
    }

    public func reportForum(
        _ reference: CommunitySafetyReference, reason: CommunityForumReportReason, detail: String?, operationId: String
    ) async throws {
        guard let client, let owner else { throw CommunitySafetyError.signInRequired }
        _ = try await client.reportForumContent(reference, reason: reason, detail: detail,
                                                operationId: operationId, expectedScope: owner.scope)
        try requireCurrent(owner)
        notice = .forumReport
    }

    public func reportStation(
        _ reference: CommunitySafetyReference, reason: CommunityStationReportReason, operationId: String
    ) async throws {
        guard let client, let owner else { throw CommunitySafetyError.signInRequired }
        try await client.reportStationComment(reference, reason: reason, operationId: operationId, expectedScope: owner.scope)
        try requireCurrent(owner)
        notice = .stationReport
    }

    public func block(_ reference: CommunitySafetyReference, operationId: String) async throws {
        guard let client, let owner else { throw CommunitySafetyError.signInRequired }
        let receipt = try await client.blockCommunityUser(reference, operationId: operationId, expectedScope: owner.scope)
        try requireCurrent(owner)
        locallyBlocked[reference] = receipt.user.id
        if receipt.replayed {
            if blockCount == 0 { blockCount = nil }
        } else {
            blockCount = blockCount.map { $0 + 1 }
        }
        if case .loaded(let users) = blockList, !users.contains(where: { $0.id == receipt.user.id }) {
            blockList = .loaded([receipt.user] + users)
        }
        notice = .blocked
        revision += 1
    }

    public func unblock(id: String, operationId: String) async throws {
        guard let client, let owner else { throw CommunitySafetyError.signInRequired }
        guard !removingBlock else { return }
        removingBlock = true
        defer {
            if self.owner == owner { removingBlock = false }
        }
        do {
            try await client.removeCommunityBlock(id: id, operationId: operationId, expectedScope: owner.scope)
            try requireCurrent(owner)
            locallyBlocked = locallyBlocked.filter { $0.value != id }
            blockCount = blockCount.map { max(0, $0 - 1) }
            if case .loaded(let users) = blockList {
                blockList = .loaded(users.filter { $0.id != id })
            }
            notice = .unblocked
            revision += 1
        } catch CommunitySafetyError.refused(status: 404, code: _, retryAfterSeconds: _) {
            try requireCurrent(owner)
            locallyBlocked = locallyBlocked.filter { $0.value != id }
            blockCount = nil
            notice = nil
            revision += 1
            await refreshBlockedUsers()
        }
    }

    private func requireCurrent(_ expected: Owner) throws {
        guard !Task.isCancelled, owner == expected else { throw CommunitySafetyError.staleOwner }
    }
}
