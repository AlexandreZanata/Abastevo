import SwiftUI
import AnpFuelCore

/// Terms Gate & 5-Step Onboarding flow synchronized with `main` (P19-T02 / P35).
public struct TermsGateView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var hasReadSummary = true
    @State private var showFullPrivacy = false

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    HStack(spacing: 12) {
                        Text("A")
                            .font(.system(size: 28, weight: .black, design: .rounded))
                            .foregroundStyle(AbastevoTheme.accentDark)
                            .frame(width: 48, height: 48)
                            .background(AbastevoTheme.ink)
                            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                        VStack(alignment: .leading, spacing: 2) {
                            Text("abastevo")
                                .font(.title2.bold())
                            Text("Transparência nos preços de combustíveis")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                        }
                    }

                    EnergiaCard {
                        Text("Antes de começar")
                            .font(.headline)
                        Text("O abastevo reúne dados oficiais da ANP e observações enviadas pela comunidade de forma separada, transparente e gratuita.")
                            .font(.body)
                        Label("Dados oficiais da ANP nunca se misturam com preços da comunidade.", systemImage: "checkmark.seal.fill")
                            .font(.subheadline)
                            .foregroundStyle(.blue)
                        Label("Sem ranking pago: pagamento nunca altera confiança ou posição de postos.", systemImage: "shield.checkered")
                            .font(.subheadline)
                            .foregroundStyle(AbastevoTheme.accentLight)
                        Label("GPS sob demanda (one-shot): sua posição exata nunca é publicada nem rastreada em segundo plano.", systemImage: "location.fill")
                            .font(.subheadline)
                        Label("Fotos enviadas expiram localmente em 24 horas e nunca vão para backup em nuvem.", systemImage: "clock.arrow.circlepath")
                            .font(.subheadline)
                    }

                    EnergiaCard {
                        Text("Privacidade e Termos de Uso")
                            .font(.headline)
                        Text("Operado por ZAFLAS INOVA SIMPLES (I.S.), CNPJ 67.852.667/0001-93. Conta gratuita opcional: você pode explorar preços, mapas e histórico anonimamente e offline.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                        Button("Ler política de privacidade completa") {
                            showFullPrivacy = true
                        }
                        .font(.footnote.bold())
                    }

                    Toggle("Li e concordo com os Termos de Uso e Política de Privacidade do abastevo", isOn: $hasReadSummary)
                        .font(.subheadline)

                    Button {
                        IosHaptics.success()
                        viewModel.acceptTerms()
                    } label: {
                        Text("Aceitar e continuar")
                            .font(.headline)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 14)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(!hasReadSummary)
                }
                .padding(20)
            }
            .navigationTitle("Termos e Privacidade")
            .sheet(isPresented: $showFullPrivacy) {
                PrivacyDocumentSheet()
            }
        }
    }
}

