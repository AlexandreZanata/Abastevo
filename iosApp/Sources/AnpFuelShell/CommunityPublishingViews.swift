import SwiftUI
import AnpFuelCore
#if canImport(AuthenticationServices)
import AuthenticationServices
#endif

/// Sign in with Apple for the free account. No name or e-mail scope is requested; the request nonce is the
/// SHA-256 of a raw nonce the app sends to `POST /v1/accounts/providers/login`.
public struct AppleSignInSection: View {
    @ObservedObject var viewModel: AppStateViewModel
    @Environment(\.colorScheme) private var colorScheme

    public init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        Section(String(localized: "identity.apple_sign_in")) {
            #if canImport(AuthenticationServices)
            SignInWithAppleButton(.signIn) { request in
                request.requestedScopes = []
                request.nonce = viewModel.prepareAppleSignInNonce()
            } onCompletion: { result in
                switch result {
                case .success(let authorization):
                    if let credential = authorization.credential as? ASAuthorizationAppleIDCredential {
                        viewModel.completeAppleSignIn(
                            identityToken: credential.identityToken,
                            authorizationCode: credential.authorizationCode
                        )
                    } else {
                        viewModel.failAppleSignIn(cancelled: false)
                    }
                case .failure(let error):
                    let cancelled = (error as? ASAuthorizationError)?.code == .canceled
                    viewModel.failAppleSignIn(cancelled: cancelled)
                }
            }
            .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
            .frame(height: 44)
            .disabled(viewModel.isAuthWorking || viewModel.isOfflineSimulation)
            Text(String(localized: "identity.apple_privacy"))
                .font(.caption)
                .foregroundStyle(.secondary)
            #else
            Text(String(localized: "identity.platform_unavailable"))
                .foregroundStyle(.secondary)
            #endif
        }
    }
}

/// One queued community write with its honest state and the actions that state allows.
public struct CommunityOutboxRow: View {
    @ObservedObject var viewModel: AppStateViewModel
    let item: CommunityOutboxItem

    public init(viewModel: AppStateViewModel, item: CommunityOutboxItem) {
        self.viewModel = viewModel
        self.item = item
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .font(.subheadline.weight(.semibold))
                .lineLimit(3)
            Text(IdentityCoordinator.statusLabel(for: item))
                .font(.caption)
                .foregroundStyle(statusColor)
            HStack(spacing: 16) {
                if item.status == .pending {
                    Button(String(localized: "android.action_retry")) {
                        viewModel.identity.retryNow(item.id)
                    }
                }
                if item.status == .unconfirmed {
                    Button(String(localized: "identity.outbox_send_again", defaultValue: "Send again")) {
                        viewModel.identity.retryNow(item.id)
                    }
                }
                if item.status == .draft && viewModel.currentAccountIdOrNil == nil {
                    Button(String(localized: "community.sign_in_publish")) {
                        viewModel.activeSheet = .auth
                    }
                }
                if item.status.isTerminal {
                    Button(String(localized: "community.remove_from_list"), role: .destructive) {
                        viewModel.identity.discard(item.id)
                    }
                } else {
                    Button(String(localized: "android.forum_topic_discard_confirm"), role: .destructive) {
                        viewModel.identity.discard(item.id)
                    }
                }
            }
            .font(.caption)
            .buttonStyle(.borderless)
        }
        .padding(.vertical, 2)
    }

    private var title: String {
        switch item.kind {
        case .priceObservation:
            guard let price = item.price else { return "" }
            var parts = [price.stationName, price.product.localizedName, AbastevoTheme.formatBrlMilli(price.amountMilli)]
            if price.photo != nil {
                parts.append(String(localized: "community.with_photo"))
            }
            return parts.joined(separator: " · ")
        case .stationComment, .forumComment:
            return item.text?.text ?? ""
        case .forumTopic:
            return item.text?.title ?? ""
        case .stationSuggestion:
            return item.stationProposal?.label ?? ""
        }
    }

    private var statusColor: Color {
        switch item.status {
        case .accepted:
            return AbastevoTheme.accentLight
        case .refused, .expired:
            return AbastevoTheme.errorRed
        case .draft, .pending, .unconfirmed:
            return Color.secondary
        }
    }
}

