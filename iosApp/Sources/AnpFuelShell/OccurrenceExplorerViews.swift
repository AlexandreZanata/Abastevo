import SwiftUI
import AnpFuelCore

@MainActor
struct OfficialOccurrenceExplorerView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var filter = OccurrenceFilter()
    @State private var showingFilters = false
    @State private var mode = Mode.records
    @State private var records: [OfficialOccurrence] = []
    @State private var stations: [OccurrenceStation] = []
    @State private var coverage: OccurrenceCoverage?
    @State private var stationCoverage: OccurrenceStationCoverage?
    @State private var lease: QualityLease?
    @State private var cursor: String?
    @State private var stale = false
    @State private var loading = false
    @State private var failure: QualityReadError?
    @State private var generation = UUID()
    @State private var loadedRequest: Request?

    private enum Mode: CaseIterable, Hashable {
        case records, stations

        var label: String {
            switch self {
            case .records: return String(localized: "android.occurrence_action_label")
            case .stations: return String(localized: "quality.occurrences.stations")
            }
        }
    }

    private struct Request: Equatable {
        let scope: QualityReadScope
        let offline: Bool
        let mode: Mode
        let filter: OccurrenceFilter
    }

    private var request: Request {
        Request(scope: viewModel.officialQualityScope, offline: viewModel.isOfflineSimulation, mode: mode, filter: filter)
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Picker(String(localized: "android.occurrence_filter_type"), selection: $mode) {
                        ForEach(Mode.allCases, id: \.self) { type in Text(type.label).tag(type) }
                    }
                    .onChange(of: mode) { _, selected in
                        if selected == .stations { filter.subject = nil }
                    }
                    Button(String(localized: "android.occurrence_filter_type")) { showingFilters = true }
                    Text(String(localized: "android.occurrence_no_guilt"))
                        .foregroundStyle(.secondary)
                }
                if loadedRequest == request {
                    currentResults
                }
            }
            .navigationTitle(String(localized: "android.occurrence_title"))
            .toolbar { Button(String(localized: "android.feed_close")) { dismiss() } }
            .task(id: request) { await load() }
            .refreshable { await load() }
            .sheet(isPresented: $showingFilters) {
                OccurrenceFilterView(initial: filter, stationList: mode == .stations) { filter = $0 }
            }
        }
    }

    @ViewBuilder private var currentResults: some View {
        if stale {
            Text(String(localized: "android.occurrence_stale")).foregroundStyle(.secondary)
        }
        if let lease, !lease.edition.isEmpty {
            Text(lease.edition).foregroundStyle(.secondary)
        }
        if let coverage {
            Text(String(localized: "android.occurrence_coverage",
                        defaultValue: "\(coverage.observations) records in \(coverage.cases) documents"))
        }
        if let stationCoverage {
            Text(String(localized: "quality.occurrences.station_coverage",
                        defaultValue: "\(stationCoverage.stations.formatted()) associated stations · \(stationCoverage.records.formatted()) records"))
        }
        if mode == .records {
            occurrenceSection(title: String(localized: "android.occurrence_company_section"), company: true)
            occurrenceSection(title: String(localized: "android.occurrence_global_section"), company: false)
        } else {
            stationSection
        }
        if loading {
            ProgressView()
        } else if let failure {
            Text(failure == .invalid ? String(localized: "android.occurrence_unsupported_filter")
                 : String(localized: "android.occurrence_unavailable"))
            Button(String(localized: "android.occurrence_retry")) { Task { await load(after: cursor) } }
        } else if lease?.available == false {
            Text(String(localized: "android.occurrence_unavailable"))
        } else if records.isEmpty && stations.isEmpty {
            Text(String(localized: "android.occurrence_empty"))
        }
        if let cursor, !loading, failure == nil {
            Button(String(localized: "quality.occurrences.more")) { Task { await load(after: cursor) } }
        }
    }

    @ViewBuilder private func occurrenceSection(title: String, company: Bool) -> some View {
        let items = records.filter { $0.isCompanyShaped == company }.sorted { $0.documentDay > $1.documentDay }
        if !items.isEmpty {
            Section(title) {
                ForEach(items) { item in
                    NavigationLink {
                        OccurrenceRecordView(viewModel: viewModel, id: item.id, revision: lease?.revisionId)
                    } label: {
                        OccurrenceRow(record: item)
                    }
                }
            }
        }
    }

    @ViewBuilder private var stationSection: some View {
        if !stations.isEmpty {
            Section(String(localized: "quality.occurrences.stations")) {
                Text(String(localized: "android.occurrence_company_note"))
                    .font(.footnote)
                ForEach(stations) { station in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(station.displayName).font(.headline)
                        Text([station.municipality, station.state].compactMap { $0 }.joined(separator: ", "))
                            .foregroundStyle(.secondary)
                        Text(String(localized: "quality.occurrences.records_count",
                                    defaultValue: "Records: \(station.recordCount.formatted())"))
                    }
                    .accessibilityElement(children: .combine)
                }
            }
        }
    }

    private func load(after: String? = nil) async {
        let current = request
        if after != nil && (loading || loadedRequest != current) { return }
        let ticket = UUID()
        generation = ticket
        if loadedRequest != current {
            records = []
            stations = []
            coverage = nil
            stationCoverage = nil
            lease = nil
            cursor = nil
            stale = false
        }
        loadedRequest = current
        failure = nil
        loading = true
        defer { if generation == ticket { loading = false } }
        let reader = viewModel.officialQualityReader
        let pinned = after == nil ? nil : lease?.revisionId
        do {
            switch current.mode {
            case .records:
                let page = try await reader.occurrences(filter: current.filter, revision: pinned, cursor: after)
                let counts: OccurrenceCoverage?
                if after == nil && page.lease?.available == true {
                    counts = try? await reader.occurrenceCoverage(revision: page.lease?.revisionId)
                } else {
                    counts = coverage
                }
                guard !Task.isCancelled, generation == ticket, current == request else { return }
                let old = after == nil ? [] : records
                let existing = Set(old.map(\.id))
                records = old + page.items.filter { !existing.contains($0.id) }
                coverage = counts?.lease.revisionId == page.lease?.revisionId ? counts : nil
                cursor = page.nextCursor
                lease = page.lease
                stale = page.stale || (after != nil && stale)
            case .stations:
                let page = try await reader.occurrenceStations(filter: current.filter, revision: pinned, cursor: after)
                let counts: OccurrenceStationCoverage?
                if after == nil && page.lease?.available == true {
                    counts = try? await reader.occurrenceStationCoverage(revision: page.lease?.revisionId)
                } else {
                    counts = stationCoverage
                }
                guard !Task.isCancelled, generation == ticket, current == request else { return }
                let old = after == nil ? [] : stations
                let existing = Set(old.map(\.id))
                stations = old + page.items.filter { !existing.contains($0.id) }
                stationCoverage = counts?.lease.revisionId == page.lease?.revisionId ? counts : nil
                cursor = page.nextCursor
                lease = page.lease
                stale = page.stale || (after != nil && stale)
            }
        } catch {
            guard !Task.isCancelled, generation == ticket, current == request else { return }
            failure = error as? QualityReadError ?? .unavailable
            if failure == .disabled {
                records = []
                stations = []
                coverage = nil
                stationCoverage = nil
                lease = nil
                cursor = nil
            }
            stale = !records.isEmpty || !stations.isEmpty
        }
    }
}

