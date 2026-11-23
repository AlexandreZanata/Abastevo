import SwiftUI
import PhotosUI
import AnpFuelCore
#if canImport(UIKit)
import UIKit
#endif

/// Abastevo Empresas: invitations, company selection and the driver's company operations.
public struct EnterpriseHubSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            EnterpriseHubContent(
                enterprise: viewModel.enterprise,
                signedIn: viewModel.currentAccountIdOrNil != nil,
                openSignIn: { viewModel.activeSheet = .auth }
            )
            .navigationTitle("Abastevo Empresas")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

struct EnterpriseHubContent: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let signedIn: Bool
    let openSignIn: () -> Void

    var body: some View {
        List {
            Section {
                Label("Ambiente de teste com dados sintéticos", systemImage: "testtube.2")
                    .font(.subheadline.weight(.semibold))
                Text("Os registros da empresa ficam separados dos seus dados pessoais e são enviados assinados pela chave deste aparelho.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if !signedIn {
                Section {
                    Text("Entre na sua conta pessoal do Abastevo para consultar suas empresas e convites.")
                    Button("Entrar na minha conta", action: openSignIn)
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                }
            } else if !enterprise.isOriginSupported {
                Section {
                    Text("O Abastevo Empresas exige um servidor HTTPS publicado. Este ambiente de rede não oferece o modo empresa.")
                }
            } else if let active = enterprise.active {
                EnterpriseCompanySections(enterprise: enterprise, active: active)
            } else {
                EnterpriseContextSections(enterprise: enterprise)
            }
            if let notice = enterprise.notice {
                Section("Aviso") {
                    Text(notice).font(.footnote)
                }
            }
        }
        .onAppear {
            enterprise.revalidate()
            if signedIn && enterprise.active == nil && !enterprise.contextsLoaded {
                enterprise.refreshContexts()
            }
        }
    }
}

// MARK: - Context selection

struct EnterpriseContextSections: View {
    @ObservedObject var enterprise: EnterpriseCoordinator

