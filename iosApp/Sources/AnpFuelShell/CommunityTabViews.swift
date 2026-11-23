import SwiftUI
import AnpFuelCore
#if canImport(PhotosUI)
import PhotosUI
#endif
#if canImport(Vision)
import Vision
#endif
#if os(iOS) && canImport(UIKit)
import UIKit
#endif

/// Community Tab + Photo Capture & OCR + Forum + Contributors + My Contributions.
public struct CommunityRootView: View {
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack(path: $viewModel.communityPath) {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    // B-BR-FOR-01: Contributing a price always comes first, forum second.
                    contributePriceCard
                        .padding(.horizontal, 16)

                    forumCard
                        .padding(.horizontal, 16)

                    shortcutsRow
                        .padding(.horizontal, 16)

                    FuelProductChipsBar(
                        selectedFuel: viewModel.snapshot.selectedFuel,
                        locale: viewModel.currentLocale,
                        onSelect: { viewModel.selectFuel($0) }
                    )

                    feedSection
                        .padding(.horizontal, 16)
                }
                .padding(.vertical, 12)
            }
            .refreshable {
                viewModel.reloadCityData()
            }
        }
    }

    private var contributePriceCard: some View {
        EnergiaCard {
            HStack(spacing: 12) {
                Image(systemName: "camera.viewfinder")
                    .font(.system(size: 32))
                    .foregroundStyle(AbastevoTheme.accentLight)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Contribuir com um preço")
                        .font(.headline)
                    Text("O OCR sugere os valores no dispositivo. O envio é assinado por este aparelho e aguarda na fila quando você está sem conexão.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Button {
                IosHaptics.lightTap()
                viewModel.activeSheet = .capturePrice
            } label: {
                Text("Abrir captura")
                    .font(.subheadline.bold())
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 10)
            }
            .buttonStyle(.borderedProminent)
            .tint(AbastevoTheme.ink)
        }
    }

    private var forumCard: some View {
        EnergiaCard {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Fórum da Comunidade & Guias")
                        .font(.headline)
                    Text("Dicas regionais, guias da ANP, qualidade de combustível e mobilidade elétrica.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Image(systemName: "bubble.left.and.bubble.right.fill")
                    .font(.title2)
                    .foregroundStyle(.blue)
            }
            Button {
                viewModel.activeSheet = .forumFeed
            } label: {
                Text("Abrir Fórum")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
        }
    }

    private var shortcutsRow: some View {
        HStack(spacing: 10) {
            Button {
                viewModel.activeSheet = .myContributions
            } label: {
                Label("Minhas contribuições (\(viewModel.identity.items(of: .priceObservation).count))", systemImage: "tray.full.fill")
                    .font(.caption.bold())
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)

            Button {
                viewModel.activeSheet = .contributors
            } label: {
                Label("Contribuidores", systemImage: "person.2.fill")
                    .font(.caption.bold())
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
        }
    }

    private var feedSection: some View {
        let items = viewModel.visibleCommunityFeed
        return VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Feed de Preços — \(viewModel.municipalityLabel)")
                    .font(.headline)
                Spacer()
                Picker("Ordem", selection: $viewModel.feedSortOrder) {
                    ForEach(FeedSortOrder.allCases) { order in
                        Text(order.labelPtBr).tag(order)
                    }
                }
                .pickerStyle(.menu)
            }

            if items.isEmpty {
                EnergiaCard {
                    Text("Nenhuma observação comunitária para \(viewModel.snapshot.selectedFuel.displayNamePtBr) ainda.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    Button("Contribuir com um preço") {
                        viewModel.activeSheet = .capturePrice
                    }
                    .font(.subheadline.bold())
                }
            } else {
                ForEach(items) { obs in
                    EnergiaCard {
                        HStack(alignment: .top) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(obs.stationName).font(.headline)
                                Text("\(obs.product.displayNamePtBr) · \(obs.condition.labelPtBr)")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                HStack(spacing: 8) {
                                    Text(obs.observedAtLabel).font(.caption2).foregroundStyle(.secondary)
                                    if obs.hasPhotoEvidence {
                                        Label("Com foto", systemImage: "photo")
                                            .font(.caption2)
                                            .foregroundStyle(AbastevoTheme.accentLight)
                                    }
                                }
                            }
                            Spacer()
                            Text(AbastevoTheme.formatBrlMilli(obs.amountMilli))
                                .font(.title3.bold())
                        }
                        Label("Confirmações: \(obs.confirmations)", systemImage: "hand.thumbsup")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }
}

public struct CapturePriceSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var selectedStationId: String = ""
    @State private var selectedFuel: FuelProduct = .gasolineRegular
    @State private var selectedCondition: PriceConditionKind = .standard
    @State private var priceInputText: String = ""
    @State private var rawPhotoData: Data?
    @State private var photoMime: String = "image/jpeg"
    @State private var ocrCandidates: [PortablePriceOcr.OcrCandidate] = []
    @State private var ocrAssociationSummary: String?
    @State private var statusBanner: String?
    @State private var draftSaved = false
    @State private var showCameraPicker = false
    @State private var photoGrant: PhotoCaptureGrant?
    @State private var photoCapturedAtMillis: Int64?
    @State private var photoIsEvidence = false
    @State private var isAuthorizingCamera = false
    @State private var zoomState = LiveBoardZoom.State(ratio: 1.0)
    #if canImport(PhotosUI)
    @State private var selectedPhotoItem: PhotosPickerItem?
    #endif

    public var body: some View {
        let cityStations = viewModel.stations
        NavigationStack {
            Form {
                Section("1. Posto e Combustível") {
                    Picker("Posto", selection: $selectedStationId) {
                        ForEach(cityStations) { st in
                            Text(st.name).tag(st.id)
                        }
                    }
                    Picker("Combustível", selection: $selectedFuel) {
                        ForEach(FuelProduct.allCases) { fuel in
                            Text(fuel.displayNamePtBr).tag(fuel)
                        }
                    }
                    Picker("Condição", selection: $selectedCondition) {
                        ForEach(PriceConditionKind.allCases) { cond in
                            Text(cond.labelPtBr).tag(cond)
                        }
                    }
                }

                Section("2. Foto da Placa (On-device OCR)") {
                    HStack(spacing: 10) {
                        #if os(iOS)
                        Button {
                            openEvidenceCamera()
                        } label: {
                            Label(isAuthorizingCamera ? "Autorizando..." : "Câmera", systemImage: "camera.fill")
                        }
                        .buttonStyle(.bordered)
                        .disabled(isAuthorizingCamera || selectedStationId.isEmpty)
                        #endif

                        #if canImport(PhotosUI)
                        PhotosPicker(selection: $selectedPhotoItem, matching: .images) {
                            Label("Galeria", systemImage: "photo.on.rectangle")
                        }
                        .buttonStyle(.bordered)
                        .onChange(of: selectedPhotoItem) { _, newItem in
                            guard let newItem = newItem else { return }
                            Task {
                                if let data = try? await newItem.loadTransferable(type: Data.self) {
                                    await processPhotoBytes(data, mime: "image/jpeg", fromCamera: false)
                                }
                            }
                        }
                        #endif

                        Button("Simular OCR") {
                            let sampleBoard = "GASOLINA COMUM R$ 5,899\nETANOL R$ 3,890\nDIESEL S10 R$ 6,149"
                            runTextOcr(sampleBoard)
                        }
                        .buttonStyle(.bordered)
                    }

                    if let data = rawPhotoData {
                        if photoIsEvidence {
                            Text("Foto autorizada como evidência (\(data.count / 1024) KiB). Será reduzida, sem metadados, e expira em 24 h.")
                                .font(.caption)
                                .foregroundStyle(AbastevoTheme.accentLight)
                        } else {
                            Text("Foto usada só para a leitura local do preço. Ela não será enviada.")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    if let notice = viewModel.identity.identityNotice, photoGrant == nil, rawPhotoData != nil {
                        Text(notice)
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }

                    if !ocrCandidates.isEmpty {
                        Text("Candidatos encontrados (ordem da placa — selecione ou digite):")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack {
                                ForEach(ocrCandidates) { cand in
                                    Button(AbastevoTheme.formatBrlMilli(cand.priceMilli)) {
                                        if let formatted = try? PortableMoney.format(cand.priceMilli) {
                                            priceInputText = formatted
                                        }
                                    }
                                    .buttonStyle(.bordered)
                                }
                            }
                        }
                    }
                    if let summary = ocrAssociationSummary {
                        Text(summary).font(.caption2).foregroundStyle(.secondary)
                    }
                }

                Section("3. Preço Observado (vírgula decimal)") {
                    TextField("Ex.: 5,899", text: $priceInputText)
                        #if os(iOS)
                        .keyboardType(.decimalPad)
                        #endif
                    let code = PortableMoney.code(of: priceInputText)
                    Text(code == "ok" ? "Valor válido em milli-BRL" : "Validação: \(code)")
                        .font(.caption)
                        .foregroundStyle(code == "ok" ? AbastevoTheme.accentLight : .secondary)
                }

                Section("4. Verificação de Localização (local, não enviada)") {
                    Button {
                        Task { await viewModel.requestOneShotDeviceLocation() }
                    } label: {
                        Label("Validar GPS atual (sem rastreamento contínuo)", systemImage: "location.fill")
                    }
                    if let risk = viewModel.lastLocationRisk {
                        Text("Status: \(risk.verdict) · Precisão: \(risk.accuracy) · Frescor: \(risk.freshness)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }

                if let banner = statusBanner {
                    Section {
                        Text(banner).font(.subheadline.bold())
                    }
                }

                Section {
                    Button {
                        guard let st = cityStations.first(where: { $0.id == selectedStationId }) ?? cityStations.first else {
                            return
                        }
                        switch viewModel.submitPriceContribution(
                            station: st,
                            product: selectedFuel,
                            priceText: priceInputText,
                            condition: selectedCondition,
                            evidencePhoto: photoIsEvidence ? rawPhotoData : nil,
                            photoMime: photoMime,
                            photoCapturedAtMillis: photoCapturedAtMillis,
                            photoGrant: photoGrant
                        ) {
                        case .success(let item):
                            IosHaptics.success()
                            statusBanner = IdentityCoordinator.statusLabel(for: item)
                            rawPhotoData = nil
                            photoIsEvidence = false
                            photoGrant = nil
                            draftSaved = true
                        case .failure(let errorMsg):
                            IosHaptics.warning()
                            statusBanner = errorMsg.localizedDescription
                        }
                    } label: {
                        Text("Enviar contribuição")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                    .disabled(draftSaved || cityStations.isEmpty)
                }
            }
            .navigationTitle("Contribuir Preço")
            .onAppear {
                if selectedStationId.isEmpty, let first = cityStations.first {
                    selectedStationId = first.id
                }
                selectedFuel = viewModel.snapshot.selectedFuel
            }
            .onChange(of: selectedStationId) { _, newStation in
                if let grant = photoGrant, grant.stationId.lowercased() != newStation.lowercased() {
                    photoIsEvidence = false
                }
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
            #if os(iOS) && canImport(UIKit)
            .sheet(isPresented: $showCameraPicker) {
                IosCameraCaptureView { imageData in
                    Task { await processPhotoBytes(imageData, mime: "image/jpeg", fromCamera: true) }
                }
            }
            #endif
        }
    }

    private func runTextOcr(_ text: String) {
        self.ocrCandidates = PortablePriceOcr.parseCandidates(text)
        let lines = text.split(separator: "\n")
        var tokens: [FuelBoardOcr.Token] = []
        for (idx, line) in lines.enumerated() {
            let y = idx * 80
            tokens.append(FuelBoardOcr.Token(text: String(line), left: 20, top: y + 10, right: 600, bottom: y + 60))
        }
        let result = FuelBoardOcr.associate(tokens: tokens)
        if !result.rows.isEmpty {
            let names = result.rows.map { "\(FuelProduct.fromOcr($0.product).displayNamePtBr): \(AbastevoTheme.formatBrlMilli($0.amountMilli))" }
            ocrAssociationSummary = "Associação da placa: \(names.joined(separator: " | "))"
        }
    }

    /// Requests a camera window for evidence first; without it the photo only feeds local OCR.
    private func openEvidenceCamera() {
        #if os(iOS) && canImport(UIKit)
        guard UIImagePickerController.isSourceTypeAvailable(.camera) else {
            photoGrant = nil
            showCameraPicker = true
            return
        }
        #endif
        isAuthorizingCamera = true
        let stationId = selectedStationId
        Task {
            photoGrant = await viewModel.identity.authorizePhotoCapture(stationId: stationId)
            isAuthorizingCamera = false
            showCameraPicker = true
        }
    }

    private func processPhotoBytes(_ data: Data, mime: String, fromCamera: Bool) async {
        self.rawPhotoData = data
        self.photoMime = mime
        let capturedAt = Int64(Date().timeIntervalSince1970 * 1000)
        self.photoCapturedAtMillis = capturedAt
        if fromCamera, let grant = photoGrant {
            self.photoIsEvidence = grant.admits(stationId: selectedStationId, capturedAtMillis: capturedAt)
        } else {
            self.photoIsEvidence = false
        }
        if let board = LiveBoardZoom.Board(left: 0.15, top: 0.20, right: 0.85, bottom: 0.80) {
            self.zoomState = LiveBoardZoom.next(state: zoomState, board: board, minRatio: 1.0, maxRatio: 5.0)
        }
        #if canImport(Vision)
        let recognized = await recognizeTextWithVision(imageData: data)
        if !recognized.isEmpty {
            runTextOcr(recognized)
        }
        #endif
    }

    #if canImport(Vision)
    private func recognizeTextWithVision(imageData: Data) async -> String {
        return await withCheckedContinuation { continuation in
            let request = VNRecognizeTextRequest { req, _ in
                let observations = req.results as? [VNRecognizedTextObservation] ?? []
                let lines = observations.compactMap { $0.topCandidates(1).first?.string }
                continuation.resume(returning: lines.joined(separator: "\n"))
            }
            request.recognitionLanguages = ["pt-BR", "en-US"]
            request.recognitionLevel = .accurate
            let handler = VNImageRequestHandler(data: imageData, options: [:])
            do {
                try handler.perform([request])
            } catch {
                continuation.resume(returning: "")
            }
        }
    }
    #endif
}

#if os(iOS) && canImport(UIKit)
struct IosCameraCaptureView: UIViewControllerRepresentable {
    let onCapture: (Data) -> Void
    @Environment(\.dismiss) private var dismiss

    func makeCoordinator() -> Coordinator {
        Coordinator(onCapture: onCapture, dismiss: dismiss)
    }

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let picker = UIImagePickerController()
        picker.sourceType = UIImagePickerController.isSourceTypeAvailable(.camera) ? .camera : .photoLibrary
        picker.delegate = context.coordinator
        return picker
    }

    func updateUIViewController(_ uiViewController: UIImagePickerController, context: Context) {}

    final class Coordinator: NSObject, UINavigationControllerDelegate, UIImagePickerControllerDelegate {
        let onCapture: (Data) -> Void
        let dismiss: DismissAction

        init(onCapture: @escaping (Data) -> Void, dismiss: DismissAction) {
            self.onCapture = onCapture
            self.dismiss = dismiss
        }

        func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
            if let image = info[.originalImage] as? UIImage,
               let data = image.jpegData(compressionQuality: 0.85) {
                onCapture(data)
            }
            dismiss()
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            dismiss()
        }
    }
}
#endif

public struct ForumFeedSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var selectedCategory: String?
    @State private var showNewDraft = false
    @State private var categories: [ForumPublicCategory] = []
    @State private var posts: [ForumPublicPost] = []
    @State private var readState: PublicReadState = .loading

    public var body: some View {
        NavigationStack {
            List {
                Section("Publicações") {
                    Picker("Categoria", selection: $selectedCategory) {
                        Text("Todas").tag(Optional<String>.none)
                        ForEach(categories) { category in
                            Text(category.name).tag(Optional(category.code))
                        }
                    }
                    switch readState {
                    case .loading:
                        ProgressView()
                    case .unavailable:
                        Text("Não foi possível carregar o fórum.").foregroundStyle(.secondary)
                    case .loaded:
                        if posts.isEmpty {
                            Text("Nenhuma publicação nesta categoria.").foregroundStyle(.secondary)
                        }
                        ForEach(posts) { post in
                            NavigationLink {
                                ForumPostDetailView(viewModel: viewModel, post: post)
                            } label: {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(post.title).font(.headline)
                                    Text(postLabel(post.kind)).font(.caption).foregroundStyle(.secondary)
                                    if let excerpt = post.excerpt {
                                        Text(excerpt).font(.caption).lineLimit(2)
                                    }
                                }
                            }
                        }
                    }
                }
                let myTopics = viewModel.identity.items(of: .forumTopic)
                if !myTopics.isEmpty {
                    Section("Meus tópicos") {
                        ForEach(myTopics) { item in
                            CommunityOutboxRow(viewModel: viewModel, item: item)
                        }
                    }
                }
                if !viewModel.snapshot.forumTopics.isEmpty {
                    Section("Rascunhos antigos neste dispositivo") {
                        ForEach(viewModel.snapshot.forumTopics) { draft in
                            NavigationLink(draft.title.isEmpty ? "Rascunhos de comentários" : draft.title) {
                                List {
                                    if !draft.body.isEmpty {
                                        Text(draft.body)
                                    }
                                    ForEach(draft.comments.filter(\.isOwn)) { comment in
                                        Text(comment.body)
                                    }
                                }
                                .navigationTitle("Rascunho local")
                            }
                        }
                    }
                }
            }
            .navigationTitle("Fórum Abastevo")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
                if !categories.isEmpty {
                    ToolbarItem(placement: .primaryAction) {
                        Button("Novo tópico", systemImage: "square.and.pencil") { showNewDraft = true }
                    }
                }
            }
            .sheet(isPresented: $showNewDraft) {
                ForumNewTopicSheetView(viewModel: viewModel, categories: categories)
            }
            .task {
                guard let client = try? viewModel.socialReadClient(),
                      let loaded = try? await client.fetchForumCategories(),
                      !Task.isCancelled else { return }
                categories = loaded
            }
            .task(id: selectedCategory) {
                posts = []
                readState = .loading
                do {
                    let loaded = try await viewModel.socialReadClient().fetchForumPosts(category: selectedCategory)
                    guard !Task.isCancelled else { return }
                    posts = loaded
                    readState = .loaded
                } catch {
                    guard !Task.isCancelled else { return }
                    readState = .unavailable
                }
            }
        }
    }

    private func postLabel(_ kind: ForumPublicPost.Kind) -> String {
        switch kind {
        case .editorial: return "Guia editorial"
        case .news: return "Notícia"
        case .topic: return "Comunidade"
        case .bulletin: return "Boletim"
        }
    }
}

public struct ForumPostDetailView: View {
    @ObservedObject var viewModel: AppStateViewModel
    let post: ForumPublicPost
    @State private var detail: ForumPublicDetail?
    @State private var comments: [ForumPublicComment]?
    @State private var isLoadingDetail = true
    @State private var isLoadingComments = true

    public var body: some View {
        List {
            Section {
                Text(post.title).font(.title3.bold())
                if let alias = post.publicAlias {
                    Text(alias).font(.caption).foregroundStyle(.secondary)
                }
                if let detail {
                    if let body = detail.bodyText {
                        Text(body)
                    }
                    if let source = detail.source,
                       ForumSafeLinks.isAllowedHttpsLink(source.url),
                       let url = URL(string: source.url) {
                        Link(source.name, destination: url)
                    }
                    HStack(spacing: 12) {
                        Label("\(detail.likeCount)", systemImage: "hand.thumbsup")
                        Label("\(detail.dislikeCount)", systemImage: "hand.thumbsdown")
                    }
                    .foregroundStyle(.secondary)
                } else if isLoadingDetail {
                    ProgressView()
                } else {
                    Text("Não foi possível carregar a publicação.").foregroundStyle(.secondary)
                }
            }
            Section("Comentários publicados") {
                if let comments {
                    if comments.isEmpty {
                        Text("Nenhum comentário publicado.").foregroundStyle(.secondary)
                    }
                    ForEach(comments) { comment in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(comment.publicAlias).font(.caption.bold())
                            Text(comment.bodyText)
                        }
                    }
                } else if isLoadingComments {
                    ProgressView()
                } else {
                    Text("Não foi possível carregar os comentários.").foregroundStyle(.secondary)
                }
            }
            ForumCommentComposer(viewModel: viewModel, postId: post.id)
        }
        .navigationTitle("Publicação")
        .task {
            if let client = try? viewModel.socialReadClient() {
                let loaded = try? await client.fetchForumPost(id: post.id)
                guard !Task.isCancelled else { return }
                detail = loaded
            }
            isLoadingDetail = false
        }
        .task {
            if let client = try? viewModel.socialReadClient() {
                let loaded = try? await client.fetchForumComments(postId: post.id)
                guard !Task.isCancelled else { return }
                comments = loaded
            }
            isLoadingComments = false
        }
    }
}

