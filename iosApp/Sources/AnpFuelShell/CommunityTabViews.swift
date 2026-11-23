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
                    Text("Contribuir preço com foto")
                        .font(.headline)
                    Text("Fotografe a placa do posto. O OCR sugere os valores no dispositivo e a foto local expira em 24h.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Button {
                IosHaptics.lightTap()
                viewModel.activeSheet = .capturePrice
            } label: {
                Text(IosStrings.contributePriceCta(viewModel.currentLocale))
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
                Text("Abrir Fórum (\(viewModel.snapshot.forumTopics.count) tópicos)")
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
                Label("Minhas contribuições (\(viewModel.snapshot.myContributions.count))", systemImage: "tray.full.fill")
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
                Text("Feed de Preços — \(viewModel.selectedMunicipality.name)")
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
                    Button("Seja o primeiro a enviar uma foto") {
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
                        HStack(spacing: 12) {
                            Button {
                                IosHaptics.lightTap()
                                viewModel.confirmObservation(obs)
                            } label: {
                                Label("Confirmar (\(obs.confirmations))", systemImage: "hand.thumbsup.fill")
                                    .font(.caption.bold())
                            }
                            .buttonStyle(.bordered)

                            Button {
                                viewModel.markObservationHelpful(obs, isHelpful: true)
                            } label: {
                                Label("Útil (\(obs.helpfulCount))", systemImage: "checkmark.circle")
                                    .font(.caption)
                            }
                            .buttonStyle(.borderless)
                        }
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
    @State private var showCameraPicker = false
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
                            showCameraPicker = true
                        } label: {
                            Label("Câmera", systemImage: "camera.fill")
                        }
                        .buttonStyle(.bordered)
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
                                    await processPhotoBytes(data, mime: "image/jpeg")
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
                        Text("Foto pronta (\(data.count / 1024) KiB) · Enquadramento zoom \(String(format: "%.1fx", zoomState.ratio)) · Expira localmente em 24h")
                            .font(.caption)
                            .foregroundStyle(AbastevoTheme.accentLight)
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

                Section("4. Verificação de Localização (One-shot)") {
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
                            rawPhotoData: rawPhotoData,
                            photoMime: photoMime
                        ) {
                        case .success(let entry):
                            IosHaptics.success()
                            statusBanner = "Registrado: \(entry.statusLabel)"
                            dismiss()
                        case .failure(let errorMsg):
                            IosHaptics.warning()
                            statusBanner = errorMsg
                        }
                    } label: {
                        Text("Enviar preço para validação")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                }
            }
            .navigationTitle("Contribuir Preço")
            .onAppear {
                if selectedStationId.isEmpty, let first = cityStations.first {
                    selectedStationId = first.id
                }
                selectedFuel = viewModel.snapshot.selectedFuel
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
            #if os(iOS) && canImport(UIKit)
            .sheet(isPresented: $showCameraPicker) {
                IosCameraCaptureView { imageData in
                    Task { await processPhotoBytes(imageData, mime: "image/jpeg") }
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

    private func processPhotoBytes(_ data: Data, mime: String) async {
        self.rawPhotoData = data
        self.photoMime = mime
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
    @State private var selectedCategorySlug: String = "all"
    @State private var showNewTopic = false

    public var body: some View {
        let blockedIds = Set(viewModel.snapshot.blockedUsers.map { $0.id })
        let topics = viewModel.snapshot.forumTopics.filter { topic in
            !blockedIds.contains(topic.authorId)
                && (selectedCategorySlug == "all" || topic.categorySlug == selectedCategorySlug)
        }
        NavigationStack {
            VStack(spacing: 10) {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        Button("Todas") { selectedCategorySlug = "all" }
                            .buttonStyle(.bordered)
                            .tint(selectedCategorySlug == "all" ? AbastevoTheme.accentLight : .secondary)
                        ForEach(SeedCatalog.forumCategories) { cat in
                            Button(cat.title) { selectedCategorySlug = cat.slug }
                                .buttonStyle(.bordered)
                                .tint(selectedCategorySlug == cat.slug ? AbastevoTheme.accentLight : .secondary)
                        }
                    }
                    .padding(.horizontal, 16)
                }

                List(topics) { topic in
                    NavigationLink {
                        ForumPostDetailView(viewModel: viewModel, topic: topic)
                    } label: {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Text(topic.kind == .editorial ? "Guia Oficial" : "Comunidade")
                                    .font(.caption2.bold())
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background((topic.kind == .editorial ? Color.blue : AbastevoTheme.accentLight).opacity(0.15))
                                    .clipShape(Capsule())
                                Text(topic.regionLabel).font(.caption2).foregroundStyle(.secondary)
                                Spacer()
                                Text(topic.createdAtLabel).font(.caption2).foregroundStyle(.secondary)
                            }
                            Text(topic.title).font(.headline)
                            Text(topic.summary).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                            HStack(spacing: 14) {
                                Label("\(topic.reactionCount)", systemImage: "hand.thumbsup")
                                Label("\(topic.comments.count)", systemImage: "bubble.right")
                            }
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
            .navigationTitle("Fórum Abastevo")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
                ToolbarItem(placement: .primaryAction) {
                    Button {
                        showNewTopic = true
                    } label: {
                        Image(systemName: "square.and.pencil")
                    }
                }
            }
            .sheet(isPresented: $showNewTopic) {
                ForumNewTopicSheetView(viewModel: viewModel)
            }
        }
    }
}

public struct ForumPostDetailView: View {
    @ObservedObject var viewModel: AppStateViewModel
    let topic: ForumTopicItem
    @State private var replyText = ""

    public var body: some View {
        let liveTopic = viewModel.snapshot.forumTopics.first(where: { $0.id == topic.id }) ?? topic
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                EnergiaCard {
                    Text(liveTopic.title).font(.title3.bold())
                    Text("\(liveTopic.authorName) · \(liveTopic.regionLabel) · \(liveTopic.createdAtLabel)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(liveTopic.body).font(.body)
                    if let urlStr = liveTopic.externalReferenceUrl,
                       ForumSafeLinks.isAllowedHttpsLink(urlStr),
                       let url = URL(string: urlStr) {
                        Link("Abrir guia completo (HTTPS)", destination: url)
                            .font(.subheadline.bold())
                    }
                    HStack(spacing: 12) {
                        Button {
                            viewModel.toggleForumReaction(topicId: liveTopic.id)
                        } label: {
                            Label("Útil (\(liveTopic.reactionCount))", systemImage: liveTopic.myReacted ? "hand.thumbsup.fill" : "hand.thumbsup")
                        }
                        .buttonStyle(.bordered)

                        if liveTopic.kind == .community {
                            Button("Bloquear autor") {
                                viewModel.blockUser(userId: liveTopic.authorId, alias: liveTopic.authorName)
                            }
                            .font(.caption)
                            .foregroundStyle(.red)
                        }
                    }
                }

                EnergiaCard {
                    Text("Comentários (\(liveTopic.comments.count))").font(.headline)
                    HStack {
                        TextField("Adicionar comentário (até 280 caracteres)...", text: $replyText)
                            .textFieldStyle(.roundedBorder)
                        Button("Enviar") {
                            viewModel.addForumComment(topicId: liveTopic.id, text: replyText)
                            replyText = ""
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                    }
                    ForEach(liveTopic.comments) { c in
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(c.authorName).font(.caption.bold())
                                Spacer()
                                Text(c.createdAtLabel).font(.caption2).foregroundStyle(.secondary)
                            }
                            Text(c.body).font(.subheadline)
                            Divider()
                        }
                    }
                }
            }
            .padding(16)
        }
        .navigationTitle("Tópico")
    }
}

