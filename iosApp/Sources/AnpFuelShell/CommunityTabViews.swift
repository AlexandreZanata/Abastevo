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
                    Text(String(localized: "android.home_hero_contribute"))
                        .font(.headline)
                    Text(String(localized: "community.capture_intro"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Button {
                IosHaptics.lightTap()
                viewModel.activeSheet = .capturePrice
            } label: {
                Text(String(localized: "android.capture_take_photo"))
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
                    Text(String(localized: "android.forum_card_title"))
                        .font(.headline)
                    Text(String(localized: "community.forum_summary"))
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
                Text(String(localized: "android.forum_card_cta"))
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
                Label(String(localized: "community.my_contributions_count", defaultValue: "My contributions (\(viewModel.identity.items(of: .priceObservation).count.formatted()))"), systemImage: "tray.full.fill")
                    .font(.caption.bold())
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)

            Button {
                viewModel.activeSheet = .contributors
            } label: {
                Label(String(localized: "community.contributors"), systemImage: "person.2.fill")
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
                Text(String(localized: "community.price_feed", defaultValue: "Price feed — \(viewModel.municipalityLabel)"))
                    .font(.headline)
                Spacer()
                Picker(String(localized: "community.sort_order"), selection: $viewModel.feedSortOrder) {
                    ForEach(FeedSortOrder.allCases) { order in
                        Text(order.localizedLabel).tag(order)
                    }
                }
                .pickerStyle(.menu)
            }

            if items.isEmpty {
                EnergiaCard {
                    Text(String(localized: "community.fuel_empty", defaultValue: "No community observations for \(viewModel.snapshot.selectedFuel.localizedName) yet."))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    Button(String(localized: "android.home_hero_contribute")) {
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
                                Text("\(obs.product.localizedName) · \(obs.condition.localizedLabel)")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                HStack(spacing: 8) {
                                    Text(obs.observedAtLabel).font(.caption2).foregroundStyle(.secondary)
                                    if obs.hasPhotoEvidence {
                                        Label(String(localized: "community.with_photo"), systemImage: "photo")
                                            .font(.caption2)
                                            .foregroundStyle(AbastevoTheme.accentLight)
                                    }
                                }
                            }
                            Spacer()
                            Text(AbastevoTheme.formatBrlMilli(obs.amountMilli))
                                .font(.title3.bold())
                        }
                        Label(String(localized: "community.confirmation_count", defaultValue: "Confirmations: \(obs.confirmations.formatted())"), systemImage: "hand.thumbsup")
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
                Section(String(localized: "capture.station_fuel")) {
                    Picker(String(localized: "capture.station"), selection: $selectedStationId) {
                        ForEach(cityStations) { st in
                            Text(st.name).tag(st.id)
                        }
                    }
                    Picker(String(localized: "android.map_filter_fuel"), selection: $selectedFuel) {
                        ForEach(FuelProduct.allCases) { fuel in
                            Text(fuel.localizedName).tag(fuel)
                        }
                    }
                    Picker(String(localized: "capture.condition"), selection: $selectedCondition) {
                        ForEach(PriceConditionKind.allCases) { cond in
                            Text(cond.localizedLabel).tag(cond)
                        }
                    }
                }

                Section(String(localized: "capture.board_photo")) {
                    HStack(spacing: 10) {
                        #if os(iOS)
                        Button {
                            openEvidenceCamera()
                        } label: {
                            Label(isAuthorizingCamera ? String(localized: "capture.authorizing") : String(localized: "capture.camera"), systemImage: "camera.fill")
                        }
                        .buttonStyle(.bordered)
                        .disabled(isAuthorizingCamera || selectedStationId.isEmpty)
                        #endif

                        #if canImport(PhotosUI)
                        PhotosPicker(selection: $selectedPhotoItem, matching: .images) {
                            Label(String(localized: "capture.gallery"), systemImage: "photo.on.rectangle")
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

                        Button(String(localized: "capture.simulate_ocr")) {
                            let sampleBoard = "GASOLINA COMUM R$ 5,899\nETANOL R$ 3,890\nDIESEL S10 R$ 6,149"
                            runTextOcr(sampleBoard)
                        }
                        .buttonStyle(.bordered)
                    }

                    if let data = rawPhotoData {
                        if photoIsEvidence {
                            Text(String(localized: "capture.evidence_photo", defaultValue: "Photo authorized as evidence (\((data.count / 1024).formatted()) KiB). It will be compressed without metadata and expires after 24 hours."))
                                .font(.caption)
                                .foregroundStyle(AbastevoTheme.accentLight)
                        } else {
                            Text(String(localized: "capture.local_photo"))
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
                        Text(String(localized: "capture.candidates"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack {
                                ForEach(ocrCandidates) { cand in
                                    Button(AbastevoTheme.formatBrlMilli(cand.priceMilli)) {
                                        priceInputText = priceInputValue(cand.priceMilli)
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

                Section(String(localized: "capture.observed_price")) {
                    TextField(String(localized: "capture.price_example", defaultValue: "For example: \(priceInputValue(5_899))"), text: $priceInputText)
                        #if os(iOS)
                        .keyboardType(.decimalPad)
                        #endif
                    let code = PortableMoney.code(of: PortableMoney.canonicalUserInput(priceInputText))
                    Text(priceValidationLabel(code))
                        .font(.caption)
                        .foregroundStyle(code == "ok" ? AbastevoTheme.accentLight : .secondary)
                }

                Section(String(localized: "capture.location_check")) {
                    Button {
                        Task { await viewModel.requestOneShotDeviceLocation() }
                    } label: {
                        Label(String(localized: "capture.location_request"), systemImage: "location.fill")
                    }
                    if let risk = viewModel.lastLocationRisk {
                        Text(String(localized: "capture.location_status", defaultValue: "Status: \(locationLabel(risk.verdict)) · Accuracy: \(locationLabel(risk.accuracy)) · Freshness: \(locationLabel(risk.freshness))"))
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
                        Text(String(localized: "capture.submit"))
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                    .disabled(draftSaved || cityStations.isEmpty)
                }
            }
            .navigationTitle(String(localized: "android.capture_screen_title"))
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
                    Button(String(localized: "android.feed_close")) { dismiss() }
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

    private func priceInputValue(_ milli: Int64) -> String {
        (Decimal(milli) / 1_000).formatted(.number.precision(.fractionLength(3)).grouping(.never))
    }

    private func priceValidationLabel(_ code: String) -> String {
        switch code {
        case "ok": return String(localized: "capture.valid_price")
        case "missing-price": return String(localized: "capture.money_missing")
        case "negative-price", "zero-price": return String(localized: "capture.money_positive")
        case "over-precision": return String(localized: "capture.money_precision")
        case "over-range": return String(localized: "capture.money_range")
        default: return String(localized: "capture.money_invalid")
        }
    }

    private func locationLabel(_ code: String) -> String {
        switch code {
        case PortableLocation.verified: return String(localized: "capture.verdict_verified")
        case PortableLocation.degraded: return String(localized: "capture.verdict_degraded")
        case PortableLocation.manual: return String(localized: "capture.verdict_manual")
        case PortableLocation.denied: return String(localized: "capture.verdict_denied")
        case PortableLocation.simulated: return String(localized: "capture.verdict_simulated")
        case PortableLocation.accuracyAccurate: return String(localized: "capture.accuracy_accurate")
        case PortableLocation.accuracyCoarse: return String(localized: "capture.accuracy_coarse")
        case PortableLocation.freshnessFresh: return String(localized: "capture.freshness_fresh")
        case PortableLocation.freshnessStale: return String(localized: "capture.freshness_stale")
        default: return String(localized: "capture.location_unknown")
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
            let names = result.rows.map { "\(FuelProduct.fromOcr($0.product).localizedName): \(AbastevoTheme.formatBrlMilli($0.amountMilli))" }
            ocrAssociationSummary = String(localized: "capture.board_association", defaultValue: "Prices recognized on the board: \(names.joined(separator: " | "))")
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
                Section(String(localized: "forum.posts")) {
                    Picker(String(localized: "android.forum_topic_category_label"), selection: $selectedCategory) {
                        Text(String(localized: "forum.all_categories")).tag(Optional<String>.none)
                        ForEach(categories) { category in
                            Text(category.name).tag(Optional(category.code))
                        }
                    }
                    switch readState {
                    case .loading:
                        ProgressView()
                    case .unavailable:
                        Text(String(localized: "android.forum_unavailable_title")).foregroundStyle(.secondary)
                    case .loaded:
                        if posts.isEmpty {
                            Text(String(localized: "android.forum_feed_empty_hint")).foregroundStyle(.secondary)
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
                    Section(String(localized: "forum.my_topics")) {
                        ForEach(myTopics) { item in
                            CommunityOutboxRow(viewModel: viewModel, item: item)
                        }
                    }
                }
                if !viewModel.snapshot.forumTopics.isEmpty {
                    Section(String(localized: "forum.local_drafts")) {
                        ForEach(viewModel.snapshot.forumTopics) { draft in
                            NavigationLink(draft.title.isEmpty ? String(localized: "forum.comment_drafts") : draft.title) {
                                List {
                                    if !draft.body.isEmpty {
                                        Text(draft.body)
                                    }
                                    ForEach(draft.comments.filter(\.isOwn)) { comment in
                                        Text(comment.body)
                                    }
                                }
                                .navigationTitle(String(localized: "forum.local_draft"))
                            }
                        }
                    }
                }
            }
            .navigationTitle(String(localized: "android.forum_screen_title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
                if !categories.isEmpty {
                    ToolbarItem(placement: .primaryAction) {
                        Button(String(localized: "android.forum_new_topic"), systemImage: "square.and.pencil") { showNewDraft = true }
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
        case .editorial: return String(localized: "android.forum_kind_editorial")
        case .news: return String(localized: "android.forum_kind_news")
        case .topic: return String(localized: "android.source_badge_community")
        case .bulletin: return String(localized: "android.forum_kind_bulletin")
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
                        Label(detail.likeCount.formatted(), systemImage: "hand.thumbsup")
                        Label(detail.dislikeCount.formatted(), systemImage: "hand.thumbsdown")
                    }
                    .foregroundStyle(.secondary)
                } else if isLoadingDetail {
                    ProgressView()
                } else {
                    Text(String(localized: "android.forum_post_not_found_hint")).foregroundStyle(.secondary)
                }
            }
            Section(String(localized: "android.forum_comments_title")) {
                if let comments {
                    if comments.isEmpty {
                        Text(String(localized: "forum.no_comments")).foregroundStyle(.secondary)
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
                    Text(String(localized: "forum.comments_unavailable")).foregroundStyle(.secondary)
                }
            }
            ForumCommentComposer(viewModel: viewModel, postId: post.id)
        }
        .navigationTitle(String(localized: "android.forum_detail_title"))
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
                Picker(String(localized: "android.forum_topic_category_label"), selection: $categoryCode) {
                    ForEach(categories) { category in
                        Text(category.name).tag(category.code)
                    }
                }
                TextField(String(localized: "forum.title_hint"), text: $title)
                TextField(String(localized: "forum.body_hint"), text: $bodyText, axis: .vertical)
                    .lineLimit(4...8)
                Text(String(localized: "forum.moderation_notice"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                if viewModel.currentAccountIdOrNil == nil {
                    Text(String(localized: "forum.topic_draft"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let errorText {
                    Text(errorText).foregroundStyle(.red)
                }
                Button(String(localized: "android.forum_topic_publish")) {
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
            .navigationTitle(String(localized: "android.forum_new_topic"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.action_cancel")) { dismiss() }
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
                    Text(String(localized: "community.contributors_unavailable")).foregroundStyle(.secondary)
                case .loaded:
                    if contributors.isEmpty {
                        Text(String(localized: "community.no_profiles")).foregroundStyle(.secondary)
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
            .navigationTitle(String(localized: "community.contributors"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
                Section(String(localized: "community.device_submissions")) {
                    if items.isEmpty {
                        Text(String(localized: "community.device_submissions_empty"))
                            .foregroundStyle(.secondary)
                    }
                    ForEach(items) { item in
                        CommunityOutboxRow(viewModel: viewModel, item: item)
                    }
                }
                OwnObservationHistorySection(viewModel: viewModel)
                if !viewModel.snapshot.myContributions.isEmpty {
                    Section(String(localized: "community.old_drafts")) {
                        ForEach(viewModel.snapshot.myContributions) { entry in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(entry.stationName).font(.headline)
                                Text("\(entry.product.localizedName) · \(AbastevoTheme.formatBrlMilli(entry.amountMilli))")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle(String(localized: "android.profile_contributions_section"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
            .task {
                viewModel.identity.dispatchPending()
                await viewModel.identity.refreshAcceptedStatuses()
            }
        }
    }
}
