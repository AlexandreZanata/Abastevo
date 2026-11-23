import Foundation
#if canImport(ImageIO)
import ImageIO
#endif
#if canImport(CoreGraphics)
import CoreGraphics
#endif

/// Portable lightweight-photo pipeline and native ImageIO + backup-excluded transient cache (P15-T02A / SEC05-C/D).
/// Zero postfix unwrap operators anywhere in this file.
public struct PhotoDims: Equatable {
    public let width: Int
    public let height: Int

    public init(width: Int, height: Int) {
        self.width = width
        self.height = height
    }
}

public struct PhotoEncodeRequest: Equatable {
    public let sampleSize: Int
    public let attempt: Int

    public init(sampleSize: Int, attempt: Int) {
        self.sampleSize = sampleSize
        self.attempt = attempt
    }
}

public protocol PhotoDecoder {
    func probeDims(bytes: Data) -> PhotoDims?
}

public protocol PhotoEncoder {
    func encode(source: Data, request: PhotoEncodeRequest) -> Data?
}

public protocol PhotoCache {
    func put(id: String, bytes: Data, capturedAtMillis: Int64)
    func get(id: String) -> Data?
    func delete(id: String)
    func sweepExpired(nowMillis: Int64) -> Int
}

public protocol PhotoClock {
    func nowMillis() -> Int64
}

public protocol PhotoIdSource {
    func nextId() -> String
}

public struct PhotoPorts {
    public let decoder: PhotoDecoder
    public let encoder: PhotoEncoder
    public let cache: PhotoCache
    public let clock: PhotoClock
    public let ids: PhotoIdSource

    public init(
        decoder: PhotoDecoder,
        encoder: PhotoEncoder,
        cache: PhotoCache,
        clock: PhotoClock,
        ids: PhotoIdSource
    ) {
        self.decoder = decoder
        self.encoder = encoder
        self.cache = cache
        self.clock = clock
        self.ids = ids
    }
}

public enum PhotoResult: Equatable {
    case ready(id: String, bytes: Int64, attempts: Int)
    case refused(code: String)
}

public final class PhotoFlow {
    private let ports: PhotoPorts

    public init(ports: PhotoPorts) {
        self.ports = ports
    }

    public func prepare(intentBytes: Data, mime: String) -> PhotoResult {
        return prepareAt(intentBytes: intentBytes, mime: mime, capturedAtMillis: ports.clock.nowMillis())
    }

    public func prepareAt(intentBytes: Data, mime: String, capturedAtMillis: Int64) -> PhotoResult {
        let now = ports.clock.nowMillis()
        if capturedAtMillis <= 0 || capturedAtMillis > now || now - capturedAtMillis >= PortablePhoto.transientTTLMillis {
            return .refused(code: "photo.expired")
        }
        if intentBytes.isEmpty || intentBytes.count > 32 * 1024 * 1024 {
            return .refused(code: PortablePhoto.undecodableInput)
        }
        if !PortablePhoto.isSupportedIntentMime(mime) {
            return .refused(code: PortablePhoto.unsupportedFormat)
        }
        guard let dims = ports.decoder.probeDims(bytes: intentBytes) else {
            return .refused(code: PortablePhoto.undecodableInput)
        }
        if dims.width <= 0 || dims.height <= 0 || Int64(dims.width) * Int64(dims.height) > 100_000_000 {
            return .refused(code: PortablePhoto.undecodableInput)
        }
        let sampleSize = PortablePhoto.sampleSizeForBounds(width: dims.width, height: dims.height)
        var sawBytes = false
        for attempt in 1...PortablePhoto.maxAttempts {
            guard let encoded = ports.encoder.encode(
                source: intentBytes,
                request: PhotoEncodeRequest(sampleSize: sampleSize, attempt: attempt)
            ) else {
                continue
            }
            if !PortablePhoto.fitsWireCap(Int64(encoded.count)) {
                sawBytes = true
                continue
            }
            let id = ports.ids.nextId()
            ports.cache.put(id: id, bytes: encoded, capturedAtMillis: capturedAtMillis)
            return .ready(id: id, bytes: Int64(encoded.count), attempts: attempt)
        }
        if sawBytes {
            return .refused(code: PortablePhoto.overBudget)
        }
        return .refused(code: PortablePhoto.encodeFailed)
    }