/// Station comment composer: publishes through the queue with the account session (280 Unicode scalars).
public struct StationCommentComposerCard: View {
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord
    @State private var text = ""
    @State private var notice: String?

    public init(viewModel: AppStateViewModel, station: StationRecord) {
        self.viewModel = viewModel
        self.station = station
    }

    public var body: some View {
        let fuel = viewModel.snapshot.selectedFuel
        let scalars = PortableText.countScalars(PortableText.normalize(text))
        let queued = viewModel.identity.items(of: .stationComment).filter { $0.text?.stationId == station.id }
        let legacy = viewModel.snapshot.stationComments.filter { $0.isOwn && $0.stationId == station.id }
        EnergiaCard {
            Text(String(localized: "community.comment_fuel", defaultValue: "Your comment · \(fuel.localizedName)"))
                .font(.headline)
            TextField(String(localized: "android.station_feedback_comment_hint"), text: $text, axis: .vertical)
                .textFieldStyle(.roundedBorder)
                .lineLimit(1...4)
            HStack {
                Text("\(scalars.formatted())/\(PortableText.maxCommentScalars.formatted())")
                    .font(.caption2)
                    .foregroundStyle(scalars > PortableText.maxCommentScalars ? AbastevoTheme.errorRed : Color.secondary)
                Spacer()
                Button(String(localized: "android.forum_comment_publish")) {
                    publish(fuel: fuel)
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.ink)
                .disabled(!PortableText.isValidComment(text))
            }
            if viewModel.currentAccountIdOrNil == nil {
                Text(String(localized: "community.comment_draft"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let notice {
                Text(notice)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            ForEach(queued) { item in
                CommunityOutboxRow(viewModel: viewModel, item: item)
            }
            if !legacy.isEmpty {
                Text(String(localized: "community.old_drafts"))
                    .font(.caption.bold())
                ForEach(legacy) { comment in
                    Text(comment.text)
                        .font(.subheadline)
                }
            }
        }
    }

    private func publish(fuel: FuelProduct) {
        switch viewModel.publishStationComment(stationId: station.id, product: fuel, text: text) {
        case .success(let item):
            text = ""
            notice = IdentityCoordinator.statusLabel(for: item)
            IosHaptics.success()
        case .failure(let error):
            notice = error.localizedDescription
            IosHaptics.warning()
        }
    }
}

/// Valid/invalid vote on a published station comment (`POST /v1/feedback/comments/{id}/votes`).
public struct StationCommentVoteBar: View {
    @ObservedObject var viewModel: AppStateViewModel
    let commentId: String
    @State private var tally: StationCommentTally?
    @State private var notice: String?
    @State private var isVoting = false

    public init(viewModel: AppStateViewModel, commentId: String) {
        self.viewModel = viewModel
        self.commentId = commentId
    }

    public var body: some View {
        HStack(spacing: 12) {
            Button {
                vote(valid: true)
            } label: {
                Label(validLabel, systemImage: "checkmark.circle")
            }
            Button {
                vote(valid: false)
            } label: {
                Label(invalidLabel, systemImage: "xmark.circle")
            }
            if let notice {
                Text(notice)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .font(.caption)
        .buttonStyle(.borderless)
        .disabled(isVoting || viewModel.currentAccountIdOrNil == nil)
    }

    private var validLabel: String {
        guard let tally else { return String(localized: "android.station_feedback_vote_valid") }
        let count = tally.valid.formatted()
        return String(localized: "community.vote_valid_count", defaultValue: "Helpful (\(count))")
    }

    private var invalidLabel: String {
        guard let tally else { return String(localized: "android.station_feedback_vote_invalid") }
        let count = tally.invalid.formatted()
        return String(localized: "community.vote_invalid_count", defaultValue: "Not helpful (\(count))")
    }

    private func vote(valid: Bool) {
        isVoting = true
        Task {
            let result = await viewModel.identity.voteStationComment(commentId: commentId, valid: valid)
            isVoting = false
            switch result {
            case .success(let updated):
                tally = updated
                notice = nil
            case .failure(let error):
                notice = error.localizedDescription
            }
        }
    }
}

/// Forum comment composer for one post (account session plus signed device proof; enters moderation).
public struct ForumCommentComposer: View {
    @ObservedObject var viewModel: AppStateViewModel
    let postId: String
    @Binding var replyingTo: ForumPublicComment?
    @FocusState private var composerFocused: Bool
    @State private var text = ""
    @State private var notice: String?

    public init(
        viewModel: AppStateViewModel, postId: String, replyingTo: Binding<ForumPublicComment?> = .constant(nil)
    ) {
        self.viewModel = viewModel
        self.postId = postId
        _replyingTo = replyingTo
    }

    public var body: some View {
        let scalars = PortableText.countScalars(PortableText.normalize(text))
        let mine = viewModel.identity.items(of: .forumComment).filter { $0.text?.postId == postId }
        Section(String(localized: "android.forum_action_comment")) {
            if let parent = replyingTo {
                Text(String(localized: "android.forum_replying_to", defaultValue: "Replying to \(parent.publicAlias)"))
                    .font(.caption)
                Button(String(localized: "android.forum_cancel_reply")) { replyingTo = nil }
                    .buttonStyle(.borderless)
            }
            TextField(String(localized: "android.forum_comment_composer_hint"), text: $text, axis: .vertical)
                .lineLimit(2...5)
                .focused($composerFocused)
                .onChange(of: replyingTo?.id) { _, id in
                    if id != nil { composerFocused = true }
                }
            HStack {
                Text("\(scalars.formatted())/\(PortableText.maxCommentScalars.formatted())")
                    .font(.caption2)
                    .foregroundStyle(scalars > PortableText.maxCommentScalars ? AbastevoTheme.errorRed : Color.secondary)
                Spacer()
                Button(String(localized: "android.forum_comment_publish")) {
                    publish()
                }
                .disabled(!PortableText.isValidComment(text))
            }
            if viewModel.currentAccountIdOrNil == nil {
                Text(String(localized: "community.comment_draft"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let notice {
                Text(notice)
                    .font(.caption)
            }
            ForEach(mine) { item in
                CommunityOutboxRow(viewModel: viewModel, item: item)
            }
        }
        .onChange(of: viewModel.forumScreenScope) { _, _ in
            text = ""
            notice = nil
            replyingTo = nil
            composerFocused = false
        }
    }

    private func publish() {
        switch viewModel.publishForumComment(postId: postId, text: text, parentId: replyingTo?.id) {
        case .success(let item):
            text = ""
            replyingTo = nil
            notice = IdentityCoordinator.statusLabel(for: item)
        case .failure(let error):
            notice = error.localizedDescription
        }
    }
}

/// Private owner history read with this device's signed proof.
public struct OwnObservationHistorySection: View {
    @ObservedObject var viewModel: AppStateViewModel

    public init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        Section(String(localized: "community.owner_history")) {
            switch viewModel.identity.ownHistoryState {
            case .loading:
                ProgressView()
            case .unavailable:
                Text(String(localized: "community.owner_history_prompt"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            case .loaded:
                if viewModel.identity.ownObservations.isEmpty {
                    Text(String(localized: "community.owner_history_empty"))
                        .foregroundStyle(.secondary)
                }
                ForEach(viewModel.identity.ownObservations) { observation in
                    VStack(alignment: .leading, spacing: 3) {
                        let fuel = observation.product?.localizedName
                            ?? String(localized: "community.unknown_fuel")
                        Text("\(fuel) · \(AbastevoTheme.formatBrlMilli(observation.amountMilli))")
                            .font(.subheadline.weight(.semibold))
                        Text(IdentityCoordinator.serverStateLabel(observation.validationState))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Text(observation.receivedAt)
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            if let notice = viewModel.identity.identityNotice {
                Text(notice)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Button(String(localized: "community.owner_history_load")) {
                Task {
                    await viewModel.identity.refreshOwnHistory()
                }
            }
            .disabled(viewModel.isOfflineSimulation || viewModel.identity.ownHistoryState == .loading)
        }
    }
}
