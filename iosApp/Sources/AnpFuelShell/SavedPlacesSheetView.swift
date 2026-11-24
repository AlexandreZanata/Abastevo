import AnpFuelCore
import Foundation
import SwiftUI
import UniformTypeIdentifiers

private struct SavedGPXDocument: FileDocument {
    static let contentType = UTType(filenameExtension: "gpx", conformingTo: .xml) ?? .xml
    static var readableContentTypes: [UTType] { [contentType] }

    let bytes: Data

    init(bytes: Data) { self.bytes = bytes }

    init(configuration: ReadConfiguration) throws {
        guard let data = configuration.file.regularFileContents,
              data.count <= SavedMapBounds.fileBytes else { throw SavedMapError.fileTooLarge }
        bytes = data
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        FileWrapper(regularFileWithContents: bytes)
    }
}

@MainActor
public struct SavedPlacesSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var showImporter = false
    @State private var pickerScope: SavedMapScope?
    @State private var showExporter = false
    @State private var confirmExport = false
    @State private var confirmRestore = false
    @State private var selection: SavedMapStore.Selection?
    @State private var document: SavedGPXDocument?
    @State private var pendingImport: (scope: SavedMapScope, content: SavedGPXContent)?
    @State private var operation: Task<Void, Never>?
    @State private var generation = UUID()
    @State private var message: String?

    public var body: some View {
        let inventory = viewModel.visibleSavedMap
        NavigationStack {
            List {
                Section {
                    Button(String(localized: "map.saved.import_gpx"), systemImage: "square.and.arrow.down") {
                        pickerScope = viewModel.currentSavedMapScope
                        showImporter = pickerScope != nil
                    }
                    .disabled(viewModel.currentSavedMapScope == nil || operation != nil)
                    Button(String(localized: "map.saved.export_gpx"), systemImage: "square.and.arrow.up") {
                        requestExport(.everything)
                    }
                    .disabled(!inventory.hasExportableRecords || operation != nil)
                    if operation != nil { ProgressView() }
                    if let status = message ?? viewModel.savedMapMessage {
                        Text(status).font(.callout).foregroundStyle(.secondary)
                    }
                }
                Section(String(localized: "map.saved.places")) {
                    if inventory.places.isEmpty, inventory.legacyPlaces.isEmpty {
                        Text(String(localized: "map.saved.no_places")).foregroundStyle(.secondary)
                    }
                    ForEach(inventory.places) { place in
                        row(title: place.name, subtitle: place.note, selection: .place(place.id))
                    }
                    ForEach(inventory.legacyPlaces) { place in
                        row(title: place.title, subtitle: place.subtitle, selection: .legacyPlace(place.id))
                    }
                }
                Section(String(localized: "map.saved.trips")) {
                    if inventory.trips.isEmpty, inventory.legacyTrips.isEmpty {
                        Text(String(localized: "map.saved.no_trips")).foregroundStyle(.secondary)
                    }
                    ForEach(inventory.trips) { trip in
                        row(title: trip.name, subtitle: trip.note, selection: .trip(trip.id))
                    }
                    ForEach(inventory.legacyTrips) { trip in
                        row(title: trip.title, subtitle: legacyTripSummary(trip), selection: .legacyTrip(trip.id))
                    }
                }
            }
            .navigationTitle(String(localized: "map.saved.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
        .fileImporter(isPresented: $showImporter, allowedContentTypes: [SavedGPXDocument.contentType, .xml]) { result in
            let scope = pickerScope
            pickerScope = nil
            guard let scope, viewModel.currentSavedMapScope == scope else { return }
            switch result {
            case .success(let url): readImport(url, scope: scope)
            case .failure(let error): show(error)
            }
        }
        .fileExporter(isPresented: $showExporter, document: document, contentType: SavedGPXDocument.contentType,
                      defaultFilename: "abastevo-saved.gpx") { result in
            document = nil
            if case .failure(let error) = result { show(error) }
        }
        .alert(String(localized: "map.saved.export_gpx"), isPresented: $confirmExport) {
            Button(String(localized: "android.action_cancel"), role: .cancel) { selection = nil }
            Button(String(localized: "map.saved.continue_export")) { prepareExport() }
        } message: {
            Text(String(localized: "map.saved.export_privacy"))
        }
        .alert(String(localized: "map.saved.restore_title"), isPresented: $confirmRestore) {
            Button(String(localized: "android.action_cancel"), role: .cancel) { pendingImport = nil }
            Button(String(localized: "map.saved.restore_deleted")) { restoreImport() }
        } message: {
            Text(String(localized: "map.saved.restore_notice"))
        }
        .onChange(of: viewModel.currentSavedMapScope) { _, _ in clearPending() }
        .onDisappear { clearPending() }
    }

    private func row(title: String, subtitle: String?, selection: SavedMapStore.Selection) -> some View {
        VStack(alignment: .leading) {
            Text(title).font(.headline)
            if let subtitle, !subtitle.isEmpty {
                Text(subtitle).font(.caption).foregroundStyle(.secondary)
            }
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                do {
                    try viewModel.deleteSavedMap(selection)
                    message = nil
                } catch {
                    show(error)
                }
            } label: {
                Label(String(localized: "map.saved.delete"), systemImage: "trash")
            }
        }
        .contextMenu {
            if case .legacyTrip = selection { } else {
                Button(String(localized: "map.saved.export_gpx"), systemImage: "square.and.arrow.up") {
                    requestExport(selection)
                }
                .disabled(operation != nil)
            }
        }
    }

    private func legacyTripSummary(_ trip: SavedTripRecord) -> String {
        let distance = Measurement(value: trip.distanceKm, unit: UnitLength.kilometers)
            .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(0))))
        return String(localized: "map.saved.trip_summary", defaultValue: "\(trip.originLabel) → \(trip.destinationLabel) (\(distance))")
    }

    private func requestExport(_ selection: SavedMapStore.Selection) {
        guard viewModel.currentSavedMapScope != nil else { return }
        self.selection = selection
        confirmExport = true
    }

    private func prepareExport() {
        guard let selection, let scope = viewModel.currentSavedMapScope else { return }
        self.selection = nil
        let store = viewModel.savedMapStore
        let unnamedPlace = String(localized: "map.saved.unnamed_place")
        let attempt = generation
        operation = Task {
            defer {
                if generation == attempt { operation = nil }
            }
            do {
                let data = try await Task.detached(priority: .userInitiated) {
                    try store.exportGPX(selection, unnamedPlace: unnamedPlace, in: scope)
                }.value
                guard !Task.isCancelled, generation == attempt, viewModel.currentSavedMapScope == scope else { return }
                document = SavedGPXDocument(bytes: data)
                showExporter = true
            } catch {
                guard !Task.isCancelled, generation == attempt, viewModel.currentSavedMapScope == scope else { return }
                show(error)
            }
        }
    }

    private func readImport(_ url: URL, scope: SavedMapScope) {
        guard operation == nil, viewModel.currentSavedMapScope == scope else { return }
        let unnamedPlace = String(localized: "map.saved.unnamed_place")
        let unnamedTrip = String(localized: "map.saved.unnamed_trip")
        let now = Int64(Date().timeIntervalSince1970 * 1_000)
        let attempt = generation
        operation = Task {
            defer {
                if generation == attempt { operation = nil }
            }
            do {
                let content = try await Task.detached(priority: .userInitiated) {
                    let data = try Self.readFile(url)
                    return try SavedGPX.parse(data, nowMillis: now, unnamedPlace: unnamedPlace, unnamedTrip: unnamedTrip)
                }.value
                guard !Task.isCancelled, generation == attempt, viewModel.currentSavedMapScope == scope else { return }
                applyImport(content, scope: scope, confirmed: false)
            } catch {
                guard !Task.isCancelled, generation == attempt, viewModel.currentSavedMapScope == scope else { return }
                show(error)
            }
        }
    }

    private nonisolated static func readFile(_ url: URL) throws -> Data {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var data = Data()
        while let chunk = try handle.read(upToCount: min(65_536, SavedMapBounds.fileBytes + 1 - data.count)), !chunk.isEmpty {
            data.append(chunk)
            guard data.count <= SavedMapBounds.fileBytes else { throw SavedMapError.fileTooLarge }
        }
        return data
    }

    private func applyImport(_ content: SavedGPXContent, scope: SavedMapScope, confirmed: Bool) {
        do {
            let result = try viewModel.importSavedGPX(content, confirmed: confirmed, scope: scope)
            if result.alreadyImported {
                message = String(localized: "map.saved.already_imported")
            } else if result.importedPlaces + result.importedTrips == 0 {
                message = String(localized: "map.saved.nothing_new")
            } else if result.skippedUnsupported != 0 || result.truncatedFields != 0 {
                message = String(localized: "map.saved.imported_with_adjustments")
            } else {
                message = String(localized: "map.saved.imported")
            }
        } catch SavedMapError.restoreConfirmationRequired {
            pendingImport = (scope, content)
            confirmRestore = true
        } catch {
            show(error)
        }
    }

    private func restoreImport() {
        guard let pendingImport, viewModel.currentSavedMapScope == pendingImport.scope else { return }
        self.pendingImport = nil
        applyImport(pendingImport.content, scope: pendingImport.scope, confirmed: true)
    }

    private func show(_ error: Error) {
        if let cocoa = error as? CocoaError, cocoa.code == .userCancelled { return }
        message = (error as? SavedMapError ?? .localStorage).savedMessage
    }

    private func clearPending() {
        generation = UUID()
        operation?.cancel()
        operation = nil
        showImporter = false
        pickerScope = nil
        showExporter = false
        confirmExport = false
        confirmRestore = false
        selection = nil
        pendingImport = nil
        document = nil
        message = nil
    }
}

extension SavedMapError {
    var savedMessage: String {
        switch self {
        case .fileTooLarge:
            return String(localized: "map.saved.file_too_large")
        case .fileEmpty, .noRecords, .nothingToExport:
            return String(localized: "map.saved.empty_file")
        case .tooManyRecords, .tooManyPoints, .placeQuota, .tripQuota, .xmlTooDeep, .xmlTooLarge:
            return String(localized: "map.saved.limit_reached")
        case .localStorage:
            return String(localized: "map.saved.storage_failed")
        case .scopeChanged:
            return String(localized: "map.saved.owner_changed")
        case .unsupportedFormat, .xmlMalformed, .xmlEncodingUnsupported, .xmlPIRefused, .xmlDoctypeRefused,
             .xmlEntityRefused, .coordinateMalformed, .coordinateOutOfRange, .invalidRecord, .duplicateRecordID,
             .restoreConfirmationRequired:
            return String(localized: "map.saved.invalid_gpx")
        }
    }
}
