import SwiftUI
import AnpFuelCore

struct ForumScreenScope: Hashable {
    let origin: String
    let accountId: String?
    let ownerGeneration: UUID?
}

extension AppStateViewModel {
    var forumScreenScope: ForumScreenScope {
        ForumScreenScope(
            origin: snapshot.apiEnvironment.origin, accountId: currentAccountIdOrNil,
            ownerGeneration: communitySafety.ownerGeneration
        )
    }
}

struct ForumThreadPage {
    var content = ForumPageAccumulator<ForumPublicComment>()
    var isLoading = false
    var issue: String?
}

@MainActor
final class ForumDiscussionModel: ObservableObject {
    @Published private(set) var detail: ForumPublicDetail?
    @Published private(set) var isLoadingDetail = true
    @Published private(set) var detailIssue: String?
    @Published private(set) var hidePost = false
    @Published private(set) var comments = ForumThreadPage()
    @Published private(set) var replies: [String: ForumThreadPage] = [:]
    @Published private(set) var expanded: Set<String> = []
    @Published private(set) var hidden: Set<CommunitySafetyReference> = []
    @Published private(set) var submittedCommentIds: Set<String> = []
    @Published private(set) var ownCommentIds: Set<String> = []
    @Published private(set) var ownsTopic = false
    @Published private(set) var ownReactionsKnown = false
    @Published private(set) var actionNotice: String?
    @Published private(set) var isWriting = false
    @Published private(set) var pending: ForumPendingCommand?
    @Published private(set) var pendingNeedsReview = false
    @Published var sort: ForumReplySort = .relevant
    @Published var replyingTo: ForumPublicComment?

    private let post: ForumPublicPost
    private var generation = UUID()
    private var commentsGeneration = UUID()
    private var visibilityGeneration = UUID()
    @Published private var selectedReactions: [ForumReactionTarget: ForumReaction] = [:]
    @Published private var savedCounts: [ForumReactionTarget: ForumReactionCounts] = [:]

    init(post: ForumPublicPost) {
        self.post = post
    }

    func owns(_ comment: ForumPublicComment) -> Bool {
        ownCommentIds.contains(comment.id.lowercased())
    }

    func reaction(to target: ForumReactionTarget) -> ForumReaction {
        selectedReactions[target] ?? .none
    }

    func counts(for target: ForumReactionTarget, base: ForumReactionCounts?) -> ForumReactionCounts? {
        savedCounts[target] ?? base
    }

    func open(_ host: AppStateViewModel, reset: Bool) async {
        guard reset || !isWriting else { return }
        let scope = host.forumScreenScope
        let token = UUID()
        generation = token
        commentsGeneration = UUID()
        visibilityGeneration = UUID()
        if reset {
            detail = nil
            comments = ForumThreadPage()
            replies = [:]
            expanded = []
            hidden = []
            submittedCommentIds = []
            ownCommentIds = []
            ownsTopic = false
            ownReactionsKnown = false
            actionNotice = nil
            isWriting = false
            pending = nil
            pendingNeedsReview = false
            replyingTo = nil
            selectedReactions = [:]
            savedCounts = [:]
            hidePost = false
        }
        isLoadingDetail = true
        detailIssue = nil
        defer {
            if generation == token { isLoadingDetail = false }
        }
        do {
            let client = try host.socialReadClient()
            let resolved = try await client.fetchForumPost(id: post.id)
            guard current(host, scope: scope, token: token) else { return }

            let references = post.kind == .topic ? [Self.postReference(post.id)] : []
            guard let checked = await host.communitySafety.filterHidden(references),
                  current(host, scope: scope, token: token) else { return }
            hidden = checked
            hidePost = references.contains(where: checked.contains)
            detail = resolved
            savedCounts.removeAll()
            guard !hidePost else { return }

            if resolved.commentsEnabled != false {
                await loadComments(host, reset: true)
            } else {
                comments = ForumThreadPage()
                replies = [:]
                expanded = []
            }
            guard current(host, scope: scope, token: token) else { return }
            await refreshOwner(host, scope: scope, token: token)
        } catch {
            guard current(host, scope: scope, token: token) else { return }
            if let read = error as? HttpClientError, case .httpStatus(404, _) = read {
                detail = nil
                comments = ForumThreadPage()
                replies = [:]
                expanded = []
                ownCommentIds = []
                ownsTopic = false
                ownReactionsKnown = false
                replyingTo = nil
                pending = nil
                pendingNeedsReview = false
                selectedReactions = [:]
                savedCounts = [:]
            }
            detailIssue = ForumFeedback.describe(error)
        }
    }

