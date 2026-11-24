import SwiftUI
import AnpFuelCore

@MainActor
extension AppStateViewModel {
    var officialQualityScope: QualityReadScope {
        QualityReadScope(origin: snapshot.apiEnvironment.origin, accountId: currentAccountIdOrNil)
    }

    var officialQualityReader: QualityPublicReader {
        QualityPublicReader(client: qualityReadClient, cache: qualityPublicCache,
                            scope: officialQualityScope, offline: isOfflineSimulation)
    }
}

@MainActor
struct StationQualityCard: View {
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord
    let fuel: FuelProduct

    var body: some View {
        EnergiaCard {
            Text(String(localized: "station.quality.title")).font(.headline)
            NavigationLink {
                StationQualityView(viewModel: viewModel, station: station, fuel: fuel)
            } label: {
                Label(String(localized: "android.quality_entry"), systemImage: "flask")
            }
            .buttonStyle(.bordered)
        }
    }
}

@MainActor
struct StationQualityView: View {
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord
    @State private var fuel: FuelProduct
    @State private var revision: String?
    @State private var summary: QualitySummary?
    @State private var assayPage: QualityPage<QualityAssay>?
    @State private var revisions: [QualityRevision] = []
    @State private var revisionCursor: String?
    @State private var loadedInput: Input?
    @State private var loadTicket = UUID()
    @State private var catalogTicket = UUID()
    @State private var failure = false
    @State private var stale = false
    @State private var loading = false
    @State private var catalogLoading = false
    @State private var catalogFailed = false
    @State private var detail: Detail?

    private struct Input: Equatable {
        let scope: QualityReadScope
        let offline: Bool
        let selection: QualitySelection
    }

    private enum Detail: Identifiable {
        case sample(QualityAssay, revision: String, fuel: FuelProduct)
        case provenance(String)

        var id: String {
            switch self {
            case .sample(let assay, let revision, _): return revision + "/" + assay.sampleRef
            case .provenance(let revision): return revision
            }
        }
    }

    init(viewModel: AppStateViewModel, station: StationRecord, fuel: FuelProduct) {
        self.viewModel = viewModel
        self.station = station
        _fuel = State(initialValue: fuel)
    }

    private var input: Input {
        Input(scope: viewModel.officialQualityScope, offline: viewModel.isOfflineSimulation,
              selection: QualitySelection(stationId: station.id, fuel: fuel, revisionId: revision))
    }

