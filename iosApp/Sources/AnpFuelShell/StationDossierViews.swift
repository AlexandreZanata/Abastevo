import SwiftUI
import AnpFuelCore

@MainActor
struct StationDirectorySourcesCard: View {
    let dossier: StationDossier

    var body: some View {
        EnergiaCard {
            Text(String(localized: "station.dossier.registry_title"))
                .font(.headline)
                .accessibilityAddTraits(.isHeader)

            if let cnpj = BrazilianCNPJ.normalized(dossier.station.cnpj) {
                Text(String(localized: "station.price.tax_id", defaultValue: "CNPJ: \(cnpj)"))
                    .font(.subheadline)
            }
            if let legalName = dossier.secondaryLegalName {
                Text(String(localized: "station.dossier.legal_name", defaultValue: "Legal name: \(legalName)"))
                    .font(.subheadline)
            }
            if let source = dossier.addressSource {
                Text(String(localized: "station.dossier.address_source", defaultValue: "Address source: \(addressSourceLabel(source))"))
                    .font(.caption)
                if source.isApproximate && !dossier.station.address.isEmpty {
                    Text(String(localized: "android.server_station_approximate_address"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            if let official = dossier.officialAnchor {
                SourceTrustBadge(isOfficial: true, label: String(localized: "station.dossier.official_registry"))
                Text(registrySourceLabel(official.source)).font(.subheadline)
                if let date = official.finishedDate {
                    Text(String(localized: "station.dossier.registry_as_of", defaultValue: "Registry snapshot: \(date.formatted(date: .abbreviated, time: .omitted))"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            } else {
                Text(String(localized: "station.dossier.registry_unknown"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private func addressSourceLabel(_ source: StationDossierAddressSource) -> String {
        switch source {
        case .anp: return String(localized: "station.dossier.source.anp")
        case .rfb: return String(localized: "station.dossier.source.rfb")
        case .osm: return String(localized: "station.dossier.source.osm")
        case .cnefe: return String(localized: "station.dossier.source.cnefe")
        case .municipality: return String(localized: "station.dossier.source.municipality")
        }
    }

    private func registrySourceLabel(_ source: String) -> String {
        switch source {
        case "registry-csv": return String(localized: "station.dossier.source.registry_csv")
        case "registry-api": return String(localized: "station.dossier.source.registry_api")
        case "dou": return String(localized: "station.dossier.source.dou")
        default: return String(localized: "station.dossier.source.unknown")
        }
    }
}

@MainActor
struct StationDossierPricesCard: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var dossier: StationDossierModel
    let retry: () -> Void
    @State private var clock = Date()

    private var fuel: FuelProduct { viewModel.snapshot.selectedFuel }

    var body: some View {
        EnergiaCard {
            Picker(String(localized: "android.vehicle_fuel_product_label"), selection: Binding(
                get: { fuel }, set: { viewModel.selectFuel($0) }
            )) {
                ForEach(FuelProduct.allCases) { product in
                    Text(product.localizedName).tag(product)
                }
            }

            Divider()
            VStack(alignment: .leading, spacing: 6) {
                SourceTrustBadge(isOfficial: false, label: String(localized: "android.home_source_community"))
                communityContent
            }
            Divider()
            VStack(alignment: .leading, spacing: 6) {
                SourceTrustBadge(isOfficial: true, label: String(localized: "android.station_detail_anp_title"))
                officialContent
            }

            Text(String(localized: "android.community_confidence_note"))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .task(id: dossier.community[fuel]?.expiryDate) {
            clock = .now
            guard let expiry = dossier.community[fuel]?.expiryDate, expiry > clock else { return }
            do {
                try await Task.sleep(for: .seconds(expiry.timeIntervalSince(clock)))
                clock = .now
            } catch {}
        }
    }

    @ViewBuilder
    private var communityContent: some View {
        if dossier.priceState == .loading {
            ProgressView()
        } else if let price = dossier.community[fuel] {
            if price.availability == .disputed {
                Text(String(localized: "station.dossier.community_disputed"))
            } else if let amount = price.amount(at: max(clock, .now)) {
                Text(AbastevoTheme.formatBrlMilli(amount)).font(.headline)
                Text(unitLabel).font(.caption)
                Text(String(localized: "station.dossier.community_support", defaultValue: "Independent supporters: \(price.independentSupporters.formatted())"))
                    .font(.caption)
                Text(String(localized: "station.price.confirmations_short", defaultValue: "Confirmations: \(price.confirmationCount.formatted())"))
                    .font(.caption)
                if let received = price.receivedDate {
                    Text(received.formatted(date: .abbreviated, time: .shortened))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            } else if price.freshness == "STALE" || price.expiryDate.map({ $0 <= max(clock, .now) }) == true {
                Text(String(localized: "station.dossier.community_expired"))
            } else {
                Text(String(localized: "android.station_page_community_empty"))
            }
        } else if let observation = dossier.feedFallback(for: fuel, on: viewModel) {
            Text(AbastevoTheme.formatBrlMilli(observation.amountMilli)).font(.headline)
            Text(observation.condition.localizedLabel).font(.caption)
            Text(String(localized: "station.price.confirmations_short", defaultValue: "Confirmations: \(observation.confirmations.formatted())"))
                .font(.caption)
        } else if dossier.priceState == .unavailable {
            Text(String(localized: "android.community_unavailable_retry"))
            Button(String(localized: "android.feed_retry"), action: retry)
        } else {
            Text(String(localized: "android.station_page_community_empty"))
        }
    }

    @ViewBuilder
    private var officialContent: some View {
        if let read = dossier.officialRead {
            if let row = read.prices.first(where: { $0.product == fuel }) {
                Text(AbastevoTheme.formatBrlMilli(row.amountMilli)).font(.headline)
                Text(unitLabel).font(.caption)
                Text(String(localized: "station.prices.official_period", defaultValue: "ANP collection: \(AnpWeekText.day(row.collectedOn)) · survey: \(AnpWeekText.range(startIso: row.surveyStart, endIso: row.surveyEnd))"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                Text(String(localized: "station.prices.official_empty"))
            }
            switch read.source {
            case .live:
                EmptyView()
            case .cached(let savedAt):
                Text(viewModel.cachedLabel(savedAt)).font(.caption).foregroundStyle(.orange)
            case .anpDownload(let week):
                Text(String(localized: "station.prices.official_cached", defaultValue: "Offline: ANP prices for downloaded week \(AnpWeekText.range(week)), matched by CNPJ."))
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
        } else if dossier.priceState == .loading {
            ProgressView()
        } else {
            Text(String(localized: "station.prices.official_unavailable"))
        }
    }

    private var unitLabel: String {
        switch fuel.unitWire {
        case "M3": return String(localized: "android.feed_unit_m3")
        case "KG_13": return String(localized: "android.feed_unit_cylinder")
        default: return String(localized: "android.feed_unit_litre")
        }
    }
}

@MainActor
struct StationLinkedPlacesCard: View {
    @ObservedObject var evCatalog: EvCatalogModel
    let stationId: String
    @State private var selectedPlace: ChargingPlace?

    var body: some View {
        let places = evCatalog.places.filter {
            $0.linkedStationId?.caseInsensitiveCompare(stationId) == .orderedSame
        }
        if !places.isEmpty {
            EnergiaCard {
                Text(String(localized: "android.station_linked_chargers"))
                    .font(.headline)
                    .accessibilityAddTraits(.isHeader)
                ForEach(places) { place in
                    Button {
                        selectedPlace = place
                    } label: {
                        Label(place.name.isEmpty ? String(localized: "android.charging_place_unnamed") : place.name,
                              systemImage: "bolt.car")
                    }
                }
            }
            .sheet(item: $selectedPlace) { place in
                EvPlaceDetailSheet(place: place, snapshot: evCatalog.snapshot)
            }
        }
    }
}
