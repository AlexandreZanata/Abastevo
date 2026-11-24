import AnpFuelCore
import CoreLocation
import Foundation
import UIKit

/// One platform location manager for explicit navigation and driver recording leases.
@MainActor
final class ActiveNavigationLocation: NSObject, @preconcurrency CLLocationManagerDelegate {
    enum Consumer: Hashable {
        case navigation
        case driver
    }

    enum Event {
        case requestingPermission
        case active
        case permissionDenied
        case serviceUnavailable
        case weakSignal
        case fix(CLLocation)
    }

    private struct Lease {
        let id = UUID()
        let activity: CLActivityType
        let receive: (Event) -> Void
    }

    private let manager: CLLocationManager
    private var leases: [Consumer: Lease] = [:]
    private var notified: Set<UUID> = []
    private var started = false
    private var prompted = false

    override init() {
        manager = CLLocationManager()
        super.init()
        manager.delegate = self
    }

    func start(
        consumer: Consumer = .navigation, activity: CLActivityType = .automotiveNavigation,
        receive: @escaping (Event) -> Void
    ) {
        guard UIApplication.shared.applicationState == .active else {
            receive(.serviceUnavailable)
            return
        }
        if let prior = leases.updateValue(Lease(activity: activity, receive: receive), forKey: consumer) {
            notified.remove(prior.id)
        }
        configureActivity()
        updateAuthorization()
    }

    func stop(consumer: Consumer = .navigation) {
        if let lease = leases.removeValue(forKey: consumer) { notified.remove(lease.id) }
        if leases.isEmpty {
            stopManager()
        } else {
            configureActivity()
        }
    }

    func foregrounded() {
        updateAuthorization()
    }

    private func configureActivity() {
        if let activity = leases[.navigation]?.activity ?? leases[.driver]?.activity {
            manager.activityType = activity
        }
        manager.desiredAccuracy = kCLLocationAccuracyBestForNavigation
        manager.distanceFilter = kCLDistanceFilterNone
        manager.pausesLocationUpdatesAutomatically = false
    }

    private func stopManager() {
        prompted = false
        notified.removeAll()
        if started {
            manager.stopUpdatingLocation()
            started = false
        }
        manager.allowsBackgroundLocationUpdates = false
        manager.showsBackgroundLocationIndicator = false
    }

    private func endAll(_ event: Event) {
        let prior = Array(leases.values)
        leases = [:]
        notified.removeAll()
        stopManager()
        for lease in prior { lease.receive(event) }
    }

    private func send(_ event: Event, to consumer: Consumer, lease: Lease) {
        guard leases[consumer]?.id == lease.id else { return }
        lease.receive(event)
    }

    private func updateAuthorization() {
        guard !leases.isEmpty else { return }
        switch manager.authorizationStatus {
        case .notDetermined:
            if started { stopManager() }
            guard !prompted, UIApplication.shared.applicationState == .active else { return }
            prompted = true
            for (consumer, lease) in leases { send(.requestingPermission, to: consumer, lease: lease) }
            if !leases.isEmpty { manager.requestWhenInUseAuthorization() }
        case .restricted, .denied:
            endAll(.permissionDenied)
        case .authorizedWhenInUse, .authorizedAlways:
            guard manager.accuracyAuthorization == .fullAccuracy else {
                endAll(.permissionDenied)
                return
            }
            if !started {
                guard UIApplication.shared.applicationState == .active else { return }
                manager.allowsBackgroundLocationUpdates = true
                manager.showsBackgroundLocationIndicator = true
                started = true
                manager.startUpdatingLocation()
            }
            for (consumer, lease) in leases where notified.insert(lease.id).inserted {
                send(.active, to: consumer, lease: lease)
            }
        @unknown default:
            endAll(.serviceUnavailable)
        }
    }

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        updateAuthorization()
    }

    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard started else { return }
        if let lease = leases[.navigation], let newest = locations.last {
            send(.fix(newest), to: .navigation, lease: lease)
        }
        if let lease = leases[.driver] {
            for fix in locations { send(.fix(fix), to: .driver, lease: lease) }
        }
    }

    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        if (error as? CLError)?.code == .denied {
            endAll(.permissionDenied)
        } else {
            for (consumer, lease) in leases { send(.weakSignal, to: consumer, lease: lease) }
        }
    }
}