    var body: some View {
        Section {
            Text("Escolha uma empresa ou continue no modo pessoal.")
                .font(.footnote)
                .foregroundStyle(.secondary)
            if enterprise.isBusy {
                ProgressView("Consultando suas empresas…")
            }
            ForEach(enterprise.contexts) { context in
                Button {
                    enterprise.select(context)
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Entrar em \(context.name)").font(.headline)
                        if let state = context.state, state != "ACTIVE" {
                            Text(EnterpriseLabels.organizationState(state))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                .disabled(enterprise.isBusy)
            }
            if enterprise.contextsLoaded && enterprise.contexts.isEmpty && !enterprise.isBusy {
                Text("Nenhuma empresa disponível nesta consulta.").foregroundStyle(.secondary)
            }
            Button("Atualizar empresas") {
                enterprise.refreshContexts()
            }
            .disabled(enterprise.isBusy)
            if let error = enterprise.accessError {
                Text(EnterpriseCoordinator.accessMessage(error))
                    .font(.footnote)
                    .foregroundStyle(AbastevoTheme.errorRed)
            }
        } header: {
            Text("Escolher espaço")
        } footer: {
            Text("A seleção de empresa exige confirmação online. Os registros continuam na empresa em que foram criados ao trocar de espaço.")
        }
        Section {
            NavigationLink {
                EnterpriseInvitationView(enterprise: enterprise)
            } label: {
                Label("Aceitar um convite", systemImage: "envelope.open")
            }
        }
    }
}

// MARK: - Active company

struct EnterpriseCompanySections: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let active: EnterpriseActiveCompany

    var body: some View {
        let snapshot = enterprise.snapshot
        let openTrip = snapshot?.openTrip
        Section {
            Text("Você está em \(active.summary.name)").font(.headline)
            if active.locked {
                Text("Acesso encerrado: registros ainda não enviados continuam guardados; nenhum registro novo é aceito.")
                    .font(.footnote)
                    .foregroundStyle(AbastevoTheme.errorRed)
            } else {
                Text("Acesso confirmado nesta consulta.").font(.footnote).foregroundStyle(.secondary)
            }
            Button("Usar modo pessoal") {
                enterprise.leaveCompany()
            }
            Button("Trocar espaço") {
                enterprise.leaveCompany()
                enterprise.refreshContexts()
            }
        } header: {
            Text("Modo empresa")
        } footer: {
            Text("Os registros continuam na empresa em que foram criados ao trocar de espaço.")
        }

        if !active.locked {
            Section {
                if let trip = openTrip {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Viagem em andamento").font(.headline)
                        Text(trip.assetLabel)
                        Text("Início \(EnterpriseLabels.dateTime(millis: trip.startedAtMillis))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    NavigationLink("Encerrar viagem") {
                        EnterpriseTripFinishView(enterprise: enterprise, trip: trip)
                    }
                } else {
                    NavigationLink("Iniciar viagem") {
                        EnterpriseTripStartView(enterprise: enterprise)
                    }
                }
                NavigationLink("Registrar despesa") {
                    EnterpriseExpenseView(enterprise: enterprise)
                }
            } header: {
                Text("Operação")
            } footer: {
                Text("Rastreamento desligado no iOS: o aplicativo não registra trajeto nem localização durante a viagem.")
            }

            Section {
                if enterprise.isRefreshingAssets {
                    ProgressView("Atualizando seus ativos…")
                }
                if enterprise.snapshot?.reference?.forbidden == true {
                    Text("O servidor não libera a lista de ativos para o seu acesso.").foregroundStyle(.secondary)
                } else if enterprise.assets.isEmpty {
                    Text("Nenhum ativo atribuído a você nesta empresa.").foregroundStyle(.secondary)
                }
                if !enterprise.assets.isEmpty && !enterprise.assetsConfirmed {
                    Text("Lista não confirmada nas últimas 24 horas. Atualize antes de escolher um ativo.")
                        .font(.footnote)
                        .foregroundStyle(AbastevoTheme.warningLight)
                }
                ForEach(enterprise.assets) { asset in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(EnterpriseCoordinator.assetLabel(asset)).font(.subheadline.weight(.semibold))
                        Text(EnterpriseLabels.assetStatus(asset, nowMillis: EnterpriseLabels.nowMillis()))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                Button("Atualizar ativos") {
                    enterprise.refreshAssets()
                }
                .disabled(enterprise.isRefreshingAssets)
            } header: {
                Text("Meus ativos")
            }
        }

        Section {
            let unsent = snapshot?.unsentCount ?? 0
            if enterprise.isDelivering {
                ProgressView("Enviando registros…")
            }
            Text(unsent == 0
                 ? String(localized: "Nenhum registro aguardando envio.")
                 : String(localized: "\(unsent) registro(s) aguardando envio."))
            NavigationLink("Fila de envio") {
                EnterpriseQueueView(enterprise: enterprise)
            }
            Button("Enviar agora") {
                enterprise.deliverNow()
            }
            .disabled(enterprise.isDelivering)
        } header: {
            Text("Sincronização")
        } footer: {
            Text("Sem conexão, os registros ficam guardados neste aparelho e são enviados depois. Sair da conta apaga estes registros do aparelho.")
        }
    }
}

// MARK: - Invitation

struct EnterpriseInvitationView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var code = ""
    @State private var consented = false

    var body: some View {
        Form {
            if let result = enterprise.invitationResult {
                Section {
                    if result.isActive {
                        Text("O servidor confirmou seu acesso à empresa.")
                    } else if result.awaitsConfirmation {
                        Text("Convite aceito. Seu acesso ainda depende da confirmação de um responsável da empresa.")
                    } else {
                        Text("Convite recusado. Nenhum acesso foi concedido.")
                    }
                    Button("Ver minhas empresas") {
                        enterprise.restartInvitation()
                        enterprise.refreshContexts()
                        dismiss()
                    }
                    Button("Usar outro código") {
                        enterprise.restartInvitation()
                        code = ""
                        consented = false
                    }
                }
            } else if let preview = enterprise.invitationPreview {
                Section {
                    Text(preview.organizationName).font(.title3.weight(.semibold))
                    Text(preview.legalEntityVerified ? "Dados jurídicos verificados" : "Dados jurídicos não verificados")
                        .foregroundStyle(.secondary)
                } header: {
                    Text("Convite da empresa")
                }
                Section("Acesso oferecido") {
                    ForEach(preview.grants, id: \.self) { grant in
                        Text("\(EnterpriseCoordinator.roleLabel(grant.role)) · \(grant.scope == "ORG" ? String(localized: "Empresa") : String(localized: "Próprios registros"))")
                    }
                }
                Section {
                    Text("A empresa verá seu cadastro de funcionário, o apelido da conta que aceitou o convite e suas ações nesta empresa. Ela não verá suas credenciais pessoais, veículos pessoais, histórico privado ou outras empresas às quais você tem acesso.")
                        .font(.footnote)
                    if preview.confirmationRequired {
                        Text("Este acesso só será ativado depois da confirmação de um responsável da empresa.")
                            .font(.footnote)
                    }
                    Toggle("Aceito participar desta empresa e compartilhar os dados descritos acima.", isOn: $consented)
                    Button(enterprise.invitationRetryAccept == true ? "Tentar a mesma decisão novamente" : "Aceitar convite") {
                        enterprise.decideInvitation(accept: true)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(!consented || enterprise.invitationBusy || enterprise.invitationRetryAccept == false)
                    Button(enterprise.invitationRetryAccept == false ? "Tentar a mesma decisão novamente" : "Recusar convite") {
                        enterprise.decideInvitation(accept: false)
                    }
                    .disabled(enterprise.invitationBusy || enterprise.invitationRetryAccept == true)
                    Button("Usar outro código") {
                        enterprise.restartInvitation()
                        consented = false
                    }
                    .disabled(enterprise.invitationBusy)
                }
            } else {
                Section {
                    Text("Digite o código que o responsável da empresa enviou para você.")
                    TextField("Código do convite", text: $code)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled(true)
                    Button("Consultar convite") {
                        enterprise.previewInvitation(code: code)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(code.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || enterprise.invitationBusy)
                } footer: {
                    Text("Conecte-se à internet para consultar ou responder a um convite. Nenhuma decisão será confirmada sem conexão.")
                }
            }
            if enterprise.invitationBusy {
                Section {
                    ProgressView("Aguardando a confirmação do servidor…")
                }
            }
            if let error = enterprise.invitationError {
                Section {
                    Text(EnterpriseCoordinator.accessMessage(error)).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
        }
        .navigationTitle("Convite da empresa")
    }
}

// MARK: - Trips

/// Reading input shared by the trip start and finish: a meter reading or a written reason.
struct EnterpriseEvidenceFields: View {
    let meters: [EnterpriseReferenceMeter]
    @Binding var meterId: String?
    @Binding var valueText: String
    @Binding var reasonText: String

    var body: some View {
        Picker("Leitura", selection: $meterId) {
            Text("Sem leitura (informar motivo)").tag(nil as String?)
            ForEach(meters) { meter in
                Text(meter.kind == .odometer ? "Hodômetro (km)" : "Horímetro (horas)").tag(Optional(meter.id))
            }
        }
        if let selected = meters.first(where: { $0.id == meterId }) {
            TextField(selected.kind == .odometer ? "Leitura em km, por exemplo 184320,5" : "Leitura em horas, por exemplo 3600,25", text: $valueText)
                .keyboardType(.decimalPad)
        } else {
            TextField("Motivo da falta de leitura", text: $reasonText, axis: .vertical)
        }
    }

    /// The evidence, or nil with the message to show.
    static func evidence(meters: [EnterpriseReferenceMeter], meterId: String?, valueText: String, reasonText: String) -> (EnterpriseMeterEvidence?, String?) {
        if let meter = meters.first(where: { $0.id == meterId }) {
            let parsedValue = meter.kind == .odometer ? EnterpriseValues.parseKilometres(valueText) : EnterpriseValues.parseHours(valueText)
            guard let value = parsedValue else {
                return (nil, meter.kind == .odometer
                    ? String(localized: "Informe a leitura em km com até três casas decimais.")
                    : String(localized: "Informe a leitura em horas com até duas casas decimais."))
            }
            if let modulus = meter.modulus, value >= modulus {
                return (nil, String(localized: "A leitura passa do limite deste medidor."))
            }
            return (.reading(meterId: meter.id, segmentId: meter.segmentId, kind: meter.kind, readingId: EnterpriseValues.newId(),
                             value: value, modulus: meter.modulus), nil)
        }
        let reason = reasonText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard EnterpriseValues.isValidText(reason, maxScalars: EnterpriseValues.maxReasonScalars) else {
            return (nil, String(localized: "Informe o motivo da falta de leitura (até 280 caracteres)."))
        }
        return (.missing(reason: reason), nil)
    }
}

struct EnterpriseTripStartView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var assetId: String?
    @State private var meterId: String?
    @State private var valueText = ""
    @State private var reasonText = ""
    @State private var reviewAccepted = false
    @State private var applicable: EnterpriseChecklistApplicable?
    @State private var checklistMessage: String?
    @State private var checklistConsulted = false
    @State private var checklistFailed = false
    @State private var skipChecklist = false
    @State private var departureRunId: String?
    @State private var blocking = false
    @State private var showChecklist = false
    @State private var checklistStartedAt = ""
    @State private var loading = false
    @State private var error: String?

    private var asset: EnterpriseReferenceAsset? {
        assetId.flatMap { enterprise.findAsset($0) }
    }

    private var needsReview: Bool {
        guard let asset else { return false }
        return !asset.isActive || asset.assignmentEnded(nowMillis: EnterpriseLabels.nowMillis()) || !enterprise.assetsConfirmed
    }

    private var checklistReady: Bool {
        if blocking { return false }
        if departureRunId != nil { return true }
        if checklistConsulted && applicable?.required == false { return true }
        return checklistFailed && skipChecklist
    }

    var body: some View {
        Form {
            Section("Ativo") {
                Picker("Ativo", selection: $assetId) {
                    Text("Selecione um ativo").tag(nil as String?)
                    ForEach(enterprise.assets) { item in
                        Text(EnterpriseCoordinator.assetLabel(item)).tag(Optional(item.id))
                    }
                }
                .onChange(of: assetId) { _, _ in
                    meterId = asset?.meters.first?.id
                    resetChecklist()
                }
                if let asset, needsReview {
                    Text(EnterpriseLabels.assetStatus(asset, nowMillis: EnterpriseLabels.nowMillis()))
                        .font(.footnote)
                        .foregroundStyle(AbastevoTheme.warningLight)
                    Toggle("Entendi e quero usar este ativo; o servidor decide", isOn: $reviewAccepted)
                }
            }
            if let asset {
                Section("Checklist de saída") {
                    if loading {
                        ProgressView("Consultando checklist…")
                    } else if departureRunId != nil {
                        Text(blocking ? "Checklist bloqueante: não inicie a viagem e fale com o gestor da frota." : "Checklist registrado neste aparelho.")
                            .foregroundStyle(blocking ? AbastevoTheme.errorRed : .primary)
                    } else if checklistConsulted && applicable?.required == false {
                        Text("Nenhum checklist obrigatório para este ativo.").foregroundStyle(.secondary)
                    } else {
                        Button("Consultar checklist de saída") {
                            consultChecklist(asset.id)
                        }
                    }
                    if let checklistMessage {
                        Text(checklistMessage).font(.footnote).foregroundStyle(.secondary)
                    }
                    if checklistFailed {
                        Toggle("Iniciar sem checklist consultado; o servidor decide", isOn: $skipChecklist)
                    }
                }
                Section("Leitura de saída") {
                    EnterpriseEvidenceFields(meters: asset.meters, meterId: $meterId, valueText: $valueText, reasonText: $reasonText)
                }
            }
            Section {
                Button("Iniciar viagem", action: start)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                    .disabled(asset == nil || (needsReview && !reviewAccepted) || !checklistReady)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            } footer: {
                Text("A viagem é guardada neste aparelho e enviada assinada. O iOS não registra trajeto nem localização.")
            }
        }
        .navigationTitle("Iniciar viagem")
        .sheet(isPresented: $showChecklist) {
            if let applicable {
                NavigationStack {
                    EnterpriseChecklistView(enterprise: enterprise, applicable: applicable, startedAt: checklistStartedAt) { runId, outcome in
                        departureRunId = runId
                        blocking = outcome == .blocking
                        showChecklist = false
                    }
                }
            }
        }
    }

    private func resetChecklist() {
        applicable = nil
        checklistMessage = nil
        checklistConsulted = false
        checklistFailed = false
        skipChecklist = false
        departureRunId = nil
        blocking = false
        reviewAccepted = false
    }

    private func consultChecklist(_ id: String) {
        loading = true
        checklistMessage = nil
        Task {
            let result = await enterprise.loadChecklist(assetId: id, context: "DEPARTURE")
            loading = false
            switch result {
            case .success(let value):
                applicable = value
                checklistConsulted = true
                checklistFailed = false
                if value.required {
                    checklistStartedAt = EnterpriseValues.formatInstant(epochMillis: EnterpriseLabels.nowMillis())
                    showChecklist = true
                }
            case .failure(let failure):
                checklistFailed = true
                checklistMessage = failure.message
            }
        }
    }

    private func start() {
        guard let asset else { return }
        let parsed = EnterpriseEvidenceFields.evidence(meters: asset.meters, meterId: meterId, valueText: valueText, reasonText: reasonText)
        guard let evidence = parsed.0 else {
            error = parsed.1
            return
        }
        switch enterprise.startTrip(assetId: asset.id, evidence: evidence, departureRunId: departureRunId) {
        case .success:
            dismiss()
        case .failure(let failure):
            error = failure.message
        }
    }
}

struct EnterpriseTripFinishView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let trip: EnterpriseTripRecord
    @Environment(\.dismiss) private var dismiss
    @State private var meterId: String?
    @State private var valueText = ""
    @State private var reasonText = ""
    @State private var error: String?

    private var meters: [EnterpriseReferenceMeter] {
        enterprise.findAsset(trip.assetId)?.meters ?? []
    }

    var body: some View {
        Form {
            Section("Viagem") {
                Text(trip.assetLabel).font(.headline)
                Text("Início \(EnterpriseLabels.dateTime(millis: trip.startedAtMillis))").foregroundStyle(.secondary)
            }
            Section("Leitura final") {
                EnterpriseEvidenceFields(meters: meters, meterId: $meterId, valueText: $valueText, reasonText: $reasonText)
            }
            Section {
                Button("Encerrar viagem", action: finish)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
        }
        .navigationTitle("Encerrar viagem")
        .onAppear {
            if meterId == nil {
                meterId = trip.startMeterId ?? meters.first?.id
            }
        }
    }

    private func finish() {
        let parsed = EnterpriseEvidenceFields.evidence(meters: meters, meterId: meterId, valueText: valueText, reasonText: reasonText)
        guard let evidence = parsed.0 else {
            error = parsed.1
            return
        }
        switch enterprise.finishTrip(evidence: evidence) {
        case .success:
            dismiss()
        case .failure(let failure):
            error = failure.message
        }
    }
}

// MARK: - Checklist

struct EnterpriseChecklistView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let applicable: EnterpriseChecklistApplicable
    let startedAt: String
    let onDone: (String, EnterpriseChecklistOutcome) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var answers: [EnterpriseChecklistAnswer?]
    @State private var notes: [String]
    @State private var acknowledged = false
    @State private var error: String?

    init(enterprise: EnterpriseCoordinator, applicable: EnterpriseChecklistApplicable, startedAt: String,
         onDone: @escaping (String, EnterpriseChecklistOutcome) -> Void) {
        self.enterprise = enterprise
        self.applicable = applicable
        self.startedAt = startedAt
        self.onDone = onDone
        _answers = State(initialValue: Array(repeating: nil, count: applicable.items.count))
        _notes = State(initialValue: Array(repeating: "", count: applicable.items.count))
    }

    private var hasCriticalIssue: Bool {
        applicable.items.indices.contains { index in
            applicable.items[index].critical && answers[index] == EnterpriseChecklistAnswer.notOk
        }
    }

    var body: some View {
        Form {
            Section {
                Text(applicable.name ?? "").font(.headline)
                Text(applicable.policyMode == "BLOCK"
                     ? "Itens críticos com problema bloqueiam a saída."
                     : "Itens críticos com problema exigem sua confirmação.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            ForEach(applicable.items) { item in
                Section {
                    Text(item.label)
                    if item.critical {
                        Text("Item crítico").font(.caption).foregroundStyle(AbastevoTheme.errorRed)
                    }
                    Picker("Resposta", selection: answerBinding(item.position - 1)) {
                        Text("Sem resposta").tag(nil as EnterpriseChecklistAnswer?)
                        Text("OK").tag(Optional(EnterpriseChecklistAnswer.ok))
                        Text("Com problema").tag(Optional(EnterpriseChecklistAnswer.notOk))
                        if item.optional {
                            Text("Não se aplica").tag(Optional(EnterpriseChecklistAnswer.notApplicable))
                        }
                    }
                    if answers[item.position - 1] == EnterpriseChecklistAnswer.notOk {
                        TextField("Observação (opcional, até 140 caracteres)", text: noteBinding(item.position - 1))
                        if item.photoOnNotOk {
                            Text("Este item exige foto quando há problema; o envio de foto do checklist ainda não está disponível no iOS.")
                                .font(.caption)
                                .foregroundStyle(AbastevoTheme.warningLight)
                        }
                    }
                }
            }
            Section {
                if hasCriticalIssue && applicable.policyMode == "WARN" {
                    Toggle("Estou ciente dos itens críticos com problema", isOn: $acknowledged)
                }
                Button("Registrar checklist", action: submit)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            } footer: {
                Text("O resultado informado é conferido pelo servidor.")
            }
        }
        .navigationTitle("Checklist de saída")
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("Cancelar") { dismiss() }
            }
        }
    }

    private func answerBinding(_ index: Int) -> Binding<EnterpriseChecklistAnswer?> {
        Binding(
            get: { index < answers.count ? answers[index] : nil },
            set: { value in
                if index < answers.count { answers[index] = value }
            }
        )
    }

    private func noteBinding(_ index: Int) -> Binding<String> {
        Binding(
            get: { index < notes.count ? notes[index] : "" },
            set: { value in
                if index < notes.count { notes[index] = value }
            }
        )
    }

    private func submit() {
        switch enterprise.completeChecklist(applicable, answers: answers, notes: notes, acknowledged: acknowledged, startedAt: startedAt) {
        case .success(let value):
            onDone(value.runId, value.outcome)
        case .failure(let failure):
            error = failure.message
        }
    }
}

// MARK: - Expenses

struct EnterpriseExpenseView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var assetId: String?
    @State private var category = "TOLL"
    @State private var paidText = ""
    @State private var companyText = ""
    @State private var payer: EnterpriseExpensePayer = .company
    @State private var receipt: Data?
    @State private var pickerItem: PhotosPickerItem?
    @State private var preparing = false
    @State private var showCamera = false
    @State private var error: String?

    var body: some View {
        Form {
            Section("Despesa") {
                Picker("Ativo", selection: $assetId) {
                    Text("Selecione um ativo").tag(nil as String?)
                    ForEach(enterprise.assets) { item in
                        Text(EnterpriseCoordinator.assetLabel(item)).tag(Optional(item.id))
                    }
                }
                Picker("Categoria", selection: $category) {
                    ForEach(EnterpriseOperationBuilder.expenseCategories, id: \.self) { code in
                        Text(EnterpriseCoordinator.categoryLabel(code)).tag(code)
                    }
                }
                TextField("Valor pago (R$)", text: $paidText)
                    .keyboardType(.decimalPad)
                TextField("Valor da empresa (R$, padrão: valor pago)", text: $companyText)
                    .keyboardType(.decimalPad)
                Picker("Quem pagou", selection: $payer) {
                    Text("Empresa").tag(EnterpriseExpensePayer.company)
                    Text("Eu (reembolso)").tag(EnterpriseExpensePayer.employee)
                }
            }
            Section {
                if preparing {
                    ProgressView("Preparando a foto…")
                } else if let receipt {
                    #if canImport(UIKit)
                    if let image = UIImage(data: receipt) {
                        Image(uiImage: image)
                            .resizable()
                            .scaledToFit()
                            .frame(maxHeight: 180)
                    }
                    #endif
                    Text("Foto pronta (\(receipt.count / 1024) KB), sem localização nem metadados.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Button("Remover foto", role: .destructive) {
                        self.receipt = nil
                    }
                } else {
                    PhotosPicker(selection: $pickerItem, matching: .images) {
                        Label("Escolher foto do comprovante", systemImage: "photo")
                    }
                    #if canImport(UIKit)
                    Button {
                        showCamera = true
                    } label: {
                        Label("Fotografar comprovante", systemImage: "camera")
                    }
                    #endif
                }
            } header: {
                Text("Comprovante (opcional)")
            } footer: {
                Text("A foto é enviada depois que o servidor recebe a despesa.")
            }
            Section {
                Button("Salvar despesa", action: submit)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(assetId == nil || preparing)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
        }
        .navigationTitle("Registrar despesa")
        .onChange(of: pickerItem) { _, newItem in
            guard let item = newItem else { return }
            preparing = true
            Task {
                let data = try? await item.loadTransferable(type: Data.self)
                await prepare(data)
                pickerItem = nil
            }
        }
        #if canImport(UIKit)
        .sheet(isPresented: $showCamera) {
            IosCameraCaptureView { data in
                Task { await prepare(data) }
            }
        }
        #endif
    }

    private func prepare(_ data: Data?) async {
        preparing = true
        guard let data else {
            preparing = false
            error = String(localized: "Não foi possível ler a foto.")
            return
        }
        let encoded = await EnterpriseReceiptEncoder.prepare(data)
        preparing = false
        if let encoded {
            receipt = encoded
            error = nil
        } else {
            error = String(localized: "A foto não pôde ser preparada com até 5 MB.")
        }
    }

    private func submit() {
        guard let assetId else { return }
        switch enterprise.submitExpense(assetId: assetId, category: category, paidText: paidText, companyText: companyText,
                                        payer: payer, receipt: receipt) {
        case .success:
            dismiss()
        case .failure(let failure):
            error = failure.message
        }
    }
}

/// Re-encodes a receipt as a metadata-free JPEG (no EXIF or GPS) within the 5 MiB enterprise bound.
enum EnterpriseReceiptEncoder {
    static func prepare(_ data: Data) async -> Data? {
        return await Task.detached(priority: .userInitiated) { () -> Data? in
            let codec = IosImageIoPhotoCodec()
            guard let dims = codec.probeDims(bytes: data) else { return nil }
            let sample = PortablePhoto.sampleSizeForBounds(width: dims.width, height: dims.height)
            for attempt in 1...PortablePhoto.maxAttempts {
                if let encoded = codec.encode(source: data, request: PhotoEncodeRequest(sampleSize: sample, attempt: attempt)),
                   !encoded.isEmpty, encoded.count <= EnterpriseAttachmentWire.maxBytes {
                    return encoded
                }
            }
            return nil
        }.value
    }
}

// MARK: - Queue

struct EnterpriseQueueView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator

    var body: some View {
        let snapshot = enterprise.snapshot
        List {
            Section("Registros") {
                let items = (snapshot?.queue ?? []).reversed()
                if items.isEmpty {
                    Text("Nenhum registro neste aparelho.").foregroundStyle(.secondary)
                }
                ForEach(Array(items)) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(item.summary).font(.subheadline.weight(.semibold))
                        Text(EnterpriseCoordinator.statusLabel(item)).font(.caption)
                        Text(EnterpriseLabels.dateTime(millis: item.createdAtMillis)).font(.caption2).foregroundStyle(.secondary)
                        if item.status == .exhausted || (item.status == .pending && item.attempts > 0) {
                            Button("Tentar agora") { enterprise.retryNow(item.id) }
                                .font(.caption)
                        }
                        if item.status.isFinal {
                            Button("Remover da lista") { enterprise.dismiss(item.id) }
                                .font(.caption)
                        }
                    }
                }
            }
            let attachments = snapshot?.attachments ?? []
            if !attachments.isEmpty {
                Section("Fotos de comprovantes") {
                    ForEach(attachments) { item in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(EnterpriseCoordinator.attachmentLabel(item)).font(.subheadline)
                            if item.status == .exhausted {
                                Button("Tentar agora") { enterprise.retryNow(item.id) }.font(.caption)
                            }
                            if item.status == .delivered || item.status == .refused {
                                Button("Remover da lista") { enterprise.dismiss(item.id) }.font(.caption)
                            }
                        }
                    }
                }
            }
            let refusedTrips = (snapshot?.trips ?? []).filter { trip in
                snapshot?.queue.first(where: { $0.id == trip.startOperationId })?.status == .rejected
            }
            if !refusedTrips.isEmpty {
                Section("Viagens recusadas") {
                    ForEach(refusedTrips) { trip in
                        HStack {
                            Text(trip.assetLabel)
                            Spacer()
                            Button("Remover") { enterprise.dismissRefusedTrip(trip.id) }
                        }
                    }
                }
            }
        }
        .navigationTitle("Fila de envio")
        .refreshable {
            enterprise.deliverNow()
        }
    }
}

