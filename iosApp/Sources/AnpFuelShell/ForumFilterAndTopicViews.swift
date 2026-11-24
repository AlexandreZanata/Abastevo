import SwiftUI
import AnpFuelCore

/// Android `ForumFilterDialog`: optional state, IBGE municipality, fuel and station, plus the publication kind.
/// The category stays on the feed picker; local text search keeps covering loaded entries.
@MainActor
struct ForumFeedFilterSheet: View {
    @Environment(\.dismiss) private var dismiss
    let city: Municipality?
    let onApply: (ForumFeedFilters) -> Void
    @State private var kind: ForumPublicPost.Kind?
    @State private var uf: String?
    @State private var municipality: String
    @State private var fuel: ForumFuelProduct?
    @State private var station: String
    @State private var issue: ForumFilterIssue?

    init(current: ForumFeedFilters, city: Municipality?, onApply: @escaping (ForumFeedFilters) -> Void) {
        self.city = city
        self.onApply = onApply
        _kind = State(initialValue: current.kind)
        _uf = State(initialValue: current.uf)
        _municipality = State(initialValue: current.municipalityIbge.map { String($0) } ?? "")
        _fuel = State(initialValue: current.fuelProduct)
        _station = State(initialValue: current.stationId ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Picker(String(localized: "forum.filter.kind", defaultValue: "Publication type"), selection: $kind) {
                    Text(String(localized: "forum.filter.all_kinds", defaultValue: "All publication types"))
                        .tag(Optional<ForumPublicPost.Kind>.none)
                    ForEach(ForumPublicPost.Kind.allCases, id: \.self) { option in
                        Text(ForumFilterLabels.kind(option)).tag(Optional(option))
                    }
                }
                Picker(String(localized: "android.forum_topic_region_label", defaultValue: "State (UF) (optional)"), selection: $uf) {
                    Text(String(localized: "android.forum_filter_all_ufs", defaultValue: "All states")).tag(Optional<String>.none)
                    ForEach(ForumRegionRules.states, id: \.self) { state in
                        Text(verbatim: state).tag(Optional(state))
                    }
                }
                TextField(String(localized: "android.forum_filter_ibge", defaultValue: "Municipality IBGE code (optional)"), text: $municipality)
                    .keyboardType(.numberPad)
                    .onChange(of: municipality) { _, value in
                        let digits = String(value.filter(\.isASCII).filter(\.isNumber).prefix(7))
                        if digits != value { municipality = digits }
                        issue = nil
                    }
                if let city, ForumRegionRules.statePrefixes[city.state.rawValue] != nil {
                    Button(String(localized: "forum.filter.use_city", defaultValue: "Use the selected city")) {
                        uf = city.state.rawValue
                        municipality = city.ibgeCode
                        issue = nil
                    }
                }
                Picker(String(localized: "android.forum_topic_fuel_label", defaultValue: "Fuel product (optional)"), selection: $fuel) {
                    Text(String(localized: "android.forum_filter_all_fuels", defaultValue: "All fuels")).tag(Optional<ForumFuelProduct>.none)
                    ForEach(ForumFuelProduct.allCases) { option in
                        Text(option.fuel.localizedName).tag(Optional(option))
                    }
                }
                TextField(String(localized: "android.forum_filter_station", defaultValue: "Station ID (optional)"), text: $station)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .onChange(of: station) { _, value in
                        if value.count > 36 { station = String(value.prefix(36)) }
                        issue = nil
                    }
                if let issue {
                    Text(ForumFilterLabels.issue(issue)).foregroundStyle(AbastevoTheme.errorRed)
                }
                Button(String(localized: "android.occurrence_apply")) { apply() }
                Button(String(localized: "android.occurrence_clear"), role: .destructive) {
                    onApply(ForumFeedFilters())
                    dismiss()
                }
            }
            .navigationTitle(String(localized: "android.forum_filters_title", defaultValue: "Filter publications"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.action_cancel")) { dismiss() }
                }
            }
        }
    }

    private func apply() {
        let parsed = ForumFeedFilters.parse(
            category: nil, kind: kind, uf: uf, municipality: municipality, fuelProduct: fuel, station: station
        )
        switch parsed {
        case .success(let filters):
            onApply(filters)
            dismiss()
        case .failure(let failure):
            issue = failure
        }
    }
}

enum ForumFilterLabels {
    static func kind(_ kind: ForumPublicPost.Kind) -> String {
        switch kind {
        case .editorial: return String(localized: "android.forum_kind_editorial")
        case .news: return String(localized: "android.forum_kind_news")
        case .topic: return String(localized: "android.forum_kind_topic", defaultValue: "Topic")
        case .bulletin: return String(localized: "android.forum_kind_bulletin")
        }
    }

    static func issue(_ issue: ForumFilterIssue) -> String {
        switch issue {
        case .state, .municipality:
            return String(localized: "android.forum_filter_invalid_ibge", defaultValue: "Invalid IBGE municipality code")
        case .station:
            return String(localized: "android.forum_filter_invalid_station", defaultValue: "Invalid station ID")
        }
    }
}

