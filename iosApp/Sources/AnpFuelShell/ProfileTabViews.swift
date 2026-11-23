import SwiftUI
import Combine
import AuthenticationServices
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
            profileNavRow(icon: "car.fill", title: "Meus Veículos & Livro de Abastecimentos", subtitle: "\(viewModel.snapshot.vehicles.count)/\(VehicleRecord.maxFreeVehicles) veículos gratuitos · Wh/km e R$/km") {
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
            profileNavRow(icon: "building.2.fill", title: "Abastevo Empresas", subtitle: viewModel.snapshot.enterpriseOrganizations.first?.name ?? "Convites, frotas, viagens, despesas e checklists") {
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
            profileNavRow(icon: "camera.badge.ellipsis", title: "Consentimento para Treinamento de OCR", subtitle: viewModel.currentTrainingConsent.granted ? "Ativo (revogável a qualquer momento)" : "Desativado por padrão (RVA08)") {
                viewModel.activeSheet = .trainingConsent
            }
            Divider()
            profileNavRow(icon: "calendar", title: "Semanas da Pesquisa ANP", subtitle: viewModel.selectedWeek.label) {
                viewModel.activeSheet = .weekPicker
            }
            Divider()
            profileNavRow(icon: "map.fill", title: "Mapas Offline Regionais", subtitle: "\(viewModel.snapshot.downloadedMapRegions.count) região(ões) baixada(s)") {
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
                Section("Sign in with Apple (Nativo iOS)") {
                    SignInWithAppleButton(.signIn) { request in
                        request.requestedScopes = [.email]
                    } onCompletion: { result in
                        switch result {
                        case .success(let auth):
                            if let cred = auth.credential as? ASAuthorizationAppleIDCredential {
                                let tokenStr = cred.identityToken.flatMap { String(data: $0, encoding: .utf8) } ?? ""
                                viewModel.completeSignInWithApple(userIdentifier: cred.user, identityToken: tokenStr)
                                dismiss()
                            }
                        case .failure:
                            viewModel.authStatusMessage = "Autenticação Apple não concluída."
                        }
                    }
                    .frame(height: 44)
                }

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
                        issuedKey = viewModel.createAnonymousKeyAccount(username: username)
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

                if let msg = viewModel.authStatusMessage {
                    Section("Status") {
                        Text(msg).font(.subheadline)
                    }
                }
            }
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
                Section("Identidade Conectada") {
                    Text(viewModel.signedInAlias ?? "Conta conectada")
                    Button("Exportar meus dados (JSON)") {
                        exportedJson = viewModel.exportMyDataJson()
                    }
                    if let json = exportedJson {
                        Text(String(json.prefix(500)) + (json.count > 500 ? "..." : ""))
                            .font(.caption2.monospaced())
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
                                    viewModel.unblockUser(user.id)
                                }
                                .font(.caption)
                            }
                        }
                    }
                }

                Section("Sessão e Exclusão") {
                    Button("Sair e limpar dados locais privados") {
                        viewModel.logout()
                        dismiss()
                    }
                    Button("Excluir conta permanentemente", role: .destructive) {
                        confirmDelete = true
                    }
                }
            }
            .navigationTitle("Gerenciar Conta")
            .alert("Excluir conta?", isPresented: $confirmDelete) {
                Button("Excluir agora", role: .destructive) {
                    viewModel.deleteAccountAndPurge()
                    dismiss()
                }
                Button("Cancelar", role: .cancel) {}
            } message: {
                Text("Seus tokens no Keychain e dados privados locais serão apagados permanentemente.")
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct VehiclesAndLedgerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var name = ""
    @State private var propulsion: Propulsion = .flex
    @State private var primaryFuel: FuelProduct = .gasolineRegular
    @State private var tankLiters = "50"
    @State private var batteryKwh = "52"
    @State private var errorBanner: String?

    @State private var entryKind: LedgerEntryKind = .refuel
    @State private var entryKmText = "42500"
    @State private var entryQtyText = "40000"
    @State private var entryCostText = "23960"
    @State private var entryNote = ""

    public var body: some View {
        NavigationStack {
            List {
                Section("Meus Veículos (\(viewModel.snapshot.vehicles.count)/\(VehicleRecord.maxFreeVehicles) gratuitos)") {
                    ForEach(viewModel.snapshot.vehicles) { v in
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(v.name).font(.headline)
                                Spacer()
                                Text(v.propulsion.labelPtBr).font(.caption).foregroundStyle(.secondary)
                            }
                            Text("Combustível: \(v.primaryFuel.displayNamePtBr) · Tanque: \(v.tankCapacityMilliLiters / 1000) L")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .onDelete { idxSet in
                        for idx in idxSet {
                            viewModel.removeVehicle(viewModel.snapshot.vehicles[idx].id)
                        }
                    }
                }

                if viewModel.snapshot.vehicles.count < VehicleRecord.maxFreeVehicles {
                    Section("Adicionar Veículo") {
                        TextField("Apelido do carro / moto", text: $name)
                        Picker("Propulsão", selection: $propulsion) {
                            ForEach(Propulsion.allCases) { p in
                                Text(p.labelPtBr).tag(p)
                            }
                        }
                        if propulsion.supportsFuel {
                            Picker("Combustível principal", selection: $primaryFuel) {
                                ForEach(FuelProduct.allCases) { f in
                                    Text(f.displayNamePtBr).tag(f)
                                }
                            }
                            TextField("Tanque (litros, ex.: 50)", text: $tankLiters)
                        }
                        if propulsion.supportsElectric {
                            TextField("Bateria (kWh, ex.: 52)", text: $batteryKwh)
                        }
                        if let err = errorBanner {
                            Text(err).font(.caption).foregroundStyle(.red)
                        }
                        Button("Salvar veículo") {
                            switch viewModel.addVehicle(name: name, propulsion: propulsion, primaryFuel: primaryFuel, tankLitersText: tankLiters, batteryKwhText: batteryKwh) {
                            case .success:
                                name = ""
                                errorBanner = nil
                            case .failure(let msg):
                                errorBanner = msg
                            }
                        }
                    }
                }

                Section("Novo Registro no Diário (Livro de Bordo)") {
                    Picker("Tipo", selection: $entryKind) {
                        ForEach(LedgerEntryKind.allCases, id: \.self) { k in
                            Text(k.labelPtBr).tag(k)
                        }
                    }
                    TextField("Hodômetro (km)", text: $entryKmText)
                    TextField("Quantidade (mL ou Wh)", text: $entryQtyText)
                    TextField("Total (centavos, ex.: 23960)", text: $entryCostText)
                    TextField("Observação", text: $entryNote)
                    Button("Registrar lançamento") {
                        let vId = viewModel.snapshot.vehicles.first?.id ?? "default"
                        let km = Int64(entryKmText) ?? 0
                        let qty = Int64(entryQtyText) ?? 0
                        let centavos = Int64(entryCostText) ?? 0
                        viewModel.addLedgerEntry(vehicleId: vId, kind: entryKind, odometerKm: km, quantityMilli: qty, totalCentavos: centavos, note: entryNote)
                        entryNote = ""
                    }
                }

                Section("Indicadores Exatos (Energia / Custo por km)") {
                    let energyResult = PremiumEnergyCalculator.compute(
                        PremiumEnergyCalculator.IntervalInput(
                            purchasedWh: 40_000,
                            costCentavos: 4_000,
                            distanceMetres: 150_000,
                            hasEnergy: true,
                            complete: true
                        )
                    )
                    Text("Exemplo EE-23 exato: \(energyResult.whPerKm) Wh/km · \(energyResult.centavosPerKm) centavos/km")
                        .font(.caption.monospaced())
                    ForEach(viewModel.snapshot.ledgerEntries) { entry in
                        HStack {
                            VStack(alignment: .leading) {
                                Text("\(entry.kind.labelPtBr) · \(entry.dateLabel)").font(.subheadline.bold())
                                Text("Km \(entry.odometerKm) · \(entry.note)")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            Text(AbastevoTheme.formatCentavos(entry.totalCentavos)).font(.subheadline.bold())
                        }
                    }
                }
            }
            .navigationTitle("Veículos & Diário")
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

    @State private var startKmText = "80000"
    @State private var endKmText = "80180"
    @State private var grossReaisText = "320,00"
    @State private var fuelReaisText = "78,50"
    @State private var minutesText = "360"

    public var body: some View {
        let activeShift = viewModel.snapshot.driverShifts.first(where: { $0.isActive })
        let closedShifts = viewModel.snapshot.driverShifts.filter { !$0.isActive }
        NavigationStack {
            List {
                Section("Turno Atual") {
                    if let active = activeShift {
                        Text("Turno em andamento · Início Km \(active.startOdometerKm)")
                            .font(.headline)
                            .foregroundStyle(AbastevoTheme.accentLight)
                        TextField("Km final", text: $endKmText)
                        TextField("Ganho bruto (R$)", text: $grossReaisText)
                        TextField("Combustível gasto (R$)", text: $fuelReaisText)
                        TextField("Duração (minutos)", text: $minutesText)
                        Button("Encerrar turno") {
                            let gross = parseReaisToCentavos(grossReaisText)
                            let fuel = parseReaisToCentavos(fuelReaisText)
                            viewModel.finishActiveDriverShift(
                                endOdometerKm: Int64(endKmText) ?? active.startOdometerKm,
                                grossCentavos: gross,
                                fuelCentavos: fuel,
                                expenseCentavos: 0,
                                minutes: Int(minutesText) ?? 60
                            )
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                    } else {
                        TextField("Hodômetro inicial (km)", text: $startKmText)
                        Button("Iniciar turno de trabalho") {
                            let vId = viewModel.snapshot.vehicles.first?.id ?? "driver-vehicle"
                            viewModel.startDriverShift(vehicleId: vId, startOdometerKm: Int64(startKmText) ?? 0)
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.accentLight)
                    }
                }

                Section("Turnos Encerrados") {
                    ForEach(closedShifts) { s in
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Líquido: \(AbastevoTheme.formatCentavos(s.netEarningsCentavos))")
                                .font(.headline)
                            Text("\(s.distanceKm) km rodados · Bruto \(AbastevoTheme.formatCentavos(s.grossEarningsCentavos)) · Combustível \(AbastevoTheme.formatCentavos(s.fuelCostCentavos))")
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

    private func parseReaisToCentavos(_ text: String) -> Int64 {
        let cleaned = text.replacingOccurrences(of: ",", with: ".").trimmingCharacters(in: .whitespacesAndNewlines)
        return Int64(((Double(cleaned) ?? 0.0) * 100.0).rounded())
    }
}

public struct EnterpriseHubSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var inviteCode = ""
    @State private var tripOdometer = "124500"
    @State private var trackingConsent = false
    @State private var checklistBrakeOk = true
    @State private var checklistTiresOk = true
    @State private var expenseDesc = ""
    @State private var expenseCentavos = "4500"

    public var body: some View {
        NavigationStack {
            List {
                Section("Contexto Corporativo & Convite") {
                    if let org = viewModel.snapshot.enterpriseOrganizations.first {
                        Text(org.name).font(.headline)
                        Text("CNPJ: \(org.cnpj) · Papel: \(org.roleTitle) · Status: \(org.status)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    HStack {
                        TextField("Código de convite (ex.: EMP-2026)", text: $inviteCode)
                        Button("Vincular") {
                            viewModel.joinSampleEnterprise(inviteCode: inviteCode)
                            inviteCode = ""
                        }
                    }
                }

                Section("Missões & Viagens Corporativas") {
                    ForEach(viewModel.snapshot.enterpriseTrips) { trip in
                        VStack(alignment: .leading, spacing: 6) {
                            Text("\(trip.code) · \(trip.vehiclePlate) (\(trip.status.rawValue))")
                                .font(.headline)
                            Text("\(trip.originLabel) → \(trip.destinationLabel)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            if trip.status == .assigned {
                                Toggle("Autorizar telemetria desta viagem nesta tela", isOn: $trackingConsent)
                                    .font(.caption)
                                TextField("Hodômetro saída", text: $tripOdometer)
                                Button("Iniciar viagem") {
                                    viewModel.startEnterpriseTrip(trip.id, odometerKm: Int64(tripOdometer) ?? 0, explicitTrackingGrant: trackingConsent)
                                }
                                .buttonStyle(.borderedProminent)
                            } else if trip.status == .active {
                                if let code = trip.fuelingAuthCode {
                                    Text("Autorização de abastecimento: \(code)")
                                        .font(.caption.monospaced())
                                        .foregroundStyle(AbastevoTheme.accentLight)
                                }
                                Button("Encerrar viagem") {
                                    viewModel.finishEnterpriseTrip(trip.id, endOdometerKm: (trip.startOdometerKm ?? 0) + 120)
                                }
                                .buttonStyle(.bordered)
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }

                Section("Checklist & Despesas") {
                    Toggle("Freios e luzes verificados", isOn: $checklistBrakeOk)
                    Toggle("Pneus e calibração verificados", isOn: $checklistTiresOk)
                    Button("Registrar checklist") {
                        let run = EnterpriseChecklistRun(
                            vehiclePlate: "ABV-3A21",
                            itemsChecked: ["Freios", "Pneus"],
                            passed: checklistBrakeOk && checklistTiresOk,
                            createdAtLabel: "Agora"
                        )
                        viewModel.snapshot.enterpriseChecklists.insert(run, at: 0)
                        viewModel.persist()
                    }
                    TextField("Descrição da despesa (pedágio/estacionamento)", text: $expenseDesc)
                    Button("Adicionar despesa (\(expenseCentavos) centavos)") {
                        let item = EnterpriseExpenseItem(
                            category: "Viagem",
                            descriptionText: expenseDesc.isEmpty ? "Pedágio" : expenseDesc,
                            amountCentavos: Int64(expenseCentavos) ?? 4500,
                            dateLabel: "Hoje"
                        )
                        viewModel.snapshot.enterpriseExpenses.insert(item, at: 0)
                        expenseDesc = ""
                        viewModel.persist()
                    }
                }
            }
            .navigationTitle("Abastevo Empresas")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
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
    @ObservedObject var viewModel: AppStateViewModel

    @State private var explicitlyChecked = false
    @State private var feedbackMsg: String?

    public var body: some View {
        let current = viewModel.currentTrainingConsent
        let digestOk = TrainingTermsContract.verifiedDigestMatches()
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    EnergiaCard {
                        Text("Estado atual: \(current.granted ? "CONSENTIDO (Coleta bloqueada nesta versão)" : "DESATIVADO")")
                            .font(.headline)
                            .foregroundStyle(current.granted ? AbastevoTheme.accentLight : .primary)
                        Text("Versão: \(TrainingTermsContract.version) · SHA-256 verificado: \(digestOk ? "OK" : "Divergente")")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        if current.pendingSync {
                            Text("Sincronização pendente com o servidor.")
                                .font(.caption)
                                .foregroundStyle(.orange)
                        }
                    }

                    EnergiaCard {
                        Text(TrainingTermsContract.fullTextPtBr)
                            .font(.footnote)
                    }

                    if viewModel.currentAccountIdOrNil == nil {
                        Text("Entre em uma conta gratuita na aba Perfil para registrar ou revogar esta preferência.")
                            .font(.subheadline)
                            .foregroundStyle(.orange)
                    } else if current.granted {
                        Button("Revogar consentimento imediatamente (OFF)") {
                            viewModel.revokeTrainingConsentImmediately()
                            feedbackMsg = "Preferência gravada como DESATIVADA localmente."
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(.red)
                    } else {
                        Toggle("Li integralmente os termos e autorizo opcionalmente o registro desta preferência.", isOn: $explicitlyChecked)
                            .font(.subheadline)
                        Button("Confirmar consentimento opcional") {
                            if viewModel.grantTrainingConsent(explicitlyChecked: explicitlyChecked) {
                                feedbackMsg = "Preferência registrada (coleta continua desligada nesta versão)."
                            } else {
                                feedbackMsg = "Marque a caixa de confirmação para registrar."
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                        .disabled(!explicitlyChecked || !digestOk)
                    }

                    if let msg = feedbackMsg {
                        Text(msg).font(.caption).foregroundStyle(.secondary)
                    }
                }
                .padding(16)
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
                    Toggle("Simular modo offline", isOn: $viewModel.isOfflineSimulation)
                }

                Section("Armazenamento Offline & Privacidade") {
                    Text("Uso aproximado em disco: \(viewModel.estimatedStorageKb()) KB (fora do backup iCloud)")
                        .font(.caption)
                    Button("Limpar cache de mapas e semanas", role: .destructive) {
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
