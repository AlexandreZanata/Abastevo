import AnpFuelCore
import PhotosUI
import SwiftUI
#if canImport(UIKit)
import UIKit
#elseif canImport(AppKit)
import AppKit
#endif

struct VehicleEditorContext: Equatable, Sendable {
    let scope: VehicleLocalScope
    let generation: UUID
}

extension VehicleEditorError {
    var message: String {
        switch self {
        case .nameRequired: return String(localized: "android.vehicle_validation_name_required")
        case .invalidTank: return String(localized: "android.vehicle_validation_tank_capacity")
        case .invalidBattery: return String(localized: "android.vehicle_validation_battery_capacity")
        case .stationRequired: return String(localized: "android.vehicle_validation_station_required")
        case .photoInvalid: return String(localized: "vehicle.photo_invalid")
        case .storageUnavailable: return String(localized: "vehicle.storage_unavailable")
        }
    }
}

@MainActor
struct VehicleEditorView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.locale) private var locale
    @ObservedObject var viewModel: AppStateViewModel
    let existing: VehicleRecord?

    @State private var draft = VehicleEditorDraft()
    @State private var photoChoice = VehiclePhotoChange.keep
    @State private var pickerItem: PhotosPickerItem?
    @State private var processedItem: PhotosPickerItem?
    @State private var savedPhoto: Data?
    @State private var context: VehicleEditorContext?
    @State private var issue: String?
    @State private var loaded = false
    @State private var loadingPhoto = false
    @State private var showPermissionsHint = false

    var body: some View {
        Form {
            if loaded {
                Section {
                    TextField(String(localized: "android.vehicle_name_hint"), text: $draft.name)
                    Picker(String(localized: "android.vehicle_propulsion_label"), selection: propulsionSelection) {
                        ForEach(Propulsion.allCases) { propulsion in
                            Text(propulsion.localizedLabel).tag(propulsion)
                        }
                    }
                    if draft.propulsion.supportsFuel {
                        fuelFields
                    }
                    if draft.propulsion.supportsElectric {
                        batteryFields
                    }
                }
                if draft.propulsion.supportsFuel { alertPreferences }
                photoSection
            } else if issue == nil {
                ProgressView()
            }
            if let issue {
                Text(issue).foregroundStyle(.red)
                if !loaded { Button(String(localized: "android.action_retry"), action: load) }
            }
        }
        .navigationTitle(existing == nil ? String(localized: "android.vehicle_add") : String(localized: "android.vehicle_edit"))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(String(localized: "android.vehicle_save"), action: save)
                    .disabled(!loaded || loadingPhoto || pickerItem != processedItem || context != viewModel.vehicleEditorContext)
            }
        }
        .task { load() }
        .task(id: pickerItem) { await preparePhoto(pickerItem) }
        .onChange(of: viewModel.vehicleEditorContext) { _, _ in
            dismiss()
        }
    }

    /// A propulsion change clears the inputs it cannot use, as Android's editor does.
    private var propulsionSelection: Binding<Propulsion> {
        Binding(get: { draft.propulsion }, set: { draft.selectPropulsion($0) })
    }

    @ViewBuilder
    private var fuelFields: some View {
        Picker(String(localized: "android.vehicle_fuel_product_label"), selection: $draft.fuel) {
            ForEach(FuelProduct.allCases) { fuel in
                Text(fuel.localizedName).tag(fuel)
            }
        }
        TextField(String(localized: "android.vehicle_tank_capacity_hint"), text: $draft.tankLiters)
            #if os(iOS)
            .keyboardType(.decimalPad)
            #endif
    }

    @ViewBuilder
    private var batteryFields: some View {
        TextField(String(localized: "android.vehicle_battery_capacity_hint"), text: $draft.batteryKwh)
            #if os(iOS)
            .keyboardType(.decimalPad)
            #endif
        Picker(String(localized: "android.vehicle_charge_connector_label"), selection: $draft.connector) {
            Text(String(localized: "vehicle.connector_unselected")).tag(nil as ChargingConnector?)
            ForEach(ChargingConnector.vehicleChoices, id: \.self) { option in
                Text(option.label).tag(Optional(option))
            }
        }
    }

    private var alertPreferences: some View {
        Section {
            Picker(String(localized: "android.vehicle_price_source_label"), selection: $draft.priceMode) {
                Text(String(localized: "android.vehicle_price_source_cheapest")).tag(VehiclePriceReferenceMode.cheapest)
                Text(String(localized: "android.vehicle_price_source_specific")).tag(VehiclePriceReferenceMode.specific)
            }
            if draft.priceMode == .specific {
                let choices = stationChoices
                Picker(String(localized: "android.vehicle_select_station"), selection: $draft.stationCNPJ) {
                    Text(String(localized: "android.vehicle_select_station")).tag(nil as String?)
                    if let chosen = draft.stationCNPJ, !choices.contains(where: { $0.id == chosen }) {
                        Text(chosen).tag(Optional(chosen))
                    }
                    ForEach(choices) { station in
                        Text(station.name).tag(Optional(station.id))
                    }
                }
                if choices.isEmpty && draft.stationCNPJ == nil {
                    Text(String(localized: "android.map_no_results")).foregroundStyle(.secondary)
                }
            }
            Toggle(String(localized: "android.vehicle_price_drop_alert_label"), isOn: $draft.priceDropAlertEnabled)
                .onChange(of: draft.priceDropAlertEnabled) { _, enabled in
                    showPermissionsHint = false
                    if enabled {
                        Task {
                            let allowed = await viewModel.anpSurvey.requestPriceDropAuthorization()
                            if draft.priceDropAlertEnabled, context == viewModel.vehicleEditorContext {
                                showPermissionsHint = !allowed
                            }
                        }
                    }
                }
            Text(String(localized: "android.vehicle_price_drop_alert_description"))
                .font(.footnote)
                .foregroundStyle(.secondary)
            if showPermissionsHint {
                Text(String(localized: "android.notification_permission_denied_hint"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private struct StationChoice: Identifiable {
        let id: String
        let name: String
    }

    private var stationChoices: [StationChoice] {
        var seen = Set<String>()
        return viewModel.sortedStations.compactMap { station in
            guard let cnpj = BrazilianCNPJ.normalized(station.cnpj), seen.insert(cnpj).inserted else { return nil }
            return StationChoice(id: cnpj, name: station.name)
        }
    }

    private var photoSection: some View {
        Section(String(localized: "android.vehicle_photo_label")) {
            if let photo = displayedPhoto {
                VehiclePhotoPreview(data: photo)
            }
            PhotosPicker(selection: $pickerItem, matching: .images, preferredItemEncoding: .automatic) {
                Label(displayedPhoto == nil ? String(localized: "android.vehicle_add_photo") : String(localized: "android.vehicle_change_photo"),
                      systemImage: "photo")
            }
            if displayedPhoto != nil {
                Button(String(localized: "android.vehicle_remove_photo"), role: .destructive) {
                    pickerItem = nil
                    photoChoice = .remove
                }
            }
            if loadingPhoto { ProgressView().accessibilityLabel(String(localized: "android.vehicle_photo_label")) }
        }
    }

    private var displayedPhoto: Data? {
        switch photoChoice {
        case .keep: return savedPhoto
        case .remove: return nil
        case .replace(let photo): return photo.jpeg
        }
    }

    private func load() {
        guard !loaded else { return }
        let current = viewModel.vehicleEditorContext
        do {
            let details = try existing.map { try viewModel.vehicleDetails.load(id: $0.id, scope: current.scope) }
            var next = VehicleEditorDraft(existing: existing, preference: details?.preference)
            let separator = locale.decimalSeparator == "." ? "." : ","
            next.tankLiters = next.tankLiters.replacingOccurrences(of: ",", with: separator)
            next.batteryKwh = next.batteryKwh.replacingOccurrences(of: ",", with: separator)
            draft = next
            savedPhoto = details?.jpeg
            context = current
            issue = nil
            loaded = true
        } catch {
            issue = VehicleEditorError.storageUnavailable.message
        }
    }

    private func preparePhoto(_ choice: PhotosPickerItem?) async {
        guard let choice else {
            loadingPhoto = false
            processedItem = nil
            return
        }
        loadingPhoto = true
        defer {
            if pickerItem == choice {
                loadingPhoto = false
                processedItem = choice
            }
        }
        do {
            guard let bytes = try await choice.loadTransferable(type: Data.self) else {
                throw VehicleEditorError.photoInvalid
            }
            let photo = try await Task.detached(priority: .userInitiated) {
                try VehicleLocalPhoto(sanitizing: bytes)
            }.value
            guard !Task.isCancelled, pickerItem == choice, context == viewModel.vehicleEditorContext else { return }
            photoChoice = .replace(photo)
            issue = nil
        } catch {
            guard !Task.isCancelled, pickerItem == choice else { return }
            issue = VehicleEditorError.photoInvalid.message
        }
    }

    private func save() {
        guard let context, !loadingPhoto, pickerItem == processedItem else { return }
        switch viewModel.saveVehicle(draft, editing: existing?.id, photo: photoChoice, context: context) {
        case .success:
            dismiss()
        case .failure(let error):
            issue = error.localizedDescription
        }
    }
}

private struct VehiclePhotoPreview: View {
    let data: Data

    var body: some View {
        #if canImport(UIKit)
        if let image = UIImage(data: data) {
            preview(Image(uiImage: image))
        }
        #elseif canImport(AppKit)
        if let image = NSImage(data: data) {
            preview(Image(nsImage: image))
        }
        #endif
    }

    private func preview(_ image: Image) -> some View {
        image.resizable()
            .scaledToFit()
            .frame(maxHeight: 200)
            .clipShape(.rect(cornerRadius: 10))
            .accessibilityLabel(String(localized: "android.vehicle_photo_content"))
    }
}
