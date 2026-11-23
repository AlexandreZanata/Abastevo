import SwiftUI
import Combine
import AnpFuelCore

/// Profile Tab + Auth + Account Management + Vehicles & Ledger + Driver Hub
/// + Enterprise Suite + Arcade Games + Training Consent + Settings/Help/Privacy.
public struct ProfileRootView: View {
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack(path: $viewModel.profilePath) {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    accountHeaderCard
                    driverAndVehiclesSection
                    enterpriseAndGamesSection
                    appearanceAndLanguageSection
                    expertAndPrivacySection
                }
                .padding(16)
            }
        }
    }

    private var accountHeaderCard: some View {
        EnergiaCard {
            if let alias = viewModel.signedInAlias {
                HStack {
                    Image(systemName: "person.crop.circle.fill")
                        .font(.largeTitle)
                        .foregroundStyle(AbastevoTheme.accentLight)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(alias).font(.headline)
                        Text("Conta gratuita ativa · Participação independente de pagamento")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                }
                Button("Gerenciar conta, exportação e bloqueios") {
                    viewModel.activeSheet = .accountManagement
                }
                .buttonStyle(.bordered)
                .frame(maxWidth: .infinity, alignment: .leading)
                Button {
                    viewModel.activeSheet = .premium
                } label: {
                    Label("Abastevo Premium", systemImage: "crown.fill")
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .buttonStyle(.bordered)
            } else {
                Text("Exploração como Convidado")
                    .font(.headline)
                Text("Você pode consultar preços ANP, mapas e veículos offline sem conta. Entre gratuitamente para sincronizar e comentar.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Button {
                    viewModel.activeSheet = .auth
                } label: {
                    Text("Entrar ou criar conta gratuita")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.ink)
            }
        }
    }

    private var driverAndVehiclesSection: some View {
        EnergiaCard {
            Text("Motorista, Veículos & Energia")
                .font(.headline)
            profileNavRow(icon: "car.fill", title: "Meus Veículos & Livro de Abastecimentos", subtitle: "Abastecimentos, recargas, despesas, indicadores e nuvem") {
                viewModel.activeSheet = .vehiclesLedger
            }
            Divider()
            profileNavRow(icon: "steeringwheel", title: "Central do Motorista (Driver Hub)", subtitle: "Turnos, distância, ganho bruto vs combustível e lucro líquido") {
                viewModel.activeSheet = .driverHub
            }
            Divider()
            profileNavRow(icon: "chart.line.uptrend.xyaxis", title: "Histórico Semanal & Alertas de Queda", subtitle: "Série oficial da ANP por combustível") {
                viewModel.activeSheet = .priceHistory
            }
        }
    }

    private var enterpriseAndGamesSection: some View {
        EnergiaCard {
            Text("Empresas & Minijogos")
                .font(.headline)
            profileNavRow(icon: "building.2.fill", title: "Abastevo Empresas", subtitle: "Convites, viagens e despesas da empresa") {
                viewModel.activeSheet = .enterpriseHub
            }
            Divider()
            profileNavRow(icon: "gamecontroller.fill", title: "Minijogos (Corrida & Voo Econômico)", subtitle: "Recordes: \(viewModel.snapshot.bestFuelRunScore) / \(viewModel.snapshot.bestFuelFlightScore) pts") {
                viewModel.activeSheet = .arcadeGames
            }
        }
    }

    private var appearanceAndLanguageSection: some View {
        EnergiaCard {
            Text("Aparência e Idioma")
                .font(.headline)
            Toggle("Tema escuro", isOn: Binding(
                get: { viewModel.snapshot.darkThemeEnabled ?? false },
                set: { viewModel.setTheme(dark: $0) }
            ))
            Picker("Idioma", selection: Binding(
                get: { viewModel.currentLocale },
                set: { viewModel.setLocale($0) }
            )) {
                ForEach(SupportedAppLocale.allCases) { loc in
                    Text("\(loc.flagEmoji) \(loc.nativeName)").tag(loc)
                }
            }
        }
    }

    private var expertAndPrivacySection: some View {
        EnergiaCard {
            Text("Ferramentas, Privacidade & Cache")
                .font(.headline)
            profileNavRow(icon: "camera.badge.ellipsis", title: "Consentimento para Treinamento de OCR", subtitle: "Coleta desativada") {
                viewModel.activeSheet = .trainingConsent
            }
            Divider()
            profileNavRow(icon: "calendar", title: "Semanas da Pesquisa ANP", subtitle: viewModel.surveyWeekLabel) {
                viewModel.activeSheet = .weekPicker
            }
            Divider()
            profileNavRow(icon: "map.fill", title: "Mapas Offline Regionais", subtitle: "Indisponível no iOS") {
                viewModel.activeSheet = .mapDownloads
            }
            Divider()
            profileNavRow(icon: "gearshape.fill", title: "Configurações & Ambiente de Rede", subtitle: viewModel.snapshot.apiEnvironment.labelPtBr) {
                viewModel.activeSheet = .settings
            }
            Divider()
            profileNavRow(icon: "lock.shield.fill", title: "Privacidade, LGPD & Termos", subtitle: "Direitos do titular e regras comunitárias") {
                viewModel.activeSheet = .privacy
            }
            Divider()
            profileNavRow(icon: "questionmark.circle", title: "Ajuda & Regras da Comunidade", subtitle: "Transparência ANP vs Comunidade") {
                viewModel.activeSheet = .help
            }
        }
    }

    private func profileNavRow(icon: String, title: String, subtitle: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 12) {
                Image(systemName: icon)
                    .foregroundStyle(AbastevoTheme.accentLight)
                    .frame(width: 24)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.subheadline.weight(.semibold))
                    Text(subtitle).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Image(systemName: "chevron.right")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .padding(.vertical, 4)
        }
        .buttonStyle(.plain)
    }
}