    public func discard(id: String) {
        ports.cache.delete(id: id)
    }

    public func sweepExpired() -> Int {
        return ports.cache.sweepExpired(nowMillis: ports.clock.nowMillis())
    }
}

/// Native ImageIO header probe and metadata-stripping JPEG encoder for iOS.
public final class IosImageIoPhotoCodec: PhotoDecoder, PhotoEncoder {
    private static let qualitySteps: [Double] = [0.82, 0.65, 0.48]

    public init() {}

    public func probeDims(bytes: Data) -> PhotoDims? {
        #if canImport(ImageIO)
        guard let source = CGImageSourceCreateWithData(bytes as CFData, nil) else {
            return nil
        }
        if CGImageSourceGetCount(source) > 1 {
            return nil
        }
        guard let props = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
              let w = props[kCGImagePropertyPixelWidth] as? Int,
              let h = props[kCGImagePropertyPixelHeight] as? Int,
              w > 0, h > 0 else {
            return nil
        }
        return PhotoDims(width: w, height: h)
        #else
        return nil
        #endif
    }

    public func encode(source: Data, request: PhotoEncodeRequest) -> Data? {
        #if canImport(ImageIO) && canImport(CoreGraphics)
        guard let cgSource = CGImageSourceCreateWithData(source as CFData, nil),
              let dims = probeDims(bytes: source) else {
            return nil
        }
        let maxSide = max(
            1,
            min(
                PortablePhoto.maxEdgePixels,
                max(dims.width, dims.height) / max(1, request.sampleSize)
            )
        )
        let thumbOptions: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceShouldCacheImmediately: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceThumbnailMaxPixelSize: maxSide,
        ]
        guard let cgImage = CGImageSourceCreateThumbnailAtIndex(cgSource, 0, thumbOptions as CFDictionary) else {
            return nil
        }
        let stepIndex = max(0, min(Self.qualitySteps.count - 1, request.attempt - 1))
        let quality = Self.qualitySteps[stepIndex]
        let outData = NSMutableData()
        guard let destination = CGImageDestinationCreateWithData(
            outData as CFMutableData,
            "public.jpeg" as CFString,
            1,
            nil
        ) else {
            return nil
        }
        let encodeProps: [CFString: Any] = [
            kCGImageDestinationLossyCompressionQuality: quality,
            kCGImagePropertyExifDictionary: [:],
            kCGImagePropertyGPSDictionary: [:],
        ]
        CGImageDestinationAddImage(destination, cgImage, encodeProps as CFDictionary)
        guard CGImageDestinationFinalize(destination) else {
            return nil
        }
        return outData as Data
        #else
        return nil
        #endif
    }
}

/// Private transient photo cache with atomic writes, 24-hour expiry, and backup exclusion (SEC05-C/D).
public final class IosTransientPhotoCache: PhotoCache {
    private struct Envelope: Codable {
        let capturedAtMillis: Int64
        let payloadBase64: String
    }

    private let directory: URL
    private let clock: () -> Int64
    private let lock = NSLock()
    private static let tempGraceMillis: Int64 = 3_600_000
    private static let futureSkewGraceMillis: Int64 = 3_600_000

    public init(directory: URL, clock: @escaping () -> Int64) {
        self.directory = directory
        self.clock = clock
        ensureDirectory()
    }

