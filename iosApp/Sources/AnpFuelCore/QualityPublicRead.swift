import Foundation

public struct QualityReadScope: Equatable, Hashable, Sendable {
    public let origin: String
    public let accountId: String?

    public init(origin: String, accountId: String?) {
        self.origin = origin
        self.accountId = accountId
    }

    fileprivate var storageKey: String {
        IosDeviceProofSigner.sha256Hex(Data((origin + "\u{0}" + (accountId ?? "")).utf8))
    }
}

public enum QualityReadError: Error, Equatable, Sendable {
    case invalid
    case unavailable
    case disabled
}

/// Only admitted public data gets a cache entry. All disk operations and purge share the same lock.
public final class QualityPublicCache: @unchecked Sendable {
    static let maxBodyBytes = 1_024 * 1_024
    private static let envelopeBytes = 2 * maxBodyBytes
    private static let retention: TimeInterval = 7 * 24 * 60 * 60
    private let directory: URL
    private let lock = NSLock()
    private var generation = UUID()

    struct Entry: Codable, Sendable {
        let body: Data
        let sha256: String
        let etag: String?
        let savedAt: Date
    }

    public init(directory: URL = IosLocalStore.defaultDirectory()) {
        self.directory = directory.appendingPathComponent("official-quality-cache", isDirectory: true)
    }

    func ticket() -> UUID { lock.withLock { generation } }

    func isCurrent(_ ticket: UUID) -> Bool { lock.withLock { generation == ticket } }

    func load(scope: QualityReadScope, url: URL, ticket: UUID, now: Date = Date()) -> Entry? {
        lock.withLock {
            guard generation == ticket else { return nil }
            let file = path(scope: scope, url: url)
            let size = try? file.resourceValues(forKeys: [.fileSizeKey]).fileSize
            guard let size, size <= Self.envelopeBytes,
                  let data = try? Data(contentsOf: file), data.count <= Self.envelopeBytes,
                  let entry = try? JSONDecoder().decode(Entry.self, from: data),
                  entry.body.count <= Self.maxBodyBytes, now >= entry.savedAt,
                  now.timeIntervalSince(entry.savedAt) < Self.retention,
                  IosDeviceProofSigner.sha256Hex(entry.body) == entry.sha256 else {
                try? FileManager.default.removeItem(at: file)
                return nil
            }
            return entry
        }
    }

    func save(scope: QualityReadScope, url: URL, body: Data, etag: String?, ticket: UUID, now: Date = Date()) {
        lock.withLock {
            guard generation == ticket, !body.isEmpty, body.count <= Self.maxBodyBytes else { return }
            let entry = Entry(body: body, sha256: IosDeviceProofSigner.sha256Hex(body), etag: etag, savedAt: now)
            let file = path(scope: scope, url: url)
            let container = file.deletingLastPathComponent()
            do {
                try FileManager.default.createDirectory(at: container, withIntermediateDirectories: true)
                try Self.excludeFromBackup(directory)
                try Self.excludeFromBackup(container)
                let data = try JSONEncoder().encode(entry)
                #if os(iOS)
                try data.write(to: file, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
                #else
                try data.write(to: file, options: .atomic)
                #endif
                try Self.excludeFromBackup(file)
                prune(container)
            } catch {
                try? FileManager.default.removeItem(at: file)
            }
        }
    }

    func remove(scope: QualityReadScope, url: URL, ticket: UUID) {
        lock.withLock {
            guard generation == ticket else { return }
            try? FileManager.default.removeItem(at: path(scope: scope, url: url))
        }
    }

    func invalidate(scope: QualityReadScope, ticket: UUID) {
        lock.withLock {
            guard generation == ticket else { return }
            generation = UUID()
            try? FileManager.default.removeItem(at: directory.appendingPathComponent(scope.storageKey, isDirectory: true))
        }
    }

    public func purge() {
        lock.withLock {
            generation = UUID()
            try? FileManager.default.removeItem(at: directory)
        }
    }

    private func path(scope: QualityReadScope, url: URL) -> URL {
        let key = IosDeviceProofSigner.sha256Hex(Data(url.absoluteString.utf8))
        return directory.appendingPathComponent(scope.storageKey, isDirectory: true).appendingPathComponent(key)
    }

    private func prune(_ container: URL) {
        let keys: Set<URLResourceKey> = [.contentModificationDateKey, .fileSizeKey]
        guard let files = try? FileManager.default.contentsOfDirectory(at: container, includingPropertiesForKeys: Array(keys)) else { return }
        let ordered = files.compactMap { file -> (URL, Date, Int)? in
            guard let values = try? file.resourceValues(forKeys: keys),
                  let modified = values.contentModificationDate, let size = values.fileSize else { return nil }
            return (file, modified, size)
        }.sorted { $0.1 > $1.1 }
        var bytes = 0
        for (offset, entry) in ordered.enumerated() {
            bytes += entry.2
            if offset >= 64 || bytes > 16 * Self.maxBodyBytes {
                try? FileManager.default.removeItem(at: entry.0)
            }
        }
    }

    private static func excludeFromBackup(_ path: URL) throws {
        var target = path
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try target.setResourceValues(values)
    }
}

private final class QualityNoRedirect: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}

