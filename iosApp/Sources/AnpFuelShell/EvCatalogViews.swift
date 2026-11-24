import SwiftUI
import MapKit
import AnpFuelCore

/// Admitted EV charging catalog: Explore list, filters, place detail and the trip planner.
public struct EvChargingExploreSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var showingSuggestion = false

    public var body: some View {
        NavigationStack {
            EvExploreContent(app: viewModel, ev: viewModel.evCatalog)
                .navigationTitle(String(localized: "ev.catalog.title"))
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
                    }
                    ToolbarItem(placement: .primaryAction) {
                        Button {
                            showingSuggestion = true
                        } label: {
                            Label(String(localized: "android.charging_add_site"), systemImage: "plus")
                        }
                    }
                }
        }
        .sheet(isPresented: $showingSuggestion) {
            SuggestStationSheetView(viewModel: viewModel, chargingSite: true)
        }
    }
}

struct EvExploreContent: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var ev: EvCatalogModel
    @State private var visibleLimit = 100

    private let powerOptions = [0, 7, 22, 50, 150]

    var body: some View {
        let near = app.lastDeviceCoordinate.map { EvCoordinate(latitude: $0.latitude, longitude: $0.longitude) }
        let places = ev.filteredPlaces(city: app.selectedMunicipality, near: near)
        List {
            Section(String(localized: "ev.catalog.catalog")) {
                EvCatalogStatusView(ev: ev)
            }
            Section(String(localized: "android.map_filters")) {
                Picker(String(localized: "ev.catalog.scope"), selection: $ev.scope) {
                    ForEach(EvCatalogScope.allCases) { scope in
                        Text(scope.label).tag(scope)
                    }
                }
                .pickerStyle(.segmented)
                Picker(String(localized: "android.charging_explore_connector_label"), selection: $ev.filter.connector) {
                    Text(String(localized: "android.charging_explore_all")).tag(ChargingConnector?.none)
                    ForEach(ChargingConnector.allCases) { connector in
                        Text(connector.label).tag(ChargingConnector?.some(connector))
                    }
                }
                Picker(String(localized: "ev.catalog.current"), selection: $ev.filter.current) {
                    Text(String(localized: "android.charging_explore_all_current")).tag(ChargingCurrent?.none)
                    Text(String(localized: "ev.catalog.ac")).tag(ChargingCurrent?.some(.ac))
                    Text(String(localized: "ev.catalog.dc")).tag(ChargingCurrent?.some(.dc))
                }
                Picker(String(localized: "ev.catalog.min_power"), selection: Binding(
                    get: { ev.filter.minPowerKw ?? 0 },
                    set: { ev.filter.minPowerKw = $0 > 0 ? $0 : nil }
                )) {
                    ForEach(powerOptions, id: \.self) { kw in
                        Text(kw == 0 ? String(localized: "ev.catalog.any_power") : EvText.power(Int64(kw) * 1_000)).tag(kw)
                    }
                }
                Toggle(String(localized: "android.charging_explore_public_only"), isOn: $ev.filter.publicOnly)
                TextField(String(localized: "ev.catalog.search"), text: $ev.filter.query)
            }
            Section(String(localized: "ev.catalog.site_count", defaultValue: "Charging sites (\(places.count.formatted()))")) {
                if ev.isLoading {
                    ProgressView()
                } else if ev.snapshot == nil {
                    Text(String(localized: "ev.catalog.not_downloaded"))
                        .foregroundStyle(.secondary)
                } else if places.isEmpty {
                    Text(String(localized: "android.charging_explore_empty"))
                        .foregroundStyle(.secondary)
                }
                ForEach(places.prefix(visibleLimit)) { place in
                    NavigationLink {
                        EvPlaceDetailView(place: place, snapshot: ev.snapshot, vehicleConnector: ev.filter.connector)
                    } label: {
                        EvPlaceRow(place: place, near: near, connector: ev.filter.connector)
                    }
                }
                if places.count > visibleLimit {
                    Button(String(localized: "android.charging_explore_more")) { visibleLimit += 100 }
                }
            }
        }
        .onAppear { ev.loadIfNeeded() }
    }
}