    func loadComments(_ host: AppStateViewModel, reset: Bool) async {
        guard detail?.commentsEnabled != false, detail != nil, !hidePost else { return }
        let scope = host.forumScreenScope
        let token = generation
        let chosenSort = sort
        let cursor = reset ? nil : comments.content.nextCursor
        if reset {
            commentsGeneration = UUID()
            comments = ForumThreadPage()
            replies = [:]
            expanded = []
            if !isWriting {
                savedCounts = savedCounts.filter { target, _ in
                    if case .post = target { return true }
                    return false
                }
            }
        } else {
            guard cursor != nil, !comments.isLoading else { return }
        }
        let read = commentsGeneration
        comments.isLoading = true
        comments.issue = nil
        defer {
            if generation == token, commentsGeneration == read { comments.isLoading = false }
        }
        do {
            let page = try await host.socialReadClient().fetchForumCommentsPage(
                postId: post.id, sort: chosenSort, cursor: cursor
            )
            guard current(host, scope: scope, token: token), commentsGeneration == read else { return }
            let refs = Self.references(page.items)
            guard let checked = await host.communitySafety.filterHidden(refs),
                  current(host, scope: scope, token: token), commentsGeneration == read else { return }
            hidden.subtract(refs)
            hidden.formUnion(checked)
            if let cursor {
                try comments.content.append(page.items, after: cursor, nextCursor: page.nextCursor)
            } else {
                try comments.content.replace(page.items, nextCursor: page.nextCursor)
            }
        } catch {
            guard current(host, scope: scope, token: token), commentsGeneration == read else { return }
            comments.issue = ForumFeedback.describe(error)
        }
    }

    func toggleReplies(for parent: ForumPublicComment, host: AppStateViewModel) async {
        if expanded.remove(parent.id) != nil { return }
        expanded.insert(parent.id)
        if replies[parent.id] == nil {
            await loadReplies(for: parent, host: host, first: true)
        }
    }

    func loadReplies(for parent: ForumPublicComment, host: AppStateViewModel, first: Bool) async {
        guard parent.depth == 0, !hidePost else { return }
        var state = replies[parent.id] ?? ForumThreadPage()
        guard !state.isLoading else { return }
        let cursor = first ? nil : state.content.nextCursor
        guard first || cursor != nil else { return }

        let scope = host.forumScreenScope
        let token = generation
        let read = commentsGeneration
        state.isLoading = true
        state.issue = nil
        replies[parent.id] = state
        defer {
            if generation == token, commentsGeneration == read {
                replies[parent.id]?.isLoading = false
            }
        }
        do {
            let page = try await host.socialReadClient().fetchForumReplies(
                postId: post.id, parentId: parent.id, cursor: cursor
            )
            guard current(host, scope: scope, token: token), commentsGeneration == read else { return }
            let refs = Self.references(page.items)
            guard let checked = await host.communitySafety.filterHidden(refs),
                  current(host, scope: scope, token: token), commentsGeneration == read else { return }
            hidden.subtract(refs)
            hidden.formUnion(checked)
            if let cursor {
                try state.content.append(page.items, after: cursor, nextCursor: page.nextCursor)
            } else {
                try state.content.replace(page.items, nextCursor: page.nextCursor)
            }
            state.isLoading = false
            replies[parent.id] = state
        } catch {
            guard current(host, scope: scope, token: token), commentsGeneration == read else { return }
            replies[parent.id]?.issue = ForumFeedback.describe(error)
        }
    }

    func refilter(_ host: AppStateViewModel) async {
        guard detail != nil else { return }
        let scope = host.forumScreenScope
        let token = generation
        let visibility = UUID()
        visibilityGeneration = visibility
        let postRef = Self.postReference(post.id)
        var refs = Set(Self.references(comments.content.items))
        for replyPage in replies.values {
            refs.formUnion(Self.references(replyPage.content.items))
        }
        if post.kind == .topic { refs.insert(postRef) }
        guard let checked = await host.communitySafety.filterHidden(Array(refs)),
              current(host, scope: scope, token: token), visibilityGeneration == visibility else { return }
        let wasHidden = hidePost
        hidden.subtract(refs)
        hidden.formUnion(checked)
        hidePost = checked.contains(postRef)
        if let reply = replyingTo, checked.contains(Self.commentReference(reply.id)) {
            replyingTo = nil
        }
        if wasHidden, !hidePost {
            await loadComments(host, reset: true)
        }
    }