public struct ForumNewTopicSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let categories: [ForumPublicCategory]
    @State private var categoryCode = ""
    @State private var title = ""
    @State private var bodyText = ""
    @State private var errorText: String?

    public var body: some View {
        NavigationStack {
            Form {
                Picker("Categoria", selection: $categoryCode) {
                    ForEach(categories) { category in
                        Text(category.name).tag(category.code)
                    }
                }
                TextField("Título do tópico (até 120 caracteres)", text: $title)
                TextField("Descrição (até 4000 caracteres)", text: $bodyText, axis: .vertical)
                    .lineLimit(4...8)
                Text("O tópico passa por moderação antes de aparecer no fórum.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                if viewModel.currentAccountIdOrNil == nil {
                    Text("Entre na conta gratuita para publicar. Até lá, o tópico fica como rascunho neste aparelho.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let errorText {
                    Text(errorText).foregroundStyle(.red)
                }
                Button("Publicar tópico") {
                    switch viewModel.publishForumTopic(categoryCode: categoryCode, title: title, body: bodyText) {
                    case .success:
                        dismiss()
                    case .failure(let error):
                        errorText = error.localizedDescription
                    }
                }
                .disabled(categoryCode.isEmpty)
            }
            .onAppear { categoryCode = categories.first?.code ?? "" }
            .navigationTitle("Novo tópico")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancelar") { dismiss() }
                }
            }
        }
    }
}

