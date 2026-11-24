import AnpFuelCore
import SwiftUI

struct DriverGpsControls: View {
    @ObservedObject var viewModel: AppStateViewModel
    let shiftId: String

    var body: some View {
        let gps = viewModel.driverGPS
        Group {
            if gps.shiftId == shiftId {
                status(gps.phase)
            }
            if gps.isRunning && gps.shiftId == shiftId {
                Button {
                    gps.stop()
                } label: {
                    Label(String(localized: "driver.gps_stop"), systemImage: "stop.circle")
                }
            } else {
                Button {
                    viewModel.startDriverGps(shiftId: shiftId)
                } label: {
                    Label(String(localized: "driver.gps_start"), systemImage: "location.circle")
                }
                .disabled(!viewModel.isDriverGpsAvailable(shiftId: shiftId))
                Text(String(localized: "driver.gps_optional"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private func status(_ phase: DriverGpsCoordinator.Phase) -> some View {
        switch phase {
        case .idle:
            EmptyView()
        case .requestingPermission:
            Text(String(localized: "driver.gps_waiting_permission"))
        case .recording(let metres, let weakSignal):
            Text(String(localized: "driver.gps_recording"))
                .font(.subheadline.weight(.semibold))
            if let metres {
                let distance = LedgerFormat.kilometres(metres)
                Text(String(localized: "driver.gps_distance_value", defaultValue: "GPS estimate · \(distance)"))
            }
            if weakSignal {
                Text(String(localized: "driver.gps_weak_signal"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        case .interrupted:
            Text(String(localized: "driver.gps_interrupted"))
        case .stopped(let metres):
            if let metres {
                let distance = LedgerFormat.kilometres(metres)
                Text(String(localized: "driver.gps_distance_value", defaultValue: "GPS estimate · \(distance)"))
                Text(String(localized: "driver.gps_incomplete"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                Text(String(localized: "driver.gps_distance_required"))
            }
        case .permissionDenied:
            Text(String(localized: "driver.gps_permission_denied"))
        case .unavailable:
            Text(String(localized: "driver.gps_unavailable"))
        case .storageUnavailable:
            Text(String(localized: "driver.gps_storage_unavailable"))
        }
    }
}