struct EvCatalogStatusView: View {
    @ObservedObject var ev: EvCatalogModel

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let snapshot = ev.snapshot {
                Text(snapshot.profile == EvCatalogService.nationalProfile ? String(localized: "ev.catalog.national") : String(localized: "ev.catalog.ocm"))
                    .font(.subheadline.bold())
                Text(String(localized: "ev.catalog.revision", defaultValue: "Sites: \(snapshot.places.count.formatted()) · revision \(String(snapshot.sourceRevision.prefix(12)))"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                if let savedAt = ev.savedAt {
                    let freshness = ev.age == .stale ? String(localized: "ev.catalog.stale") : String(localized: "ev.catalog.current_snapshot")
                    Text(String(localized: "ev.catalog.saved_status", defaultValue: "Saved \(AnpWeekText.age(since: savedAt)) · \(freshness)"))
                        .font(.caption)
                        .foregroundStyle(ev.age == .stale ? Color.orange : Color.secondary)
                }
                Text(String(localized: "ev.catalog.license", defaultValue: "Source: \(snapshot.license). Reference data only. Live availability is unknown."))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                Text(String(localized: "ev.catalog.empty"))
                    .font(.subheadline)
            }
            if let progress = ev.refreshProgress {
                HStack {
                    if let expected = progress.expectedPages, expected > 0 {
                        ProgressView(value: Double(min(progress.pages, expected)), total: Double(expected))
                    } else {
                        ProgressView()
                    }
                    Text(String(localized: "ev.catalog.page", defaultValue: "Page \(progress.pages.formatted())"))
                        .font(.caption)
                    Button(String(localized: "android.ev_cancel")) { ev.cancelRefresh() }
                        .font(.caption)
                }
            } else {
                Button {
                    ev.refresh()
                } label: {
                    Label(String(localized: "android.charging_explore_refresh"), systemImage: "arrow.clockwise")
                }
            }
            if let message = ev.errorMessage {
                Text(message)
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
        }
    }
}

struct EvPlaceRow: View {
    let place: ChargingPlace
    let near: EvCoordinate?
    let connector: ChargingConnector?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(place.displayName)
                    .font(.headline)
                Spacer()
                if let power = place.maxPowerW {
                    Text(EvText.power(power))
                        .font(.caption.bold())
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(AbastevoTheme.accentLight.opacity(0.16))
                        .clipShape(Capsule())
                }
            }
            if let operatorName = place.operatorName {
                Text(operatorName)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let address = place.addressLine {
                Text(address)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            HStack(spacing: 8) {
                Text(place.connectors.isEmpty ? String(localized: "android.charging_explore_no_groups") : place.connectors.map(\.label).joined(separator: ", "))
                    .font(.caption)
                if let near, let coordinate = place.coordinate {
                    Text(EvText.distance(ChargingRules.distanceMeters(near, coordinate)))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if place.coordinate == nil {
                    Text(String(localized: "ev.site.unverified_location"))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            if let connector {
                EvCompatibilityBadge(verdict: ChargingRules.assess(vehicleConnector: connector, groups: place.groups))
            }
        }
        .padding(.vertical, 2)
    }
}

struct EvCompatibilityBadge: View {
    let verdict: ChargingCompatibilityVerdict

    var body: some View {
        switch verdict {
        case .compatible:
            Label(String(localized: "android.charging_explore_compatible"), systemImage: "checkmark.circle")
                .font(.caption2)
                .foregroundStyle(AbastevoTheme.accentLight)
        case .unknown:
            Label(String(localized: "android.charging_explore_unknown_compat"), systemImage: "questionmark.circle")
                .font(.caption2)
                .foregroundStyle(.secondary)
        case .incompatible:
            Label(String(localized: "android.charging_explore_incompatible"), systemImage: "xmark.circle")
                .font(.caption2)
                .foregroundStyle(.orange)
        }
    }
}

struct EvPlaceDetailView: View {
    let place: ChargingPlace
    let snapshot: EvCatalogSnapshot?
    let vehicleConnector: ChargingConnector?

    var body: some View {
        List {
            Section {
                Text(place.displayName).font(.title3.bold())
                if let operatorName = place.operatorName {
                    Text(String(localized: "android.charging_place_operator", defaultValue: "Operator · \(operatorName)"))
                }
                if let address = place.addressLine {
                    Text(address).foregroundStyle(.secondary)
                }
                Text(place.publicAccess == .publicAccess ? String(localized: "ev.site.public_access") : String(localized: "android.ev_access_unknown"))
                    .font(.caption)
                if place.confirmedByTwoSources {
                    Label(String(localized: "android.charging_place_two_sources"), systemImage: "checkmark.seal")
                        .font(.caption)
                }
                if let coordinate = place.coordinate {
                    Text(String(localized: "ev.site.coordinates", defaultValue: "Verified coordinates: \(EvText.coordinate(coordinate))"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Button(String(localized: "map.route.open_apple_maps")) {
                        let item = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: coordinate.latitude, longitude: coordinate.longitude)))
                        item.name = place.displayName
                        item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeDriving])
                    }
                } else {
                    Text(String(localized: "ev.site.unverified_location_details"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Section(String(localized: "ev.site.connector_counts", defaultValue: "Connectors (groups: \(place.groups.count.formatted()) · reported points: \(place.reportedPoints.formatted()))")) {
                if place.groups.isEmpty {
                    Text(String(localized: "android.charging_explore_no_groups")).foregroundStyle(.secondary)
                }
                ForEach(place.groups) { group in
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(group.connector.label) · \(EvText.current(group.current))")
                            .font(.subheadline.bold())
                        Text(group.powerW.map(EvText.power) ?? String(localized: "ev.site.power_unknown"))
                            .font(.caption)
                        if let quantity = group.reportedQuantity {
                            Text(String(localized: "ev.site.quantity", defaultValue: "Reported quantity: \(quantity.formatted())")).font(.caption2)
                        }
                        if group.connector == .unknown {
                            Text(String(localized: "ev.site.source_code", defaultValue: "Source code: \(group.rawConnectorCode)"))
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                if let vehicleConnector {
                    EvCompatibilityBadge(verdict: ChargingRules.assess(vehicleConnector: vehicleConnector, groups: place.groups))
                }
            }
            Section(String(localized: "ev.site.sources")) {
                ForEach(Array(place.sources.enumerated()), id: \.offset) { _, source in
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(source.source) · \(source.externalId)").font(.caption.bold())
                        Text(source.attribution).font(.caption2)
                        Text(source.license).font(.caption2).foregroundStyle(.secondary)
                    }
                }
                if let snapshot {
                    Text(String(localized: "ev.site.snapshot", defaultValue: "Catalog \(snapshot.profile), revision \(String(snapshot.sourceRevision.prefix(12))), saved \(AnpWeekText.age(since: Date(timeIntervalSince1970: TimeInterval(snapshot.savedAtEpochSeconds))))."))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
                Text(String(localized: "android.ev_operation_unknown"))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle(String(localized: "ev.site.title"))
    }
}

/// Map pin detail sheet.
struct EvPlaceDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    let place: ChargingPlace
    let snapshot: EvCatalogSnapshot?

    var body: some View {
        NavigationStack {
            EvPlaceDetailView(place: place, snapshot: snapshot, vehicleConnector: nil)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
                    }
                }
        }
    }
}

enum EvText {
    static func current(_ current: ChargingCurrent) -> String {
        switch current {
        case .ac: return String(localized: "ev.catalog.ac")
        case .dc: return String(localized: "ev.catalog.dc")
        case .unknown: return String(localized: "android.charging_explore_unknown")
        }
    }

    static func power(_ powerW: Int64) -> String {
        let kw = Double(powerW) / 1_000
        let measurement = Measurement(value: kw, unit: UnitPower.kilowatts)
        return measurement.formatted(.measurement(
            width: .abbreviated,
            usage: .asProvided,
            numberFormatStyle: .number.precision(.fractionLength(kw >= 10 ? 0 : 1))
        ))
    }

    static func distance(_ meters: Double) -> String {
        let longDistance = meters >= 1_000
        let measurement = longDistance
            ? Measurement(value: meters / 1_000, unit: UnitLength.kilometers)
            : Measurement(value: meters, unit: UnitLength.meters)
        return measurement.formatted(.measurement(
            width: .abbreviated,
            usage: .asProvided,
            numberFormatStyle: .number.precision(.fractionLength(longDistance ? 1 : 0))
        ))
    }

    static func coordinate(_ value: EvCoordinate) -> String {
        "\(value.latitude.formatted(.number.precision(.fractionLength(5)))), \(value.longitude.formatted(.number.precision(.fractionLength(5))))"
    }

    static func percent(_ basisPoints: Int64) -> String {
        (Double(basisPoints) / 10_000).formatted(.percent.precision(.fractionLength(0...1)))
    }

    static func energy(_ wattHours: Int64) -> String {
        Measurement(value: Double(wattHours) / 1_000, unit: UnitEnergy.kilowattHours)
            .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(1))))
    }

    static func duration(_ seconds: Int64) -> String {
        let minutes = max(1, (seconds + 59) / 60)
        return Duration.seconds(minutes * 60).formatted(.units(allowed: [.hours, .minutes], width: .abbreviated))
    }

    static func issue(_ code: EvIssueCode) -> String {
        switch code {
        case .snapshotMissing: return String(localized: "ev.trip.snapshot_missing")
        case .coverageUnknown: return String(localized: "ev.trip.coverage_unknown")
        case .noCompatibleCandidate: return String(localized: "android.ev_status_no_candidates")
        case .noRoadRoute: return String(localized: "ev.trip.no_route")
        case .routeBudgetExhausted: return String(localized: "ev.trip.route_budget")
        case .roadWarning: return String(localized: "ev.trip.road_warning")
        case .accessUnknown: return String(localized: "android.ev_access_unknown")
        case .hoursUnknown: return String(localized: "android.ev_hours_unknown")
        case .powerUnknown: return String(localized: "ev.site.power_unknown")
        case .tariffUnknown: return String(localized: "android.ev_tariff_unknown")
        case .stopUnreachable: return String(localized: "ev.trip.stop_unreachable")
        case .destinationUnreachable: return String(localized: "android.ev_status_insufficient_energy")
        case .stopLimitReached: return String(localized: "ev.trip.stop_limit")
        case .noEvidencedAlternative: return String(localized: "android.ev_no_alternative")
        case .vehicleConnectorMissing: return String(localized: "ev.trip.connector_required")
        case .batteryMissing, .invalidBattery: return String(localized: "ev.trip.battery_required")
        case .consumptionMissing, .invalidConsumption: return String(localized: "android.ev_invalid_consumption")
        case .invalidTargetSoc: return String(localized: "android.ev_invalid_target")
        case .invalidRoute: return String(localized: "ev.trip.invalid_route")
        case .energyUnknown: return String(localized: "ev.trip.energy_unknown")
        default: return code.rawValue
        }
    }

    static func status(_ status: EvPlanStatus) -> String {
        switch status {
        case .planned: return String(localized: "android.ev_status_planned")
        case .invalidInput: return String(localized: "android.ev_status_invalid")
        case .noRoute: return String(localized: "ev.trip.no_route")
        case .insufficientData: return String(localized: "ev.trip.insufficient_data")
        case .noCandidates: return String(localized: "android.ev_status_no_candidates")
        case .insufficientEnergy: return String(localized: "android.ev_status_insufficient_energy")
        case .budgetExhausted: return String(localized: "android.ev_status_budget")
        case .cancelled: return String(localized: "android.ev_status_cancelled")
        }
    }
}

/// MAP-28 owner-declared inputs over the MAP-27 planner. Routes come from MapKit
/// (online); chargers come only from the admitted catalog snapshot.
public struct EvTripPlannerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            EvPlannerContent(app: viewModel, ev: viewModel.evCatalog)
                .navigationTitle(String(localized: "ev.trip.title"))
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
                    }
                }
        }
    }
}