/// Android `ForumTopicFormViewModel` in edit mode: the owner changes category, title, text, region and fuel
/// at the revision it last read. The public alias is fixed at creation and the station link is kept.
@MainActor
struct ForumTopicEditSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var discussion: ForumDiscussionModel
    let topic: ForumOwnerTopic
    @State private var categories: [ForumPublicCategory] = []
    @State private var category: String
    @State private var title: String
    @State private var bodyText: String
    @State private var uf: String?
    @State private var municipality: String
    @State private var fuel: ForumFuelProduct?
    @State private var regionInvalid = false

    init(viewModel: AppStateViewModel, discussion: ForumDiscussionModel, topic: ForumOwnerTopic) {
        self.viewModel = viewModel
        self.discussion = discussion
        self.topic = topic
        _category = State(initialValue: topic.draft.category)
        _title = State(initialValue: topic.draft.title)
        _bodyText = State(initialValue: topic.draft.bodyText)
        _uf = State(initialValue: topic.draft.regionUf)
        _municipality = State(initialValue: topic.draft.regionMunicipalityIbge.map { String($0) } ?? "")
        _fuel = State(initialValue: topic.draft.fuelProduct)
    }

    private var draft: ForumTopicDraft? {
        let code: Int?
        do {
            code = try ForumRegionRules.municipality(municipality)
        } catch {
            return nil
        }
        let next = ForumTopicDraft(
            category: category, title: title, bodyText: bodyText, regionUf: uf, regionMunicipalityIbge: code,
            fuelProduct: fuel, stationId: topic.draft.stationId
        )
        return next.isValid ? next : nil
    }

    var body: some View {
        NavigationStack {
            Form {
                Picker(String(localized: "android.forum_topic_category_label"), selection: $category) {
                    if !categories.contains(where: { $0.code == category }) {
                        Text(verbatim: category).tag(category)
                    }
                    ForEach(categories) { option in
                        Text(option.name).tag(option.code)
                    }
                }
                TextField(String(localized: "forum.title_hint"), text: $title)
                Text(verbatim: "\(title.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count.formatted())/\(CommunityWire.maxTopicTitleScalars.formatted())")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                TextField(String(localized: "forum.body_hint"), text: $bodyText, axis: .vertical)
                    .lineLimit(4...10)
                Text(verbatim: "\(bodyText.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count.formatted())/\(CommunityWire.maxTopicBodyScalars.formatted())")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                Picker(String(localized: "android.forum_topic_region_label", defaultValue: "State (UF) (optional)"), selection: $uf) {
                    Text(String(localized: "android.forum_filter_all_ufs", defaultValue: "All states")).tag(Optional<String>.none)
                    ForEach(ForumRegionRules.states, id: \.self) { state in
                        Text(verbatim: state).tag(Optional(state))
                    }
                }
                TextField(String(localized: "android.forum_topic_municipality_label", defaultValue: "City IBGE code (optional)"), text: $municipality)
                    .keyboardType(.numberPad)
                    .onChange(of: municipality) { _, value in
                        let digits = String(value.filter(\.isASCII).filter(\.isNumber).prefix(7))
                        if digits != value { municipality = digits }
                        regionInvalid = false
                    }
                Picker(String(localized: "android.forum_topic_fuel_label", defaultValue: "Fuel product (optional)"), selection: $fuel) {
                    Text(String(localized: "android.forum_filter_all_fuels", defaultValue: "All fuels")).tag(Optional<ForumFuelProduct>.none)
                    ForEach(ForumFuelProduct.allCases) { option in
                        Text(option.fuel.localizedName).tag(Optional(option))
                    }
                }
                if regionInvalid {
                    Text(ForumFilterLabels.issue(.municipality)).foregroundStyle(AbastevoTheme.errorRed)
                }
                Text(String(localized: "forum.moderation_notice"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                if let notice = discussion.actionNotice {
                    Text(notice).foregroundStyle(.secondary)
                }
                if discussion.pendingNeedsReview {
                    Button(String(localized: "android.forum_retry")) {
                        Task { await discussion.refreshAfterConflict(viewModel) }
                    }
                    .disabled(discussion.isLoadingDetail)
                }
                Button(String(localized: "android.forum_edit_save")) { save() }
                    .disabled(discussion.isWriting || discussion.pendingNeedsReview
                              || !CommunityWire.isValidTopic(title: title, body: bodyText))
            }
            .navigationTitle(String(localized: "android.forum_edit_topic", defaultValue: "Edit topic"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.action_cancel")) { dismiss() }
                }
            }
            .task {
                guard categories.isEmpty, let client = try? viewModel.socialReadClient(),
                      let loaded = try? await client.fetchForumCategories(), !Task.isCancelled else { return }
                categories = loaded
            }
        }
    }

    private func save() {
        let code: Int?
        do {
            code = try ForumRegionRules.municipality(municipality)
        } catch {
            regionInvalid = true
            return
        }
        regionInvalid = ForumRegionRules.issue(uf: uf, municipalityIbge: code) != nil
        guard !regionInvalid, let draft else { return }
        Task {
            if await discussion.editTopic(draft, host: viewModel) { dismiss() }
        }
    }
}