    var body: some View {
        List {
            Section {
                Text(station.name).font(.headline)
                Picker(String(localized: "android.vehicle_fuel_product_label"), selection: $fuel) {
                    ForEach(FuelProduct.allCases) { product in Text(product.localizedName).tag(product) }
                }
                Text(String(localized: "android.quality_score_unavailable")).foregroundStyle(.secondary)
                if let summary, loadedInput == input {
                    summaryDetails(summary)
                }
                if loading {
                    ProgressView()
                }
                if failure {
                    Text(String(localized: "android.quality_unavailable")).foregroundStyle(.secondary)
                    Button(String(localized: "android.action_retry")) { Task { await load() } }
                }
            }
            Section(String(localized: "android.quality_periods")) {
                Picker(String(localized: "android.quality_periods"), selection: $revision) {
                    Text(String(localized: "android.quality_current")).tag(nil as String?)
                    ForEach(revisions.filter(\.isSelectable)) { item in
                        Text(item.edition).tag(Optional(item.id))
                    }
                }
                .disabled(catalogLoading)
                if catalogFailed {
                    Text(String(localized: "android.quality_revisions_unavailable")).foregroundStyle(.secondary)
                    Button(String(localized: "android.action_retry")) { Task { await loadRevisions() } }
                }
                if revisionCursor != nil {
                    Button(String(localized: "android.quality_more_periods")) { Task { await loadRevisions(next: true) } }
                        .disabled(catalogLoading)
                }
            }
            if let page = assayPage, loadedInput == input {
                Section {
                    if page.items.isEmpty {
                        Text(String(localized: "android.quality_empty"))
                    }
                    ForEach(page.items) { assay in
                        if let revision = page.lease?.revisionId {
                            Button {
                                detail = .sample(assay, revision: revision, fuel: fuel)
                            } label: {
                                QualityAssayRow(assay: assay)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    if let cursor = page.nextCursor {
                        Button(String(localized: "android.quality_more_assays")) { Task { await load(cursor: cursor) } }
                            .disabled(loading)
                    }
                } header: {
                    Text(String(localized: "android.quality_aspect_note"))
                }
            }
            Section {
                NavigationLink {
                    QualityReportScreen(viewModel: viewModel, target: QualityReportTarget(stationId: station.id, fuel: fuel))
                } label: {
                    Text(String(localized: "android.quality_reports_entry"))
                }
            }
        }
        .navigationTitle(String(localized: "android.quality_title"))
        .task(id: input) { await load() }
        .task(id: viewModel.officialQualityScope) { await loadRevisions() }
        .refreshable { await load() }
        .sheet(item: $detail) { item in
            NavigationStack {
                switch item {
                case .sample(let assay, let revision, let selectedFuel):
                    QualitySampleView(viewModel: viewModel, stationId: station.id, fuel: selectedFuel,
                                      revision: revision, ref: assay.sampleRef)
                case .provenance(let revision):
                    QualitySourceView(viewModel: viewModel, revision: revision)
                }
            }
        }
    }

    @ViewBuilder private func summaryDetails(_ result: QualitySummary) -> some View {
        if stale || result.stale {
            Text(String(localized: "android.quality_stale")).foregroundStyle(.secondary)
        }
        if !result.lease.available {
            Text(String(localized: "android.quality_no_source"))
        } else {
            let edition = result.lease.edition
            let published = QualityDisplay.date(result.lease.publishedAt)
            Text(String(localized: "android.quality_edition", defaultValue: "Edition \(edition) · published \(published)"))
            if result.lease.isCurrentStream == false {
                Text(String(localized: "android.quality_historical")).foregroundStyle(.secondary)
            }
            if let dimensions = result.sampleDimensions {
                ForEach(dimensions, id: \.fuelProduct) { item in
                    Text(String(localized: "android.quality_samples", defaultValue: "\(item.samples) samples · \(item.visits) visits"))
                    Text(String(localized: "android.quality_verdicts", defaultValue: "\(item.conforming) conforming · \(item.nonconforming) nonconforming · \(item.unresolved) unresolved"))
                    let first = QualityDisplay.date(item.oldestObservedOn)
                    let last = QualityDisplay.date(item.newestObservedOn)
                    Text(String(localized: "android.quality_sample_range", defaultValue: "From \(first) to \(last)"))
                }
            } else {
                Text(String(localized: "android.quality_pending_dimensions"))
            }
            ForEach(result.cells, id: \.conformity) { cell in
                Text(QualityDisplay.verdict(cell.conformity))
                let last = QualityDisplay.date(cell.lastObserved)
                Text(String(localized: "android.quality_assay_count", defaultValue: "\(cell.count) assays · last on \(last)"))
            }
            if let edition = result.lease.revisionId {
                Button(String(localized: "android.quality_source")) { detail = .provenance(edition) }
            }
        }
    }

    private func load(cursor: String? = nil) async {
        let request = input
        if cursor != nil && loading { return }
        if loadedInput != request {
            summary = nil
            assayPage = nil
            stale = false
        }
        let ticket = UUID()
        loadTicket = ticket
        loading = true
        failure = false
        let reader = viewModel.officialQualityReader
        defer { if loadTicket == ticket { loading = false } }
        do {
            let previous = cursor == nil ? nil : summary
            let result: QualitySummary
            if let previous { result = previous } else { result = try await reader.summary(request.selection) }
            guard !Task.isCancelled, ticket == loadTicket, request == input else { return }
            let page: QualityPage<QualityAssay>?
            if result.lease.available, let revision = result.lease.revisionId {
                let selection = QualitySelection(stationId: station.id, fuel: request.selection.fuel, revisionId: revision)
                let next = try await reader.assays(selection, cursor: cursor)
                if let existing = assayPage?.items.last, cursor != nil, let first = next.items.first,
                   first.observedOn + first.id <= existing.observedOn + existing.id {
                    throw QualityReadError.invalid
                }
                page = next
            } else {
                page = nil
            }
            guard !Task.isCancelled, ticket == loadTicket, request == input else { return }
            summary = result
            assayPage = page
            stale = result.stale || (page?.stale ?? false)
            loadedInput = request
        } catch {
            guard !Task.isCancelled, ticket == loadTicket, request == input else { return }
            if error as? QualityReadError == .disabled {
                summary = nil
                assayPage = nil
            }
            failure = true
            stale = summary != nil
        }
    }

    private func loadRevisions(next: Bool = false) async {
        if next && catalogLoading { return }
        let scope = viewModel.officialQualityScope
        let ticket = UUID()
        catalogTicket = ticket
        let requestCursor = next ? revisionCursor : nil
        catalogLoading = true
        catalogFailed = false
        defer { if catalogTicket == ticket { catalogLoading = false } }
        do {
            let page = try await viewModel.officialQualityReader.revisions(cursor: requestCursor)
            guard !Task.isCancelled, catalogTicket == ticket, viewModel.officialQualityScope == scope else { return }
            let before = next ? revisions : []
            let existing = Set(before.map(\.id))
            revisions = before + page.items.filter { !existing.contains($0.id) }
            revisionCursor = page.nextCursor
        } catch {
            guard !Task.isCancelled, catalogTicket == ticket, viewModel.officialQualityScope == scope else { return }
            catalogFailed = true
            if !next { revisions = [] }
        }
    }
}

enum QualityDisplay {
    static func date(_ text: String?) -> String {
        guard let text else { return String(localized: "android.quality_unknown") }
        if let day = AnpCalendar.parseIsoDay(text) {
            var style = Date.FormatStyle(date: .abbreviated, time: .omitted)
            style.timeZone = .gmt
            return day.formatted(style)
        }
        if let instant = ISO8601DateFormatter().date(from: text) {
            return instant.formatted(date: .abbreviated, time: .omitted)
        }
        return String(localized: "android.quality_unknown")
    }

    static func verdict(_ value: String) -> String {
        switch value {
        case "CONFORMING": return String(localized: "android.quality_conforming")
        case "NONCONFORMING": return String(localized: "android.quality_nonconforming")
        default: return String(localized: "android.quality_unknown")
        }
    }
}

private struct QualityAssayRow: View {
    let assay: QualityAssay

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(assay.assayRef).font(.headline)
            let observed = QualityDisplay.date(assay.sourceObservedAt ?? assay.observedOn)
            Text(String(localized: "android.quality_observed", defaultValue: "Observed \(observed)"))
            let value = [assay.resultText, assay.unit].filter { !$0.isEmpty }.joined(separator: " ")
            Text(value.isEmpty ? String(localized: "android.quality_blank_result") : value)
            Text(QualityDisplay.verdict(assay.conformity))
        }
        .accessibilityElement(children: .combine)
    }
}

@MainActor
private struct QualitySampleView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let stationId: String
    let fuel: FuelProduct
    let revision: String
    let ref: String
    @State private var sample: QualitySample?
    @State private var failed = false

