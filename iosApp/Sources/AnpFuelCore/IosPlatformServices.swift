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

/// One localized turn prompt at a time. Other audio is ducked only during speech.
#if os(iOS) && canImport(AVFoundation)
@MainActor
public final class IosVoiceGuidance: NSObject, @preconcurrency AVSpeechSynthesizerDelegate {
    private let synthesizer = AVSpeechSynthesizer()
    private var current: AVSpeechUtterance?
    private var audioActive = false

    public override init() {
        super.init()
        synthesizer.delegate = self
    }

    public func speak(instruction: String, localeCode: String) -> Bool {
        let language = localeCode == "zh-Hans" ? "zh-CN" : localeCode
        let base = language.split(separator: "-").first
        let selected = AVSpeechSynthesisVoice(language: language)
            ?? AVSpeechSynthesisVoice.speechVoices().first { $0.language.split(separator: "-").first == base }
        guard !instruction.isEmpty, let selected else { return false }
        stop()
        do {
            let audio = AVAudioSession.sharedInstance()
            try audio.setCategory(.playback, mode: .voicePrompt, options: [.duckOthers])
            try audio.setActive(true)
            audioActive = true
        } catch {
            return false
        }
        let utterance = AVSpeechUtterance(string: instruction)
        utterance.voice = selected
        utterance.rate = AVSpeechUtteranceDefaultSpeechRate
        current = utterance
        synthesizer.speak(utterance)
        return true
    }

    public func stop() {
        current = nil
        if synthesizer.isSpeaking { synthesizer.stopSpeaking(at: .immediate) }
        releaseAudio()
    }

    public func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        finish(utterance)
    }

    public func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
        finish(utterance)
    }

    private func finish(_ utterance: AVSpeechUtterance) {
        guard current === utterance else { return }
        current = nil
        releaseAudio()
    }

    private func releaseAudio() {
        guard audioActive else { return }
        audioActive = false
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }
}
#else
@MainActor
public final class IosVoiceGuidance {
    public init() {}
    public func speak(instruction: String, localeCode: String) -> Bool { false }
    public func stop() {}
}
#endif

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