public struct ContributorsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var contributors: [PublicContributor] = []
    @State private var readState: PublicReadState = .loading

    public var body: some View {
        NavigationStack {
            List {
                switch readState {
                case .loading:
                    ProgressView()
                case .unavailable:
                    Text("Não foi possível carregar os colaboradores.").foregroundStyle(.secondary)
                case .loaded:
                    if contributors.isEmpty {
                        Text("Nenhum perfil público disponível.").foregroundStyle(.secondary)
                    }
                    ForEach(contributors.indices, id: \.self) { index in
                        let contributor = contributors[index]
                        VStack(alignment: .leading, spacing: 3) {
                            Text(contributor.displayName).font(.headline)
                            if let roles = contributor.roles, !roles.isEmpty {
                                Text(roles.joined(separator: " · "))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Contribuidores")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
            .task {
                do {
                    let loaded = try await viewModel.socialReadClient().fetchContributors()
                    guard !Task.isCancelled else { return }
                    contributors = loaded
                    readState = .loaded
                } catch {
                    guard !Task.isCancelled else { return }
                    readState = .unavailable
                }
            }
        }
    }
}

public struct MyContributionsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        let items = viewModel.identity.items(of: .priceObservation)
        NavigationStack {
            List {
                Section("Envios deste aparelho") {
                    if items.isEmpty {
                        Text("Nenhuma contribuição enviada por este aparelho.")
                            .foregroundStyle(.secondary)
                    }
                    ForEach(items) { item in
                        CommunityOutboxRow(viewModel: viewModel, item: item)
                    }
                }
                OwnObservationHistorySection(viewModel: viewModel)
                if !viewModel.snapshot.myContributions.isEmpty {
                    Section("Rascunhos antigos (não enviados)") {
                        ForEach(viewModel.snapshot.myContributions) { entry in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(entry.stationName).font(.headline)
                                Text("\(entry.product.displayNamePtBr) · \(AbastevoTheme.formatBrlMilli(entry.amountMilli))")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Minhas Contribuições")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
            .task {
                viewModel.identity.dispatchPending()
                await viewModel.identity.refreshAcceptedStatuses()
            }
        }
    }
}
