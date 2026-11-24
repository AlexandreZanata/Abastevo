import SwiftUI
import AnpFuelCore

@MainActor
struct StationFeedbackCard: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var safety: CommunitySafetyViewModel
    let station: StationRecord
    let fuel: FuelProduct

    @State private var comments: [PublicStationComment] = []
    @State private var cursor: String?
    @State private var replies: [String: [PublicStationComment]] = [:]
    @State private var replyCursors: [String: String] = [:]
    @State private var loadingComments = false
    @State private var commentReadToken = UUID()
    @State private var loadingReplies: [String: UUID] = [:]
    @State private var commentError: String?
    @State private var ratingStats: StationRatingStats?
    @State private var ratingError: String?
    @State private var ratingReadToken = UUID()
    @State private var selectedStars = 0
    @State private var editor: Editor?
    @State private var confirmation: PublicStationComment?
    @State private var activeWrite: Task<Void, Never>?
    @State private var isWriting = false
    @State private var intent: Intent?
    @State private var notice: String?

    private struct Scope: Hashable {
        let accountId: String?
        let origin: String
        let fuel: FuelProduct
        let stationId: String
        let offline: Bool
    }

    private struct ReadID: Hashable {
        let scope: Scope
        let blockRevision: Int
    }

    private struct Editor: Identifiable {
        enum Mode: String {
            case reply
            case edit
        }

        let mode: Mode
        let comment: PublicStationComment
        var id: String { mode.rawValue + ":" + comment.id }
    }

    private enum Action: Equatable {
        case rate(Int)
        case removeRating
        case reply(String, String)
        case edit(String, Int64, String)
        case remove(String)
    }

    private struct Intent: Equatable {
        let action: Action
        let scope: Scope
        let id: String
    }

    init(app: AppStateViewModel, station: StationRecord, fuel: FuelProduct) {
        self.app = app
        self.safety = app.communitySafety
        self.station = station
        self.fuel = fuel
    }

    private var scope: Scope {
        Scope(accountId: app.currentAccountIdOrNil, origin: app.snapshot.apiEnvironment.origin,
              fuel: fuel, stationId: station.id, offline: app.isOfflineSimulation)
    }

    private var ownedComments: Set<String> {
        app.identity.ownedStationComments(stationId: station.id, fuel: fuel)
    }

    var body: some View {
        let readID = ReadID(scope: scope, blockRevision: safety.revision)
        EnergiaCard {
            Text(String(localized: "android.station_page_experience_title")).font(.headline)
            Text(String(localized: "android.station_page_experience_note"))
                .font(.caption)
                .foregroundStyle(.secondary)
            ratingSection
            Divider()
            Text(String(localized: "android.station_feedback_title")).font(.headline)
            commentSection
            if let notice {
                Text(notice).font(.caption)
            }
            CommunitySafetyNoticeLine(model: safety)
        }
        .task(id: readID) {
            comments = []
            cursor = nil
            loadingReplies = [:]
            replies = [:]
            replyCursors = [:]
            await loadComments(after: nil)
        }
        .onChange(of: ownedComments) { _, _ in
            Task { await loadComments(after: nil) }
        }
        .task(id: scope) {
            selectedStars = 0
            ratingStats = nil
            notice = nil
            intent = nil
            await loadStats()
        }
        .onDisappear { activeWrite?.cancel() }
        .sheet(item: $editor) { opened in
            StationFeedbackEditor(
                title: opened.mode == .edit ? String(localized: "station.feedback.edit") : String(localized: "station.feedback.reply"),
                initialText: opened.mode == .edit ? opened.comment.text : "",
                commit: { text in submitText(opened, text: text) }
            )
        }
        .confirmationDialog(
            String(localized: "station.feedback.remove_title"), isPresented: removalIsPresented, presenting: confirmation
        ) { comment in
            Button(String(localized: "station.feedback.remove"), role: .destructive) {
                startWrite(.remove(comment.id), comment: comment)
            }
        }
    }

    private var removalIsPresented: Binding<Bool> {
        Binding(get: { confirmation != nil }, set: { if !$0 { confirmation = nil } })
    }

    @ViewBuilder
    private var ratingSection: some View {
        if let stats = ratingStats {
            if let mean = stats.meanTenths {
                let value = (Decimal(mean) / 10).formatted(.number.precision(.fractionLength(1)))
                Text(String(localized: "station.feedback.rating_summary", defaultValue: "(value) stars · ratings: (stats.count.formatted())"))
                    .font(.subheadline)
            } else {
                Text(String(localized: "android.station_page_rating_empty")).font(.subheadline)
            }
        }
        if let ratingError {
            Text(ratingError).font(.caption)
            Button(String(localized: "android.action_retry")) { Task { await loadStats() } }
        }
        if scope.accountId == nil {
            Button(String(localized: "android.station_feedback_guest")) { app.activeSheet = .auth }
                .font(.caption)
        } else {
            Picker(String(localized: "android.station_feedback_rate"), selection: $selectedStars) {
                Text(String(localized: "station.feedback.choose_stars")).tag(0)
                ForEach(1...5, id: \.self) { stars in Text(stars.formatted()).tag(stars) }
            }
            Button(String(localized: "android.station_page_rating_save")) {
                startWrite(.rate(selectedStars))
            }.disabled(isWriting || selectedStars == 0)
            Button(String(localized: "android.station_feedback_delete_rating"), role: .destructive) {
                startWrite(.removeRating)
            }.disabled(isWriting)
        }
    }

    @ViewBuilder
    private var commentSection: some View {
        if loadingComments && comments.isEmpty {
            ProgressView(String(localized: "android.station_page_feedback_loading"))
        } else if let commentError {
            Text(commentError).font(.caption)
            Button(String(localized: "android.action_retry")) { Task { await loadComments(after: cursor) } }
                .disabled(loadingComments)
        } else if comments.isEmpty {
            Text(String(localized: "android.station_page_feedback_empty")).font(.subheadline)
        }
        ForEach(comments) { comment in
            if !safety.isLocallyHidden(reference(comment)) {
                commentRow(comment, canReply: true)
                if let loaded = replies[comment.id] {
                    ForEach(loaded) { reply in
                        if !safety.isLocallyHidden(reference(reply)) {
                            commentRow(reply, canReply: false)
                                .padding(.leading, 16)
                        }
                    }
                }
                if replies[comment.id] == nil || replyCursors[comment.id] != nil {
                    Button(String(localized: "station.feedback.show_replies")) {
                        Task { await loadReplies(to: comment, after: replyCursors[comment.id]) }
                    }
                    .disabled(loadingReplies[comment.id] != nil)
                    .font(.caption)
                }
            }
        }
        if let cursor {
            Button(String(localized: "android.station_feedback_show_more")) {
                Task { await loadComments(after: cursor) }
            }.disabled(loadingComments)
        }
    }

    private func commentRow(_ comment: PublicStationComment, canReply: Bool) -> some View {
        let isOwn = ownedComments.contains(comment.id.lowercased())
        return VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(comment.publicAlias).font(.caption.bold())
                Spacer()
                if isOwn {
                    Menu {
                        Button(String(localized: "station.feedback.edit")) {
                            editor = Editor(mode: .edit, comment: comment)
                        }.disabled(comment.revision == nil)
                        Button(String(localized: "station.feedback.remove"), role: .destructive) { confirmation = comment }
                    } label: {
                        Label(String(localized: "station.feedback.comment_actions"), systemImage: "ellipsis.circle")
                            .labelStyle(.iconOnly)
                    }
                } else {
                    CommunitySafetyActions(model: safety, reference: reference(comment), onSignIn: { app.activeSheet = .auth })
                }
            }
            Text(comment.text)
            if !isOwn {
                StationCommentVoteBar(viewModel: app, commentId: comment.id)
            }
            if canReply {
                Button(String(localized: "station.feedback.reply")) {
                    if scope.accountId == nil { app.activeSheet = .auth }
                    else { editor = Editor(mode: .reply, comment: comment) }
                }
                .font(.caption)
                .disabled(isWriting)
            }
        }
    }

    private func reference(_ comment: PublicStationComment) -> CommunitySafetyReference {
        CommunitySafetyReference(kind: .stationComment, id: comment.id)
    }

    private func loadStats() async {
        let originalScope = scope
        let token = UUID()
        ratingReadToken = token
        ratingError = nil
        do {
            let stats = try await app.socialReadClient().stationRatingStats(stationId: station.id, fuel: fuel)
            guard originalScope == scope, ratingReadToken == token, !Task.isCancelled else { return }
            ratingStats = stats
        } catch {
            guard originalScope == scope, ratingReadToken == token, !Task.isCancelled else { return }
            ratingError = feedbackMessage(error)
        }
    }

    private func loadComments(after next: String?) async {
        guard !loadingComments || next == nil else { return }
        let originalScope = scope
        let revision = safety.revision
        let token = UUID()
        commentReadToken = token
        loadingComments = true
        commentError = nil
        defer {
            if commentReadToken == token { loadingComments = false }
        }
        do {
            let page = try await app.socialReadClient().stationFeedbackPage(stationId: station.id, fuel: fuel, cursor: next)
            guard let hidden = await safety.filterHidden(page.comments.map(reference)),
                  originalScope == scope, revision == safety.revision, commentReadToken == token, !Task.isCancelled else { return }
            let visible = page.comments.filter { !hidden.contains(reference($0)) }
            comments = appendUnique(visible, to: next == nil ? [] : comments)
            cursor = page.nextCursor == next ? nil : page.nextCursor
        } catch {
            guard originalScope == scope, revision == safety.revision, commentReadToken == token, !Task.isCancelled else { return }
            if let publicError = error as? HttpClientError, case .httpStatus(400, _) = publicError { cursor = nil }
            commentError = feedbackMessage(error)
        }
    }

    private func loadReplies(to parent: PublicStationComment, after next: String?) async {
        guard loadingReplies[parent.id] == nil else { return }
        let token = UUID()
        loadingReplies[parent.id] = token
        let originalScope = scope
        let revision = safety.revision
        defer {
            if loadingReplies[parent.id] == token { loadingReplies.removeValue(forKey: parent.id) }
        }
        do {
            let page = try await app.socialReadClient().stationFeedbackPage(
                stationId: station.id, fuel: fuel, parentId: parent.id, cursor: next
            )
            guard let hidden = await safety.filterHidden(page.comments.map(reference)),
                  originalScope == scope, revision == safety.revision, loadingReplies[parent.id] == token, !Task.isCancelled else { return }
            replies[parent.id] = appendUnique(
                page.comments.filter { !hidden.contains(reference($0)) },
                to: next == nil ? [] : (replies[parent.id] ?? [])
            )
            replyCursors[parent.id] = page.nextCursor == next ? nil : page.nextCursor
        } catch {
            guard originalScope == scope, revision == safety.revision, loadingReplies[parent.id] == token, !Task.isCancelled else { return }
            notice = feedbackMessage(error)
        }
    }

    private func appendUnique(_ inserted: [PublicStationComment], to existing: [PublicStationComment]) -> [PublicStationComment] {
        var ids = Set(existing.map(\.id))
        return existing + inserted.filter { ids.insert($0.id).inserted }
    }

    private func submitText(_ opened: Editor, text: String) {
        let text = PortableText.normalize(text)
        switch opened.mode {
        case .edit:
            guard let revision = opened.comment.revision else { return }
            startWrite(.edit(opened.comment.id, revision, text), comment: opened.comment)
        case .reply:
            startWrite(.reply(opened.comment.id, text), comment: opened.comment)
        }
    }

    private func startWrite(_ action: Action, comment: PublicStationComment? = nil) {
        guard !isWriting, !app.isAuthWorking else { return }
        let originalScope = scope
        let gateway = app.communityGateway
        guard originalScope.accountId != nil, let owner = gateway.transport.ownerScope() else {
            app.activeSheet = .auth
            return
        }
        guard !originalScope.offline else {
            notice = String(localized: "station.feedback.offline")
            return
        }
        let operation = (intent?.action == action && intent?.scope == originalScope ? intent?.id : nil)
            ?? UUID().uuidString.lowercased()
        intent = Intent(action: action, scope: originalScope, id: operation)
        isWriting = true
        notice = nil
        activeWrite = Task {
            defer { isWriting = false }
            do {
                switch action {
                case .rate(let stars):
                    let receipt = try await gateway.rateStation(stationId: station.id, fuel: fuel, stars: stars,
                                                                ownerScope: owner, operationId: operation)
                    guard originalScope == scope, !Task.isCancelled else { return }
                    ratingReadToken = UUID()
                    ratingStats = receipt.stats
                case .removeRating:
                    try await gateway.removeStationRating(stationId: station.id, fuel: fuel, ownerScope: owner, operationId: operation)
                    guard originalScope == scope, !Task.isCancelled else { return }
                    selectedStars = 0
                    await loadStats()
                case .reply(_, let text):
                    guard let parent = comment else { return }
                    let reply = try await gateway.replyToStationComment(parent, fuel: fuel, text: text,
                                                                        ownerScope: owner, operationId: operation)
                    guard originalScope == scope, !Task.isCancelled else { return }
                    app.identity.recordStationReply(reply, fuel: fuel, text: text, ownerScope: owner, operationId: operation)
                    if let firstPage = replies[parent.id] {
                        replies[parent.id] = appendUnique([reply], to: firstPage)
                    } else {
                        await loadReplies(to: parent, after: nil)
                    }
                case .edit(_, _, let text):
                    guard let current = comment else { return }
                    let edited = try await gateway.editStationComment(current, fuel: fuel, text: text,
                                                                     ownerScope: owner, operationId: operation)
                    guard originalScope == scope, !Task.isCancelled else { return }
                    if let parent = edited.parentId {
                        replies[parent] = replies[parent]?.map { $0.id == edited.id ? edited : $0 }
                    } else {
                        comments = comments.map { $0.id == edited.id ? edited : $0 }
                    }
                case .remove(let id):
                    try await gateway.removeStationComment(commentId: id, ownerScope: owner, operationId: operation)
                    guard originalScope == scope, !Task.isCancelled else { return }
                    app.identity.forgetStationCommentReceipt(id, ownerScope: owner)
                    if let parent = comment?.parentId {
                        replies[parent]?.removeAll { $0.id == id }
                    } else {
                        comments.removeAll { $0.id == id }
                        replies.removeValue(forKey: id)
                        replyCursors.removeValue(forKey: id)
                    }
                }
                guard originalScope == scope, !Task.isCancelled else { return }
                intent = nil
                notice = String(localized: "android.station_page_write_received")
            } catch {
                guard originalScope == scope, !Task.isCancelled else { return }
                if case .reply = action, writeMayHaveReachedServer(error) {
                    notice = String(localized: "station.feedback.reply_unknown")
                    if let parent = comment { await loadReplies(to: parent, after: nil) }
                } else {
                    notice = feedbackMessage(error)
                    if let refusal = error as? SignedTransportError, case .refused(409, _, _) = refusal {
                        if let parentId = comment?.parentId,
                           let parent = comments.first(where: { $0.id == parentId }) {
                            await loadReplies(to: parent, after: nil)
                        } else {
                            await loadComments(after: nil)
                        }
                    }
                }
            }
        }
    }

    private func writeMayHaveReachedServer(_ error: Error) -> Bool {
        switch error {
        case SignedTransportError.offline, SignedTransportError.invalidResponse,
             CommunityWriteError.invalidReceipt, is DecodingError:
            return true
        case SignedTransportError.refused(let status, _, _):
            return status == 408 || status >= 500
        default:
            return false
        }
    }

    private func feedbackMessage(_ error: Error) -> String {
        let code: String?
        switch error {
        case SignedTransportError.sessionRequired, CommunityWriteError.signInRequired:
            code = "feedback.session-invalid"
        case SignedTransportError.refused(_, let responseCode, _), HttpClientError.httpStatus(_, let responseCode):
            code = responseCode
        case CommunityWriteError.invalid(let responseCode):
            code = responseCode
        default:
            code = nil
        }
        switch code {
        case "feedback.session-invalid":
            return String(localized: "android.station_feedback_guest")
        case "feedback.rating-out-of-range":
            return String(localized: "android.station_feedback_rate")
        case "feedback.rating-not-found":
            return String(localized: "station.feedback.no_own_rating")
        case "feedback.comment-not-found", "feedback.parent-invalid":
            return String(localized: "station.feedback.comment_gone")
        case "feedback.stale-revision":
            return String(localized: "station.feedback.stale")
        case "feedback.text-empty", "feedback.text-too-long", "feedback.text-invalid-encoding":
            return String(localized: "android.station_feedback_comment_hint")
        case "feedback.author-forbidden":
            return String(localized: "station.feedback.author_forbidden")
        case "feedback.quota-exceeded":
            return String(localized: "station.feedback.rate_limited")
        default:
            return String(localized: "android.station_page_feedback_failed")
        }
    }
}

private struct StationFeedbackEditor: View {
    @Environment(\.dismiss) private var dismiss
    @State var text: String
    let title: String
    let commit: (String) -> Void

    init(title: String, initialText: String, commit: @escaping (String) -> Void) {
        self.title = title
        self._text = State(initialValue: initialText)
        self.commit = commit
    }

    var body: some View {
        NavigationStack {
            Form {
                TextField(String(localized: "android.station_feedback_comment_hint"), text: $text, axis: .vertical)
                    .lineLimit(3...8)
            }
            .navigationTitle(title)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.map_cancel")) { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "android.forum_comment_publish")) {
                        commit(text)
                        dismiss()
                    }.disabled(!PortableText.isValidComment(text))
                }
            }
        }
    }
}