// MARK: - Labels

enum EnterpriseLabels {
    static func nowMillis() -> Int64 {
        Int64(Date().timeIntervalSince1970 * 1000)
    }

    static func dateTime(millis: Int64) -> String {
        let date = Date(timeIntervalSince1970: TimeInterval(millis) / 1000)
        return date.formatted(date: .abbreviated, time: .shortened)
    }

    static func organizationState(_ state: String) -> String {
        switch state {
        case "SUSPENDED": return String(localized: "Empresa suspensa")
        case "ARCHIVED": return String(localized: "Empresa arquivada")
        case "SETUP", "DRAFT": return String(localized: "Empresa em configuração")
        default: return state
        }
    }

    static func assetStatus(_ asset: EnterpriseReferenceAsset, nowMillis: Int64) -> String {
        if !asset.isActive {
            return String(localized: "Ativo indisponível no momento (\(asset.state))")
        }
        if asset.assignmentEnded(nowMillis: nowMillis) {
            return String(localized: "Sua atribuição a este ativo terminou")
        }
        switch asset.assignmentKind {
        case "TEMPORARY": return String(localized: "Atribuição temporária")
        case "ADDITIONAL": return String(localized: "Atribuição adicional")
        default: return String(localized: "Atribuição principal")
        }
    }
}