/// Uses the existing anonymous URLSession with an origin-fixed request and streaming size limit.
public struct QualityPublicReader {
    private let client: AbastevoHttpClient
    private let cache: QualityPublicCache
    private let scope: QualityReadScope
    private let offline: Bool

    public init(client: AbastevoHttpClient, cache: QualityPublicCache, scope: QualityReadScope, offline: Bool = false) {
        self.client = client
        self.cache = cache
        self.scope = scope
        self.offline = offline
    }

    func read<Value>(
        _ route: String, query: [URLQueryItem] = [], detail: String? = nil,
        decode: (Data, Bool) throws -> Value
    ) async throws -> Value {
        guard client.environment.origin == scope.origin, var components = URLComponents(string: scope.origin) else {
            throw QualityReadError.unavailable
        }
        components.path = "/v1/quality/" + route
        if let detail {
            let allowed = CharacterSet.urlPathAllowed.subtracting(CharacterSet(charactersIn: "/%?#"))
            guard QualityWire.validSampleReference(detail), detail != ".", detail != "..",
                  let encoded = detail.addingPercentEncoding(withAllowedCharacters: allowed) else {
                throw QualityReadError.invalid
            }
            components.percentEncodedPath += "/" + encoded
        }
        components.queryItems = query.isEmpty ? nil : query
        guard let url = components.url else { throw QualityReadError.invalid }
        let ticket = cache.ticket()
        let cached = cache.load(scope: scope, url: url, ticket: ticket)
        let response = try await fetch(url: url, cached: cached, ticket: ticket)
        guard cache.isCurrent(ticket), client.environment.origin == scope.origin else { throw CancellationError() }
        do {
            let result = try decode(response.body, response.stale)
            if response.shouldSave {
                cache.save(scope: scope, url: url, body: response.body, etag: response.etag, ticket: ticket)
            }
            return result
        } catch {
            cache.remove(scope: scope, url: url, ticket: ticket)
            throw error
        }
    }

    private struct Read {
        let body: Data
        let etag: String?
        let stale: Bool
        let shouldSave: Bool
    }

    private func retained(_ cached: QualityPublicCache.Entry?) throws -> Read {
        guard let cached else { throw QualityReadError.unavailable }
        return Read(body: cached.body, etag: cached.etag, stale: true, shouldSave: false)
    }

    private func fetch(url: URL, cached: QualityPublicCache.Entry?, ticket: UUID) async throws -> Read {
        if offline { return try retained(cached) }
        var request = URLRequest(url: url)
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("no-cache", forHTTPHeaderField: "Cache-Control")
        if let etag = cached?.etag { request.setValue(etag, forHTTPHeaderField: "If-None-Match") }
        do {
            let (stream, response) = try await client.session.bytes(for: request, delegate: QualityNoRedirect())
            guard let http = response as? HTTPURLResponse, http.url == url else { throw QualityReadError.unavailable }
            if http.statusCode >= 500 { return try retained(cached) }
            let cacheable = !(http.value(forHTTPHeaderField: "Cache-Control") ?? "")
                .lowercased().split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }.contains("no-store")
            if !cacheable { cache.remove(scope: scope, url: url, ticket: ticket) }
            if http.statusCode == 304 {
                guard cacheable, let cached else { throw QualityReadError.unavailable }
                return Read(body: cached.body, etag: cached.etag, stale: false, shouldSave: false)
            }
            guard http.statusCode == 200 else {
                cache.invalidate(scope: scope, ticket: ticket)
                throw http.statusCode == 404 ? QualityReadError.disabled : QualityReadError.unavailable
            }
            var data = Data()
            for try await byte in stream {
                guard data.count < QualityPublicCache.maxBodyBytes else { throw QualityReadError.invalid }
                data.append(byte)
            }
            let etag = http.value(forHTTPHeaderField: "ETag").flatMap { value in
                value.utf8.count <= 200 && !value.unicodeScalars.contains(where: CharacterSet.controlCharacters.contains) ? value : nil
            }
            return Read(body: data, etag: etag, stale: false, shouldSave: cacheable)
        } catch is CancellationError {
            throw CancellationError()
        } catch let error as URLError {
            if error.code == .cancelled { throw CancellationError() }
            return try retained(cached)
        }
    }
}