    private func refreshOwner(_ host: AppStateViewModel, scope: ForumScreenScope, token: UUID) async {
        let client = host.communityGateway
        guard scope.accountId != nil, let owner = client.transport.ownerScope() else { return }
        let postId = post.id
        let isTopic = post.kind == .topic
        async let ownComments = try? client.forumOwnCommentIds(postId: postId, expectedScope: owner)
        async let ownReactions = try? client.forumOwnReactions(postId: postId, expectedScope: owner)
        async let ownTopic: ForumWriteReceipt? = isTopic
            ? (try? client.forumOwnTopic(topicId: postId, expectedScope: owner))
            : nil
        let (ids, reactions, topic) = await (ownComments, ownReactions, ownTopic)
        guard current(host, scope: scope, token: token), client.transport.ownerScope() == owner else { return }
        ownCommentIds = ids ?? []
        ownsTopic = topic != nil
        if let reactions, !isWriting, pending == nil {
            ownReactionsKnown = true
            selectedReactions = [.post(post.id): reactions.postReaction]
            for comment in reactions.comments {
                selectedReactions[.comment(comment.commentId)] = comment.reaction
            }
        }
    }

    func react(_ tapped: ForumReaction, target: ForumReactionTarget, base: ForumReactionCounts?, host: AppStateViewModel) async {
        guard !isWriting else { return }
        guard host.currentAccountIdOrNil != nil else {
            host.activeSheet = .auth
            return
        }
        if !ownReactionsKnown {
            let scope = host.forumScreenScope
            let token = generation
            await refreshOwner(host, scope: scope, token: token)
            guard current(host, scope: scope, token: token) else { return }
            guard ownReactionsKnown else {
                actionNotice = String(localized: "android.forum_unavailable_hint")
                return
            }
        }
        let next = reaction(to: target).toggled(with: tapped)
        let intent = ForumCommand.reaction(target: target, reaction: next)
        _ = await submit(intent, baseCounts: base, host: host)
    }

    func edit(_ comment: ForumPublicComment, text: String, host: AppStateViewModel) async -> Bool {
        guard owns(comment), revisionIsCurrent(comment) else { return false }
        let intent = ForumCommand.editComment(commentId: comment.id, revision: comment.revision, text: text)
        return await submit(intent, host: host)
    }

    func remove(_ comment: ForumPublicComment, host: AppStateViewModel) async {
        guard owns(comment), revisionIsCurrent(comment) else { return }
        let intent = ForumCommand.removeComment(commentId: comment.id, revision: comment.revision)
        _ = await submit(intent, host: host)
    }

    func retry(_ host: AppStateViewModel) async {
        guard let pending else { return }
        let counts: ForumReactionCounts?
        if case .reaction(let target, _) = pending.intent {
            switch target {
            case .post: counts = detail?.reactionCounts
            case .comment(let id): counts = commentValue(id: id)?.reactionCounts
            }
        } else {
            counts = nil
        }
        _ = await submit(pending.intent, baseCounts: counts, host: host)
    }

    private func submit(
        _ intent: ForumCommand, baseCounts: ForumReactionCounts? = nil, host: AppStateViewModel
    ) async -> Bool {
        guard !isWriting, !hidePost else { return false }
        let scope = host.forumScreenScope
        let client = host.communityGateway
        let token = generation
        guard scope.accountId != nil, let owner = client.transport.ownerScope() else {
            host.activeSheet = .auth
            return false
        }
        if pendingNeedsReview || (pending != nil && pending?.intent != intent) {
            actionNotice = String(localized: "forum.actions.resolve_first")
            return false
        }
        var command = pending ?? ForumPendingCommand(intent: intent)
        guard command.canRetry(at: .now) else {
            actionNotice = ForumFeedback.rateLimited(until: command.retryNotBefore)
            return false
        }

        let prior: (ForumReactionTarget, ForumReaction, ForumReactionCounts?)?
        if case .reaction(let target, let next) = intent {
            let previous = reaction(to: target)
            prior = (target, previous, savedCounts[target])
            selectedReactions[target] = next
            if let base = counts(for: target, base: baseCounts) {
                savedCounts[target] = base.applying(from: previous, to: next)
            }
        } else {
            prior = nil
        }
        pending = command
        isWriting = true
        actionNotice = nil
        defer {
            if generation == token { isWriting = false }
        }
        do {
            let result = try await client.submitForumCommand(command, expectedScope: owner)
            guard current(host, scope: scope, token: token), client.transport.ownerScope() == owner else { return false }
            pending = nil
            switch result {
            case .comment(let receipt):
                removeFromView(id: receipt.id)
                if receipt.publicationState == "PENDING_REVIEW" {
                    actionNotice = String(localized: "android.forum_comment_pending_receipt")
                }
            case .reaction(let receipt):
                if case .reaction(let target, _) = intent {
                    selectedReactions[target] = receipt.reaction
                    savedCounts[target] = receipt.counts
                }
            }
            return true
        } catch {
            guard current(host, scope: scope, token: token), client.transport.ownerScope() == owner else { return false }
            if let (target, reaction, counts) = prior {
                selectedReactions[target] = reaction
                savedCounts[target] = counts
            }
            if command.retainAfterFailure(error, now: .now) {
                pending = command
            } else {
                pending = nil
            }
            if let signed = error as? SignedTransportError, case .refused(409, _, _) = signed {
                pendingNeedsReview = true
            }
            actionNotice = ForumFeedback.describe(error)
            return false
        }
    }

