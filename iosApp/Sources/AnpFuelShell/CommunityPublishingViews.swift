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
        Section("Iniciar sessão com a Apple") {
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
            Text("Nome e e-mail não são solicitados à Apple. Ao entrar, a chave deste aparelho é vinculada à sua conta gratuita para publicar.")
                .font(.caption)
                .foregroundStyle(.secondary)
            #else
            Text("Indisponível nesta plataforma.")
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
                    Button("Tentar agora") {
                        viewModel.identity.retryNow(item.id)
                    }
                }
                if item.status == .draft && viewModel.currentAccountIdOrNil == nil {
                    Button("Entrar para publicar") {
                        viewModel.activeSheet = .auth
                    }
                }
                if item.status.isTerminal {
                    Button("Remover da lista", role: .destructive) {
                        viewModel.identity.discard(item.id)
                    }
                } else {
                    Button("Descartar", role: .destructive) {
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
            let photo = price.photo == nil ? "" : " · com foto"
            return "\(price.stationName) · \(price.product.displayNamePtBr) · \(AbastevoTheme.formatBrlMilli(price.amountMilli))\(photo)"
        case .stationComment, .forumComment:
            return item.text?.text ?? ""
        case .forumTopic:
            return item.text?.title ?? ""
        }
    }

    private var statusColor: Color {
        switch item.status {
        case .accepted:
            return AbastevoTheme.accentLight
        case .refused, .expired:
            return AbastevoTheme.errorRed
        case .draft, .pending:
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
            Text("Seu comentário · \(fuel.displayNamePtBr)")
                .font(.headline)
            TextField("Escreva sua experiência (até 280 caracteres)...", text: $text, axis: .vertical)
                .textFieldStyle(.roundedBorder)
                .lineLimit(1...4)
            HStack {
                Text("\(scalars)/\(PortableText.maxCommentScalars)")
                    .font(.caption2)
                    .foregroundStyle(scalars > PortableText.maxCommentScalars ? AbastevoTheme.errorRed : Color.secondary)
                Spacer()
                Button("Publicar") {
                    publish(fuel: fuel)
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.ink)
                .disabled(!PortableText.isValidComment(text))
            }
            if viewModel.currentAccountIdOrNil == nil {
                Text("Entre na conta gratuita para publicar. Até lá, o texto fica como rascunho neste aparelho.")
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
                Text("Rascunhos antigos (não enviados)")
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
        guard let tally else { return String(localized: "Válido") }
        let count = String(tally.valid)
        return String(localized: "Válido (\(count))")
    }

    private var invalidLabel: String {
        guard let tally else { return String(localized: "Inválido") }
        let count = String(tally.invalid)
        return String(localized: "Inválido (\(count))")
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
    @State private var text = ""
    @State private var notice: String?

    public init(viewModel: AppStateViewModel, postId: String) {
        self.viewModel = viewModel
        self.postId = postId
    }

    public var body: some View {
        let scalars = PortableText.countScalars(PortableText.normalize(text))
        let mine = viewModel.identity.items(of: .forumComment).filter { $0.text?.postId == postId }
        Section("Comentar") {
            TextField("Seu comentário (até 280 caracteres)", text: $text, axis: .vertical)
                .lineLimit(2...5)
            HStack {
                Text("\(scalars)/\(PortableText.maxCommentScalars)")
                    .font(.caption2)
                    .foregroundStyle(scalars > PortableText.maxCommentScalars ? AbastevoTheme.errorRed : Color.secondary)
                Spacer()
                Button("Publicar comentário") {
                    publish()
                }
                .disabled(!PortableText.isValidComment(text))
            }
            if viewModel.currentAccountIdOrNil == nil {
                Text("Entre na conta gratuita para publicar. Até lá, o texto fica como rascunho neste aparelho.")
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
    }

    private func publish() {
        switch viewModel.publishForumComment(postId: postId, text: text) {
        case .success(let item):
            text = ""
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
        Section("Histórico no servidor (privado)") {
            switch viewModel.identity.ownHistoryState {
            case .loading:
                ProgressView()
            case .unavailable:
                Text("Carregue as contribuições assinadas por este aparelho.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            case .loaded:
                if viewModel.identity.ownObservations.isEmpty {
                    Text("Nenhuma contribuição registrada no servidor.")
                        .foregroundStyle(.secondary)
                }
                ForEach(viewModel.identity.ownObservations) { observation in
                    VStack(alignment: .leading, spacing: 3) {
                        Text("\(observation.product?.displayNamePtBr ?? observation.fuelWire) · \(AbastevoTheme.formatBrlMilli(observation.amountMilli))")
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
            Button("Carregar histórico do servidor") {
                Task {
                    await viewModel.identity.refreshOwnHistory()
                }
            }
            .disabled(viewModel.isOfflineSimulation || viewModel.identity.ownHistoryState == .loading)
        }
    }
}
