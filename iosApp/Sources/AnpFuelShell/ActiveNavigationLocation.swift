import AnpFuelCore
import CoreLocation
import Foundation
import UIKit

/// Called only by an explicit, foreground navigation or driver shift action.
@MainActor
final class ActiveNavigationLocation: NSObject, @preconcurrency CLLocationManagerDelegate {
    enum Event {
        case requestingPermission
        case active
        case permissionDenied
        case serviceUnavailable
        case weakSignal
        case fix(CLLocation)
    }

    private let manager: CLLocationManager
    private var receive: ((Event) -> Void)?
    private var started = false
    private var prompted = false

    override init() {
        manager = CLLocationManager()
        super.init()
        manager.delegate = self
    }

    func start(activity: CLActivityType = .automotiveNavigation, receive: @escaping (Event) -> Void) {
        stop()
        guard UIApplication.shared.applicationState == .active else {
            receive(.serviceUnavailable)
            return
        }
        self.receive = receive
        manager.activityType = activity
        manager.desiredAccuracy = kCLLocationAccuracyBestForNavigation
        manager.distanceFilter = kCLDistanceFilterNone
        manager.pausesLocationUpdatesAutomatically = false
        updateAuthorization()
    }

    func stop() {
        receive = nil
        prompted = false
        if started {
            manager.stopUpdatingLocation()
            started = false
        }
        manager.allowsBackgroundLocationUpdates = false
        manager.showsBackgroundLocationIndicator = false
    }

    func foregrounded() {
        if !started { updateAuthorization() }
    }

    private func updateAuthorization() {
        guard let receive else { return }
        switch manager.authorizationStatus {
        case .notDetermined:
            guard !prompted, UIApplication.shared.applicationState == .active else { return }
            prompted = true
            receive(.requestingPermission)
            manager.requestWhenInUseAuthorization()
        case .restricted, .denied:
            stop()
            receive(.permissionDenied)
        case .authorizedWhenInUse, .authorizedAlways:
            guard manager.accuracyAuthorization == .fullAccuracy else {
                stop()
                receive(.permissionDenied)
                return
            }
            guard !started else { return }
            guard UIApplication.shared.applicationState == .active else { return }
            manager.allowsBackgroundLocationUpdates = true
            manager.showsBackgroundLocationIndicator = true
            started = true
            manager.startUpdatingLocation()
            receive(.active)
        @unknown default:
            stop()
            receive(.serviceUnavailable)
        }
    }

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        updateAuthorization()
    }

    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard started, let receive, let fix = locations.last else { return }
        receive(.fix(fix))
    }

    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        guard let receive else { return }
        if (error as? CLError)?.code == .denied {
            stop()
            receive(.permissionDenied)
        } else {
            receive(.weakSignal)
        }
    }
}