public struct OnboardingFlowView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var step = 0
    @State private var citySearch = ""

    public var body: some View {
        NavigationStack {
            VStack(spacing: 16) {
                ProgressView(value: Double(step + 1), total: 5.0)
                    .tint(AbastevoTheme.accentLight)
                    .padding(.horizontal, 20)
                    .padding(.top, 12)

                TabView(selection: $step) {
                    onboardingCard(
                        icon: "fuelpump.fill",
                        title: "Bem-vindo ao abastevo",
                        subtitle: "Compare preços da ANP e relatos da comunidade com foto da placa do posto.",
                        detail: "Acompanhe gasolina, etanol, diesel, GNV, GLP e pontos de recarga elétrica em todo o Brasil."
                    )
                    .tag(0)

                    onboardingCard(
                        icon: "arrow.down.circle.fill",
                        title: "Funciona 100% offline",
                        subtitle: "Baixe pesquisas semanais da ANP e mapas regionais para consultar mesmo sem sinal.",
                        detail: "Contribuições feitas sem internet ficam seguras na fila local e enviam quando a conexão retornar."
                    )
                    .tag(1)

                    onboardingCard(
                        icon: "checkmark.shield.fill",
                        title: "Regras da comunidade",
                        subtitle: "Dados oficiais da ANP e preços enviados por motoristas ficam sempre separados.",
                        detail: "Conta gratuita sem anúncios intrusivos, com veículos, histórico e diário de abastecimentos incluídos."
                    )
                    .tag(2)

                    onboardingCard(
                        icon: "calendar",
                        title: "Pesquisa oficial da ANP",
                        subtitle: "As datas das coletas disponíveis aparecem na ficha de cada posto.",
                        detail: "Os preços da comunidade aparecem em uma seção própria."
                    )
                    .tag(3)

                    VStack(alignment: .leading, spacing: 12) {
                        Text("Escolha sua cidade")
                            .font(.title3.bold())

                        TextField("Buscar município, UF ou código IBGE", text: $citySearch)
                            .textFieldStyle(.roundedBorder)

                        let cities = viewModel.municipalityCatalog.search(query: citySearch)
                        if viewModel.municipalityCatalog.count == 0 {
                            Text("Catálogo de municípios indisponível.")
                                .foregroundStyle(.secondary)
                        } else if cities.isEmpty {
                            Text("Nenhum município encontrado.")
                                .foregroundStyle(.secondary)
                        }
                        ScrollView {
                            LazyVStack(spacing: 6) {
                                ForEach(cities) { city in
                                    Button {
                                        viewModel.selectMunicipality(city)
                                    } label: {
                                        HStack {
                                            Text(city.displayTitle)
                                            Spacer()
                                            if viewModel.selectedMunicipality?.ibgeCode == city.ibgeCode {
                                                Image(systemName: "checkmark.circle.fill")
                                                    .foregroundStyle(AbastevoTheme.accentLight)
                                            }
                                        }
                                        .padding(10)
                                        .background(Color.secondary.opacity(0.08))
                                        .clipShape(RoundedRectangle(cornerRadius: 10))
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }
                    .padding(20)
                    .tag(4)
                }
                #if os(iOS)
                .tabViewStyle(.page(indexDisplayMode: .never))
                #endif

                HStack(spacing: 12) {
                    if step > 0 {
                        Button("Voltar") {
                            withAnimation { step -= 1 }
                        }
                        .buttonStyle(.bordered)
                    }
                    Spacer()
                    if step < 4 {
                        Button("Próximo") {
                            withAnimation { step += 1 }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                    } else {
                        Button("Começar a usar") {
                            IosHaptics.success()
                            viewModel.completeOnboarding()
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.accentLight)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 20)
            }
            .navigationTitle("Passo \(step + 1) de 5")
        }
    }

    private func onboardingCard(icon: String, title: String, subtitle: String, detail: String) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            Spacer()
            Image(systemName: icon)
                .font(.system(size: 46))
                .foregroundStyle(AbastevoTheme.accentLight)
            Text(title)
                .font(.title2.bold())
            Text(subtitle)
                .font(.headline)
                .foregroundStyle(.primary)
            Text(detail)
                .font(.body)
                .foregroundStyle(.secondary)
            Spacer()
        }
        .padding(24)
    }
}

public struct PrivacyDocumentSheet: View {
    @Environment(\.dismiss) private var dismiss

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text("Política de Privacidade — abastevo")
                        .font(.title3.bold())
                    Text("Controlador: ZAFLAS INOVA SIMPLES (I.S.), CNPJ 67.852.667/0001-93, Sorriso/MT.")
                        .font(.subheadline.bold())
                    Text("1. Dados públicos vs. privados: Consultas a médias da ANP e postos podem ser feitas sem conta. Dados de veículos e diário ficam armazenados localmente e sincronizados apenas quando autorizado.\n\n2. Localização one-shot: A localização do aparelho é consultada apenas mediante ação explícita do usuário para localizar sua cidade ou validar proximidade pontual. O histórico contínuo de GPS nunca é coletado em segundo plano para anúncios.\n\n3. Fotos e evidências: Fotografias de placas são reprocessadas para JPEG compacto sem metadados EXIF/GPS e expiram localmente em 24 horas.\n\n4. Direitos LGPD: Você pode exportar seus dados em JSON ou excluir sua conta e apagar todos os registros locais a qualquer momento na aba Perfil.")
                        .font(.body)
                }
                .padding(20)
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}