private struct OccurrenceFilterView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var draft: OccurrenceFilter
    @State private var fromDay: String
    @State private var toDay: String
    let stationList: Bool
    let apply: (OccurrenceFilter) -> Void

    init(initial: OccurrenceFilter, stationList: Bool, apply: @escaping (OccurrenceFilter) -> Void) {
        self.stationList = stationList
        self.apply = apply
        _draft = State(initialValue: initial)
        _fromDay = State(initialValue: initial.fromDay ?? "")
        _toDay = State(initialValue: initial.toDay ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Picker(String(localized: "android.occurrence_filter_uf"), selection: $draft.state) {
                    Text(String(localized: "android.occurrence_all")).tag(nil as BrazilianState?)
                    ForEach(BrazilianState.allCases) { state in Text(state.rawValue).tag(Optional(state)) }
                }
                Picker(String(localized: "android.occurrence_filter_segment"), selection: $draft.segment) {
                    Text(String(localized: "android.occurrence_all")).tag(nil as String?)
                    Text(String(localized: "quality.occurrences.fuel_retail"))
                        .tag(Optional("Revenda de Combustíveis"))
                    Text(String(localized: "quality.occurrences.lpg_retail"))
                        .tag(Optional("Revenda de GLP"))
                }
                if !stationList {
                    Picker(String(localized: "android.occurrence_filter_kind"), selection: $draft.subject) {
                        Text(String(localized: "android.occurrence_all")).tag(nil as OccurrenceSubject?)
                        ForEach(OccurrenceSubject.allCases) { subject in
                            Text(subject.label).tag(Optional(subject))
                        }
                    }
                }
                Section(String(localized: "android.occurrence_filter_period")) {
                    TextField(String(localized: "quality.occurrences.from_day"), text: $fromDay)
                    TextField(String(localized: "quality.occurrences.to_day"), text: $toDay)
                }
            }
            .navigationTitle(String(localized: "android.occurrence_filter_type"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.occurrence_clear")) { apply(OccurrenceFilter()); dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "android.occurrence_apply")) {
                        draft.fromDay = fromDay.isEmpty ? nil : fromDay
                        draft.toDay = toDay.isEmpty ? nil : toDay
                        apply(draft)
                        dismiss()
                    }
                }
            }
        }
    }
}