public struct AuthSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var email = ""
    @State private var code = ""
    @State private var username = ""
    @State private var accountKeyInput = ""
    @State private var issuedKey: AuthKeyIssued?

    public var body: some View {
        NavigationStack {
            Form {
                AppleSignInSection(viewModel: viewModel)

                Section("E-mail + Código de 6 dígitos") {
                    TextField("seu@email.com", text: $email)
                    Button("Enviar código") {
                        viewModel.requestEmailOtp(email: email)
                    }
                    TextField("Código (6 dígitos)", text: $code)
                    Button("Validar código e entrar") {
                        viewModel.verifyEmailOtp(email: email, code: code)
                    }
                }

                Section("Conta Anônima com Chave de Recuperação") {
                    TextField("Apelido (3 a 20 letras/números)", text: $username)
                    Button("Gerar nova conta com chave") {
                        Task {
                            issuedKey = await viewModel.createAnonymousKeyAccount(username: username)
                        }
                    }
                    if let issued = issuedKey {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Usuário: \(issued.username)").font(.subheadline.bold())
                            Text("Chave única (salve agora): \(issued.accountKey)")
                                .font(.caption.monospaced())
                                .foregroundStyle(.orange)
                        }
                    }
                    TextField("Ou entrar com chave existente", text: $accountKeyInput)
                    Button("Desbloquear com chave") {
                        viewModel.loginWithAccountKey(accountKeyInput)
                    }
                }

                if viewModel.isAuthWorking || viewModel.authStatusMessage != nil {
                    Section("Status") {
                        if viewModel.isAuthWorking {
                            ProgressView()
                        }
                        if let msg = viewModel.authStatusMessage {
                            Text(msg).font(.subheadline)
                        }
                    }
                }
            }
            .disabled(viewModel.isAuthWorking)
            .navigationTitle("Conta Gratuita")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct AccountManagementSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var exportedJson: String?
    @State private var confirmDelete = false

    public var body: some View {
        NavigationStack {
            List {
                if let alias = viewModel.signedInAlias {
                    Section("Identidade Conectada") {
                        Text(alias)
                        Button("Exportar dados locais deste aparelho (JSON)") {
                            exportedJson = viewModel.exportMyDataJson()
                        }
                        if let json = exportedJson {
                            Text(String(json.prefix(500)) + (json.count > 500 ? "..." : ""))
                                .font(.caption2.monospaced())
                        }
                    }
                }

                Section("Usuários Bloqueados (\(viewModel.snapshot.blockedUsers.count))") {
                    if viewModel.snapshot.blockedUsers.isEmpty {
                        Text("Nenhum usuário bloqueado.").foregroundStyle(.secondary)
                    } else {
                        ForEach(viewModel.snapshot.blockedUsers) { user in
                            HStack {
                                Text(user.displayAlias)
                                Spacer()
                                Button("Desbloquear") {
                                    viewModel.unblockUser(userId: user.id)
                                }
                                .font(.caption)
                            }
                        }
                    }
                }

                if viewModel.currentAccountIdOrNil != nil {
                    Section("Sessão e Exclusão") {
                        Button("Sair e limpar dados locais privados") {
                            viewModel.logout()
                            dismiss()
                        }
                        Button("Solicitar exclusão da conta", role: .destructive) {
                            confirmDelete = true
                        }
                    }
                }
                if let message = viewModel.authStatusMessage {
                    Section {
                        Text(message)
                    }
                }
            }
            .navigationTitle("Gerenciar Conta")
            .alert("Excluir conta?", isPresented: $confirmDelete) {
                Button("Solicitar exclusão", role: .destructive) {
                    viewModel.deleteAccountAndPurge()
                }
                Button("Cancelar", role: .cancel) {}
            } message: {
                Text("A solicitação encerra seu acesso. Após a confirmação do servidor, o app limpará os dados privados locais da conta.")
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct DriverHubSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var driverVehicleId: String?
    @State private var startKmText = ""
    @State private var endKmText = ""
    @State private var grossReaisText = ""
    @State private var fuelReaisText = ""
    @State private var minutesText = ""
    @State private var shiftError: String?

    public var body: some View {
        let activeShift = viewModel.snapshot.driverShifts.first(where: { $0.isActive })
        let closedShifts = viewModel.snapshot.driverShifts.filter { !$0.isActive }
        NavigationStack {
            List {
                Section("Turno manual neste aparelho") {
                    if let active = activeShift {
                        Text("Turno em andamento · Início Km \(active.startOdometerKm)")
                            .font(.headline)
                            .foregroundStyle(AbastevoTheme.accentLight)
                        if let started = formattedDate(active.startedAtLabel) {
                            Text("Início em \(started)").foregroundStyle(.secondary)
                        }
                        TextField("Km final", text: $endKmText)
                        TextField("Ganho bruto (R$)", text: $grossReaisText)
                        TextField("Combustível gasto (R$)", text: $fuelReaisText)
                        TextField("Duração informada (minutos)", text: $minutesText)
                        Button("Encerrar turno") {
                            finishShift(active)
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                    } else if viewModel.snapshot.vehicles.isEmpty {
                        Text("Cadastre um veículo para iniciar um turno.").foregroundStyle(.secondary)
                    } else {
                        Picker("Veículo", selection: $driverVehicleId) {
                            Text("Selecione um veículo").tag(nil as String?)
                            ForEach(viewModel.snapshot.vehicles) { vehicle in
                                Text(vehicle.name).tag(Optional(vehicle.id))
                            }
                        }
                        TextField("Hodômetro inicial (km)", text: $startKmText)
                        Button("Iniciar turno de trabalho", action: startShift)
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.accentLight)
                        .disabled(driverVehicleId == nil)
                    }
                    if let shiftError {
                        Text(shiftError).font(.caption).foregroundStyle(.red)
                    }
                }

                Section("Turnos Encerrados") {
                    ForEach(closedShifts) { s in
                        VStack(alignment: .leading, spacing: 4) {
                            if let balance = s.netEarningsCentavos {
                                Text("Saldo registrado \(AbastevoTheme.formatCentavos(balance))").font(.headline)
                            } else {
                                Text("Saldo indisponível").font(.headline)
                            }
                            if let distance = s.distanceKm {
                                Text("\(distance) km rodados")
                            }
                            if let ended = formattedDate(s.endedAtLabel) {
                                Text("Encerrado em \(ended)")
                            }
                            Text("Bruto \(AbastevoTheme.formatCentavos(s.grossEarningsCentavos)) · Combustível \(AbastevoTheme.formatCentavos(s.fuelCostCentavos))")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .navigationTitle("Central do Motorista")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }

    private func startShift() {
        guard let vehicleId = driverVehicleId,
              let startKm = Int64(startKmText.trimmingCharacters(in: .whitespacesAndNewlines)) else {
            shiftError = "Informe o hodômetro inicial em quilômetros."
            return
        }
        switch viewModel.startDriverShift(vehicleId: vehicleId, startOdometerKm: startKm) {
        case .success:
            startKmText = ""
            shiftError = nil
        case .failure(let error):
            shiftError = error.localizedDescription
        }
    }

    private func finishShift(_ active: DriverShiftRecord) {
        guard let endKm = Int64(endKmText.trimmingCharacters(in: .whitespacesAndNewlines)),
              let minutes = Int(minutesText.trimmingCharacters(in: .whitespacesAndNewlines)),
              let gross = try? PortableMoney.parseBrlCentavos(grossReaisText),
              let fuel = try? PortableMoney.parseBrlCentavos(fuelReaisText) else {
            shiftError = "Confira o hodômetro, a duração e os valores do turno."
            return
        }
        switch viewModel.finishActiveDriverShift(shiftId: active.id, endOdometerKm: endKm, grossCentavos: gross, fuelCentavos: fuel, minutes: minutes) {
        case .success:
            endKmText = ""
            grossReaisText = ""
            fuelReaisText = ""
            minutesText = ""
            shiftError = nil
        case .failure(let error):
            shiftError = error.localizedDescription
        }
    }

    private func formattedDate(_ timestamp: String?) -> String? {
        guard let timestamp, let date = ISO8601DateFormatter().date(from: timestamp) else { return nil }
        return date.formatted(date: .abbreviated, time: .shortened)
    }
}

public struct ArcadeGamesSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var gameKind: ArcadeGameKind = .fuelRun
    @State private var gameState = ArcadeState()
    @State private var tick = 0
    @State private var isPlaying = false

    private let timer = Timer.publish(every: 0.14, on: .main, in: .common).autoconnect()

    public var body: some View {
        NavigationStack {
            VStack(spacing: 16) {
                Picker("Jogo", selection: $gameKind) {
                    ForEach(ArcadeGameKind.allCases) { kind in
                        Text(kind.titlePtBr).tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)

                HStack {
                    Text("Pontos: \(gameState.score)").font(.headline)
                    Spacer()
                    Text("Combustível: \(Int(gameState.fuelPercent))%").font(.subheadline.bold())
                        .foregroundStyle(gameState.fuelPercent < 25 ? .red : AbastevoTheme.accentLight)
                }
                .padding(.horizontal, 20)

                ZStack {
                    RoundedRectangle(cornerRadius: 16)
                        .fill(Color.secondary.opacity(0.12))
                        .frame(height: 240)

                    VStack {
                        HStack {
                            Text(gameKind == .fuelRun ? "Pista 1" : "Alto").font(.caption2)
                            Spacer()
                            Text(gameKind == .fuelRun ? "Pista 2" : "Médio").font(.caption2)
                            Spacer()
                            Text(gameKind == .fuelRun ? "Pista 3" : "Baixo").font(.caption2)
                        }
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 24)

                        Spacer()
                        HStack {
                            ForEach(0..<3, id: \.self) { laneIdx in
                                VStack {
                                    if gameState.obstacleLane == laneIdx {
                                        Image(systemName: "exclamationmark.triangle.fill")
                                            .foregroundStyle(.orange)
                                            .offset(y: CGFloat(gameState.obstacleProgress * 110))
                                    }
                                    if gameState.pickupLane == laneIdx {
                                        Image(systemName: "fuelpump.fill")
                                            .foregroundStyle(AbastevoTheme.accentLight)
                                            .offset(y: CGFloat(gameState.pickupProgress * 110))
                                    }
                                    Spacer()
                                    if gameState.lane == laneIdx {
                                        Image(systemName: gameKind == .fuelRun ? "car.fill" : "airplane")
                                            .font(.title)
                                            .foregroundStyle(.primary)
                                    }
                                }
                                .frame(maxWidth: .infinity)
                            }
                        }
                        .padding(.horizontal, 20)
                        .padding(.bottom, 16)
                    }
                }
                .padding(.horizontal, 16)

                if gameState.isGameOver {
                    Text("Fim de jogo! Pontuação final: \(gameState.score)")
                        .font(.headline)
                        .foregroundStyle(.orange)
                }

                HStack(spacing: 16) {
                    Button("◀ Esquerda") {
                        gameState.lane = max(0, gameState.lane - 1)
                        gameState.altitude = min(0.9, gameState.altitude + 0.18)
                    }
                    .buttonStyle(.bordered)

                    Button(isPlaying && !gameState.isGameOver ? "Pausar" : "Jogar / Reiniciar") {
                        if gameState.isGameOver {
                            gameState = ArcadeState()
                        }
                        isPlaying.toggle()
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)

                    Button("Direita ▶") {
                        gameState.lane = min(2, gameState.lane + 1)
                        gameState.altitude = max(0.1, gameState.altitude - 0.15)
                    }
                    .buttonStyle(.bordered)
                }

                Spacer()
            }
            .padding(.top, 12)
            .navigationTitle("Minijogos Abastevo")
            .onReceive(timer) { _ in
                guard isPlaying && !gameState.isGameOver else { return }
                tick += 1
                gameState.step(kind: gameKind, tick: tick)
                if gameState.isGameOver {
                    if gameKind == .fuelRun && gameState.score > viewModel.snapshot.bestFuelRunScore {
                        viewModel.snapshot.bestFuelRunScore = gameState.score
                        viewModel.persist()
                    } else if gameKind == .fuelFlight && gameState.score > viewModel.snapshot.bestFuelFlightScore {
                        viewModel.snapshot.bestFuelFlightScore = gameState.score
                        viewModel.persist()
                    }
                }
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct TrainingConsentSheetView: View {
    @Environment(\.dismiss) private var dismiss

    public var body: some View {
        NavigationStack {
            List {
                Section {
                    Text("A coleta para treinamento está desativada. O gerenciamento dessa preferência ainda está indisponível no iOS.")
                }
                if TrainingTermsContract.verifiedDigestMatches() {
                    Section("Termo de consentimento") {
                        Text(TrainingTermsContract.fullTextPtBr)
                            .font(.footnote)
                    }
                }
            }
            .navigationTitle("Treinamento de OCR")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct SettingsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            Form {
                Section("Rede e Servidor") {
                    Picker("Ambiente API", selection: Binding(
                        get: { viewModel.snapshot.apiEnvironment },
                        set: { viewModel.setApiEnvironment($0) }
                    )) {
                        ForEach(ApiEnvironment.allCases, id: \.self) { env in
                            Text(env.labelPtBr).tag(env)
                        }
                    }
                    .disabled(!viewModel.canSwitchApiEnvironment)
                    if !viewModel.canSwitchApiEnvironment {
                        Text("Encerre a sessão para trocar de servidor.")
                            .font(.caption)
                    }
                    Toggle("Simular modo offline", isOn: $viewModel.isOfflineSimulation)
                }

                Section("Armazenamento Offline & Privacidade") {
                    Text("Uso aproximado em disco: \(viewModel.estimatedStorageKb()) KB (fora do backup iCloud)")
                        .font(.caption)
                    Button("Limpar cache local, incluindo fotos temporárias", role: .destructive) {
                        viewModel.clearOfflineCache()
                    }
                }

                Section("Versão iOS") {
                    Text("abastevo iOS · Versão 3.1.18 (build 30118)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle("Configurações")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct HelpSheetView: View {
    @Environment(\.dismiss) private var dismiss

    public var body: some View {
        NavigationStack {
            List {
                Section("Como o abastevo funciona") {
                    Text("1. Explore preços oficiais da ANP e compare com relatos com foto da comunidade.")
                    Text("2. Contribua fotografando a placa do posto: o OCR roda localmente no iPhone.")
                    Text("3. Nunca há ranking pago ou venda de confiança.")
                }
                Section("Contato & Código Aberto") {
                    Text("Operador: ZAFLAS INOVA SIMPLES (I.S.) — CNPJ 67.852.667/0001-93")
                        .font(.caption)
                }
            }
            .navigationTitle("Ajuda & Regras")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}