    func refreshAfterConflict(_ host: AppStateViewModel) async {
        let scope = host.forumScreenScope
        await open(host, reset: false)
        if scope == host.forumScreenScope, detailIssue == nil, comments.issue == nil {
            pendingNeedsReview = false
            actionNotice = nil
        }
    }

    func closeScreen() {
        generation = UUID()
        commentsGeneration = UUID()
        visibilityGeneration = UUID()
        ownCommentIds = []
        ownsTopic = false
        ownReactionsKnown = false
        selectedReactions = [:]
        savedCounts = [:]
        replyingTo = nil
        pending = nil
        isWriting = false
    }

    func freshEditTarget(_ comment: ForumPublicComment) -> ForumPublicComment? {
        guard owns(comment), !hidePost else { return nil }
        let reference = Self.commentReference(comment.id)
        guard !hidden.contains(reference) else { return nil }
        return commentValue(id: comment.id)
    }

    private func current(_ host: AppStateViewModel, scope: ForumScreenScope, token: UUID) -> Bool {
        !Task.isCancelled && host.forumScreenScope == scope && generation == token
    }

    private func revisionIsCurrent(_ original: ForumPublicComment) -> Bool {
        commentValue(id: original.id)?.revision == original.revision
    }

    private func commentValue(id: String) -> ForumPublicComment? {
        if let comment = comments.content.items.first(where: { $0.id == id }) { return comment }
        for state in replies.values {
            if let reply = state.content.items.first(where: { $0.id == id }) { return reply }
        }
        return nil
    }

    private func removeFromView(id: String) {
        comments.content.remove(id)
        replies[id] = nil
        expanded.remove(id)
        for parent in Array(replies.keys) {
            replies[parent]?.content.remove(id)
        }
        submittedCommentIds.insert(id.lowercased())
        if replyingTo?.id == id { replyingTo = nil }
    }

    static func postReference(_ id: String) -> CommunitySafetyReference {
        CommunitySafetyReference(kind: .forumPost, id: id)
    }

    static func commentReference(_ id: String) -> CommunitySafetyReference {
        CommunitySafetyReference(kind: .forumComment, id: id)
    }

    private static func references(_ items: [ForumPublicComment]) -> [CommunitySafetyReference] {
        items.flatMap { comment in
            var refs = [commentReference(comment.id)]
            if let preview = comment.topReply { refs.append(commentReference(preview.id)) }
            return refs
        }
    }
}

enum ForumFeedback {
    static func describe(_ error: Error) -> String {
        if let write = error as? CommunityWriteError {
            switch write {
            case .signInRequired:
                return String(localized: "android.forum_sign_in_to_participate")
            case .invalid(let code):
                return response(400, code)
            default:
                break
            }
        }
        if let signed = error as? SignedTransportError {
            switch signed {
            case .refused(let status, let code, let retryAfter):
                if status == 429, let retryAfter {
                    return String(localized: "android.forum_rate_limited_error", defaultValue: "Limit exceeded. Please wait \(min(retryAfter, 86_400)) seconds.")
                }
                return response(status, code)
            case .sessionRequired:
                return String(localized: "android.forum_sign_in_to_participate")
            default:
                break
            }
        }
        if let read = error as? HttpClientError, case .httpStatus(let status, let code) = read {
            return response(status, code)
        }
        return String(localized: "android.forum_unavailable_hint")
    }

    private static func response(_ status: Int, _ code: String?) -> String {
        if code == "forum.comments-disabled" { return String(localized: "android.forum_comments_disabled") }
        if code == "forum.bad-cursor" { return String(localized: "forum.cursor_invalid") }
        switch status {
        case 401: return String(localized: "android.forum_sign_in_to_participate")
        case 404: return String(localized: "android.forum_post_not_found_hint")
        case 409: return String(localized: "android.forum_conflict_error")
        case 429: return String(localized: "forum.rate_limited")
        case 400: return String(localized: "forum.invalid_request")
        default: return String(localized: "android.forum_unavailable_hint")
        }
    }

    static func rateLimited(until date: Date?) -> String {
        guard let date else { return String(localized: "forum.rate_limited") }
        let seconds = Int64(max(0, ceil(date.timeIntervalSinceNow)))
        return String(localized: "android.forum_rate_limited_error", defaultValue: "Limit exceeded. Please wait \(seconds) seconds.")
    }
}
