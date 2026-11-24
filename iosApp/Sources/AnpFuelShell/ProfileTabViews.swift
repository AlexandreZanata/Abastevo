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
                    appearanceSection
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
                        Text(String(localized: "profile.free_account"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                }
                Button(String(localized: "profile.account_management")) {
                    viewModel.activeSheet = .accountManagement
                }
                .buttonStyle(.bordered)
                .frame(maxWidth: .infinity, alignment: .leading)
                Button {
                    viewModel.activeSheet = .premium
                } label: {
                    Label(String(localized: "premium.title"), systemImage: "crown.fill")
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .buttonStyle(.bordered)
            } else {
                Text(String(localized: "android.profile_guest_title"))
                    .font(.headline)
                Text(String(localized: "profile.guest_summary"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Button {
                    viewModel.activeSheet = .auth
                } label: {
                    Text(String(localized: "android.profile_action_sign_in"))
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.ink)
            }
        }
    }

    private var driverAndVehiclesSection: some View {
        EnergiaCard {
            Text(String(localized: "profile.driver_vehicles_energy"))
                .font(.headline)
            profileNavRow(icon: "car.fill", title: String(localized: "profile.vehicles_ledger"), subtitle: String(localized: "profile.vehicles_ledger_summary")) {
                viewModel.activeSheet = .vehiclesLedger
            }
            Divider()
            profileNavRow(icon: "steeringwheel", title: String(localized: "android.driver_hub_title"), subtitle: String(localized: "android.driver_hub_profile_subtitle")) {
                viewModel.activeSheet = .driverHub
            }
            Divider()
            profileNavRow(icon: "chart.line.uptrend.xyaxis", title: String(localized: "profile.history_alerts"), subtitle: String(localized: "profile.official_fuel_series")) {
                viewModel.activeSheet = .priceHistory
            }
        }
    }

    private var enterpriseAndGamesSection: some View {
        EnergiaCard {
            Text(String(localized: "profile.enterprises_arcade"))
                .font(.headline)
            profileNavRow(icon: "building.2.fill", title: String(localized: "enterprise.title"), subtitle: String(localized: "enterprise.profile_subtitle")) {
                viewModel.activeSheet = .enterpriseHub
            }
            Divider()
            profileNavRow(icon: "gamecontroller.fill", title: String(localized: "profile.games_title"), subtitle: String(localized: "profile.game_records", defaultValue: "High scores: \(viewModel.snapshot.bestFuelRunScore.formatted()) / \(viewModel.snapshot.bestFuelFlightScore.formatted()) pts")) {
                viewModel.activeSheet = .arcadeGames
            }
        }
    }

    private var appearanceSection: some View {
        EnergiaCard {
            Text(String(localized: "profile.appearance"))
                .font(.headline)
            Toggle(String(localized: "android.profile_dark_theme_title"), isOn: Binding(
                get: { viewModel.snapshot.darkThemeEnabled ?? false },
                set: { viewModel.setTheme(dark: $0) }
            ))
        }
    }

    private var expertAndPrivacySection: some View {
        EnergiaCard {
            Text(String(localized: "profile.tools_privacy_cache"))
                .font(.headline)
            profileNavRow(icon: "camera.badge.ellipsis", title: String(localized: "privacy.ocr_consent"), subtitle: String(localized: "privacy.collection_off")) {
                viewModel.activeSheet = .trainingConsent
            }
            Divider()
            profileNavRow(icon: "calendar", title: String(localized: "android.week_picker_title"), subtitle: viewModel.surveyWeekLabel) {
                viewModel.activeSheet = .weekPicker
            }
            Divider()
            profileNavRow(icon: "map.fill", title: String(localized: "profile.regional_offline_maps"), subtitle: String(localized: "profile.ios_unavailable")) {
                viewModel.activeSheet = .mapDownloads
            }
            Divider()
            profileNavRow(icon: "gearshape.fill", title: String(localized: "profile.network_settings"), subtitle: viewModel.snapshot.apiEnvironment.localizedLabel) {
                viewModel.activeSheet = .settings
            }
            Divider()
            profileNavRow(icon: "lock.shield.fill", title: String(localized: "profile.privacy_lgpd_terms"), subtitle: String(localized: "profile.privacy_rights")) {
                viewModel.activeSheet = .privacy
            }
            Divider()
            profileNavRow(icon: "questionmark.circle", title: String(localized: "android.profile_tool_help_title"), subtitle: String(localized: "profile.source_transparency")) {
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

                Section(String(localized: "auth.email_code")) {
                    TextField(String(localized: "auth.email_example"), text: $email)
                    Button(String(localized: "auth.send_code")) {
                        viewModel.requestEmailOtp(email: email)
                    }
                    TextField(String(localized: "android.auth_code_label"), text: $code)
                    Button(String(localized: "auth.verify_code")) {
                        viewModel.verifyEmailOtp(email: email, code: code)
                    }
                }

                Section(String(localized: "auth.recovery_account")) {
                    TextField(String(localized: "auth.alias_hint"), text: $username)
                    Button(String(localized: "auth.create_with_key")) {
                        Task {
                            issuedKey = await viewModel.createAnonymousKeyAccount(username: username)
                        }
                    }
                    if let issued = issuedKey {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(String(localized: "android.profile_backup_username", defaultValue: "Username: \(issued.username)")).font(.subheadline.bold())
                            Text(String(localized: "auth.key_save_now", defaultValue: "Unique key (save it now): \(issued.accountKey)"))
                                .font(.caption.monospaced())
                                .foregroundStyle(.orange)
                        }
                    }
                    TextField(String(localized: "auth.key_sign_in_hint"), text: $accountKeyInput)
                    Button(String(localized: "android.auth_key_login")) {
                        viewModel.loginWithAccountKey(accountKeyInput)
                    }
                }

                if viewModel.isAuthWorking || viewModel.authStatusMessage != nil {
                    Section(String(localized: "auth.status")) {
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
            .navigationTitle(String(localized: "android.auth_title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
                    Section(String(localized: "account.connected_identity")) {
                        Text(alias)
                        Button(String(localized: "account.local_export")) {
                            exportedJson = viewModel.exportMyDataJson()
                        }
                        if let json = exportedJson {
                            Text(String(json.prefix(500)) + (json.count > 500 ? "..." : ""))
                                .font(.caption2.monospaced())
                        }
                    }
                }

                Section(String(localized: "account.blocked_count", defaultValue: "Blocked Users (\(viewModel.snapshot.blockedUsers.count.formatted()))")) {
                    if viewModel.snapshot.blockedUsers.isEmpty {
                        Text(String(localized: "account.blocked_empty")).foregroundStyle(.secondary)
                    } else {
                        ForEach(viewModel.snapshot.blockedUsers) { user in
                            HStack {
                                Text(user.displayAlias)
                                Spacer()
                                Button(String(localized: "account.unblock")) {
                                    viewModel.unblockUser(userId: user.id)
                                }
                                .font(.caption)
                            }
                        }
                    }
                }

                if viewModel.currentAccountIdOrNil != nil {
                    Section(String(localized: "account.session_deletion")) {
                        Button(String(localized: "account.logout_and_purge")) {
                            viewModel.logout()
                            dismiss()
                        }
                        Button(String(localized: "account.request_deletion"), role: .destructive) {
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
            .navigationTitle(String(localized: "android.account_screen_title"))
            .alert(String(localized: "android.auth_delete_title"), isPresented: $confirmDelete) {
                Button(String(localized: "account.request_deletion_action"), role: .destructive) {
                    viewModel.deleteAccountAndPurge()
                }
                Button(String(localized: "android.action_cancel"), role: .cancel) {}
            } message: {
                Text(String(localized: "account.deletion_notice"))
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
                Section(String(localized: "driver.manual_shift")) {
                    if let active = activeShift {
                        Text(String(localized: "driver.active_shift", defaultValue: "Shift in progress · Starting odometer \(active.startOdometerKm.formatted()) km"))
                            .font(.headline)
                            .foregroundStyle(AbastevoTheme.accentLight)
                        if let started = formattedDate(active.startedAtLabel) {
                            Text(String(localized: "driver.started", defaultValue: "Started \(started)")).foregroundStyle(.secondary)
                        }
                        TextField(String(localized: "driver.end_odometer"), text: $endKmText)
                        TextField(String(localized: "driver.gross_earnings"), text: $grossReaisText)
                        TextField(String(localized: "driver.fuel_cost"), text: $fuelReaisText)
                        TextField(String(localized: "driver.declared_duration"), text: $minutesText)
                        Button(String(localized: "driver.end_shift")) {
                            finishShift(active)
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                    } else if viewModel.snapshot.vehicles.isEmpty {
                        Text(String(localized: "driver.vehicle_required")).foregroundStyle(.secondary)
                    } else {
                        Picker(String(localized: "driver.vehicle"), selection: $driverVehicleId) {
                            Text(String(localized: "driver.select_vehicle")).tag(nil as String?)
                            ForEach(viewModel.snapshot.vehicles) { vehicle in
                                Text(vehicle.name).tag(Optional(vehicle.id))
                            }
                        }
                        TextField(String(localized: "driver.start_odometer"), text: $startKmText)
                        Button(String(localized: "driver.start_shift"), action: startShift)
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.accentLight)
                        .disabled(driverVehicleId == nil)
                    }
                    if let shiftError {
                        Text(shiftError).font(.caption).foregroundStyle(.red)
                    }
                }

                Section(String(localized: "driver.finished_shifts")) {
                    ForEach(closedShifts) { s in
                        VStack(alignment: .leading, spacing: 4) {
                            if let balance = s.netEarningsCentavos {
                                Text(String(localized: "driver.saved_balance", defaultValue: "Recorded balance \(AbastevoTheme.formatCentavos(balance))")).font(.headline)
                            } else {
                                Text(String(localized: "driver.balance_unavailable")).font(.headline)
                            }
                            if let distance = s.distanceKm {
                                Text(String(localized: "driver.distance_driven", defaultValue: "\(distance.formatted()) km driven"))
                            }
                            if let ended = formattedDate(s.endedAtLabel) {
                                Text(String(localized: "driver.ended", defaultValue: "Ended \(ended)"))
                            }
                            Text(String(localized: "driver.gross_and_fuel", defaultValue: "Gross \(AbastevoTheme.formatCentavos(s.grossEarningsCentavos)) · Fuel \(AbastevoTheme.formatCentavos(s.fuelCostCentavos))"))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .navigationTitle(String(localized: "android.driver_hub_title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }

    private func startShift() {
        guard let vehicleId = driverVehicleId,
              let startKm = Int64(startKmText.trimmingCharacters(in: .whitespacesAndNewlines)) else {
            shiftError = String(localized: "driver.invalid_start")
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
            shiftError = String(localized: "driver.invalid_end")
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
                Picker(String(localized: "arcade.game"), selection: $gameKind) {
                    ForEach(ArcadeGameKind.allCases) { kind in
                        Text(kind.localizedTitle).tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)

                HStack {
                    Text(String(localized: "arcade.score", defaultValue: "Score: \(gameState.score.formatted())")).font(.headline)
                    Spacer()
                    Text(String(localized: "arcade.fuel", defaultValue: "Fuel: \((gameState.fuelPercent / 100).formatted(.percent.precision(.fractionLength(0))))")).font(.subheadline.bold())
                        .foregroundStyle(gameState.fuelPercent < 25 ? .red : AbastevoTheme.accentLight)
                }
                .padding(.horizontal, 20)

                ZStack {
                    RoundedRectangle(cornerRadius: 16)
                        .fill(Color.secondary.opacity(0.12))
                        .frame(height: 240)

                    VStack {
                        HStack {
                            Text(gameKind == .fuelRun ? String(localized: "arcade.lane", defaultValue: "Lane \(1.formatted())") : String(localized: "arcade.high")).font(.caption2)
                            Spacer()
                            Text(gameKind == .fuelRun ? String(localized: "arcade.lane", defaultValue: "Lane \(2.formatted())") : String(localized: "arcade.middle")).font(.caption2)
                            Spacer()
                            Text(gameKind == .fuelRun ? String(localized: "arcade.lane", defaultValue: "Lane \(3.formatted())") : String(localized: "arcade.low")).font(.caption2)
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
                    Text(String(localized: "arcade.game_over", defaultValue: "Game over! Final score: \(gameState.score.formatted())"))
                        .font(.headline)
                        .foregroundStyle(.orange)
                }

                HStack(spacing: 16) {
                    Button(String(localized: "arcade.left")) {
                        gameState.lane = max(0, gameState.lane - 1)
                        gameState.altitude = min(0.9, gameState.altitude + 0.18)
                    }
                    .buttonStyle(.bordered)

                    Button(isPlaying && !gameState.isGameOver ? String(localized: "arcade.pause") : String(localized: "arcade.play_restart")) {
                        if gameState.isGameOver {
                            gameState = ArcadeState()
                        }
                        isPlaying.toggle()
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)

                    Button(String(localized: "arcade.right")) {
                        gameState.lane = min(2, gameState.lane + 1)
                        gameState.altitude = max(0.1, gameState.altitude - 0.15)
                    }
                    .buttonStyle(.bordered)
                }

                Spacer()
            }
            .padding(.top, 12)
            .navigationTitle(String(localized: "arcade.title"))
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
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
                    Text(String(localized: "privacy.ocr_unavailable"))
                }
                if TrainingTermsContract.verifiedDigestMatches() {
                    Section(String(localized: "privacy.consent_document")) {
                        Text(String(localized: "android.privacy_terms_language"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Text(verbatim: TrainingTermsContract.fullTextPtBr)
                            .font(.footnote)
                    }
                }
            }
            .navigationTitle(String(localized: "privacy.ocr_training"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
                Section(String(localized: "settings.network_server")) {
                    Picker(String(localized: "settings.api_environment"), selection: Binding(
                        get: { viewModel.snapshot.apiEnvironment },
                        set: { viewModel.setApiEnvironment($0) }
                    )) {
                        ForEach(ApiEnvironment.allCases, id: \.self) { env in
                            Text(env.localizedLabel).tag(env)
                        }
                    }
                    .disabled(!viewModel.canSwitchApiEnvironment)
                    if !viewModel.canSwitchApiEnvironment {
                        Text(String(localized: "settings.sign_out_to_switch"))
                            .font(.caption)
                    }
                    Toggle(String(localized: "settings.offline_simulation"), isOn: $viewModel.isOfflineSimulation)
                }

                Section(String(localized: "settings.offline_storage")) {
                    Text(String(localized: "settings.disk_use", defaultValue: "Approximate disk usage: \((viewModel.estimatedStorageKb() * 1_024).formatted(.byteCount(style: .file))) (excluded from iCloud backups)"))
                        .font(.caption)
                    Button(String(localized: "settings.clear_cache_and_photos"), role: .destructive) {
                        viewModel.clearOfflineCache()
                    }
                }

                Section(String(localized: "settings.ios_version")) {
                    Text(String(localized: "settings.build_version"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle(String(localized: "android.settings_title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
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
                Section(String(localized: "help.about")) {
                    Text(String(localized: "help.compare_prices"))
                    Text(String(localized: "help.device_ocr"))
                    Text(String(localized: "help.no_paid_ranking"))
                }
                Section(String(localized: "help.contact_source")) {
                    Text(String(localized: "help.operator"))
                        .font(.caption)
                }
            }
            .navigationTitle(String(localized: "android.profile_tool_help_title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }
}