private extension OccurrenceSubject {
    var label: String {
        switch self {
        case .company: return String(localized: "quality.occurrences.subject_company")
        case .personal: return String(localized: "quality.occurrences.subject_personal")
        case .other: return String(localized: "quality.occurrences.subject_other")
        }
    }
}

private struct OccurrenceRow: View {
    let record: OfficialOccurrence

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(record.procedure.isEmpty ? String(localized: "android.occurrence_action_label") : record.procedure)
                .font(.headline)
            let date = QualityDisplay.date(record.documentDay)
            Text(String(localized: "android.occurrence_document_day", defaultValue: "Document date: \(date)"))
            let location = [record.municipality, record.uf].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: ", ")
            Text(location.isEmpty ? String(localized: "android.quality_unknown") : location)
        }
        .accessibilityElement(children: .combine)
    }
}

@MainActor
private struct OccurrenceRecordView: View {
    @ObservedObject var viewModel: AppStateViewModel
    let id: String
    let revision: String?
    @State private var detail: OfficialOccurrenceDetail?
    @State private var failed = false

    var body: some View {
        List {
            if let detail {
                if detail.stale { Text(String(localized: "android.occurrence_stale")) }
                OccurrenceRow(record: detail.record)
                LabeledContent(String(localized: "quality.occurrences.document_number"), value: detail.record.documentNumber)
                LabeledContent(String(localized: "quality.occurrences.source_result"), value: detail.record.result)
                LabeledContent(String(localized: "quality.occurrences.source_reference"), value: detail.record.evidenceRef)
                Text(String(localized: "android.occurrence_no_guilt")).foregroundStyle(.secondary)
            } else if failed {
                Text(String(localized: "android.occurrence_unavailable"))
            } else {
                ProgressView()
            }
        }
        .navigationTitle(String(localized: "android.occurrence_detail_title"))
        .task(id: viewModel.officialQualityScope) {
            detail = nil
            failed = false
            let scope = viewModel.officialQualityScope
            let loaded = try? await viewModel.officialQualityReader.occurrence(id: id, revision: revision)
            guard !Task.isCancelled, viewModel.officialQualityScope == scope else { return }
            detail = loaded
            failed = loaded == nil
        }
    }
}
