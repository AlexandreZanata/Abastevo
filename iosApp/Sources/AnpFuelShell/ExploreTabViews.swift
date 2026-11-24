import SwiftUI
import AnpFuelCore

/// Explore Tab + Station Detail + Price History + EV Explore + Location/Week Pickers + Quality/Suggest/FAQ.
@MainActor
public struct ExploreRootView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var tankLitersDouble: Double = 50.0
    @State private var searchStationText: String = ""
    @StateObject private var stationSearch = StationCatalogSearchModel()

    public var body: some View {
        NavigationStack(path: $viewModel.explorePath) {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    FuelProductChipsBar(
                        selectedFuel: viewModel.snapshot.selectedFuel,
                        onSelect: { viewModel.selectFuel($0) }
                    )

                    AnpCityAverageCard(app: viewModel, anp: viewModel.anpSurvey)
                        .padding(.horizontal, 16)

                    communitySummaryCard
                        .padding(.horizontal, 16)

                    if viewModel.snapshot.selectedFuel.unitWire == "L" {
                        tankFillCalculatorCard
                            .padding(.horizontal, 16)
                    }

                    discoveryShortcutsGrid
                        .padding(.horizontal, 16)

                    stationListSection
                        .padding(.horizontal, 16)
                }
                .padding(.vertical, 12)
            }
            .refreshable {
                await viewModel.refreshCityData()
                if !stationSearchScope.query.isEmpty {
                    await stationSearch.load(stationSearchScope, host: viewModel, reset: true)
                }
                await viewModel.anpSurvey.manualSync()
            }
        }
    }

    private var communitySummaryCard: some View {
        let feed = viewModel.visibleCommunityFeed
        let latest = feed.first
        return EnergiaCard {
            HStack {
                SourceTrustBadge(isOfficial: false, label: String(localized: "android.home_source_community"))
                Spacer()
                Button {
                    viewModel.activeSheet = .capturePrice
                } label: {
                    Label(String(localized: "explore.capture_photo"), systemImage: "camera.fill")
                        .font(.caption.bold())
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.accentLight)
            }
            if let latest = latest {
                Text(String(localized: "explore.community.recent_price", defaultValue: "Recent community price: \(AbastevoTheme.formatBrlMilli(latest.amountMilli))"))
                    .font(.headline)
                Text(String(localized: "explore.community.recent_summary", defaultValue: "\(latest.stationName) · \(latest.condition.localizedLabel) · confirmations: \(latest.confirmations.formatted())"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else if viewModel.communityReadState == .loading {
                ProgressView()
            } else if viewModel.communityReadState == .unavailable {
                Text(String(localized: "android.home_community_unavailable"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else {
                Text(String(localized: "android.home_community_empty"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            if let cachedAt = viewModel.communityCachedAt {
                Text(viewModel.cachedLabel(cachedAt))
                    .font(.caption2)
                    .foregroundStyle(.orange)
                if viewModel.communityExpiredCount > 0 {
                    Text(String(localized: "explore.community.expired", defaultValue: "Expired reports hidden: \(viewModel.communityExpiredCount.formatted())."))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private var tankFillCalculatorCard: some View {
        let fuel = viewModel.snapshot.selectedFuel
        let stationPrices: [StationPrice] = viewModel.sortedStations.compactMap { st in
            guard let price = viewModel.communityObservation(stationId: st.id, product: fuel) else { return nil }
            return StationPrice(station: st.name, unitMilli: price.amountMilli)
        }
        let capacity = Measurement(value: tankLitersDouble, unit: UnitVolume.liters)
            .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(0))))
        let ml = Int64(tankLitersDouble * 1000.0)
        let cheapestFill = try? TankFillUseCase.fillCheapest(prices: stationPrices, capacityMilliLiters: ml)

        return EnergiaCard {
            HStack {
                Text(String(localized: "explore.tank.title", defaultValue: "Tank fill estimate (\(capacity))"))
                    .font(.subheadline.bold())
                Spacer()
                Stepper("", value: $tankLitersDouble, in: 10...120, step: 5)
                    .labelsHidden()
            }
            if let fill = cheapestFill {
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(fill.station)
                            .font(.subheadline.weight(.semibold))
                        Text(String(localized: "explore.tank.community_price", defaultValue: "Community price (\(AbastevoTheme.formatBrlMilli(fill.unitMilli))/L)"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text(AbastevoTheme.formatBrlMilli(fill.totalMilli))
                        .font(.title3.bold())
                }
            } else {
                Text(String(localized: "explore.tank.no_prices"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var discoveryShortcutsGrid: some View {
        LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible()), GridItem(.flexible())], spacing: 10) {
            shortcutButton(title: String(localized: "android.nav_history"), icon: "chart.line.uptrend.xyaxis") {
                viewModel.activeSheet = .priceHistory
            }
            shortcutButton(title: String(localized: "ev.catalog.title"), icon: "bolt.car.fill") {
                viewModel.activeSheet = .evExplore
            }
            shortcutButton(title: String(localized: "explore.quality"), icon: "flask.fill") {
                viewModel.activeSheet = .qualityReports
            }
            shortcutButton(title: String(localized: "explore.suggest_station"), icon: "plus.circle.fill") {
                viewModel.activeSheet = .suggestStation
            }
            shortcutButton(title: String(localized: "explore.faq"), icon: "questionmark.circle.fill") {
                viewModel.activeSheet = .masterFaq
            }
            shortcutButton(title: String(localized: "ev.trip.title"), icon: "map.fill") {
                viewModel.activeSheet = .evTripPlanner
            }
        }
    }

    private func shortcutButton(title: String, icon: String, action: @escaping () -> Void) -> some View {
        Button {
            IosHaptics.lightTap()
            action()
        } label: {
            VStack(spacing: 6) {
                Image(systemName: icon)
                    .font(.title3)
                    .foregroundStyle(AbastevoTheme.accentLight)
                Text(title)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.primary)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 12)
            .background(Color.secondary.opacity(0.08))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        }
        .buttonStyle(.plain)
    }

    private var stationSearchScope: StationCatalogSearchModel.Scope {
        stationSearch.activeScope(for: searchStationText, on: viewModel)
    }

    private var stationListSection: some View {
        let searching = !stationSearchScope.query.isEmpty
        let currentSearch = stationSearch.isCurrent(stationSearchScope)
        let rows = searching
            ? (currentSearch ? viewModel.sortStationRows(stationSearch.rows) : [])
            : viewModel.sortedStations
        return VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(viewModel.selectedMunicipality.map { String(localized: "explore.stations.in_city", defaultValue: "Stations in \($0.name)") } ?? String(localized: "android.nav_stations"))
                    .font(.headline)
                Spacer()
                Picker(String(localized: "android.home_filter_title"), selection: $viewModel.sortOrder) {
                    ForEach(StationSortOrder.allCases) { order in
                        Text(order.localizedLabel).tag(order)
                    }
                }
                .pickerStyle(.menu)
            }

            TextField(String(localized: "explore.stations.filter"), text: $searchStationText)
                .textFieldStyle(.roundedBorder)

            if viewModel.selectedMunicipality == nil {
                Button(String(localized: "android.feed_choose_city")) { viewModel.activeSheet = .locationPicker }
            } else if searching {
                stationSearchStatus(current: currentSearch, hasRows: !rows.isEmpty)
            } else if viewModel.stationReadState == .loading {
                ProgressView()
            } else if viewModel.stationReadState == .unavailable {
                Text(String(localized: "explore.stations.unavailable"))
                Button(String(localized: "android.feed_retry")) { viewModel.reloadCityData() }
            } else if rows.isEmpty {
                Text(String(localized: "explore.stations.no_match"))
            }
            if !searching, let cachedAt = viewModel.directoryCachedAt {
                Text(viewModel.cachedLabel(cachedAt))
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
            if viewModel.sortOrder == .distance && viewModel.lastDeviceCoordinate == nil {
                Text(String(localized: "explore.stations.sort_by_location"))
                    .font(.caption)
            }
            ForEach(rows) { station in
                Button {
                    viewModel.selectedStation = station
                } label: {
                    stationRowCard(station)
                }
                .buttonStyle(.plain)
            }
            if searching, currentSearch {
                if let failure = stationSearch.failure {
                    let staleCursor = failure == .httpStatus(400, "station.bad-cursor")
                    Button(String(localized: "android.feed_retry")) {
                        Task {
                            await stationSearch.load(
                                stationSearchScope, host: viewModel,
                                reset: staleCursor || stationSearch.cursor == nil
                            )
                        }
                    }
                    .disabled(stationSearch.isLoading)
                } else if !stationSearch.isLoading, stationSearch.cursor != nil {
                    Button(String(localized: "explore.stations.load_more")) {
                        Task { await stationSearch.load(stationSearchScope, host: viewModel, reset: false) }
                    }
                }
            } else if !searching {
                if viewModel.stationPageUnavailable {
                    Text(String(localized: "explore.stations.next_failed")).font(.caption)
                }
                if viewModel.isLoadingMoreStations {
                    ProgressView()
                } else if viewModel.nextStationCursor != nil {
                    Button(String(localized: "explore.stations.load_more")) {
                        Task { await viewModel.loadMoreStations() }
                    }
                }
            }
        }
        .task(id: stationSearchScope) {
            guard searching else { return }
            await stationSearch.load(stationSearchScope, host: viewModel, reset: true, debounce: true)
        }
    }

    @ViewBuilder
    private func stationSearchStatus(current: Bool, hasRows: Bool) -> some View {
        if !current || stationSearch.isLoading {
            ProgressView()
        } else if let failure = stationSearch.failure {
            if stationSearch.showsLoadedFallback && hasRows {
                Text(String(localized: "station.directory.loaded_matches")).font(.caption)
            } else if failure == .httpStatus(400, "station.bad-cursor") {
                Text(String(localized: "station.directory.cursor_invalid")).font(.caption)
            } else {
                Text(String(localized: "explore.stations.unavailable"))
            }
        } else if !hasRows {
            Text(String(localized: "explore.stations.no_match"))
        }
    }

    private func stationRowCard(_ station: StationRecord) -> some View {
        let fuel = viewModel.snapshot.selectedFuel
        let community = viewModel.communityObservation(stationId: station.id, product: fuel)
        return EnergiaCard {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(station.name)
                        .font(.headline)
                    Text(station.neighborhood)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(station.address)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                    if let promo = station.ownerPromotionText {
                        Text(promo)
                            .font(.caption2.bold())
                            .foregroundStyle(AbastevoTheme.accentLight)
                    }
                }
                Spacer()
                VStack(alignment: .trailing, spacing: 4) {
                    if let comm = community {
                        Text(AbastevoTheme.formatBrlMilli(comm.amountMilli))
                            .font(.title3.bold())
                        SourceTrustBadge(isOfficial: false, label: String(localized: "station.price.confirmations_short", defaultValue: "Confirmations: \(comm.confirmations.formatted())"))
                    }
                    if community == nil {
                        Text(String(localized: "android.home_view_price_details"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }
}

@MainActor
public struct StationDetailSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord

    @StateObject private var dossier = StationDossierModel()
    @State private var readAttempt = 0
    @State private var showRoute = false

    private struct ReadScope: Hashable {
        let stationId: String
        let origin: String
        let attempt: Int
    }

    public var body: some View {
        let read = ReadScope(stationId: station.id, origin: viewModel.snapshot.apiEnvironment.origin, attempt: readAttempt)
        let current = dossier.matches(seed: station, origin: read.origin)
        let verified = current ? dossier.canonicalStation : nil
        let identityState: StationDossierModel.IdentityState = current ? dossier.identityState : .loading
        let displayStation = verified ?? station
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    identityCard(displayStation, state: identityState)
                    if current, let registry = dossier.identity {
                        StationDirectorySourcesCard(dossier: registry)
                    }

                    if current, identityState != .notFound {
                        StationDossierPricesCard(viewModel: viewModel, dossier: dossier, retry: retry)
                        StationOfficialHistoryCard(viewModel: viewModel, station: displayStation)
                    }

                    if let canonical = verified {
                        StationLinkedPlacesCard(evCatalog: viewModel.evCatalog, stationId: canonical.id)
                        StationQualityCard(viewModel: viewModel, station: canonical, fuel: viewModel.snapshot.selectedFuel)
                        StationFeedbackCard(app: viewModel, station: canonical, fuel: viewModel.snapshot.selectedFuel)
                            .id(canonical.id + viewModel.snapshot.selectedFuel.wireCode)
                        StationCommentComposerCard(viewModel: viewModel, station: canonical)
                        StationBusinessProfileCard(
                            viewModel: viewModel, canonicalStationId: canonical.id, fuel: viewModel.snapshot.selectedFuel
                        )
                    } else {
                        if identityState == .unavailable && CommunityWire.isUuid(station.id) {
                            StationLinkedPlacesCard(evCatalog: viewModel.evCatalog, stationId: station.id)
                        }
                        EnergiaCard {
                            Text(String(localized: "android.station_page_experience_title"))
                                .font(.headline)
                            Text(String(localized: "android.station_page_participation_pending"))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                .padding(16)
            }
            .task(id: read) {
                await dossier.load(station, on: viewModel)
            }
            .sheet(isPresented: $showRoute) {
                let station = displayStation
                NativeRouteSheetView(viewModel: viewModel, station: station)
            }
            .navigationTitle(String(localized: "station.details.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }

    private func retry() { readAttempt += 1 }

    private func identityCard(_ displayStation: StationRecord, state: StationDossierModel.IdentityState) -> some View {
        EnergiaCard {
            if state == .loading { ProgressView() }
            HStack {
                Text(displayStation.pinTier.localizedLabel)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                Spacer()
                let saved = viewModel.snapshot.savedPlaces.contains(where: { $0.stationId == displayStation.id })
                Button {
                    viewModel.toggleSavedStation(displayStation)
                } label: {
                    Image(systemName: saved ? "bookmark.fill" : "bookmark")
                }
                .accessibilityLabel(saved ? String(localized: "station.dossier.remove_saved")
                                    : String(localized: "station.dossier.save"))
                .disabled(state == .notFound)
            }
            Text(displayStation.name)
                .font(.title3.bold())
                .accessibilityAddTraits(.isHeader)
            let address = [displayStation.address, displayStation.neighborhood].filter { !$0.isEmpty }.joined(separator: " — ")
            if !address.isEmpty {
                Text(address).font(.subheadline).foregroundStyle(.secondary)
            }
            if state != .resolved, let cnpj = BrazilianCNPJ.normalized(displayStation.cnpj) {
                Text(String(localized: "station.price.tax_id", defaultValue: "CNPJ: \(cnpj)"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let promotion = displayStation.ownerPromotionText {
                Text(String(localized: "station.price.promotion", defaultValue: "Reported promotion: \(promotion)"))
                    .font(.caption.bold())
                    .foregroundStyle(AbastevoTheme.accentLight)
            }
            if state == .unavailable || state == .notFound {
                Text(state == .notFound ? String(localized: "station.dossier.not_found")
                     : String(localized: "android.station_profile_unavailable"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Button(String(localized: "android.feed_retry"), action: retry)
            }
            HStack(spacing: 10) {
                Button {
                    viewModel.activeSheet = .capturePrice
                } label: {
                    Label(String(localized: "explore.capture_photo"), systemImage: "camera.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.accentLight)
                .disabled(state != .resolved)

                Button {
                    showRoute = true
                } label: {
                    Label(String(localized: "android.stations_navigate_action"), systemImage: "arrow.triangle.turn.up.right.diamond.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .disabled(state == .notFound || !displayStation.pinTier.allowsMapPin)
            }
        }
    }
}

public struct LocationPickerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var query = ""
    @State private var selectedState: BrazilianState?

    public var body: some View {
        NavigationStack {
            VStack(spacing: 12) {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 6) {
                        Button(String(localized: "android.map_filter_all")) { selectedState = nil }
                            .buttonStyle(.bordered)
                        ForEach(BrazilianState.allCases) { uf in
                            Button(uf.rawValue) { selectedState = uf }
                                .buttonStyle(.bordered)
                                .tint(selectedState == uf ? AbastevoTheme.accentLight : .secondary)
                        }
                    }
                    .padding(.horizontal, 16)
                }

                TextField(String(localized: "city.search.filter"), text: $query)
                    .textFieldStyle(.roundedBorder)
                    .padding(.horizontal, 16)

                let cities = viewModel.municipalityCatalog.search(query: query, state: selectedState)
                if viewModel.municipalityCatalog.count == 0 {
                    ContentUnavailableView(String(localized: "city.search.unavailable"), systemImage: "building.2")
                } else if cities.isEmpty {
                    ContentUnavailableView(String(localized: "android.location_search_no_matches"), systemImage: "magnifyingglass")
                } else {
                    List(cities) { city in
                        Button {
                            viewModel.selectMunicipality(city)
                            dismiss()
                        } label: {
                            HStack {
                                VStack(alignment: .leading) {
                                    Text(city.name).font(.headline)
                                    Text(String(localized: "city.search.row", defaultValue: "\(city.state.fullName) · IBGE \(String(city.ibgeCode))"))
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                                Spacer()
                                if city.ibgeCode == viewModel.selectedMunicipality?.ibgeCode {
                                    Image(systemName: "checkmark.circle.fill")
                                        .foregroundStyle(AbastevoTheme.accentLight)
                                }
                            }
                        }
                    }
                }
            }
            .navigationTitle(String(localized: "android.feed_choose_city"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }
}

public struct MasterFaqSheetView: View {
    @Environment(\.dismiss) private var dismiss

    public var body: some View {
        NavigationStack {
            List {
                faqRow(q: String(localized: "explore.faq.sources_question"), a: String(localized: "explore.faq.sources_answer"))
                faqRow(q: String(localized: "explore.faq.payment_question"), a: String(localized: "explore.faq.payment_answer"))
                faqRow(q: String(localized: "explore.faq.offline_question"), a: String(localized: "explore.faq.offline_answer"))
                faqRow(q: String(localized: "explore.faq.photos_question"), a: String(localized: "explore.faq.photos_answer"))
            }
            .navigationTitle(String(localized: "explore.faq"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }

    private func faqRow(q: String, a: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(q).font(.headline)
            Text(a).font(.subheadline).foregroundStyle(.secondary)
        }
        .padding(.vertical, 4)
    }
}