    public static func defaultDirectory() -> URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        return base.appendingPathComponent("anpfuel-transient-photos", isDirectory: true)
    }

    public func put(id: String, bytes: Data, capturedAtMillis: Int64) {
        lock.lock()
        defer { lock.unlock() }
        ensureDirectory()
        let safeId = sanitizeId(id)
        let target = directory.appendingPathComponent("\(safeId).json")
        let temp = directory.appendingPathComponent("\(safeId).tmp")
        let envelope = Envelope(capturedAtMillis: capturedAtMillis, payloadBase64: bytes.base64EncodedString())
        guard let encoded = try? JSONEncoder().encode(envelope) else { return }
        try? encoded.write(to: temp, options: .atomic)
        try? FileManager.default.removeItem(at: target)
        try? FileManager.default.moveItem(at: temp, to: target)
        markBackupExcluded(target)
    }

    public func get(id: String) -> Data? {
        lock.lock()
        defer { lock.unlock() }
        let safeId = sanitizeId(id)
        let file = directory.appendingPathComponent("\(safeId).json")
        guard let raw = try? Data(contentsOf: file),
              let envelope = try? JSONDecoder().decode(Envelope.self, from: raw) else {
            try? FileManager.default.removeItem(at: file)
            return nil
        }
        let now = clock()
        if PortablePhoto.isTransientExpired(capturedAtMillis: envelope.capturedAtMillis, nowMillis: now)
            || envelope.capturedAtMillis - now > Self.futureSkewGraceMillis {
            try? FileManager.default.removeItem(at: file)
            return nil
        }
        guard let payload = Data(base64Encoded: envelope.payloadBase64) else {
            try? FileManager.default.removeItem(at: file)
            return nil
        }
        return payload
    }

    public func delete(id: String) {
        lock.lock()
        defer { lock.unlock() }
        let safeId = sanitizeId(id)
        try? FileManager.default.removeItem(at: directory.appendingPathComponent("\(safeId).json"))
        try? FileManager.default.removeItem(at: directory.appendingPathComponent("\(safeId).tmp"))
    }

    public func sweepExpired(nowMillis: Int64) -> Int {
        lock.lock()
        defer { lock.unlock() }
        guard let files = try? FileManager.default.contentsOfDirectory(
            at: directory,
            includingPropertiesForKeys: [.contentModificationDateKey]
        ) else {
            return 0
        }
        var purged = 0
        for file in files {
            if file.pathExtension == "tmp" {
                let attrs = try? file.resourceValues(forKeys: [.contentModificationDateKey])
                if let mtime = attrs?.contentModificationDate {
                    let ageMillis = nowMillis - Int64(mtime.timeIntervalSince1970 * 1000)
                    if ageMillis >= Self.tempGraceMillis {
                        try? FileManager.default.removeItem(at: file)
                        purged += 1
                    }
                }
                continue
            }
            guard file.pathExtension == "json" else { continue }
            guard let raw = try? Data(contentsOf: file),
                  let envelope = try? JSONDecoder().decode(Envelope.self, from: raw) else {
                try? FileManager.default.removeItem(at: file)
                purged += 1
                continue
            }
            if PortablePhoto.isTransientExpired(capturedAtMillis: envelope.capturedAtMillis, nowMillis: nowMillis)
                || envelope.capturedAtMillis - nowMillis > Self.futureSkewGraceMillis {
                try? FileManager.default.removeItem(at: file)
                purged += 1
            }
        }
        return purged
    }

    public func purgeAll() {
        lock.lock()
        defer { lock.unlock() }
        guard let files = try? FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil) else {
            return
        }
        for file in files {
            try? FileManager.default.removeItem(at: file)
        }
    }

    private func ensureDirectory() {
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        markBackupExcluded(directory)
    }

    private func markBackupExcluded(_ url: URL) {
        var mutableUrl = url
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableUrl.setResourceValues(values)
    }

    private func sanitizeId(_ id: String) -> String {
        let allowed = id.unicodeScalars.filter {
            ($0 >= "a" && $0 <= "z") || ($0 >= "A" && $0 <= "Z") || ($0 >= "0" && $0 <= "9") || $0 == "-" || $0 == "_"
        }
        let cleaned = String(String.UnicodeScalarView(allowed))
        return cleaned.isEmpty ? "empty" : cleaned
    }
}