public struct ForumNewTopicSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var categorySlug = "dicas-regionais"
    @State private var title = ""
    @State private var bodyText = ""
    @State private var refUrl = ""
    @State private var errorText: String?

    public var body: some View {
        NavigationStack {
            Form {
                Picker("Categoria", selection: $categorySlug) {
                    ForEach(SeedCatalog.forumCategories) { cat in
                        Text(cat.title).tag(cat.slug)
                    }
                }
                TextField("Título do tópico", text: $title)
                TextField("Descrição", text: $bodyText, axis: .vertical)
                    .lineLimit(4...8)
                TextField("Link HTTPS opcional", text: $refUrl)
                if let err = errorText {
                    Text(err).foregroundStyle(.red).font(.caption)
                }
                Button("Publicar tópico") {
                    switch viewModel.createForumTopic(categorySlug: categorySlug, title: title, body: bodyText, referenceUrl: refUrl) {
                    case .success:
                        dismiss()
                    case .failure(let msg):
                        errorText = msg
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.accentLight)
            }
            .navigationTitle("Novo Tópico")
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

    public var body: some View {
        NavigationStack {
            List(SeedCatalog.contributors) { c in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(c.alias).font(.headline)
                        Text("\(c.cityLabel) · \(c.badgeTitle)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    VStack(alignment: .trailing) {
                        Text("\(c.acceptedPrices) preços").font(.subheadline.bold())
                        Text("\(c.confirmationsGiven) confirmações").font(.caption2).foregroundStyle(.secondary)
                    }
                }
            }
            .navigationTitle("Contribuidores")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct MyContributionsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            List {
                if viewModel.snapshot.myContributions.isEmpty {
                    Text("Você ainda não enviou contribuições neste dispositivo.")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(viewModel.snapshot.myContributions) { item in
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(item.stationName).font(.headline)
                                Spacer()
                                Text(AbastevoTheme.formatBrlMilli(item.amountMilli))
                                    .font(.headline)
                            }
                            Text("\(item.product.displayNamePtBr) · \(item.condition.labelPtBr)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            Text("Status: \(item.statusLabel) · GPS: \(item.locationVerdict) · \(item.createdAtLabel)")
                                .font(.caption2)
                                .foregroundStyle(AbastevoTheme.accentLight)
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
            .navigationTitle("Minhas Contribuições")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}
