import Foundation
import CoreLocation
#if canImport(AVFoundation)
import AVFoundation
#endif

/// Native iOS one-shot location, and voice guidance.
/// Zero postfix unwrap operators anywhere in this file.
/// Zero continuous or background location APIs anywhere in this file.
public final class IosLocationManager: NSObject, CLLocationManagerDelegate {
    private let manager: CLLocationManager?
    private var pendingContinuation: CheckedContinuation<(LocationSignalReading, CLLocationCoordinate2D?), Never>?
    private var waitingForAuth = false
    private let lock = NSLock()

    public override init() {
        #if os(iOS)
        let mgr = CLLocationManager()
        self.manager = mgr
        super.init()
        mgr.delegate = self
        mgr.desiredAccuracy = kCLLocationAccuracyBest
        #else
        self.manager = nil
        super.init()
        #endif
    }

    public func currentAuthorizationStatus() -> Bool {
        #if os(iOS)
        guard let mgr = manager else { return false }
        let status = mgr.authorizationStatus
        return status == .authorizedWhenInUse || status == .authorizedAlways
        #else
        return false
        #endif
    }

    public func requestOneShotFix() async -> (LocationSignalReading, CLLocationCoordinate2D?) {
        #if os(iOS)
        guard let mgr = manager else {
            return (LocationReading.denied(), nil)
        }
        let status = mgr.authorizationStatus
        if status == .denied || status == .restricted {
            return (LocationReading.denied(), nil)
        }
        return await withCheckedContinuation { continuation in
            lock.lock()
            if self.pendingContinuation != nil {
                lock.unlock()
                continuation.resume(returning: (LocationReading.absent(), nil))
                return
            }
            self.pendingContinuation = continuation
            if status == .notDetermined {
                self.waitingForAuth = true
                lock.unlock()
                mgr.requestWhenInUseAuthorization()
            } else {
                self.waitingForAuth = false
                lock.unlock()
                mgr.requestLocation()
            }
        }
        #else
        return (LocationReading.absent(), nil)
        #endif
    }

    public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        #if os(iOS)
        lock.lock()
        let shouldHandle = waitingForAuth && pendingContinuation != nil
        let status = manager.authorizationStatus
        if !shouldHandle || status == .notDetermined {
            lock.unlock()
            return
        }
        waitingForAuth = false
        if status == .authorizedWhenInUse || status == .authorizedAlways {
            lock.unlock()
            manager.requestLocation()
        } else {
            let cont = pendingContinuation
            pendingContinuation = nil
            lock.unlock()
            cont?.resume(returning: (LocationReading.denied(), nil))
        }
        #endif
    }

    public func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        lock.lock()
        let continuation = pendingContinuation
        pendingContinuation = nil
        lock.unlock()
        guard let cont = continuation else { return }
        guard let fix = locations.last else {
            cont.resume(returning: (LocationReading.absent(), nil))
            return
        }
        let simulated: Bool?
        if #available(iOS 15.0, macOS 12.0, *) {
            simulated = fix.sourceInformation?.isSimulatedBySoftware
        } else {
            simulated = nil
        }
        let ageSeconds = max(0, Int64(-fix.timestamp.timeIntervalSinceNow))
        let reading = LocationReading.fromFix(
            permissionGranted: true,
            isSimulatedBySoftware: simulated,
            horizontalAccuracy: fix.horizontalAccuracy,
            fixAgeSeconds: ageSeconds
        )
        cont.resume(returning: (reading, fix.coordinate))
    }

    public func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        lock.lock()
        let continuation = pendingContinuation
        pendingContinuation = nil
        lock.unlock()
        continuation?.resume(returning: (LocationReading.absent(), nil))
    }
}

/// AVSpeechSynthesizer voice guidance adapter supporting all 8 Abastevo locales.
public final class IosVoiceGuidance {
    #if canImport(AVFoundation)
    private let synthesizer = AVSpeechSynthesizer()
    #endif

    public init() {}

    public func speak(instruction: String, localeCode: String) {
        #if canImport(AVFoundation)
        if synthesizer.isSpeaking {
            synthesizer.stopSpeaking(at: .immediate)
        }
        let utterance = AVSpeechUtterance(string: instruction)
        utterance.voice = AVSpeechSynthesisVoice(language: localeCode)
        utterance.rate = AVSpeechUtteranceDefaultSpeechRate
        synthesizer.speak(utterance)
        #endif
    }

    public func stop() {
        #if canImport(AVFoundation)
        synthesizer.stopSpeaking(at: .immediate)
        #endif
    }
}

enum CoreLocalization {
    static let bundle: Bundle = {
        #if SWIFT_PACKAGE
        return .module
        #else
        return Bundle(for: LocalizationBundleMarker.self)
        #endif
    }()

    private final class LocalizationBundleMarker: NSObject {}
}