struct EvPlannerContent: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var ev: EvCatalogModel

    @AppStorage("ev-planner-connector") private var connectorRaw = ChargingConnector.ccs2.rawValue
    @AppStorage("ev-planner-soc") private var socText = "30"
    @AppStorage("ev-planner-reserve") private var reserveText = "10"
    @AppStorage("ev-planner-target") private var targetText = "80"
    @AppStorage("ev-planner-consumption") private var consumptionText = "180"
    @AppStorage("ev-planner-phev-share") private var phevShareText = "100"
    @State private var vehicleId = ""
    @State private var manualBatteryText = ""
    @State private var destinationText = ""
    @State private var originText = ""
    @State private var useCurrentLocation = true
    @State private var isPlanning = false
    @State private var plan: EvTripPlan?
    @State private var lastInput: EvTripInput?
    @State private var message: String?
    @State private var planTask: Task<Void, Never>?

    private var electricVehicles: [VehicleRecord] {
        app.snapshot.vehicles.filter { $0.propulsion.supportsElectric && $0.batteryCapacityWh > 0 }
    }

    private var selectedVehicle: VehicleRecord? {
        electricVehicles.first { $0.id == vehicleId } ?? electricVehicles.first
    }

    var body: some View {
        List {
            Section(String(localized: "android.enterprise_assets_kind_vehicle")) {
                if electricVehicles.isEmpty {
                    TextField(String(localized: "ev.trip.battery_kwh"), text: $manualBatteryText)
                        .decimalKeyboard()
                    Text(String(localized: "ev.trip.register_vehicle"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Picker(String(localized: "android.enterprise_assets_kind_vehicle"), selection: $vehicleId) {
                        ForEach(electricVehicles) { vehicle in
                            Text(vehicle.name).tag(vehicle.id)
                        }
                    }
                    if let vehicle = selectedVehicle {
                        Text(String(localized: "ev.trip.battery_value", defaultValue: "Usable battery: \(EvText.energy(vehicle.batteryCapacityWh))"))
                            .font(.caption)
                        if vehicle.propulsion == .plugInHybrid {
                            TextField(String(localized: "android.ev_phev_share"), text: $phevShareText)
                                .decimalKeyboard()
                        }
                    }
                }
                Picker(String(localized: "ev.trip.connector"), selection: $connectorRaw) {
                    ForEach(ChargingConnector.vehicleChoices) { connector in
                        Text(connector.label).tag(connector.rawValue)
                    }
                }
            }
            Section(String(localized: "ev.trip.energy")) {
                LabeledContent(String(localized: "android.ev_soc")) {
                    TextField("30", text: $socText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
                LabeledContent(String(localized: "android.ev_reserve")) {
                    TextField("10", text: $reserveText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
                LabeledContent(String(localized: "android.ev_target")) {
                    TextField("80", text: $targetText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
                LabeledContent(String(localized: "android.ev_consumption")) {
                    TextField("180", text: $consumptionText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
            }
            Section(String(localized: "ev.trip.route")) {
                Toggle(String(localized: "ev.trip.from_current"), isOn: $useCurrentLocation)
                if !useCurrentLocation {
                    TextField(String(localized: "ev.trip.origin"), text: $originText)
                }
                TextField(String(localized: "ev.trip.destination"), text: $destinationText)
                if isPlanning {
                    HStack {
                        ProgressView()
                        Text(String(localized: "ev.trip.calculating"))
                        Spacer()
                        Button(String(localized: "android.ev_cancel")) { planTask?.cancel() }
                    }
                } else {
                    Button(String(localized: "android.ev_plan")) { startPlan(failures: []) }
                        .disabled(destinationText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
                if ev.snapshot == nil {
                    Text(String(localized: "ev.trip.download_catalog"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let message {
                    Text(message).font(.caption).foregroundStyle(.orange)
                }
            }
            if let plan {
                planSections(plan)
            }
            Section {
                Text(String(localized: "ev.trip.assumptions"))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .onAppear {
            ev.loadIfNeeded()
            if vehicleId.isEmpty { vehicleId = electricVehicles.first?.id ?? "" }
        }
        .onDisappear { planTask?.cancel() }
    }

    @ViewBuilder
    private func planSections(_ plan: EvTripPlan) -> some View {
        Section(String(localized: "ev.trip.result", defaultValue: "Result: \(EvText.status(plan.status))")) {
            if let distance = plan.mainRoute?.road.distanceMeters {
                Text(String(localized: "ev.trip.distance", defaultValue: "Road distance: \(EvText.distance(Double(distance)))"))
            }
            if let energy = plan.mainRoute?.energyWh.value {
                Text(String(localized: "ev.trip.energy_estimate", defaultValue: "Estimated trip energy: \(EvText.energy(energy))"))
            }
            if let arrival = plan.arrivalSocBp?.value {
                Text(String(localized: "ev.trip.arrival", defaultValue: "Estimated arrival charge: \(EvText.percent(arrival))"))
                    .font(.headline)
            }
            Text(String(localized: "ev.trip.search_counts", defaultValue: "Stops: \(plan.stops.count.formatted()) · chargers checked: \(plan.candidateCount.formatted()) · route requests: \(plan.routeCalls.formatted())"))
                .font(.caption)
                .foregroundStyle(.secondary)
            if plan.scope == .electricPortionOnly {
                Text(String(localized: "ev.trip.electric_share"))
                    .font(.caption2)
            }
        }
        if !plan.stops.isEmpty {
            Section(String(localized: "ev.trip.suggested_stops")) {
                ForEach(Array(plan.stops.enumerated()), id: \.offset) { index, stop in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("\((index + 1).formatted()). \(stop.candidate.siteName)").font(.headline)
                        Text("\(stop.candidate.connector.label) · \(stop.candidate.powerW.map(EvText.power) ?? String(localized: "ev.site.power_unknown"))")
                            .font(.caption)
                        Text(String(localized: "ev.trip.stop_charge", defaultValue: "Arrive at \(EvText.percent(stop.arrivalSocBp)) · charge \(EvText.energy(stop.chargeEnergyWh)) to \(EvText.percent(stop.targetSocBp))"))
                            .font(.caption)
                        if let seconds = stop.minimumChargeSeconds.value {
                            Text(String(localized: "android.ev_charge_time", defaultValue: "Minimum charge time: \(EvText.duration(seconds))")).font(.caption)
                        }
                        if let alternative = stop.alternative {
                            Text(String(localized: "android.ev_alternative", defaultValue: "Alternative from failure: \(alternative.candidate.siteName)"))
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                Button(String(localized: "ev.trip.replan")) { replanWithoutFirstStop(plan) }
            }
        }
        let codes = uniqueCodes(plan.issues)
        if !codes.isEmpty {
            Section(String(localized: "ev.trip.warnings")) {
                ForEach(codes, id: \.self) { code in
                    Text(EvText.issue(code)).font(.caption)
                }
            }
        }
    }

    private func uniqueCodes(_ issues: [EvIssue]) -> [EvIssueCode] {
        var seen: [EvIssueCode] = []
        for issue in issues where !seen.contains(issue.code) { seen.append(issue.code) }
        return seen
    }

    private func batteryInput() -> EvBatteryInput? {
        let capacity: Int64?
        var portion: Int64?
        if let vehicle = selectedVehicle {
            capacity = vehicle.batteryCapacityWh
            if vehicle.propulsion == .plugInHybrid {
                guard let share = EvPlannerInputParser.percentBp(phevShareText, min: 1, max: 100) else { return nil }
                portion = max(1, share / 10)
            }
        } else {
            capacity = try? PortableMoney.parseBatteryCapacityWh(manualBatteryText)
        }
        guard let soc = EvPlannerInputParser.percentBp(socText, min: 0, max: 100),
              let reserve = EvPlannerInputParser.percentBp(reserveText, min: 0, max: 100) else { return nil }
        return EvBatteryInput(capacityWh: capacity, socBp: soc, reserveBp: reserve, usablePermille: portion)
    }

    private func startPlan(failures: [EvFailure]) {
        guard let battery = batteryInput() else {
            message = String(localized: "ev.trip.check_energy")
            return
        }
        guard let target = EvPlannerInputParser.percentBp(targetText, min: 0, max: 100) else {
            message = String(localized: "android.ev_invalid_target")
            return
        }
        guard let consumption = EvPlannerInputParser.consumptionMilli(consumptionText) else {
            message = String(localized: "android.ev_invalid_consumption")
            return
        }
        let connector = ChargingConnector(rawValue: connectorRaw)
        let destinationQuery = destinationText
        let originQuery = useCurrentLocation ? nil : originText
        message = nil
        plan = nil
        isPlanning = true
        planTask?.cancel()
        planTask = Task {
            defer { isPlanning = false }
            guard let origin = await resolveOrigin(originQuery) else {
                message = String(localized: "ev.trip.origin_unavailable")
                return
            }
            guard let destination = await search(destinationQuery) else {
                message = String(localized: "ev.trip.destination_not_found")
                return
            }
            var input = EvTripInput(
                origin: origin, destination: destination, departureAt: Date(), battery: battery,
                consumption: EvConsumptionInput(consMilliWhPerKm: consumption), vehicleConnector: connector,
                chargeTargetSocBp: target
            )
            input.failures = failures
            lastInput = input
            let planner = EvTripPlanner(router: MapKitEvRouter())
            let source = ev.candidateSource(connector: connector ?? .unknown)
            let result = await planner.plan(input, candidates: source)
            guard !Task.isCancelled else { return }
            plan = result
        }
    }

    /// The failed charger gives no energy: replan from its arrival charge (Android `markPrimaryFailed`).
    private func replanWithoutFirstStop(_ current: EvTripPlan) {
        guard let stop = current.stops.first, var input = lastInput else { return }
        let failurePoint = EvRoutePoint(latitude: stop.candidate.point.latitude, longitude: stop.candidate.point.longitude, label: stop.candidate.siteName)
        input.initialPoint = failurePoint
        input.initialSocBp = stop.arrivalSocBp
        input.failures.append(EvFailure(scope: .site, siteId: stop.candidate.siteId))
        input.departureAt = Date()
        let source = ev.candidateSource(connector: input.vehicleConnector ?? .unknown)
        let next = input
        lastInput = next
        plan = nil
        isPlanning = true
        planTask?.cancel()
        planTask = Task {
            defer { isPlanning = false }
            let result = await EvTripPlanner(router: MapKitEvRouter()).plan(next, candidates: source)
            guard !Task.isCancelled else { return }
            plan = result
        }
    }

    private func resolveOrigin(_ query: String?) async -> EvRoutePoint? {
        if let query {
            return await search(query)
        }
        guard let coordinate = await app.requestRouteOrigin() else { return nil }
        return EvRoutePoint(latitude: coordinate.latitude, longitude: coordinate.longitude, label: String(localized: "ev.trip.my_location"))
    }

    private func search(_ text: String) async -> EvRoutePoint? {
        let query = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty, !app.isOfflineSimulation else { return nil }
        let request = MKLocalSearch.Request()
        request.naturalLanguageQuery = query
        guard let response = try? await MKLocalSearch(request: request).start(),
              let item = response.mapItems.first else { return nil }
        let coordinate = item.placemark.coordinate
        guard CLLocationCoordinate2DIsValid(coordinate) else { return nil }
        return EvRoutePoint(latitude: coordinate.latitude, longitude: coordinate.longitude, label: item.name ?? query)
    }
}

extension View {
    /// Decimal keypad on iPhone and iPad; a no-op where the modifier does not exist.
    @ViewBuilder
    func decimalKeyboard() -> some View {
        #if os(iOS)
        self.keyboardType(.decimalPad)
        #else
        self
        #endif
    }
}