    var body: some View {
        List {
            if let sample {
                if sample.stale {
                    Text(String(localized: "android.quality_stale"))
                }
                Text(String(localized: "android.quality_aspect_note"))
                ForEach(sample.assays) { assay in QualityAssayRow(assay: assay) }
                NavigationLink {
                    QualityReportScreen(
                        viewModel: viewModel,
                        target: QualityReportTarget(stationId: stationId, fuel: fuel, revision: revision, sample: ref)
                    )
                } label: {
                    Text(String(localized: "android.quality_reports_link"))
                }
            } else if failed {
                Text(String(localized: "android.quality_unavailable"))
            } else {
                ProgressView()
            }
        }
        .navigationTitle(String(localized: "android.quality_sample_title"))
        .toolbar { Button(String(localized: "android.feed_close")) { dismiss() } }
        .task(id: viewModel.officialQualityScope) {
            sample = nil
            failed = false
            let scope = viewModel.officialQualityScope
            let result = try? await viewModel.officialQualityReader.sample(ref: ref, revision: revision, fuel: fuel)
            guard !Task.isCancelled, viewModel.officialQualityScope == scope else { return }
            sample = result
            failed = result == nil
        }
    }
}

@MainActor
private struct QualitySourceView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let revision: String
    @State private var source: QualityProvenance?
    @State private var failed = false

    var body: some View {
        List {
            if let source {
                if source.stale { Text(String(localized: "android.quality_stale")) }
                ForEach(source.resources) { resource in
                    VStack(alignment: .leading, spacing: 6) {
                        if let member = resource.memberName, !member.isEmpty { Text(member).font(.headline) }
                        Text(String(localized: "android.quality_hash", defaultValue: "SHA-256: \(resource.sha256)"))
                            .font(.caption)
                        Link(String(localized: "android.quality_source"), destination: resource.url)
                    }
                }
            } else if failed {
                Text(String(localized: "android.quality_unavailable"))
            } else {
                ProgressView()
            }
        }
        .navigationTitle(String(localized: "android.quality_source"))
        .toolbar { Button(String(localized: "android.feed_close")) { dismiss() } }
        .task(id: viewModel.officialQualityScope) {
            source = nil
            failed = false
            let scope = viewModel.officialQualityScope
            let result = try? await viewModel.officialQualityReader.provenance(revision: revision)
            guard !Task.isCancelled, viewModel.officialQualityScope == scope else { return }
            source = result
            failed = result == nil
        }
    }
}
