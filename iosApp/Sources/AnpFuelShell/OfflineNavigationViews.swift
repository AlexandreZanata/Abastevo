import SwiftUI
import AnpFuelCore

/// B-BR-IOS-01 on iOS: turn-by-turn navigation without a connection uses the
/// offline maps the user downloads in Apple Maps (iOS 17+). The app never
/// downloads its own regional packages or claims offline availability it
/// cannot verify; it hands a routable station to Apple Maps.
public struct MapDownloadsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
    }

    /// Opens Apple Maps on the selected city so the user can download it.
    static func appleMapsURL(for municipality: Municipality?) -> URL? {
        guard let municipality else { return URL(string: "https://maps.apple.com/") }
        var components = URLComponents(string: "https://maps.apple.com/")
        components?.queryItems = [URLQueryItem(name: "q", value: "\(municipality.name), \(municipality.state.rawValue), Brasil")]
        return components?.url
    }

    public var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(String(localized: "map.offline.description"))
                    Label(String(localized: "map.offline.open_city_instruction"), systemImage: "1.circle")
                    Label(String(localized: "map.offline.apple_maps"), systemImage: "2.circle")
                    Label(String(localized: "map.offline.download_area"), systemImage: "3.circle")
                    Label(String(localized: "map.offline.station_instruction"), systemImage: "4.circle")
                    if let url = Self.appleMapsURL(for: viewModel.selectedMunicipality) {
                        Link(destination: url) {
                            Label(String(localized: "map.offline.open_city", defaultValue: "Open \(viewModel.municipalityLabel) in Apple Maps"), systemImage: "map")
                        }
                    }
                } header: {
                    Text(String(localized: "map.offline.navigate"))
                }

                Section {
                    Text(String(localized: "map.offline.unavailable"))
                        .font(.headline)
                    Text(String(localized: "map.offline.region_description"))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                } header: {
                    Text(String(localized: "map.offline.region_packages"))
                }
            }
            .navigationTitle(String(localized: "map.offline.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }
}
