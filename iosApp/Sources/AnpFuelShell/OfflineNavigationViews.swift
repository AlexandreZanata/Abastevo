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
                    Text("No iPhone, a navegação curva a curva sem internet usa os mapas offline do Apple Maps (iOS 17 ou posterior). Baixe a sua região uma vez e abra as rotas do abastevo no Apple Maps.")
                    Label("Abra a cidade no Apple Maps pelo botão abaixo.", systemImage: "1.circle")
                    Label("No Apple Maps, toque na sua foto ou iniciais, depois em Mapas Offline e Baixar Novo Mapa.", systemImage: "2.circle")
                    Label("Ajuste a área da região e toque em Baixar.", systemImage: "3.circle")
                    Label("Sem internet, escolha um posto no abastevo e toque em Abrir no Apple Maps.", systemImage: "4.circle")
                    if let url = Self.appleMapsURL(for: viewModel.selectedMunicipality) {
                        Link(destination: url) {
                            Label("Abrir \(viewModel.municipalityLabel) no Apple Maps", systemImage: "map")
                        }
                    }
                } header: {
                    Text("Navegar sem internet")
                }

                Section {
                    Text("Mapas regionais indisponíveis")
                        .font(.headline)
                    Text("Os pacotes de mapa do abastevo usam o motor de mapas do Android. No iOS, o app não baixa pacotes próprios e não indica disponibilidade offline que não consegue verificar.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                } header: {
                    Text("Pacotes regionais do abastevo")
                }
            }
            .navigationTitle("Navegação offline")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}
